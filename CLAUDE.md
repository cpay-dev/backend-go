# CPay.dev

Crypto payment platform on Polygon. Merchants publish products, create invoices, payment links, and recurring subscriptions using counterfactual accounts.

## Architecture

- **Go backend** (`backend/`): chi router, sqlc + pgx (PostgreSQL), SIWE auth, MinIO storage, NATS JetStream
- **Rust chain service** (`chain-service/`): alloy.rs + tonic gRPC, Polygon RPC provider
- **Next.js frontend** (`frontend/`): App Router, wagmi + viem, WalletConnect v2, shadcn/ui
- **Infrastructure**: PostgreSQL 18, NATS, MinIO — all via Docker Compose

## Key Commands

```bash
make dev          # docker compose up --build (all services)
make dev-db       # start only postgres, nats, minio
make sqlc         # regenerate Go DB code (runs in Docker)
make migrate      # run migrations (requires running postgres)
make build-go     # compile Go backend
make frontend-dev # next.js dev server
```

## Code Generation

- sqlc: `make sqlc` — queries in `backend/queries/`, output in `backend/internal/db/`
- Proto: `proto/chain/v1/chain_service.proto` — Go stubs in `backend/internal/grpc/gen/chainv1/`, Rust stubs via tonic-build

## Database

- Migrations in `migrations/` (golang-migrate format)
- Module: `github.com/cpay-dev/backend`
- sqlc config: `backend/sqlc.yaml`

## Auth Flow

SIWE (Sign-In with Ethereum) → Go backend verifies signature → issues JWT (HS256, 24h). JWT claims: sub, wallet, role.

## Reference

Patterns adapted from sibling project at `../streaming-yield/`.
