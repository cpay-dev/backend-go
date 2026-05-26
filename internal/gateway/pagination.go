package gateway

import (
	"net/http"
	"strconv"
)

func parsePagination(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= maxLimit {
			limit = v
		}
	}

	offset := 0
	if q := r.URL.Query().Get("offset"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v >= 0 {
			offset = v
		}
	}
	return limit, offset
}
