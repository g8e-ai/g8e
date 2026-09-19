// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadVCSStamp_ReturnsStampWithoutPanicking(t *testing.T) {
	stamp := ReadVCSStamp()
	if stamp.Present {
		assert.NotEmpty(t, stamp.Revision)
		return
	}
	assert.Empty(t, stamp.Revision)
	assert.False(t, stamp.Modified)
}
