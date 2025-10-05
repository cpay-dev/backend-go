BEGIN;

CREATE TYPE payment.INTENT_STATUS AS ENUM (
  'AWAITING_PAYMENT', 'PAID', 'EXPIRED',
  'AML_CHECK_PENDING', 'AML_CHECK_FAILED',
  'REFUND_PENDING', 'REFUNDED'
);

CREATE TYPE payment.METHOD AS ENUM (
  'WALLET_CUSTODIAL', 'WALLET_NON_CUSTODIAL'
);

CREATE TYPE payment.INTENT_TRANSFER_STATUS AS ENUM (
  'PENDING', 'DROPPED', 'COMPLETE'
);

CREATE TABLE payment.intents (
  id public.ulid NOT NULL,
  merchant_id public.ulid NOT NULL,
  asset_id public.ulid NOT NULL,
  status payment.INTENT_STATUS NOT NULL,
  amount_usd BIGINT NOT NULL,
  amount_asset TEXT NOT NULL,
  amount_paid_asset TEXT NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (id),
  FOREIGN KEY (merchant_id) REFERENCES app.merchants (id),
  FOREIGN KEY (asset_id) REFERENCES blockchain.assets (id)
);

CREATE TABLE payment.intent_methods (
  id public.ulid NOT NULL,
  intent_id public.ulid NOT NULL,
  method_type payment.METHOD NOT NULL,
  method_id TEXT NOT NULL,
  method_data JSONB NOT NULL,
  PRIMARY KEY (id),
  FOREIGN KEY (intent_id) REFERENCES payment.intents (id),
  UNIQUE (intent_id, method_type, method_id)
);

CREATE TABLE payment.intent_transfers (
  id public.ulid NOT NULL,
  intent_id public.ulid NOT NULL,
  status payment.INTENT_TRANSFER_STATUS NOT NULL,
  amount_asset TEXT NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (id),
  FOREIGN KEY (intent_id) REFERENCES payment.intents (id)
);

CREATE INDEX intents_merchant_id_idx ON payment.intents (merchant_id);
CREATE INDEX intents_status_idx ON payment.intents (status);

CREATE INDEX intent_methods_intent_id_idx ON payment.intent_methods (intent_id);

CREATE INDEX intent_transfers_intent_id_idx ON payment.intent_transfers (intent_id);
CREATE INDEX intent_transfers_status_idx ON payment.intent_transfers (status);

COMMIT;
