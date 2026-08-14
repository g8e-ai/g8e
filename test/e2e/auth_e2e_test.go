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
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDockerGateway_Auth tests cross-container authentication behaviors that are
// observable from outside the containers — log-based assertions and HTTP health
// checks only. No mTLS connections from the test process, no cert extraction
// from Docker volumes.
func TestDockerGateway_Auth(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	t.Run("mTLS handshake over network", func(t *testing.T) {
		// The operator establishes mTLS with the gateway over the Docker bridge
		// network. Observable via operator logs containing the auth success marker.
		f.CheckOperatorContainer(t)
	})

	t.Run("CA bundle consistency", func(t *testing.T) {
		// Fetch the CA bundle from the gateway's well-known endpoint and verify
		// it contains valid PEM certificates. The operator trusts this same bundle
		// (observable in its logs via the authentication success marker).
		bundle := f.GetCABundle(t)
		require.NotEmpty(t, bundle, "CA bundle is empty")
		require.Contains(t, bundle, "BEGIN CERTIFICATE", "CA bundle does not contain PEM certificate")

		// Verify operator logs show successful authentication using the same CA chain.
		// Wait for the operator to complete bootstrap authentication, since it may
		// still be enrolling when the gateway first becomes healthy.
		opContainerName := f.ContainerPrefix + "-operator"
		startedAt := f.OperatorStartedAt(t)
		require.Eventually(t, func() bool {
			logsCmd := exec.Command("docker", "logs", "--since", startedAt, opContainerName)
			logsOutput, err := logsCmd.CombinedOutput()
			if err != nil {
				return false
			}
			return strings.Contains(string(logsOutput), "Authentication successful")
		}, 120*time.Second, 2*time.Second,
			"Operator logs do not contain authentication success marker — CA bundle may be inconsistent")
	})

	t.Run("restart persistence", func(t *testing.T) {
		// Restart the operator container. After restart, it should re-authenticate
		// using its persisted enrolled identity (from the Docker volume), without
		// needing a fresh bootstrap.
		f.RestartOperator(t)

		// Verify the operator re-authenticated after restart using windowed logs.
		// OperatorLogsSince uses the post-restart StartedAt timestamp, so this
		// assertion can only be satisfied by a post-restart auth success line —
		// not a stale pre-restart line.
		startedAt := f.OperatorStartedAt(t)
		recentLogs := f.OperatorLogsSince(t, startedAt)
		require.Contains(t, recentLogs, "Authentication successful",
			"Operator did not re-authenticate after restart — persisted identity may be lost")
	})
}

// TestDockerGateway_RestartLogWindowing verifies that log-windowing after a
// restart excludes pre-restart stale lines. This is a regression test for the
// bug where CheckOperatorContainer grepped the full log buffer, causing
// post-restart assertions to match pre-restart "Authentication successful"
// lines and return true before the operator actually re-authenticated.
//
// The test captures a specific pre-restart log line before restarting, then
// verifies after restart that: (1) the full log buffer still contains that
// line (proving it existed pre-restart), and (2) the windowed log buffer
// (--since the post-restart StartedAt) does NOT contain it (proving the
// windowing fix excludes stale lines). This approach is timing-independent:
// it does not rely on re-auth being slow, only on docker logs --since
// correctly filtering by timestamp. Finally, it waits for genuine
// re-authentication via CheckOperatorContainer, which uses the same windowed
// approach.
func TestDockerGateway_RestartLogWindowing(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	// Ensure the operator is initially authenticated.
	f.CheckOperatorContainer(t)

	opContainerName := f.ContainerPrefix + "-operator"

	// Capture a pre-restart log line to use as a stale-line probe. The first
	// non-empty line includes a timestamp unique to the pre-restart start, so
	// it cannot appear in post-restart logs. This is the marker we will verify
	// is excluded from windowed logs after restart.
	preRestartLogs := f.OperatorLogs(t)
	preRestartLines := strings.Split(strings.TrimSpace(preRestartLogs), "\n")
	require.NotEmpty(t, preRestartLines, "Operator should have pre-restart logs")
	preRestartMarker := strings.TrimSpace(preRestartLines[0])
	require.NotEmpty(t, preRestartMarker, "First pre-restart log line should not be empty")

	// Restart the operator.
	t.Logf("Restarting operator for log-windowing regression test: %s", opContainerName)
	restartCmd := exec.Command("docker", "restart", opContainerName)
	restartOutput, err := restartCmd.CombinedOutput()
	require.NoError(t, err, "Failed to restart operator: %s", string(restartOutput))

	// Capture the post-restart start time for log windowing.
	startedAt := f.OperatorStartedAt(t)

	// 1. The FULL log buffer still contains the pre-restart line (stale).
	//    This is the source of the bug: without windowing, post-restart
	//    assertions would match this line.
	fullLogs := f.OperatorLogs(t)
	require.Contains(t, fullLogs, preRestartMarker,
		"Pre-restart log line should be present in full log buffer — this is the bug source")

	// 2. The WINDOWED log buffer (--since startedAt) must NOT contain the
	//    pre-restart line. This proves the windowing fix works: post-restart
	//    log checks cannot be satisfied by stale pre-restart lines, regardless
	//    of how fast re-auth completes.
	windowedLogs := f.OperatorLogsSince(t, startedAt)
	require.NotContains(t, windowedLogs, preRestartMarker,
		"Post-restart windowed logs must not contain pre-restart lines — windowing fix is broken")

	// Now wait for real re-authentication using the windowed approach, proving
	// the fix works: CheckOperatorContainer uses --since StartedAt and will only
	// return once the operator genuinely re-authenticates after the restart.
	f.CheckOperatorContainer(t)
}
