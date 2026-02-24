-- name: CreateWithdrawal :one
INSERT INTO withdrawals (shop_id, token_address, chain_id, amount, tx_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListWithdrawalsByShop :many
SELECT * FROM withdrawals
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: GetWithdrawalByTxHash :one
SELECT * FROM withdrawals
WHERE tx_hash = $1;
