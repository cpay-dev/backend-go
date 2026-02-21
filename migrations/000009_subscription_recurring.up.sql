-- Email on users (required for merchants at onboarding)
ALTER TABLE users ADD COLUMN email TEXT;

-- Subscriber status enum
CREATE TYPE subscriber_status AS ENUM ('active', 'past_due', 'cancelled', 'expired');
-- Payment method enum
CREATE TYPE payment_method AS ENUM ('prepaid', 'approved', 'manual');

-- Active subscription instances (one per payer x plan)
CREATE TABLE IF NOT EXISTS subscribers (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    subscription_id  TEXT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    shop_id          TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    payer_address    TEXT NOT NULL,
    payer_email      TEXT NOT NULL,
    status           subscriber_status NOT NULL DEFAULT 'active',
    method           payment_method NOT NULL,
    -- prepaid tracking
    periods_paid     INT NOT NULL DEFAULT 0,
    periods_used     INT NOT NULL DEFAULT 0,
    -- approved tracking (raw token units as text)
    approved_amount  TEXT,
    spent_amount     TEXT DEFAULT '0',
    -- schedule
    next_due         TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(subscription_id, payer_address)
);

CREATE INDEX IF NOT EXISTS subscribers_status_next_due_idx ON subscribers(status, next_due);
CREATE INDEX IF NOT EXISTS subscribers_subscription_id_idx ON subscribers(subscription_id);

CREATE TRIGGER trg_subscribers_updated_at
    BEFORE UPDATE ON subscribers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Extend subscription_payments with verification + subscriber link
ALTER TABLE subscription_payments ADD COLUMN subscriber_id TEXT REFERENCES subscribers(id);
ALTER TABLE subscription_payments ADD COLUMN verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE subscription_payments ADD COLUMN method payment_method NOT NULL DEFAULT 'manual';
