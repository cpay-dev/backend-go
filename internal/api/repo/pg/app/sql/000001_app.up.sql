BEGIN;

CREATE TYPE app.user_status AS ENUM ('ACTIVE', 'INACTIVE', 'BANNED');
CREATE TYPE app.user_identity_type AS ENUM ('EMAIL', 'WALLET');

CREATE TYPE app.merchant_status AS ENUM ('ACTIVE', 'INACTIVE', 'BANNED');

CREATE TABLE app.users (
  id public.ulid NOT NULL,
  status app.user_status NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  PRIMARY KEY (id)
);

CREATE TABLE app.user_identities (
  id public.ulid NOT NULL,
  user_id public.ulid NOT NULL,
  identity_type app.user_identity_type NOT NULL,
  identity TEXT NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  PRIMARY KEY (id),
  FOREIGN KEY (user_id) REFERENCES app.users (id),
  UNIQUE (identity_type, identity)
);

CREATE TABLE app.merchants (
  id public.ulid NOT NULL,
  user_id public.ulid NOT NULL,
  name TEXT NOT NULL,
  status app.merchant_status NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  PRIMARY KEY (id),
  FOREIGN KEY (user_id) REFERENCES app.users (id)
);

CREATE TABLE app.merchant_api_keys (
  id public.ulid NOT NULL,
  user_id public.ulid NOT NULL,
  merchant_id public.ulid NOT NULL,
  name TEXT NOT NULL,
  key TEXT NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  PRIMARY KEY (id),
  FOREIGN KEY (user_id) REFERENCES app.users (id),
  FOREIGN KEY (merchant_id) REFERENCES app.merchants (id),
  UNIQUE (merchant_id, name)
);

COMMIT;
