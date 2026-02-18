CREATE TYPE payment_kind AS ENUM ('product', 'payment_link', 'subscription');

CREATE TABLE IF NOT EXISTS payments (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id         TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    kind            payment_kind NOT NULL,
    -- nullable references depending on kind
    product_id      TEXT REFERENCES products(id) ON DELETE SET NULL,
    payment_link_id TEXT REFERENCES payment_links(id) ON DELETE SET NULL,
    -- payer (may be anonymous for payment links)
    payer_address   TEXT,
    -- token details snapshot
    token_address   TEXT NOT NULL,
    chain_id        BIGINT NOT NULL,
    amount          DECIMAL(36,18) NOT NULL,
    -- on-chain tx hash
    tx_hash         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS payments_shop_id_idx ON payments(shop_id);
CREATE INDEX IF NOT EXISTS payments_product_id_idx ON payments(product_id);
CREATE INDEX IF NOT EXISTS payments_payment_link_id_idx ON payments(payment_link_id);
