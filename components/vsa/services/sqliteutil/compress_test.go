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

package sqliteutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompress_RoundTrip(t *testing.T) {
	original := []byte("hello world, this is some data to compress")

	compressed, err := Compress(original)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
	assert.NotEqual(t, original, compressed)

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompress_EmptyInput(t *testing.T) {
	compressed, err := Compress(nil)
	require.NoError(t, err)
	assert.Nil(t, compressed)

	compressed, err = Compress([]byte{})
	require.NoError(t, err)
	assert.Nil(t, compressed)
}

func TestCompress_LargeInput(t *testing.T) {
	original := []byte(strings.Repeat("abcdefghij", 10000))

	compressed, err := Compress(original)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)
	assert.Less(t, len(compressed), len(original), "compressed should be smaller than original for repetitive data")

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompress_BinaryData(t *testing.T) {
	original := make([]byte, 256)
	for i := range original {
		original[i] = byte(i)
	}

	compressed, err := Compress(original)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestDecompress_EmptyInput(t *testing.T) {
	decompressed, err := Decompress(nil)
	require.NoError(t, err)
	assert.Nil(t, decompressed)

	decompressed, err = Decompress([]byte{})
	require.NoError(t, err)
	assert.Nil(t, decompressed)
}

func TestDecompress_InvalidData(t *testing.T) {
	_, err := Decompress([]byte("this is not gzip data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gzip reader init failed")
}

func TestDecompress_TruncatedData(t *testing.T) {
	original := []byte("some data to compress and then truncate")
	compressed, err := Compress(original)
	require.NoError(t, err)

	truncated := compressed[:len(compressed)/2]
	_, err = Decompress(truncated)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "gzip read failed") || strings.Contains(err.Error(), "gzip reader init failed"),
		"unexpected error: %v", err)
}

func TestHashBytes_Deterministic(t *testing.T) {
	data := []byte("test input")
	h1 := HashBytes(data)
	h2 := HashBytes(data)
	assert.Equal(t, h1, h2)
}

func TestHashBytes_KnownValue(t *testing.T) {
	h := HashBytes([]byte(""))
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", h)
}

func TestHashBytes_DifferentInputsDifferentHashes(t *testing.T) {
	h1 := HashBytes([]byte("aaa"))
	h2 := HashBytes([]byte("bbb"))
	assert.NotEqual(t, h1, h2)
}

func TestHashBytes_HexEncoded(t *testing.T) {
	h := HashBytes([]byte("data"))
	assert.Len(t, h, 64, "SHA-256 hex string must be 64 characters")
	for _, c := range h {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hash must be lowercase hex, got char %q", c)
	}
}

func TestHashString_MatchesHashBytes(t *testing.T) {
	s := "some string value"
	assert.Equal(t, HashBytes([]byte(s)), HashString(s))
}

func TestHashString_Empty(t *testing.T) {
	h := HashString("")
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", h)
}

func TestHashBytes_NilInput(t *testing.T) {
	h := HashBytes(nil)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", h, "nil should hash the same as empty")
}

func TestCompress_SingleByte(t *testing.T) {
	original := []byte{0x42}

	compressed, err := Compress(original)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompress_Decompress_AllZeroBytes(t *testing.T) {
	original := make([]byte, 1024)

	compressed, err := Compress(original)
	require.NoError(t, err)
	assert.NotEmpty(t, compressed)

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompress_OutputIsValidGzip(t *testing.T) {
	compressed, err := Compress([]byte("valid gzip test"))
	require.NoError(t, err)

	assert.Equal(t, byte(0x1f), compressed[0], "gzip magic byte 0")
	assert.Equal(t, byte(0x8b), compressed[1], "gzip magic byte 1")
}

func TestCompress_Decompress_PreservesEmptyString(t *testing.T) {
	original := []byte(" ")

	compressed, err := Compress(original)
	require.NoError(t, err)

	decompressed, err := Decompress(compressed)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(original, decompressed))
}
