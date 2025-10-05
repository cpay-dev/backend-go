BEGIN;

CREATE TYPE payment.INTENT_STATUS AS ENUM (
  'AWAITING_PAYMENT', 'PAID', 'EXPIRED',
  'AML_CHECK_PENDING', 'AML_CHECK_FAILED',
  'REFUND_PENDING', 'REFUNDED'
);

CREATE TYPE payment.INTENT_WALLET_TYPE AS ENUM (
  'CUSTODIAL', 'NON_CUSTODIAL'
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

CREATE TABLE payment.intent_wallets (
  id public.ulid NOT NULL,
  intent_id public.ulid NOT NULL,
  wallet_id TEXT NOT NULL,
  wallet_type payment.INTENT_WALLET_TYPE NOT NULL,
  asset_address TEXT NOT NULL,
  PRIMARY KEY (id),
  FOREIGN KEY (intent_id) REFERENCES payment.intents (id),
  FOREIGN KEY (wallet_id) REFERENCES wallet.wallets (id)
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

CREATE INDEX intent_wallets_intent_id_idx ON payment.intent_wallets (intent_id);
CREATE INDEX intent_wallets_wallet_id_idx ON payment.intent_wallets (wallet_id);
CREATE INDEX intent_wallets_asset_address_idx ON payment.intent_wallets (asset_address);

CREATE INDEX intent_transfers_intent_id_idx ON payment.intent_transfers (intent_id);
CREATE INDEX intent_transfers_status_idx ON payment.intent_transfers (status);

COMMIT;
