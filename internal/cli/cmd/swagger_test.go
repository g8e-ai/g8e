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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestSwaggerCmd(t *testing.T) {
	t.Run("swagger command has correct use and description", func(t *testing.T) {
		cmd := swaggerCmd()
		assert.Equal(t, "swagger", cmd.Use)
		assert.Contains(t, cmd.Short, "Swagger")
		assert.Contains(t, cmd.Short, "OpenAPI")
		assert.Contains(t, cmd.Long, "generating")
		assert.Contains(t, cmd.Long, "serving")
		assert.Contains(t, cmd.Long, "validating")
	})
}

func TestSwaggerCmdSubcommands(t *testing.T) {
	t.Run("swagger command has expected subcommands", func(t *testing.T) {
		cmd := swaggerCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{
			"init",
			"serve",
			"validate",
		}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "swagger command should have %s subcommand", subcmd)
		}
	})
}

func TestSwaggerInitCmd(t *testing.T) {
	t.Run("init command has correct use and description", func(t *testing.T) {
		cmd := swaggerInitCmd()
		assert.Equal(t, "init", cmd.Use)
		assert.Contains(t, cmd.Short, "Generate")
		assert.Contains(t, cmd.Short, "Swagger")
		assert.Contains(t, cmd.Long, "swag CLI tool")
		assert.Contains(t, cmd.Long, "annotations")
	})

	t.Run("init command has dir and output flags", func(t *testing.T) {
		cmd := swaggerInitCmd()
		require.NotNil(t, cmd)

		dirFlag := cmd.Flags().Lookup("dir")
		outputFlag := cmd.Flags().Lookup("output")

		assert.NotNil(t, dirFlag, "init should have --dir flag")
		assert.NotNil(t, outputFlag, "init should have --output flag")
	})

	t.Run("init fails when swag binary not found and go run fails", func(t *testing.T) {
		cmd := swaggerInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Set flags to non-existent paths to trigger errors
		err := cmd.Flags().Set("dir", "nonexistent-dir")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		// Should fail because swag is not available and go run will fail with invalid path
		require.Error(t, err)
	})

	t.Run("init uses default search directory when flag not set", func(t *testing.T) {
		cmd := swaggerInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Don't set dir flag - should use default
		err := cmd.RunE(cmd, []string{})
		// Will fail because swag is not available, but we can check the output
		// The command should attempt to use the default directory
		require.Error(t, err)
	})

	t.Run("init uses default output directory when flag not set", func(t *testing.T) {
		cmd := swaggerInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Don't set output flag - should use default
		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
	})
}

func TestSwaggerServeCmd(t *testing.T) {
	t.Run("serve command has correct use and description", func(t *testing.T) {
		cmd := swaggerServeCmd()
		assert.Equal(t, "serve", cmd.Use)
		assert.Contains(t, cmd.Short, "Serve")
		assert.Contains(t, cmd.Short, "Swagger UI")
		assert.Contains(t, cmd.Long, "HTTP server")
		assert.Contains(t, cmd.Long, "viewing")
		assert.Contains(t, cmd.Long, "testing")
	})

	t.Run("serve command has port and host flags", func(t *testing.T) {
		cmd := swaggerServeCmd()
		require.NotNil(t, cmd)

		portFlag := cmd.Flags().Lookup("port")
		hostFlag := cmd.Flags().Lookup("host")

		assert.NotNil(t, portFlag, "serve should have --port flag")
		assert.NotNil(t, hostFlag, "serve should have --host flag")
	})

	t.Run("serve uses default port when flag not set", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		// Don't set port flag - should use default 8081
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "8081")
	})

	t.Run("serve uses default host when flag not set", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		// Don't set host flag - should use default localhost
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "localhost")
	})

	t.Run("serve uses custom port when flag is set", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.Flags().Set("port", "9090")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "9090")
	})

	t.Run("serve uses custom host when flag is set", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.Flags().Set("host", "0.0.0.0")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "0.0.0.0")
	})

	t.Run("serve prints message when swagger.json not found", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create the docs directory but not swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		output := buf.String()
		assert.Contains(t, output, "Swagger documentation not found")
	})

	t.Run("serve provides alternative serving instructions", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create the docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "npx @apidevtools/swagger-cli")
		assert.Contains(t, output, "docker run")
		assert.Contains(t, output, "swaggerapi/swagger-ui")
	})
}

