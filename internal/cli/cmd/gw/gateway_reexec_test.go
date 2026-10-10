// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestGatewayReExecArgs_PreserveSpectatorAndFollowingFlags(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg := serve.GatewayConfig{
				Posture:                    constants.PostureDoctrine,
				HTTPPort:                   18080,
				HTTPSPort:                  18443,
				PublicSpectatorEnabled:     enabled,
				PublicSpectatorPrivateAddr: "127.0.0.1:18081",
				EvalExplorerAddr:           "127.0.0.1:15173",
				EnsembleUpstreamURL:        "http://127.0.0.1:18000",
			}
			pm := &platform.ProcessManager{}
			args, err := pm.BuildReExecArgs(platform.OperatorStartOptions{GatewayConfig: cfg})
			require.NoError(t, err)

			cmd := gatewayStartCmd()
			require.NoError(t, cmd.ParseFlags(args[2:]))
			assert.Empty(t, cmd.Flags().Args(), "a boolean value must not become a positional argument")
			spectator, err := cmd.Flags().GetBool("public-spectator")
			require.NoError(t, err)
			assert.Equal(t, enabled, spectator)
			for flag, want := range map[string]string{
				"public-spectator-private-listen": cfg.PublicSpectatorPrivateAddr,
				"eval-explorer-listen":            cfg.EvalExplorerAddr,
				"ensemble-upstream-url":           cfg.EnsembleUpstreamURL,
			} {
				value, err := cmd.Flags().GetString(flag)
				require.NoError(t, err)
				assert.Equal(t, want, value, "--%s must survive background re-execution", flag)
			}
		})
	}
}
