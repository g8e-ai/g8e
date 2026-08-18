// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeCredIDMatchesRawURLEncoding(t *testing.T) {

	input := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0xfe, 0xff}
	encoded := encodeCredID(input)
	expected := "AQIDBAX-_w"
	assert.Equal(t, expected, encoded)
}
