# CPay Backend (Stripe-like Crypto Payment Links)

Backend-first crypto payment platform in Go 1.26 with Postgres 18, MinIO, NATS and Docker.
External API is REST on `api-gateway`; internal service-to-service communication is gRPC.

## Implemented foundations

- Microservices:
  - `api-gateway`
  - `auth-service`
  - `catalog-service`
  - `payment-link-service`
  - `checkout-service`
  - `chain-observer-service`
  - `webhook-service`
  - `email-service`
  - `payout-service`
  - `subscription-service`
  - `outbox-relay-service`
- Internal gRPC contracts:
  - `AuthService`
  - `PaymentLinkService`
  - `CheckoutService`
  - Generated stubs live under `internal/gen/cpay/v1`
- Shared platform packages:
  - config loader
  - zerolog bootstrap
  - request-id + idempotency middleware
  - JWT + API key utilities
  - EVM chain adapter interface + implementation
  - swap adapter interface (M3 contract)
  - encryption helpers (AES-GCM)
  - event envelope schema
- Postgres schema-per-service:
  - `auth`: merchants, users, api_keys
  - `catalog`: products, payment_links, link_options
  - `checkout`: checkout/payment/webhook/payout/subscription tables
  - `platform`: outbox_events, idempotency_keys

## API coverage (M1 + recurring primitives)

- Auth
  - `POST /v1/auth/signup` (complete wallet/OAuth onboarding)
  - `POST /v1/auth/refresh`
  - `POST /v1/auth/google/start`
  - `POST /v1/auth/google/consume`
  - `POST /v1/auth/wallet/challenge`
  - `POST /v1/auth/wallet/verify`
  - `POST /v1/auth/passkeys/login/options`
  - `POST /v1/auth/passkeys/login/verify`
  - `POST /v1/auth/passkeys/register/options`
  - `POST /v1/auth/passkeys/register/verify`
- Profile security
  - `GET /v1/profile/security`
  - `POST /v1/profile/security/google/consume`
  - `POST /v1/profile/security/wallet/verify`
  - `DELETE /v1/profile/security/identities/{id}`
  - `DELETE /v1/profile/security/passkeys/{id}`
- API keys
  - `POST /v1/api_keys`
  - `GET /v1/api_keys`
  - `POST /v1/api_keys/{id}/revoke`
- Merchant settings
  - `GET /v1/merchant/settings`
  - `PATCH /v1/merchant/settings`
- Products
  - `POST /v1/products`
  - `GET /v1/products`
  - `GET /v1/products/{id}`
  - `POST /v1/products/{id}`
  - `DELETE /v1/products/{id}`
- Payment links
  - `POST /v1/payment_links`
  - `GET /v1/payment_links`
  - `GET /v1/payment_links/{id}`
  - `POST /v1/payment_links/{id}/archive`
- Checkout / payments
  - `POST /v1/payment_links/{id}/sessions`
  - `POST /v1/public/payment_links/{id}/sessions`
  - `GET /v1/checkout/{session_id}`
  - `GET /v1/public/checkout/{session_id}?client_secret=...`
  - `POST /v1/checkout/{session_id}/confirm`
  - `GET /v1/payments/{id}`
- Webhooks
  - `POST /v1/webhook_endpoints`
- Subscriptions
  - `POST /v1/subscriptions`
  - `POST /v1/subscriptions/{id}/pause`
  - `POST /v1/subscriptions/{id}/resume`
  - `GET /v1/subscriptions/{id}/cycles`

## Event + worker pipeline

- Transactional outbox table is written by service handlers (`platform.outbox_events`).
- `outbox-relay-service` publishes outbox events to NATS subjects (`events.<event_type>`).
- `webhook-service` subscribes to NATS and delivers signed merchant webhooks with retries.
- `chain-observer-service` expires stale pending payment intents.
- `payout-service` schedules/completes payouts and marks intents settled.
- `subscription-service` processes due cycles against prepaid vault balances.

Payouts run in production mode by default. Chainlist-sourced public RPC defaults are built in for Ethereum, Polygon, Arbitrum One, Base, Avalanche C-Chain, BNB Smart Chain, Optimism, and HyperEVM; configure `CHAIN_RPC_URLS` as a comma-separated map such as `base=https://...` to override or add chain URLs. Set each merchant's `settlement_address` via `/v1/merchant/settings`. New EVM checkout sessions require CREATE2 checkout wallets: deploy `contracts/CheckoutWalletFactory.sol` per EVM chain with the payout hot wallet as `initialSweeper`, then configure `CHECKOUT_WALLET_FACTORY_ADDRESSES`, `PAYOUT_HOT_WALLET_PRIVATE_KEY`, and optional `PAYOUT_GAS_BUFFER_PERCENT`. Generic CREATE2 deployers, including ERC-2470-style singleton factories, are deployment helpers only; `CHECKOUT_WALLET_FACTORY_ADDRESSES` must point to this repo's `CheckoutWalletFactory` because payouts call its `deployAndSweep(...)` ABI. Existing legacy EOA deposit rows are not retried as automatic payouts after CREATE2-only mode and require manual cleanup. Use `PAYOUT_MODE=mock` only for local/demo runs that should preserve mocked sweep hashes.

## Local run

### 1. Bring up stack

```bash
docker compose up -d --build
```

### 2. Health checks

- API gateway: `http://localhost:8080/health`
- Auth service: `http://localhost:8086/health` (gRPC `:9091`)
- Payment-link service: `http://localhost:8088/health` (gRPC `:9092`)
- Checkout service: `http://localhost:8089/health` (gRPC `:9093`)
- Outbox relay: `http://localhost:8081/health`
- Webhook service: `http://localhost:8082/health`
- Email service: `http://localhost:8090/health`

### 3. Default bootstrap account

- Email: `admin@cpay.dev`

Password sign-in is disabled. Use wallet, OAuth, or passkey sign-in.

You can change the bootstrap account metadata using env vars:
- `BOOTSTRAP_ADMIN_EMAIL`
- `BOOTSTRAP_MERCHANT_NAME`

Google OAuth and passkeys use:
- `PUBLIC_WEB_ORIGIN`
- `WEBAUTHN_RP_ID`
- `WEBAUTHN_RP_NAME`
- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `GOOGLE_REDIRECT_URI`

## Dev commands

```bash
make proto
make proto-docker
make tidy
make fmt
make test
make run
```

## Migrations

- Raw SQL migration files are stored in `migrations/`.
- Migrations are applied with `golang-migrate` from code (`internal/platform/migrate`).
- Run manually: `make migrate`

## Email

- Email delivery is handled by `email-service` using the Resend Go SDK.
- Configure:
  - `RESEND_API_KEY`
  - `EMAIL_FROM`
  - `EMAIL_REPLY_TO` (optional)
- Triggered events:
  - `user.signed_up`
  - `invoice.created`
  - `payment.confirmed`
  - `payment.expired` / `payment.failed`

## Notes

- Recurring model is prepaid-vault based.
- Payment tolerance is set to `0.25%` by default (`DEFAULT_TOLERANCE_PERCENT`).
- Swap interface is defined but swap execution is intentionally deferred (M3).
