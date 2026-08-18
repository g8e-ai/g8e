// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package system

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_Now(t *testing.T) {
	t.Parallel()
	c := &RealClock{}
	now := c.Now()
	assert.False(t, now.IsZero())
	assert.Equal(t, time.UTC, now.Location())
}
