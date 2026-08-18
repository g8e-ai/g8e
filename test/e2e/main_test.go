// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"fmt"
	"log"
	"os"
	"testing"
)

// sharedFixture is the single Docker Compose stack spun up by TestMain and
// shared across all E2E test functions. This eliminates redundant
// up/down/build cycles that previously caused each test to rebuild the
// image and wait for health independently.
var sharedFixture *DockerE2EFixture

// TestMain spins up a single Docker Compose stack once for all E2E tests,
// runs the test suite, then tears down the stack. Tests that require the
// Docker fixture check sharedFixture for nil and skip if unavailable.
// Tests that do not require Docker (e.g. MCP config output tests) run
// regardless.
//
// Failure semantics: E2E tests are Tier 3 and require Docker — there is no
// opt-out. A fixture-setup failure is a hard error: the suite exits non-zero
// with a FATAL message so a broken Docker environment can never produce a
// green build with zero tests run. On any non-zero exit, container logs and
// compose state are captured to a temp dir before teardown so CI failures
// are diagnosable without a local reproduce.
func TestMain(m *testing.M) {
	fixture, err := setupSharedE2EFixture("docker-compose.yml")
	if err != nil {
		// Setup failure: fail loudly. Do not run the suite, since every
		// Docker test would otherwise t.Skip and the run would exit 0.
		fmt.Fprintf(os.Stderr, "FATAL: E2E fixture setup failed — Docker tests cannot run: %v\n", err)
		os.Exit(1)
	}

	sharedFixture = fixture
	code := m.Run()

	// Capture diagnostics before teardown while the containers are still up.
	// Any non-zero exit (test failure or panic) triggers capture.
	if code != 0 {
		sharedFixture.captureDiagnostics(log.Printf)
	}

	if err := sharedFixture.teardown(); err != nil {
		log.Printf("E2E: Warning: teardown failed: %v\n", err)
	}

	os.Exit(code)
}
