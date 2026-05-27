package httpx

import (
	"net/http"
	"strconv"
)

func ParsePagination(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := ParseLimit(r, defaultLimit, maxLimit)
	offset := 0
	if q := r.URL.Query().Get("offset"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 0 {
			offset = v
		}
	}
	return limit, offset
}

func ParseLimit(r *http.Request, defaultLimit, maxLimit int) int {
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= maxLimit {
			return v
		}
	}
	return defaultLimit
}
