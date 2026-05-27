package format

import (
	"testing"
	"time"
)

func TestJSONOrDefault(t *testing.T) {
	if got, err := JSONOrDefault("", "{}"); err != nil || got != "{}" {
		t.Fatalf("expected default JSON, got %q err=%v", got, err)
	}
	if got, err := JSONOrDefault(`{"ok":true}`, "{}"); err != nil || got != `{"ok":true}` {
		t.Fatalf("expected original JSON, got %q err=%v", got, err)
	}
	if _, err := JSONOrDefault("{", "{}"); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestJSONHelpersWithDefaults(t *testing.T) {
	if got := JSONStringOrDefault(nil, "{}"); got != "{}" {
		t.Fatalf("expected default JSON string, got %q", got)
	}
	if got := JSONStringOrDefault(map[string]any{"ok": true}, "{}"); got != `{"ok":true}` {
		t.Fatalf("unexpected JSON string: %q", got)
	}

	def := "default"
	if got := JSONValueOrDefault("", def); got != def {
		t.Fatalf("expected default for empty JSON")
	}
	if got := JSONValueOrDefault("{", def); got != def {
		t.Fatalf("expected default for invalid JSON")
	}
	got, ok := JSONValueOrDefault(`{"ok":true}`, def).(map[string]any)
	if !ok || got["ok"] != true {
		t.Fatalf("unexpected parsed JSON value: %#v", got)
	}
}

func TestValueFormatters(t *testing.T) {
	ts := time.Date(2026, 5, 27, 10, 30, 0, 0, time.FixedZone("UTC+3", 3*3600))
	if got := TimePtr(&ts); got != "2026-05-27T07:30:00Z" {
		t.Fatalf("unexpected time string: %q", got)
	}
	if got := TimePtr(nil); got != "" {
		t.Fatalf("expected empty nil time, got %q", got)
	}

	value := "x"
	if got := StringPtr(&value); got != "x" {
		t.Fatalf("unexpected string ptr value: %q", got)
	}
	if got := StringPtr(nil); got != "" {
		t.Fatalf("expected empty nil string, got %q", got)
	}
	if got := StringOrNil(" x "); got != "x" {
		t.Fatalf("unexpected string or nil value: %#v", got)
	}
	if got := StringOrNil(" "); got != nil {
		t.Fatalf("expected blank string to become nil, got %#v", got)
	}

	if got := BytesOrDefault(nil, "{}"); got != "{}" {
		t.Fatalf("expected default bytes string, got %q", got)
	}
	if got := BytesOrDefault([]byte("raw"), "{}"); got != "raw" {
		t.Fatalf("expected raw bytes string, got %q", got)
	}
}

func TestFloatFormatters(t *testing.T) {
	if got := Float64OrZero(" 12.5 "); got != 12.5 {
		t.Fatalf("unexpected float value: %v", got)
	}
	if got := Float64OrZero("bad"); got != 0 {
		t.Fatalf("expected invalid float to become zero, got %v", got)
	}

	if got := Float64Ptr("2.25"); got == nil || *got != 2.25 {
		t.Fatalf("unexpected float pointer: %v", got)
	}
	if got := Float64Ptr(""); got != nil {
		t.Fatalf("expected empty float pointer to be nil, got %v", *got)
	}
	if got := Float64Ptr("bad"); got != nil {
		t.Fatalf("expected invalid float pointer to be nil, got %v", *got)
	}
}
