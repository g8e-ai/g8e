// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDockerGateway_Health tests the Docker-based gateway health endpoints.
func TestDockerGateway_Health(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	t.Run("gateway HTTP health", func(t *testing.T) {
		health := f.GetHealth(t)
		require.Equal(t, "ok", health["status"], "health check failed")
		t.Logf("Health status: %v", health)
	})

	t.Run("CA bundle discoverable over HTTP", func(t *testing.T) {
		bundle := f.GetCABundle(t)
		require.NotEmpty(t, bundle, "CA bundle is empty")
		require.Contains(t, bundle, "BEGIN CERTIFICATE", "CA bundle does not contain PEM certificate")
		t.Logf("CA bundle retrieved successfully (length: %d)", len(bundle))
	})

	t.Run("enrolled operator cert completes mTLS handshake", func(t *testing.T) {
		f.DialGatewayMTLS(t)
		t.Log("mTLS handshake succeeded with enrolled operator certificate")
	})

	t.Run("operator container connected", func(t *testing.T) {
		f.CheckOperatorContainer(t)
	})
}
