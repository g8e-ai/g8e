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
	"github.com/stretchr/testify/require"
)

func minimalPathsJSON(t *testing.T) string {
	t.Helper()

	paths := config.DefaultInfraPaths()
	paths.Infra.AppCertDir = ".g8e/app/certs"
	paths.Infra.CACertPath = ".g8e/pki/trust/g8eg-ca-bundle.pem"
	paths.Infra.DBPath = ".g8e/g8e.db"
	paths.Infra.DocsDir = "docs"
	paths.Infra.PKIDir = ".g8e/pki"
	paths.Infra.ProtocolConstantsDir = "protocol/constants"
	paths.Infra.ProtocolDir = "protocol"
	paths.Infra.ProtocolModelsDir = "protocol/models"
	paths.Infra.SecretsDir = ".g8e/secrets"
	paths.Infra.SSHConfigPath = ".g8e/ssh/config"

	b, err := json.Marshal(paths)
	require.NoError(t, err)
	return string(b)
}
