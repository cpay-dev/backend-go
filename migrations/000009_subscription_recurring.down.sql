ALTER TABLE subscription_payments DROP COLUMN IF EXISTS method;
ALTER TABLE subscription_payments DROP COLUMN IF EXISTS verified;
ALTER TABLE subscription_payments DROP COLUMN IF EXISTS subscriber_id;

DROP TRIGGER IF EXISTS trg_subscribers_updated_at ON subscribers;
DROP TABLE IF EXISTS subscribers;

DROP TYPE IF EXISTS payment_method;
DROP TYPE IF EXISTS subscriber_status;

ALTER TABLE users DROP COLUMN IF EXISTS email;
