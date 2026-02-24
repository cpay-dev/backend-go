-- name: CreateWebhook :one
INSERT INTO webhooks (shop_id, url, secret, event_types)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetWebhookByID :one
SELECT * FROM webhooks WHERE id = $1;

-- name: ListWebhooksByShop :many
SELECT * FROM webhooks
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: UpdateWebhook :one
UPDATE webhooks
SET url = $2, event_types = $3, active = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = $1;

-- name: ListActiveWebhooksByShopAndEvent :many
SELECT * FROM webhooks
WHERE shop_id = $1
  AND active = TRUE
  AND sqlc.arg(event_type)::text = ANY(event_types);

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (webhook_id, event_id, event_type, payload, status)
VALUES ($1, $2, $3, $4, 'pending')
RETURNING *;

-- name: UpdateWebhookDelivery :exec
UPDATE webhook_deliveries
SET status = $2, attempts = $3, last_status_code = $4, last_error = $5, updated_at = NOW()
WHERE id = $1;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1
ORDER BY created_at DESC
LIMIT 50;

-- name: GetWebhookDeliveryByEventAndWebhook :one
SELECT * FROM webhook_deliveries
WHERE event_id = $1 AND webhook_id = $2
LIMIT 1;
