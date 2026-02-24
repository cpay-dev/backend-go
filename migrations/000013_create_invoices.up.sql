CREATE TYPE invoice_status AS ENUM ('draft', 'sent', 'paid', 'overdue', 'cancelled');

CREATE TABLE IF NOT EXISTS invoices (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    shop_id          TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    invoice_number   INTEGER NOT NULL,
    title            TEXT NOT NULL,
    memo             TEXT,
    amount           NUMERIC(36,18) NOT NULL CHECK (amount > 0),
    token_address    TEXT NOT NULL DEFAULT '0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582',
    chain_id         INTEGER NOT NULL DEFAULT 80002,
    status           invoice_status NOT NULL DEFAULT 'draft',
    recipient_email  TEXT,
    due_date         TIMESTAMPTZ,
    line_items       JSONB,
    notes            TEXT,
    payer_address    TEXT,
    tx_hash          TEXT,
    paid_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(shop_id, invoice_number)
);

CREATE INDEX IF NOT EXISTS invoices_shop_id_idx ON invoices(shop_id);
CREATE INDEX IF NOT EXISTS invoices_status_idx ON invoices(shop_id, status);

CREATE TRIGGER trg_invoices_updated_at
    BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
