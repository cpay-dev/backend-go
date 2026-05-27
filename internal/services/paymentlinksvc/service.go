package paymentlinksvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/payment"
	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/platform/outbox"
	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/cpay-dev/cpay/internal/shared/format"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/random"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
)

type Service struct {
	cpayv1.UnimplementedPaymentLinkServiceServer

	cfg    config.Config
	log    zerolog.Logger
	db     *pgxpool.Pool
	outbox *outbox.Publisher
}

func New(cfg config.Config, log zerolog.Logger, db *pgxpool.Pool) *Service {
	return &Service{
		cfg:    cfg,
		log:    log,
		db:     db,
		outbox: outbox.New(db, cfg.ServiceName),
	}
}

func (s *Service) CreatePaymentLink(ctx context.Context, req *cpayv1.CreatePaymentLinkRequest) (*cpayv1.CreatePaymentLinkResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	if strings.TrimSpace(req.GetPricingMode()) == "" {
		if req.Amount != nil {
			req.PricingMode = "fixed"
		} else {
			req.PricingMode = "open"
		}
	}
	if strings.TrimSpace(req.GetCurrency()) == "" {
		req.Currency = "USD"
	}
	if req.AfterPayment == nil {
		req.AfterPayment = &cpayv1.AfterPaymentConfig{Type: "confirmation_page"}
	}
	if strings.TrimSpace(req.AfterPayment.GetType()) == "" {
		req.AfterPayment.Type = "confirmation_page"
	}
	if strings.TrimSpace(req.GetCtaText()) == "" {
		req.CtaText = "Pay"
	}

	reusable := true
	if req.Reusable != nil {
		reusable = req.GetReusable()
	}

	linkInput := payment.LinkInput{
		Title:            req.GetTitle(),
		PricingMode:      req.GetPricingMode(),
		Amount:           req.Amount,
		Currency:         req.GetCurrency(),
		AllowedTokens:    len(req.GetAllowedTokens()),
		Reusable:         reusable,
		AfterPaymentType: req.AfterPayment.GetType(),
		RedirectURL:      req.AfterPayment.GetRedirectUrl(),
		MinAmount:        req.MinAmount,
		MaxAmount:        req.MaxAmount,
	}
	if req.MaxPayments != nil {
		v := int(req.GetMaxPayments())
		linkInput.MaxPayments = &v
	}
	if strings.TrimSpace(req.GetExpiresAt()) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.GetExpiresAt()))
		if err != nil {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "expires_at must be RFC3339")
		}
		linkInput.ExpiresAt = &t
	}
	if err := payment.ValidateLinkInput(linkInput); err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", err.Error())
	}

	linkID := ids.New()
	code, err := random.Code("0123456789ABCDEFGHJKMNPQRSTVWXYZ", 16)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to generate link code")
	}

	allowedTokensRaw, err := json.Marshal(req.GetAllowedTokens())
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "allowed_tokens is invalid")
	}
	customerFieldsRaw, _ := json.Marshal(req.GetCustomerFields())
	customFieldsRaw, _ := json.Marshal(req.GetCustomFieldsJson())
	metadataJSON, err := format.JSONOrDefault(req.GetMetadataJson(), "{}")
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "metadata_json must be valid JSON")
	}
	optMetadataJSON, err := format.JSONOrDefault(req.GetOptions().GetMetadataJson(), "{}")
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "options.metadata_json must be valid JSON")
	}

	var productID any
	if strings.TrimSpace(req.GetProductId()) != "" {
		pid, err := ids.Parse(strings.TrimSpace(req.GetProductId()))
		if err != nil {
			return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "product_id is invalid")
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog.products WHERE id=$1 AND merchant_id=$2)`, pid, merchantID).Scan(&exists); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "product lookup failed")
		}
		if !exists {
			return nil, rpcx.E(codes.NotFound, "not_found", "product not found")
		}
		productID = pid
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	var expiresAt any
	if strings.TrimSpace(req.GetExpiresAt()) != "" {
		exp, _ := time.Parse(time.RFC3339, strings.TrimSpace(req.GetExpiresAt()))
		expiresAt = exp
	}

	adjust := 0.0
	if req.AdjustPercent != nil {
		adjust = req.GetAdjustPercent()
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO catalog.payment_links(
			id, merchant_id, product_id, code, title, description, image_url, pricing_mode, amount, currency, reusable,
			max_payments, expires_at, cta_text, after_payment_type, success_message, redirect_url, status,
			adjust_percent, min_amount, max_amount, allowed_tokens, customer_fields, custom_fields, metadata,
			created_at, updated_at
		)
		VALUES(
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17, 'active',
			$18, $19, $20, $21::jsonb, $22::jsonb, $23::jsonb, $24::jsonb,
			NOW(), NOW()
		)
	`, linkID, merchantID, productID, code, req.GetTitle(), req.GetDescription(), req.GetImageUrl(), strings.ToLower(req.GetPricingMode()), req.Amount, strings.ToUpper(req.GetCurrency()), reusable,
		req.MaxPayments, expiresAt, req.GetCtaText(), strings.ToLower(req.AfterPayment.GetType()), req.AfterPayment.GetSuccessMessage(), req.AfterPayment.GetRedirectUrl(),
		adjust, req.MinAmount, req.MaxAmount, string(allowedTokensRaw), string(customerFieldsRaw), string(customFieldsRaw), metadataJSON)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create payment link")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO catalog.link_options(
			payment_link_id, collect_email, collect_name, collect_phone, collect_address, collect_business_name,
			require_terms_acceptance, allow_promo_codes, collect_tax_automatically, add_invoice_pdf, metadata
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
	`, linkID, req.GetOptions().GetCollectEmail(), req.GetOptions().GetCollectName(), req.GetOptions().GetCollectPhone(), req.GetOptions().GetCollectAddress(), req.GetOptions().GetCollectBusinessName(),
		req.GetOptions().GetRequireTermsAcceptance(), req.GetOptions().GetAllowPromoCodes(), req.GetOptions().GetCollectTaxAutomatically(), req.GetOptions().GetAddInvoicePdf(), optMetadataJSON)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to save link options")
	}

	if err = s.outbox.EnqueueTx(ctx, tx, "payment_link", linkID, &merchantID, "payment_link.created", map[string]any{
		"payment_link_id": linkID,
		"code":            code,
		"title":           req.GetTitle(),
		"pricing_mode":    strings.ToLower(req.GetPricingMode()),
	}); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to enqueue outbox event")
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to commit transaction")
	}

	return &cpayv1.CreatePaymentLinkResponse{
		Id:        linkID,
		Code:      code,
		Url:       fmt.Sprintf("/pay/%s", code),
		Title:     req.GetTitle(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) ListPaymentLinks(ctx context.Context, req *cpayv1.ListPaymentLinksRequest) (*cpayv1.ListPaymentLinksResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(ctx, `
		SELECT id::text, code, title, pricing_mode, COALESCE(amount::text, ''), currency, status, reusable, max_payments, expires_at, created_at
		FROM catalog.payment_links
		WHERE merchant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, merchantID, limit, offset)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to list payment links")
	}
	defer rows.Close()

	items := make([]*cpayv1.PaymentLink, 0, limit)
	for rows.Next() {
		var id, code, title, mode, amountRaw, currency, status string
		var reusable bool
		var maxPayments *int32
		var expiresAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &code, &title, &mode, &amountRaw, &currency, &status, &reusable, &maxPayments, &expiresAt, &createdAt); err != nil {
			return nil, rpcx.E(codes.Internal, "internal_error", "failed to scan payment links")
		}
		item := &cpayv1.PaymentLink{
			Id:          id,
			Code:        code,
			Title:       title,
			PricingMode: mode,
			Currency:    currency,
			Status:      status,
			Reusable:    reusable,
			ExpiresAt:   format.TimePtr(expiresAt),
			CreatedAt:   createdAt.UTC().Format(time.RFC3339Nano),
		}
		if amount := format.Float64Ptr(amountRaw); amount != nil {
			item.Amount = amount
		}
		if maxPayments != nil {
			item.MaxPayments = maxPayments
		}
		items = append(items, item)
	}

	return &cpayv1.ListPaymentLinksResponse{Data: items, Limit: int32(limit), Offset: int32(offset)}, nil
}

