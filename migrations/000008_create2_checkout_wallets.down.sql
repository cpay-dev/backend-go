ALTER TABLE checkout.deposit_addresses
    DROP CONSTRAINT IF EXISTS deposit_addresses_wallet_material_check,
    DROP CONSTRAINT IF EXISTS deposit_addresses_wallet_type_check;

ALTER TABLE checkout.deposit_addresses
    DROP COLUMN IF EXISTS init_code_hash,
    DROP COLUMN IF EXISTS wallet_salt,
    DROP COLUMN IF EXISTS factory_address,
    DROP COLUMN IF EXISTS wallet_type;

ALTER TABLE checkout.deposit_addresses
    ALTER COLUMN encrypted_private_key SET NOT NULL;
