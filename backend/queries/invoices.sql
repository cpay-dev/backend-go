-- name: CreateInvoice :one
INSERT INTO invoices (
    shop_id, invoice_number, title, memo, amount,
    token_address, chain_id, status, recipient_email,
    due_date, line_items, notes
) VALUES (
    $1,
    (SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices WHERE shop_id = $1),
    $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: GetInvoiceByID :one
SELECT * FROM invoices WHERE id = $1;

-- name: GetPublicInvoice :one
SELECT i.*, u.wallet_address AS merchant_wallet, ca.address AS merchant_address
FROM invoices i
JOIN shops s ON s.id = i.shop_id
JOIN users u ON u.id = s.user_id
LEFT JOIN counterfactual_accounts ca ON ca.user_id = u.id AND ca.chain_id = 80002
WHERE i.id = $1 AND i.status != 'draft';

-- name: ListShopInvoices :many
SELECT * FROM invoices
WHERE shop_id = $1
ORDER BY created_at DESC;

-- name: UpdateInvoice :one
UPDATE invoices
SET title = $2, memo = $3, amount = $4, token_address = $5, chain_id = $6,
    recipient_email = $7, due_date = $8, line_items = $9, notes = $10
WHERE id = $1
RETURNING *;

-- name: UpdateInvoiceStatus :one
UPDATE invoices
SET status = $2
WHERE id = $1
RETURNING *;

-- name: RecordInvoicePayment :one
UPDATE invoices
SET status = 'paid', payer_address = $2, tx_hash = $3, paid_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteInvoice :exec
DELETE FROM invoices WHERE id = $1;
