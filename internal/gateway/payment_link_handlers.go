package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/format"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type paymentLinkAllowedToken struct {
	Chain    string `json:"chain"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

type paymentLinkAfterPayment struct {
	Type           string `json:"type"`
	RedirectURL    string `json:"redirect_url,omitempty"`
	SuccessMessage string `json:"success_message,omitempty"`
}

type paymentLinkOptionsRequest struct {
	CollectEmail            bool           `json:"collect_email"`
	CollectName             bool           `json:"collect_name"`
	CollectPhone            bool           `json:"collect_phone"`
	CollectAddress          bool           `json:"collect_address"`
	CollectBusinessName     bool           `json:"collect_business_name"`
	RequireTermsAcceptance  bool           `json:"require_terms_acceptance"`
	AllowPromoCodes         bool           `json:"allow_promo_codes"`
	CollectTaxAutomatically bool           `json:"collect_tax_automatically"`
	AddInvoicePDF           bool           `json:"add_invoice_pdf"`
	Metadata                map[string]any `json:"metadata,omitempty"`
}

type createPaymentLinkRequest struct {
	ProductID      *string                   `json:"product_id,omitempty"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description,omitempty"`
	ImageURL       string                    `json:"image_url,omitempty"`
	PricingMode    string                    `json:"pricing_mode"`
	Amount         *float64                  `json:"amount,omitempty"`
	Currency       string                    `json:"currency"`
	AllowedTokens  []paymentLinkAllowedToken `json:"allowed_tokens"`
	Reusable       *bool                     `json:"reusable,omitempty"`
	MaxPayments    *int                      `json:"max_payments,omitempty"`
	ExpiresAt      *time.Time                `json:"expires_at,omitempty"`
	CTAText        string                    `json:"cta_text,omitempty"`
	AfterPayment   paymentLinkAfterPayment   `json:"after_payment"`
	AdjustPercent  *float64                  `json:"adjust_percent,omitempty"`
	MinAmount      *float64                  `json:"min_amount,omitempty"`
	MaxAmount      *float64                  `json:"max_amount,omitempty"`
	CustomerFields []string                  `json:"customer_fields,omitempty"`
	CustomFields   []map[string]any          `json:"custom_fields,omitempty"`
	Options        paymentLinkOptionsRequest `json:"options"`
	Metadata       map[string]any            `json:"metadata,omitempty"`
}

