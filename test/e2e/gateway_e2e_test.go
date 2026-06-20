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
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDockerGateway_Health tests the Docker-based gateway health endpoints.
func TestDockerGateway_Health(t *testing.T) {
	f := NewDockerE2EFixture(t, "docker-compose.yml")

	t.Run("gateway HTTP health", func(t *testing.T) {
		health := f.GetHealth(t)
		require.Equal(t, "running", health["status"], "health check failed")
		t.Logf("Health status: %v", health)
	})

	t.Run("CA bundle discoverable over HTTP", func(t *testing.T) {
		bundle := f.GetCABundle(t)
		require.NotEmpty(t, bundle, "CA bundle is empty")
		require.Contains(t, bundle, "BEGIN CERTIFICATE", "CA bundle does not contain PEM certificate")
		t.Logf("CA bundle retrieved successfully (length: %d)", len(bundle))
	})

	t.Run("HTTPS port reachable (no mTLS)", func(t *testing.T) {
		// TCP dial to verify port is reachable (we don't have mTLS certs for this test)
		conn, err := net.DialTimeout("tcp", "localhost:8443", 5*time.Second)
		require.NoError(t, err, "HTTPS port not reachable")
		conn.Close()
		t.Log("HTTPS port is reachable")
	})

	t.Run("operator container connected", func(t *testing.T) {
		f.CheckOperatorContainer(t)
	})
}
