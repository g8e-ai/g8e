// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGatewayCmd(t *testing.T) {
	t.Run("gw command has correct use and description", func(t *testing.T) {
		cmd := gatewayCmd()
		assert.Equal(t, "gw", cmd.Use)
		assert.Contains(t, cmd.Short, "g8e Gateway")
		assert.Contains(t, cmd.Long, "lifecycle")
	})

	t.Run("gw command has gateway alias", func(t *testing.T) {
		cmd := gatewayCmd()
		assert.Contains(t, cmd.Aliases, "gateway")
	})

	t.Run("gw command has all expected subcommands", func(t *testing.T) {
		cmd := gatewayCmd()
		subcommands := cmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, sub := range subcommands {
			subcommandNames[sub.Name()] = true
		}

		// Verify core lifecycle commands
		assert.True(t, subcommandNames["start"], "start subcommand should exist")
		assert.True(t, subcommandNames["stop"], "stop subcommand should exist")
		assert.True(t, subcommandNames["status"], "status subcommand should exist")
		assert.True(t, subcommandNames["restart"], "restart subcommand should exist")
		assert.True(t, subcommandNames["logs"], "logs subcommand should exist")
		assert.True(t, subcommandNames["settings"], "settings subcommand should exist")
		assert.True(t, subcommandNames["reset"], "reset subcommand should exist")
		assert.True(t, subcommandNames["clean"], "clean subcommand should exist")

		// Verify data and security subcommands
		assert.True(t, subcommandNames["data"], "data subcommand should exist")
		assert.True(t, subcommandNames["security"], "security subcommand should exist")
	})
}

