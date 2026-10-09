// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestGatewayConstructsAndRegistersAllEmbeddedOperatorRoles(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)
	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.VaultDir = fileSvc.Resolve(constants.VaultDirname)
	cfg.Gateway.Posture = config.PostureDoctrine
	cfg.OperatorRoles = constants.OperatorRoles{constants.OperatorRoleEmbedded, constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver}
	cfg.Inference = config.InferenceConfig{Enabled: true, OllamaEndpoint: "http://127.0.0.1:11434", KeepAlive: "-1"}
	cfg.ProviderBoundaryObserver = config.ProviderBoundaryObserverConfig{Enabled: true, ObserverID: "embedded-observer"}
	cfg.ProvenanceOperator = config.ProvenanceOperatorConfig{Enabled: true, OperatorID: "embedded-provenance", ModelStorageRoot: testutil.TempDir(t)}
	svc, err := NewGatewayModeService(cfg, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { svc.Stop(context.Background()) })
	_, sessionID, err := svc.embeddedOperator.ClaimEmbeddedOperator(t.Context(), "owner")
	require.NoError(t, err)
	ops, err := svc.reg.ListUserOperators(t.Context(), "owner")
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, sessionID, ops[0].OperatorSessionId)
	require.Equal(t, cfg.OperatorRoles, operatorcapability.GetOperatorRoles(ops[0]))
	require.True(t, operatorcapability.IsDataOperator(ops[0]))
	require.Len(t, operatorcapability.ActiveProvenanceOperators(ops), 1)
	require.Len(t, operatorcapability.ActiveProviderBoundaryObservers(ops), 1)
}
