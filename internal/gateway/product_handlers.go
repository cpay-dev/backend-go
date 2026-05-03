package gateway

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
)

const maxProductImageBytes = 5 << 20

type createProductRequest struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	ImageURL        string         `json:"image_url,omitempty"`
	DefaultCurrency string         `json:"default_currency,omitempty"`
	DefaultAmount   *float64       `json:"default_amount,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type updateProductRequest struct {
	Name            *string         `json:"name,omitempty"`
	Description     *string         `json:"description,omitempty"`
	ImageURL        *string         `json:"image_url,omitempty"`
	DefaultCurrency *string         `json:"default_currency,omitempty"`
	DefaultAmount   *float64        `json:"default_amount,omitempty"`
	Metadata        *map[string]any `json:"metadata,omitempty"`
}

func (s *Server) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}

	var req createProductRequest
	if !s.parseJSON(w, r, &req) {
		return
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/products", func() (int, any, error) {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", "name is required")
		}
		if req.DefaultAmount != nil && *req.DefaultAmount < 0 {
			return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", "default_amount must be non-negative")
		}

		currency, err := normalizeCurrency(req.DefaultCurrency, "USD")
		if err != nil {
			return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", err.Error())
		}

		productID := ids.New()
		description := nullableText(req.Description)
		imageURL := nullableText(req.ImageURL)
		metadataJSON := mustJSON(req.Metadata, "{}")

		var defaultAmount any
		if req.DefaultAmount != nil {
			defaultAmount = *req.DefaultAmount
		}

		_, err = s.db.Exec(r.Context(), `
			INSERT INTO catalog.products(
				id, merchant_id, name, description, image_url, default_currency, default_amount, metadata, created_at, updated_at
			)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW(), NOW())
		`, productID, reqAuth.MerchantID, name, description, imageURL, currency, defaultAmount, metadataJSON)
		if err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to create product")
		}
		if err := s.createDefaultPaymentLinkForProduct(r.Context(), reqAuth.MerchantID, productID, name, description, imageURL, currency, defaultAmount, metadataJSON); err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to create product payment link")
		}

		payload, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, productID)
		if err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch created product")
		}
		if !found {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "created product not found")
		}
		return http.StatusCreated, payload, nil
	})
}

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
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
		SELECT p.id::text, p.name, p.description, p.image_url, p.default_currency, COALESCE(p.default_amount::text, ''), p.metadata, p.created_at, p.updated_at,
			active_link.code
		FROM catalog.products p
		LEFT JOIN LATERAL (
			SELECT code
			FROM catalog.payment_links pl
			WHERE pl.product_id=p.id AND pl.status='active' AND (pl.expires_at IS NULL OR pl.expires_at > NOW())
			ORDER BY pl.created_at DESC
			LIMIT 1
		) active_link ON TRUE
		WHERE p.merchant_id=$1
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`, reqAuth.MerchantID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list products", middleware.GetRequestID(r.Context()))
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		product, scanErr := scanProductRow(rows)
		if scanErr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to scan products", middleware.GetRequestID(r.Context()))
			return
		}
		items = append(items, product)
	}
	if rows.Err() != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to read products", middleware.GetRequestID(r.Context()))
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	product, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get product", middleware.GetRequestID(r.Context()))
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "product not found", middleware.GetRequestID(r.Context()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, product)
}

func (s *Server) handleGetPublicProduct(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	product, found, err := s.fetchPublicProduct(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get product", middleware.GetRequestID(r.Context()))
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "product not found", middleware.GetRequestID(r.Context()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, product)
}

func (s *Server) handleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	var req updateProductRequest
	if !s.parseJSON(w, r, &req) {
		return
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/products/:id", func() (int, any, error) {
		setParts := make([]string, 0, 6)
		args := []any{reqAuth.MerchantID, id}

		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", "name cannot be empty")
			}
			setParts = append(setParts, "name=$"+strconv.Itoa(len(args)+1))
			args = append(args, name)
		}

		if req.Description != nil {
			setParts = append(setParts, "description=$"+strconv.Itoa(len(args)+1))
			args = append(args, nullableText(*req.Description))
		}

		if req.ImageURL != nil {
			setParts = append(setParts, "image_url=$"+strconv.Itoa(len(args)+1))
			args = append(args, nullableText(*req.ImageURL))
		}

		if req.DefaultCurrency != nil {
			currency, err := normalizeCurrency(*req.DefaultCurrency, "")
			if err != nil {
				return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", err.Error())
			}
			setParts = append(setParts, "default_currency=$"+strconv.Itoa(len(args)+1))
			args = append(args, currency)
		}

		if req.DefaultAmount != nil {
			if *req.DefaultAmount < 0 {
				return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", "default_amount must be non-negative")
			}
			setParts = append(setParts, "default_amount=$"+strconv.Itoa(len(args)+1))
			args = append(args, *req.DefaultAmount)
		}

		if req.Metadata != nil {
			setParts = append(setParts, "metadata=$"+strconv.Itoa(len(args)+1)+"::jsonb")
			args = append(args, mustJSON(*req.Metadata, "{}"))
		}

		if len(setParts) == 0 {
			return 0, nil, rpcx.E(codes.InvalidArgument, "invalid_request", "at least one field must be provided")
		}

		setParts = append(setParts, "updated_at=NOW()")
		query := `
			UPDATE catalog.products
			SET ` + strings.Join(setParts, ", ") + `
			WHERE merchant_id=$1 AND id::text=$2
		`
		tag, err := s.db.Exec(r.Context(), query, args...)
		if err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to update product")
		}
		if tag.RowsAffected() == 0 {
			return 0, nil, rpcx.E(codes.NotFound, "not_found", "product not found")
		}

		product, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, id)
		if err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to fetch updated product")
		}
		if !found {
			return 0, nil, rpcx.E(codes.NotFound, "not_found", "product not found")
		}
		return http.StatusOK, product, nil
	})
}

