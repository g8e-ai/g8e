// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
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
