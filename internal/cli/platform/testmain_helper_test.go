// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"os"
	"testing"
)

// TestMain detects when the test binary is re-executed by StartOperator as a
// Gateway subprocess (via "gw start --follow ..."). In that case the first
// positional argument is "gw", which never happens during a normal test run.
// The re-executed subprocess exits immediately so that process-lifecycle
// integration tests can exercise start-failure cleanup (child termination,
// stale-PID removal) without needing a real Gateway binary that responds to
// health checks. During normal test runs TestMain delegates to the default
// test runner.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "gw" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
