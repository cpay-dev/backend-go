CREATE TYPE subscription_status AS ENUM ('active', 'paused', 'cancelled');
CREATE TYPE billing_period AS ENUM ('daily', 'weekly', 'monthly', 'yearly');

CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id         TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    description     TEXT,
    amount          DECIMAL(36,18) NOT NULL,
    token_address   TEXT NOT NULL,
    chain_id        BIGINT NOT NULL DEFAULT 137,
    period          billing_period NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subscription_payments (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    subscription_id TEXT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    shop_id         TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    payer_address   TEXT NOT NULL,
    amount          DECIMAL(36,18) NOT NULL,
    token_address   TEXT NOT NULL,
    chain_id        BIGINT NOT NULL,
    tx_hash         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS subscriptions_shop_id_idx ON subscriptions(shop_id);
CREATE INDEX IF NOT EXISTS subscription_payments_subscription_id_idx ON subscription_payments(subscription_id);
CREATE INDEX IF NOT EXISTS subscription_payments_shop_id_idx ON subscription_payments(shop_id);
