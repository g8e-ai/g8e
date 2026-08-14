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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestDockerGateway_OperatorRegistryActive verifies that after automatic enrollment
// and re-authentication, the gateway's operator registry records the operator as active.
func TestDockerGateway_OperatorRegistryActive(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	// Ensure operator has completed bootstrap authentication
	f.CheckOperatorContainer(t)

	// Obtain session ID from container env
	sessionID := f.GetOperatorSessionID(t)
	require.NotEmpty(t, sessionID, "Operator session ID should not be empty")

	// Query gateway registry via GET /api/v1/operators/session/{id}
	op := f.GetOperatorBySession(t, sessionID)
	require.NotNil(t, op, "Operator document should be returned from gateway registry")

	assert.Equal(t, constants.OperatorStatusActive, op.Status, "Operator status in gateway registry should be active")
	assert.Equal(t, sessionID, op.OperatorSessionID, "Operator session ID should match session ID from container env")
	assert.NotEmpty(t, op.ID, "Operator ID should not be empty")
}
