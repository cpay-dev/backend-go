CREATE TABLE products (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id       TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT,
    price         NUMERIC(36,18) NOT NULL CHECK (price > 0),
    currency      TEXT NOT NULL DEFAULT 'USD',
    token_address TEXT NOT NULL DEFAULT '0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359',
    chain_id      INTEGER NOT NULL DEFAULT 137,
    image_url     TEXT,
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_shop_id ON products (shop_id);
CREATE INDEX idx_products_shop_active ON products (shop_id, active);

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
