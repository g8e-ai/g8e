// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package agent is the single source of truth for the external coding agents
// that `g8e mcp agent run` can launch. Each supported agent is one Integration
// entry; config writing, launch arguments, and tool-interception verification
// are all driven from that entry. The launcher prepares identity and config;
// the Gateway governs execution.
package agent

import (
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ConfigStrategy selects how an agent's MCP configuration is written.
type ConfigStrategy string

const (
	// ConfigTempJSON writes a throwaway mcpServers JSON file passed to the agent
	// by flag (Claude, Codex).
	ConfigTempJSON ConfigStrategy = "temp_json"
	// ConfigDevinJSON writes mcpServers JSON to the Devin CLI config file.
	ConfigDevinJSON ConfigStrategy = "devin_json"
	// ConfigGeminiSettings merges the g8e server and an empty tools.core list
	// into Gemini's settings.json.
	ConfigGeminiSettings ConfigStrategy = "gemini_settings"
	// ConfigGooseYAML merges the g8e extension into Goose's config.yaml.
	ConfigGooseYAML ConfigStrategy = "goose_yaml"
)

// LaunchStrategy selects the argv the agent binary is started with.
type LaunchStrategy string

const (
	// LaunchMCPConfigFlags passes the generated config and native-tool lockdown
	// by flag (--mcp-config, --strict-mcp-config, --disallowed-tools).
	LaunchMCPConfigFlags LaunchStrategy = "mcp_config_flags"
	// LaunchGooseExtension starts a profile-less Goose session with g8e as the
	// only extension.
	LaunchGooseExtension LaunchStrategy = "goose_extension"
	// LaunchConfigOnly starts the agent with no extra arguments; everything the
	// agent needs is in its config file.
	LaunchConfigOnly LaunchStrategy = "config_only"
)

// ToolLockdown states how completely the launcher can disable an agent's
// built-in tools so that every action must route through the Gateway.
type ToolLockdown string

const (
	// LockdownStrict means every built-in tool is disabled by the launcher.
	LockdownStrict ToolLockdown = "strict"
	// LockdownPartial means g8e is the only configured MCP server but the agent
	// offers no mechanism to disable its native tools.
	LockdownPartial ToolLockdown = "partial"
)

// VerifyInput is what a VerifyHook inspects: the config the launcher just
// wrote and the argv it computed for the agent binary.
type VerifyInput struct {
	ConfigPath string
	LaunchArgs []string
}

// VerifyHook checks one aspect of tool interception. A failure wraps
// constants.ErrToolInterceptionVerification.
type VerifyHook func(VerifyInput) error

// Integration is the complete launcher definition of one supported agent.
// Adding an agent means adding one entry to registry plus its verify hooks.
type Integration struct {
	ID             constants.AgentBinary
	DisplayName    string
	BinaryName     string // looked up on PATH
	ConfigStrategy ConfigStrategy
	LaunchStrategy LaunchStrategy
	ToolLockdown   ToolLockdown
	VerifyHooks    []VerifyHook
}

// registry lists every supported agent in display order.
var registry = []Integration{
	{
		ID:             constants.AgentBinaryClaude,
		DisplayName:    "Anthropic Claude Desktop / Claude Code",
		BinaryName:     string(constants.AgentBinaryClaude),
		ConfigStrategy: ConfigTempJSON,
		LaunchStrategy: LaunchMCPConfigFlags,
		ToolLockdown:   LockdownStrict,
		VerifyHooks:    []VerifyHook{verifyMCPServersJSON, verifyStrictLaunchFlags},
	},
	{
		ID:             constants.AgentBinaryCodex,
		DisplayName:    "OpenAI Codex AI coding assistant",
		BinaryName:     string(constants.AgentBinaryCodex),
		ConfigStrategy: ConfigTempJSON,
		LaunchStrategy: LaunchMCPConfigFlags,
		ToolLockdown:   LockdownStrict,
		VerifyHooks:    []VerifyHook{verifyMCPServersJSON, verifyStrictLaunchFlags},
	},
	{
		ID:             constants.AgentBinaryDevin,
		DisplayName:    "Devin CLI local coding agent",
		BinaryName:     string(constants.AgentBinaryDevin),
		ConfigStrategy: ConfigDevinJSON,
		LaunchStrategy: LaunchConfigOnly,
		ToolLockdown:   LockdownPartial,
		VerifyHooks:    []VerifyHook{verifyMCPServersJSON},
	},
	{
		ID:             constants.AgentBinaryGemini,
		DisplayName:    "Google Gemini CLI",
		BinaryName:     string(constants.AgentBinaryGemini),
		ConfigStrategy: ConfigGeminiSettings,
		LaunchStrategy: LaunchConfigOnly,
		ToolLockdown:   LockdownStrict,
		VerifyHooks:    []VerifyHook{verifyMCPServersJSON, verifyGeminiToolsCoreEmpty},
	},
	{
		ID:             constants.AgentBinaryGoose,
		DisplayName:    "Goose AI coding assistant",
		BinaryName:     string(constants.AgentBinaryGoose),
		ConfigStrategy: ConfigGooseYAML,
		LaunchStrategy: LaunchGooseExtension,
		ToolLockdown:   LockdownStrict,
		VerifyHooks:    []VerifyHook{verifyGooseLaunchFlags, verifyGooseExtensionEntry},
	},
}

// All returns the supported integrations in display order.
func All() []Integration {
	out := make([]Integration, len(registry))
	copy(out, registry)
	return out
}

// Lookup resolves an agent ID case-insensitively. An unknown ID wraps
// constants.ErrAgentNotFound.
func Lookup(id string) (Integration, error) {
	for _, integration := range registry {
		if strings.EqualFold(string(integration.ID), id) {
			return integration, nil
		}
	}
	return Integration{}, fmt.Errorf("%w: %q (supported agents: %s)", constants.ErrAgentNotFound, id, supportedIDs())
}

func supportedIDs() string {
	ids := make([]string, len(registry))
	for i, integration := range registry {
		ids[i] = string(integration.ID)
	}
	return strings.Join(ids, ", ")
}
