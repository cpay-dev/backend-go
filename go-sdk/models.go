package cpay

import (
	"encoding/json"
	"time"
)

// ── Auth ────────────────────────────────────────────────────────────────

// VerifyRequest is the body for SIWE verification.
type VerifyRequest struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

// VerifyResponse is returned by Verify and SetRole.
type VerifyResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// ── User ────────────────────────────────────────────────────────────────

// User represents a platform user.
type User struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"wallet_address"`
	Role          *string   `json:"role"`
	Email         *string   `json:"email"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ── Shop ────────────────────────────────────────────────────────────────

// Shop represents a merchant's shop.
type Shop struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Website   *string   `json:"website"`
	Title     *string   `json:"title"`
	AvatarURL *string   `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateShopRequest is the body for creating a shop.
type CreateShopRequest struct {
	Name      string  `json:"name"`
	Website   *string `json:"website,omitempty"`
	Title     *string `json:"title,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// UpdateShopRequest is the body for updating a shop.
type UpdateShopRequest = CreateShopRequest

// ── Product ─────────────────────────────────────────────────────────────

// Product represents a merchant product.
type Product struct {
	ID           string  `json:"id"`
	ShopID       string  `json:"shop_id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	Price        string  `json:"price"`
	Currency     string  `json:"currency"`
	TokenAddress string  `json:"token_address"`
	ChainID      int32   `json:"chain_id"`
	ImageURL     *string `json:"image_url"`
	Active       bool    `json:"active"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// PublicProduct extends Product with the merchant's counterfactual address.
type PublicProduct struct {
	Product
	MerchantAddress *string `json:"merchant_address"`
}

// CreateProductRequest is the body for creating a product.
type CreateProductRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	Price        string  `json:"price"`
	Currency     string  `json:"currency,omitempty"`
	TokenAddress string  `json:"token_address,omitempty"`
	ChainID      int32   `json:"chain_id,omitempty"`
	ImageURL     *string `json:"image_url,omitempty"`
}

// UpdateProductRequest is the body for updating a product.
type UpdateProductRequest = CreateProductRequest

// ── Payment ─────────────────────────────────────────────────────────────

// Payment represents a recorded payment.
type Payment struct {
	ID            string    `json:"id"`
	ShopID        string    `json:"shop_id"`
	Kind          string    `json:"kind"`
	ProductID     *string   `json:"product_id,omitempty"`
	PaymentLinkID *string   `json:"payment_link_id,omitempty"`
	PayerAddress  *string   `json:"payer_address,omitempty"`
	PayerEmail    *string   `json:"payer_email,omitempty"`
	TokenAddress  string    `json:"token_address"`
	ChainID       int64     `json:"chain_id"`
	Amount        string    `json:"amount"`
	TxHash        string    `json:"tx_hash"`
	CreatedAt     time.Time `json:"created_at"`
}

// RecordPaymentRequest is the body for recording a payment (product or link).
type RecordPaymentRequest struct {
	PayerAddress string `json:"payer_address"`
	PayerEmail   string `json:"payer_email,omitempty"`
	TokenAddress string `json:"token_address"`
	ChainID      int64  `json:"chain_id,omitempty"`
	Amount       string `json:"amount"`
	TxHash       string `json:"tx_hash"`
}

// ── Payment Link ────────────────────────────────────────────────────────

// PaymentLink represents a reusable payment link.
type PaymentLink struct {
	ID           string  `json:"id"`
	ShopID       string  `json:"shop_id"`
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	Amount       string  `json:"amount"`
	TokenAddress string  `json:"token_address"`
	ChainID      int32   `json:"chain_id"`
	MaxUses      *int32  `json:"max_uses"`
	UseCount     int32   `json:"use_count"`
	Active       bool    `json:"active"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// PublicPaymentLink extends PaymentLink with the merchant's counterfactual address.
type PublicPaymentLink struct {
	PaymentLink
	MerchantAddress *string `json:"merchant_address"`
}

// CreatePaymentLinkRequest is the body for creating a payment link.
type CreatePaymentLinkRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description,omitempty"`
	Amount       string  `json:"amount"`
	TokenAddress string  `json:"token_address,omitempty"`
	ChainID      int32   `json:"chain_id,omitempty"`
	MaxUses      *int32  `json:"max_uses,omitempty"`
}

