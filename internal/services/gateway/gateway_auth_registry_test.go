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

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestPrivilegedRouteRegistry_PrivilegedPaths(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	registry := NewPrivilegedRouteRegistry()

	assert.True(t, registry.IsPrivileged(constants.APIPaths.GovernanceEnvelopes),
		"Governance envelopes must be privileged (app certs blocked)")
	assert.True(t, registry.IsPrivileged(constants.APIPaths.QueryPrefix),
		"Query prefix must be privileged (app certs blocked)")
}