func (s *Server) handleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/products/:id/delete", func() (int, any, error) {
		tag, err := s.db.Exec(r.Context(), `
			DELETE FROM catalog.products
			WHERE merchant_id=$1 AND id::text=$2
		`, reqAuth.MerchantID, id)
		if err != nil {
			return 0, nil, rpcx.E(codes.Internal, "internal_error", "failed to delete product")
		}
		if tag.RowsAffected() == 0 {
			return 0, nil, rpcx.E(codes.NotFound, "not_found", "product not found")
		}
		return http.StatusOK, map[string]any{"id": id, "deleted": true}, nil
	})
}

func (s *Server) handleUploadProductImage(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := mustRequester(w, r)
	if !ok {
		return
	}
	if s.mediaStore == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "media_unavailable", "media upload is not configured", middleware.GetRequestID(r.Context()))
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}

	_, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get product", middleware.GetRequestID(r.Context()))
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "product not found", middleware.GetRequestID(r.Context()))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxProductImageBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "invalid_request", "image must be 5MB or smaller", middleware.GetRequestID(r.Context()))
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "multipart field \"file\" is required", middleware.GetRequestID(r.Context()))
		return
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, maxProductImageBytes+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "failed to read image", middleware.GetRequestID(r.Context()))
		return
	}
	if len(payload) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "image cannot be empty", middleware.GetRequestID(r.Context()))
		return
	}
	if len(payload) > maxProductImageBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "invalid_request", "image must be 5MB or smaller", middleware.GetRequestID(r.Context()))
		return
	}

	contentType, ext, ok := productImageContentType(payload, header.Header.Get("Content-Type"))
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "file must be a JPEG, PNG, WebP, or GIF image", middleware.GetRequestID(r.Context()))
		return
	}

	imageName := ids.New() + ext
	objectKey := fmt.Sprintf("product-images/%s/%s/%s", reqAuth.MerchantID, id, imageName)
	if err := s.mediaStore.PutObjectBytes(r.Context(), objectKey, contentType, payload); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to store image", middleware.GetRequestID(r.Context()))
		return
	}

	imageURL := productImageURL(r, reqAuth.MerchantID, id, imageName)
	tag, err := s.db.Exec(r.Context(), `
		UPDATE catalog.products
		SET image_url=$3, updated_at=NOW()
		WHERE merchant_id=$1 AND id::text=$2
	`, reqAuth.MerchantID, id, imageURL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update product image", middleware.GetRequestID(r.Context()))
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "product not found", middleware.GetRequestID(r.Context()))
		return
	}

	product, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to fetch updated product", middleware.GetRequestID(r.Context()))
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "product not found", middleware.GetRequestID(r.Context()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"image_url": imageURL,
		"product":   product,
	})
}

func (s *Server) handleGetProductImage(w http.ResponseWriter, r *http.Request) {
	if s.mediaStore == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "media_unavailable", "media storage is not configured", middleware.GetRequestID(r.Context()))
		return
	}

	merchantID := strings.TrimSpace(chi.URLParam(r, "merchant_id"))
	productID := strings.TrimSpace(chi.URLParam(r, "product_id"))
	imageID := strings.TrimSpace(chi.URLParam(r, "image_id"))
	if merchantID == "" || productID == "" || imageID == "" || strings.Contains(imageID, "/") {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid image path", middleware.GetRequestID(r.Context()))
		return
	}

	objectKey := fmt.Sprintf("product-images/%s/%s/%s", merchantID, productID, imageID)
	payload, contentType, err := s.mediaStore.GetObjectBytes(r.Context(), objectKey)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "image not found", middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) fetchProduct(ctx context.Context, merchantID string, id string) (map[string]any, bool, error) {
	row := s.db.QueryRow(ctx, `
		SELECT p.id::text, p.name, p.description, p.image_url, p.default_currency, COALESCE(p.default_amount::text, ''), p.metadata, p.created_at, p.updated_at,
			active_link.code
		FROM catalog.products p
		LEFT JOIN LATERAL (
			SELECT code
			FROM catalog.payment_links pl
			WHERE pl.product_id=p.id AND pl.status='active' AND (pl.expires_at IS NULL OR pl.expires_at > NOW())
			ORDER BY pl.created_at DESC
			LIMIT 1
		) active_link ON TRUE
		WHERE p.merchant_id=$1 AND p.id::text=$2
	`, merchantID, id)

	product, err := scanProductRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return product, true, nil
}