// ── Subscription ────────────────────────────────────────────────────────

// Subscription represents a recurring subscription plan.
type Subscription struct {
	ID           string    `json:"id"`
	ShopID       string    `json:"shop_id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	Amount       string    `json:"amount"`
	TokenAddress string    `json:"token_address"`
	ChainID      int64     `json:"chain_id"`
	Period       string    `json:"period"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PublicSubscription extends Subscription with the merchant's counterfactual address.
type PublicSubscription struct {
	Subscription
	MerchantAddress *string `json:"merchant_address"`
}

// CreateSubscriptionRequest is the body for creating a subscription plan.
type CreateSubscriptionRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description,omitempty"`
	Amount       string  `json:"amount"`
	TokenAddress string  `json:"token_address"`
	Period       string  `json:"period"`
	ChainID      int64   `json:"chain_id,omitempty"`
}

// SubscriptionPayment represents a payment towards a subscription.
type SubscriptionPayment struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	ShopID         string    `json:"shop_id"`
	PayerAddress   string    `json:"payer_address"`
	Amount         string    `json:"amount"`
	TokenAddress   string    `json:"token_address"`
	ChainID        int64     `json:"chain_id"`
	TxHash         string    `json:"tx_hash"`
	CreatedAt      time.Time `json:"created_at"`
}

// RecordSubscriptionPaymentRequest is the body for recording a subscription payment.
type RecordSubscriptionPaymentRequest struct {
	PayerAddress string  `json:"payer_address"`
	TokenAddress string  `json:"token_address"`
	ChainID      int64   `json:"chain_id,omitempty"`
	Amount       string  `json:"amount"`
	TxHash       string  `json:"tx_hash"`
	SubscriberID *string `json:"subscriber_id,omitempty"`
	Method       string  `json:"method,omitempty"`
}

// ── Subscriber ──────────────────────────────────────────────────────────

// Subscriber represents a user subscribed to a plan.
type Subscriber struct {
	ID             string  `json:"id"`
	SubscriptionID string  `json:"subscription_id"`
	ShopID         string  `json:"shop_id"`
	PayerAddress   string  `json:"payer_address"`
	PayerEmail     string  `json:"payer_email"`
	Status         string  `json:"status"`
	Method         string  `json:"method"`
	PeriodsPaid    int32   `json:"periods_paid"`
	PeriodsUsed    int32   `json:"periods_used"`
	ApprovedAmount *string `json:"approved_amount,omitempty"`
	SpentAmount    *string `json:"spent_amount,omitempty"`
	NextDue        *string `json:"next_due,omitempty"`
	CancelledAt    *string `json:"cancelled_at,omitempty"`
	ExpiresAt      *string `json:"expires_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// CreateSubscriberRequest is the body for creating a subscriber.
type CreateSubscriberRequest struct {
	PayerAddress   string  `json:"payer_address"`
	Email          string  `json:"email"`
	Method         string  `json:"method"`
	Periods        int32   `json:"periods,omitempty"`
	ApprovedAmount *string `json:"approved_amount,omitempty"`
}

// CancelSubscriberRequest is the body for cancelling a subscriber.
type CancelSubscriberRequest struct {
	PayerAddress string `json:"payer_address"`
}

// ── Invoice ─────────────────────────────────────────────────────────────

// LineItem represents a single line item on an invoice.
type LineItem struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	Amount      string `json:"amount"`
}

// Invoice represents a merchant invoice.
type Invoice struct {
	ID             string     `json:"id"`
	ShopID         string     `json:"shop_id"`
	InvoiceNumber  int        `json:"invoice_number"`
	Title          string     `json:"title"`
	Memo           *string    `json:"memo,omitempty"`
	Amount         string     `json:"amount"`
	TokenAddress   string     `json:"token_address"`
	ChainID        int        `json:"chain_id"`
	Status         string     `json:"status"`
	RecipientEmail *string    `json:"recipient_email,omitempty"`
	DueDate        *string    `json:"due_date,omitempty"`
	LineItems      []LineItem `json:"line_items,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	PayerAddress   *string    `json:"payer_address,omitempty"`
	TxHash         *string    `json:"tx_hash,omitempty"`
	PaidAt         *string    `json:"paid_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

// PublicInvoice extends Invoice with the merchant's counterfactual address.
type PublicInvoice struct {
	Invoice
	MerchantAddress *string `json:"merchant_address"`
}

// CreateInvoiceRequest is the body for creating an invoice.
type CreateInvoiceRequest struct {
	Title          string     `json:"title"`
	Memo           *string    `json:"memo,omitempty"`
	Amount         string     `json:"amount"`
	TokenAddress   string     `json:"token_address,omitempty"`
	ChainID        int        `json:"chain_id,omitempty"`
	RecipientEmail *string    `json:"recipient_email,omitempty"`
	DueDate        *string    `json:"due_date,omitempty"`
	LineItems      []LineItem `json:"line_items,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
}

