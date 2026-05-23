package gateway

import (
	"encoding/json"
	"io"
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

type signupRequest struct {
	MerchantName    string `json:"merchant_name"`
	ShopName        string `json:"shop_name"`
	ShopURL         string `json:"shop_url"`
	ShopDescription string `json:"shop_description"`
	ContactName     string `json:"contact_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	OnboardingToken string `json:"onboarding_token"`
}

type googleStartRequest struct {
	RedirectURI string `json:"redirect_uri"`
}

type googleConsumeRequest struct {
	Code        string `json:"code"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

type walletChallengeRequest struct {
	Address string `json:"address"`
	ChainID string `json:"chain_id"`
}

type walletVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Address     string `json:"address"`
	ChainID     string `json:"chain_id"`
	Message     string `json:"message"`
	Signature   string `json:"signature"`
}

type passkeyOptionsRequest struct {
	Email string `json:"email"`
}

type passkeyVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
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
			"email":       resp.GetUser().GetEmail(),
		},
	})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.Signup(s.rpcContext(r.Context()), &cpayv1.SignupRequest{
		MerchantName:    req.MerchantName,
		ShopName:        req.ShopName,
		ShopUrl:         req.ShopURL,
		ShopDescription: req.ShopDescription,
		ContactName:     req.ContactName,
		Email:           req.Email,
		Password:        req.Password,
		OnboardingToken: req.OnboardingToken,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeAuthExchange(w, http.StatusCreated, resp)
}

func (s *Server) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	var req googleStartRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.GoogleStart(s.rpcContext(r.Context()), &cpayv1.GoogleStartRequest{RedirectUri: req.RedirectURI})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"authorization_url": resp.GetAuthorizationUrl(), "state": resp.GetState()})
}

