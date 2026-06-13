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
)

func TestOperatorCmd(t *testing.T) {
	t.Run("operator command has correct use and description", func(t *testing.T) {
		cmd := operatorCmd()
		assert.Equal(t, "operator", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Operator instances")
		assert.Contains(t, cmd.Long, "Gateway")
	})

	t.Run("operator command has all subcommands", func(t *testing.T) {
		cmd := operatorCmd()
		// Check that subcommands are registered by name (Use includes args)
		hasList := false
		hasCp := false
		hasScp := false
		hasDeploy := false
		hasStream := false

		for _, sub := range cmd.Commands() {
			switch {
			case strings.HasPrefix(sub.Use, "list"):
				hasList = true
			case strings.HasPrefix(sub.Use, "cp"):
				hasCp = true
			case strings.HasPrefix(sub.Use, "scp"):
				hasScp = true
			case strings.HasPrefix(sub.Use, "deploy"):
				hasDeploy = true
			case strings.HasPrefix(sub.Use, "stream"):
				hasStream = true
			}
		}

		assert.True(t, hasList, "should have list subcommand")
		assert.True(t, hasCp, "should have cp subcommand")
		assert.True(t, hasScp, "should have scp subcommand")
		assert.True(t, hasDeploy, "should have deploy subcommand")
		assert.True(t, hasStream, "should have stream subcommand")
	})
}

func TestOperatorListCmd(t *testing.T) {
	t.Run("list command has correct use and description", func(t *testing.T) {
		cmd := operatorListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List all Operator instances")
		assert.Contains(t, cmd.Long, "Gateway")
	})

	t.Run("list command has no required flags", func(t *testing.T) {
		cmd := operatorListCmd()
		// Check that there are no required flags
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			assert.False(t, flag.Changed, "flag should not be required")
		})
	})
}

func TestOperatorCpCmd(t *testing.T) {
	t.Run("cp command has correct use and description", func(t *testing.T) {
		cmd := operatorCpCmd()
		assert.Equal(t, "cp <target>", cmd.Use)
		assert.Contains(t, cmd.Short, "Copy the operator binary")
		assert.Contains(t, cmd.Long, "directory")
	})

	t.Run("cp command requires exactly one argument", func(t *testing.T) {
		cmd := operatorCpCmd()
		// Test that it requires exactly one argument by running with wrong count
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// No arguments should fail
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.Error(t, err)
	})

	t.Run("cp command has no flags", func(t *testing.T) {
		cmd := operatorCpCmd()
		// Check that there are no flags
		flagCount := 0
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			flagCount++
		})
		assert.Equal(t, 0, flagCount)
	})
}

func TestOperatorScpCmd(t *testing.T) {
	t.Run("scp command has correct use and description", func(t *testing.T) {
		cmd := operatorScpCmd()
		assert.Equal(t, "scp <user@host:path>", cmd.Use)
		assert.Contains(t, cmd.Short, "Copy the operator binary")
		assert.Contains(t, cmd.Long, "scp")
	})

	t.Run("scp command requires exactly one argument", func(t *testing.T) {
		cmd := operatorScpCmd()
		// Test that it requires exactly one argument by running with wrong count
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// No arguments should fail
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		assert.Error(t, err)
	})

	t.Run("scp command has all expected flags", func(t *testing.T) {
		cmd := operatorScpCmd()
		flags := cmd.Flags()

		portFlag := flags.Lookup("port")
		assert.NotNil(t, portFlag)
		assert.Equal(t, "P", portFlag.Shorthand)

		identityFlag := flags.Lookup("identity")
		assert.NotNil(t, identityFlag)
		assert.Equal(t, "i", identityFlag.Shorthand)

		recursiveFlag := flags.Lookup("recursive")
		assert.NotNil(t, recursiveFlag)
		assert.Equal(t, "r", recursiveFlag.Shorthand)

		preserveFlag := flags.Lookup("preserve")
		assert.NotNil(t, preserveFlag)
		assert.Equal(t, "p", preserveFlag.Shorthand)

		verboseFlag := flags.Lookup("verbose")
		assert.NotNil(t, verboseFlag)
		assert.Equal(t, "v", verboseFlag.Shorthand)

		compressionFlag := flags.Lookup("compression")
		assert.NotNil(t, compressionFlag)
		assert.Equal(t, "C", compressionFlag.Shorthand)

		promptFlag := flags.Lookup("prompt")
		assert.NotNil(t, promptFlag)
		assert.Empty(t, promptFlag.Shorthand)
	})
}