// UpdateInvoiceRequest is the body for updating an invoice.
type UpdateInvoiceRequest = CreateInvoiceRequest

// RecordInvoicePaymentRequest is the body for recording an invoice payment.
type RecordInvoicePaymentRequest struct {
	PayerAddress string `json:"payer_address"`
	PayerEmail   string `json:"payer_email,omitempty"`
	TokenAddress string `json:"token_address"`
	ChainID      int64  `json:"chain_id,omitempty"`
	Amount       string `json:"amount"`
	TxHash       string `json:"tx_hash"`
}

// ── Webhook ─────────────────────────────────────────────────────────────

// Webhook represents a registered webhook endpoint.
type Webhook struct {
	ID         string   `json:"id"`
	ShopID     string   `json:"shop_id"`
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	Active     bool     `json:"active"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// CreateWebhookRequest is the body for creating a webhook.
type CreateWebhookRequest struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
}

// CreateWebhookResponse includes the webhook and its signing secret (returned only on creation).
type CreateWebhookResponse struct {
	Webhook Webhook `json:"webhook"`
	Secret  string  `json:"secret"`
}

// UpdateWebhookRequest is the body for updating a webhook.
type UpdateWebhookRequest struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	Active     bool     `json:"active"`
}

// WebhookDelivery represents a webhook delivery attempt.
type WebhookDelivery struct {
	ID             string          `json:"id"`
	WebhookID      string          `json:"webhook_id"`
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int32           `json:"attempts"`
	LastStatusCode *int32          `json:"last_status_code"`
	LastError      *string         `json:"last_error"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ── Upload ──────────────────────────────────────────────────────────────

type uploadResponse struct {
	URL string `json:"url"`
}

// ── CF Address / Balance ────────────────────────────────────────────────

// CFBalanceResponse contains the CF account address and token balance.
type CFBalanceResponse struct {
	Address string `json:"address"`
	Balance string `json:"balance"`
}

// CFWithdrawInfoResponse contains CF wallet info for withdrawals.
type CFWithdrawInfoResponse struct {
	CFAddress  string `json:"cf_address"`
	Balance    string `json:"balance"`
	IsDeployed bool   `json:"is_deployed"`
	Token      string `json:"token"`
	ChainID    int64  `json:"chain_id"`
}

// ── Withdrawal ──────────────────────────────────────────────────────────

// RecordWithdrawalRequest is the body for recording a withdrawal.
type RecordWithdrawalRequest struct {
	TokenAddress string `json:"token_address"`
	ChainID      int64  `json:"chain_id"`
	Amount       string `json:"amount"`
	TxHash       string `json:"tx_hash"`
}

// WithdrawalResponse is returned after recording a withdrawal.
type WithdrawalResponse struct {
	ID     string `json:"id"`
	TxHash string `json:"tx_hash"`
}
