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
	"fmt"
	"os"
	"os/exec"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

var defaultToolPaths = map[string][]string{
	"claude": {"claude", "/usr/local/bin/claude", "/opt/homebrew/bin/claude"},
	"cursor": {"cursor", "/usr/local/bin/cursor", "/opt/homebrew/bin/cursor"},
	"code":   {"code", "/usr/local/bin/code", "/opt/homebrew/bin/code", "/usr/bin/code"},
	"cline":  {"cline", "/usr/local/bin/cline"},
}

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent [tool-name] -- [tool-args]",
		Short: "Wrap agentic coding tools with g8e zero-trust gateway",
		Long: `Wrap agentic coding tools (Claude Code, Cursor, VS Code, Cline) with g8e governance.

Examples:
  ./g8e agent claude -- --help
  ./g8e agent cursor -- --help
  ./g8e agent code -- --help`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("tool name required (e.g., claude, cursor, code)")
			}

			toolName := args[0]
			toolArgs := args[1:]

			return runAgentWrapper(toolName, toolArgs)
		},
	}

	cmd.AddCommand(
		agentClaudeCmd(),
	)

	return cmd
}

func agentClaudeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude -- [claude-args]",
		Short: "Execute Claude Code proxied through g8e gateway",
		Long:  `Execute Claude Code with all tool calls proxied through the g8e zero-trust gateway. Automatically configures MCP integration and handles L3 approvals.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentWrapper("claude", args)
		},
	}

	return cmd
}

func runAgentWrapper(toolName string, toolArgs []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	running, _, err := checkGatewayStatus(cfg)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("gateway is not running. Start it with: ./g8e gw start")
	}

	creds, err := auth.LoadCredentials(cfg)
	if err != nil {
		return fmt.Errorf("failed to load credentials: %w (run './g8e gw cli auth login' to authenticate)", err)
	}
	if creds == nil {
		return fmt.Errorf("CLI not authenticated (no credentials found at %s). Run: ./g8e gw cli auth login", cfg.CredentialsFile())
	}

	toolBinary, err := detectToolBinary(toolName, defaultToolPaths)
	if err != nil {
		return err
	}

	env := prepareAgentEnvironment(cfg, creds)

	return executeTool(toolBinary, toolArgs, env)
}

func detectToolBinary(toolName string, toolPaths map[string][]string) (string, error) {
	paths, ok := toolPaths[toolName]
	if !ok {
		supportedTools := make([]string, 0, len(toolPaths))
		for tool := range toolPaths {
			supportedTools = append(supportedTools, tool)
		}
		return "", fmt.Errorf("unknown tool: %s (supported tools: %v)", toolName, supportedTools)
	}

	for _, path := range paths {
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("tool binary not found: %s (checked: %v)", toolName, paths)
}

func prepareAgentEnvironment(cfg *config.Config, creds *auth.Credentials) []string {
	env := os.Environ()

	mcpConfig := generateMCPConfigForStdio(cfg)

	env = append(env, fmt.Sprintf("G8E_MCP_CONFIG=%s", mcpConfig))
	env = append(env, fmt.Sprintf("G8E_GATEWAY_URL=https://g8e.local:%d/mcp", constants.Ports.OperatorHttps))
	env = append(env, fmt.Sprintf("G8E_CLIENT_CERT=%s", cfg.CLICertFile()))
	env = append(env, fmt.Sprintf("G8E_CLIENT_KEY=%s", cfg.CLIKeyFile()))
	env = append(env, fmt.Sprintf("G8E_CA_BUNDLE=%s", cfg.TrustBundlePath()))
	env = append(env, fmt.Sprintf("G8E_OPERATOR_SESSION_ID=%s", creds.OperatorSessionID))
	env = append(env, fmt.Sprintf("G8E_USER_ID=%s", creds.UserID))

	return env
}

func generateMCPConfigForStdio(cfg *config.Config) string {
	return fmt.Sprintf(`{
  "mcpServers": {
    "g8e-gateway": {
      "transport": {
        "type": "stdio",
        "command": "%s",
        "args": ["mcp", "gov"]
      },
      "capabilities": {
        "tools": true,
        "resources": true,
        "prompts": true
      }
    }
  }
}`, cfg.ProjectRoot+"/g8e")
}

func executeTool(binary string, args []string, env []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	setSysProcAttr(cmd)

	return cmd.Run()
}

func checkGatewayStatus(cfg *config.Config) (bool, int, error) {
	pm, err := platform.NewProcessManager(cfg.ProjectRoot)
	if err != nil {
		return false, 0, fmt.Errorf("failed to create process manager: %w", err)
	}

	return pm.OperatorStatus()
}
