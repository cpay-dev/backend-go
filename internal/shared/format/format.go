package format

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func JSONOrDefault(raw, def string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	var dst any
	if err := json.Unmarshal([]byte(raw), &dst); err != nil {
		return "", err
	}
	return raw, nil
}

func JSONStringOrDefault(v any, def string) string {
	if v == nil {
		return def
	}
	b, err := json.Marshal(v)
	if err != nil {
		return def
	}
	return string(b)
}

func JSONValueOrDefault(raw string, def any) any {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return def
	}
	return out
}

func TimePtr(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}

func StringPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func StringOrNil(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func BytesOrDefault(raw []byte, def string) string {
	if len(raw) == 0 {
		return def
	}
	return string(raw)
}

func Float64OrZero(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

func Float64Ptr(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}