func (s *Server) handleGoogleConsume(w http.ResponseWriter, r *http.Request) {
	var req googleConsumeRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.GoogleConsume(s.rpcContext(r.Context()), &cpayv1.GoogleConsumeRequest{
		Code:        req.Code,
		State:       req.State,
		RedirectUri: req.RedirectURI,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeAuthExchange(w, http.StatusOK, resp)
}

func (s *Server) handleWalletChallenge(w http.ResponseWriter, r *http.Request) {
	var req walletChallengeRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.WalletChallenge(s.rpcContext(r.Context()), &cpayv1.WalletChallengeRequest{Address: req.Address, ChainId: req.ChainID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"challenge_id": resp.GetChallengeId(), "message": resp.GetMessage()})
}

func (s *Server) handleWalletVerify(w http.ResponseWriter, r *http.Request) {
	var req walletVerifyRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.WalletVerify(s.rpcContext(r.Context()), &cpayv1.WalletVerifyRequest{
		ChallengeId: req.ChallengeID,
		Address:     req.Address,
		ChainId:     req.ChainID,
		Message:     req.Message,
		Signature:   req.Signature,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeAuthExchange(w, http.StatusOK, resp)
}

func (s *Server) handleBeginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.BeginPasskeyRegistration(s.rpcContext(r.Context()), &cpayv1.PasskeyOptionsRequest{UserId: *reqAuth.UserID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	s.writePasskeyOptions(w, r, resp)
}

func (s *Server) handleFinishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	challengeID, credentialJSON, ok := readPasskeyVerify(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.FinishPasskeyRegistration(s.rpcContext(r.Context()), &cpayv1.PasskeyVerifyRequest{
		ChallengeId:    challengeID,
		CredentialJson: credentialJSON,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeProfileSecurity(w, resp)
	_ = reqAuth
}

func (s *Server) handleBeginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	var req passkeyOptionsRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.BeginPasskeyLogin(s.rpcContext(r.Context()), &cpayv1.PasskeyOptionsRequest{Email: req.Email})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	s.writePasskeyOptions(w, r, resp)
}

func (s *Server) handleFinishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	challengeID, credentialJSON, ok := readPasskeyVerify(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.FinishPasskeyLogin(s.rpcContext(r.Context()), &cpayv1.PasskeyVerifyRequest{
		ChallengeId:    challengeID,
		CredentialJson: credentialJSON,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeAuthExchange(w, http.StatusOK, resp)
}

func (s *Server) handleGetProfileSecurity(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	resp, err := s.authClient.GetProfileSecurity(s.rpcContext(r.Context()), &cpayv1.ProfileSecurityRequest{UserId: *reqAuth.UserID, MerchantId: reqAuth.MerchantID})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeProfileSecurity(w, resp)
}

func (s *Server) handleLinkGoogle(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req googleConsumeRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.LinkGoogle(s.rpcContext(r.Context()), &cpayv1.LinkGoogleRequest{
		UserId:      *reqAuth.UserID,
		MerchantId:  reqAuth.MerchantID,
		Code:        req.Code,
		State:       req.State,
		RedirectUri: req.RedirectURI,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeProfileSecurity(w, resp)
}

func (s *Server) handleLinkWallet(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req walletVerifyRequest
	if !s.parseJSON(w, r, &req) {
		return
	}
	resp, err := s.authClient.LinkWallet(s.rpcContext(r.Context()), &cpayv1.LinkWalletRequest{
		UserId:      *reqAuth.UserID,
		MerchantId:  reqAuth.MerchantID,
		ChallengeId: req.ChallengeID,
		Address:     req.Address,
		ChainId:     req.ChainID,
		Message:     req.Message,
		Signature:   req.Signature,
	})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	writeProfileSecurity(w, resp)
}

func (s *Server) handleDeleteIdentity(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	resp, err := s.authClient.DeleteIdentity(s.rpcContext(r.Context()), &cpayv1.DeleteIdentityRequest{UserId: *reqAuth.UserID, MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "deleted": resp.GetDeleted()})
}

func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	reqAuth, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	resp, err := s.authClient.DeletePasskey(s.rpcContext(r.Context()), &cpayv1.DeletePasskeyRequest{UserId: *reqAuth.UserID, MerchantId: reqAuth.MerchantID, Id: id})
	if err != nil {
		s.writeRPCError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": resp.GetId(), "deleted": resp.GetDeleted()})
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

func writeAuthExchange(w http.ResponseWriter, status int, resp *cpayv1.AuthExchangeResponse) {
	if resp.GetOnboardingRequired() {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"onboarding_required": true,
			"onboarding_token":    resp.GetOnboardingToken(),
			"email":               emptyToNil(resp.GetEmail()),
			"provider":            resp.GetProvider(),
		})
		return
	}
	httpx.WriteJSON(w, status, map[string]any{
		"access_token":  resp.GetTokens().GetAccessToken(),
		"refresh_token": resp.GetTokens().GetRefreshToken(),
		"token_type":    resp.GetTokens().GetTokenType(),
		"expires_in":    resp.GetTokens().GetExpiresIn(),
		"user": map[string]any{
			"id":          resp.GetUser().GetId(),
			"merchant_id": resp.GetUser().GetMerchantId(),
			"role":        resp.GetUser().GetRole(),
			"email":       resp.GetUser().GetEmail(),
		},
	})
}

func (s *Server) writePasskeyOptions(w http.ResponseWriter, r *http.Request, resp *cpayv1.PasskeyOptionsResponse) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.GetPublicKeyJson()), &payload); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "passkey options are invalid", middleware.GetRequestID(r.Context()))
		return
	}
	payload["challenge_id"] = resp.GetChallengeId()
	httpx.WriteJSON(w, http.StatusOK, payload)
}

func readPasskeyVerify(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid json", middleware.GetRequestID(r.Context()))
		return "", "", false
	}
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid json: "+err.Error(), middleware.GetRequestID(r.Context()))
		return "", "", false
	}
	challengeID, _ := body["challenge_id"].(string)
	if strings.TrimSpace(challengeID) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "challenge_id is required", middleware.GetRequestID(r.Context()))
		return "", "", false
	}
	delete(body, "challenge_id")
	credentialRaw, _ := json.Marshal(body)
	return challengeID, string(credentialRaw), true
}

func writeProfileSecurity(w http.ResponseWriter, resp *cpayv1.ProfileSecurityResponse) {
	identities := make([]map[string]any, 0, len(resp.GetIdentities()))
	for _, item := range resp.GetIdentities() {
		identities = append(identities, map[string]any{
			"id":               item.GetId(),
			"provider":         item.GetProvider(),
			"provider_subject": item.GetProviderSubject(),
			"email":            emptyToNil(item.GetEmail()),
			"display_name":     emptyToNil(item.GetDisplayName()),
			"created_at":       item.GetCreatedAt(),
		})
	}
	passkeys := make([]map[string]any, 0, len(resp.GetPasskeys()))
	for _, item := range resp.GetPasskeys() {
		passkeys = append(passkeys, map[string]any{
			"id":            item.GetId(),
			"credential_id": item.GetCredentialId(),
			"name":          emptyToNil(item.GetName()),
			"created_at":    item.GetCreatedAt(),
			"last_used_at":  emptyToNil(item.GetLastUsedAt()),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"identities": identities, "passkeys": passkeys})
}
