BEGIN;

DROP TABLE IF EXISTS app.merchant_api_keys;
DROP TABLE IF EXISTS app.merchants;
DROP TABLE IF EXISTS app.user_identities;
DROP TABLE IF EXISTS app.users;

DROP TYPE IF EXISTS app.merchant_status;
DROP TYPE IF EXISTS app.user_identity_type;
DROP TYPE IF EXISTS app.user_status;

COMMIT;
