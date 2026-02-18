-- name: CreateProduct :one
INSERT INTO products (shop_id, name, description, price, currency, token_address, chain_id, image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetProductByID :one
SELECT * FROM products WHERE id = $1;

-- name: GetPublicProduct :one
SELECT p.*, ca.address AS merchant_address
FROM products p
JOIN shops s ON s.id = p.shop_id
JOIN users u ON u.id = s.user_id
LEFT JOIN counterfactual_accounts ca ON ca.user_id = u.id
WHERE p.id = $1 AND p.active = true;

-- name: ListShopProducts :many
SELECT * FROM products WHERE shop_id = $1 ORDER BY created_at DESC;

-- name: ListShopActiveProducts :many
SELECT * FROM products WHERE shop_id = $1 AND active = true ORDER BY created_at DESC;

-- name: UpdateProduct :one
UPDATE products
SET name = $2, description = $3, price = $4, currency = $5, token_address = $6, chain_id = $7, image_url = $8
WHERE id = $1
RETURNING *;

-- name: SetProductActive :exec
UPDATE products SET active = $2 WHERE id = $1;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = $1;
