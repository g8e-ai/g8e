// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeCredIDRoundTrip(t *testing.T) {
	t.Parallel()

	sizes := []int{0, 1, 16, 32, 64, 128, 256, 1023, 1024}
	for _, size := range sizes {
		size := size
		t.Run("size_"+itoa(size), func(t *testing.T) {
			t.Parallel()
			original := make([]byte, size)
			if size > 0 {
				_, err := rand.Read(original)
				require.NoError(t, err)
			}

			encoded := encodeCredID(original)
			decoded, err := decodeCredID(encoded)
			require.NoError(t, err)
			assert.Equal(t, original, decoded)
		})
	}
}

func TestDecodeCredIDInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string", "", false},
		{"valid base64url", "AQIDBA", false},
		{"contains padding", "AQIDBA==", true},
		{"contains invalid char", "!!!invalid!!!", true},
		{"contains space", "A QIDBA", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeCredID(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEncodeCredIDMatchesRawURLEncoding(t *testing.T) {
	t.Parallel()

	input := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0xfe, 0xff}
	encoded := encodeCredID(input)
	expected := "AQIDBAX-_w"
	assert.Equal(t, expected, encoded)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