func (s *Server) handleCreatePaymentLink(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}

	var req createPaymentLinkRequest
	if !s.parseJSON(w, r, &req) {
		return
	}

	customFieldsJSON := make([]string, 0, len(req.CustomFields))
	for _, item := range req.CustomFields {
		b, err := json.Marshal(item)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "custom_fields must be valid json objects", middleware.GetRequestID(r.Context()))
			return
		}
		customFieldsJSON = append(customFieldsJSON, string(b))
	}

	allowedTokens := make([]*cpayv1.AllowedToken, 0, len(req.AllowedTokens))
	for _, t := range req.AllowedTokens {
		allowedTokens = append(allowedTokens, &cpayv1.AllowedToken{
			Chain:    t.Chain,
			Symbol:   t.Symbol,
			Address:  t.Address,
			Decimals: int32(t.Decimals),
		})
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/payment_links", func() (int, any, error) {
		rpcReq := &cpayv1.CreatePaymentLinkRequest{
			MerchantId:       reqAuth.MerchantID,
			Title:            req.Title,
			Description:      req.Description,
			ImageUrl:         req.ImageURL,
			PricingMode:      req.PricingMode,
			Amount:           req.Amount,
			Currency:         req.Currency,
			AllowedTokens:    allowedTokens,
			Reusable:         req.Reusable,
			CtaText:          req.CTAText,
			AfterPayment:     &cpayv1.AfterPaymentConfig{Type: req.AfterPayment.Type, RedirectUrl: req.AfterPayment.RedirectURL, SuccessMessage: req.AfterPayment.SuccessMessage},
			AdjustPercent:    req.AdjustPercent,
			MinAmount:        req.MinAmount,
			MaxAmount:        req.MaxAmount,
			CustomerFields:   req.CustomerFields,
			CustomFieldsJson: customFieldsJSON,
			Options: &cpayv1.LinkOptions{
				CollectEmail:            req.Options.CollectEmail,
				CollectName:             req.Options.CollectName,
				CollectPhone:            req.Options.CollectPhone,
				CollectAddress:          req.Options.CollectAddress,
				CollectBusinessName:     req.Options.CollectBusinessName,
				RequireTermsAcceptance:  req.Options.RequireTermsAcceptance,
				AllowPromoCodes:         req.Options.AllowPromoCodes,
				CollectTaxAutomatically: req.Options.CollectTaxAutomatically,
				AddInvoicePdf:           req.Options.AddInvoicePDF,
				MetadataJson:            format.JSONStringOrDefault(req.Options.Metadata, "{}"),
			},
			MetadataJson: format.JSONStringOrDefault(req.Metadata, "{}"),
		}
		if req.ProductID != nil {
			rpcReq.ProductId = strings.TrimSpace(*req.ProductID)
		}
		if req.MaxPayments != nil {
			v := int32(*req.MaxPayments)
			rpcReq.MaxPayments = &v
		}
		if req.ExpiresAt != nil {
			rpcReq.ExpiresAt = req.ExpiresAt.UTC().Format(time.RFC3339)
		}
		resp, err := s.linkClient.CreatePaymentLink(s.rpcContext(r.Context()), rpcReq)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{
			"id":         resp.GetId(),
			"code":       resp.GetCode(),
			"url":        resp.GetUrl(),
			"title":      resp.GetTitle(),
			"created_at": resp.GetCreatedAt(),
		}, nil
	})
}

