// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package sqliteutil

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("compress: gzip write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("compress: gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decompress: gzip reader init: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decompress: gzip read: %w", err)
	}
	return decompressed, nil
}

func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
