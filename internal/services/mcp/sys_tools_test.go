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

package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSysEnvVarsTool_Name(t *testing.T) {
	tool := &SysEnvVarsTool{}
	require.Equal(t, "sys_env_vars", tool.Name())
}

func TestSysEnvVarsTool_Description(t *testing.T) {
	tool := &SysEnvVarsTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "environment")
}

func TestSysEnvVarsTool_InputSchema(t *testing.T) {
	tool := &SysEnvVarsTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	patternProp, ok := schema.Properties["pattern"]
	require.True(t, ok)
	require.Equal(t, "string", patternProp.Type)

	redactProp, ok := schema.Properties["redact_secrets"]
	require.True(t, ok)
	require.Equal(t, "boolean", redactProp.Type)
}

func TestSysInfoTool_Name(t *testing.T) {
	tool := &SysInfoTool{}
	require.Equal(t, "sys_info", tool.Name())
}

func TestSysInfoTool_Description(t *testing.T) {
	tool := &SysInfoTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "system")
}

func TestSysInfoTool_InputSchema(t *testing.T) {
	tool := &SysInfoTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)
}

func TestSysServiceStatusTool_Name(t *testing.T) {
	tool := &SysServiceStatusTool{}
	require.Equal(t, "sys_service_status", tool.Name())
}

func TestSysServiceStatusTool_Description(t *testing.T) {
	tool := &SysServiceStatusTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "service")
}

func TestSysServiceStatusTool_InputSchema(t *testing.T) {
	tool := &SysServiceStatusTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	require.Contains(t, schema.Required, "service_name")

	serviceNameProp, ok := schema.Properties["service_name"]
	require.True(t, ok)
	require.Equal(t, "string", serviceNameProp.Type)
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		pattern string
		want    bool
		wantErr bool
	}{
		{
			name:    "empty pattern matches all",
			key:     "TEST_VAR",
			pattern: "",
			want:    true,
		},
		{
			name:    "exact match case insensitive",
			key:     "TEST_VAR",
			pattern: "test_var",
			want:    true,
		},
		{
			name:    "exact match case sensitive",
			key:     "TEST_VAR",
			pattern: "TEST_VAR",
			want:    true,
		},
		{
			name:    "wildcard prefix",
			key:     "G8E_TEST",
			pattern: "G8E_*",
			want:    true,
		},
		{
			name:    "wildcard suffix",
			key:     "TEST_G8E",
			pattern: "*_G8E",
			want:    true,
		},
		{
			name:    "wildcard both sides",
			key:     "PREFIX_G8E_SUFFIX",
			pattern: "PREFIX_*_SUFFIX",
			want:    true,
		},
		{
			name:    "no match",
			key:     "OTHER_VAR",
			pattern: "G8E_*",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchPattern(tt.key, tt.pattern)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRedactEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{
			name:  "password key redacted",
			key:   "DB_PASSWORD",
			value: "secret123",
			want:  "REDACTED",
		},
		{
			name:  "secret key redacted",
			key:   "API_SECRET",
			value: "abc123",
			want:  "REDACTED",
		},
		{
			name:  "token key redacted",
			key:   "AUTH_TOKEN",
			value: "xyz789",
			want:  "REDACTED",
		},
		{
			name:  "key keyword redacted",
			key:   "PRIVATE_KEY",
			value: "pemdata",
			want:  "REDACTED",
		},
		{
			name:  "cert keyword redacted",
			key:   "TLS_CERTIFICATE",
			value: "certdata",
			want:  "REDACTED",
		},
		{
			name:  "non-sensitive key not redacted",
			key:   "PATH",
			value: "/usr/bin",
			want:  "/usr/bin",
		},
		{
			name:  "empty value not redacted",
			key:   "DB_PASSWORD",
			value: "",
			want:  "",
		},
		{
			name:  "case insensitive keyword",
			key:   "api_key",
			value: "key123",
			want:  "REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactEnvValue(tt.key, tt.value)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetProp(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]string
		key      string
		expected string
	}{
		{
			name:     "key exists",
			props:    map[string]string{"key1": "value1"},
			key:      "key1",
			expected: "value1",
		},
		{
			name:     "key missing",
			props:    map[string]string{"key1": "value1"},
			key:      "key2",
			expected: "unknown",
		},
		{
			name:     "empty map",
			props:    map[string]string{},
			key:      "key1",
			expected: "unknown",
		},
		{
			name:     "empty key",
			props:    map[string]string{"": "value"},
			key:      "",
			expected: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getProp(tt.props, tt.key)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestCloudMetadataTool_Name(t *testing.T) {
	tool := &CloudMetadataTool{}
	require.Equal(t, "cloud_metadata", tool.Name())
}

func TestCloudMetadataTool_Description(t *testing.T) {
	tool := &CloudMetadataTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "cloud")
}

func TestCloudMetadataTool_InputSchema(t *testing.T) {
	tool := &CloudMetadataTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	operationProp, ok := schema.Properties["operation"]
	require.True(t, ok)
	require.Equal(t, "string", operationProp.Type)
	require.Contains(t, operationProp.Enum, "detect")
	require.Contains(t, operationProp.Enum, "instance")
	require.Contains(t, operationProp.Enum, "region")
}

func TestConfigDiffMaskTool_Name(t *testing.T) {
	tool := &ConfigDiffMaskTool{}
	require.Equal(t, "config_diff_mask", tool.Name())
}

func TestConfigDiffMaskTool_Description(t *testing.T) {
	tool := &ConfigDiffMaskTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "config")
}

func TestConfigDiffMaskTool_InputSchema(t *testing.T) {
	tool := &ConfigDiffMaskTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)
}

func TestDBDiscoverTopologyTool_Name(t *testing.T) {
	tool := &DBDiscoverTopologyTool{}
	require.Equal(t, "db_discover_topology", tool.Name())
}

func TestDBDiscoverTopologyTool_Description(t *testing.T) {
	tool := &DBDiscoverTopologyTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "database")
}

func TestDBDiscoverTopologyTool_InputSchema(t *testing.T) {
	tool := &DBDiscoverTopologyTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)
}
