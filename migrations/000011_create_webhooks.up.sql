CREATE TYPE webhook_delivery_status AS ENUM ('pending', 'success', 'failed');

CREATE TABLE IF NOT EXISTS webhooks (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id     TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL,
    event_types TEXT[] NOT NULL DEFAULT '{}',
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS webhooks_shop_id_idx ON webhooks(shop_id);
CREATE INDEX IF NOT EXISTS webhooks_active_idx ON webhooks(shop_id, active) WHERE active = TRUE;

CREATE TRIGGER trg_webhooks_updated_at
    BEFORE UPDATE ON webhooks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    webhook_id       TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id         TEXT NOT NULL,
    event_type       TEXT NOT NULL,
    payload          JSONB NOT NULL,
    status           webhook_delivery_status NOT NULL DEFAULT 'pending',
    attempts         INT NOT NULL DEFAULT 0,
    last_status_code INT,
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_id_idx ON webhook_deliveries(webhook_id);
CREATE INDEX IF NOT EXISTS webhook_deliveries_created_at_idx ON webhook_deliveries(created_at);

CREATE TRIGGER trg_webhook_deliveries_updated_at
    BEFORE UPDATE ON webhook_deliveries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
