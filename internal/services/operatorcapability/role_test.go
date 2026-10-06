// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOperatorRoles(t *testing.T) {
	t.Parallel()

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(nil))
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(&models.RuntimeConfig{}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleInference}, ResolveOperatorRoles(&models.RuntimeConfig{
		InferenceEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleProvenance}, ResolveOperatorRoles(&models.RuntimeConfig{
		ProvenanceOperatorEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleObserver}, ResolveOperatorRoles(&models.RuntimeConfig{
		ProviderBoundaryObserverEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(&models.RuntimeConfig{
		Roles: constants.OperatorRoles{constants.OperatorRoleData},
	}))
}

func TestGetOperatorRoles(t *testing.T) {
	t.Parallel()

	opDoc := models.OperatorDocumentGo{
		OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference},
		RuntimeConfig: &models.RuntimeConfig{
			Roles: constants.OperatorRoles{constants.OperatorRoleData}, // OperatorRole takes precedence
		},
	}
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, GetOperatorRoles(opDoc))

	opDoc2 := models.OperatorDocumentGo{
		RuntimeConfig: &models.RuntimeConfig{
			ProviderBoundaryObserverEnabled: true,
		},
	}
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleObserver}, GetOperatorRoles(opDoc2))
}

func TestValidateOperatorRoleCapabilities(t *testing.T) {
	t.Parallel()

	infOp := models.OperatorDocumentGo{
		ID:            "inf-op",
		OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference},
	}

	require.NoError(t, ValidateOperatorRoleCapabilities(infOp, constants.OperatorRoleInference))

	err := ValidateOperatorRoleCapabilities(infOp, constants.OperatorRoleData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inference")
	assert.Contains(t, err.Error(), "data")
}

func TestVerifyOperatorSeparation(t *testing.T) {
	t.Parallel()

	t.Run("separated by distinct roles on same host", func(t *testing.T) {
		opInf := models.OperatorDocumentGo{
			ID:                "op-inf",
			LocalDir:          "/home/user/runtime",
			Port:              8444,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleInference},
			SystemFingerprint: "fp-1",
		}
		opData := models.OperatorDocumentGo{
			ID:                "op-data",
			LocalDir:          "/home/user/runtime",
			Port:              8444,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-2",
		}

		separated, reason := VerifyOperatorSeparation(opInf, opData)
		assert.True(t, separated)
		assert.Contains(t, reason, "separated by role")
	})

	t.Run("separated by local directory", func(t *testing.T) {
		op1 := models.OperatorDocumentGo{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op1",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op2",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-2",
		}

		separated, reason := VerifyOperatorSeparation(op1, op2)
		assert.True(t, separated)
		assert.Contains(t, reason, "separated by local directory")
	})

	t.Run("separated by port", func(t *testing.T) {
		op1 := models.OperatorDocumentGo{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8444,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-2",
		}

		separated, reason := VerifyOperatorSeparation(op1, op2)
		assert.True(t, separated)
		assert.Contains(t, reason, "separated by port")
	})

	t.Run("separated by account", func(t *testing.T) {
		op1 := models.OperatorDocumentGo{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "alice",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-2",
		}

		separated, reason := VerifyOperatorSeparation(op1, op2)
		assert.True(t, separated)
		assert.Contains(t, reason, "separated by launching account")
	})

	t.Run("colliding operators on same system detected", func(t *testing.T) {
		op1 := models.OperatorDocumentGo{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-same-123456",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-same-123456",
		}

		separated, reason := VerifyOperatorSeparation(op1, op2)
		assert.False(t, separated)
		assert.Contains(t, reason, "identical")
	})
}

func TestIsActiveOperatorSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   models.OperatorDocumentGo
		want bool
	}{
		{"active remote", models.OperatorDocumentGo{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, OperatorSessionID: "s"}, true},
		{"active embedded with runtime config", models.OperatorDocumentGo{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorSessionID: "s", RuntimeConfig: &models.RuntimeConfig{}}, true},
		{"active embedded without runtime config", models.OperatorDocumentGo{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorSessionID: "s"}, false},
		{"stale remote", models.OperatorDocumentGo{Status: constants.OperatorStatusStale, OperatorType: constants.OperatorTypeRemote, OperatorSessionID: "s"}, false},
		{"active remote without session", models.OperatorDocumentGo{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote}, false},
		{"active with unset type", models.OperatorDocumentGo{Status: constants.OperatorStatusActive, OperatorSessionID: "s"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsActiveOperatorSession(tt.op))
		})
	}
}

func TestHasActiveRoleRequiresActiveSessionAndRole(t *testing.T) {
	t.Parallel()

	inference := models.OperatorDocumentGo{
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		OperatorSessionID: "s",
		RuntimeConfig:     &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleInference}},
	}
	assert.True(t, HasActiveRole(inference, constants.OperatorRoleInference))
	assert.False(t, HasActiveRole(inference, constants.OperatorRoleData))

	inference.Status = constants.OperatorStatusStopped
	assert.False(t, HasActiveRole(inference, constants.OperatorRoleInference))
}
