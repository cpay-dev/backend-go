package authsvc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/ids"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc/codes"
)

func (s *Service) WalletChallenge(ctx context.Context, req *cpayv1.WalletChallengeRequest) (*cpayv1.WalletChallengeResponse, error) {
	if !common.IsHexAddress(req.GetAddress()) {
		return nil, rpcx.E(codes.InvalidArgument, "invalid_request", "address is invalid")
	}
	address := common.HexToAddress(req.GetAddress()).Hex()
	nonce := randomToken(24)
	issuedAt := time.Now().UTC().Format(time.RFC3339)
	message := s.walletMessage(address, strings.TrimSpace(req.GetChainId()), nonce, issuedAt)
	challengeID := ids.New()
	payload, _ := json.Marshal(map[string]string{
		"address":   address,
		"chain_id":  strings.TrimSpace(req.GetChainId()),
		"nonce":     nonce,
		"issued_at": issuedAt,
	})
	if _, err := s.db.Exec(ctx, `
		INSERT INTO auth.auth_challenges(id, kind, provider, provider_subject, challenge_hash, payload, expires_at)
		VALUES($1, 'wallet_login', 'wallet', $2, $3, $4::jsonb, NOW() + $5::interval)
	`, challengeID, strings.ToLower(address), hashString(message), string(payload), intervalSeconds(authChallengeTTL)); err != nil {
		return nil, rpcx.E(codes.Internal, "internal_error", "failed to create wallet challenge")
	}
	return &cpayv1.WalletChallengeResponse{ChallengeId: challengeID, Message: message}, nil
}

func (s *Service) WalletVerify(ctx context.Context, req *cpayv1.WalletVerifyRequest) (*cpayv1.AuthExchangeResponse, error) {
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	address, err := s.consumeWalletChallenge(ctx, challengeID, req.GetAddress(), req.GetMessage(), req.GetSignature())
	if err != nil {
		return nil, err
	}
	return s.authOrOnboard(ctx, onboardingPayload{
		Provider:        "wallet",
		ProviderSubject: strings.ToLower(address),
		WalletAddress:   address,
		ChainID:         strings.TrimSpace(req.GetChainId()),
	})
}

func (s *Service) LinkWallet(ctx context.Context, req *cpayv1.LinkWalletRequest) (*cpayv1.ProfileSecurityResponse, error) {
	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	merchantID, err := parseID(req.GetMerchantId(), "merchant_id")
	if err != nil {
		return nil, err
	}
	challengeID, err := parseID(req.GetChallengeId(), "challenge_id")
	if err != nil {
		return nil, err
	}
	address, err := s.consumeWalletChallenge(ctx, challengeID, req.GetAddress(), req.GetMessage(), req.GetSignature())
	if err != nil {
		return nil, err
	}
	payload := &onboardingPayload{
		Provider:        "wallet",
		ProviderSubject: strings.ToLower(address),
		WalletAddress:   address,
		ChainID:         strings.TrimSpace(req.GetChainId()),
	}
	if err = s.insertIdentityTx(ctx, s.db, userID, merchantID, payload); err != nil {
		return nil, err
	}
	return s.profileSecurity(ctx, userID, merchantID)
}

func (s *Service) consumeWalletChallenge(ctx context.Context, challengeID, rawAddress, message, signature string) (string, error) {
	if !common.IsHexAddress(rawAddress) {
		return "", rpcx.E(codes.InvalidArgument, "invalid_request", "address is invalid")
	}
	address := common.HexToAddress(rawAddress).Hex()

	err := s.db.QueryRow(ctx, `
		SELECT id::text
		FROM auth.auth_challenges
		WHERE id=$1 AND kind='wallet_login' AND provider_subject=$2 AND challenge_hash=$3 AND consumed_at IS NULL AND expires_at > NOW()
	`, challengeID, strings.ToLower(address), hashString(message)).Scan(&challengeID)
	if err != nil {
		return "", rpcx.E(codes.Unauthenticated, "invalid_challenge", "wallet challenge is invalid or expired")
	}
	if !verifyPersonalSignature(address, message, signature) {
		return "", rpcx.E(codes.Unauthenticated, "invalid_signature", "wallet signature is invalid")
	}
	_, _ = s.db.Exec(ctx, `UPDATE auth.auth_challenges SET consumed_at=NOW() WHERE id=$1`, challengeID)
	return address, nil
}

func (s *Service) walletMessage(address, chainID, nonce, issuedAt string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.PublicWebOrigin, "https://"), "http://")
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	lines := []string{
		host + " wants you to sign in with your Ethereum account:",
		address,
		"",
		"Sign in to cpay.dev.",
		"",
		"URI: " + s.cfg.PublicWebOrigin,
		"Version: 1",
		"Nonce: " + nonce,
		"Issued At: " + issuedAt,
	}
	if strings.TrimSpace(chainID) != "" {
		lines = append(lines, "Chain ID: "+strings.TrimSpace(chainID))
	}
	return strings.Join(lines, "\n")
}

func verifyPersonalSignature(address, message, signature string) bool {
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sig) != 65 {
		return false
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(accounts.TextHash([]byte(message)), sig)
	if err != nil {
		return false
	}
	recovered := crypto.PubkeyToAddress(*pub)
	return strings.EqualFold(recovered.Hex(), common.HexToAddress(address).Hex())
}
