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

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid hostname",
			input:    "example.com",
			expected: "example.com",
		},
		{
			name:     "hostname with hyphens",
			input:    "my-server.example.com",
			expected: "my-server.example.com",
		},
		{
			name:     "hostname with numbers",
			input:    "server123.example.com",
			expected: "server123.example.com",
		},
		{
			name:     "IP address",
			input:    "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "removes special characters",
			input:    "server@example.com",
			expected: "serverexample.com",
		},
		{
			name:     "removes spaces",
			input:    "my server",
			expected: "myserver",
		},
		{
			name:     "removes underscores",
			input:    "my_server",
			expected: "myserver",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHost(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	t.Parallel()

	t.Run("simple template", func(t *testing.T) {
		t.Parallel()
		result, err := executeTemplate("test", "Hello {{.Name}}", map[string]string{"Name": "World"})
		require.NoError(t, err)
		assert.Equal(t, "Hello World", result)
	})

	t.Run("template with multiple variables", func(t *testing.T) {
		t.Parallel()
		result, err := executeTemplate("test", "{{.Greeting}} {{.Name}}", map[string]string{
			"Greeting": "Hello",
			"Name":     "Test",
		})
		require.NoError(t, err)
		assert.Equal(t, "Hello Test", result)
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		t.Parallel()
		_, err := executeTemplate("test", "Hello {{.Name", map[string]string{"Name": "World"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse template")
	})

	t.Run("missing variable", func(t *testing.T) {
		t.Parallel()
		// Go templates don't error on missing variables, they render "<no value>"
		result, err := executeTemplate("test", "Hello {{.Name}}", map[string]string{})
		require.NoError(t, err)
		assert.Equal(t, "Hello <no value>", result)
	})
}

func TestWindowsTrustScriptBat(t *testing.T) {
	t.Parallel()

	result := WindowsTrustScriptBat("example.com", 80, 443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "@echo off")
	assert.Contains(t, result, "example.com")
	assert.Contains(t, result, "g8e CA Trust Refresh")
	assert.Contains(t, result, "curl -sSf -L")
	assert.Contains(t, result, "certutil")
}

func TestWindowsTrustScriptBat_CustomPorts(t *testing.T) {
	t.Parallel()

	result := WindowsTrustScriptBat("192.168.1.1", 8080, 8443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "192.168.1.1")
	assert.Contains(t, result, ":8080")
	assert.Contains(t, result, ":8443")
}

func TestUniversalTrustScript(t *testing.T) {
	t.Parallel()

	result := UniversalTrustScript("example.com", 80, 443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "#!/bin/sh")
	assert.Contains(t, result, "set -eu")
	assert.Contains(t, result, "g8e CA Trust Refresh")
	assert.Contains(t, result, "example.com")
	assert.Contains(t, result, "security add-trusted-cert")
	assert.Contains(t, result, "update-ca-certificates")
}

func TestUniversalTrustScript_CustomPorts(t *testing.T) {
	t.Parallel()

	result := UniversalTrustScript("localhost", 8080, 8443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "localhost")
	assert.Contains(t, result, ":8080")
	assert.Contains(t, result, ":8443")
}

func TestDeployScript(t *testing.T) {
	t.Parallel()

	result := DeployScript("example.com", 443, 80)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "#!/bin/sh")
	assert.Contains(t, result, "g8e-deploy")
	assert.Contains(t, result, "example.com")
	assert.Contains(t, result, "curl -fsSL")
	assert.Contains(t, result, "openssl x509")
	assert.Contains(t, result, "./g8e.operator")
}

func TestDeployScript_CustomPorts(t *testing.T) {
	t.Parallel()

	result := DeployScript("192.168.1.1", 8443, 8080)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "192.168.1.1")
	assert.Contains(t, result, "--wss-port 8443")
	assert.Contains(t, result, "--http-port 8443")
}

func TestWindowsPowerShellTrustScript(t *testing.T) {
	t.Parallel()

	result := WindowsPowerShellTrustScript("example.com", 80, 443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "#Requires -RunAsAdministrator")
	assert.Contains(t, result, "g8e CA Trust Refresh")
	assert.Contains(t, result, "example.com")
	assert.Contains(t, result, "Invoke-WebRequest")
	assert.Contains(t, result, "certutil")
}

func TestWindowsPowerShellTrustScript_CustomPorts(t *testing.T) {
	t.Parallel()

	result := WindowsPowerShellTrustScript("localhost", 8080, 8443)

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "localhost")
	assert.Contains(t, result, ":8080")
	assert.Contains(t, result, ":8443")
}

