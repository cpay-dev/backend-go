CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS catalog;
CREATE SCHEMA IF NOT EXISTS checkout;
CREATE SCHEMA IF NOT EXISTS platform;

CREATE EXTENSION IF NOT EXISTS ulid;

CREATE TABLE IF NOT EXISTS auth.merchants (
    id ulid PRIMARY KEY,
    name TEXT NOT NULL,
    settlement_address TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth.users (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (merchant_id, email)
);

CREATE TABLE IF NOT EXISTS auth.api_keys (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (key_hash)
);

CREATE TABLE IF NOT EXISTS catalog.products (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    image_url TEXT,
    default_currency TEXT NOT NULL DEFAULT 'USD',
    default_amount NUMERIC(36, 18),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS catalog.payment_links (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    product_id ulid REFERENCES catalog.products(id) ON DELETE SET NULL,
    code TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    image_url TEXT,
    pricing_mode TEXT NOT NULL,
    amount NUMERIC(36, 18),
    currency TEXT NOT NULL,
    reusable BOOLEAN NOT NULL DEFAULT TRUE,
    max_payments INTEGER,
    expires_at TIMESTAMPTZ,
    cta_text TEXT NOT NULL DEFAULT 'Pay',
    after_payment_type TEXT NOT NULL DEFAULT 'confirmation_page',
    success_message TEXT,
    redirect_url TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    adjust_percent NUMERIC(12, 6) NOT NULL DEFAULT 0,
    min_amount NUMERIC(36, 18),
    max_amount NUMERIC(36, 18),
    allowed_tokens JSONB NOT NULL DEFAULT '[]'::jsonb,
    customer_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    custom_fields JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (code),
    CHECK (pricing_mode IN ('fixed', 'open')),
    CHECK (status IN ('active', 'archived')),
    CHECK (after_payment_type IN ('confirmation_page', 'redirect'))
);

CREATE TABLE IF NOT EXISTS catalog.link_options (
    payment_link_id ulid PRIMARY KEY REFERENCES catalog.payment_links(id) ON DELETE CASCADE,
    collect_email BOOLEAN NOT NULL DEFAULT FALSE,
    collect_name BOOLEAN NOT NULL DEFAULT FALSE,
    collect_phone BOOLEAN NOT NULL DEFAULT FALSE,
    collect_address BOOLEAN NOT NULL DEFAULT FALSE,
    collect_business_name BOOLEAN NOT NULL DEFAULT FALSE,
    require_terms_acceptance BOOLEAN NOT NULL DEFAULT FALSE,
    allow_promo_codes BOOLEAN NOT NULL DEFAULT FALSE,
    collect_tax_automatically BOOLEAN NOT NULL DEFAULT FALSE,
    add_invoice_pdf BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS checkout.checkout_sessions (
    id ulid PRIMARY KEY,
    payment_link_id ulid NOT NULL REFERENCES catalog.payment_links(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    customer_email TEXT,
    customer_name TEXT,
    customer_phone TEXT,
    customer_address JSONB,
    amount NUMERIC(36, 18) NOT NULL,
    currency TEXT NOT NULL,
    chain TEXT NOT NULL,
    token_symbol TEXT NOT NULL,
    token_address TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    success_url TEXT,
    client_secret_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('created', 'awaiting_funds', 'paid', 'expired', 'failed', 'canceled'))
);

CREATE TABLE IF NOT EXISTS checkout.payment_intents (
    id ulid PRIMARY KEY,
    checkout_session_id ulid NOT NULL REFERENCES checkout.checkout_sessions(id) ON DELETE CASCADE,
    payment_link_id ulid NOT NULL REFERENCES catalog.payment_links(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    chain TEXT NOT NULL,
    token_symbol TEXT NOT NULL,
    token_address TEXT,
    expected_amount NUMERIC(36, 18) NOT NULL,
    tolerance_percent NUMERIC(12, 6) NOT NULL DEFAULT 0.25,
    min_acceptable_amount NUMERIC(36, 18) NOT NULL,
    max_acceptable_amount NUMERIC(36, 18) NOT NULL,
    received_amount NUMERIC(36, 18) NOT NULL DEFAULT 0,
    tx_hash TEXT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    required_confirmations INTEGER NOT NULL,
    confirmed_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('created', 'awaiting_funds', 'partial', 'overpaid', 'confirmed', 'settled', 'expired', 'failed'))
);

CREATE TABLE IF NOT EXISTS checkout.deposit_addresses (
    id ulid PRIMARY KEY,
    payment_intent_id ulid NOT NULL UNIQUE REFERENCES checkout.payment_intents(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    chain TEXT NOT NULL,
    address TEXT NOT NULL,
    encrypted_private_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (chain, address),
    CHECK (status IN ('active', 'swept', 'compromised'))
);

CREATE TABLE IF NOT EXISTS checkout.chain_transactions (
    id ulid PRIMARY KEY,
    payment_intent_id ulid NOT NULL REFERENCES checkout.payment_intents(id) ON DELETE CASCADE,
    chain TEXT NOT NULL,
    tx_hash TEXT NOT NULL,
    block_number BIGINT,
    from_address TEXT,
    to_address TEXT,
    amount NUMERIC(36, 18) NOT NULL,
    token_symbol TEXT NOT NULL,
    token_address TEXT,
    confirmations INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    raw_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (chain, tx_hash, payment_intent_id),
    CHECK (status IN ('detected', 'confirmed', 'reorged'))
);

CREATE TABLE IF NOT EXISTS checkout.webhook_endpoints (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    events JSONB NOT NULL DEFAULT '[]'::jsonb,
    secret_encrypted TEXT NOT NULL,
    max_retries INTEGER NOT NULL DEFAULT 8,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS checkout.webhook_deliveries (
    id ulid PRIMARY KEY,
    webhook_endpoint_id ulid NOT NULL REFERENCES checkout.webhook_endpoints(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    event_id ulid NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    response_code INTEGER,
    response_body TEXT,
    next_retry_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('pending', 'scheduled', 'delivered', 'failed'))
);

CREATE TABLE IF NOT EXISTS checkout.invoices (
    id ulid PRIMARY KEY,
    payment_intent_id ulid NOT NULL UNIQUE REFERENCES checkout.payment_intents(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    amount NUMERIC(36, 18) NOT NULL,
    currency TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS checkout.payouts (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    schedule_at TIMESTAMPTZ NOT NULL,
    tx_hash TEXT,
    total_amount NUMERIC(36, 18) NOT NULL DEFAULT 0,
    fee_amount NUMERIC(36, 18) NOT NULL DEFAULT 0,
    token_symbol TEXT NOT NULL,
    chain TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CHECK (status IN ('scheduled', 'processing', 'completed', 'failed'))
);

CREATE TABLE IF NOT EXISTS checkout.payout_items (
    id ulid PRIMARY KEY,
    payout_id ulid NOT NULL REFERENCES checkout.payouts(id) ON DELETE CASCADE,
    payment_intent_id ulid NOT NULL UNIQUE REFERENCES checkout.payment_intents(id) ON DELETE CASCADE,
    amount NUMERIC(36, 18) NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    tx_hash TEXT,
    last_error TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE TABLE IF NOT EXISTS checkout.subscriptions (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    payment_link_id ulid REFERENCES catalog.payment_links(id) ON DELETE SET NULL,
    customer_ref TEXT,
    status TEXT NOT NULL,
    chain TEXT NOT NULL,
    token_symbol TEXT NOT NULL,
    token_address TEXT,
    amount NUMERIC(36, 18) NOT NULL,
    currency TEXT NOT NULL,
    interval_unit TEXT NOT NULL,
    interval_count INTEGER NOT NULL,
    next_billing_at TIMESTAMPTZ NOT NULL,
    vault_contract_address TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('active', 'paused', 'canceled', 'past_due')),
    CHECK (interval_unit IN ('day', 'week', 'month'))
);

CREATE TABLE IF NOT EXISTS checkout.subscription_cycles (
    id ulid PRIMARY KEY,
    subscription_id ulid NOT NULL REFERENCES checkout.subscriptions(id) ON DELETE CASCADE,
    cycle_index INTEGER NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    due_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    amount NUMERIC(36, 18) NOT NULL,
    payment_intent_id ulid REFERENCES checkout.payment_intents(id) ON DELETE SET NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (subscription_id, cycle_index),
    CHECK (status IN ('due', 'processing', 'paid', 'failed', 'skipped'))
);

CREATE TABLE IF NOT EXISTS checkout.vault_authorizations (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    subscription_id ulid NOT NULL UNIQUE REFERENCES checkout.subscriptions(id) ON DELETE CASCADE,
    chain TEXT NOT NULL,
    contract_address TEXT NOT NULL,
    customer_wallet TEXT NOT NULL,
    token_symbol TEXT NOT NULL,
    token_address TEXT,
    max_total_amount NUMERIC(36, 18) NOT NULL,
    remaining_amount NUMERIC(36, 18) NOT NULL,
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('active', 'revoked', 'expired'))
);

CREATE TABLE IF NOT EXISTS platform.outbox_events (
    id ulid PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    merchant_id ulid,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (status IN ('pending', 'published', 'failed'))
);

CREATE TABLE IF NOT EXISTS platform.idempotency_keys (
    id ulid PRIMARY KEY,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT,
    response_status INTEGER NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    UNIQUE (merchant_id, endpoint, idempotency_key)
);

CREATE TABLE IF NOT EXISTS platform.email_notifications (
    id ulid PRIMARY KEY,
    event_id ulid NOT NULL,
    event_type TEXT NOT NULL,
    merchant_id ulid,
    template TEXT NOT NULL,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    provider_message_id TEXT,
    last_error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, template, recipient),
    CHECK (status IN ('pending', 'sent', 'failed'))
);

ALTER TABLE checkout.payouts
    ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error TEXT;

ALTER TABLE checkout.payout_items
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS tx_hash TEXT,
    ADD COLUMN IF NOT EXISTS last_error TEXT,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'payout_items_status_check'
          AND conrelid = 'checkout.payout_items'::regclass
    ) THEN
        ALTER TABLE checkout.payout_items
            ADD CONSTRAINT payout_items_status_check
            CHECK (status IN ('pending', 'processing', 'completed', 'failed'));
    END IF;
END $$;

UPDATE checkout.payment_intents
SET required_confirmations = CASE
    WHEN lower(chain) IN ('ethereum', 'ethereum mainnet', 'mainnet') THEN 64
    WHEN lower(chain) = 'polygon' THEN 6
    WHEN lower(chain) IN ('arbitrum', 'arbitrum one') THEN 4800
    WHEN lower(chain) = 'base' THEN 600
    WHEN lower(chain) IN ('hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm') THEN 3
    WHEN lower(chain) IN ('bnb', 'bsc', 'bnb smart chain') THEN 6
    WHEN lower(chain) = 'optimism' THEN 600
    WHEN lower(chain) = 'solana' THEN 32
    WHEN lower(chain) = 'tron' THEN 21
    ELSE required_confirmations
END,
updated_at = NOW()
WHERE status IN ('created', 'awaiting_funds', 'partial', 'expired')
  AND (
    required_confirmations IN (1, 2, 3, 12, 15, 19, 20, 32, 64, 600, 4800)
    OR lower(chain) IN (
        'polygon',
        'bnb', 'bsc', 'bnb smart chain',
        'hyperevm', 'hyper evm', 'hyperliquid', 'hyperliquid evm',
        'tron'
    )
  );

CREATE INDEX IF NOT EXISTS idx_auth_api_keys_merchant_active ON auth.api_keys (merchant_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_catalog_payment_links_merchant ON catalog.payment_links (merchant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_link ON checkout.checkout_sessions (payment_link_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_client_secret ON checkout.checkout_sessions (id, client_secret_hash);
CREATE INDEX IF NOT EXISTS idx_payment_intents_status ON checkout.payment_intents (status, chain, token_symbol);
CREATE INDEX IF NOT EXISTS idx_payment_intents_expiry ON checkout.payment_intents (expires_at) WHERE status IN ('created', 'awaiting_funds', 'partial');
CREATE INDEX IF NOT EXISTS idx_chain_transactions_intent ON checkout.chain_transactions (payment_intent_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retry ON checkout.webhook_deliveries (status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_sub_cycles_due ON checkout.subscription_cycles (status, due_at);
CREATE INDEX IF NOT EXISTS idx_platform_outbox_pending ON platform.outbox_events (status, available_at, created_at);
