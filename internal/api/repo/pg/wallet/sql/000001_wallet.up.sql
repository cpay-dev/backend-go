BEGIN;

CREATE TYPE wallet.WALLET_STATUS AS ENUM ('AVAILABLE', 'IN_USE' 'RECENTLY_USED');

CREATE TABLE wallet.wallets (
  id public.ulid NOT NULL,
  chain_id TEXT NOT NULL,
  status wallet.WALLET_STATUS NOT NULL,
  kek_version INT NOT NULL,
  public_key TEXT NOT NULL,
  encrypted_private_key bytea NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (id),
  FOREIGN KEY (chain_id) REFERENCES blockchain.chains (id)
);

CREATE INDEX wallets_chain_id_idx ON wallet.wallets (chain_id);
CREATE INDEX wallets_status_idx ON wallet.wallets (status);

COMMIT;
