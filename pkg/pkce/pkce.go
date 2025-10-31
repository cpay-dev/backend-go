package pkce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func GenerateVerifier() string {
	var bb [32]byte
	_, _ = rand.Read(bb[:]) // can't fail
	return base64.RawURLEncoding.EncodeToString(bb[:])
}

func ChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func GenerateStateS256() (verifier string, challenge string) {
	verifier = GenerateVerifier()
	challenge = ChallengeS256(verifier)
	return
}
