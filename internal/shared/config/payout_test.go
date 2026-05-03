package config

import "testing"

func TestParseChainRPCURLs(t *testing.T) {
	got := parseChainRPCURLs("base=https://base.example, polygon=http://polygon.example")
	if got["base"] != "https://base.example" {
		t.Fatalf("unexpected base rpc: %q", got["base"])
	}
	if got["polygon"] != "http://polygon.example" {
		t.Fatalf("unexpected polygon rpc: %q", got["polygon"])
	}
}

func TestValidateChainRPCURLsRequiresURLs(t *testing.T) {
	if err := ValidateChainRPCURLs(nil); err == nil {
		t.Fatalf("expected missing urls error")
	}
}

func TestValidateChainRPCURLsRejectsInvalidURL(t *testing.T) {
	if err := ValidateChainRPCURLs(map[string]string{"base": "not-a-url"}); err == nil {
		t.Fatalf("expected invalid url error")
	}
}
