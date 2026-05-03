package gateway

import (
	"net/http"
	"strings"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/httpx"
	"github.com/cpay-dev/cpay/internal/shared/middleware"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-chi/chi/v5"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type updateMerchantSettingsRequest struct {
	SettlementAddress string `json:"settlement_address"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.Login(s.rpcContext(r.Context()), &cpayv1.LoginRequest{Email: req.Email, Password: req.Password})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  resp.GetTokens().GetAccessToken(),
		"refresh_token": resp.GetTokens().GetRefreshToken(),
		"token_type":    resp.GetTokens().GetTokenType(),
		"expires_in":    resp.GetTokens().GetExpiresIn(),
		"user": map[string]any{
			"id":          resp.GetUser().GetId(),
			"merchant_id": resp.GetUser().GetMerchantId(),
			"role":        resp.GetUser().GetRole(),
		},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.Refresh(s.rpcContext(r.Context()), &cpayv1.RefreshRequest{RefreshToken: req.RefreshToken})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  resp.GetTokens().GetAccessToken(),
		"refresh_token": resp.GetTokens().GetRefreshToken(),
		"token_type":    resp.GetTokens().GetTokenType(),
		"expires_in":    resp.GetTokens().GetExpiresIn(),
	})
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req createAPIKeyRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required", middleware.GetRequestID(r.Context()))
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"*"}
	}

	s.withIdempotency(w, r, reqAuth.MerchantID, "/v1/api_keys", func() (int, any, error) {
		resp, err := s.authClient.CreateApiKey(s.rpcContext(r.Context()), &cpayv1.CreateApiKeyRequest{
			MerchantId: reqAuth.MerchantID,
			Name:       req.Name,
			Scopes:     req.Scopes,
		})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, map[string]any{
			"id":         resp.GetApiKey().GetId(),
			"name":       resp.GetApiKey().GetName(),
			"prefix":     resp.GetApiKey().GetPrefix(),
			"key":        resp.GetPlainKey(),
			"scopes":     resp.GetApiKey().GetScopes(),
			"created_at": resp.GetApiKey().GetCreatedAt(),
		}, nil
	})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.ListApiKeys(s.rpcContext(r.Context()), &cpayv1.ListApiKeysRequest{MerchantId: reqAuth.MerchantID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(resp.GetData()))
	for _, item := range resp.GetData() {
		items = append(items, map[string]any{
			"id":           item.GetId(),
			"name":         item.GetName(),
			"prefix":       item.GetPrefix(),
			"scopes":       item.GetScopes(),
			"revoked_at":   emptyToNil(item.GetRevokedAt()),
			"last_used_at": emptyToNil(item.GetLastUsedAt()),
			"created_at":   item.GetCreatedAt(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "id is required", middleware.GetRequestID(r.Context()))
		return
	}
	resp, err := s.authClient.RevokeApiKey(s.rpcContext(r.Context()), &cpayv1.RevokeApiKeyRequest{MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "revoked": resp.GetRevoked()})
}

func (s *Server) handleGetMerchantSettings(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.GetMerchantSettings(s.rpcContext(r.Context()), &cpayv1.GetMerchantSettingsRequest{MerchantId: reqAuth.MerchantID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	settings := resp.GetSettings()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"merchant_id":        settings.GetMerchantId(),
		"settlement_address": emptyToNil(settings.GetSettlementAddress()),
	})
}

func (s *Server) handleUpdateMerchantSettings(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req updateMerchantSettingsRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	req.SettlementAddress = strings.TrimSpace(req.SettlementAddress)
	if !common.IsHexAddress(req.SettlementAddress) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "settlement_address is invalid", middleware.GetRequestID(r.Context()))
		return
	}
	checksumAddress := common.HexToAddress(req.SettlementAddress).Hex()

	resp, err := s.authClient.UpdateMerchantSettings(s.rpcContext(r.Context()), &cpayv1.UpdateMerchantSettingsRequest{
		MerchantId:        reqAuth.MerchantID,
		SettlementAddress: checksumAddress,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	settings := resp.GetSettings()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"merchant_id":        settings.GetMerchantId(),
		"settlement_address": settings.GetSettlementAddress(),
	})
}

func emptyToNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
