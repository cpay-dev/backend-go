-- name: CreatePayment :one
INSERT INTO payments (
    shop_id, kind, product_id, payment_link_id,
    payer_address, payer_email, token_address, chain_id, amount, tx_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListPaymentsByShop :many
SELECT * FROM payments
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: ListPaymentsByProduct :many
SELECT * FROM payments
WHERE product_id = $1
ORDER BY created_at DESC;

-- name: ListPaymentsByPaymentLink :many
SELECT * FROM payments
WHERE payment_link_id = $1
ORDER BY created_at DESC;

-- name: GetPaymentByTxHash :one
SELECT * FROM payments
WHERE tx_hash = $1;

-- name: GetPaymentByProductAndPayer :one
SELECT * FROM payments
WHERE product_id = $1 AND payer_address = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: GetPaymentByLinkAndPayer :one
SELECT * FROM payments
WHERE payment_link_id = $1 AND payer_address = $2
ORDER BY created_at DESC
LIMIT 1;
