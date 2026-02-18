-- name: CreatePaymentLink :one
INSERT INTO payment_links (shop_id, title, description, amount, token_address, chain_id, max_uses)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetPaymentLinkByID :one
SELECT * FROM payment_links WHERE id = $1;

-- name: GetPublicPaymentLink :one
SELECT pl.*, ca.address AS merchant_address
FROM payment_links pl
JOIN shops s ON s.id = pl.shop_id
JOIN users u ON u.id = s.user_id
LEFT JOIN counterfactual_accounts ca ON ca.user_id = u.id AND ca.chain_id = 80002
WHERE pl.id = $1;

-- name: ListShopPaymentLinks :many
SELECT * FROM payment_links WHERE shop_id = $1 ORDER BY created_at DESC;

-- name: IncrementPaymentLinkUseCount :one
UPDATE payment_links
SET use_count = use_count + 1
WHERE id = $1
RETURNING *;

-- name: SetPaymentLinkActive :exec
UPDATE payment_links SET active = $2 WHERE id = $1;

-- name: DeletePaymentLink :exec
DELETE FROM payment_links WHERE id = $1;
