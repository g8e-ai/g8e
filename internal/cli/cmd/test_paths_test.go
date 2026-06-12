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

	"github.com/stretchr/testify/require"
)

func minimalPathsJSON(t *testing.T) string {
	t.Helper()

	data := map[string]any{
		"host": "localhost",
		"infra": map[string]string{
			"app_cert_dir":           ".g8e/app/certs",
			"ca_cert_path":           ".g8e/pki/trust/g8eg-ca-bundle.pem",
			"db_path":                ".g8e/g8e.db",
			"docs_dir":               "docs",
			"pki_dir":                ".g8e/pki",
			"protocol_constants_dir": "protocol/constants",
			"protocol_dir":           "protocol",
			"protocol_models_dir":    "protocol/models",
			"secrets_dir":            ".g8e/secrets",
			"ssh_config_path":        ".g8e/ssh/config",
		},
	}

	b, err := json.Marshal(data)
	require.NoError(t, err)
	return string(b)
}