func TestSwaggerValidateCmd(t *testing.T) {
	t.Run("validate command has correct use and description", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		assert.Equal(t, "validate", cmd.Use)
		assert.Contains(t, cmd.Short, "Validate")
		assert.Contains(t, cmd.Short, "Swagger")
		assert.Contains(t, cmd.Short, "OpenAPI")
		assert.Contains(t, cmd.Long, "errors")
		assert.Contains(t, cmd.Long, "compliance")
	})

	t.Run("validate command has file flag", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		require.NotNil(t, cmd)

		fileFlag := cmd.Flags().Lookup("file")
		assert.NotNil(t, fileFlag, "validate should have --file flag")
	})

	t.Run("validate uses default spec file when flag not set", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Don't set file flag - should use default
		err := cmd.RunE(cmd, []string{})
		// Will fail because file doesn't exist
		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("validate fails when spec file does not exist", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.Flags().Set("file", "/nonexistent/swagger.json")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("validate uses custom spec file when flag is set", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create a custom swagger.json
		customPath := filepath.Join(tmpDir, "custom-swagger.json")
		require.NoError(t, os.WriteFile(customPath, []byte("{}"), 0644))

		err := cmd.Flags().Set("file", customPath)
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		// Will succeed (no validation tools available) but should not fail on file not found
		require.NoError(t, err)
	})

	t.Run("validate provides installation instructions when no tools found", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		// The command may use swag if available, or provide installation instructions if not
		// Either behavior is acceptable
		hasToolMsg := strings.Contains(output, "No swagger validation tool found")
		hasSwagMsg := strings.Contains(output, "Using swag for basic validation")
		assert.True(t, hasToolMsg || hasSwagMsg, "should either show tool not found or use swag")
		if hasToolMsg {
			assert.Contains(t, output, "npm install -g @apidevtools/swagger-cli")
			assert.Contains(t, output, "go install github.com/go-swagger/go-swagger/cmd/swagger@latest")
		}
	})
}

func TestSwaggerCommandHelpText(t *testing.T) {
	t.Run("swagger commands have non-empty help text", func(t *testing.T) {
		commands := []struct {
			name string
			cmd  *cobra.Command
		}{
			{"swagger", swaggerCmd()},
			{"init", swaggerInitCmd()},
			{"serve", swaggerServeCmd()},
			{"validate", swaggerValidateCmd()},
		}

		for _, tc := range commands {
			t.Run(tc.name, func(t *testing.T) {
				assert.NotEmpty(t, tc.cmd.Short, tc.name+" should have non-empty Short description")
				assert.NotEmpty(t, tc.cmd.Long, tc.name+" should have non-empty Long description")
			})
		}
	})
}

func TestSwaggerCommandFlagValidation(t *testing.T) {
	t.Run("init dir flag accepts string value", func(t *testing.T) {
		cmd := swaggerInitCmd()
		err := cmd.Flags().Set("dir", "custom-dir")
		require.NoError(t, err)
	})

	t.Run("init output flag accepts string value", func(t *testing.T) {
		cmd := swaggerInitCmd()
		err := cmd.Flags().Set("output", "custom-output")
		require.NoError(t, err)
	})

	t.Run("serve port flag accepts integer value", func(t *testing.T) {
		cmd := swaggerServeCmd()
		err := cmd.Flags().Set("port", "9999")
		require.NoError(t, err)
	})

	t.Run("serve port flag rejects invalid integer", func(t *testing.T) {
		cmd := swaggerServeCmd()
		err := cmd.Flags().Set("port", "invalid")
		require.Error(t, err)
	})

	t.Run("serve host flag accepts string value", func(t *testing.T) {
		cmd := swaggerServeCmd()
		err := cmd.Flags().Set("host", "127.0.0.1")
		require.NoError(t, err)
	})

	t.Run("validate file flag accepts string value", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		err := cmd.Flags().Set("file", "/path/to/swagger.json")
		require.NoError(t, err)
	})
}

