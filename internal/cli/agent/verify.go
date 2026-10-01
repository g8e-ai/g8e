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
	"os"

	"gopkg.in/yaml.v3"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Prepared is a written agent config plus the argv to launch the agent with.
// Cleanup removes any throwaway config and is always safe to call.
type Prepared struct {
	ConfigPath string
	LaunchArgs []string
	Cleanup    func()
}

// Prepare writes the agent config beneath homeDir, computes the launch argv,
// and (when verify is set) runs the verify hooks. It never starts the agent
// binary, so `agent run` and `agent verify` share one path.
func (i Integration) Prepare(homeDir, binaryPath, appName string, verify bool) (Prepared, error) {
	configPath, cleanup, err := i.WriteConfig(homeDir, binaryPath, appName)
	if err != nil {
		return Prepared{}, fmt.Errorf("agent %s: write config: %w", i.ID, err)
	}
	if cleanup == nil {
		cleanup = func() {}
	}

	launchArgs, err := i.LaunchArgs(configPath, binaryPath, appName)
	if err != nil {
		cleanup()
		return Prepared{}, fmt.Errorf("agent %s: launch args: %w", i.ID, err)
	}

	if verify {
		if err := i.Verify(configPath, launchArgs); err != nil {
			cleanup()
			return Prepared{}, err
		}
	}
	return Prepared{ConfigPath: configPath, LaunchArgs: launchArgs, Cleanup: cleanup}, nil
}

// VerifyIsolated writes the integration's config into a throwaway home
// directory, computes its launch argv, and runs the verify hooks. The agent
// binary is never started and the real agent config is never touched, so it is
// safe to run anywhere. It is the one verification path shared by
// `g8e mcp agent verify` and the evaluation environment canaries.
func (i Integration) VerifyIsolated(binaryPath string) error {
	homeDir, err := os.MkdirTemp("", "g8e-agent-verify-*")
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	defer os.RemoveAll(homeDir)

	prepared, err := i.Prepare(homeDir, binaryPath, string(i.ID), true)
	if err != nil {
		return err
	}
	prepared.Cleanup()
	return nil
}

// VerifyAllIsolated runs VerifyIsolated for every registered integration and
// returns the first failure.
func VerifyAllIsolated(binaryPath string) error {
	for _, integration := range All() {
		if err := integration.VerifyIsolated(binaryPath); err != nil {
			return err
		}
	}
	return nil
}

// Verify runs every VerifyHook of the integration against the config the
// launcher wrote and the argv it computed. It catches config write failures,
// missing launch flags, and config format drift before the agent starts.
func (i Integration) Verify(configPath string, launchArgs []string) error {
	input := VerifyInput{ConfigPath: configPath, LaunchArgs: launchArgs}
	for _, hook := range i.VerifyHooks {
		if err := hook(input); err != nil {
			return fmt.Errorf("agent %s: %w", i.ID, err)
		}
	}
	return nil
}

func verifyFailure(format string, args ...any) error {
	return fmt.Errorf("%w: %s", constants.ErrToolInterceptionVerification, fmt.Sprintf(format, args...))
}

// hasFlag reports whether args contains flag, and (when needValue) a value
// following it.
func hasFlag(args []string, flag string, needValue bool) bool {
	for idx, arg := range args {
		if arg != flag {
			continue
		}
		return !needValue || idx+1 < len(args)
	}
	return false
}

// verifyMCPServersJSON verifies the JSON config file exists and carries the
// g8e entry in its mcpServers map. It serves Claude/Codex temp files, Devin's
// config.json, and Gemini's settings.json, which share that shape.
func verifyMCPServersJSON(in VerifyInput) error {
	data, err := os.ReadFile(in.ConfigPath)
	if err != nil {
		return verifyFailure("read mcp config %q: %v", in.ConfigPath, err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return verifyFailure("parse mcp config: %v", err)
	}
	if _, ok := cfg.MCPServers[constants.MCPServerNameG8E]; !ok {
		return verifyFailure("mcp config missing %s server entry", constants.MCPServerNameG8E)
	}
	return nil
}

// verifyStrictLaunchFlags verifies the launch args lock Claude/Codex to the
// g8e MCP server and disable their native tools.
func verifyStrictLaunchFlags(in VerifyInput) error {
	if !hasFlag(in.LaunchArgs, "--strict-mcp-config", false) {
		return verifyFailure("launch args missing --strict-mcp-config flag")
	}
	if !hasFlag(in.LaunchArgs, "--disallowed-tools", true) {
		return verifyFailure("launch args missing --disallowed-tools flag")
	}
	return nil
}

// verifyGooseLaunchFlags verifies the launch args start Goose with zero
// profile extensions and g8e as the sole extension.
func verifyGooseLaunchFlags(in VerifyInput) error {
	if !hasFlag(in.LaunchArgs, "--no-profile", false) {
		return verifyFailure("launch args missing --no-profile flag")
	}
	if !hasFlag(in.LaunchArgs, "--with-extension", true) {
		return verifyFailure("launch args missing --with-extension flag")
	}
	return nil
}

// verifyGooseExtensionEntry verifies Goose's config.yaml carries the g8e extension.
func verifyGooseExtensionEntry(in VerifyInput) error {
	data, err := os.ReadFile(in.ConfigPath)
	if err != nil {
		return verifyFailure("read goose config %q: %v", in.ConfigPath, err)
	}
	var cfg gooseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return verifyFailure("parse goose config: %v", err)
	}
	if _, ok := cfg.Extensions[constants.MCPServerNameG8E]; !ok {
		return verifyFailure("goose config missing %s extension entry", constants.MCPServerNameG8E)
	}
	return nil
}

// verifyGeminiToolsCoreEmpty verifies Gemini's settings.json sets tools.core to
// an empty array, which disables every built-in tool.
func verifyGeminiToolsCoreEmpty(in VerifyInput) error {
	data, err := os.ReadFile(in.ConfigPath)
	if err != nil {
		return verifyFailure("read gemini settings %q: %v", in.ConfigPath, err)
	}
	var settings geminiSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return verifyFailure("parse gemini settings: %v", err)
	}
	switch {
	case settings.Tools == nil:
		return verifyFailure("gemini settings missing tools.core configuration")
	case settings.Tools.Core == nil:
		return verifyFailure("gemini settings tools.core is null (expected empty array)")
	case len(settings.Tools.Core) != 0:
		return verifyFailure("gemini settings tools.core has %d entries (expected empty array to disable all built-in tools)", len(settings.Tools.Core))
	}
	return nil
}
