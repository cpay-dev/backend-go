package checkoutsvc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/payment"
	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/outbox"
	"github.com/cpay-dev/cpay/internal/platform/storage"
	"github.com/cpay-dev/cpay/internal/shared/auth"
	"github.com/cpay-dev/cpay/internal/shared/chain"
	"github.com/cpay-dev/cpay/internal/shared/config"
	cryptox "github.com/cpay-dev/cpay/internal/shared/crypto"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jung-kurt/gofpdf/v2"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
)

type Service struct {
	cpayv1.UnimplementedCheckoutServiceServer

	cfg        config.Config
	log        zerolog.Logger
	db         *pgxpool.Pool
	chain      chain.ChainAdapter
	encryptKey []byte
	minio      *storage.MinIO
	outbox     *outbox.Publisher
}

func New(cfg config.Config, log zerolog.Logger, db *pgxpool.Pool, chainAdapter chain.ChainAdapter, encryptKey []byte, minio *storage.MinIO) *Service {
	return &Service{
		cfg:        cfg,
		log:        log,
		db:         db,
		chain:      chainAdapter,
		encryptKey: encryptKey,
		minio:      minio,
		outbox:     outbox.New(db, cfg.ServiceName),
	}
}

type createSessionInput struct {
	MerchantID      *string
	LinkIdentifier  string
	Amount          *float64
	Chain           string
	TokenSymbol     string
	TokenAddress    string
	CustomerEmail   string
	CustomerName    string
	CustomerPhone   string
	CustomerAddress string
	SuccessURL      string
	ExpiresInSec    int
	MetadataJSON    string
	IncludeSecret   bool
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type allowedToken struct {
	Chain    string `json:"chain"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

func (s *Service) CreateCheckoutSession(ctx context.Context, req *cpayv1.CreateCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	in := createSessionInput{
		MerchantID:      &merchantID,
		LinkIdentifier:  req.GetLinkIdentifier(),
		Amount:          req.Amount,
		Chain:           req.GetChain(),
		TokenSymbol:     req.GetTokenSymbol(),
		TokenAddress:    req.GetTokenAddress(),
		CustomerEmail:   req.GetCustomerEmail(),
		CustomerName:    req.GetCustomerName(),
		CustomerPhone:   req.GetCustomerPhone(),
		CustomerAddress: req.GetCustomerAddressJson(),
		SuccessURL:      req.GetSuccessUrl(),
		ExpiresInSec:    int(req.GetExpiresInSec()),
		MetadataJSON:    req.GetMetadataJson(),
	}
	return s.createSession(ctx, in)
}

func (s *Service) CreatePublicCheckoutSession(ctx context.Context, req *cpayv1.CreatePublicCheckoutSessionRequest) (*cpayv1.CreateCheckoutSessionResponse, error) {
	in := createSessionInput{
		LinkIdentifier:  req.GetLinkIdentifier(),
		Amount:          req.Amount,
		Chain:           req.GetChain(),
		TokenSymbol:     req.GetTokenSymbol(),
		TokenAddress:    req.GetTokenAddress(),
		CustomerEmail:   req.GetCustomerEmail(),
		CustomerName:    req.GetCustomerName(),
		CustomerPhone:   req.GetCustomerPhone(),
		CustomerAddress: req.GetCustomerAddressJson(),
		SuccessURL:      req.GetSuccessUrl(),
		ExpiresInSec:    int(req.GetExpiresInSec()),
		MetadataJSON:    req.GetMetadataJson(),
		IncludeSecret:   true,
	}
	return s.createSession(ctx, in)
}

func (s *Service) createSession(ctx context.Context, in createSessionInput) (*cpayv1.CreateCheckoutSessionResponse, error) {
	linkIdentifier := strings.TrimSpace(in.LinkIdentifier)
	if linkIdentifier == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "link id is required")
	}
	if strings.TrimSpace(in.Chain) == "" || strings.TrimSpace(in.TokenSymbol) == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "chain and token_symbol are required")
	}

	query := `
		SELECT p.id::text, p.merchant_id::text, p.code, p.title, p.pricing_mode, COALESCE(p.amount::text, ''), COALESCE(p.min_amount::text, ''),
			COALESCE(p.max_amount::text, ''), COALESCE(p.adjust_percent::text, '0'), p.currency, p.reusable, p.max_payments,
			p.expires_at, p.status, p.allowed_tokens, p.after_payment_type, p.redirect_url,
			COALESCE(l.add_invoice_pdf, false)
		FROM catalog.payment_links p
		LEFT JOIN catalog.link_options l ON l.payment_link_id = p.id
		WHERE `
	args := []any{}
	if in.MerchantID != nil {
		query += `p.merchant_id=$1 AND (p.id::text=$2 OR p.code=$2)`
		args = append(args, *in.MerchantID, linkIdentifier)
	} else {
		query += `(p.id::text=$1 OR p.code=$1)`
		args = append(args, linkIdentifier)
	}

	var linkIDStr, merchantIDStr, code, title, pricingMode, amountRaw, minRaw, maxRaw, adjustRaw, currency, afterType string
	var reusable bool
	var maxPayments *int
	var linkExpiresAt *time.Time
	var linkStatus string
	var allowedRaw []byte
	var redirectURL *string
	var addInvoice bool
	if err := s.db.QueryRow(ctx, query, args...).Scan(
		&linkIDStr, &merchantIDStr, &code, &title, &pricingMode, &amountRaw, &minRaw,
		&maxRaw, &adjustRaw, &currency, &reusable, &maxPayments,
		&linkExpiresAt, &linkStatus, &allowedRaw, &afterType, &redirectURL, &addInvoice,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "payment link not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to load payment link")
	}
	if linkStatus != "active" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "payment link is not active")
	}
	if linkExpiresAt != nil && linkExpiresAt.Before(time.Now().UTC()) {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "payment link is expired")
	}

	if !reusable {
		var usedCount int
		if err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM checkout.payment_intents
			WHERE payment_link_id=$1 AND status IN ('confirmed', 'overpaid', 'settled')
		`, linkIDStr).Scan(&usedCount); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to validate reusable option")
		}
		if usedCount > 0 {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "payment link has already been paid")
		}
	}

	if maxPayments != nil {
		var paidCount int
		if err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM checkout.payment_intents
			WHERE payment_link_id=$1 AND status IN ('confirmed', 'overpaid', 'settled')
		`, linkIDStr).Scan(&paidCount); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to validate max payments")
		}
		if paidCount >= *maxPayments {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "payment link has reached max payments")
		}
	}

	amount := 0.0
	if strings.ToLower(pricingMode) == "fixed" {
		amountVal := parseFloatValue(amountRaw)
		if amountVal <= 0 {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "invalid fixed amount")
		}
		amount = amountVal
	} else {
		if in.Amount == nil || *in.Amount <= 0 {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "amount is required for open links")
		}
		amount = *in.Amount
		if minVal := parseFloatValue(minRaw); minVal > 0 && amount < minVal {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "amount is below min_amount")
		}
		if maxVal := parseFloatValue(maxRaw); maxVal > 0 && amount > maxVal {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "amount is above max_amount")
		}
	}
	if adjustVal := parseFloatValue(adjustRaw); adjustVal != 0 {
		amount = amount + (amount * adjustVal / 100)
	}
	tokenAddress := strings.TrimSpace(in.TokenAddress)
	if tokenAddress == "" {
		if contract, ok := chain.KnownEVMTokenContract(in.Chain, in.TokenSymbol); ok {
			tokenAddress = contract.Address
		}
	}

	allowedTokens := []allowedToken{}
	if err := json.Unmarshal(allowedRaw, &allowedTokens); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to decode allowed tokens")
	}
	if !tokenAllowed(allowedTokens, in.Chain, in.TokenSymbol, tokenAddress) {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "token is not allowed for this link")
	}

	expiresIn := 30 * time.Minute
	if in.ExpiresInSec > 0 {
		if in.ExpiresInSec < 60 {
			in.ExpiresInSec = 60
		}
		if in.ExpiresInSec > 86400 {
			in.ExpiresInSec = 86400
		}
		expiresIn = time.Duration(in.ExpiresInSec) * time.Second
	}
	sessionExpiresAt := time.Now().UTC().Add(expiresIn)
	if linkExpiresAt != nil && linkExpiresAt.Before(sessionExpiresAt) {
		sessionExpiresAt = *linkExpiresAt
	}

	requiredConf := s.cfg.ConfirmationForChain(in.Chain)
	minAccept, maxAccept := payment.ComputeBounds(amount, s.cfg.DefaultTolerancePercent)
	wallet, err := s.chain.GenerateDepositWallet(ctx, in.Chain)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to generate deposit wallet")
	}
	encryptedPK, err := cryptox.EncryptString(s.encryptKey, wallet.PrivateKeyHex)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to encrypt private key")
	}

	merchantID, _ := ids.Parse(merchantIDStr)
	sessionID := ids.New()
	intentID := ids.New()
	addressID := ids.New()
	clientSecret, err := newClientSecret()
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to generate client secret")
	}
	clientSecretHash := auth.HashToken(clientSecret)

	customerAddressJSON, err := normalizeJSON(in.CustomerAddress, "{}")
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "customer_address_json is invalid")
	}
	if _, err := normalizeJSON(in.MetadataJSON, "{}"); err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "metadata_json is invalid")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.checkout_sessions(
			id, payment_link_id, merchant_id, status, customer_email, customer_name, customer_phone, customer_address,
			amount, currency, chain, token_symbol, token_address, expires_at, success_url, client_secret_hash, created_at, updated_at
		)
		VALUES($1, $2, $3, 'awaiting_funds', $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
	`, sessionID, linkIDStr, merchantID, nullIfEmpty(in.CustomerEmail), nullIfEmpty(in.CustomerName), nullIfEmpty(in.CustomerPhone), customerAddressJSON,
		amount, currency, strings.ToLower(in.Chain), strings.ToUpper(in.TokenSymbol), nullIfEmpty(tokenAddress), sessionExpiresAt, nullIfEmpty(in.SuccessURL), clientSecretHash)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create checkout session")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.payment_intents(
			id, checkout_session_id, payment_link_id, merchant_id, status, chain, token_symbol, token_address,
			expected_amount, tolerance_percent, min_acceptable_amount, max_acceptable_amount, received_amount,
			confirmations, required_confirmations, expires_at, created_at, updated_at
		)
		VALUES(
			$1, $2, $3, $4, 'awaiting_funds', $5, $6, $7,
			$8, $9, $10, $11, 0,
			0, $12, $13, NOW(), NOW()
	)
	`, intentID, sessionID, linkIDStr, merchantID, strings.ToLower(in.Chain), strings.ToUpper(in.TokenSymbol), nullIfEmpty(tokenAddress),
		amount, s.cfg.DefaultTolerancePercent, minAccept, maxAccept, requiredConf, sessionExpiresAt)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create payment intent")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.deposit_addresses(
			id, payment_intent_id, merchant_id, chain, address, encrypted_private_key, status, created_at
		)
		VALUES($1, $2, $3, $4, $5, $6, 'active', NOW())
	`, addressID, intentID, merchantID, strings.ToLower(in.Chain), wallet.Address, encryptedPK)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create deposit address")
	}

	if err = s.outbox.EnqueueTx(ctx, tx, "payment_intent", intentID, &merchantID, "payment_intent.created", map[string]any{
		"payment_intent_id": intentID,
		"checkout_session":  sessionID,
		"payment_link_id":   linkIDStr,
		"chain":             strings.ToLower(in.Chain),
		"token_symbol":      strings.ToUpper(in.TokenSymbol),
		"expected_amount":   amount,
		"add_invoice_pdf":   addInvoice,
	}); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to enqueue outbox event")
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to commit transaction")
	}

	resp := &cpayv1.CreateCheckoutSessionResponse{
		Id:                      sessionID,
		PaymentIntentId:         intentID,
		PaymentLinkCode:         code,
		Status:                  "awaiting_funds",
		Amount:                  amount,
		Currency:                currency,
		Chain:                   strings.ToLower(in.Chain),
		TokenSymbol:             strings.ToUpper(in.TokenSymbol),
		DepositAddress:          wallet.Address,
		TolerancePercent:        s.cfg.DefaultTolerancePercent,
		MinAcceptableAmount:     minAccept,
		MaxAcceptableAmount:     maxAccept,
		ExpiresAt:               sessionExpiresAt.UTC().Format(time.RFC3339Nano),
		AfterPaymentType:        afterType,
		AfterPaymentRedirectUrl: strValue(redirectURL),
	}
	if in.IncludeSecret {
		resp.ClientSecret = clientSecret
	}
	return resp, nil
}

