package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
)

const testJWTSecret = "test-secret-for-webhook-tests"

func issueTestToken(t *testing.T, userID, wallet string, role *string) string {
	t.Helper()
	svc := auth.NewService(testJWTSecret)
	tok, err := svc.IssueJWT(userID, wallet, role)
	if err != nil {
		t.Fatalf("issue test token: %v", err)
	}
	return tok
}

func merchantToken(t *testing.T) string {
	t.Helper()
	role := "merchant"
	return issueTestToken(t, "user-1", "0xWallet", &role)
}

func newTestRouter(h *handlers) chi.Router {
	authSvc := auth.NewService(testJWTSecret)
	r := chi.NewRouter()
	r.Use(authSvc.Middleware)
	r.Post("/webhooks", h.createWebhook)
	r.Get("/webhooks", h.listWebhooks)
	r.Put("/webhooks/{id}", h.updateWebhook)
	r.Delete("/webhooks/{id}", h.deleteWebhook)
	r.Get("/webhooks/{id}/deliveries", h.listWebhookDeliveries)
	r.Post("/withdrawals", h.recordWithdrawal)
	return r
}

// --- Scan helpers for mock rows ---

func shopScanFn(s db.Shop) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = s.ID
		*dest[1].(*string) = s.UserID
		*dest[2].(*string) = s.Name
		*dest[3].(**string) = s.Website
		*dest[4].(**string) = s.Title
		*dest[5].(**string) = s.AvatarUrl
		*dest[6].(*time.Time) = s.CreatedAt
		*dest[7].(*time.Time) = s.UpdatedAt
		return nil
	}
}

func webhookScanFn(w db.Webhook) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = w.ID
		*dest[1].(*string) = w.ShopID
		*dest[2].(*string) = w.Url
		*dest[3].(*string) = w.Secret
		*dest[4].(*[]string) = w.EventTypes
		*dest[5].(*bool) = w.Active
		*dest[6].(*time.Time) = w.CreatedAt
		*dest[7].(*time.Time) = w.UpdatedAt
		return nil
	}
}

func deliveryScanFn(d db.WebhookDelivery) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = d.ID
		*dest[1].(*string) = d.WebhookID
		*dest[2].(*string) = d.EventID
		*dest[3].(*string) = d.EventType
		*dest[4].(*[]byte) = d.Payload
		*dest[5].(*db.WebhookDeliveryStatus) = d.Status
		*dest[6].(*int32) = d.Attempts
		*dest[7].(**int32) = d.LastStatusCode
		*dest[8].(**string) = d.LastError
		*dest[9].(*time.Time) = d.CreatedAt
		*dest[10].(*time.Time) = d.UpdatedAt
		return nil
	}
}

func withdrawalScanFn(w db.Withdrawal) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = w.ID
		*dest[1].(*string) = w.ShopID
		*dest[2].(*string) = w.TokenAddress
		*dest[3].(*int64) = w.ChainID
		*dest[4].(*pgtype.Numeric) = w.Amount
		*dest[5].(*string) = w.TxHash
		*dest[6].(*time.Time) = w.CreatedAt
		return nil
	}
}

// --- Test fixtures ---

var testShop = db.Shop{
	ID:        "shop-1",
	UserID:    "user-1",
	Name:      "TestShop",
	CreatedAt: time.Now(),
	UpdatedAt: time.Now(),
}

var testWebhook = db.Webhook{
	ID:         "wh-1",
	ShopID:     "shop-1",
	Url:        "https://example.com/webhook",
	Secret:     "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
	EventTypes: []string{"payment.received", "product.created"},
	Active:     true,
	CreatedAt:  time.Now(),
	UpdatedAt:  time.Now(),
}

// mockWithShop returns a mockDBTX that handles GetShopByUserID.
func mockWithShop() *mockDBTX {
	m := newMockDBTX()
	m.queryRowFns["FROM shops WHERE user_id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: shopScanFn(testShop)}
	}
	return m
}

// --- Auth guard tests (no DB needed) ---

