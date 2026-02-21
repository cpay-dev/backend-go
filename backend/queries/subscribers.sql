-- name: CreateSubscriber :one
INSERT INTO subscribers (
    subscription_id, shop_id, payer_address, payer_email,
    status, method, periods_paid, periods_used,
    approved_amount, spent_amount, next_due
) VALUES (
    $1, $2, $3, $4, 'active', $5, $6, 0, $7, '0', $8
) RETURNING *;

-- name: GetSubscriber :one
SELECT * FROM subscribers
WHERE subscription_id = $1 AND payer_address = $2
LIMIT 1;

-- name: GetSubscriberByID :one
SELECT * FROM subscribers WHERE id = $1;

-- name: GetActiveSubscriber :one
SELECT * FROM subscribers
WHERE subscription_id = $1 AND payer_address = $2 AND status = 'active'
LIMIT 1;

-- name: ListSubscribers :many
SELECT * FROM subscribers
WHERE subscription_id = $1
ORDER BY created_at DESC;

-- name: ListSubscribersByShop :many
SELECT * FROM subscribers
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: ListDueSubscribers :many
SELECT sub.*, s.amount AS sub_amount, s.token_address AS sub_token,
       s.chain_id AS sub_chain_id, s.period AS sub_period, s.title AS sub_title,
       s.shop_id AS sub_shop_id
FROM subscribers sub
JOIN subscriptions s ON s.id = sub.subscription_id
WHERE sub.status = 'active' AND sub.next_due <= $1;

-- name: ListUpcomingDueSubscribers :many
SELECT sub.*, s.amount AS sub_amount, s.token_address AS sub_token,
       s.chain_id AS sub_chain_id, s.period AS sub_period, s.title AS sub_title
FROM subscribers sub
JOIN subscriptions s ON s.id = sub.subscription_id
WHERE sub.status = 'active' AND sub.next_due > $1 AND sub.next_due <= $2;

-- name: UpdateSubscriberStatus :one
UPDATE subscribers
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CancelSubscriber :one
UPDATE subscribers
SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ExpireSubscriber :one
UPDATE subscribers
SET status = 'expired', expires_at = NOW(), updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSubscriberPastDue :one
UPDATE subscribers
SET status = 'past_due', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: IncrementPeriodsUsed :one
UPDATE subscribers
SET periods_used = periods_used + 1, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateSpentAmount :one
UPDATE subscribers
SET spent_amount = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetNextDue :one
UPDATE subscribers
SET next_due = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListPastDueSubscribers :many
SELECT * FROM subscribers
WHERE status = 'past_due' AND updated_at <= $1;

-- name: GetMerchantEmail :one
SELECT u.email FROM users u
JOIN shops sh ON sh.user_id = u.id
WHERE sh.id = $1;

-- name: UpdateUserEmail :one
UPDATE users SET email = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetSubscriptionWithMerchant :one
SELECT s.*, u.wallet_address AS merchant_wallet, u.email AS merchant_email,
       ca.address AS merchant_cf_address
FROM subscriptions s
JOIN shops sh ON sh.id = s.shop_id
JOIN users u ON u.id = sh.user_id
LEFT JOIN counterfactual_accounts ca ON ca.user_id = u.id AND ca.chain_id = s.chain_id
WHERE s.id = $1;
