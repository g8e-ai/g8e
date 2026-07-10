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

//go:build e2e

package e2e

import (
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
func TestMain(m *testing.M) {
	if os.Getenv("G8E_E2E_SKIP_DOCKER") == "1" {
		log.Println("E2E: Skipping Docker setup (G8E_E2E_SKIP_DOCKER=1)")
		os.Exit(m.Run())
	}

	fixture, err := setupSharedE2EFixture("docker-compose.yml")
	if err != nil {
		log.Printf("E2E: Docker setup failed: %v — Docker tests will be skipped\n", err)
		sharedFixture = nil
		os.Exit(m.Run())
	}

	sharedFixture = fixture
	code := m.Run()

	if err := sharedFixture.teardown(); err != nil {
		log.Printf("E2E: Warning: teardown failed: %v\n", err)
	}

	os.Exit(code)
}
