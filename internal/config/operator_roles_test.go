// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package config_test

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestStartupEnablesEveryRequestedOperatorRoleCombination(t *testing.T) {
	all := []constants.OperatorRole{constants.OperatorRoleEmbedded, constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver}
	for mask := 1; mask < 1<<len(all); mask++ {
		roles := constants.OperatorRoles{}
		for i, role := range all {
			if mask&(1<<i) != 0 {
				roles = append(roles, role)
			}
		}
		t.Run(roles.String(), func(t *testing.T) {
			var cfg *config.Config
			var err error
			if roles.Has(constants.OperatorRoleEmbedded) {
				cfg, err = config.LoadGateway(config.GatewayOptions{OperatorRoles: roles})
			} else {
				cfg, err = config.Load(config.LoadOptions{OperatorRoles: roles, OperatorEndpoint: constants.DefaultEndpoint, WorkDir: testutil.TempDir(t)})
			}
			require.NoError(t, err)
			require.Equal(t, roles, cfg.EffectiveOperatorRoles())
			require.Equal(t, roles.Has(constants.OperatorRoleInference), cfg.Inference.Enabled)
			require.Equal(t, roles.Has(constants.OperatorRoleProvenance), cfg.ProvenanceOperator.Enabled)
			require.Equal(t, roles.Has(constants.OperatorRoleObserver), cfg.ProviderBoundaryObserver.Enabled)
		})
	}
}
