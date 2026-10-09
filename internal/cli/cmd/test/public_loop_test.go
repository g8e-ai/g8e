// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testcmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPublicLoopRoot_IsUnderLocalDevNeverTemp(t *testing.T) {
	now := time.Date(2026, 10, 9, 14, 5, 6, 0, time.FixedZone("x", 3600))
	root := defaultPublicLoopRoot(now)
	assert.Equal(t, filepath.Join(".local.dev", "public-loop", "2026-10-09T13-05-06Z"), root)
	assert.False(t, filepath.IsAbs(root), "the default is relative to the working directory, not the OS temp directory")
}

