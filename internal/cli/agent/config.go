// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
)

// nativeToolsToDisable lists built-in tools that Claude Code and Codex must
// disable via --disallowed-tools to force all I/O through g8e's MCP gateway.
// Other agents use different mechanisms (see their Integration entries).
var nativeToolsToDisable = []string{
	"Bash", "Read", "Write", "Edit", "Glob", "Grep", "WebSearch", "WebFetch",
}

// mcpConfig is the mcpServers JSON structure shared by Claude, Codex, and Devin.
type mcpConfig struct {
	MCPServers   map[string]mcpServer `json:"mcpServers"`
	ExcludeTools []string             `json:"excludeTools,omitempty"`
}

// mcpServer is a single stdio MCP server entry.
type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// geminiSettings is the Gemini settings.json structure.
type geminiSettings struct {
	MCPServers map[string]mcpServer `json:"mcpServers,omitempty"`
	Tools      *geminiToolsConfig   `json:"tools,omitempty"`
}

// geminiToolsConfig controls Gemini's built-in tool enablement. When
// tools.core is set to any value only the listed tools are enabled; an empty
// array means zero built-in tools, so every action must go through MCP.
type geminiToolsConfig struct {
	Core    []string `json:"core"`
	Exclude []string `json:"exclude,omitempty"`
}

// gooseConfig is the Goose config.yaml structure (an extensions map).
type gooseConfig struct {
	Extensions map[string]gooseExtension `yaml:"extensions"`
}

type gooseExtension struct {
	Enabled bool           `yaml:"enabled"`
	Config  gooseExtConfig `yaml:"config"`
}

// gooseExtConfig holds the stdio transport configuration for a Goose extension.
type gooseExtConfig struct {
	Type        string   `yaml:"type"`
	Name        string   `yaml:"name"`
	Cmd         string   `yaml:"cmd"`
	Args        []string `yaml:"args"`
	Description string   `yaml:"description"`
	Timeout     int      `yaml:"timeout"`
}

// StdioServerArgs returns the argv that makes an agent launch the stdio bridge
// under the enrolled application identity appName. The app name is the only
// credential selector passed to the agent child; the bridge loads the managed
// cert/key for that name itself.
func StdioServerArgs(appName string) []string {
	return []string{"mcp", "stdio", "--" + constants.Flag.App, appName}
}

// ResolveHomeDir returns the user's home directory, preferring
// os.UserHomeDir and falling back to the HOME host fact.
func ResolveHomeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		return homeDir, nil
	}
	if fallback := os.Getenv(string(constants.EnvVar.Home)); fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("%w: %w", constants.ErrMCPGetHomeDirectory, err)
}

// WriteConfig writes the agent's MCP configuration beneath homeDir so that the
// g8e stdio bridge (binaryPath, running as application appName) is the agent's
// MCP server. It returns the config path and a cleanup func (nil unless the
// config is a throwaway file).
func (i Integration) WriteConfig(homeDir, binaryPath, appName string) (string, func(), error) {
	server := mcpServer{Command: binaryPath, Args: StdioServerArgs(appName)}
	agentPaths := paths.GetAgentConfigPaths(homeDir)

	switch i.ConfigStrategy {
	case ConfigDevinJSON:
		configJSON, err := json.Marshal(mcpConfig{MCPServers: map[string]mcpServer{constants.MCPServerNameG8E: server}})
		if err != nil {
			return "", nil, fmt.Errorf("agent: marshal %s config: %w", i.ID, err)
		}
		return writeConfigWithBackup(agentPaths.DevinConfigDir, agentPaths.DevinConfigPath, configJSON)

	case ConfigGeminiSettings:
		return writeGeminiSettings(agentPaths, server)

	case ConfigGooseYAML:
		return writeGooseConfig(agentPaths, binaryPath, appName)

	case ConfigTempJSON:
		return writeTempMCPConfig(server)

	default:
		return "", nil, fmt.Errorf("%w: agent %q has unknown config strategy %q", constants.ErrAgentNotSupported, i.ID, i.ConfigStrategy)
	}
}

// writeTempMCPConfig writes the mcpServers file passed to Claude/Codex via
// --mcp-config, with the native tools listed for exclusion.
func writeTempMCPConfig(server mcpServer) (string, func(), error) {
	configJSON, err := json.Marshal(mcpConfig{
		MCPServers:   map[string]mcpServer{constants.MCPServerNameG8E: server},
		ExcludeTools: nativeToolsToDisable,
	})
	if err != nil {
		return "", nil, fmt.Errorf("agent: marshal temp mcp config: %w", err)
	}
	tmpFile, err := os.CreateTemp("", "g8e-mcp-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(configJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return tmpPath, func() {
		if err := os.Remove(tmpPath); err != nil {
			slog.Warn("Failed to cleanup temp MCP config file", "path", tmpPath, "error", err)
		}
	}, nil
}

