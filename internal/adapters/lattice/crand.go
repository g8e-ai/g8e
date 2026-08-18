// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"crypto/rand"
	"encoding/binary"
)

// cryptoRandInt63n returns a non-negative pseudo-random int64 in [0, n).
// It uses crypto/rand to satisfy gosec G404. Modulo bias is acceptable
// for jitter calculations.
func cryptoRandInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	return v % n
}
