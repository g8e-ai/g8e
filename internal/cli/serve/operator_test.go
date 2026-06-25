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

package serve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeOperatorOptions_ZeroValue(t *testing.T) {
	var opts ServeOperatorOptions

	assert.Equal(t, "", opts.LogLevel)
	assert.Equal(t, "", opts.Endpoint)
	assert.Equal(t, "", opts.TrustBundlePath)
	assert.Equal(t, "", opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert)
	assert.Equal(t, "", opts.WorkingDir)
	assert.Equal(t, "", opts.LaunchDir)
	assert.False(t, opts.CloudMode)
	assert.Equal(t, "", opts.CloudProvider)
	assert.False(t, opts.ExecutionVault)
	assert.False(t, opts.NoGit)
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval)
}

func TestServeOperatorOptions_FullAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:          "debug",
		Endpoint:          "192.168.1.10",
		TrustBundlePath:   "/etc/g8e/ca.pem",
		PrivateKey:        "/etc/g8e/operator.key",
		ClientCert:        "/etc/g8e/operator.crt",
		WorkingDir:        "/var/lib/g8e",
		LaunchDir:         "/opt/g8e",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 30 * time.Second,
	}

	assert.Equal(t, "debug", opts.LogLevel)
	assert.Equal(t, "192.168.1.10", opts.Endpoint)
	assert.Equal(t, "/etc/g8e/ca.pem", opts.TrustBundlePath)
	assert.Equal(t, "/etc/g8e/operator.key", opts.PrivateKey)
	assert.Equal(t, "/etc/g8e/operator.crt", opts.ClientCert)
	assert.Equal(t, "/var/lib/g8e", opts.WorkingDir)
	assert.Equal(t, "/opt/g8e", opts.LaunchDir)
	assert.True(t, opts.CloudMode)
	assert.Equal(t, "aws", opts.CloudProvider)
	assert.True(t, opts.ExecutionVault)
	assert.True(t, opts.NoGit)
	assert.Equal(t, 30*time.Second, opts.HeartbeatInterval)
}

func TestServeOperatorOptions_Equality(t *testing.T) {
	a := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       "localhost",
		PrivateKey:     "/key.pem",
		ClientCert:     "/cert.pem",
		WorkingDir:     "/work",
		LaunchDir:      "/launch",
		CloudMode:      false,
		ExecutionVault: true,
	}
	b := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       "localhost",
		PrivateKey:     "/key.pem",
		ClientCert:     "/cert.pem",
		WorkingDir:     "/work",
		LaunchDir:      "/launch",
		CloudMode:      false,
		ExecutionVault: true,
	}
	c := a
	c.ExecutionVault = false

	require.True(t, a == b, "structs with identical fields should be equal")
	require.False(t, a == c, "structs differing in any field should not be equal")
}

func TestServeOperatorOptions_PartialAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:   "info",
		Endpoint:   "10.0.0.1",
		PrivateKey: "/tmp/key.pem",
	}

	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "10.0.0.1", opts.Endpoint)
	assert.Equal(t, "/tmp/key.pem", opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert, "unassigned ClientCert should be zero value")
	assert.Equal(t, "", opts.WorkingDir, "unassigned WorkingDir should be zero value")
	assert.Equal(t, "", opts.LaunchDir, "unassigned LaunchDir should be zero value")
	assert.False(t, opts.CloudMode, "unassigned CloudMode should be false")
	assert.False(t, opts.ExecutionVault, "unassigned ExecutionVault should be false")
	assert.False(t, opts.NoGit, "unassigned NoGit should be false")
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval, "unassigned HeartbeatInterval should be zero")
}

func TestServeOperatorOptions_HeartbeatInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"one second", 1 * time.Second, 1 * time.Second},
		{"thirty seconds", 30 * time.Second, 30 * time.Second},
		{"one minute", time.Minute, 60 * time.Second},
		{"five minutes", 5 * time.Minute, 300 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServeOperatorOptions{HeartbeatInterval: tt.duration}
			assert.Equal(t, tt.expected, opts.HeartbeatInterval)
		})
	}
}

func TestServeOperatorOptions_CloudProviderValues(t *testing.T) {
	providers := []string{"aws", "gcp", "azure", ""}

	for _, p := range providers {
		opts := ServeOperatorOptions{CloudMode: true, CloudProvider: p}
		assert.Equal(t, p, opts.CloudProvider)
		assert.True(t, opts.CloudMode)
	}
}

func TestResolveOperatorEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string returns default", "", "localhost"},
		{"whitespace only returns default", "   ", "localhost"},
		{"tab only returns default", "\t", "localhost"},
		{"simple hostname", "localhost", "localhost"},
		{"ip address", "192.168.1.10", "192.168.1.10"},
		{"hostname with port", "gateway.local:8080", "gateway.local:8080"},
		{"leading whitespace trimmed", "  localhost", "localhost"},
		{"trailing whitespace trimmed", "localhost  ", "localhost"},
		{"surrounding whitespace trimmed", "  10.0.0.1  ", "10.0.0.1"},
		{"fqdn", "operator.example.com", "operator.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOperatorEndpoint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveOperatorEndpoint_DefaultConstant(t *testing.T) {
	result := resolveOperatorEndpoint("")
	assert.Equal(t, "localhost", result, "default endpoint should be localhost")
}

func TestResolveOperatorEndpoint_NoTrimmingForNonWhitespace(t *testing.T) {
	result := resolveOperatorEndpoint("my-endpoint")
	assert.Equal(t, "my-endpoint", result)
}

func TestResolveWorkingDir(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"working dir set, launch dir set", "/var/work", "/opt/launch", "/var/work"},
		{"working dir set, launch dir empty", "/var/work", "", "/var/work"},
		{"working dir empty, launch dir set", "", "/opt/launch", "/opt/launch"},
		{"both empty", "", "", ""},
		{"working dir takes precedence", "/custom", "/default", "/custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkingDir_WorkingDirPrecedence(t *testing.T) {
	workingDir := "/explicit/working"
	launchDir := "/explicit/launch"

	result := resolveWorkingDir(workingDir, launchDir)

	assert.Equal(t, workingDir, result, "WorkingDir should take precedence over LaunchDir")
	assert.NotEqual(t, launchDir, result, "result should not be the LaunchDir when WorkingDir is set")
}

func TestResolveWorkingDir_LaunchDirFallback(t *testing.T) {
	launchDir := "/fallback/launch"

	result := resolveWorkingDir("", launchDir)

	assert.Equal(t, launchDir, result, "should fall back to LaunchDir when WorkingDir is empty")
}

func TestResolveWorkingDir_BothEmpty(t *testing.T) {
	result := resolveWorkingDir("", "")
	assert.Equal(t, "", result)
}