func (s *Service) GetCheckoutSession(ctx context.Context, req *cpayv1.GetCheckoutSessionRequest) (*cpayv1.CheckoutSession, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	sessionID, err := ids.Parse(strings.TrimSpace(req.GetSessionId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "session_id is invalid")
	}

	query := `
		SELECT c.id::text, c.payment_link_id::text, c.status, c.amount::text, c.currency, c.chain, c.token_symbol, c.token_address,
			c.customer_email, c.customer_name, c.customer_phone, c.customer_address,
			c.expires_at, c.success_url, c.created_at,
			i.id::text, i.status, i.expected_amount::text, i.received_amount::text, i.tolerance_percent::text,
			i.min_acceptable_amount::text, i.max_acceptable_amount::text, i.confirmations, i.required_confirmations, i.tx_hash,
			i.confirmed_at, i.settled_at, i.expires_at, i.created_at, i.updated_at,
			d.address
		FROM checkout.checkout_sessions c
		JOIN checkout.payment_intents i ON i.checkout_session_id=c.id
		JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		WHERE c.id=$1 AND c.merchant_id=$2
	`
	var sid, linkID, sessStatus, amountRaw, currency, chainName, tokenSymbol string
	var tokenAddress, customerEmail, customerName, customerPhone, successURL, txHash *string
	var customerAddress []byte
	var expiresAt, createdAt time.Time
	var intentID, intentStatus, expectedRaw, receivedRaw, tolRaw, minRaw, maxRaw string
	var confirmations, requiredConfs int
	var confirmedAt, settledAt *time.Time
	var intentExpiresAt, intentCreatedAt, intentUpdatedAt time.Time
	var depositAddress string
	if err := s.db.QueryRow(ctx, query, sessionID, merchantID).Scan(
		&sid, &linkID, &sessStatus, &amountRaw, &currency, &chainName, &tokenSymbol, &tokenAddress,
		&customerEmail, &customerName, &customerPhone, &customerAddress,
		&expiresAt, &successURL, &createdAt,
		&intentID, &intentStatus, &expectedRaw, &receivedRaw, &tolRaw,
		&minRaw, &maxRaw, &confirmations, &requiredConfs, &txHash,
		&confirmedAt, &settledAt, &intentExpiresAt, &intentCreatedAt, &intentUpdatedAt,
		&depositAddress,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "checkout session not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch checkout session")
	}

	return &cpayv1.CheckoutSession{
		Id:                  sid,
		PaymentLinkId:       linkID,
		Status:              sessStatus,
		Amount:              parseFloatValue(amountRaw),
		Currency:            currency,
		Chain:               chainName,
		TokenSymbol:         tokenSymbol,
		TokenAddress:        strValue(tokenAddress),
		CustomerEmail:       strValue(customerEmail),
		CustomerName:        strValue(customerName),
		CustomerPhone:       strValue(customerPhone),
		CustomerAddressJson: bytesOrDefault(customerAddress, "{}"),
		ExpiresAt:           expiresAt.UTC().Format(time.RFC3339Nano),
		SuccessUrl:          strValue(successURL),
		CreatedAt:           createdAt.UTC().Format(time.RFC3339Nano),
		PaymentIntent: &cpayv1.PaymentIntent{
			Id:                    intentID,
			CheckoutSessionId:     sid,
			PaymentLinkId:         linkID,
			Status:                intentStatus,
			Chain:                 chainName,
			TokenSymbol:           tokenSymbol,
			TokenAddress:          strValue(tokenAddress),
			ExpectedAmount:        parseFloatValue(expectedRaw),
			ReceivedAmount:        parseFloatValue(receivedRaw),
			TolerancePercent:      parseFloatValue(tolRaw),
			MinAcceptableAmount:   parseFloatValue(minRaw),
			MaxAcceptableAmount:   parseFloatValue(maxRaw),
			TxHash:                strValue(txHash),
			Confirmations:         int32(confirmations),
			RequiredConfirmations: int32(requiredConfs),
			ConfirmedAt:           formatTimePtr(confirmedAt),
			SettledAt:             formatTimePtr(settledAt),
			ExpiresAt:             intentExpiresAt.UTC().Format(time.RFC3339Nano),
			DepositAddress:        depositAddress,
			CreatedAt:             intentCreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:             intentUpdatedAt.UTC().Format(time.RFC3339Nano),
		},
	}, nil
}

