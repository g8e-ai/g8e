// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration && !race

package tests

import (
	"testing"
	"time"
)

// The burst contract runs at the scale that failed in production: 2000
// Operators enrolling at once, approved in one owner decision.
const (
	burstOperators         = 2000
	burstHeartbeatInterval = 10 * time.Second
	burstTimeout           = 10 * time.Minute
)

// requireBurstBuild skips coverage runs, which cannot carry 2000 instrumented
// Operators; make test-operator-burst runs this size.
func requireBurstBuild(t *testing.T) {
	t.Helper()
	if testing.CoverMode() != "" {
		t.Skip("the 2000-Operator burst requires an uninstrumented build; run make test-operator-burst")
	}
}