func (s *Service) GetPaymentLink(ctx context.Context, req *cpayv1.GetPaymentLinkRequest) (*cpayv1.PaymentLink, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "id is required")
	}

	query := `
		SELECT p.id::text, p.code, p.title, p.description, p.image_url, p.pricing_mode, COALESCE(p.amount::text, ''), p.currency,
			p.reusable, p.max_payments, p.expires_at, p.cta_text, p.after_payment_type, p.success_message, p.redirect_url,
			p.status, COALESCE(p.adjust_percent::text, ''), COALESCE(p.min_amount::text, ''), COALESCE(p.max_amount::text, ''),
			p.allowed_tokens, p.customer_fields, p.custom_fields, p.metadata,
			l.collect_email, l.collect_name, l.collect_phone, l.collect_address, l.collect_business_name,
			l.require_terms_acceptance, l.allow_promo_codes, l.collect_tax_automatically, l.add_invoice_pdf, l.metadata,
			p.created_at, p.updated_at
		FROM catalog.payment_links p
		LEFT JOIN catalog.link_options l ON l.payment_link_id=p.id
		WHERE p.merchant_id=$1 AND (p.id::text=$2 OR p.code=$2)
	`

	var linkID, code, title, mode, amountRaw, currency, cta, afterType, status, adjustRaw, minRaw, maxRaw string
	var description, imageURL, successMsg, redirectURL *string
	var reusable bool
	var maxPayments *int32
	var expiresAt *time.Time
	var allowedRaw, customerRaw, customRaw, metaRaw []byte
	var collectEmail, collectName, collectPhone, collectAddress, collectBusiness bool
	var requireTerms, allowPromo, collectTax, addInvoice bool
	var optsMetaRaw []byte
	var createdAt, updatedAt time.Time

	err = s.db.QueryRow(ctx, query, merchantID, id).Scan(
		&linkID, &code, &title, &description, &imageURL, &mode, &amountRaw, &currency,
		&reusable, &maxPayments, &expiresAt, &cta, &afterType, &successMsg, &redirectURL,
		&status, &adjustRaw, &minRaw, &maxRaw,
		&allowedRaw, &customerRaw, &customRaw, &metaRaw,
		&collectEmail, &collectName, &collectPhone, &collectAddress, &collectBusiness,
		&requireTerms, &allowPromo, &collectTax, &addInvoice, &optsMetaRaw,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rpcx.E(codes.NotFound, "not_found", "payment link not found")
		}
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch payment link")
	}

	allowedTokens := []*cpayv1.AllowedToken{}
	if len(allowedRaw) > 0 {
		_ = json.Unmarshal(allowedRaw, &allowedTokens)
	}
	customerFields := []string{}
	if len(customerRaw) > 0 {
		_ = json.Unmarshal(customerRaw, &customerFields)
	}
	customFieldsJSON := parseCustomFields(customRaw)

	resp := &cpayv1.PaymentLink{
		Id:          linkID,
		Code:        code,
		Title:       title,
		Description: format.StringPtr(description),
		ImageUrl:    format.StringPtr(imageURL),
		PricingMode: mode,
		Currency:    currency,
		Reusable:    reusable,
		ExpiresAt:   format.TimePtr(expiresAt),
		CtaText:     cta,
		AfterPayment: &cpayv1.AfterPaymentConfig{
			Type:           afterType,
			RedirectUrl:    format.StringPtr(redirectURL),
			SuccessMessage: format.StringPtr(successMsg),
		},
		Status:           status,
		AllowedTokens:    allowedTokens,
		CustomerFields:   customerFields,
		CustomFieldsJson: customFieldsJSON,
		MetadataJson:     format.BytesOrDefault(metaRaw, "{}"),
		Options: &cpayv1.LinkOptions{
			CollectEmail:            collectEmail,
			CollectName:             collectName,
			CollectPhone:            collectPhone,
			CollectAddress:          collectAddress,
			CollectBusinessName:     collectBusiness,
			RequireTermsAcceptance:  requireTerms,
			AllowPromoCodes:         allowPromo,
			CollectTaxAutomatically: collectTax,
			AddInvoicePdf:           addInvoice,
			MetadataJson:            format.BytesOrDefault(optsMetaRaw, "{}"),
		},
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
	}
	if amount := format.Float64Ptr(amountRaw); amount != nil {
		resp.Amount = amount
	}
	if adjust := format.Float64Ptr(adjustRaw); adjust != nil {
		resp.AdjustPercent = adjust
	}
	if min := format.Float64Ptr(minRaw); min != nil {
		resp.MinAmount = min
	}
	if max := format.Float64Ptr(maxRaw); max != nil {
		resp.MaxAmount = max
	}
	if maxPayments != nil {
		resp.MaxPayments = maxPayments
	}
	return resp, nil
}