func (s *Service) GetPublicCheckoutSession(ctx context.Context, req *cpayv1.GetPublicCheckoutSessionRequest) (*cpayv1.PublicCheckoutSession, error) {
	sessionID, err := ids.Parse(strings.TrimSpace(req.GetSessionId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "session_id is invalid")
	}
	query := `
		SELECT c.id::text, c.status, c.amount::text, c.currency, c.chain, c.token_symbol, c.expires_at,
			i.id::text, i.status, i.received_amount::text, i.confirmations, i.required_confirmations, d.address,
			p.title, p.description, p.image_url, p.cta_text, p.after_payment_type, p.redirect_url, p.success_message
		FROM checkout.checkout_sessions c
		JOIN checkout.payment_intents i ON i.checkout_session_id=c.id
		JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		JOIN catalog.payment_links p ON p.id=c.payment_link_id
		WHERE c.id=$1
	`
	var id, status, amountRaw, currency, chainName, tokenSymbol string
	var expiresAt time.Time
	var intentID, intentStatus, receivedRaw, depositAddress, linkTitle, ctaText, afterType string
	var linkDescription, linkImage, redirectURL, successMessage *string
	var confirmations, requiredConfs int
	if err := s.db.QueryRow(ctx, query, sessionID).Scan(
		&id, &status, &amountRaw, &currency, &chainName, &tokenSymbol, &expiresAt,
		&intentID, &intentStatus, &receivedRaw, &confirmations, &requiredConfs, &depositAddress,
		&linkTitle, &linkDescription, &linkImage, &ctaText, &afterType, &redirectURL, &successMessage,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "checkout session not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch checkout session")
	}

	transactions, err := s.checkoutTransactions(ctx, s.db, intentID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch checkout transactions")
	}

	return &cpayv1.PublicCheckoutSession{
		Id:                         id,
		Status:                     status,
		Amount:                     parseFloatValue(amountRaw),
		Currency:                   currency,
		Chain:                      chainName,
		TokenSymbol:                tokenSymbol,
		ExpiresAt:                  expiresAt.UTC().Format(time.RFC3339Nano),
		PaymentIntentId:            intentID,
		DepositAddress:             depositAddress,
		PaymentIntentStatus:        intentStatus,
		ReceivedAmount:             parseFloatValue(receivedRaw),
		Confirmations:              int32(confirmations),
		RequiredConfirmations:      int32(requiredConfs),
		Transactions:               transactions,
		LinkTitle:                  linkTitle,
		LinkDescription:            strValue(linkDescription),
		LinkImageUrl:               strValue(linkImage),
		CtaText:                    ctaText,
		AfterPaymentType:           afterType,
		AfterPaymentRedirectUrl:    strValue(redirectURL),
		AfterPaymentSuccessMessage: strValue(successMessage),
	}, nil
}

