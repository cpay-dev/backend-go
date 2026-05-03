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