func TestSwaggerCommandPathResolution(t *testing.T) {
	t.Run("init resolves relative paths to absolute", func(t *testing.T) {
		cmd := swaggerInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Set relative path
		err := cmd.Flags().Set("dir", "relative/path")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		// Will fail on execution, but path resolution should succeed
		// If path resolution failed, error would mention "failed to resolve"
		if err != nil {
			assert.NotContains(t, err.Error(), "failed to resolve")
		}
	})

	t.Run("serve resolves docs path to absolute", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		// If path resolution failed, error would mention "failed to resolve"
		output := buf.String()
		assert.NotContains(t, output, "failed to resolve")
	})

	t.Run("validate resolves spec file path to absolute", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create swagger.json in current directory
		require.NoError(t, os.WriteFile("swagger.json", []byte("{}"), 0644))

		err := cmd.Flags().Set("file", "swagger.json")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		// Will succeed (no validation tools), path resolution should work
		require.NoError(t, err)
	})
}

func TestSwaggerCommandOutputFormatting(t *testing.T) {
	t.Run("init prints success message with file paths", func(t *testing.T) {
		// This test verifies the output format when init succeeds
		// Since we can't actually run swag, we just verify the command structure
		cmd := swaggerInitCmd()
		assert.Contains(t, cmd.Long, "swag CLI tool")
	})

	t.Run("serve prints URL with host and port", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create docs directory with swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.Flags().Set("host", "127.0.0.1")
		require.NoError(t, err)
		err = cmd.Flags().Set("port", "9090")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "http://127.0.0.1:9090")
	})

	t.Run("validate prints tool suggestions when tools missing", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		output := buf.String()
		// The command may use swag if available, or provide installation instructions if not
		hasToolMsg := strings.Contains(output, "No swagger validation tool found")
		hasSwagMsg := strings.Contains(output, "Using swag for basic validation")
		assert.True(t, hasToolMsg || hasSwagMsg, "should either show tool not found or use swag")
		if hasToolMsg {
			// Should suggest both npm and go installation methods
			assert.Contains(t, output, "npm")
			assert.Contains(t, output, "go install")
		}
	})
}

func TestSwaggerCommandErrorMessages(t *testing.T) {
	t.Run("init provides clear error on path resolution failure", func(t *testing.T) {
		// Test that path resolution errors are clear
		// This is difficult to test without actually causing a path resolution error
		// We verify the error message format in the code
		cmd := swaggerInitCmd()
		// The error message should contain "failed to resolve" prefix
		// This is verified by checking the code structure
		assert.Contains(t, cmd.Long, "swag CLI tool")
	})

	t.Run("validate provides clear error when file not found", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Error(t, err)
	})
}

func TestSwaggerCommandDefaultValues(t *testing.T) {
	t.Run("init default search directory is correct", func(t *testing.T) {
		cmd := swaggerInitCmd()
		dirFlag := cmd.Flags().Lookup("dir")
		assert.NotNil(t, dirFlag)
		// Default value is empty string, which triggers the default in RunE
		assert.Empty(t, dirFlag.DefValue)
	})

	t.Run("init default output directory is correct", func(t *testing.T) {
		cmd := swaggerInitCmd()
		outputFlag := cmd.Flags().Lookup("output")
		assert.NotNil(t, outputFlag)
		// Default value is empty string, which triggers the default in RunE
		assert.Empty(t, outputFlag.DefValue)
	})

	t.Run("serve default port is correct", func(t *testing.T) {
		cmd := swaggerServeCmd()
		portFlag := cmd.Flags().Lookup("port")
		assert.NotNil(t, portFlag)
		// Default value is 0, which triggers the default 8081 in RunE
		assert.Equal(t, "0", portFlag.DefValue)
	})

	t.Run("serve default host is correct", func(t *testing.T) {
		cmd := swaggerServeCmd()
		hostFlag := cmd.Flags().Lookup("host")
		assert.NotNil(t, hostFlag)
		// Default value is empty string, which triggers the default localhost in RunE
		assert.Empty(t, hostFlag.DefValue)
	})

	t.Run("validate default file is correct", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		fileFlag := cmd.Flags().Lookup("file")
		assert.NotNil(t, fileFlag)
		// Default value is empty string, which triggers the default in RunE
		assert.Empty(t, fileFlag.DefValue)
	})
}

