package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
)

// parsePriceToNumeric converts a JSON number/string price to pgtype.Numeric.
// Supports up to 18 decimal places.
func parsePriceToNumeric(val interface{}) (pgtype.Numeric, error) {
	var s string
	switch v := val.(type) {
	case float64:
		s = fmt.Sprintf("%.18f", v)
		// Trim trailing zeros but keep at least one decimal
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	case string:
		s = v
	case json.Number:
		s = v.String()
	default:
		return pgtype.Numeric{}, fmt.Errorf("unsupported price type: %T", val)
	}

	if s == "" {
		return pgtype.Numeric{}, fmt.Errorf("empty price")
	}

	// Parse via big.Float for full precision
	bf, _, err := big.ParseFloat(s, 10, 128, big.ToNearestEven)
	if err != nil {
		return pgtype.Numeric{}, fmt.Errorf("invalid price: %w", err)
	}
	if bf.Sign() <= 0 {
		return pgtype.Numeric{}, fmt.Errorf("price must be positive")
	}

	// Convert to integer * 10^-exp form
	// Determine scale from string representation
	scale := int32(0)
	if dotIdx := strings.IndexByte(s, '.'); dotIdx >= 0 {
		scale = int32(len(s) - dotIdx - 1)
	}
	// Remove the dot and parse as big.Int
	intStr := strings.Replace(s, ".", "", 1)
	intStr = strings.TrimLeft(intStr, "0")
	if intStr == "" {
		return pgtype.Numeric{}, fmt.Errorf("price must be positive")
	}
	intVal := new(big.Int)
	if _, ok := intVal.SetString(intStr, 10); !ok {
		return pgtype.Numeric{}, fmt.Errorf("invalid price integer: %s", intStr)
	}

	return pgtype.Numeric{
		Int:   intVal,
		Exp:   -scale,
		Valid: true,
	}, nil
}

// numericToString converts pgtype.Numeric to a human-readable decimal string.
func numericToString(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}

	// n.Int * 10^n.Exp
	intStr := n.Int.String()
	if n.Exp >= 0 {
		// No decimal places needed, just add zeros
		return intStr + strings.Repeat("0", int(n.Exp))
	}

	decPlaces := int(-n.Exp)
	negative := false
	if intStr[0] == '-' {
		negative = true
		intStr = intStr[1:]
	}

	// Pad with leading zeros if needed
	for len(intStr) <= decPlaces {
		intStr = "0" + intStr
	}

	whole := intStr[:len(intStr)-decPlaces]
	frac := intStr[len(intStr)-decPlaces:]
	// Trim trailing zeros from fraction
	frac = strings.TrimRight(frac, "0")

	result := whole
	if frac != "" {
		result = whole + "." + frac
	}
	if negative {
		result = "-" + result
	}
	return result
}

// productResponse is the JSON-friendly version of db.Product
type productResponse struct {
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

type publicProductResponse struct {
	productResponse
	MerchantAddress *string `json:"merchant_address"`
}

func toProductResponse(p db.Product) productResponse {
	return productResponse{
		ID:           p.ID,
		ShopID:       p.ShopID,
		Name:         p.Name,
		Description:  p.Description,
		Price:        numericToString(p.Price),
		Currency:     p.Currency,
		TokenAddress: p.TokenAddress,
		ChainID:      p.ChainID,
		ImageURL:     p.ImageUrl,
		Active:       p.Active,
		CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toProductResponseList(products []db.Product) []productResponse {
	result := make([]productResponse, len(products))
	for i, p := range products {
		result[i] = toProductResponse(p)
	}
	return result
}

func (h *handlers) createProduct(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can create products")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create a shop first")
		return
	}

	var req struct {
		Name         string      `json:"name"`
		Description  *string     `json:"description"`
		Price        interface{} `json:"price"`
		Currency     string      `json:"currency"`
		TokenAddress string      `json:"token_address"`
		ChainID      int32       `json:"chain_id"`
		ImageURL     *string     `json:"image_url"`
	}

	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Price == nil {
		writeError(w, http.StatusBadRequest, "price is required")
		return
	}

	price, err := parsePriceToNumeric(req.Price)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid price: %v", err))
		return
	}

	// Defaults
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.TokenAddress == "" {
		req.TokenAddress = "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359" // USDC on Polygon
	}
	if req.ChainID == 0 {
		req.ChainID = 137
	}

	product, err := h.queries.CreateProduct(r.Context(), db.CreateProductParams{
		ShopID:       shop.ID,
		Name:         req.Name,
		Description:  req.Description,
		Price:        price,
		Currency:     req.Currency,
		TokenAddress: req.TokenAddress,
		ChainID:      req.ChainID,
		ImageUrl:     req.ImageURL,
	})
	if err != nil {
		slog.Error("failed to create product", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create product")
		return
	}

	evt, err := events.NewEvent(events.EventProductCreated, events.SubjectProducts, map[string]string{
		"product_id": product.ID,
		"shop_id":    shop.ID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			slog.Error("failed to publish product created event", "error", pubErr)
		}
	}

	writeJSON(w, http.StatusCreated, toProductResponse(product))
}

func (h *handlers) listMyProducts(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "shop not found")
		return
	}

	products, err := h.queries.ListShopProducts(r.Context(), shop.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list products")
		return
	}

	writeJSON(w, http.StatusOK, toProductResponseList(products))
}