func TestCreateWebhook_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCreateWebhook_NotMerchant(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	userRole := "user"
	token := issueTestToken(t, "user-1", "0xWallet", &userRole)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "only merchants can manage webhooks" {
		t.Errorf("error = %q, want 'only merchants can manage webhooks'", resp["error"])
	}
}

func TestCreateWebhook_NoRole(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	token := issueTestToken(t, "user-1", "0xWallet", nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestListWebhooks_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestUpdateWebhook_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/webhooks/wh-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDeleteWebhook_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/webhooks/wh-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWebhookDeliveries_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/wh-1/deliveries", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRecordWithdrawal_Unauthorized(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/withdrawals", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCreateWebhook_InvalidToken(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCreateWebhook_WrongSecret(t *testing.T) {
	h := &handlers{queries: db.New(nil)}
	r := newTestRouter(h)

	differentSecret := auth.NewService("wrong-secret")
	merchantRole := "merchant"
	token, _ := differentSecret.IssueJWT("user-1", "0xWallet", &merchantRole)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// --- DB-dependent webhook CRUD tests ---

func TestCreateWebhook_NoShop(t *testing.T) {
	m := newMockDBTX()
	m.queryRowFns["FROM shops WHERE user_id"] = func(args ...interface{}) pgx.Row {
		return errRow(pgx.ErrNoRows)
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://example.com/hook","event_types":["payment.received"]}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "create a shop first" {
		t.Errorf("error = %q, want 'create a shop first'", resp["error"])
	}
}

func TestCreateWebhook_InvalidBody(t *testing.T) {
	m := mockWithShop()
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateWebhook_MissingFields(t *testing.T) {
	m := mockWithShop()
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	tests := []struct {
		name string
		body string
	}{
		{"missing url", `{"event_types":["payment.received"]}`},
		{"missing event_types", `{"url":"https://example.com/hook"}`},
		{"empty event_types", `{"url":"https://example.com/hook","event_types":[]}`},
		{"empty url", `{"url":"","event_types":["payment.received"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+merchantToken(t))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCreateWebhook_InvalidEventType(t *testing.T) {
	m := mockWithShop()
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://example.com/hook","event_types":["user.created"]}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "invalid event type") {
		t.Errorf("error = %q, want to contain 'invalid event type'", resp["error"])
	}
}

func TestCreateWebhook_Success(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["INSERT INTO webhooks"] = func(args ...interface{}) pgx.Row {
		wh := testWebhook
		wh.Url = args[1].(string)
		wh.Secret = args[2].(string)
		wh.EventTypes = args[3].([]string)
		return &mockRow{scanFn: webhookScanFn(wh)}
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://example.com/hook","event_types":["payment.received","product.created"]}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp struct {
		Webhook webhookResponse `json:"webhook"`
		Secret  string         `json:"secret"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Webhook.ID == "" {
		t.Error("webhook ID is empty")
	}
	if resp.Secret == "" {
		t.Error("secret is empty")
	}
	if len(resp.Secret) != 64 {
		t.Errorf("secret length = %d, want 64 hex chars", len(resp.Secret))
	}
	if resp.Webhook.URL != "https://example.com/hook" {
		t.Errorf("URL = %q, want https://example.com/hook", resp.Webhook.URL)
	}
}

func TestListWebhooks_Success(t *testing.T) {
	m := mockWithShop()
	m.queryFns["FROM webhooks\nWHERE shop_id"] = func(args ...interface{}) (pgx.Rows, error) {
		wh2 := testWebhook
		wh2.ID = "wh-2"
		wh2.EventTypes = []string{"withdrawal.completed"}
		return &mockRows{scanFns: []func(dest ...any) error{
			webhookScanFn(testWebhook),
			webhookScanFn(wh2),
		}}, nil
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []webhookResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("got %d webhooks, want 2", len(resp))
	}
	if resp[0].ID != "wh-1" {
		t.Errorf("resp[0].ID = %q, want wh-1", resp[0].ID)
	}
	if resp[1].ID != "wh-2" {
		t.Errorf("resp[1].ID = %q, want wh-2", resp[1].ID)
	}
}

func TestListWebhooks_Empty(t *testing.T) {
	m := mockWithShop()
	m.queryFns["FROM webhooks\nWHERE shop_id"] = func(args ...interface{}) (pgx.Rows, error) {
		return &mockRows{scanFns: nil}, nil
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []webhookResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("got %d webhooks, want 0", len(resp))
	}
}

func TestUpdateWebhook_Success(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: webhookScanFn(testWebhook)}
	}
	m.queryRowFns["UPDATE webhooks"] = func(args ...interface{}) pgx.Row {
		updated := testWebhook
		updated.Url = args[1].(string)
		updated.EventTypes = args[2].([]string)
		updated.Active = args[3].(bool)
		return &mockRow{scanFn: webhookScanFn(updated)}
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://new-url.com/hook","event_types":["withdrawal.completed"],"active":false}`
	req := httptest.NewRequest(http.MethodPut, "/webhooks/wh-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp webhookResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.URL != "https://new-url.com/hook" {
		t.Errorf("URL = %q, want https://new-url.com/hook", resp.URL)
	}
	if resp.Active {
		t.Error("Active = true, want false")
	}
}

func TestUpdateWebhook_NotFound(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return errRow(pgx.ErrNoRows)
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://example.com","event_types":["payment.received"],"active":true}`
	req := httptest.NewRequest(http.MethodPut, "/webhooks/nonexistent", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUpdateWebhook_WrongOwner(t *testing.T) {
	m := mockWithShop()
	foreignWebhook := testWebhook
	foreignWebhook.ShopID = "other-shop"
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: webhookScanFn(foreignWebhook)}
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"url":"https://example.com","event_types":["payment.received"],"active":true}`
	req := httptest.NewRequest(http.MethodPut, "/webhooks/wh-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDeleteWebhook_Success(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: webhookScanFn(testWebhook)}
	}
	m.execFns["DELETE FROM webhooks"] = func(args ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/webhooks/wh-1", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return errRow(pgx.ErrNoRows)
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/webhooks/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDeleteWebhook_WrongOwner(t *testing.T) {
	m := mockWithShop()
	foreignWebhook := testWebhook
	foreignWebhook.ShopID = "other-shop"
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: webhookScanFn(foreignWebhook)}
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/webhooks/wh-1", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestListWebhookDeliveries_Success(t *testing.T) {
	m := mockWithShop()
	m.queryRowFns["FROM webhooks WHERE id"] = func(args ...interface{}) pgx.Row {
		return &mockRow{scanFn: webhookScanFn(testWebhook)}
	}
	now := time.Now()
	code := int32(200)
	m.queryFns["FROM webhook_deliveries"] = func(args ...interface{}) (pgx.Rows, error) {
		d := db.WebhookDelivery{
			ID:             "del-1",
			WebhookID:      "wh-1",
			EventID:        "evt-abc",
			EventType:      "payment.received",
			Payload:        []byte(`{"id":"evt-abc"}`),
			Status:         db.WebhookDeliveryStatusSuccess,
			Attempts:       1,
			LastStatusCode: &code,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return &mockRows{scanFns: []func(dest ...any) error{deliveryScanFn(d)}}, nil
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/wh-1/deliveries", nil)
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []db.WebhookDelivery
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("got %d deliveries, want 1", len(resp))
	}
	if resp[0].EventID != "evt-abc" {
		t.Errorf("EventID = %q, want evt-abc", resp[0].EventID)
	}
}

// --- Withdrawal handler tests ---

func TestRecordWithdrawal_NoShop(t *testing.T) {
	m := newMockDBTX()
	m.queryRowFns["FROM shops WHERE user_id"] = func(args ...interface{}) pgx.Row {
		return errRow(pgx.ErrNoRows)
	}
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	body := `{"token_address":"0xToken","chain_id":137,"amount":"100.5","tx_hash":"0xabc"}`
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestRecordWithdrawal_MissingFields(t *testing.T) {
	m := mockWithShop()
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	tests := []struct {
		name string
		body string
	}{
		{"missing tx_hash", `{"token_address":"0xToken","chain_id":137,"amount":"100"}`},
		{"missing token_address", `{"chain_id":137,"amount":"100","tx_hash":"0xabc"}`},
		{"missing amount", `{"token_address":"0xToken","chain_id":137,"tx_hash":"0xabc"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/withdrawals", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+merchantToken(t))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestRecordWithdrawal_InvalidBody(t *testing.T) {
	m := mockWithShop()
	h := &handlers{queries: db.New(m)}
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/withdrawals", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRecordWithdrawal_Success(t *testing.T) {
	m := mockWithShop()
	now := time.Now()
	m.queryRowFns["INSERT INTO withdrawals"] = func(args ...interface{}) pgx.Row {
		wd := db.Withdrawal{
			ID:           "wd-1",
			ShopID:       "shop-1",
			TokenAddress: args[1].(string),
			ChainID:      args[2].(int64),
			Amount:       args[3].(pgtype.Numeric),
			TxHash:       args[4].(string),
			CreatedAt:    now,
		}
		return &mockRow{scanFn: withdrawalScanFn(wd)}
	}
	h := &handlers{queries: db.New(m)} // eventPub is nil — handler skips publish
	r := newTestRouter(h)

	body := `{"token_address":"0xToken","chain_id":137,"amount":"100.5","tx_hash":"0xabc123"}`
	req := httptest.NewRequest(http.MethodPost, "/withdrawals", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+merchantToken(t))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] != "wd-1" {
		t.Errorf("id = %q, want wd-1", resp["id"])
	}
	if resp["tx_hash"] != "0xabc123" {
		t.Errorf("tx_hash = %q, want 0xabc123", resp["tx_hash"])
	}
}

// --- Allowed event types ---

func TestAllowedWebhookEvents(t *testing.T) {
	expected := []string{
		"product.created",
		"payment.received",
		"subscription.created",
		"subscription.payment",
		"withdrawal.completed",
	}

	for _, et := range expected {
		if !allowedWebhookEvents[et] {
			t.Errorf("event type %q not found in allowedWebhookEvents", et)
		}
	}

	if len(allowedWebhookEvents) != len(expected) {
		t.Errorf("allowedWebhookEvents has %d entries, want %d", len(allowedWebhookEvents), len(expected))
	}
}

func TestAllowedWebhookEvents_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"invalid",
		"product.updated",
		"product.deleted",
		"payment.failed",
		"user.created",
		"shop.created",
		"subscriber.cancelled",
	}

	for _, et := range invalid {
		t.Run(et, func(t *testing.T) {
			if allowedWebhookEvents[et] {
				t.Errorf("event type %q should not be in allowedWebhookEvents", et)
			}
		})
	}
}

// --- Response format ---

func TestWebhookResponseFormat(t *testing.T) {
	now := time.Now()
	wh := db.Webhook{
		ID:         "wh-123",
		ShopID:     "shop-456",
		Url:        "https://example.com/webhook",
		Secret:     "should-not-appear-in-response",
		EventTypes: []string{"payment.received", "product.created"},
		Active:     true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	resp := toWebhookResponse(wh)

	if resp.ID != "wh-123" {
		t.Errorf("ID = %q, want wh-123", resp.ID)
	}
	if resp.URL != "https://example.com/webhook" {
		t.Errorf("URL = %q, want https://example.com/webhook", resp.URL)
	}
	if len(resp.EventTypes) != 2 {
		t.Errorf("EventTypes length = %d, want 2", len(resp.EventTypes))
	}
	if !resp.Active {
		t.Error("Active = false, want true")
	}

	// Verify secret is NOT in the JSON response
	jsonBytes, _ := json.Marshal(resp)
	var raw map[string]interface{}
	json.Unmarshal(jsonBytes, &raw)
	if _, hasSecret := raw["secret"]; hasSecret {
		t.Error("webhook response should not include secret field")
	}
}

