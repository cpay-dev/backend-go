package authsvc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"google.golang.org/grpc/codes"
)

type googleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Aud           string `json:"aud"`
	Name          string `json:"name"`
}

func (s *Service) GoogleStart(ctx context.Context, req *cpayv1.GoogleStartRequest) (*cpayv1.GoogleStartResponse, error) {
	if strings.TrimSpace(s.cfg.GoogleClientID) == "" {
		return nil, rpcx.E(codes.FailedPrecondition, "google_not_configured", "google sign-in is not configured")
	}
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	state := randomToken(32)
	challengeID := ids.New()
	payload, _ := json.Marshal(map[string]string{"state": state, "redirect_uri": redirectURI})
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, provider, challenge_hash, payload, expires_at)
		VALUES($1, 'google_oauth', 'google', $2, $3::jsonb, NOW() + $4::interval)
	`, challengeID, hashString(state), string(payload), intervalSeconds(authChallengeTTL)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create google state")
	}
	values := url.Values{}
	values.Set("client_id", s.cfg.GoogleClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", "openid email profile")
	values.Set("state", state)
	values.Set("access_type", "offline")
	return &cpayv1.GoogleStartResponse{
		AuthorizationUrl: "https://accounts.google.com/o/oauth2/auth?" + values.Encode(),
		State:            state,
	}, nil
}

func (s *Service) GoogleConsume(ctx context.Context, req *cpayv1.GoogleConsumeRequest) (*cpayv1.AuthExchangeResponse, error) {
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	payload, err := s.consumeGoogleOAuth(ctx, req.GetState(), req.GetCode(), redirectURI)
	if err != nil {
		return nil, err
	}
	return s.authOrOnboard(ctx, *payload)
}

func (s *Service) LinkGoogle(ctx context.Context, req *cpayv1.LinkGoogleRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(req.GetState())
	redirectURI := strings.TrimSpace(req.GetRedirectUri())
	if redirectURI == "" {
		redirectURI = s.cfg.GoogleRedirectURI
	}
	payload, err := s.consumeGoogleOAuth(ctx, state, req.GetCode(), redirectURI)
	if err != nil {
		return nil, err
	}
	if err = s.insertIdentityTx(ctx, s.db, userID, merchantID, payload); err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) consumeGoogleOAuth(ctx context.Context, state, code, redirectURI string) (*onboardingPayload, error) {
	state = strings.TrimSpace(state)
	if state == "" || strings.TrimSpace(code) == "" {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "code and state are required")
	}

	var challengeID string
	var statePayload []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, payload
		FROM auth.auth_challenges
		WHERE kind='google_oauth' AND challenge_hash=$1 AND consumed_at IS NULL AND expires_at > NOW()
		LIMIT 1
	`, hashString(state)).Scan(&challengeID, &statePayload)
	if err != nil || !googleStateRedirectMatches(statePayload, redirectURI) {
		return nil, rpcx.E(codes.Unauthenticated, "invalid_state", "google state is invalid or expired")
	}

	claims, err := s.exchangeGoogleCode(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `UPDATE auth.auth_challenges SET consumed_at=NOW() WHERE id=$1`, challengeID)
	return &onboardingPayload{
		Provider:        "google",
		ProviderSubject: claims.Sub,
		Email:           strings.ToLower(strings.TrimSpace(claims.Email)),
		DisplayName:     strings.TrimSpace(claims.Name),
	}, nil
}

func (s *Service) exchangeGoogleCode(ctx context.Context, code, redirectURI string) (*googleClaims, error) {
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" {
		return nil, rpcx.E(codes.FailedPrecondition, "google_not_configured", "google sign-in is not configured")
	}
	form := url.Values{}
	form.Set("client_id", s.cfg.GoogleClientID)
	form.Set("client_secret", s.cfg.GoogleClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, rpcx.E(codes.Unavailable, "google_unavailable", "google token exchange failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google token exchange was rejected")
	}
	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err = json.Unmarshal(body, &tokenResp); err != nil || tokenResp.IDToken == "" {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google did not return an identity token")
	}
	infoURL := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(tokenResp.IDToken)
	infoReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	infoResp, err := http.DefaultClient.Do(infoReq)
	if err != nil {
		return nil, rpcx.E(codes.Unavailable, "google_unavailable", "google identity verification failed")
	}
	defer infoResp.Body.Close()
	infoBody, _ := io.ReadAll(infoResp.Body)
	if infoResp.StatusCode < 200 || infoResp.StatusCode >= 300 {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	var claims googleClaims
	if err = json.Unmarshal(infoBody, &claims); err != nil {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	if claims.Aud != s.cfg.GoogleClientID || claims.Sub == "" || strings.ToLower(claims.EmailVerified) != "true" || strings.TrimSpace(claims.Email) == "" {
		return nil, rpcx.E(codes.Unauthenticated, "google_rejected", "google identity token is invalid")
	}
	return &claims, nil
}

func googleStateRedirectMatches(raw []byte, redirectURI string) bool {
	var payload struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(payload.RedirectURI) == strings.TrimSpace(redirectURI)
}