func (h *handlers) getProduct(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")

	product, err := h.queries.GetProductByID(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, toProductResponse(product))
}

func (h *handlers) getPublicProduct(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "id")

	product, err := h.queries.GetPublicProduct(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	resp := publicProductResponse{
		productResponse: productResponse{
			ID:           product.ID,
			ShopID:       product.ShopID,
			Name:         product.Name,
			Description:  product.Description,
			Price:        numericToString(product.Price),
			Currency:     product.Currency,
			TokenAddress: product.TokenAddress,
			ChainID:      product.ChainID,
			ImageURL:     product.ImageUrl,
			Active:       product.Active,
			CreatedAt:    product.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    product.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
		MerchantAddress: product.MerchantAddress,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) updateProduct(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "id")

	// Verify ownership: user → shop → product
	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetProductByID(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var req struct {
		Name         string      `json:"name"`
		Description  *string     `json:"description"`
		Price        interface{} `json:"price"`
		Currency     string      `json:"currency"`
		TokenAddress string      `json:"token_address"`
		ChainID      int32       `json:"chain_id"`
		ImageURL     *string     `json:"image_url"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Price == nil {
		writeError(w, http.StatusBadRequest, "price is required")
		return
	}

	price, err := parsePriceToNumeric(req.Price)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid price: %v", err))
		return
	}

	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.TokenAddress == "" {
		req.TokenAddress = existing.TokenAddress
	}
	if req.ChainID == 0 {
		req.ChainID = existing.ChainID
	}

	updated, err := h.queries.UpdateProduct(r.Context(), db.UpdateProductParams{
		ID:           productID,
		Name:         req.Name,
		Description:  req.Description,
		Price:        price,
		Currency:     req.Currency,
		TokenAddress: req.TokenAddress,
		ChainID:      req.ChainID,
		ImageUrl:     req.ImageURL,
	})
	if err != nil {
		slog.Error("failed to update product", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	evt, err := events.NewEvent(events.EventProductUpdated, events.SubjectProducts, map[string]string{
		"product_id": updated.ID,
		"shop_id":    shop.ID,
	})
	if err == nil {
		if pubErr := h.eventPub.Publish(r.Context(), evt); pubErr != nil {
			slog.Error("failed to publish product updated event", "error", pubErr)
		}
	}

	writeJSON(w, http.StatusOK, toProductResponse(updated))
}

func (h *handlers) toggleProductActive(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "id")

	// Verify ownership
	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetProductByID(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.queries.SetProductActive(r.Context(), db.SetProductActiveParams{
		ID:     productID,
		Active: req.Active,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"active": req.Active})
}

func (h *handlers) deleteProduct(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	productID := chi.URLParam(r, "id")

	// Verify ownership
	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.queries.GetProductByID(r.Context(), productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if existing.ShopID != shop.ID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.queries.DeleteProduct(r.Context(), productID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

const maxProductImageSize = 5 << 20 // 5MB

func (h *handlers) uploadProductImage(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	role := auth.RoleFromContext(r.Context())

	if role != "merchant" {
		writeError(w, http.StatusForbidden, "only merchants can upload product images")
		return
	}

	shop, err := h.queries.GetShopByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "create a shop first")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxProductImageSize)
	if err := r.ParseMultipartForm(maxProductImageSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 5MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !allowedContentTypes[contentType] {
		writeError(w, http.StatusBadRequest, "invalid file type (allowed: png, jpeg, webp)")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		switch contentType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		}
	}
	filename := fmt.Sprintf("product-%s%s", strings.ToLower(strings.Replace(r.FormValue("product_id"), " ", "", -1)), strings.ToLower(ext))
	if r.FormValue("product_id") == "" {
		filename = fmt.Sprintf("product%s", strings.ToLower(ext))
	}

	url, err := h.storage.UploadProductImage(r.Context(), shop.ID, filename, file, header.Size, contentType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload file")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
