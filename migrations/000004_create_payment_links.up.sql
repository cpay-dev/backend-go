CREATE TABLE payment_links (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id       TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    description   TEXT,
    amount        NUMERIC(36,18) NOT NULL CHECK (amount > 0),
    token_address TEXT NOT NULL DEFAULT '0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359',
    chain_id      INTEGER NOT NULL DEFAULT 137,
    max_uses      INTEGER,
    use_count     INTEGER NOT NULL DEFAULT 0,
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_links_shop_id ON payment_links (shop_id);

CREATE TRIGGER trg_payment_links_updated_at
    BEFORE UPDATE ON payment_links
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
