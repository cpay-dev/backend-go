ALTER TABLE checkout.payout_items
    DROP CONSTRAINT IF EXISTS payout_items_status_check,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS tx_hash,
    DROP COLUMN IF EXISTS status;

ALTER TABLE checkout.payouts
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS attempt_count;
