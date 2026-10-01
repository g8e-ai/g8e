// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

// agentLauncher impersonates the external agent the launcher configures. Its
// MCP traffic is what the generated config makes the real agent send: a
// stdio bridge to the same Gateway /mcp endpoint every other persona uses.
var agentLauncher = clientpkg.Persona{ID: "agent-launcher", UserAgent: "g8e-agent-launcher/1.x (stdio-bridge)"}

func agentLauncherScenarios() []Scenario {
	return []Scenario{
		{
			Name: "agent-launcher-config", Title: "Agent Launcher Config and Tool Lockdown", Persona: agentLauncher, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Prove every registered agent's launcher output (config file,
				// launch argv, verify hooks) without spawning a third-party agent
				// binary, then prove the governed MCP endpoint those configs point
				// the agent at is serving tools on the real Gateway.
				binaryPath, err := os.Executable()
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
				}
				verified, err := verifyLauncherConfigs(binaryPath)
				if err != nil {
					return err
				}
				for _, line := range verified {
					r.note("%s", line)
				}

				list, err := c.MCPToolsList(ctx, agentLauncher)
				if err != nil {
					return fmt.Errorf("gateway MCP tools/list for launcher persona: %w", err)
				}
				if list == nil || list.Error != nil || firstTool(list, "") == "" {
					return fmt.Errorf("gateway MCP endpoint served no tools to the launcher persona")
				}
				r.note("gateway /mcp serves tools to the stdio bridge the launcher configures")
				return nil
			},
		},
	}
}

// verifyLauncherConfigs prepares every registry integration in an isolated
// home directory, runs its verify hooks, and asserts the generated config or
// launch argv wires the agent to `g8e mcp stdio --app <agent>`. It returns one
// human-readable line per verified agent.
func verifyLauncherConfigs(binaryPath string) ([]string, error) {
	var lines []string
	for _, integration := range agent.All() {
		line, err := verifyLauncherConfig(integration, binaryPath)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", integration.ID, err)
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// assertStdioBridgeWiring fails unless wiring (the generated config plus launch
// argv) names the g8e binary, the stdio subcommand, and the agent's app name.
func assertStdioBridgeWiring(wiring, binaryPath, appName string) error {
	for _, want := range []string{binaryPath, "stdio", "--" + constants.Flag.App, appName} {
		if want == "" || !strings.Contains(wiring, want) {
			return fmt.Errorf("launcher output does not wire the g8e stdio bridge: missing %q", want)
		}
	}
	return nil
}

func verifyLauncherConfig(integration agent.Integration, binaryPath string) (string, error) {
	homeDir, err := os.MkdirTemp("", "g8e-agent-launcher-*")
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	defer os.RemoveAll(homeDir)

	appName := string(integration.ID)
	prepared, err := integration.Prepare(homeDir, binaryPath, appName, true)
	if err != nil {
		return "", err
	}
	defer prepared.Cleanup()

	config, err := os.ReadFile(prepared.ConfigPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrFileReadFailed, err)
	}

	// The config (or, for Goose, the launch argv) must make the stdio bridge the
	// agent's MCP server under the agent's own application identity.
	if err := assertStdioBridgeWiring(string(config)+"\n"+strings.Join(prepared.LaunchArgs, " "), binaryPath, appName); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: g8e stdio bridge is the only MCP server, %s tool lockdown verified", integration.ID, integration.ToolLockdown), nil
}
