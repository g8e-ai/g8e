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

func TestResolveOperatorRole(t *testing.T) {
	t.Parallel()

	assert.Equal(t, constants.OperatorRoleData, ResolveOperatorRole(nil))
	assert.Equal(t, constants.OperatorRoleData, ResolveOperatorRole(&models.RuntimeConfig{}))

	assert.Equal(t, constants.OperatorRoleInference, ResolveOperatorRole(&models.RuntimeConfig{
		InferenceEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoleProvenance, ResolveOperatorRole(&models.RuntimeConfig{
		ProvenanceOperatorEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoleObserver, ResolveOperatorRole(&models.RuntimeConfig{
		ProviderBoundaryObserverEnabled: true,
	}))

	assert.Equal(t, constants.OperatorRoleData, ResolveOperatorRole(&models.RuntimeConfig{
		Role: constants.OperatorRoleData,
	}))
}

func TestGetOperatorRole(t *testing.T) {
	t.Parallel()

	opDoc := models.OperatorDocumentGo{
		OperatorRole: constants.OperatorRoleInference,
		RuntimeConfig: &models.RuntimeConfig{
			Role: constants.OperatorRoleData, // OperatorRole takes precedence
		},
	}
	assert.Equal(t, constants.OperatorRoleInference, GetOperatorRole(opDoc))

	opDoc2 := models.OperatorDocumentGo{
		RuntimeConfig: &models.RuntimeConfig{
			ProviderBoundaryObserverEnabled: true,
		},
	}
	assert.Equal(t, constants.OperatorRoleObserver, GetOperatorRole(opDoc2))
}

func TestValidateOperatorRoleCapabilities(t *testing.T) {
	t.Parallel()

	infOp := models.OperatorDocumentGo{
		ID:           "inf-op",
		OperatorRole: constants.OperatorRoleInference,
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
			OperatorRole:      constants.OperatorRoleInference,
			SystemFingerprint: "fp-1",
		}
		opData := models.OperatorDocumentGo{
			ID:                "op-data",
			LocalDir:          "/home/user/runtime",
			Port:              8444,
			Account:           "bob",
			OperatorRole:      constants.OperatorRoleData,
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
			OperatorRole:      constants.OperatorRoleData,
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op2",
			Port:              8443,
			Account:           "bob",
			OperatorRole:      constants.OperatorRoleData,
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
			OperatorRole:      constants.OperatorRoleData,
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8444,
			Account:           "bob",
			OperatorRole:      constants.OperatorRoleData,
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
			OperatorRole:      constants.OperatorRoleData,
			SystemFingerprint: "fp-1",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRole:      constants.OperatorRoleData,
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
			OperatorRole:      constants.OperatorRoleData,
			SystemFingerprint: "fp-same-123456",
		}
		op2 := models.OperatorDocumentGo{
			ID:                "op-2",
			LocalDir:          "/data/g8e/op",
			Port:              8443,
			Account:           "bob",
			OperatorRole:      constants.OperatorRoleData,
			SystemFingerprint: "fp-same-123456",
		}

		separated, reason := VerifyOperatorSeparation(op1, op2)
		assert.False(t, separated)
		assert.Contains(t, reason, "identical")
	})
}
