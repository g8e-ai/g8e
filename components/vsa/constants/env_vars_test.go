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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvVarConstants_OperatorKeys(t *testing.T) {
	assert.Equal(t, "G8E_OPERATOR_API_KEY", string(EnvVar.OperatorAPIKey))
	assert.Equal(t, "G8E_OPERATOR_SESSION_ID", string(EnvVar.OperatorSessionID))
	assert.Equal(t, "G8E_OPERATOR_ENDPOINT", string(EnvVar.OperatorEndpoint))
	assert.Equal(t, "G8E_DEVICE_TOKEN", string(EnvVar.DeviceToken))
	assert.Equal(t, "G8E_LOG_LEVEL", string(EnvVar.LogLevel))
	assert.Equal(t, "G8E_LOCAL_STORE_ENABLED", string(EnvVar.LocalStoreEnabled))
	assert.Equal(t, "G8E_LOCAL_DB_PATH", string(EnvVar.LocalDBPath))
	assert.Equal(t, "G8E_LOCAL_STORE_MAX_SIZE_MB", string(EnvVar.LocalStoreMaxSizeMB))
	assert.Equal(t, "G8E_LOCAL_STORE_RETENTION_DAYS", string(EnvVar.LocalStoreRetentionDays))
	assert.Equal(t, "G8E_DATA_DIR", string(EnvVar.DataDir))
	assert.Equal(t, "G8E_IP_SERVICE", string(EnvVar.IPService))
	assert.Equal(t, "G8E_IP_RESOLVER", string(EnvVar.IPResolver))
}

func TestEnvVarConstants_SystemEnvVars(t *testing.T) {
	assert.Equal(t, "SHELL", string(EnvVar.Shell))
	assert.Equal(t, "LANG", string(EnvVar.Lang))
	assert.Equal(t, "TERM", string(EnvVar.Term))
	assert.Equal(t, "PATH", string(EnvVar.Path))
	assert.Equal(t, "SSH_AUTH_SOCK", string(EnvVar.SSHAuthSock))
	assert.Equal(t, "USER", string(EnvVar.User))
	assert.Equal(t, "USERNAME", string(EnvVar.Username))
	assert.Equal(t, "LOGNAME", string(EnvVar.LogName))
	assert.Equal(t, "OPENCLAW_GATEWAY_TOKEN", string(EnvVar.OpenClawGatewayToken))
}
