// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(testutil.TempDir(t), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestPrepare_EveryRegisteredAgentWritesAndVerifies(t *testing.T) {
	for _, integration := range All() {
		t.Run(string(integration.ID), func(t *testing.T) {
			prepared, err := integration.Prepare(testutil.TempDir(t), "/fake/g8e", string(integration.ID), true)
			require.NoError(t, err)
			t.Cleanup(prepared.Cleanup)

			assert.FileExists(t, prepared.ConfigPath)
			data, err := os.ReadFile(prepared.ConfigPath)
			require.NoError(t, err)
			assert.Contains(t, string(data), "/fake/g8e", "config must point at the g8e stdio bridge")
			assert.Contains(t, string(data), string(integration.ID), "config must select credentials by app name")

			if integration.LaunchStrategy == LaunchMCPConfigFlags {
				assert.Contains(t, prepared.LaunchArgs, "--strict-mcp-config")
				assert.Contains(t, prepared.LaunchArgs, "--disallowed-tools")
			}
		})
	}
}

func TestVerifyAllIsolated_VerifiesEveryRegisteredAgentWithoutTouchingRealConfig(t *testing.T) {
	require.NoError(t, VerifyAllIsolated("/fake/g8e"))
}

func TestVerifyIsolated_ReportsTheAgentThatFailsVerification(t *testing.T) {
	integration := All()[0]
	integration.VerifyHooks = []VerifyHook{func(VerifyInput) error { return verifyFailure("lockdown drift") }}

	err := integration.VerifyIsolated("/fake/g8e")

	require.ErrorIs(t, err, constants.ErrToolInterceptionVerification)
	assert.Contains(t, err.Error(), string(integration.ID))
}

func TestPrepare_VerifyFalseSkipsHooks(t *testing.T) {
	broken := Integration{
		ID:             "broken",
		ConfigStrategy: ConfigTempJSON,
		LaunchStrategy: LaunchConfigOnly,
		VerifyHooks:    []VerifyHook{verifyStrictLaunchFlags},
	}

	prepared, err := broken.Prepare(testutil.TempDir(t), "/fake/g8e", "broken", false)
	require.NoError(t, err)
	prepared.Cleanup()

	_, err = broken.Prepare(testutil.TempDir(t), "/fake/g8e", "broken", true)
	require.ErrorIs(t, err, constants.ErrToolInterceptionVerification)
}

func TestPrepare_FailureCleansUpTempConfig(t *testing.T) {
	var seen string
	broken := Integration{
		ID:             "broken",
		ConfigStrategy: ConfigTempJSON,
		LaunchStrategy: LaunchConfigOnly,
		VerifyHooks: []VerifyHook{func(in VerifyInput) error {
			seen = in.ConfigPath
			return verifyFailure("always fails")
		}},
	}

	_, err := broken.Prepare(testutil.TempDir(t), "/fake/g8e", "broken", true)
	require.ErrorIs(t, err, constants.ErrToolInterceptionVerification)
	require.NotEmpty(t, seen)
	_, statErr := os.Stat(seen)
	assert.True(t, os.IsNotExist(statErr), "a rejected launch must not leave its temp config behind")
}

func TestPrepare_UnknownStrategyFailsClosed(t *testing.T) {
	_, err := Integration{ID: "bogus", ConfigStrategy: "bogus"}.Prepare(testutil.TempDir(t), "/fake/g8e", "bogus", true)
	require.ErrorIs(t, err, constants.ErrAgentNotSupported)
}