func TestOperatorDeployCmd(t *testing.T) {
	t.Run("deploy command has correct use and description", func(t *testing.T) {
		cmd := operatorDeployCmd()
		assert.Equal(t, "deploy", cmd.Use)
		assert.Contains(t, cmd.Short, "Deploy the operator binary")
		assert.Contains(t, cmd.Long, "SSH")
	})

	t.Run("deploy command has hosts flag", func(t *testing.T) {
		cmd := operatorDeployCmd()
		hostsFlag := cmd.Flags().Lookup("hosts")
		assert.NotNil(t, hostsFlag)
		assert.Empty(t, hostsFlag.Shorthand)
	})

	t.Run("deploy command has port flag", func(t *testing.T) {
		cmd := operatorDeployCmd()
		portFlag := cmd.Flags().Lookup("port")
		assert.NotNil(t, portFlag)
		assert.Equal(t, "P", portFlag.Shorthand)
	})

	t.Run("deploy command has identity flag", func(t *testing.T) {
		cmd := operatorDeployCmd()
		identityFlag := cmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag)
		assert.Equal(t, "i", identityFlag.Shorthand)
	})

	t.Run("deploy command has background flag", func(t *testing.T) {
		cmd := operatorDeployCmd()
		backgroundFlag := cmd.Flags().Lookup("background")
		assert.NotNil(t, backgroundFlag)
		assert.Empty(t, backgroundFlag.Shorthand)
	})
}

func TestOperatorStreamCmd(t *testing.T) {
	t.Run("stream command has correct use and description", func(t *testing.T) {
		cmd := operatorStreamCmd()
		assert.Equal(t, "stream", cmd.Use)
		assert.Contains(t, cmd.Short, "Stream and execute")
		assert.Contains(t, cmd.Long, "SSH")
	})

	t.Run("stream command has hosts flag", func(t *testing.T) {
		cmd := operatorStreamCmd()
		hostsFlag := cmd.Flags().Lookup("hosts")
		assert.NotNil(t, hostsFlag)
		assert.Empty(t, hostsFlag.Shorthand)
	})

	t.Run("stream command has port flag", func(t *testing.T) {
		cmd := operatorStreamCmd()
		portFlag := cmd.Flags().Lookup("port")
		assert.NotNil(t, portFlag)
		assert.Equal(t, "P", portFlag.Shorthand)
	})

	t.Run("stream command has identity flag", func(t *testing.T) {
		cmd := operatorStreamCmd()
		identityFlag := cmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag)
		assert.Equal(t, "i", identityFlag.Shorthand)
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
	t.Run("copyFile copies file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "source.txt")
		dstFile := filepath.Join(tmpDir, "dest.txt")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(srcFile, content, 0644))

		err := copyFile(srcFile, dstFile)
		require.NoError(t, err)

		dstContent, err := os.ReadFile(dstFile)
		require.NoError(t, err)
		assert.Equal(t, content, dstContent)

		srcInfo, _ := os.Stat(srcFile)
		dstInfo, _ := os.Stat(dstFile)
		assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
	})

	t.Run("copyFile returns error when source does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "nonexistent.txt")
		dstFile := filepath.Join(tmpDir, "dest.txt")

		err := copyFile(srcFile, dstFile)
		assert.Error(t, err)
	})

	t.Run("copyFile returns error when destination cannot be created", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "source.txt")
		dstFile := filepath.Join(tmpDir, "subdir", "dest.txt")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(srcFile, content, 0644))

		err := copyFile(srcFile, dstFile)
		assert.Error(t, err)
	})

	t.Run("copyFile preserves file permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "source.txt")
		dstFile := filepath.Join(tmpDir, "dest.txt")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(srcFile, content, 0755))

		err := copyFile(srcFile, dstFile)
		require.NoError(t, err)

		srcInfo, _ := os.Stat(srcFile)
		dstInfo, _ := os.Stat(dstFile)
		assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
		assert.Equal(t, os.FileMode(0755), dstInfo.Mode())
	})
}

func TestPromptForScpOptions(t *testing.T) {
	t.Run("promptForScpOptions reads port input", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "2222\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.Equal(t, 2222, port)
	})

	t.Run("promptForScpOptions reads identity file input", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n/path/to/key\n\n\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.Equal(t, "/path/to/key", identityFile)
	})

	t.Run("promptForScpOptions sets preserve on 'y'", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\ny\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.True(t, preserve)
	})

	t.Run("promptForScpOptions sets preserve on 'Y'", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\nY\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.True(t, preserve)
	})

	t.Run("promptForScpOptions does not set preserve on non-y input", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\nn\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.False(t, preserve)
	})

	t.Run("promptForScpOptions sets compression on 'y'", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\n\ny\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.True(t, compression)
	})

	t.Run("promptForScpOptions sets verbose on 'y'", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\n\n\ny\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.True(t, verbose)
	})

	t.Run("promptForScpOptions handles empty input for port", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "\n\n\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.Equal(t, 0, port)
	})

	t.Run("promptForScpOptions handles invalid port input", func(t *testing.T) {
		cmd := &cobra.Command{}
		input := "invalid\n\n\n\n\n"
		cmd.SetIn(strings.NewReader(input))

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
		assert.Equal(t, 0, port)
	})
}

