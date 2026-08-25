// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemosScenariosCmd(t *testing.T) {
	t.Run("scenarios command has correct Use and description", func(t *testing.T) {
		cmd := demosScenariosCmd()
		assert.Equal(t, "scenarios", cmd.Use)
		assert.Contains(t, cmd.Short, "List and run demo scenarios")
	})

	t.Run("scenarios has expected subcommands", func(t *testing.T) {
		cmd := demosScenariosCmd()
		require.NotNil(t, cmd)

		expectedSubcommands := []string{"list", "run"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "scenarios command should have %s subcommand", subcmd)
		}
	})
}

func TestDemosScenariosRunCmd(t *testing.T) {
	t.Run("scenarios run command has correct use", func(t *testing.T) {
		cmd := demosScenariosRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Run scenarios")
	})

	t.Run("scenarios run has required flags", func(t *testing.T) {
		cmd := demosScenariosRunCmd()
		require.NotNil(t, cmd)

		flags := []string{"config", "mtls-url", "public-url", "approval-url", "cert", "key", "ca", "api-key", "operator-session", "out", "verbose", "phase"}

		for _, flagName := range flags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "scenarios run should have --%s flag", flagName)
		}
	})

	t.Run("scenarios run phase flag has default value", func(t *testing.T) {
		cmd := demosScenariosRunCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("phase")
		require.NotNil(t, flag)
		assert.Equal(t, "all", flag.DefValue)
	})
}

func TestApplyAgentHarnessFlags(t *testing.T) {
	t.Run("applyAgentHarnessFlags sets MTLS URL", func(t *testing.T) {
		harnessMTLSURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttp)
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttp), cfg.MTLSBaseURL)
		harnessMTLSURL = ""
	})

	t.Run("applyAgentHarnessFlags sets public URL", func(t *testing.T) {
		harnessPublicURL = "https://example.com:" + strconv.Itoa(constants.Ports.OperatorHttps)
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "https://example.com:"+strconv.Itoa(constants.Ports.OperatorHttps), cfg.PublicBaseURL)
		harnessPublicURL = ""
	})

	t.Run("applyAgentHarnessFlags sets approval display URL", func(t *testing.T) {
		harnessApprovalURL = "https://localhost:8450"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "https://localhost:8450", cfg.ApprovalDisplayURL)
		harnessApprovalURL = ""
	})

	t.Run("applyAgentHarnessFlags sets cert", func(t *testing.T) {
		harnessCert = "/path/to/cert.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/cert.pem", cfg.Auth.ClientCert)
		harnessCert = ""
	})

	t.Run("applyAgentHarnessFlags sets key", func(t *testing.T) {
		harnessKey = "/path/to/key.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/key.pem", cfg.Auth.ClientKey)
		harnessKey = ""
	})

	t.Run("applyAgentHarnessFlags sets CA bundle", func(t *testing.T) {
		harnessCA = "/path/to/ca.pem"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/ca.pem", cfg.Auth.CABundle)
		harnessCA = ""
	})

	t.Run("applyAgentHarnessFlags sets API key", func(t *testing.T) {
		harnessAPIKey = "test-api-key"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "test-api-key", cfg.Auth.APIKey)
		harnessAPIKey = ""
	})

	t.Run("applyAgentHarnessFlags sets operator session ID", func(t *testing.T) {
		harnessSessionID = "session-123"
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "session-123", cfg.OperatorSessionID)
		harnessSessionID = ""
	})

	t.Run("applyAgentHarnessFlags sets out directory", func(t *testing.T) {
		testOutDir := testutil.TempDir(t)
		harnessOutDir = testOutDir
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, testOutDir, cfg.OutDir)
		harnessOutDir = ""
	})

	t.Run("applyAgentHarnessFlags sets verbose flag", func(t *testing.T) {
		harnessVerbose = true
		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.True(t, cfg.Verbose)
		harnessVerbose = false
	})
}

func TestFailedScenariosError(t *testing.T) {
	t.Run("all results OK returns nil", func(t *testing.T) {
		results := []scenarios.Result{
			{Name: "scenario-a", OK: true},
			{Name: "scenario-b", OK: true},
		}
		err := failedScenariosError(results)
		assert.NoError(t, err)
	})

	t.Run("all results failed returns error with all names", func(t *testing.T) {
		results := []scenarios.Result{
			{Name: "scenario-a", OK: false},
			{Name: "scenario-b", OK: false},
		}
		err := failedScenariosError(results)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "2/2 scenarios failed")
		assert.Contains(t, err.Error(), "scenario-a")
		assert.Contains(t, err.Error(), "scenario-b")
	})

	t.Run("mixed results returns error with only failed names", func(t *testing.T) {
		results := []scenarios.Result{
			{Name: "scenario-a", OK: true},
			{Name: "scenario-b", OK: false},
			{Name: "scenario-c", OK: true},
			{Name: "scenario-d", OK: false},
		}
		err := failedScenariosError(results)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "2/4 scenarios failed")
		assert.Contains(t, err.Error(), "scenario-b")
		assert.Contains(t, err.Error(), "scenario-d")
		assert.NotContains(t, err.Error(), "scenario-a")
		assert.NotContains(t, err.Error(), "scenario-c")
	})

	t.Run("empty results returns nil", func(t *testing.T) {
		results := []scenarios.Result{}
		err := failedScenariosError(results)
		assert.NoError(t, err)
	})

	t.Run("single failed result returns error", func(t *testing.T) {
		results := []scenarios.Result{
			{Name: "only-failed", OK: false},
		}
		err := failedScenariosError(results)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "1/1 scenarios failed")
		assert.Contains(t, err.Error(), "only-failed")
	})
}