func (s *Server) handleListPaymentLinks(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	limit, offset := httpx.ParsePagination(r, 20, 100)

	resp, err := s.linkClient.ListPaymentLinks(s.rpcContext(r.Context()), &cpayv1.ListPaymentLinksRequest{
		MerchantId: reqAuth.MerchantID,
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}

	items := make([]map[string]any, 0, len(resp.GetData()))
	for _, item := range resp.GetData() {
		items = append(items, map[string]any{
			"id":           item.GetId(),
			"code":         item.GetCode(),
			"title":        item.GetTitle(),
			"pricing_mode": item.GetPricingMode(),
			"amount":       floatPtrValue(item.Amount),
			"currency":     item.GetCurrency(),
			"status":       item.GetStatus(),
			"reusable":     item.GetReusable(),
			"max_payments": int32PtrValue(item.MaxPayments),
			"expires_at":   format.StringOrNil(item.GetExpiresAt()),
			"created_at":   item.GetCreatedAt(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items, "limit": limit, "offset": offset})
}

func (s *Server) handleGetPaymentLink(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	resp, err := s.linkClient.GetPaymentLink(s.rpcContext(r.Context()), &cpayv1.GetPaymentLinkRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}

	allowed := make([]map[string]any, 0, len(resp.GetAllowedTokens()))
	for _, t := range resp.GetAllowedTokens() {
		allowed = append(allowed, map[string]any{
			"chain":    t.GetChain(),
			"symbol":   t.GetSymbol(),
			"address":  format.StringOrNil(t.GetAddress()),
			"decimals": t.GetDecimals(),
		})
	}
	customFields := make([]any, 0, len(resp.GetCustomFieldsJson()))
	for _, raw := range resp.GetCustomFieldsJson() {
		customFields = append(customFields, format.JSONValueOrDefault(raw, map[string]any{}))
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":           resp.GetId(),
		"code":         resp.GetCode(),
		"title":        resp.GetTitle(),
		"description":  format.StringOrNil(resp.GetDescription()),
		"image_url":    format.StringOrNil(resp.GetImageUrl()),
		"pricing_mode": resp.GetPricingMode(),
		"amount":       floatPtrValue(resp.Amount),
		"currency":     resp.GetCurrency(),
		"reusable":     resp.GetReusable(),
		"max_payments": int32PtrValue(resp.MaxPayments),
		"expires_at":   format.StringOrNil(resp.GetExpiresAt()),
		"cta_text":     resp.GetCtaText(),
		"after_payment": map[string]any{
			"type":            resp.GetAfterPayment().GetType(),
			"success_message": format.StringOrNil(resp.GetAfterPayment().GetSuccessMessage()),
			"redirect_url":    format.StringOrNil(resp.GetAfterPayment().GetRedirectUrl()),
		},
		"status":          resp.GetStatus(),
		"adjust_percent":  floatPtrValue(resp.AdjustPercent),
		"min_amount":      floatPtrValue(resp.MinAmount),
		"max_amount":      floatPtrValue(resp.MaxAmount),
		"allowed_tokens":  allowed,
		"customer_fields": resp.GetCustomerFields(),
		"custom_fields":   customFields,
		"metadata":        format.JSONValueOrDefault(resp.GetMetadataJson(), map[string]any{}),
		"options": map[string]any{
			"collect_email":             resp.GetOptions().GetCollectEmail(),
			"collect_name":              resp.GetOptions().GetCollectName(),
			"collect_phone":             resp.GetOptions().GetCollectPhone(),
			"collect_address":           resp.GetOptions().GetCollectAddress(),
			"collect_business_name":     resp.GetOptions().GetCollectBusinessName(),
			"require_terms_acceptance":  resp.GetOptions().GetRequireTermsAcceptance(),
			"allow_promo_codes":         resp.GetOptions().GetAllowPromoCodes(),
			"collect_tax_automatically": resp.GetOptions().GetCollectTaxAutomatically(),
			"add_invoice_pdf":           resp.GetOptions().GetAddInvoicePdf(),
			"metadata":                  format.JSONValueOrDefault(resp.GetOptions().GetMetadataJson(), map[string]any{}),
		},
		"created_at": resp.GetCreatedAt(),
		"updated_at": resp.GetUpdatedAt(),
	})
}

func (s *Server) handleArchivePaymentLink(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.linkClient.ArchivePaymentLink(s.rpcContext(r.Context()), &cpayv1.ArchivePaymentLinkRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "archived": resp.GetArchived()})
}

type updatePaymentLinkRequest struct {
	Title         string                    `json:"title"`
	PricingMode   string                    `json:"pricing_mode"`
	Amount        *float64                  `json:"amount"`
	Currency      string                    `json:"currency"`
	AllowedTokens []paymentLinkAllowedToken `json:"allowed_tokens"`
	Metadata      map[string]any            `json:"metadata"`
}

func (s *Server) handleUpdatePaymentLink(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	var req updatePaymentLinkRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	allowedTokens := make([]*cpayv1.AllowedToken, 0, len(req.AllowedTokens))
	for _, t := range req.AllowedTokens {
		allowedTokens = append(allowedTokens, &cpayv1.AllowedToken{
			Chain:    t.Chain,
			Symbol:   t.Symbol,
			Address:  t.Address,
			Decimals: int32(t.Decimals),
		})
	}
	resp, err := s.linkClient.UpdatePaymentLink(s.rpcContext(r.Context()), &cpayv1.UpdatePaymentLinkRequest{
		MerchantId:          reqAuth.MerchantID,
		Id:                  id,
		Title:               req.Title,
		PricingMode:         req.PricingMode,
		Amount:              req.Amount,
		Currency:            req.Currency,
		MetadataJson:        format.JSONStringOrDefault(req.Metadata, "{}"),
		AllowedTokens:       allowedTokens,
		UpdateAllowedTokens: req.AllowedTokens != nil,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "updated": resp.GetUpdated()})
}

func (s *Server) handleGetPublicPaymentLink(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(chi.URLParam(r, "code"))
	if code == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "code is required", middleware.GetRequestID(r.Context()))
		return
	}

	var linkID, merchantID, linkCode, title, mode, currency, cta, amountRaw, afterType string
	var description, imageURL, successMessage, redirectURL *string
	var productID, productName, productDescription, productImageURL *string
	var allowedRaw, customerFieldsRaw, customFieldsRaw []byte
	var collectEmail, collectName, collectPhone, collectAddress bool

	err := s.db.QueryRow(r.Context(), `
		SELECT p.id::text, p.merchant_id::text, p.code, p.title, p.description, p.image_url, p.pricing_mode, COALESCE(p.amount::text, ''), p.currency,
			p.cta_text, p.allowed_tokens, p.customer_fields, p.custom_fields, p.after_payment_type, p.success_message, p.redirect_url,
			COALESCE(l.collect_email, false), COALESCE(l.collect_name, false), COALESCE(l.collect_phone, false), COALESCE(l.collect_address, false),
			pr.id::text, pr.name, pr.description, pr.image_url
		FROM catalog.payment_links p
		LEFT JOIN catalog.link_options l ON l.payment_link_id = p.id
		LEFT JOIN catalog.products pr ON pr.id = p.product_id
		WHERE p.code = $1 AND p.status = 'active' AND (p.expires_at IS NULL OR p.expires_at > NOW())
	`, code).Scan(
		&linkID, &merchantID, &linkCode, &title, &description, &imageURL, &mode, &amountRaw, &currency,
		&cta, &allowedRaw, &customerFieldsRaw, &customFieldsRaw, &afterType, &successMessage, &redirectURL,
		&collectEmail, &collectName, &collectPhone, &collectAddress,
		&productID, &productName, &productDescription, &productImageURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "payment link not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to lookup payment link", middleware.GetRequestID(r.Context()))
		return
	}

	var amount any
	if amountRaw != "" {
		amount = format.JSONValueOrDefault(amountRaw, nil)
	}

	payload := map[string]any{
		"id":              linkID,
		"code":            linkCode,
		"title":           title,
		"description":     strPtrToAny(description),
		"image_url":       strPtrToAny(imageURL),
		"pricing_mode":    mode,
		"amount":          amount,
		"currency":        currency,
		"cta_text":        cta,
		"allowed_tokens":  format.JSONValueOrDefault(string(allowedRaw), []any{}),
		"customer_fields": format.JSONValueOrDefault(string(customerFieldsRaw), []any{}),
		"custom_fields":   format.JSONValueOrDefault(string(customFieldsRaw), []any{}),
		"after_payment": map[string]any{
			"type":            afterType,
			"success_message": strPtrToAny(successMessage),
			"redirect_url":    strPtrToAny(redirectURL),
		},
		"options": map[string]any{
			"collect_email":   collectEmail,
			"collect_name":    collectName,
			"collect_phone":   collectPhone,
			"collect_address": collectAddress,
		},
	}
	if productID != nil {
		payload["product"] = map[string]any{
			"id":          strPtrToAny(productID),
			"name":        strPtrToAny(productName),
			"description": strPtrToAny(productDescription),
			"image_url":   strPtrToAny(productImageURL),
		}
	}

	if err := s.recordCheckoutConversionEvent(r.Context(), r, checkoutConversionEventInput{
		EventType:     conversionEventCheckoutPageOpened,
		MerchantID:    merchantID,
		PaymentLinkID: linkID,
		Metadata: map[string]any{
			"payment_link_code": linkCode,
		},
	}); err != nil {
		s.log.Warn().Err(err).Str("payment_link_id", linkID).Msg("failed to record checkout page open")
	}

	httpx.WriteJSON(w, http.StatusOK, payload)
}

func floatPtrValue(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func int32PtrValue(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}
