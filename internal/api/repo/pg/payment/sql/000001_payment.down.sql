BEGIN;

DROP TABLE IF EXISTS payment.intent_transfers;
DROP TABLE IF EXISTS payment.intents;

DROP TYPE IF EXISTS payment.intent_transfer_status;
DROP TYPE IF EXISTS payment.intent_status;

COMMIT;
