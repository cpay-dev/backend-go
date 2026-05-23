DROP TABLE IF EXISTS auth.auth_challenges;
DROP TABLE IF EXISTS auth.passkey_credentials;
DROP TABLE IF EXISTS auth.identities;
DROP INDEX IF EXISTS auth.idx_auth_users_email_global;
ALTER TABLE auth.users
    ALTER COLUMN password_hash SET NOT NULL;
