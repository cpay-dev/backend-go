package rpcx

import (
	"testing"

	"google.golang.org/grpc/codes"
)

func TestParse(t *testing.T) {
	err := E(codes.InvalidArgument, "invalid_request", "bad input")
	code, reason, msg := Parse(err)
	if code != codes.InvalidArgument {
		t.Fatalf("expected code %v, got %v", codes.InvalidArgument, code)
	}
	if reason != "invalid_request" {
		t.Fatalf("expected reason invalid_request, got %q", reason)
	}
	if msg != "bad input" {
		t.Fatalf("expected msg bad input, got %q", msg)
	}
}

func TestHTTPFromGRPC(t *testing.T) {
	if got := HTTPFromGRPC(codes.Unauthenticated); got != 401 {
		t.Fatalf("expected 401, got %d", got)
	}
	if got := HTTPFromGRPC(codes.NotFound); got != 404 {
		t.Fatalf("expected 404, got %d", got)
	}
	if got := HTTPFromGRPC(codes.FailedPrecondition); got != 412 {
		t.Fatalf("expected 412, got %d", got)
	}
	if got := HTTPFromGRPC(codes.Internal); got != 500 {
		t.Fatalf("expected 500, got %d", got)
	}
}
