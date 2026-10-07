// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
)

func TestNewCampaignFormationObservationLoader(t *testing.T) {
	t.Run("returns a loader backed by the gateway reader when healthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, true)
		cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

		loader, err := NewCampaignFormationObservationLoader(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loader)
	})

	t.Run("still builds a local-only loader when the gateway is unhealthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		loader, err := NewCampaignFormationObservationLoader(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loader)
	})
}
