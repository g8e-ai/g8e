// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOperatorRoles(t *testing.T) {
	t.Parallel()

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(nil))
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleInference}, ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{
		InferenceEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleProvenance}, ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{
		ProvenanceOperatorEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleObserver}, ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{
		ProviderBoundaryObserverEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{
		Roles: constants.OperatorRoles{constants.OperatorRoleData},
	}))
}

func TestGetOperatorRoles(t *testing.T) {
	t.Parallel()

	opDoc := operatorv1.OperatorDocument{
		OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference},
		RuntimeConfig: &operatorv1.OperatorRuntimeConfig{
			Roles: constants.OperatorRoles{constants.OperatorRoleData}, // OperatorRole takes precedence
		},
	}
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleData}, GetOperatorRoles(opDoc))

	opDoc2 := operatorv1.OperatorDocument{
		RuntimeConfig: &operatorv1.OperatorRuntimeConfig{
			ProviderBoundaryObserverEnabled: true,
		},
	}
	assert.Equal(t, constants.OperatorRoles{constants.OperatorRoleObserver}, GetOperatorRoles(opDoc2))
}

func TestValidateOperatorRoleCapabilities(t *testing.T) {
	t.Parallel()

	infOp := operatorv1.OperatorDocument{
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
		opInf := operatorv1.OperatorDocument{
			ID:                "op-inf",
			LocalDir:          "/home/user/runtime",
			Port:              8444,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleInference},
			SystemFingerprint: "fp-1",
		}
		opData := operatorv1.OperatorDocument{
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
		op1 := operatorv1.OperatorDocument{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op1",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := operatorv1.OperatorDocument{
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
		op1 := operatorv1.OperatorDocument{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := operatorv1.OperatorDocument{
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
		op1 := operatorv1.OperatorDocument{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "alice",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-1",
		}
		op2 := operatorv1.OperatorDocument{
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
		op1 := operatorv1.OperatorDocument{
			ID:                "op-1",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleData},
			SystemFingerprint: "fp-same-123456",
		}
		op2 := operatorv1.OperatorDocument{
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
		op   *operatorv1.OperatorDocument
		want bool
	}{
		{"active remote", operatorv1.OperatorDocument{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote, OperatorSessionID: "s"}, true},
		{"active embedded with runtime config", operatorv1.OperatorDocument{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorSessionID: "s", RuntimeConfig: &operatorv1.OperatorRuntimeConfig{}}, true},
		{"active embedded without runtime config", operatorv1.OperatorDocument{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeEmbedded, OperatorSessionID: "s"}, false},
		{"stale remote", operatorv1.OperatorDocument{Status: constants.OperatorStatusStale, OperatorType: constants.OperatorTypeRemote, OperatorSessionID: "s"}, false},
		{"active remote without session", operatorv1.OperatorDocument{Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote}, false},
		{"active with unset type", operatorv1.OperatorDocument{Status: constants.OperatorStatusActive, OperatorSessionID: "s"}, false},
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

	inference := operatorv1.OperatorDocument{
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		OperatorSessionID: "s",
		RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleInference}},
	}
	assert.True(t, HasActiveRole(inference, constants.OperatorRoleInference))
	assert.False(t, HasActiveRole(inference, constants.OperatorRoleData))

	inference.Status = constants.OperatorStatusStopped
	assert.False(t, HasActiveRole(inference, constants.OperatorRoleInference))
}
