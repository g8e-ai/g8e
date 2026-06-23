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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/tools/chaos"
)

var (
	chaosCount   int
	chaosDataDir string
	chaosPKIDir  string
)

func chaosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "Generate realistic governance events against the local g8e audit stack",
		Long: `chaos generates a realistic distribution of governance events against
the local g8e audit stack. It bypasses network/TLS by driving the
TransactionVerifier + Actuator stack directly in-process, which is the same
path exercised by the live Operator when payloads arrive over pub/sub.

Distribution:
  70%  Good Actor  – valid sig, safe intent (FS_LIST)       → EXECUTED
  20%  Prompt Inj  – valid sig, L1 forbidden cmd (sudo/rm)  → REJECTED (L1)
  10%  MitM        – corrupted transaction hash              → REJECTED (hash mismatch)`,
		RunE: runChaos,
	}

	cmd.Flags().IntVar(&chaosCount, "count", 100, "number of payloads to fire")
	cmd.Flags().StringVar(&chaosDataDir, "data-dir", "", "audit vault data dir (default: <project-root>/"+paths.Infra.TestVaultDir+"/<timestamp>)")
	cmd.Flags().StringVar(&chaosPKIDir, "pki-dir", "", "PKI dir for trusted_signers (default: <cwd>/"+paths.Infra.PkiDir+")")

	return cmd
}

func runChaos(cmd *cobra.Command, args []string) error {
	cfg := chaos.Config{
		Count:   chaosCount,
		DataDir: chaosDataDir,
		PKIDir:  chaosPKIDir,
	}
	if err := chaos.Run(cfg); err != nil {
		return fmt.Errorf("chaos: failed to run chaos test: %w", err)
	}
	return nil
}
