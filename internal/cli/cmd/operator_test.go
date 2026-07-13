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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestOperatorCmd(t *testing.T) {
	tests := []struct {
		name     string
		use      string
		short    string
		long     string
		expected bool
	}{
		{
			name:     "operator command has correct use and description",
			use:      "operator",
			short:    "Manage Operator instances",
			long:     "Gateway",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("operator command has all subcommands", func(t *testing.T) {
		cmd := operatorCmd()
		expectedSubcommands := map[string]bool{
			"list":   false,
			"cp":     false,
			"scp":    false,
			"deploy": false,
			"stream": false,
		}

		for _, sub := range cmd.Commands() {
			for name := range expectedSubcommands {
				if strings.HasPrefix(sub.Use, name) {
					expectedSubcommands[name] = true
				}
			}
		}

		for name, found := range expectedSubcommands {
			assert.True(t, found, "should have %s subcommand", name)
		}
	})
}

func TestOperatorListCmd(t *testing.T) {
	tests := []struct {
		name  string
		use   string
		short string
		long  string
	}{
		{
			name:  "list command has correct use and description",
			use:   "list",
			short: "List all Operator instances",
			long:  "Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorListCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("list command has no required flags", func(t *testing.T) {
		cmd := operatorListCmd()
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			assert.False(t, flag.Changed, "flag should not be required")
		})
	})
}

func TestOperatorCpCmd(t *testing.T) {
	tests := []struct {
		name  string
		use   string
		short string
		long  string
	}{
		{
			name:  "cp command has correct use and description",
			use:   "cp <target>",
			short: "Copy the operator binary",
			long:  "directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorCpCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("cp command requires exactly one argument", func(t *testing.T) {
		cmd := operatorCpCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.Error(t, err)
	})

	t.Run("cp command has no flags", func(t *testing.T) {
		cmd := operatorCpCmd()
		flagCount := 0
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			flagCount++
		})
		assert.Equal(t, 0, flagCount)
	})
}

func TestOperatorScpCmd(t *testing.T) {
	tests := []struct {
		name  string
		use   string
		short string
		long  string
	}{
		{
			name:  "scp command has correct use and description",
			use:   "scp <user@host:path>",
			short: "Copy the operator binary",
			long:  "scp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorScpCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("scp command requires exactly one argument", func(t *testing.T) {
		cmd := operatorScpCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.Error(t, err)
	})

	t.Run("scp command has all expected flags", func(t *testing.T) {
		cmd := operatorScpCmd()
		flags := cmd.Flags()

		expectedFlags := []struct {
			name      string
			shorthand string
		}{
			{"port", "P"},
			{"identity", "i"},
			{"recursive", "r"},
			{"preserve", ""},
			{"verbose", "v"},
			{"compression", "C"},
			{"prompt", ""},
		}

		for _, ef := range expectedFlags {
			flag := flags.Lookup(ef.name)
			assert.NotNil(t, flag, "flag %s should exist", ef.name)
			assert.Equal(t, ef.shorthand, flag.Shorthand, "flag %s shorthand should match", ef.name)
		}
	})
}

func TestOperatorDeployCmd(t *testing.T) {
	tests := []struct {
		name  string
		use   string
		short string
		long  string
	}{
		{
			name:  "deploy command has correct use and description",
			use:   "deploy",
			short: "Deploy the operator binary",
			long:  "SSH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorDeployCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("deploy command has all expected flags", func(t *testing.T) {
		cmd := operatorDeployCmd()
		flags := cmd.Flags()

		expectedFlags := []struct {
			name      string
			shorthand string
		}{
			{"hosts", ""},
			{"port", "P"},
			{"identity", "i"},
			{"background", ""},
		}

		for _, ef := range expectedFlags {
			flag := flags.Lookup(ef.name)
			assert.NotNil(t, flag, "flag %s should exist", ef.name)
			assert.Equal(t, ef.shorthand, flag.Shorthand, "flag %s shorthand should match", ef.name)
		}
	})
}

func TestOperatorStreamCmd(t *testing.T) {
	tests := []struct {
		name  string
		use   string
		short string
		long  string
	}{
		{
			name:  "stream command has correct use and description",
			use:   "stream [host...] [flags]",
			short: "Stream and execute",
			long:  "SSH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := operatorStreamCmd()
			assert.Equal(t, tt.use, cmd.Use)
			assert.Contains(t, cmd.Short, tt.short)
			assert.Contains(t, cmd.Long, tt.long)
		})
	}

	t.Run("stream command has all expected flags", func(t *testing.T) {
		cmd := operatorStreamCmd()
		flags := cmd.Flags()

		expectedFlags := []struct {
			name      string
			shorthand string
		}{
			{"hosts", ""},
			{"arch", ""},
			{"ssh-identity-file", ""},
		}

		for _, ef := range expectedFlags {
			flag := flags.Lookup(ef.name)
			assert.NotNil(t, flag, "flag %s should exist", ef.name)
			assert.Equal(t, ef.shorthand, flag.Shorthand, "flag %s shorthand should match", ef.name)
		}
	})
}

