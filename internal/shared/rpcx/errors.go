package rpcx

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func E(code codes.Code, reason, msg string) error {
	reason = strings.TrimSpace(reason)
	msg = strings.TrimSpace(msg)
	if reason == "" {
		reason = "internal_error"
	}
	if msg == "" {
		msg = "internal error"
	}
	return status.Error(code, reason+"|"+msg)
}

func Parse(err error) (codes.Code, string, string) {
	if err == nil {
		return codes.OK, "", ""
	}
	st, ok := status.FromError(err)
	if !ok {
		return codes.Internal, "internal_error", "internal error"
	}
	parts := strings.SplitN(st.Message(), "|", 2)
	reason := "internal_error"
	msg := st.Message()
	if len(parts) == 2 {
		reason = strings.TrimSpace(parts[0])
		msg = strings.TrimSpace(parts[1])
	}
	if reason == "" {
		reason = defaultReason(st.Code())
	}
	if msg == "" {
		msg = "internal error"
	}
	return st.Code(), reason, msg
}

func HTTPFromGRPC(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func defaultReason(code codes.Code) string {
	switch code {
	case codes.InvalidArgument:
		return "invalid_request"
	case codes.Unauthenticated:
		return "unauthorized"
	case codes.PermissionDenied:
		return "forbidden"
	case codes.NotFound:
		return "not_found"
	case codes.AlreadyExists:
		return "conflict"
	case codes.ResourceExhausted:
		return "rate_limited"
	case codes.Unavailable:
		return "service_unavailable"
	default:
		return "internal_error"
	}
}
