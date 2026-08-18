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