func TestBuildScpArgs(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		identity    string
		recursive   bool
		preserve    bool
		verbose     bool
		compression bool
		source      string
		target      string
		expected    []string
	}{
		{
			name:        "minimal args",
			port:        0,
			identity:    "",
			recursive:   false,
			preserve:    false,
			verbose:     false,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"/path/to/source", "user@host:/path"},
		},
		{
			name:        "with port",
			port:        2222,
			identity:    "",
			recursive:   false,
			preserve:    false,
			verbose:     false,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-P", "2222", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "with identity",
			port:        0,
			identity:    "/path/to/key",
			recursive:   false,
			preserve:    false,
			verbose:     false,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-i", "/path/to/key", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "with recursive",
			port:        0,
			identity:    "",
			recursive:   true,
			preserve:    false,
			verbose:     false,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-r", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "with preserve",
			port:        0,
			identity:    "",
			recursive:   false,
			preserve:    true,
			verbose:     false,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-p", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "with verbose",
			port:        0,
			identity:    "",
			recursive:   false,
			preserve:    false,
			verbose:     true,
			compression: false,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-v", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "with compression",
			port:        0,
			identity:    "",
			recursive:   false,
			preserve:    false,
			verbose:     false,
			compression: true,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-C", "/path/to/source", "user@host:/path"},
		},
		{
			name:        "all flags",
			port:        2222,
			identity:    "/path/to/key",
			recursive:   true,
			preserve:    true,
			verbose:     true,
			compression: true,
			source:      "/path/to/source",
			target:      "user@host:/path",
			expected:    []string{"-P", "2222", "-i", "/path/to/key", "-r", "-p", "-v", "-C", "/path/to/source", "user@host:/path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildScpArgs(tt.port, tt.identity, tt.recursive, tt.preserve, tt.verbose, tt.compression, tt.source, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCopyFile(t *testing.T) {
	tests := []struct {
		name       string
		srcContent []byte
		srcMode    os.FileMode
		dstPath    string
		setup      func(tmpDir string) (string, string)
		wantErr    bool
		errorCheck func(t *testing.T, err error)
		skipOn     string
	}{
		{
			name:       "copyFile copies file successfully",
			srcContent: []byte("test content"),
			srcMode:    0644,
			dstPath:    "dest.txt",
			setup: func(tmpDir string) (string, string) {
				srcFile := filepath.Join(tmpDir, "source.txt")
				dstFile := filepath.Join(tmpDir, "dest.txt")
				return srcFile, dstFile
			},
			wantErr: false,
		},
		{
			name:       "copyFile returns error when source does not exist",
			srcContent: nil,
			srcMode:    0,
			dstPath:    "dest.txt",
			setup: func(tmpDir string) (string, string) {
				srcFile := filepath.Join(tmpDir, "nonexistent.txt")
				dstFile := filepath.Join(tmpDir, "dest.txt")
				return srcFile, dstFile
			},
			wantErr: true,
		},
		{
			name:       "copyFile returns error when destination cannot be created",
			srcContent: []byte("test content"),
			srcMode:    0644,
			dstPath:    "subdir/dest.txt",
			setup: func(tmpDir string) (string, string) {
				srcFile := filepath.Join(tmpDir, "source.txt")
				dstFile := filepath.Join(tmpDir, "subdir", "dest.txt")
				return srcFile, dstFile
			},
			wantErr: true,
		},
		{
			name:       "copyFile preserves file permissions",
			srcContent: []byte("test content"),
			srcMode:    0755,
			dstPath:    "dest.txt",
			setup: func(tmpDir string) (string, string) {
				srcFile := filepath.Join(tmpDir, "source.txt")
				dstFile := filepath.Join(tmpDir, "dest.txt")
				return srcFile, dstFile
			},
			wantErr: false,
			skipOn:  "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOn != "" && os.Getenv("GOOS") == tt.skipOn {
				t.Skipf("Test skipped on %s", tt.skipOn)
			}

			tmpDir := testutil.TempDir(t)
			srcFile, dstFile := tt.setup(tmpDir)

			if tt.srcContent != nil {
				require.NoError(t, os.WriteFile(srcFile, tt.srcContent, tt.srcMode))
			}

			err := copyFile(srcFile, dstFile)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			dstContent, err := os.ReadFile(dstFile)
			require.NoError(t, err)
			assert.Equal(t, tt.srcContent, dstContent)

			srcInfo, _ := os.Stat(srcFile)
			dstInfo, _ := os.Stat(dstFile)
			assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
		})
	}
}

func TestPromptForScpOptions(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPort     int
		wantIdentity string
		wantPreserve bool
		wantVerbose  bool
		wantCompress bool
	}{
		{
			name:         "reads port input",
			input:        "2222\n",
			wantPort:     2222,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "reads identity file input",
			input:        "\n/path/to/key\n\n\n\n\n",
			wantPort:     0,
			wantIdentity: "/path/to/key",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "sets preserve on 'y'",
			input:        "\n\ny\n\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: true,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "sets preserve on 'Y'",
			input:        "\n\nY\n\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: true,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "does not set preserve on non-y input",
			input:        "\n\nn\n\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "sets compression on 'y'",
			input:        "\n\n\ny\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: true,
		},
		{
			name:         "sets verbose on 'y'",
			input:        "\n\n\n\ny\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  true,
			wantCompress: false,
		},
		{
			name:         "handles empty input for port",
			input:        "\n\n\n\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: false,
		},
		{
			name:         "handles invalid port input",
			input:        "invalid\n\n\n\n\n",
			wantPort:     0,
			wantIdentity: "",
			wantPreserve: false,
			wantVerbose:  false,
			wantCompress: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tt.input))

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			port := 0
			identityFile := ""
			recursive := false
			preserve := false
			verbose := false
			compression := false

			err := promptForScpOptions(cmd, &port, &identityFile, &recursive, &preserve, &verbose, &compression)
			require.NoError(t, err)

			assert.Equal(t, tt.wantPort, port)
			assert.Equal(t, tt.wantIdentity, identityFile)
			assert.Equal(t, tt.wantPreserve, preserve)
			assert.Equal(t, tt.wantVerbose, verbose)
			assert.Equal(t, tt.wantCompress, compression)
		})
	}
}
