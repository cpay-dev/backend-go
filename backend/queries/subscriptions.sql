-- name: CreateSubscription :one
INSERT INTO subscriptions (
    shop_id, title, description, amount, token_address, chain_id, period
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions
WHERE id = $1;

-- name: GetPublicSubscription :one
SELECT s.*, u.wallet_address AS merchant_wallet, ca.address AS merchant_address
FROM subscriptions s
JOIN shops sh ON sh.id = s.shop_id
JOIN users u ON u.id = sh.user_id
LEFT JOIN counterfactual_accounts ca ON ca.user_id = u.id AND ca.chain_id = 80002
WHERE s.id = $1 AND s.active = true;

-- name: ListMySubscriptions :many
SELECT s.*
FROM subscriptions s
JOIN shops sh ON sh.id = s.shop_id
WHERE sh.user_id = $1
ORDER BY s.created_at DESC;

-- name: UpdateSubscription :one
UPDATE subscriptions
SET title = $2, description = $3, amount = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ToggleSubscriptionActive :one
UPDATE subscriptions
SET active = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteSubscription :exec
DELETE FROM subscriptions WHERE id = $1;

-- name: CreateSubscriptionPayment :one
INSERT INTO subscription_payments (
    subscription_id, shop_id, payer_address,
    amount, token_address, chain_id, tx_hash,
    subscriber_id, verified, method
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: MarkPaymentVerified :exec
UPDATE subscription_payments SET verified = true WHERE id = $1;

-- name: ListSubscriptionPayments :many
SELECT * FROM subscription_payments
WHERE subscription_id = $1
ORDER BY created_at DESC;

-- name: ListSubscriptionPaymentsByShop :many
SELECT * FROM subscription_payments
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: GetSubscriptionPaymentByPayer :one
SELECT * FROM subscription_payments
WHERE subscription_id = $1 AND payer_address = $2
ORDER BY created_at DESC
LIMIT 1;