func (s *Server) fetchPublicProduct(ctx context.Context, id string) (map[string]any, bool, error) {
	row := s.db.QueryRow(ctx, `
		SELECT p.id::text, p.name, p.description, p.image_url, p.default_currency, COALESCE(p.default_amount::text, ''), p.metadata, p.created_at, p.updated_at,
			active_link.code
		FROM catalog.products p
		LEFT JOIN LATERAL (
			SELECT code
			FROM catalog.payment_links pl
			WHERE pl.product_id=p.id AND pl.status='active' AND (pl.expires_at IS NULL OR pl.expires_at > NOW())
			ORDER BY pl.created_at DESC
			LIMIT 1
		) active_link ON TRUE
		WHERE p.id::text=$1
	`, id)

	product, err := scanProductRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return product, true, nil
}

func scanProductRow(scanner interface {
	Scan(dest ...any) error
}) (map[string]any, error) {
	var productID, name, currency, amountRaw string
	var description, imageURL, activePaymentLinkCode *string
	var metadataRaw []byte
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&productID, &name, &description, &imageURL, &currency, &amountRaw, &metadataRaw, &createdAt, &updatedAt, &activePaymentLinkCode); err != nil {
		return nil, err
	}

	return map[string]any{
		"id":                       productID,
		"name":                     name,
		"description":              strPtrToAny(description),
		"image_url":                strPtrToAny(imageURL),
		"default_currency":         currency,
		"default_amount":           parseAmount(amountRaw),
		"metadata":                 parseJSONValue(string(metadataRaw), map[string]any{}),
		"created_at":               createdAt.UTC().Format(time.RFC3339Nano),
		"updated_at":               updatedAt.UTC().Format(time.RFC3339Nano),
		"active_payment_link_code": strPtrToAny(activePaymentLinkCode),
	}, nil
}

func (s *Server) createDefaultPaymentLinkForProduct(
	ctx context.Context,
	merchantID string,
	productID string,
	title string,
	description any,
	imageURL any,
	currency string,
	amount any,
	metadataJSON string,
) error {
	code, err := newProductPaymentLinkCode()
	if err != nil {
		return err
	}
	pricingMode := "fixed"
	if amount == nil {
		pricingMode = "open"
	}
	linkID := ids.New()
	_, err = s.db.Exec(ctx, `
		INSERT INTO catalog.payment_links(
			id, merchant_id, product_id, code, title, description, image_url, pricing_mode, amount, currency,
			reusable, cta_text, after_payment_type, status, allowed_tokens, customer_fields, custom_fields, metadata,
			created_at, updated_at
		)
		VALUES(
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			TRUE, 'Pay', 'confirmation_page', 'active', '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, $11::jsonb,
			NOW(), NOW()
		)
	`, linkID, merchantID, productID, code, title, description, imageURL, pricingMode, amount, currency, metadataJSON)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO catalog.link_options(payment_link_id, collect_email, metadata)
		VALUES($1, TRUE, '{}'::jsonb)
	`, linkID)
	return err
}

func newProductPaymentLinkCode() (string, error) {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
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

func normalizeCurrency(raw string, def string) (string, error) {
	cur := strings.ToUpper(strings.TrimSpace(raw))
	if cur == "" {
		cur = strings.ToUpper(strings.TrimSpace(def))
	}
	if cur == "" {
		return "", fmt.Errorf("default_currency is required")
	}
	if len(cur) != 3 {
		return "", fmt.Errorf("default_currency must be a 3-letter code")
	}
	for _, ch := range cur {
		if ch < 'A' || ch > 'Z' {
			return "", fmt.Errorf("default_currency must contain letters only")
		}
	}
	return cur, nil
}

func nullableText(v string) any {
	t := strings.TrimSpace(v)
	if t == "" {
		return nil
	}
	return t
}

func strPtrToAny(v *string) any {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return t
}

func parseAmount(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return v
}

func productImageContentType(payload []byte, _ string) (string, string, bool) {
	if len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff {
		return "image/jpeg", ".jpg", true
	}
	if len(payload) >= 8 &&
		payload[0] == 0x89 && payload[1] == 'P' && payload[2] == 'N' && payload[3] == 'G' &&
		payload[4] == '\r' && payload[5] == '\n' && payload[6] == 0x1a && payload[7] == '\n' {
		return "image/png", ".png", true
	}
	if len(payload) >= 6 && (string(payload[:6]) == "GIF87a" || string(payload[:6]) == "GIF89a") {
		return "image/gif", ".gif", true
	}
	if len(payload) >= 12 && string(payload[:4]) == "RIFF" && string(payload[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}
	return "", "", false
}

func productImageURL(r *http.Request, merchantID, productID, imageName string) string {
	path := fmt.Sprintf("/v1/public/product_images/%s/%s/%s", merchantID, productID, imageName)
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return path
	}
	return proto + "://" + host + path
}
