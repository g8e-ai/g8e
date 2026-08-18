// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sqliteutil

import (
	"strings"
	"testing"
)

// BenchmarkCompress benchmarks gzip compression at various payload sizes.
func BenchmarkCompress_1KB(b *testing.B) {
	data := []byte(strings.Repeat("a", 1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Compress(data)
	}
}

func BenchmarkCompress_10KB(b *testing.B) {
	data := []byte(strings.Repeat("a", 10*1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Compress(data)
	}
}

func BenchmarkCompress_100KB(b *testing.B) {
	data := []byte(strings.Repeat("a", 100*1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Compress(data)
	}
}

// BenchmarkDecompress benchmarks gzip decompression at various payload sizes.
func BenchmarkDecompress_1KB(b *testing.B) {
	data := []byte(strings.Repeat("a", 1024))
	compressed, _ := Compress(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decompress(compressed)
	}
}

func BenchmarkDecompress_10KB(b *testing.B) {
	data := []byte(strings.Repeat("a", 10*1024))
	compressed, _ := Compress(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decompress(compressed)
	}
}

// BenchmarkHashBytes benchmarks SHA-256 hashing.
func BenchmarkHashBytes(b *testing.B) {
	data := []byte(strings.Repeat("a", 1024))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashBytes(data)
	}
}

// BenchmarkCompressDecompressRoundTrip benchmarks the full compress+decompress cycle.
func BenchmarkCompressDecompressRoundTrip(b *testing.B) {
	data := []byte(strings.Repeat("execution output line\n", 500))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed, _ := Compress(data)
		_, _ = Decompress(compressed)
	}
}