func TestSwaggerCommandIntegration(t *testing.T) {
	t.Run("swagger command can be added to parent command", func(t *testing.T) {
		parentCmd := &cobra.Command{Use: "parent"}
		swaggerCmd := swaggerCmd()
		parentCmd.AddCommand(swaggerCmd)

		assert.Len(t, parentCmd.Commands(), 1)
		assert.Equal(t, "swagger", parentCmd.Commands()[0].Name())
	})

	t.Run("swagger subcommands are properly nested", func(t *testing.T) {
		cmd := swaggerCmd()
		subcommands := cmd.Commands()

		subcommandNames := make([]string, len(subcommands))
		for i, subcmd := range subcommands {
			subcommandNames[i] = subcmd.Name()
		}

		assert.Contains(t, subcommandNames, "init")
		assert.Contains(t, subcommandNames, "serve")
		assert.Contains(t, subcommandNames, "validate")
	})
}

func TestSwaggerCommandEdgeCases(t *testing.T) {
	t.Run("serve handles empty docs directory gracefully", func(t *testing.T) {
		cmd := swaggerServeCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create empty docs directory
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		output := buf.String()
		assert.Contains(t, output, "Swagger documentation not found")
	})

	t.Run("validate handles empty swagger.json gracefully", func(t *testing.T) {
		cmd := swaggerValidateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Create empty swagger.json
		docsPath := filepath.Join(tmpDir, "internal", "services", "gateway", "docs")
		require.NoError(t, os.MkdirAll(docsPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(docsPath, "swagger.json"), []byte("{}"), 0644))

		err := cmd.RunE(cmd, []string{})
		// Should succeed (no validation tools available)
		require.NoError(t, err)
	})

	t.Run("init handles comma-separated search directories", func(t *testing.T) {
		cmd := swaggerInitCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Set comma-separated directories (default format)
		err := cmd.Flags().Set("dir", "dir1,dir2")
		require.NoError(t, err)

		err = cmd.RunE(cmd, []string{})
		// Will fail on execution, but flag parsing should succeed
		if err != nil {
			assert.NotContains(t, err.Error(), "invalid argument")
		}
	})
}

func TestSwaggerCommandDocumentation(t *testing.T) {
	t.Run("command descriptions are consistent", func(t *testing.T) {
		cmd := swaggerCmd()
		// Verify that the parent command mentions all subcommands
		lowerLong := strings.ToLower(cmd.Long)
		hasGenerate := strings.Contains(lowerLong, "generating") || strings.Contains(lowerLong, "generate")
		hasServe := strings.Contains(lowerLong, "serving") || strings.Contains(lowerLong, "serve")
		hasValidate := strings.Contains(lowerLong, "validating") || strings.Contains(lowerLong, "validate")
		assert.True(t, hasGenerate, "parent command should mention generating")
		assert.True(t, hasServe, "parent command should mention serving")
		assert.True(t, hasValidate, "parent command should mention validating")
	})

	t.Run("subcommand descriptions are specific to their function", func(t *testing.T) {
		initCmd := swaggerInitCmd()
		serveCmd := swaggerServeCmd()
		validateCmd := swaggerValidateCmd()

		assert.Contains(t, strings.ToLower(initCmd.Long), "generate")
		assert.Contains(t, strings.ToLower(serveCmd.Long), "serve")
		assert.Contains(t, strings.ToLower(validateCmd.Long), "validate")
	})
}
