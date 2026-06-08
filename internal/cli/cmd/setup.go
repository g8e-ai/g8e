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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func getToolConfigPaths() map[string][]string {
	switch runtime.GOOS {
	case "windows":
		return map[string][]string{
			"claude": {"~/AppData/Roaming/Claude Code/config.json"},
			"cursor": {"~/AppData/Roaming/Cursor/User/config.json"},
			"code":   {"~/AppData/Roaming/Code/User/settings.json"},
			"cline":  {"~/AppData/Roaming/Cline/config.json"},
		}
	case "darwin":
		return map[string][]string{
			"claude": {"~/Library/Application Support/Claude Code/config.json"},
			"cursor": {"~/.cursor/config.json"},
			"code":   {"~/Library/Application Support/Code/User/settings.json"},
			"cline":  {"~/Library/Application Support/Cline/config.json"},
		}
	default: // linux and others
		return map[string][]string{
			"claude": {"~/.config/claude-code/config.json"},
			"cursor": {"~/.cursor/config.json"},
			"code":   {"~/.config/Code/User/settings.json"},
			"cline":  {"~/.config/cline/config.json"},
		}
	}
}

func setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Auto-discover and configure agentic coding tools for g8e integration",
		Long:  `Auto-discover installed agentic coding tools (Claude Code, Cursor, VS Code, Cline) and configure them for g8e zero-trust gateway integration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup()
		},
	}

	cmd.AddCommand(
		setupDiscoverCmd(),
		setupConfigureCmd(),
	)

	return cmd
}

// platformSetupCmd is a top-level command for platform setup (building, dependencies, etc.)
func platformSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Run platform setup (validate dependencies, build binary)",
		Long:  `Auto-detect OS and run the appropriate setup script to validate dependencies and build the g8e binary.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlatformSetup()
		},
	}

	return cmd
}

func setupDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover installed agentic coding tools",
		Long:  `Scan the system for installed agentic coding tools (Claude Code, Cursor, VS Code, Cline).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscover()
		},
	}

	return cmd
}

func setupConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure [tool-name]",
		Short: "Configure a specific tool for g8e integration",
		Long:  `Generate and apply g8e MCP configuration for a specific tool (claude, cursor, code, cline).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigure(args[0])
		},
	}

	return cmd
}

