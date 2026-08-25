// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/require"
)

func minimalPathsJSON(t *testing.T) string {
	t.Helper()

	paths := config.DefaultInfraPaths()
	paths.Infra.AppCertDir = constants.DefaultPKIDir + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirApps
	paths.Infra.CACertPath = constants.DefaultPKIDir + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileGatewayBundle
	paths.Infra.DBPath = constants.DefaultDataDir + "/g8e.db"
	paths.Infra.DocsDir = constants.DocsDirname
	paths.Infra.PKIDir = constants.DefaultPKIDir
	paths.Infra.ProtocolConstantsDir = constants.ProtocolDirname + "/" + constants.ProtocolConstantsDirname
	paths.Infra.ProtocolDir = constants.ProtocolDirname
	paths.Infra.ProtocolModelsDir = constants.ProtocolDirname + "/" + constants.ProtocolModelsDirname
	paths.Infra.SecretsDir = constants.DefaultSecretsDir
	paths.Infra.SSHConfigPath = constants.RuntimeDirname + "/" + constants.SshConfigFilename

	b, err := json.Marshal(paths)
	require.NoError(t, err)
	return string(b)
}