func (s *Service) ArchivePaymentLink(ctx context.Context, req *cpayv1.ArchivePaymentLinkRequest) (*cpayv1.ArchivePaymentLinkResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "id is required")
	}
	cmd, err := s.db.Exec(ctx, `
		UPDATE catalog.payment_links
		SET status='archived', archived_at=NOW(), updated_at=NOW()
		WHERE merchant_id=$1 AND (id::text=$2 OR code=$2) AND status!='archived'
	`, merchantID, id)
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to archive payment link")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "payment link not found")
	}
	_ = s.outbox.Enqueue(ctx, "payment_link", id, &merchantID, "payment_link.archived", map[string]any{"payment_link": id})
	return &cpayv1.ArchivePaymentLinkResponse{Id: id, Archived: true}, nil
}

func (s *Service) UpdatePaymentLink(ctx context.Context, req *cpayv1.UpdatePaymentLinkRequest) (*cpayv1.UpdatePaymentLinkResponse, error) {
	merchantID, err := ids.Parse(strings.TrimSpace(req.GetMerchantId()))
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "merchant_id is invalid")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "id is required")
	}
	title := strings.TrimSpace(req.GetTitle())
	if title == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "title is required")
	}
	currency := strings.TrimSpace(req.GetCurrency())
	if currency == "" {
		currency = "USD"
	}
	pricingMode := strings.TrimSpace(req.GetPricingMode())
	if pricingMode == "" {
		pricingMode = "fixed"
	}
	metadataJSON, err := format.JSONOrDefault(req.GetMetadataJson(), "{}")
	if err != nil {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "metadata is invalid")
	}
	allowedTokensJSON := "[]"
	if req.GetUpdateAllowedTokens() {
		tokens := make([]map[string]any, 0, len(req.GetAllowedTokens()))
		for _, t := range req.GetAllowedTokens() {
			tokens = append(tokens, map[string]any{
				"chain":    t.GetChain(),
				"symbol":   t.GetSymbol(),
				"address":  t.GetAddress(),
				"decimals": t.GetDecimals(),
			})
		}
		b, _ := json.Marshal(tokens)
		allowedTokensJSON = string(b)
	}
	var cmd interface{ RowsAffected() int64 }
	if req.GetUpdateAllowedTokens() {
		cmd, err = s.db.Exec(ctx, `
			UPDATE catalog.payment_links
			SET title=$3, pricing_mode=$4, amount=$5, currency=$6, metadata=$7::jsonb, allowed_tokens=$8::jsonb, updated_at=NOW()
			WHERE merchant_id=$1 AND (id::text=$2 OR code=$2) AND status!='archived'
		`, merchantID, id, title, strings.ToLower(pricingMode), req.Amount, strings.ToUpper(currency), metadataJSON, allowedTokensJSON)
	} else {
		cmd, err = s.db.Exec(ctx, `
			UPDATE catalog.payment_links
			SET title=$3, pricing_mode=$4, amount=$5, currency=$6, metadata=$7::jsonb, updated_at=NOW()
			WHERE merchant_id=$1 AND (id::text=$2 OR code=$2) AND status!='archived'
		`, merchantID, id, title, strings.ToLower(pricingMode), req.Amount, strings.ToUpper(currency), metadataJSON)
	}
	if err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to update payment link")
	}
	if cmd.RowsAffected() == 0 {
		return nil, rpcx.E(codes.NotFound, "not_found", "payment link not found")
	}
	return &cpayv1.UpdatePaymentLinkResponse{Id: id, Updated: true}, nil
}

func parseCustomFields(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		return asStrings
	}
	var asAny []any
	if err := json.Unmarshal(raw, &asAny); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(asAny))
	for _, item := range asAny {
		b, err := json.Marshal(item)
		if err != nil {
			continue
		}
		out = append(out, string(b))
	}
	return out
}