func TestGatewayStartCmd(t *testing.T) {
	t.Run("start command has correct use", func(t *testing.T) {
		cmd := gatewayStartCmd()
		assert.Equal(t, "start", cmd.Use)
		assert.Contains(t, cmd.Short, "Start")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})

	t.Run("start command has all required flags", func(t *testing.T) {
		cmd := gatewayStartCmd()
		flags := cmd.Flags()

		// Verify posture flag
		postureFlag := flags.Lookup("posture")
		assert.NotNil(t, postureFlag, "posture flag should exist")
		assert.Equal(t, "doctrine", postureFlag.DefValue)

		// Verify port flags
		httpPortFlag := flags.Lookup("http-port")
		assert.NotNil(t, httpPortFlag, "http-port flag should exist")
		assert.Equal(t, "0", httpPortFlag.DefValue)

		httpsPortFlag := flags.Lookup("https-port")
		assert.NotNil(t, httpsPortFlag, "https-port flag should exist")
		assert.Equal(t, "0", httpsPortFlag.DefValue)

		// Verify directory flags
		dataDirFlag := flags.Lookup("data-dir")
		assert.NotNil(t, dataDirFlag, "data-dir flag should exist")
		assert.Equal(t, "", dataDirFlag.DefValue)

		pkiDirFlag := flags.Lookup("pki-dir")
		assert.NotNil(t, pkiDirFlag, "pki-dir flag should exist")
		assert.Equal(t, "", pkiDirFlag.DefValue)

		secretsDirFlag := flags.Lookup("secrets-dir")
		assert.NotNil(t, secretsDirFlag, "secrets-dir flag should exist")
		assert.Equal(t, "", secretsDirFlag.DefValue)

		vaultDirFlag := flags.Lookup("vault-dir")
		assert.NotNil(t, vaultDirFlag, "vault-dir flag should exist")
		assert.Equal(t, "", vaultDirFlag.DefValue)

		vaultKeyFlag := flags.Lookup("vault-key")
		assert.NotNil(t, vaultKeyFlag, "vault-key flag should exist")
		assert.Equal(t, "", vaultKeyFlag.DefValue)

		// Verify passkey flags
		passkeyRpIDFlag := flags.Lookup("passkey-rp-id")
		assert.NotNil(t, passkeyRpIDFlag, "passkey-rp-id flag should exist")
		assert.Equal(t, "", passkeyRpIDFlag.DefValue)

		passkeyRpNameFlag := flags.Lookup("passkey-rp-name")
		assert.NotNil(t, passkeyRpNameFlag, "passkey-rp-name flag should exist")
		assert.Equal(t, "", passkeyRpNameFlag.DefValue)

		// Verify rate limiting flags
		rateLimitRPSFlag := flags.Lookup("rate-limit-rps")
		assert.NotNil(t, rateLimitRPSFlag, "rate-limit-rps flag should exist")
		assert.Equal(t, "0", rateLimitRPSFlag.DefValue)

		rateLimitBurstFlag := flags.Lookup("rate-limit-burst")
		assert.NotNil(t, rateLimitBurstFlag, "rate-limit-burst flag should exist")
		assert.Equal(t, "0", rateLimitBurstFlag.DefValue)

		// Verify log level flag
		logLevelFlag := flags.Lookup("log")
		assert.NotNil(t, logLevelFlag, "log flag should exist")
		assert.Equal(t, "info", logLevelFlag.DefValue)

		// Verify cert mode flag
		certModeFlag := flags.Lookup("cert-mode")
		assert.NotNil(t, certModeFlag, "cert-mode flag should exist")
		assert.Equal(t, "", certModeFlag.DefValue)

		// Verify follow flag
		followFlag := flags.Lookup("follow")
		assert.NotNil(t, followFlag, "follow flag should exist")
		assert.Equal(t, "false", followFlag.DefValue)
		assert.Equal(t, "f", followFlag.Shorthand)
	})

	t.Run("start command has RunE function set", func(t *testing.T) {
		cmd := gatewayStartCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayStopCmd(t *testing.T) {
	t.Run("stop command has correct use", func(t *testing.T) {
		cmd := gatewayStopCmd()
		assert.Equal(t, "stop", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})

	t.Run("stop command has RunE function set", func(t *testing.T) {
		cmd := gatewayStopCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayStatusCmd(t *testing.T) {
	t.Run("status command has correct use", func(t *testing.T) {
		cmd := gatewayStatusCmd()
		assert.Equal(t, "status", cmd.Use)
		assert.Contains(t, cmd.Short, "health")
		assert.Contains(t, cmd.Short, "status")
	})

	t.Run("status command has RunE function set", func(t *testing.T) {
		cmd := gatewayStatusCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayRestartCmd(t *testing.T) {
	t.Run("restart command has correct use", func(t *testing.T) {
		cmd := gatewayRestartCmd()
		assert.Equal(t, "restart", cmd.Use)
		assert.Contains(t, cmd.Short, "Restart")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})

	t.Run("restart command has RunE function set", func(t *testing.T) {
		cmd := gatewayRestartCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayLogsCmd(t *testing.T) {
	t.Run("logs command has correct use", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		assert.Equal(t, "logs", cmd.Use)
		assert.Contains(t, cmd.Short, "logs")
	})

	t.Run("logs command has follow flag", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		flags := cmd.Flags()

		followFlag := flags.Lookup("follow")
		assert.NotNil(t, followFlag, "follow flag should exist")
		assert.Equal(t, "false", followFlag.DefValue)
		assert.Equal(t, "f", followFlag.Shorthand)
	})

	t.Run("logs command has RunE function set", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewaySettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := gatewaySettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "settings")
	})

	t.Run("settings command has RunE function set", func(t *testing.T) {
		cmd := gatewaySettingsCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayResetCmd(t *testing.T) {
	t.Run("reset command has correct use", func(t *testing.T) {
		cmd := gatewayResetCmd()
		assert.Equal(t, "reset", cmd.Use)
		assert.Contains(t, cmd.Short, "Reset")
		assert.Contains(t, cmd.Short, "data")
		assert.Contains(t, cmd.Short, "secrets")
	})

	t.Run("reset has force flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("reset has y shorthand flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		flag := cmd.Flags().Lookup("y")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("reset has yes flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("reset command has RunE function set", func(t *testing.T) {
		cmd := gatewayResetCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayCleanCmd(t *testing.T) {
	t.Run("clean command has correct use", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		assert.Equal(t, "clean", cmd.Use)
		assert.Contains(t, cmd.Short, "Destructively")
		assert.Contains(t, cmd.Short, "remove")
	})

	t.Run("clean has force flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("clean has y shorthand flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		flag := cmd.Flags().Lookup("y")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("clean has yes flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("clean command has RunE function set", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestGatewayCommandFlags(t *testing.T) {
	t.Run("reset and clean share force flags", func(t *testing.T) {
		resetCmd := gatewayResetCmd()
		cleanCmd := gatewayCleanCmd()

		resetForce := resetCmd.Flags().Lookup("force")
		cleanForce := cleanCmd.Flags().Lookup("force")

		assert.NotNil(t, resetForce)
		assert.NotNil(t, cleanForce)
	})
}
