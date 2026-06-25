package uuid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewString returns a randomly generated RFC 4122 version 4 UUID string
// in canonical 36-character form (8-4-4-4-12) with hyphens.
// It panics if the system's cryptographic random source fails.
func NewString() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("uuid: crypto/rand failed: %v", err))
	}
	// Set version 4 and variant bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return format(b)
}

// Parse decodes a canonical 36-character UUID string into a [16]byte.
// Returns an error if the string is not a valid UUID.
func Parse(s string) ([16]byte, error) {
	var b [16]byte
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return b, fmt.Errorf("uuid: invalid UUID format %q", s)
	}
	hexStr := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return b, fmt.Errorf("uuid: invalid UUID hex %q: %w", s, err)
	}
	copy(b[:], decoded)
	return b, nil
}

func format(b [16]byte) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
