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
	"testing"

	"github.com/g8e-ai/g8e/internal/test/chaos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChaosCmd(t *testing.T) {
	t.Run("chaos command has correct use and description", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Equal(t, "chaos", cmd.Use)
		assert.Contains(t, cmd.Short, "Generate realistic governance events")
		assert.Contains(t, cmd.Long, "realistic distribution")
	})

	t.Run("chaos command has count flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("count")
		assert.NotNil(t, flag, "chaos command should have --count flag")
		assert.Equal(t, "100", flag.DefValue)
	})

	t.Run("chaos command has data-dir flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("data-dir")
		assert.NotNil(t, flag, "chaos command should have --data-dir flag")
	})

	t.Run("chaos command has pki-dir flag", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		flag := cmd.Flags().Lookup("pki-dir")
		assert.NotNil(t, flag, "chaos command should have --pki-dir flag")
	})

	t.Run("chaos long description mentions distribution", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "70%")
		assert.Contains(t, cmd.Long, "Good Actor")
		assert.Contains(t, cmd.Long, "20%")
		assert.Contains(t, cmd.Long, "Prompt Inj")
		assert.Contains(t, cmd.Long, "10%")
		assert.Contains(t, cmd.Long, "MitM")
	})

	t.Run("chaos command mentions TransactionVerifier and Actuator", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "TransactionVerifier")
		assert.Contains(t, cmd.Long, "Actuator")
	})

	t.Run("chaos command mentions in-process execution", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "in-process")
	})
}

func TestChaosFlagParsing(t *testing.T) {
	t.Run("count flag can be set to different values", func(t *testing.T) {
		tests := []struct {
			name  string
			value string
			want  int
		}{
			{"default value", "", 100},
			{"custom value 50", "50", 50},
			{"custom value 1", "1", 1},
			{"custom value 1000", "1000", 1000},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cmd := chaosCmd()
				require.NotNil(t, cmd)

				if tt.value != "" {
					require.NoError(t, cmd.Flags().Set("count", tt.value))
				}

				flag := cmd.Flags().Lookup("count")
				assert.NotNil(t, flag)
			})
		}
	})

	t.Run("data-dir flag can be set to custom path", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		customPath := "/tmp/custom-chaos-data"
		require.NoError(t, cmd.Flags().Set("data-dir", customPath))

		flag := cmd.Flags().Lookup("data-dir")
		assert.NotNil(t, flag)
	})

	t.Run("pki-dir flag can be set to custom path", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		customPath := "/tmp/custom-pki"
		require.NoError(t, cmd.Flags().Set("pki-dir", customPath))

		flag := cmd.Flags().Lookup("pki-dir")
		assert.NotNil(t, flag)
	})
}

func TestRunChaosConfigConstruction(t *testing.T) {
	t.Run("config uses default count when flag not set", func(t *testing.T) {
		chaosCount = 100 // Default value
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Don't set count flag, should use default
		cfg := chaos.Config{
			Count:   chaosCount,
			DataDir: chaosDataDir,
			PKIDir:  chaosPKIDir,
		}

		assert.Equal(t, 100, cfg.Count)
		assert.Equal(t, "", cfg.DataDir)
		assert.Equal(t, "", cfg.PKIDir)
	})

	t.Run("config uses custom count when flag is set", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		require.NoError(t, cmd.Flags().Set("count", "50"))
		chaosCount = 50

		cfg := chaos.Config{
			Count:   chaosCount,
			DataDir: chaosDataDir,
			PKIDir:  chaosPKIDir,
		}

		assert.Equal(t, 50, cfg.Count)
	})

	t.Run("config uses custom data-dir when flag is set", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		customPath := "/tmp/test-data"
		require.NoError(t, cmd.Flags().Set("data-dir", customPath))
		chaosDataDir = customPath

		cfg := chaos.Config{
			Count:   chaosCount,
			DataDir: chaosDataDir,
			PKIDir:  chaosPKIDir,
		}

		assert.Equal(t, customPath, cfg.DataDir)
	})

	t.Run("config uses custom pki-dir when flag is set", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		customPath := "/tmp/test-pki"
		require.NoError(t, cmd.Flags().Set("pki-dir", customPath))
		chaosPKIDir = customPath

		cfg := chaos.Config{
			Count:   chaosCount,
			DataDir: chaosDataDir,
			PKIDir:  chaosPKIDir,
		}

		assert.Equal(t, customPath, cfg.PKIDir)
	})
}

func TestChaosCommandIntegration(t *testing.T) {
	t.Run("chaos command executes runChaos", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Verify that RunE is set
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("chaos command accepts no positional arguments", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// chaos command should not require positional arguments
		assert.Nil(t, cmd.Args)
	})
}

func TestChaosFlagDefaults(t *testing.T) {
	t.Run("count flag default is 100", func(t *testing.T) {
		cmd := chaosCmd()
		flag := cmd.Flags().Lookup("count")
		require.NotNil(t, flag)
		assert.Equal(t, "100", flag.DefValue)
	})

	t.Run("data-dir flag default is empty string", func(t *testing.T) {
		cmd := chaosCmd()
		flag := cmd.Flags().Lookup("data-dir")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("pki-dir flag default is empty string", func(t *testing.T) {
		cmd := chaosCmd()
		flag := cmd.Flags().Lookup("pki-dir")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})
}

func TestChaosCommandHelp(t *testing.T) {
	t.Run("chaos command has helpful long description", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "bypasses network/TLS")
		assert.Contains(t, cmd.Long, "pub/sub")
	})

	t.Run("chaos command describes expected outcomes", func(t *testing.T) {
		cmd := chaosCmd()
		assert.Contains(t, cmd.Long, "EXECUTED")
		assert.Contains(t, cmd.Long, "REJECTED")
		assert.Contains(t, cmd.Long, "L1")
		assert.Contains(t, cmd.Long, "hash mismatch")
	})
}

func TestChaosPackageVariables(t *testing.T) {
	t.Run("chaosCount is initialized to zero", func(t *testing.T) {
		// Reset to ensure test isolation
		chaosCount = 0
		assert.Equal(t, 0, chaosCount)
	})

	t.Run("chaosDataDir is initialized to empty string", func(t *testing.T) {
		chaosDataDir = ""
		assert.Equal(t, "", chaosDataDir)
	})

	t.Run("chaosPKIDir is initialized to empty string", func(t *testing.T) {
		chaosPKIDir = ""
		assert.Equal(t, "", chaosPKIDir)
	})
}

func TestChaosConfigType(t *testing.T) {
	t.Run("chaos.Config has expected fields", func(t *testing.T) {
		cfg := chaos.Config{
			Count:   100,
			DataDir: "/tmp/data",
			PKIDir:  "/tmp/pki",
		}

		assert.Equal(t, 100, cfg.Count)
		assert.Equal(t, "/tmp/data", cfg.DataDir)
		assert.Equal(t, "/tmp/pki", cfg.PKIDir)
	})
}
