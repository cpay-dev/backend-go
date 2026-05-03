package ids

import "testing"

func TestNewReturnsValidULID(t *testing.T) {
	id := New()
	if !IsValid(id) {
		t.Fatalf("generated invalid ULID: %q", id)
	}
	if len(id) != encodedLen {
		t.Fatalf("expected length %d, got %d", encodedLen, len(id))
	}
}

func TestParseNormalizesULID(t *testing.T) {
	got, err := Parse(" 01k8vnbkvt3d54mte6w2yfhz49 ")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "01K8VNBKVT3D54MTE6W2YFHZ49" {
		t.Fatalf("unexpected normalized ULID: %q", got)
	}
}

func TestParseRejectsInvalidULID(t *testing.T) {
	if _, err := Parse("not-a-ulid"); err == nil {
		t.Fatalf("expected invalid ULID error")
	}
}
