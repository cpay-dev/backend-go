package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	siwe "github.com/spruceid/siwe-go"
)

type contextKey string

const userIDKey contextKey = "userID"
const walletKey contextKey = "wallet"
const roleKey contextKey = "role"

type Service struct {
	jwtSecret []byte
}

func NewService(secret string) *Service {
	return &Service{jwtSecret: []byte(secret)}
}

func (s *Service) GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) VerifySIWE(message, signature string) (string, error) {
	msg, err := siwe.ParseMessage(message)
	if err != nil {
		return "", fmt.Errorf("parse SIWE message: %w", err)
	}

	_, err = msg.Verify(signature, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("verify SIWE signature: %w", err)
	}

	return msg.GetAddress().String(), nil
}

func (s *Service) IssueJWT(userID, walletAddress string, role *string) (string, error) {
	claims := jwt.MapClaims{
		"sub":    userID,
		"wallet": walletAddress,
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
	}
	if role != nil {
		claims["role"] = *role
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) ValidateJWT(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse JWT: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid JWT claims")
	}

	return claims, nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		claims, err := s.ValidateJWT(tokenString)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims["sub"].(string))
		ctx = context.WithValue(ctx, walletKey, claims["wallet"].(string))
		if role, ok := claims["role"].(string); ok {
			ctx = context.WithValue(ctx, roleKey, role)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

func WalletFromContext(ctx context.Context) string {
	v, _ := ctx.Value(walletKey).(string)
	return v
}

func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(roleKey).(string)
	return v
}
