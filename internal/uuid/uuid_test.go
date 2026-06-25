package uuid

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"testing"
)

var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewString(t *testing.T) {
	s := NewString()
	if !uuidV4Regex.MatchString(s) {
		t.Fatalf("NewString() = %q, want a valid RFC 4122 v4 UUID", s)
	}
}

func TestNewStringUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		s := NewString()
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate UUID generated: %s", s)
		}
		seen[s] = struct{}{}
	}
}

func TestNewStringLength(t *testing.T) {
	s := NewString()
	if len(s) != 36 {
		t.Fatalf("NewString() length = %d, want 36", len(s))
	}
}

func TestParse(t *testing.T) {
	original := "550e8400-e29b-41d4-a716-446655440000"
	b, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(b) != 16 {
		t.Fatalf("Parse() returned %d bytes, want 16", len(b))
	}
	// Round-trip: format the bytes back and compare
	formatted := fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
	if formatted != original {
		t.Fatalf("round-trip mismatch: got %q, want %q", formatted, original)
	}
}

func TestParseInvalid(t *testing.T) {
	invalid := []string{
		"",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716",
		"550e8400e29b41d4a716446655440000",
		"gggggggg-gggg-gggg-gggg-gggggggggggg",
	}
	for _, s := range invalid {
		if _, err := Parse(s); err == nil {
			t.Fatalf("Parse(%q) succeeded, want error", s)
		}
	}
}
