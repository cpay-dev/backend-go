package random

import "testing"

func TestPrefixedToken(t *testing.T) {
	got, err := PrefixedToken("whsec_", 24)
	if err != nil {
		t.Fatalf("PrefixedToken returned error: %v", err)
	}
	if len(got) <= len("whsec_") || got[:len("whsec_")] != "whsec_" {
		t.Fatalf("unexpected token: %q", got)
	}
}

func TestCode(t *testing.T) {
	const alphabet = "ABC123"
	got, err := Code(alphabet, 16)
	if err != nil {
		t.Fatalf("Code returned error: %v", err)
	}
	if len(got) != 16 {
		t.Fatalf("expected 16 chars, got %d", len(got))
	}
	for _, ch := range got {
		if !containsRune(alphabet, ch) {
			t.Fatalf("code contains char outside alphabet: %q in %q", ch, got)
		}
	}
}

func containsRune(s string, want rune) bool {
	for _, ch := range s {
		if ch == want {
			return true
		}
	}
	return false
}
