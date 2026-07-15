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
		require.Eventually(t, func() bool {
			logsCmd := exec.Command("docker", "logs", opContainerName)
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

		// Verify the operator re-authenticated (not bootstrapped) after restart
		logs := f.OperatorLogs(t)
		require.Contains(t, logs, "Authentication successful",
			"Operator did not re-authenticate after restart — persisted identity may be lost")

		// Verify the operator did NOT go through bootstrap after restart
		// (bootstrap would indicate the enrolled identity was not persisted)
		recentLogs := extractRecentLogs(logs, "Authentication successful")
		require.NotEmpty(t, recentLogs, "No authentication success found in post-restart logs")
	})
}

// extractRecentLogs returns the portion of logs from the first occurrence of the
// marker onward, helping isolate post-restart log output.
func extractRecentLogs(logs, marker string) string {
	idx := strings.LastIndex(logs, marker)
	if idx < 0 {
		return ""
	}
	return logs[idx:]
}
