-- name: CreateShop :one
INSERT INTO shops (user_id, name, website, title, avatar_url)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetShopByUserID :one
SELECT * FROM shops WHERE user_id = $1;

-- name: GetShopByID :one
SELECT * FROM shops WHERE id = $1;

-- name: UpdateShop :one
UPDATE shops
SET name = $2, website = $3, title = $4, avatar_url = $5
WHERE id = $1
RETURNING *;
