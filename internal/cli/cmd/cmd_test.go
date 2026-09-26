// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
)

func commandNamed(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestRootCommandStructure(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	assert.Equal(t, "g8e", root.Use)
	assert.Contains(t, root.Short, "g8e Platform Manager")
}

func TestCommandRegistration(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	for _, name := range []string{"gw", "auth", "mcp", "operator", "vault", "test", "demos", "docker", "audit", "report", "public", "swagger", "tui", "version", "compliance", "eval"} {
		assert.NotNil(t, commandNamed(root, name), "root should register %s", name)
	}
}

func TestGatewayCommandSubcommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	cmd := commandNamed(root, "gw")
	require.NotNil(t, cmd)

	for _, subcmd := range []string{"start", "stop", "status", "restart", "logs", "settings", "reset", "clean", "setup", "data", "security", "tunnel"} {
		assert.NotNil(t, commandNamed(cmd, subcmd), "gateway command should have %s subcommand", subcmd)
	}
}

func TestAuthCommandSubcommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	cmd := commandNamed(root, "auth")
	require.NotNil(t, cmd)

	for _, subcmd := range []string{"enroll", "logout", "approve", "approve-recovery", "refresh", "context"} {
		assert.NotNil(t, commandNamed(cmd, subcmd), "auth command should have %s subcommand", subcmd)
	}
}

func TestDataCommandSubcommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	gw := commandNamed(root, "gw")
	require.NotNil(t, gw)
	cmd := commandNamed(gw, "data")
	require.NotNil(t, cmd)

	for _, subcmd := range []string{"users", "operators", "settings", "store", "audit"} {
		assert.NotNil(t, commandNamed(cmd, subcmd), "data command should have %s subcommand", subcmd)
	}
}

func TestSecurityCommandSubcommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	gw := commandNamed(root, "gw")
	require.NotNil(t, gw)
	cmd := commandNamed(gw, "security")
	require.NotNil(t, cmd)
	assert.NotNil(t, commandNamed(cmd, "validate"), "security command should have validate subcommand")
}

func TestOperatorCommandSubcommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	cmd := commandNamed(root, "operator")
	require.NotNil(t, cmd)

	for _, subcmd := range []string{"list", "show", "bind", "run", "start", "cp", "scp", "deploy", "stream"} {
		assert.NotNil(t, commandNamed(cmd, subcmd), "operator command should have %s subcommand", subcmd)
	}
}

func TestCommandHelpText(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	for _, name := range []string{"gw", "auth", "operator"} {
		cmd := commandNamed(root, name)
		require.NotNil(t, cmd)
		assert.NotEmpty(t, cmd.Short, name+" should have non-empty Short description")
		assert.NotEmpty(t, cmd.Long, name+" should have non-empty Long description")
	}
}

func TestCommandFlagValidation(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	gw := commandNamed(root, "gw")
	require.NotNil(t, gw)

	reset := commandNamed(gw, "reset")
	require.NotNil(t, reset)
	assert.NotNil(t, reset.Flags().Lookup("force"), "gateway reset should have --force flag")
	assert.NotNil(t, reset.Flags().Lookup("y"), "gateway reset should have -y flag")
	assert.NotNil(t, reset.Flags().Lookup("yes"), "gateway reset should have --yes flag")

	clean := commandNamed(gw, "clean")
	require.NotNil(t, clean)
	assert.NotNil(t, clean.Flags().Lookup("force"), "gateway clean should have --force flag")
	assert.NotNil(t, clean.Flags().Lookup("y"), "gateway clean should have -y flag")
	assert.NotNil(t, clean.Flags().Lookup("yes"), "gateway clean should have --yes flag")

	security := commandNamed(gw, "security")
	require.NotNil(t, security)
	validate := commandNamed(security, "validate")
	require.NotNil(t, validate)
	assert.NotNil(t, validate.Flags().Lookup("pki-dir"), "security validate should have --pki-dir flag")
	assert.NotNil(t, validate.Flags().Lookup("secrets-dir"), "security validate should have --secrets-dir flag")
}

func TestCommandAliases(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	gw := commandNamed(root, "gw")
	require.NotNil(t, gw)
	logs := commandNamed(gw, "logs")
	require.NotNil(t, logs)
	assert.NotNil(t, logs.Flags().Lookup("follow"))
	assert.NotNil(t, logs.Flags().ShorthandLookup("f"))
}

func TestPlaceholderCommands(t *testing.T) {
	root := NewRootCmd("test", serve.VersionInfo{})
	auth := commandNamed(root, "auth")
	require.NotNil(t, auth)
	approve := commandNamed(auth, "approve")
	require.NotNil(t, approve)
	assert.Contains(t, approve.Use, "approve")
}
