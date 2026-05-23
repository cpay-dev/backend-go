ALTER TABLE auth.users
    ALTER COLUMN password_hash DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_users_email_global
    ON auth.users (lower(email));

CREATE TABLE IF NOT EXISTS auth.identities (
    id ulid PRIMARY KEY,
    user_id ulid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    email TEXT,
    display_name TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_subject),
    CHECK (provider IN ('google', 'wallet'))
);

CREATE INDEX IF NOT EXISTS idx_auth_identities_user_provider
    ON auth.identities (user_id, provider);

CREATE TABLE IF NOT EXISTS auth.passkey_credentials (
    id ulid PRIMARY KEY,
    user_id ulid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    merchant_id ulid NOT NULL REFERENCES auth.merchants(id) ON DELETE CASCADE,
    credential_id TEXT NOT NULL,
    credential JSONB NOT NULL,
    name TEXT,
    transports JSONB NOT NULL DEFAULT '[]'::jsonb,
    sign_count BIGINT NOT NULL DEFAULT 0,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (credential_id)
);

CREATE INDEX IF NOT EXISTS idx_auth_passkey_credentials_user
    ON auth.passkey_credentials (user_id);

CREATE TABLE IF NOT EXISTS auth.auth_challenges (
    id ulid PRIMARY KEY,
    kind TEXT NOT NULL,
    user_id ulid REFERENCES auth.users(id) ON DELETE CASCADE,
    merchant_id ulid REFERENCES auth.merchants(id) ON DELETE CASCADE,
    provider TEXT,
    provider_subject TEXT,
    challenge_hash TEXT,
    session_data JSONB,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (kind IN ('passkey_register', 'passkey_login', 'wallet_login', 'google_oauth', 'onboarding'))
);

CREATE INDEX IF NOT EXISTS idx_auth_challenges_active
    ON auth.auth_challenges (kind, expires_at)
    WHERE consumed_at IS NULL;
