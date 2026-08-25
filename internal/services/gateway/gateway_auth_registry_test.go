// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestPrivilegedRouteRegistry_PrivilegedPaths(t *testing.T) {

	registry := NewPrivilegedRouteRegistry()

	privilegedPaths := []string{
		constants.APIPaths.GovernanceEnvelopes,
		constants.APIPaths.QueryPrefix,
		constants.APIPaths.QueryPrefix + "/some-query",
		constants.APIPaths.QueryPrefix + "/operators",
	}

	for _, path := range privilegedPaths {
		assert.True(t, registry.IsPrivileged(path), "Path %s should be privileged", path)
	}
}

func TestPrivilegedRouteRegistry_NonPrivilegedPaths(t *testing.T) {

	registry := NewPrivilegedRouteRegistry()

	nonPrivilegedPaths := []string{
		constants.APIPaths.Health,
		constants.APIPaths.State,
		constants.APIPaths.MCPEndpoint,
		constants.APIPaths.A2ACall,
		constants.APIPaths.AuditReceipts,
		constants.APIPaths.Operators,
		constants.APIPaths.DataSettings,
		constants.APIPaths.KV,
		"/api/v1/pki/apps/enroll",
	}

	for _, path := range nonPrivilegedPaths {
		assert.False(t, registry.IsPrivileged(path), "Path %s should NOT be privileged", path)
	}
}

func TestPrivilegedRouteRegistry_CanonicalCoverage(t *testing.T) {

	registry := NewPrivilegedRouteRegistry()

	assert.True(t, registry.IsPrivileged(constants.APIPaths.GovernanceEnvelopes),
		"Governance envelopes must be privileged (app certs blocked)")
	assert.True(t, registry.IsPrivileged(constants.APIPaths.QueryPrefix),
		"Query prefix must be privileged (app certs blocked)")
}