func runSetup() error {
	fmt.Println("Discovering installed agentic coding tools...")

	tools, err := discoverTools()
	if err != nil {
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	if len(tools) == 0 {
		fmt.Println("No supported tools found.")
		return nil
	}

	fmt.Printf("Found %d tool(s):\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("  - %s\n", tool)
	}

	fmt.Println("\nTo configure a tool, run: ./g8e gw setup configure <tool-name>")
	return nil
}

func runDiscover() error {
	tools, err := discoverTools()
	if err != nil {
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	if len(tools) == 0 {
		fmt.Println("No supported tools found.")
		return nil
	}

	fmt.Printf("Found %d tool(s):\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("  - %s\n", tool)
	}

	return nil
}

func runConfigure(toolName string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	configPaths, ok := getToolConfigPaths()[toolName]
	if !ok {
		return fmt.Errorf("unsupported tool: %s (supported: claude, cursor, code, cline)", toolName)
	}

	var existingConfigPath string
	for _, path := range configPaths {
		expandedPath := expandPath(path)
		if _, err := os.Stat(expandedPath); err == nil {
			existingConfigPath = expandedPath
			break
		}
	}

	if existingConfigPath == "" {
		return fmt.Errorf("tool config file not found for %s (checked: %v)", toolName, configPaths)
	}

	mcpConfig := generateMCPConfigForTool(cfg, toolName)

	configDir := filepath.Dir(existingConfigPath)
	standaloneConfigFile := filepath.Join(configDir, "g8e-mcp-config.json")

	var existingConfig map[string]interface{}
	if data, err := os.ReadFile(existingConfigPath); err == nil {
		dataStr := string(data)

		// Try to parse as-is first
		parseErr := json.Unmarshal(data, &existingConfig)
		if parseErr != nil {
			// If that fails, try stripping JSON comments (common in config files)
			strippedData := stripJSONComments(dataStr)
			parseErr = json.Unmarshal([]byte(strippedData), &existingConfig)
			if parseErr != nil {
				// As a fallback, write standalone config file
				if err := os.MkdirAll(configDir, 0755); err != nil {
					return fmt.Errorf("failed to create config directory: %w", err)
				}
				if err := os.WriteFile(standaloneConfigFile, []byte(mcpConfig), 0644); err != nil {
					return fmt.Errorf("failed to write fallback config file: %w", err)
				}
				fmt.Printf("Warning: failed to parse existing config at %s: %v\n", existingConfigPath, parseErr)
				fmt.Printf("The file may contain unsupported comment syntax or invalid JSON.\n")
				fmt.Printf("As a fallback, a standalone config file has been written to:\n  %s\n", standaloneConfigFile)
				fmt.Printf("You can manually merge this into your %s config or add it as an external file reference.\n", toolName)
				return nil
			}
		}

		if err := mergeMCPConfig(existingConfig, mcpConfig); err != nil {
			return fmt.Errorf("failed to merge config: %w", err)
		}
		mergedData, err := json.MarshalIndent(existingConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal merged config: %w", err)
		}
		if err := os.WriteFile(existingConfigPath, mergedData, 0644); err != nil {
			return fmt.Errorf("failed to write merged config: %w", err)
		}
		fmt.Printf("Configuration merged into: %s\n", existingConfigPath)
		return nil
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(standaloneConfigFile, []byte(mcpConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Configuration written to: %s\n", standaloneConfigFile)
	fmt.Printf("Add the following to your %s config:\n", toolName)
	fmt.Printf("  \"mcpServers\": {\n")
	fmt.Printf("    \"g8e-gateway\": %s\n", mcpConfig)
	fmt.Printf("  }\n")

	return nil
}

func mergeMCPConfig(existingConfig map[string]interface{}, mcpConfig string) error {
	var mcpConfigMap map[string]interface{}
	if err := json.Unmarshal([]byte(mcpConfig), &mcpConfigMap); err != nil {
		return err
	}

	if existingConfig["mcpServers"] == nil {
		existingConfig["mcpServers"] = map[string]interface{}{}
	}

	mcpServers, ok := existingConfig["mcpServers"].(map[string]interface{})
	if !ok {
		existingConfig["mcpServers"] = map[string]interface{}{}
		mcpServers = existingConfig["mcpServers"].(map[string]interface{})
	}

	mcpServers["g8e-gateway"] = mcpConfigMap
	return nil
}

func discoverTools() ([]string, error) {
	var tools []string

	for toolName, paths := range defaultToolPaths {
		for _, path := range paths {
			if _, err := exec.LookPath(path); err == nil {
				tools = append(tools, toolName)
				break
			}
		}
	}

	return tools, nil
}

func generateMCPConfigForTool(cfg *config.Config, _ string) string {
	return generateMCPConfig(cfg)
}

func generateMCPConfig(cfg *config.Config) string {
	config := map[string]interface{}{
		"command": cfg.ProjectRoot + "/g8e",
		"args":    []string{"mcp", "gov"},
	}
	configJSON, _ := json.MarshalIndent(config, "  ", "  ")
	return string(configJSON)
}

func stripJSONComments(data string) string {
	var result []byte
	inString := false
	inSingleLineComment := false
	inMultiLineComment := false
	escapeNext := false

	for i := 0; i < len(data); i++ {
		ch := data[i]

		if escapeNext {
			result = append(result, ch)
			escapeNext = false
			continue
		}

		if inMultiLineComment {
			if ch == '*' && i+1 < len(data) && data[i+1] == '/' {
				inMultiLineComment = false
				i++
			}
			continue
		}

		if inSingleLineComment {
			if ch == '\n' {
				inSingleLineComment = false
				result = append(result, ch)
			}
			continue
		}

		if inString {
			if ch == '\\' {
				escapeNext = true
				result = append(result, ch)
			} else if ch == '"' {
				inString = false
				result = append(result, ch)
			} else {
				result = append(result, ch)
			}
			continue
		}

		if ch == '"' {
			inString = true
			result = append(result, ch)
			continue
		}

		if ch == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				inSingleLineComment = true
				result = stripTrailingWhitespaceBytes(result)
				continue
			}
			if data[i+1] == '*' {
				inMultiLineComment = true
				result = stripTrailingWhitespaceBytes(result)
				i++
				continue
			}
		}

		result = append(result, ch)
	}

	return string(result)
}

func stripTrailingWhitespaceBytes(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

func runPlatformSetup() error {
	fmt.Println("Running platform setup...")

	// Get the directory where the g8e binary is located
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Get the directory containing the binary
	binDir := filepath.Dir(execPath)
	// If we're in a build directory, go up to the repo root
	if filepath.Base(binDir) == "bin" {
		binDir = filepath.Dir(binDir)
	}

	// Determine the appropriate setup script based on OS
	var scriptName string
	var scriptArgs []string

	switch runtime.GOOS {
	case "windows":
		scriptName = "windows-setup.ps1"
		scriptArgs = []string{"-ExecutionPolicy", "Bypass", "-File", filepath.Join(binDir, "scripts", scriptName)}
	case "darwin":
		scriptName = "macos-setup.sh"
		scriptArgs = []string{filepath.Join(binDir, "scripts", scriptName)}
	default: // linux and others
		scriptName = "linux-setup.sh"
		scriptArgs = []string{filepath.Join(binDir, "scripts", scriptName)}
	}

	scriptPath := filepath.Join(binDir, "scripts", scriptName)

	// Check if the script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("setup script not found: %s\nEnsure you're running from the g8e repository root", scriptPath)
	}

	fmt.Printf("Detected OS: %s\n", runtime.GOOS)
	fmt.Printf("Running setup script: %s\n", scriptPath)

	var setupCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		setupCmd = exec.Command("pwsh", scriptArgs...)
	} else {
		setupCmd = exec.Command("bash", scriptArgs...)
	}

	// Set up stdin/stdout/stderr
	setupCmd.Stdin = os.Stdin
	setupCmd.Stdout = os.Stdout
	setupCmd.Stderr = os.Stderr

	// Run the setup script
	if err := setupCmd.Run(); err != nil {
		return fmt.Errorf("setup script failed: %w", err)
	}

	fmt.Println("\nPlatform setup completed successfully!")
	return nil
}
