CREATE TABLE IF NOT EXISTS withdrawals (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id        TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    token_address  TEXT NOT NULL,
    chain_id       BIGINT NOT NULL,
    amount         DECIMAL(36,18) NOT NULL,
    tx_hash        TEXT NOT NULL UNIQUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS withdrawals_shop_id_idx ON withdrawals(shop_id);
