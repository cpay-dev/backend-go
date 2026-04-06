package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/domain/payment"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type allowedToken struct {
	Chain    string `json:"chain"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

type afterPaymentConfig struct {
	Type           string `json:"type"`
	RedirectURL    string `json:"redirect_url,omitempty"`
	SuccessMessage string `json:"success_message,omitempty"`
}

type linkOptionsRequest struct {
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
	ProductID      *string            `json:"product_id,omitempty"`
	Title          string             `json:"title"`
	Description    string             `json:"description,omitempty"`
	ImageURL       string             `json:"image_url,omitempty"`
	PricingMode    string             `json:"pricing_mode"`
	Amount         *float64           `json:"amount,omitempty"`
	Currency       string             `json:"currency"`
	AllowedTokens  []allowedToken     `json:"allowed_tokens"`
	Reusable       *bool              `json:"reusable,omitempty"`
	MaxPayments    *int               `json:"max_payments,omitempty"`
	ExpiresAt      *time.Time         `json:"expires_at,omitempty"`
	CTAText        string             `json:"cta_text,omitempty"`
	AfterPayment   afterPaymentConfig `json:"after_payment"`
	AdjustPercent  *float64           `json:"adjust_percent,omitempty"`
	MinAmount      *float64           `json:"min_amount,omitempty"`
	MaxAmount      *float64           `json:"max_amount,omitempty"`
	CustomerFields []string           `json:"customer_fields,omitempty"`
	CustomFields   []map[string]any   `json:"custom_fields,omitempty"`
	Options        linkOptionsRequest `json:"options"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
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

	if strings.TrimSpace(req.PricingMode) == "" {
		if req.Amount != nil {
			req.PricingMode = "fixed"
		} else {
			req.PricingMode = "open"
		}
	}
	if strings.TrimSpace(req.Currency) == "" {
		req.Currency = "USD"
	}
	if strings.TrimSpace(req.AfterPayment.Type) == "" {
		req.AfterPayment.Type = "confirmation_page"
	}
	if strings.TrimSpace(req.CTAText) == "" {
		req.CTAText = "Pay"
	}
	reusable := true
	if req.Reusable != nil {
		reusable = *req.Reusable
	}

	if err := payment.ValidateLinkInput(payment.LinkInput{
		Title:            req.Title,
		PricingMode:      req.PricingMode,
		Amount:           req.Amount,
		Currency:         req.Currency,
		AllowedTokens:    len(req.AllowedTokens),
		Reusable:         reusable,
		MaxPayments:      req.MaxPayments,
		ExpiresAt:        req.ExpiresAt,
		AfterPaymentType: req.AfterPayment.Type,
		RedirectURL:      req.AfterPayment.RedirectURL,
		MinAmount:        req.MinAmount,
		MaxAmount:        req.MaxAmount,
	}); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/payment_links", func() (int, any, error) {
		linkID := uuid.New()
		code, err := newLinkCode()
		if err != nil {
			return 0, nil, err
		}

		allowedRaw, _ := json.Marshal(req.AllowedTokens)
		customerFieldsRaw, _ := json.Marshal(req.CustomerFields)
		customFieldsRaw, _ := json.Marshal(req.CustomFields)
		metadataRaw, _ := json.Marshal(req.Metadata)
		optsMetaRaw, _ := json.Marshal(req.Options.Metadata)
		adjustPercent := 0.0
		if req.AdjustPercent != nil {
			adjustPercent = *req.AdjustPercent
		}

		var productID any
		if req.ProductID != nil && strings.TrimSpace(*req.ProductID) != "" {
			pid, err := uuid.Parse(strings.TrimSpace(*req.ProductID))
			if err != nil {
				return 0, nil, badRequest("invalid product_id")
			}
			productID = pid
			var exists bool
			if err := s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND merchant_id=$2)`, pid, reqAuth.MerchantID).Scan(&exists); err != nil {
				return 0, nil, err
			}
			if !exists {
				return 0, nil, notFound("product not found")
			}
		}

		tx, err := s.db.Begin(r.Context())
		if err != nil {
			return 0, nil, err
		}
		defer tx.Rollback(r.Context())

		_, err = tx.Exec(r.Context(), `
			INSERT INTO payment_links(
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
		`, linkID, reqAuth.MerchantID, productID, code, req.Title, req.Description, req.ImageURL, strings.ToLower(req.PricingMode), req.Amount, strings.ToUpper(req.Currency), reusable,
			req.MaxPayments, req.ExpiresAt, req.CTAText, strings.ToLower(req.AfterPayment.Type), req.AfterPayment.SuccessMessage, req.AfterPayment.RedirectURL,
			adjustPercent, req.MinAmount, req.MaxAmount, string(allowedRaw), string(customerFieldsRaw), string(customFieldsRaw), string(metadataRaw))
		if err != nil {
			return 0, nil, err
		}

		_, err = tx.Exec(r.Context(), `
			INSERT INTO link_options(
				payment_link_id, collect_email, collect_name, collect_phone, collect_address, collect_business_name,
				require_terms_acceptance, allow_promo_codes, collect_tax_automatically, add_invoice_pdf, metadata
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
		`, linkID, req.Options.CollectEmail, req.Options.CollectName, req.Options.CollectPhone, req.Options.CollectAddress, req.Options.CollectBusinessName,
			req.Options.RequireTermsAcceptance, req.Options.AllowPromoCodes, req.Options.CollectTaxAutomatically, req.Options.AddInvoicePDF, string(optsMetaRaw))
		if err != nil {
			return 0, nil, err
		}

		if err = s.enqueueEventTx(r.Context(), tx, "payment_link", linkID.String(), reqAuth.MerchantID, "payment_link.created", map[string]any{
			"payment_link_id": linkID,
			"code":            code,
			"title":           req.Title,
			"pricing_mode":    strings.ToLower(req.PricingMode),
		}); err != nil {
			return 0, nil, err
		}

		if err = tx.Commit(r.Context()); err != nil {
			return 0, nil, err
		}

		return http.StatusCreated, map[string]any{
			"id":         linkID,
			"code":       code,
			"url":        fmt.Sprintf("/pay/%s", code),
			"title":      req.Title,
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		}, nil
	})
}

func (s *Server) handleListPaymentLinks(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	limit := 20
	offset := 0
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if q := r.URL.Query().Get("offset"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 0 {
			offset = v
		}
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT id::text, code, title, pricing_mode, COALESCE(amount::text, ''), currency, status, reusable, max_payments, expires_at, created_at
		FROM payment_links
		WHERE merchant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, reqAuth.MerchantID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list payment links", middleware.GetRequestID(r.Context()))
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, code, title, mode, amountRaw, currency, status string
		var reusable bool
		var maxPayments *int
		var expiresAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&id, &code, &title, &mode, &amountRaw, &currency, &status, &reusable, &maxPayments, &expiresAt, &createdAt); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan payment links", middleware.GetRequestID(r.Context()))
			return
		}
		items = append(items, map[string]any{
			"id":           id,
			"code":         code,
			"title":        title,
			"pricing_mode": mode,
			"amount":       parseFloatMaybe(amountRaw),
			"currency":     currency,
			"status":       status,
			"reusable":     reusable,
			"max_payments": maxPayments,
			"expires_at":   expiresAt,
			"created_at":   createdAt,
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

	query := `
		SELECT p.id::text, p.code, p.title, p.description, p.image_url, p.pricing_mode, COALESCE(p.amount::text, ''), p.currency,
			p.reusable, p.max_payments, p.expires_at, p.cta_text, p.after_payment_type, p.success_message, p.redirect_url,
			p.status, COALESCE(p.adjust_percent::text, ''), COALESCE(p.min_amount::text, ''), COALESCE(p.max_amount::text, ''),
			p.allowed_tokens, p.customer_fields, p.custom_fields, p.metadata,
			l.collect_email, l.collect_name, l.collect_phone, l.collect_address, l.collect_business_name,
			l.require_terms_acceptance, l.allow_promo_codes, l.collect_tax_automatically, l.add_invoice_pdf, l.metadata,
			p.created_at, p.updated_at
		FROM payment_links p
		LEFT JOIN link_options l ON l.payment_link_id=p.id
		WHERE p.merchant_id=$1 AND (p.id::text=$2 OR p.code=$2)
	`
	row := s.db.QueryRow(r.Context(), query, reqAuth.MerchantID, id)
	var resp map[string]any
	var linkID, code, title, mode, amountRaw, currency, cta, afterType, status, adjustRaw, minRaw, maxRaw string
	var description, imageURL, successMsg, redirectURL *string
	var reusable bool
	var maxPayments *int
	var expiresAt *time.Time
	var allowedRaw, customerRaw, customRaw, metaRaw []byte
	var collectEmail, collectName, collectPhone, collectAddress, collectBusinessName bool
	var requireTerms, allowPromo, collectTax, addInvoice bool
	var optsMetaRaw []byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&linkID, &code, &title, &description, &imageURL, &mode, &amountRaw, &currency,
		&reusable, &maxPayments, &expiresAt, &cta, &afterType, &successMsg, &redirectURL,
		&status, &adjustRaw, &minRaw, &maxRaw,
		&allowedRaw, &customerRaw, &customRaw, &metaRaw,
		&collectEmail, &collectName, &collectPhone, &collectAddress, &collectBusinessName,
		&requireTerms, &allowPromo, &collectTax, &addInvoice, &optsMetaRaw,
		&createdAt, &updatedAt,
	); err != nil {
		if strings.Contains(err.Error(), "no rows") {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "payment link not found", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan payment link", middleware.GetRequestID(r.Context()))
		return
	}
	resp = map[string]any{
		"id":           linkID,
		"code":         code,
		"title":        title,
		"description":  description,
		"image_url":    imageURL,
		"pricing_mode": mode,
		"amount":       parseFloatMaybe(amountRaw),
		"currency":     currency,
		"reusable":     reusable,
		"max_payments": maxPayments,
		"expires_at":   expiresAt,
		"cta_text":     cta,
		"after_payment": map[string]any{
			"type":            afterType,
			"success_message": successMsg,
			"redirect_url":    redirectURL,
		},
		"status":          status,
		"adjust_percent":  parseFloatMaybe(adjustRaw),
		"min_amount":      parseFloatMaybe(minRaw),
		"max_amount":      parseFloatMaybe(maxRaw),
		"allowed_tokens":  parseRawJSON(allowedRaw),
		"customer_fields": parseRawJSON(customerRaw),
		"custom_fields":   parseRawJSON(customRaw),
		"metadata":        parseRawJSON(metaRaw),
		"options": map[string]any{
			"collect_email":             collectEmail,
			"collect_name":              collectName,
			"collect_phone":             collectPhone,
			"collect_address":           collectAddress,
			"collect_business_name":     collectBusinessName,
			"require_terms_acceptance":  requireTerms,
			"allow_promo_codes":         allowPromo,
			"collect_tax_automatically": collectTax,
			"add_invoice_pdf":           addInvoice,
			"metadata":                  parseRawJSON(optsMetaRaw),
		},
		"created_at": createdAt,
		"updated_at": updatedAt,
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
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
	cmd, err := s.db.Exec(r.Context(), `
		UPDATE payment_links
		SET status='archived', archived_at=NOW(), updated_at=NOW()
		WHERE merchant_id=$1 AND (id::text=$2 OR code=$2) AND status!='archived'
	`, reqAuth.MerchantID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to archive payment link", middleware.GetRequestID(r.Context()))
		return
	}
	if cmd.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "payment link not found", middleware.GetRequestID(r.Context()))
		return
	}
	_ = s.enqueueEvent(r.Context(), "payment_link", id, reqAuth.MerchantID, "payment_link.archived", map[string]any{"payment_link": id})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "archived": true})
}

func newLinkCode() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i := range buf {
		out[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(out), nil
}

func parseFloatMaybe(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	return f
}

func parseRawJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}
