-- name: GetUserByWallet :one
SELECT * FROM users WHERE wallet_address = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (wallet_address)
VALUES ($1)
RETURNING *;

-- name: UpdateUserRole :one
UPDATE users SET role = $1 WHERE id = $2
RETURNING *;