func (s *Service) checkoutTransactions(ctx context.Context, q queryer, intentID string) ([]*cpayv1.CheckoutTransaction, error) {
	rows, err := q.Query(ctx, `
		SELECT tx_hash, amount::text, chain, token_symbol, confirmations, status, block_number
		FROM checkout.chain_transactions
		WHERE payment_intent_id=$1
		ORDER BY observed_at DESC, created_at DESC
	`, intentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*cpayv1.CheckoutTransaction
	for rows.Next() {
		var txHash, amountRaw, chainName, tokenSymbol, status string
		var confirmations int
		var blockNumber *int64
		if err := rows.Scan(&txHash, &amountRaw, &chainName, &tokenSymbol, &confirmations, &status, &blockNumber); err != nil {
			return nil, err
		}
		item := &cpayv1.CheckoutTransaction{
			TxHash:        txHash,
			Amount:        parseFloatValue(amountRaw),
			Chain:         chainName,
			TokenSymbol:   tokenSymbol,
			Confirmations: int32(confirmations),
			Status:        status,
		}
		if blockNumber != nil {
			item.BlockNumber = *blockNumber
			item.HasBlockNumber = true
		}
		transactions = append(transactions, item)
	}
	return transactions, rows.Err()
}

func (s *Service) ConfirmCheckoutSession(ctx context.Context, req *cpayv1.ConfirmCheckoutSessionRequest) (*cpayv1.ConfirmCheckoutSessionResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	sessionID, err := ids.Parse(strings.TrimSpace(req.GetSessionId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "session_id is invalid")
	}
	if strings.TrimSpace(req.GetTxHash()) == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "tx_hash is required")
	}

	query := `
		SELECT i.id::text, i.payment_link_id::text, i.expected_amount::text, i.tolerance_percent::text, i.required_confirmations,
			i.chain, i.token_symbol, c.currency, p.title, COALESCE(l.add_invoice_pdf, false)
		FROM checkout.payment_intents i
		JOIN checkout.checkout_sessions c ON c.id=i.checkout_session_id
		JOIN catalog.payment_links p ON p.id=i.payment_link_id
		LEFT JOIN catalog.link_options l ON l.payment_link_id=p.id
		WHERE c.id=$1 AND i.merchant_id=$2
	`
	var intentIDStr, expectedRaw, toleranceRaw string
	var requiredConfs int
	var chainName, tokenSymbol, currency, title string
	var addInvoice bool
	if err := s.db.QueryRow(ctx, query, sessionID, merchantID).Scan(
		&intentIDStr, new(string), &expectedRaw, &toleranceRaw, &requiredConfs,
		&chainName, &tokenSymbol, &currency, &title, &addInvoice,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "checkout session not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to load payment intent")
	}

	expected := parseFloatValue(expectedRaw)
	tolerance := parseFloatValue(toleranceRaw)
	txStatus := "detected"
	if int(req.GetConfirmations()) >= requiredConfs {
		txStatus = "confirmed"
	}

	intentID, _ := ids.Parse(intentIDStr)
	rawPayload := req.GetRawPayloadJson()
	if strings.TrimSpace(rawPayload) == "" {
		rawPayload = "{}"
	}
	if _, err := normalizeJSON(rawPayload, "{}"); err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "raw_payload_json is invalid")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	var blockNumber any
	if req.GetHasBlockNumber() {
		blockNumber = req.GetBlockNumber()
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.chain_transactions(
			id, payment_intent_id, chain, tx_hash, block_number, from_address, to_address, amount,
			token_symbol, token_address, confirmations, status, raw_payload, observed_at, created_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, $10, $11, $12::jsonb, NOW(), NOW())
		ON CONFLICT (chain, tx_hash, payment_intent_id)
		DO UPDATE SET
			block_number=COALESCE(EXCLUDED.block_number, checkout.chain_transactions.block_number),
			from_address=COALESCE(EXCLUDED.from_address, checkout.chain_transactions.from_address),
			to_address=COALESCE(EXCLUDED.to_address, checkout.chain_transactions.to_address),
			amount=EXCLUDED.amount,
			token_symbol=EXCLUDED.token_symbol,
			confirmations=EXCLUDED.confirmations,
			status=EXCLUDED.status,
			raw_payload=EXCLUDED.raw_payload,
			observed_at=NOW()
	`, ids.New(), intentID, chainName, req.GetTxHash(), blockNumber, nullIfEmpty(req.GetFromAddress()), nullIfEmpty(req.GetToAddress()), req.GetReceivedAmount(),
		tokenSymbol, req.GetConfirmations(), txStatus, rawPayload)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to write chain transaction")
	}

	var totalReceivedRaw, confirmedReceivedRaw, latestTxHash string
	var aggregateConfirmations int
	if err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount), 0)::text,
			COALESCE(SUM(amount) FILTER (WHERE confirmations >= $2), 0)::text,
			COALESCE(MIN(confirmations), 0),
			COALESCE((ARRAY_AGG(tx_hash ORDER BY observed_at DESC, created_at DESC))[1], '')
		FROM checkout.chain_transactions
		WHERE payment_intent_id=$1
	`, intentID, requiredConfs).Scan(&totalReceivedRaw, &confirmedReceivedRaw, &aggregateConfirmations, &latestTxHash); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to aggregate chain transactions")
	}
	totalReceived := parseFloatValue(totalReceivedRaw)
	confirmedReceived := parseFloatValue(confirmedReceivedRaw)
	confirmedStatus := payment.ResolveIntentStatus(expected, confirmedReceived, tolerance, requiredConfs, requiredConfs)
	statusIsPaid := payment.IntentStatusIsPaid(confirmedStatus)
	newStatus := confirmedStatus
	if !statusIsPaid {
		newStatus = payment.ResolveIntentStatus(expected, totalReceived, tolerance, aggregateConfirmations, requiredConfs)
	}

	var confirmedAt any
	if statusIsPaid {
		confirmedAt = time.Now().UTC()
	}
	_, err = tx.Exec(ctx, `
		UPDATE checkout.payment_intents
		SET status=$1, received_amount=$2, tx_hash=$3, confirmations=$4, confirmed_at=COALESCE(confirmed_at, $5), updated_at=NOW()
		WHERE id=$6
	`, newStatus, totalReceived, latestTxHash, aggregateConfirmations, confirmedAt, intentID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to update payment intent")
	}

	sessionStatus := "awaiting_funds"
	if statusIsPaid {
		sessionStatus = "paid"
	} else if newStatus == "expired" || newStatus == "failed" {
		sessionStatus = "failed"
	}
	_, err = tx.Exec(ctx, `UPDATE checkout.checkout_sessions SET status=$1, updated_at=NOW() WHERE id=$2`, sessionStatus, sessionID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to update checkout session")
	}

	_ = s.outbox.EnqueueTx(ctx, tx, "payment_intent", intentIDStr, &merchantID, "payment.detected", map[string]any{
		"payment_intent_id": intentID,
		"tx_hash":           req.GetTxHash(),
		"received_amount":   req.GetReceivedAmount(),
		"confirmations":     req.GetConfirmations(),
		"status":            newStatus,
	})
	if statusIsPaid {
		_ = s.outbox.EnqueueTx(ctx, tx, "payment_intent", intentIDStr, &merchantID, "payment.confirmed", map[string]any{
			"payment_intent_id": intentID,
			"tx_hash":           req.GetTxHash(),
			"received_amount":   req.GetReceivedAmount(),
		})
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to commit transaction")
	}

	if statusIsPaid && addInvoice {
		if objectKey, invErr := s.generateAndStoreInvoice(ctx, merchantID, intentID, req.GetReceivedAmount(), currency, title); invErr == nil && objectKey != "" {
			invoiceID := ids.New()
			cmd, insErr := s.db.Exec(ctx, `
				INSERT INTO checkout.invoices(id, payment_intent_id, merchant_id, object_key, amount, currency, created_at)
				VALUES($1, $2, $3, $4, $5, $6, NOW())
				ON CONFLICT (payment_intent_id) DO NOTHING
			`, invoiceID, intentID, merchantID, objectKey, req.GetReceivedAmount(), currency)
			if insErr == nil && cmd.RowsAffected() > 0 {
				_ = s.outbox.Enqueue(ctx, "invoice", invoiceID, &merchantID, "invoice.created", map[string]any{
					"invoice_id":          invoiceID,
					"payment_intent_id":   intentID,
					"merchant_id":         merchantID,
					"object_key":          objectKey,
					"amount":              req.GetReceivedAmount(),
					"currency":            currency,
					"checkout_session_id": sessionID,
				})
			}
		}
	}

	return &cpayv1.ConfirmCheckoutSessionResponse{
		CheckoutSessionId:     sessionID,
		PaymentIntentId:       intentID,
		Status:                newStatus,
		Confirmations:         int32(aggregateConfirmations),
		RequiredConfirmations: int32(requiredConfs),
	}, nil
}

