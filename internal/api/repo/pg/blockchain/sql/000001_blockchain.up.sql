BEGIN;

CREATE TABLE blockchain.chains (
  id TEXT NOT NULL,
  name TEXT NOT NULL UNIQUE,
  PRIMARY KEY (id)
);

CREATE TABLE blockchain.assets (
  id TEXT NOT NULL,
  chain_id TEXT NOT NULL,
  name TEXT NOT NULL,
  symbol TEXT NOT NULL,
  metadata JSONB,
  PRIMARY KEY (id),
  FOREIGN KEY (chain_id) REFERENCES blockchain.chains (id)
);

CREATE INDEX assets_chain_id_idx ON blockchain.assets (chain_id);

COMMIT;
