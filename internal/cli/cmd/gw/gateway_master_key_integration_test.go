// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gw

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
)

func TestGatewayMasterKey_EnvironmentPersistsAcrossRestart(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	t.Setenv(string(constants.EnvVar.MasterKeyFile), " /run/secrets/original ")
	resolved := resolveGatewayFlags(GatewayFlags{MasterKeyFile: " ", Posture: "doctrine", HTTPPort: 8080, HTTPSPort: 8443, CertIdentityMode: "localhost", LogLevel: "info"})
	require.Equal(t, "/run/secrets/original", resolved.MasterKeyFile)
	cfg := gatewayFlagsToServeConfig(resolved)
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, cfg))
	for _, env := range []string{"", "/run/secrets/different"} {
		t.Setenv(string(constants.EnvVar.MasterKeyFile), env)
		profile, err := serve.ReadLaunchProfile(fileSvc)
		require.NoError(t, err)
		require.Equal(t, "/run/secrets/original", profile.Config.MasterKeyFile)
		require.Equal(t, "/run/secrets/original", keystore.ResolveMasterKeyFile(profile.Config.MasterKeyFile))
	}
	require.Equal(t, "/explicit", resolveGatewayFlags(GatewayFlags{MasterKeyFile: " /explicit "}).MasterKeyFile)
}