func (s *Service) GetPaymentIntent(ctx context.Context, req *cpayv1.GetPaymentIntentRequest) (*cpayv1.PaymentIntent, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	intentID := strings.TrimSpace(req.GetId())
	if intentID == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "id is required")
	}

	query := `
		SELECT i.id::text, i.checkout_session_id::text, i.payment_link_id::text, i.status, i.chain, i.token_symbol, i.token_address,
			i.expected_amount::text, i.tolerance_percent::text, i.min_acceptable_amount::text, i.max_acceptable_amount::text,
			i.received_amount::text, i.tx_hash, i.confirmations, i.required_confirmations,
			i.confirmed_at, i.settled_at, i.expires_at, i.created_at, i.updated_at,
			d.address
		FROM checkout.payment_intents i
		LEFT JOIN checkout.deposit_addresses d ON d.payment_intent_id=i.id
		WHERE i.merchant_id=$1 AND i.id::text=$2
	`
	var id, sessionID, linkID, status, chainName, tokenSymbol string
	var tokenAddress, txHash, depositAddress *string
	var expectedRaw, tolRaw, minRaw, maxRaw, receivedRaw string
	var confs, requiredConfs int
	var confirmedAt, settledAt *time.Time
	var expiresAt, createdAt, updatedAt time.Time
	if err := s.db.QueryRow(ctx, query, merchantID, intentID).Scan(
		&id, &sessionID, &linkID, &status, &chainName, &tokenSymbol, &tokenAddress,
		&expectedRaw, &tolRaw, &minRaw, &maxRaw, &receivedRaw, &txHash, &confs, &requiredConfs,
		&confirmedAt, &settledAt, &expiresAt, &createdAt, &updatedAt,
		&depositAddress,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "payment intent not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch payment intent")
	}

	return &cpayv1.PaymentIntent{
		Id:                    id,
		CheckoutSessionId:     sessionID,
		PaymentLinkId:         linkID,
		Status:                status,
		Chain:                 chainName,
		TokenSymbol:           tokenSymbol,
		TokenAddress:          strValue(tokenAddress),
		ExpectedAmount:        parseFloatValue(expectedRaw),
		TolerancePercent:      parseFloatValue(tolRaw),
		MinAcceptableAmount:   parseFloatValue(minRaw),
		MaxAcceptableAmount:   parseFloatValue(maxRaw),
		ReceivedAmount:        parseFloatValue(receivedRaw),
		TxHash:                strValue(txHash),
		Confirmations:         int32(confs),
		RequiredConfirmations: int32(requiredConfs),
		ConfirmedAt:           formatTimePtr(confirmedAt),
		SettledAt:             formatTimePtr(settledAt),
		ExpiresAt:             expiresAt.UTC().Format(time.RFC3339Nano),
		DepositAddress:        strValue(depositAddress),
		CreatedAt:             createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:             updatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) CreateWebhookEndpoint(ctx context.Context, req *cpayv1.CreateWebhookEndpointRequest) (*cpayv1.CreateWebhookEndpointResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	endpointURL := strings.TrimSpace(req.GetUrl())
	if endpointURL == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "url is required")
	}
	u, err := url.Parse(endpointURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "url must be http or https")
	}
	if u.Host == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "url host is required")
	}

	events := req.GetEvents()
	if len(events) == 0 {
		events = []string{"*"}
	}
	maxRetries := s.cfg.WebhookMaxRetries
	if req.GetMaxRetries() > 0 {
		maxRetries = int(req.GetMaxRetries())
	}

	secret, err := generateWebhookSecret()
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to generate webhook secret")
	}
	encryptedSecret, err := cryptox.EncryptString(s.encryptKey, secret)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to encrypt webhook secret")
	}
	eventsRaw, _ := json.Marshal(events)
	id := ids.New()
	_, err = s.db.Exec(ctx, `
		INSERT INTO checkout.webhook_endpoints(
			id, merchant_id, url, description, enabled, events, secret_encrypted, max_retries, created_at, updated_at
		)
		VALUES($1, $2, $3, $4, TRUE, $5::jsonb, $6, $7, NOW(), NOW())
	`, id, merchantID, endpointURL, nullIfEmpty(req.GetDescription()), string(eventsRaw), encryptedSecret, maxRetries)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create webhook endpoint")
	}

	_ = s.outbox.Enqueue(ctx, "webhook_endpoint", id, &merchantID, "webhook_endpoint.created", map[string]any{
		"webhook_endpoint_id": id,
		"url":                 endpointURL,
		"events":              events,
	})

	return &cpayv1.CreateWebhookEndpointResponse{
		Id:          id,
		Url:         endpointURL,
		Description: req.GetDescription(),
		Events:      events,
		MaxRetries:  int32(maxRetries),
		Secret:      secret,
	}, nil
}

