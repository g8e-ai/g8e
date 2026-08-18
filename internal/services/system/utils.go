// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package system

import (
	"time"
)

// Clock is an injectable time source for deterministic testing.
type Clock interface {
	Now() time.Time
}

// RealClock uses actual wall time.
type RealClock struct{}

func (c *RealClock) Now() time.Time {
	return time.Now().UTC()
}
