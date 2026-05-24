package authsvc

import (
	"context"
	"testing"

	cpayv1 "github.com/cpay-dev/cpay/internal/gen/cpay/v1"
	"github.com/cpay-dev/cpay/internal/shared/rpcx"
	"google.golang.org/grpc/codes"
)

func TestLoginPasswordDisabled(t *testing.T) {
	svc := &Service{}

	_, err := svc.Login(context.Background(), &cpayv1.LoginRequest{
		Email:    "admin@cpay.dev",
		Password: "admin123",
	})
	if err == nil {
		t.Fatalf("expected disabled password login error")
	}
	code, reason, _ := rpcx.Parse(err)
	if code != codes.FailedPrecondition {
		t.Fatalf("expected %v, got %v", codes.FailedPrecondition, code)
	}
	if reason != "password_login_disabled" {
		t.Fatalf("expected password_login_disabled, got %q", reason)
	}
}

func TestSignupPasswordDisabledWithoutOnboarding(t *testing.T) {
	svc := &Service{}

	_, err := svc.Signup(context.Background(), &cpayv1.SignupRequest{
		MerchantName: "Demo Merchant",
		Email:        "admin@cpay.dev",
		Password:     "admin123",
	})
	if err == nil {
		t.Fatalf("expected disabled password signup error")
	}
	code, reason, _ := rpcx.Parse(err)
	if code != codes.FailedPrecondition {
		t.Fatalf("expected %v, got %v", codes.FailedPrecondition, code)
	}
	if reason != "password_signup_disabled" {
		t.Fatalf("expected password_signup_disabled, got %q", reason)
	}
}
