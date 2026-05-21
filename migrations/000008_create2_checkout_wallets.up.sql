ALTER TABLE checkout.deposit_addresses
    ADD COLUMN IF NOT EXISTS wallet_type TEXT NOT NULL DEFAULT 'eoa',
    ADD COLUMN IF NOT EXISTS factory_address TEXT,
    ADD COLUMN IF NOT EXISTS wallet_salt TEXT,
    ADD COLUMN IF NOT EXISTS init_code_hash TEXT;

ALTER TABLE checkout.deposit_addresses
    ALTER COLUMN encrypted_private_key DROP NOT NULL;

UPDATE checkout.deposit_addresses
SET wallet_type = 'eoa'
WHERE wallet_type IS NULL OR wallet_type = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'deposit_addresses_wallet_type_check'
          AND conrelid = 'checkout.deposit_addresses'::regclass
    ) THEN
        ALTER TABLE checkout.deposit_addresses
            ADD CONSTRAINT deposit_addresses_wallet_type_check
            CHECK (wallet_type IN ('eoa', 'create2'));
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'deposit_addresses_wallet_material_check'
          AND conrelid = 'checkout.deposit_addresses'::regclass
    ) THEN
        ALTER TABLE checkout.deposit_addresses
            ADD CONSTRAINT deposit_addresses_wallet_material_check
            CHECK (
                (wallet_type = 'eoa' AND encrypted_private_key IS NOT NULL)
                OR (wallet_type = 'create2' AND encrypted_private_key IS NULL AND factory_address IS NOT NULL AND wallet_salt IS NOT NULL)
            );
    END IF;
END $$;