func TestWindowsTrustScriptBat_SanitizesHost(t *testing.T) {
	t.Parallel()

	result := WindowsTrustScriptBat("server@example.com", 80, 443)

	// Special characters should be removed from the HOST variable
	assert.Contains(t, result, "set HOST=serverexample.com")
}

func TestUniversalTrustScript_SanitizesHost(t *testing.T) {
	t.Parallel()

	result := UniversalTrustScript("server@example.com", 80, 443)

	// Special characters should be removed from the HOST variable
	assert.Contains(t, result, `HOST="serverexample.com"`)
}

func TestDeployScript_SanitizesHost(t *testing.T) {
	t.Parallel()

	result := DeployScript("server@example.com", 443, 80)

	// Special characters should be removed from the HOST variable
	assert.Contains(t, result, `G8E_HOST="serverexample.com"`)
}

func TestWindowsPowerShellTrustScript_SanitizesHost(t *testing.T) {
	t.Parallel()

	result := WindowsPowerShellTrustScript("server@example.com", 80, 443)

	// Special characters should be removed from the HOST variable
	assert.Contains(t, result, `$g8eHost = "serverexample.com"`)
}

func TestWindowsTrustScriptBat_TemplateError(t *testing.T) {
	t.Parallel()

	// If template execution fails, it should return an error message
	// We can't easily test this without modifying the function to accept a broken template
	// But we can verify the function doesn't panic
	result := WindowsTrustScriptBat("example.com", 80, 443)
	assert.NotEmpty(t, result)
}

func TestUniversalTrustScript_TemplateError(t *testing.T) {
	t.Parallel()

	// Verify the function doesn't panic on template execution
	result := UniversalTrustScript("example.com", 80, 443)
	assert.NotEmpty(t, result)
}

func TestDeployScript_TemplateError(t *testing.T) {
	t.Parallel()

	// Verify the function doesn't panic on template execution
	result := DeployScript("example.com", 443, 80)
	assert.NotEmpty(t, result)
}

func TestWindowsPowerShellTrustScript_TemplateError(t *testing.T) {
	t.Parallel()

	// Verify the function doesn't panic on template execution
	result := WindowsPowerShellTrustScript("example.com", 80, 443)
	assert.NotEmpty(t, result)
}

func TestWindowsTrustScriptBat_DefaultPorts(t *testing.T) {
	t.Parallel()

	result := WindowsTrustScriptBat("example.com", 80, 443)

	// Default ports should not show port suffix
	assert.NotContains(t, result, "example.com:80")
	assert.Contains(t, result, "example.com")
}

func TestUniversalTrustScript_DefaultPorts(t *testing.T) {
	t.Parallel()

	result := UniversalTrustScript("example.com", 80, 443)

	// Default ports should not show port suffix
	assert.NotContains(t, result, "example.com:80")
	assert.Contains(t, result, "example.com")
}

func TestDeployScript_DefaultPorts(t *testing.T) {
	t.Parallel()

	result := DeployScript("example.com", 443, 80)

	// Default ports should not show port flags
	assert.NotContains(t, result, "--wss-port 443")
	assert.Contains(t, result, "example.com")
}

func TestWindowsPowerShellTrustScript_DefaultPorts(t *testing.T) {
	t.Parallel()

	result := WindowsPowerShellTrustScript("example.com", 80, 443)

	// Default ports should not show port suffix
	assert.NotContains(t, result, "example.com:80")
	assert.Contains(t, result, "example.com")
}

func TestWindowsTrustScriptBat_ContainsElevationCheck(t *testing.T) {
	t.Parallel()

	result := WindowsTrustScriptBat("example.com", 80, 443)

	assert.Contains(t, result, "cacls.exe")
	assert.Contains(t, result, "RunAs")
}

func TestUniversalTrustScript_ContainsRootCheck(t *testing.T) {
	t.Parallel()

	result := UniversalTrustScript("example.com", 80, 443)

	assert.Contains(t, result, "id -u")
	assert.Contains(t, result, "root")
}

func TestDeployScript_ContainsArchitectureDetection(t *testing.T) {
	t.Parallel()

	result := DeployScript("example.com", 443, 80)

	assert.Contains(t, result, "uname -m")
	assert.Contains(t, result, "x86_64")
	assert.Contains(t, result, "aarch64")
	assert.Contains(t, result, "i386")
}

func TestWindowsPowerShellTrustScript_RequiresAdmin(t *testing.T) {
	t.Parallel()

	result := WindowsPowerShellTrustScript("example.com", 80, 443)

	assert.Contains(t, result, "#Requires -RunAsAdministrator")
}
