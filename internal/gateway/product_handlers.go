package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
)

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

		productID := uuid.New()
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

		payload, found, err := s.fetchProduct(r.Context(), reqAuth.MerchantID, productID.String())
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
		SELECT id::text, name, description, image_url, default_currency, COALESCE(default_amount::text, ''), metadata, created_at, updated_at
		FROM catalog.products
		WHERE merchant_id=$1
		ORDER BY created_at DESC
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

func (s *Server) fetchProduct(ctx context.Context, merchantID uuid.UUID, id string) (map[string]any, bool, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, name, description, image_url, default_currency, COALESCE(default_amount::text, ''), metadata, created_at, updated_at
		FROM catalog.products
		WHERE merchant_id=$1 AND id::text=$2
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

func scanProductRow(scanner interface {
	Scan(dest ...any) error
}) (map[string]any, error) {
	var productID, name, currency, amountRaw string
	var description, imageURL *string
	var metadataRaw []byte
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&productID, &name, &description, &imageURL, &currency, &amountRaw, &metadataRaw, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	return map[string]any{
		"id":               productID,
		"name":             name,
		"description":      strPtrToAny(description),
		"image_url":        strPtrToAny(imageURL),
		"default_currency": currency,
		"default_amount":   parseAmount(amountRaw),
		"metadata":         parseJSONValue(string(metadataRaw), map[string]any{}),
		"created_at":       createdAt.UTC().Format(time.RFC3339Nano),
		"updated_at":       updatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
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
