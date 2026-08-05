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
	"io"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/scenarios"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectAgentHarnessScenarios(t *testing.T) {
	t.Run("returns all scenarios when phase is 'all' and no names specified", func(t *testing.T) {
		result := selectAgentHarnessScenarios("all", nil)
		all := scenarios.Registry()
		assert.Len(t, result, len(all))
	})

	t.Run("filters to doctrine only when phase is 'doctrine'", func(t *testing.T) {
		result := selectAgentHarnessScenarios("doctrine", nil)
		for _, s := range result {
			assert.Equal(t, scenarios.Doctrine, s.RequiresPosture)
		}
	})

	t.Run("filters to consensus+notary when phase is 'notary'", func(t *testing.T) {
		result := selectAgentHarnessScenarios("notary", nil)
		for _, s := range result {
			assert.True(t, s.RequiresPosture == scenarios.Consensus || s.RequiresPosture == scenarios.Notary,
				"phase 'notary' should only return consensus or notary scenarios")
		}
	})

	t.Run("returns specific scenarios when names are provided", func(t *testing.T) {
		all := scenarios.Registry()
		require.NotEmpty(t, all)
		result := selectAgentHarnessScenarios("all", []string{all[0].Name})
		assert.Len(t, result, 1)
		assert.Equal(t, all[0].Name, result[0].Name)
	})
}

func TestNeedsGovKit(t *testing.T) {
	t.Run("returns false for empty slice", func(t *testing.T) {
		assert.False(t, needsGovKit(nil))
	})

	t.Run("returns false for doctrine-only scenarios", func(t *testing.T) {
		all := scenarios.Registry()
		var doctrineOnly []scenarios.Scenario
		for _, s := range all {
			if s.RequiresPosture == scenarios.Doctrine &&
				!strings.HasPrefix(s.Name, scenarios.DhsScenarioPrefix) &&
				!strings.HasPrefix(s.Name, scenarios.FedRAMPScenarioPrefix) {
				doctrineOnly = append(doctrineOnly, s)
			}
		}
		if len(doctrineOnly) > 0 {
			assert.False(t, needsGovKit(doctrineOnly))
		}
	})

	t.Run("returns true when consensus scenario present", func(t *testing.T) {
		all := scenarios.Registry()
		var hasConsensus []scenarios.Scenario
		for _, s := range all {
			if s.RequiresPosture == scenarios.Consensus {
				hasConsensus = append(hasConsensus, s)
				break
			}
		}
		if len(hasConsensus) > 0 {
			assert.True(t, needsGovKit(hasConsensus))
		}
	})

	t.Run("returns true when notary scenario present", func(t *testing.T) {
		all := scenarios.Registry()
		var hasNotary []scenarios.Scenario
		for _, s := range all {
			if s.RequiresPosture == scenarios.Notary {
				hasNotary = append(hasNotary, s)
				break
			}
		}
		if len(hasNotary) > 0 {
			assert.True(t, needsGovKit(hasNotary))
		}
	})
}

func TestPrintAgentHarnessSummary(t *testing.T) {
	t.Run("prints summary with results without panic", func(t *testing.T) {
		results := []scenarios.Result{
			{Name: "test1", OK: true, RequiresPosture: scenarios.Doctrine, Persona: "agent1"},
			{Name: "test2", OK: false, RequiresPosture: scenarios.Doctrine, Persona: "agent2"},
		}

		assert.NotPanics(t, func() {
			printAgentHarnessSummary(io.Discard, results)
		})
	})

	t.Run("prints summary with empty results without panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			printAgentHarnessSummary(io.Discard, nil)
		})
	})
}

func TestSelectAgentHarnessScenarios_Consensus(t *testing.T) {
	t.Run("consensus phase returns doctrine+consensus but not notary", func(t *testing.T) {
		result := selectAgentHarnessScenarios("consensus", nil)
		assert.NotEmpty(t, result)
		for _, s := range result {
			assert.True(t, s.RequiresPosture == scenarios.Doctrine || s.RequiresPosture == scenarios.Consensus,
				"consensus phase should only return doctrine or consensus scenarios, got %s for %s",
				s.RequiresPosture, s.Name)
		}
		for _, s := range result {
			assert.NotEqual(t, scenarios.Notary, s.RequiresPosture,
				"consensus phase must NOT return notary scenarios")
		}
	})

	t.Run("consensus phase includes dhs scenarios", func(t *testing.T) {
		result := selectAgentHarnessScenarios("consensus", nil)
		var dhsCount int
		for _, s := range result {
			if strings.HasPrefix(s.Name, scenarios.DhsScenarioPrefix) {
				dhsCount++
			}
		}
		assert.Greater(t, dhsCount, 0, "consensus phase should include DHS scenarios")
	})
}

func TestDemosScenariosRunCmd_IdentityFlags(t *testing.T) {
	cmd := demosScenariosRunCmd()

	t.Run("has --user-id flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("user-id")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
		assert.Contains(t, flag.Usage, "user_id")
	})

	t.Run("has --cli-session-id flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("cli-session-id")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
		assert.Contains(t, flag.Usage, "session")
	})
}

func TestApplyAgentHarnessFlags_IdentityFields(t *testing.T) {
	t.Run("propagates user-id and cli-session-id to config", func(t *testing.T) {
		harnessUserID = "test-user"
		harnessCLISessionID = "test-session"
		defer func() {
			harnessUserID = ""
			harnessCLISessionID = ""
		}()

		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "test-user", cfg.UserID)
		assert.Equal(t, "test-session", cfg.CLISessionID)
	})

	t.Run("leaves identity empty when flags unset", func(t *testing.T) {
		harnessUserID = ""
		harnessCLISessionID = ""

		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Empty(t, cfg.UserID)
		assert.Empty(t, cfg.CLISessionID)
	})
}

func TestDemosScenariosRunCmd_CLICertFlags(t *testing.T) {
	cmd := demosScenariosRunCmd()

	t.Run("has --cli-cert flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("cli-cert")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
		assert.Contains(t, flag.Usage, "notary")
	})

	t.Run("has --cli-key flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("cli-key")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
		assert.Contains(t, flag.Usage, "notary")
	})

	t.Run("has --cli-ca flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("cli-ca")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
		assert.Contains(t, flag.Usage, "CLI")
	})
}

func TestApplyAgentHarnessFlags_CLIAuth(t *testing.T) {
	t.Run("propagates cli-cert/cli-key/cli-ca to config.CLIAuth", func(t *testing.T) {
		harnessCLICert = "/path/to/cli.crt"
		harnessCLIKey = "/path/to/cli.key"
		harnessCLICA = "/path/to/ca-bundle.pem"
		defer func() {
			harnessCLICert = ""
			harnessCLIKey = ""
			harnessCLICA = ""
		}()

		cfg := config.Default()
		applyAgentHarnessFlags(&cfg)
		assert.Equal(t, "/path/to/cli.crt", cfg.CLIAuth.ClientCert)
		assert.Equal(t, "/path/to/cli.key", cfg.CLIAuth.ClientKey)
		assert.Equal(t, "/path/to/ca-bundle.pem", cfg.CLIAuth.CABundle)
	})

	t.Run("leaves CLIAuth empty when flags unset", func(t *testing.T) {
		harnessCLICert = ""
		harnessCLIKey = ""
		harnessCLICA = ""

		cfg := config.Config{}
		applyAgentHarnessFlags(&cfg)
		assert.Empty(t, cfg.CLIAuth.ClientCert)
		assert.Empty(t, cfg.CLIAuth.ClientKey)
		assert.Empty(t, cfg.CLIAuth.CABundle)
	})
}
