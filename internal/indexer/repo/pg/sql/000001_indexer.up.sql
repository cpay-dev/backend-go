BEGIN;

CREATE TYPE indexer.CHAIN AS ENUM ('UNICHAIN');
CREATE TYPE indexer.TRANSFER_KIND AS ENUM ('NATIVE', 'CONTRACT');

CREATE TABLE indexer.transfers (
  id public.ulid NOT NULL,
  chain indexer.CHAIN NOT NULL,
  block_hash TEXT NOT NULL,
  block_number BIGINT NOT NULL,
  tx_hash TEXT NOT NULL,
  index SMALLINT NOT NULL,
  amount TEXT NOT NULL,
  from_address TEXT NOT NULL,
  to_address TEXT NOT NULL,
  contract_address TEXT,
  kind indexer.TRANSFER_KIND NOT NULL,
  block_timestamp TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (chain, block_hash, tx_hash, index, kind)
) PARTITION BY LIST (chain);

CREATE INDEX transfers_block_hash_idx ON indexer.transfers (chain, block_hash);
CREATE INDEX transfers_block_timestamp_idx ON indexer.transfers (block_timestamp);

CREATE TABLE indexer.transfers_unichain
  PARTITION OF indexer.transfers
  FOR VALUES IN ('UNICHAIN');

COMMIT;