func TestVerifyHooks(t *testing.T) {
	const g8eServerJSON = `{"mcpServers":{"g8e":{"command":"g8e","args":["mcp","stdio"]}}}`
	strict := []string{"--mcp-config", "x", "--strict-mcp-config", "--disallowed-tools", "Bash"}
	gooseArgs := []string{"session", "--no-profile", "--with-extension", "/fake/g8e mcp stdio"}

	tests := []struct {
		name    string
		hook    VerifyHook
		in      func(t *testing.T) VerifyInput
		wantErr string // empty means pass
	}{
		{
			name: "mcp servers json: valid",
			hook: verifyMCPServersJSON,
			in:   func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "c.json", g8eServerJSON)} },
		},
		{
			name:    "mcp servers json: missing file",
			hook:    verifyMCPServersJSON,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: filepath.Join(testutil.TempDir(t), "nope.json")} },
			wantErr: "read mcp config",
		},
		{
			name:    "mcp servers json: invalid json",
			hook:    verifyMCPServersJSON,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "c.json", `{invalid`)} },
			wantErr: "parse mcp config",
		},
		{
			name:    "mcp servers json: no g8e server",
			hook:    verifyMCPServersJSON,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "c.json", `{"mcpServers":{}}`)} },
			wantErr: "missing g8e server entry",
		},
		{
			name: "strict flags: present",
			hook: verifyStrictLaunchFlags,
			in:   func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: strict} },
		},
		{
			name:    "strict flags: missing --strict-mcp-config",
			hook:    verifyStrictLaunchFlags,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: []string{"--disallowed-tools", "Bash"}} },
			wantErr: "missing --strict-mcp-config",
		},
		{
			name:    "strict flags: --disallowed-tools without a value",
			hook:    verifyStrictLaunchFlags,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: []string{"--strict-mcp-config", "--disallowed-tools"}} },
			wantErr: "missing --disallowed-tools",
		},
		{
			name: "goose flags: present",
			hook: verifyGooseLaunchFlags,
			in:   func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: gooseArgs} },
		},
		{
			name:    "goose flags: missing --no-profile",
			hook:    verifyGooseLaunchFlags,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: []string{"session", "--with-extension", "x"}} },
			wantErr: "missing --no-profile",
		},
		{
			name:    "goose flags: missing --with-extension",
			hook:    verifyGooseLaunchFlags,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{LaunchArgs: []string{"session", "--no-profile"}} },
			wantErr: "missing --with-extension",
		},
		{
			name: "goose extension: valid",
			hook: verifyGooseExtensionEntry,
			in: func(t *testing.T) VerifyInput {
				return VerifyInput{ConfigPath: writeFile(t, "config.yaml", "extensions:\n  g8e:\n    enabled: true\n")}
			},
		},
		{
			name:    "goose extension: no g8e entry",
			hook:    verifyGooseExtensionEntry,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "config.yaml", "extensions: {}\n")} },
			wantErr: "missing g8e extension entry",
		},
		{
			name:    "goose extension: missing file",
			hook:    verifyGooseExtensionEntry,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: filepath.Join(testutil.TempDir(t), "nope.yaml")} },
			wantErr: "read goose config",
		},
		{
			name: "gemini tools.core: empty array passes",
			hook: verifyGeminiToolsCoreEmpty,
			in:   func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "s.json", `{"tools":{"core":[]}}`)} },
		},
		{
			name:    "gemini tools.core: tools missing",
			hook:    verifyGeminiToolsCoreEmpty,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "s.json", g8eServerJSON)} },
			wantErr: "missing tools.core",
		},
		{
			name:    "gemini tools.core: null",
			hook:    verifyGeminiToolsCoreEmpty,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "s.json", `{"tools":{"exclude":["read_file"]}}`)} },
			wantErr: "tools.core is null",
		},
		{
			name:    "gemini tools.core: entries leave native tools enabled",
			hook:    verifyGeminiToolsCoreEmpty,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "s.json", `{"tools":{"core":["read_file","write_file"]}}`)} },
			wantErr: "tools.core has 2 entries",
		},
		{
			name:    "gemini tools.core: invalid json",
			hook:    verifyGeminiToolsCoreEmpty,
			in:      func(t *testing.T) VerifyInput { return VerifyInput{ConfigPath: writeFile(t, "s.json", `{invalid`)} },
			wantErr: "parse gemini settings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hook(tt.in(t))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, constants.ErrToolInterceptionVerification)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
