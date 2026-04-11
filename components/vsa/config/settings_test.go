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

package config

import (
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/stretchr/testify/assert"
)

func TestLoadSettings_OperatorFields(t *testing.T) {
	t.Setenv(string(constants.EnvVar.OperatorAPIKey), "test-api-key")
	t.Setenv(string(constants.EnvVar.OperatorEndpoint), "10.0.0.1")
	t.Setenv(string(constants.EnvVar.OperatorSessionID), "sess-abc123")
	t.Setenv(string(constants.EnvVar.DeviceToken), "dtok_xyz")
	t.Setenv(string(constants.EnvVar.LogLevel), "debug")
	t.Setenv(string(constants.EnvVar.DataDir), "/custom/data")

	s := LoadSettings()

	assert.Equal(t, "test-api-key", s.OperatorAPIKey)
	assert.Equal(t, "10.0.0.1", s.OperatorEndpoint)
	assert.Equal(t, "sess-abc123", s.OperatorSessionID)
	assert.Equal(t, "dtok_xyz", s.DeviceToken)
	assert.Equal(t, "debug", s.LogLevel)
	assert.Equal(t, "/custom/data", s.DataDir)
}

func TestLoadSettings_IPFields(t *testing.T) {
	t.Setenv(string(constants.EnvVar.IPService), "https://example.com/ip")
	t.Setenv(string(constants.EnvVar.IPResolver), "1.1.1.1:80")

	s := LoadSettings()

	assert.Equal(t, "https://example.com/ip", s.IPService)
	assert.Equal(t, "1.1.1.1:80", s.IPResolver)
}

func TestLoadSettings_OpenClaw(t *testing.T) {
	t.Setenv(string(constants.EnvVar.OpenClawGatewayToken), "oc-token-secret")

	s := LoadSettings()

	assert.Equal(t, "oc-token-secret", s.OpenClawGatewayToken)
}

func TestLoadSettings_SystemEnvVars(t *testing.T) {
	t.Setenv(string(constants.EnvVar.Shell), "/bin/zsh")
	t.Setenv(string(constants.EnvVar.Lang), "en_US.UTF-8")
	t.Setenv(string(constants.EnvVar.Term), "xterm-256color")
	t.Setenv(string(constants.EnvVar.Path), "/usr/local/bin:/usr/bin")
	t.Setenv(string(constants.EnvVar.SSHAuthSock), "/tmp/ssh-agent.sock")
	t.Setenv(string(constants.EnvVar.LogName), "alice")

	s := LoadSettings()

	assert.Equal(t, "/bin/zsh", s.Shell)
	assert.Equal(t, "en_US.UTF-8", s.Lang)
	assert.Equal(t, "xterm-256color", s.Term)
	assert.Equal(t, "/usr/local/bin:/usr/bin", s.Path)
	assert.Equal(t, "/tmp/ssh-agent.sock", s.SSHAuthSock)
	assert.Equal(t, "alice", s.LogName)
}

func TestLoadSettings_User_FromUSER(t *testing.T) {
	t.Setenv(string(constants.EnvVar.User), "admin")
	t.Setenv(string(constants.EnvVar.Username), "")

	s := LoadSettings()

	assert.Equal(t, "admin", s.User)
}

func TestLoadSettings_User_FallsBackToUSERNAME(t *testing.T) {
	t.Setenv(string(constants.EnvVar.User), "")
	t.Setenv(string(constants.EnvVar.Username), "carol")

	s := LoadSettings()

	assert.Equal(t, "carol", s.User)
}

func TestLoadSettings_User_USERTakesPriorityOverUSERNAME(t *testing.T) {
	t.Setenv(string(constants.EnvVar.User), "primary")
	t.Setenv(string(constants.EnvVar.Username), "secondary")

	s := LoadSettings()

	assert.Equal(t, "primary", s.User)
}

func TestLoadSettings_EmptyWhenEnvNotSet(t *testing.T) {
	t.Setenv(string(constants.EnvVar.OperatorAPIKey), "")
	t.Setenv(string(constants.EnvVar.OperatorEndpoint), "")
	t.Setenv(string(constants.EnvVar.OperatorSessionID), "")
	t.Setenv(string(constants.EnvVar.DeviceToken), "")
	t.Setenv(string(constants.EnvVar.LogLevel), "")
	t.Setenv(string(constants.EnvVar.DataDir), "")
	t.Setenv(string(constants.EnvVar.IPService), "")
	t.Setenv(string(constants.EnvVar.IPResolver), "")
	t.Setenv(string(constants.EnvVar.OpenClawGatewayToken), "")

	s := LoadSettings()

	assert.Empty(t, s.OperatorAPIKey)
	assert.Empty(t, s.OperatorEndpoint)
	assert.Empty(t, s.OperatorSessionID)
	assert.Empty(t, s.DeviceToken)
	assert.Empty(t, s.LogLevel)
	assert.Empty(t, s.DataDir)
	assert.Empty(t, s.IPService)
	assert.Empty(t, s.IPResolver)
	assert.Empty(t, s.OpenClawGatewayToken)
}

func TestLoadSettings_AllFieldsPresent(t *testing.T) {
	t.Setenv(string(constants.EnvVar.OperatorAPIKey), "k")
	t.Setenv(string(constants.EnvVar.OperatorEndpoint), "e")
	t.Setenv(string(constants.EnvVar.OperatorSessionID), "s")
	t.Setenv(string(constants.EnvVar.DeviceToken), "d")
	t.Setenv(string(constants.EnvVar.LogLevel), "info")
	t.Setenv(string(constants.EnvVar.DataDir), "/data")
	t.Setenv(string(constants.EnvVar.IPService), "https://ip.svc")
	t.Setenv(string(constants.EnvVar.IPResolver), "8.8.8.8:80")
	t.Setenv(string(constants.EnvVar.OpenClawGatewayToken), "oc")
	t.Setenv(string(constants.EnvVar.Shell), "/bin/bash")
	t.Setenv(string(constants.EnvVar.Lang), "C")
	t.Setenv(string(constants.EnvVar.Term), "dumb")
	t.Setenv(string(constants.EnvVar.Path), "/bin")
	t.Setenv(string(constants.EnvVar.SSHAuthSock), "/run/sock")
	t.Setenv(string(constants.EnvVar.User), "root")
	t.Setenv(string(constants.EnvVar.LogName), "root")

	s := LoadSettings()

	assert.Equal(t, "k", s.OperatorAPIKey)
	assert.Equal(t, "e", s.OperatorEndpoint)
	assert.Equal(t, "s", s.OperatorSessionID)
	assert.Equal(t, "d", s.DeviceToken)
	assert.Equal(t, "info", s.LogLevel)
	assert.Equal(t, "/data", s.DataDir)
	assert.Equal(t, "https://ip.svc", s.IPService)
	assert.Equal(t, "8.8.8.8:80", s.IPResolver)
	assert.Equal(t, "oc", s.OpenClawGatewayToken)
	assert.Equal(t, "/bin/bash", s.Shell)
	assert.Equal(t, "C", s.Lang)
	assert.Equal(t, "dumb", s.Term)
	assert.Equal(t, "/bin", s.Path)
	assert.Equal(t, "/run/sock", s.SSHAuthSock)
	assert.Equal(t, "root", s.User)
	assert.Equal(t, "root", s.LogName)
}
