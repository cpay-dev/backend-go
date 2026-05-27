package authsvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const authChallengeTTL = 10 * time.Minute

type onboardingPayload struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject"`
	Email           string `json:"email,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	WalletAddress   string `json:"wallet_address,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashString(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func credentialIDString(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func intervalSeconds(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