func (s *Service) CreateSubscription(ctx context.Context, req *cpayv1.CreateSubscriptionRequest) (*cpayv1.CreateSubscriptionResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	if strings.TrimSpace(req.GetChain()) == "" || strings.TrimSpace(req.GetTokenSymbol()) == "" || req.GetAmount() <= 0 {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "chain, token_symbol and positive amount are required")
	}
	currency := strings.ToUpper(strings.TrimSpace(req.GetCurrency()))
	if currency == "" {
		currency = "USD"
	}
	intervalCount := int(req.GetIntervalCount())
	if intervalCount <= 0 {
		intervalCount = 1
	}
	intervalUnit := strings.ToLower(strings.TrimSpace(req.GetIntervalUnit()))
	if intervalUnit == "" {
		intervalUnit = "month"
	}
	if intervalUnit != "day" && intervalUnit != "week" && intervalUnit != "month" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "interval_unit must be day, week or month")
	}
	if strings.TrimSpace(req.GetVaultContractAddress()) == "" || strings.TrimSpace(req.GetVaultCustomerWallet()) == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "vault contract and customer wallet are required")
	}
	if req.GetVaultMaxTotalAmount() <= 0 {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "vault_max_total_amount must be positive")
	}
	remaining := req.GetVaultRemainingAmount()
	if remaining <= 0 {
		remaining = req.GetVaultMaxTotalAmount()
	}

	now := time.Now().UTC()
	nextBilling := addInterval(now, intervalUnit, intervalCount)
	if strings.TrimSpace(req.GetFirstBillingAt()) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.GetFirstBillingAt()))
		if err != nil {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "first_billing_at must be RFC3339")
		}
		nextBilling = t.UTC()
	}
	periodEnd := addInterval(nextBilling, intervalUnit, intervalCount)

	var paymentLinkID any
	if strings.TrimSpace(req.GetPaymentLinkId()) != "" {
		pid, err := ids.Parse(strings.TrimSpace(req.GetPaymentLinkId()))
		if err != nil {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "payment_link_id is invalid")
		}
		paymentLinkID = pid
	}
	metadataJSON, err := normalizeJSON(req.GetMetadataJson(), "{}")
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "metadata_json is invalid")
	}

	var vaultExpiresAt any
	if strings.TrimSpace(req.GetVaultExpiresAt()) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.GetVaultExpiresAt()))
		if err != nil {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "vault_expires_at must be RFC3339")
		}
		vaultExpiresAt = t.UTC()
	}

	subID := ids.New()
	vaultID := ids.New()
	cycleID := ids.New()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.subscriptions(
			id, merchant_id, payment_link_id, customer_ref, status, chain, token_symbol, token_address,
			amount, currency, interval_unit, interval_count, next_billing_at, vault_contract_address, metadata,
			created_at, updated_at
		)
		VALUES($1, $2, $3, $4, 'active', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, NOW(), NOW())
	`, subID, merchantID, paymentLinkID, nullIfEmpty(req.GetCustomerRef()), strings.ToLower(req.GetChain()), strings.ToUpper(req.GetTokenSymbol()), nullIfEmpty(req.GetTokenAddress()),
		req.GetAmount(), currency, intervalUnit, intervalCount, nextBilling, req.GetVaultContractAddress(), metadataJSON)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create subscription")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.vault_authorizations(
			id, merchant_id, subscription_id, chain, contract_address, customer_wallet,
			token_symbol, token_address, max_total_amount, remaining_amount, expires_at, status,
			created_at, updated_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active', NOW(), NOW())
	`, vaultID, merchantID, subID, strings.ToLower(req.GetChain()), req.GetVaultContractAddress(), req.GetVaultCustomerWallet(),
		strings.ToUpper(req.GetTokenSymbol()), nullIfEmpty(req.GetTokenAddress()), req.GetVaultMaxTotalAmount(), remaining, vaultExpiresAt)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create vault authorization")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout.subscription_cycles(
			id, subscription_id, cycle_index, period_start, period_end, due_at, status, amount,
			retry_count, created_at, updated_at
		)
		VALUES($1, $2, 1, $3, $4, $3, 'due', $5, 0, NOW(), NOW())
	`, cycleID, subID, nextBilling, periodEnd, req.GetAmount())
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create subscription cycle")
	}

	if err = s.outbox.EnqueueTx(ctx, tx, "subscription", subID, &merchantID, "subscription.created", map[string]any{
		"subscription_id": subID,
		"next_billing_at": nextBilling,
		"amount":          req.GetAmount(),
	}); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to enqueue outbox event")
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to commit transaction")
	}

	return &cpayv1.CreateSubscriptionResponse{
		Id:                   subID,
		Status:               "active",
		Chain:                strings.ToLower(req.GetChain()),
		TokenSymbol:          strings.ToUpper(req.GetTokenSymbol()),
		Amount:               req.GetAmount(),
		Currency:             currency,
		IntervalUnit:         intervalUnit,
		IntervalCount:        int32(intervalCount),
		NextBillingAt:        nextBilling.UTC().Format(time.RFC3339Nano),
		VaultAuthorizationId: vaultID,
		VaultRemainingAmount: remaining,
		FirstCycleId:         cycleID,
	}, nil
}

func (s *Service) PauseSubscription(ctx context.Context, req *cpayv1.PauseSubscriptionRequest) (*cpayv1.SubscriptionMutationResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	subID, err := ids.Parse(strings.TrimSpace(req.GetId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "subscription id is invalid")
	}
	cmd, err := s.db.Exec(ctx, `
		UPDATE checkout.subscriptions SET status='paused', updated_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND status='active'
	`, subID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to pause subscription")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "subscription not found or not active")
	}
	_ = s.outbox.Enqueue(ctx, "subscription", subID, &merchantID, "subscription.paused", map[string]any{"subscription_id": subID})
	return &cpayv1.SubscriptionMutationResponse{Id: subID, Status: "paused"}, nil
}

func (s *Service) ResumeSubscription(ctx context.Context, req *cpayv1.ResumeSubscriptionRequest) (*cpayv1.SubscriptionMutationResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	subID, err := ids.Parse(strings.TrimSpace(req.GetId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "subscription id is invalid")
	}
	cmd, err := s.db.Exec(ctx, `
		UPDATE checkout.subscriptions SET status='active', updated_at=NOW()
		WHERE id=$1 AND merchant_id=$2 AND status IN ('paused', 'past_due')
	`, subID, merchantID)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to resume subscription")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "subscription not found")
	}
	_ = s.outbox.Enqueue(ctx, "subscription", subID, &merchantID, "subscription.resumed", map[string]any{"subscription_id": subID})
	return &cpayv1.SubscriptionMutationResponse{Id: subID, Status: "active"}, nil
}

func (s *Service) GetSubscriptionCycles(ctx context.Context, req *cpayv1.GetSubscriptionCyclesRequest) (*cpayv1.GetSubscriptionCyclesResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	subID, err := ids.Parse(strings.TrimSpace(req.GetId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "subscription id is invalid")
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 20
	}

	rows, err := s.db.Query(ctx, `
		SELECT c.id::text, c.cycle_index, c.period_start, c.period_end, c.due_at, c.status, c.amount::text,
			c.payment_intent_id::text, c.retry_count, c.last_error, c.created_at, c.updated_at
		FROM checkout.subscription_cycles c
		JOIN checkout.subscriptions s ON s.id=c.subscription_id
		WHERE c.subscription_id=$1 AND s.merchant_id=$2
		ORDER BY c.cycle_index DESC
		LIMIT $3
	`, subID, merchantID, limit)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list cycles")
	}
	defer rows.Close()

	items := make([]*cpayv1.SubscriptionCycle, 0)
	for rows.Next() {
		var id string
		var idx int
		var start, end, due time.Time
		var status, amountRaw string
		var paymentIntentID, lastErr *string
		var retryCount int
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &idx, &start, &end, &due, &status, &amountRaw, &paymentIntentID, &retryCount, &lastErr, &createdAt, &updatedAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan cycle")
		}
		items = append(items, &cpayv1.SubscriptionCycle{
			Id:              id,
			CycleIndex:      int32(idx),
			PeriodStart:     start.UTC().Format(time.RFC3339Nano),
			PeriodEnd:       end.UTC().Format(time.RFC3339Nano),
			DueAt:           due.UTC().Format(time.RFC3339Nano),
			Status:          status,
			Amount:          parseFloatValue(amountRaw),
			PaymentIntentId: strValue(paymentIntentID),
			RetryCount:      int32(retryCount),
			LastError:       strValue(lastErr),
			CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:       updatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	return &cpayv1.GetSubscriptionCyclesResponse{SubscriptionId: subID, Data: items}, nil
}

func tokenAllowed(allowed []allowedToken, chainName, symbol, address string) bool {
	if len(allowed) == 0 {
		return true
	}
	chainName = strings.ToLower(strings.TrimSpace(chainName))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	address = strings.ToLower(strings.TrimSpace(address))
	for _, t := range allowed {
		if strings.ToLower(strings.TrimSpace(t.Chain)) != chainName {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(t.Symbol)) != symbol {
			continue
		}
		if strings.TrimSpace(t.Address) == "" || address == "" {
			return true
		}
		if strings.ToLower(strings.TrimSpace(t.Address)) == address {
			return true
		}
	}
	return false
}

func newClientSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "chksec_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func generateWebhookSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeJSON(raw, def string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	var dst any
	if err := json.Unmarshal([]byte(raw), &dst); err != nil {
		return "", err
	}
	return raw, nil
}

func addInterval(t time.Time, unit string, count int) time.Time {
	if count <= 0 {
		count = 1
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "day":
		return t.Add(time.Duration(count) * 24 * time.Hour)
	case "week":
		return t.Add(time.Duration(count*7) * 24 * time.Hour)
	default:
		return t.AddDate(0, count, 0)
	}
}

func parseFloatValue(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func formatTimePtr(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}

func strValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func nullIfEmpty(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func bytesOrDefault(raw []byte, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}

func (s *Service) generateAndStoreInvoice(ctx context.Context, merchantID, paymentIntentID string, amount float64, currency, title string) (string, error) {
	if s.minio == nil {
		return "", nil
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 18)
	pdf.Cell(40, 10, "CPay Invoice")
	pdf.Ln(14)
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(80, 8, fmt.Sprintf("Payment Intent: %s", paymentIntentID))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Merchant: %s", merchantID))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Title: %s", title))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Amount: %.8f %s", amount, currency))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", err
	}

	objectKey := fmt.Sprintf("invoices/%s/%s.pdf", merchantID, paymentIntentID)
	if err := s.minio.PutObjectBytes(ctx, objectKey, "application/pdf", buf.Bytes()); err != nil {
		return "", err
	}
	return objectKey, nil
}
