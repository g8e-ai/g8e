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

// FixedClock returns a fixed time for deterministic testing.
type FixedClock struct {
	fixed time.Time
}

func (c *FixedClock) Now() time.Time {
	return c.fixed
}

// NewFixedClock creates a FixedClock set to the given time.
func NewFixedClock(t time.Time) *FixedClock {
	return &FixedClock{fixed: t}
}

