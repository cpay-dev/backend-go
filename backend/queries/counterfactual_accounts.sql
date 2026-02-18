-- name: UpsertCounterfactualAccount :one
INSERT INTO counterfactual_accounts (user_id, chain_id, address)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, chain_id) DO UPDATE SET address = EXCLUDED.address
RETURNING *;

-- name: GetCounterfactualAccount :one
SELECT * FROM counterfactual_accounts
WHERE user_id = $1 AND chain_id = $2;

-- name: GetCounterfactualAccountByAddress :one
SELECT * FROM counterfactual_accounts
WHERE address = $1;