// writeGeminiSettings merges the g8e server into Gemini's settings.json and
// disables all built-in tools by setting tools.core to an empty array.
func writeGeminiSettings(agentPaths paths.AgentConfigPaths, server mcpServer) (string, func(), error) {
	if err := os.MkdirAll(agentPaths.GeminiConfigDir, constants.PermDirStandard); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}

	var settings geminiSettings
	if existingData, err := os.ReadFile(agentPaths.GeminiConfigPath); err == nil {
		if err := json.Unmarshal(existingData, &settings); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}
	}
	if settings.MCPServers == nil {
		settings.MCPServers = make(map[string]mcpServer)
	}
	settings.MCPServers[constants.MCPServerNameG8E] = server
	settings.Tools = &geminiToolsConfig{Core: []string{}}

	configJSON, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("agent: marshal gemini settings: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s with native tools disabled\n", pathutil.ToSlash(agentPaths.GeminiConfigPath))
	if err := os.WriteFile(agentPaths.GeminiConfigPath, configJSON, constants.PermFilePublic); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return agentPaths.GeminiConfigPath, nil, nil
}

// writeGooseConfig merges the g8e extension into Goose's config.yaml,
// preserving provider and other settings. The --no-profile launch flag skips
// every profile extension (including the developer extension that provides
// shell/file tools); --with-extension loads g8e as the sole MCP server.
func writeGooseConfig(agentPaths paths.AgentConfigPaths, binaryPath, appName string) (string, func(), error) {
	g8eExt := gooseExtension{
		Enabled: true,
		Config: gooseExtConfig{
			Type:        constants.MCPTransportStdio,
			Name:        constants.MCPServerNameG8E,
			Cmd:         binaryPath,
			Args:        StdioServerArgs(appName),
			Description: constants.MCPG8EDescription,
			Timeout:     constants.GooseExtTimeout,
		},
	}

	// rawConfig preserves all unknown Goose config fields (provider, etc.)
	// during the read-modify-write cycle. gooseConfig only models the
	// extensions map, so schema-less passthrough is required to avoid
	// dropping fields on re-marshal.
	var rawConfig map[string]any
	if existingData, err := os.ReadFile(agentPaths.GooseYAMLConfigPath); err == nil {
		if err := yaml.Unmarshal(existingData, &rawConfig); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}
	}
	if rawConfig == nil {
		rawConfig = make(map[string]any)
	}

	// extMap stays map[string]any to preserve extension entries from other tools.
	extMap, _ := rawConfig["extensions"].(map[string]any)
	if extMap == nil {
		extMap = make(map[string]any)
	}
	extMap[constants.MCPServerNameG8E] = g8eExt
	rawConfig["extensions"] = extMap

	configYAML, err := yaml.Marshal(rawConfig)
	if err != nil {
		return "", nil, fmt.Errorf("agent: marshal goose config: %w", err)
	}
	return writeConfigWithBackup(agentPaths.GooseYAMLConfigDir, agentPaths.GooseYAMLConfigPath, configYAML)
}

// writeConfigWithBackup creates the config dir, backs up an existing config,
// and writes the new config.
func writeConfigWithBackup(configDir, configPath string, content []byte) (string, func(), error) {
	if err := os.MkdirAll(configDir, constants.PermDirStandard); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	if existing, err := os.ReadFile(configPath); err == nil {
		if err := os.WriteFile(configPath+".bak", existing, constants.PermFilePublic); err != nil {
			return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
		}
		fmt.Fprintf(os.Stderr, "[g8e] Backing up existing config to %s\n", pathutil.ToSlash(configPath+".bak"))
	}
	fmt.Fprintf(os.Stderr, "[g8e] Writing MCP config to %s (g8e as only MCP server for governance)\n", pathutil.ToSlash(configPath))
	if err := os.WriteFile(configPath, content, constants.PermFilePublic); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrFileWriteFailed, err)
	}
	return configPath, nil, nil
}

// LaunchArgs returns the argv to pass to the agent binary for a governed
// session. Governance is enforced by making g8e the only MCP server in the
// agent's config; agents that can disable native tools by flag get those too.
func (i Integration) LaunchArgs(configPath, binaryPath, appName string) ([]string, error) {
	switch i.LaunchStrategy {
	case LaunchMCPConfigFlags:
		// --mcp-config          load g8e as the only MCP server
		// --strict-mcp-config   ignore all other configured MCP servers
		// --disallowed-tools    disable native tools so every I/O action must
		//                       go through g8e MCP tools and is therefore audited
		return []string{
			"--mcp-config", configPath,
			"--strict-mcp-config",
			"--disallowed-tools", strings.Join(nativeToolsToDisable, ","),
		}, nil
	case LaunchGooseExtension:
		// --no-profile starts goose with zero profile extensions; --with-extension
		// loads g8e as the sole MCP server, since --no-profile also skips
		// extensions defined in config.yaml.
		return []string{"session", "--no-profile", "--with-extension", binaryPath + " " + strings.Join(StdioServerArgs(appName), " ")}, nil
	case LaunchConfigOnly:
		return []string{}, nil
	default:
		return nil, fmt.Errorf("%w: agent %q has unknown launch strategy %q", constants.ErrAgentNotSupported, i.ID, i.LaunchStrategy)
	}
}
