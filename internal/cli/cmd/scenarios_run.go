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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/scenarios"
)

var (
	harnessConfigPath    string
	harnessMTLSURL       string
	harnessPublicURL     string
	harnessCert          string
	harnessKey           string
	harnessCA            string
	harnessAPIKey        string
	harnessSessionID     string
	harnessOutDir        string
	harnessL3Mode        string
	harnessEnsemble      int
	harnessVerbose       bool
	harnessPhase         string
	harnessConsensusSeed string
	harnessTribunalID    string
)

func demosScenariosRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flags] [scenario ...]",
		Short: "Run scenarios against a real Gateway/Operator",
		RunE:  runAgentHarness,
	}

	cmd.Flags().StringVar(&harnessConfigPath, "config", "", "JSON config overlay")
	cmd.Flags().StringVar(&harnessMTLSURL, "mtls-url", "", "Gateway mTLS surface")
	cmd.Flags().StringVar(&harnessPublicURL, "public-url", "", "Gateway public surface for OOB approve")
	cmd.Flags().StringVar(&harnessCert, "cert", "", "client cert PEM")
	cmd.Flags().StringVar(&harnessKey, "key", "", "client key PEM")
	cmd.Flags().StringVar(&harnessCA, "ca", "", "gateway CA bundle PEM")
	cmd.Flags().StringVar(&harnessAPIKey, "api-key", "", "operator API key for MCP/A2A surface")
	cmd.Flags().StringVar(&harnessSessionID, "operator-session", "", "scope audit to a specific Operator session")
	cmd.Flags().StringVar(&harnessOutDir, "out", "", "report output dir")
	cmd.Flags().StringVar(&harnessL3Mode, "l3-mode", "", "mock|suspend")
	cmd.Flags().IntVar(&harnessEnsemble, "ensemble", 3, "mock consensus voters")
	cmd.Flags().BoolVar(&harnessVerbose, "verbose", false, "echo each request/response")
	cmd.Flags().StringVar(&harnessPhase, "phase", "all", "doctrine|consensus|notary|all")
	cmd.Flags().StringVar(&harnessConsensusSeed, "consensus-seed", "", "hex-encoded Ed25519 seed for deterministic ensemble key (or path to seed file)")
	cmd.Flags().StringVar(&harnessTribunalID, "tribunal-id", "", "TribunalPolicy ID for L2 consensus (defaults to test-tribunal)")

	return cmd
}

func runAgentHarness(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	applyAgentHarnessFlags(&cfg)

	if harnessConfigPath != "" {
		if err := cfg.LoadFile(harnessConfigPath); err != nil {
			return fmt.Errorf("scenarios run: load config: %w", err)
		}
	}

	names := args
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		return fmt.Errorf("scenarios run: client: %w", err)
	}

	selected := selectAgentHarnessScenarios(harnessPhase, names)
	if len(selected) == 0 {
		return fmt.Errorf("scenarios run: %w", constants.ErrHarnessNoScenarios)
	}

	if needsGovKit(selected) {
		if err := setupGovKit(ctx, client, cfg); err != nil {
			return fmt.Errorf("scenarios run: gov kit setup: %w", err)
		}
	}

	results := make([]scenarios.Result, 0, len(selected))
	for _, s := range selected {
		results = append(results, scenarios.Execute(ctx, client, s))
	}

	opSession := cfg.OperatorSessionID
	if opSession == "" {
		opSession = client.DiscoverOperatorSession(ctx)
	}
	if export, err := client.ExportReceipts(ctx, opSession); err == nil && len(export) > 0 {
		if mkErr := os.MkdirAll(cfg.OutDir, constants.PermDirStandard); mkErr != nil {
			return fmt.Errorf("scenarios run: create output dir: %w", mkErr)
		}
		if writeErr := os.WriteFile(filepath.Join(cfg.OutDir, constants.ReceiptsExportFilename), export, constants.PermFilePublic); writeErr != nil {
			return fmt.Errorf("scenarios run: write receipts export: %w", writeErr)
		}
	}

	printAgentHarnessSummary(cmd.OutOrStdout(), results)
	return failedScenariosError(results)
}

func failedScenariosError(results []scenarios.Result) error {
	var failed []string
	for _, r := range results {
		if !r.OK {
			failed = append(failed, r.Name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("scenarios run: %d/%d scenarios failed: %s", len(failed), len(results), strings.Join(failed, ", "))
	}
	return nil
}

func applyAgentHarnessFlags(cfg *config.Config) {
	if harnessMTLSURL != "" {
		cfg.MTLSBaseURL = harnessMTLSURL
	}
	if harnessPublicURL != "" {
		cfg.PublicBaseURL = harnessPublicURL
	}
	if harnessCert != "" {
		cfg.Auth.ClientCert = harnessCert
	}
	if harnessKey != "" {
		cfg.Auth.ClientKey = harnessKey
	}
	if harnessCA != "" {
		cfg.Auth.CABundle = harnessCA
	}
	if harnessAPIKey != "" {
		cfg.Auth.APIKey = harnessAPIKey
	}
	if harnessSessionID != "" {
		cfg.OperatorSessionID = harnessSessionID
	}
	if harnessOutDir != "" {
		cfg.OutDir = harnessOutDir
	}
	if harnessL3Mode != "" {
		cfg.L3Mode = harnessL3Mode
	}
	if harnessEnsemble != 0 {
		cfg.EnsembleSize = harnessEnsemble
	}
	if harnessVerbose {
		cfg.Verbose = harnessVerbose
	}
	if harnessConsensusSeed != "" {
		cfg.ConsensusSeed = harnessConsensusSeed
	}
	if harnessTribunalID != "" {
		cfg.TribunalID = harnessTribunalID
	}
}

func selectAgentHarnessScenarios(phase string, names []string) []scenarios.Scenario {
	all := scenarios.Registry()
	if len(names) > 0 {
		var out []scenarios.Scenario
		for _, n := range names {
			if s, ok := scenarios.Find(n); ok {
				out = append(out, s)
			} else {
				fmt.Fprintf(os.Stderr, "warning: unknown scenario %q\n", n)
			}
		}
		return out
	}
	var out []scenarios.Scenario
	for _, s := range all {
		switch phase {
		case "doctrine":
			if s.RequiresPosture == scenarios.Doctrine {
				out = append(out, s)
			}
		case "consensus":
			if s.RequiresPosture == scenarios.Doctrine || s.RequiresPosture == scenarios.Consensus {
				out = append(out, s)
			}
		case "notary":
			if s.RequiresPosture == scenarios.Consensus || s.RequiresPosture == scenarios.Notary {
				out = append(out, s)
			}
		default:
			out = append(out, s)
		}
	}
	return out
}

func needsGovKit(ss []scenarios.Scenario) bool {
	for _, s := range ss {
		if s.RequiresPosture == scenarios.Consensus || s.RequiresPosture == scenarios.Notary ||
			strings.HasPrefix(s.Name, scenarios.DhsScenarioPrefix) || strings.HasPrefix(s.Name, scenarios.FedRAMPScenarioPrefix) {
			return true
		}
	}
	return false
}

func setupGovKit(ctx context.Context, client *clientpkg.Client, cfg config.Config) error {
	var ens *clientpkg.Ensemble
	var err error
	if cfg.ConsensusSeed != "" {
		seedHex := cfg.ConsensusSeed
		if _, statErr := os.Stat(seedHex); statErr == nil {
			data, readErr := os.ReadFile(seedHex)
			if readErr != nil {
				return fmt.Errorf("read consensus seed file: %w", readErr)
			}
			seedHex = strings.TrimSpace(string(data))
		}
		ens, err = clientpkg.NewEnsembleFromSeed(cfg.ConsensusKeyID, cfg.EnsembleSize, seedHex)
		if err != nil {
			return fmt.Errorf("setup gov kit: ensemble from seed: %w", err)
		}
	} else {
		ens, err = clientpkg.NewEnsemble(cfg.ConsensusKeyID, cfg.EnsembleSize)
		if err != nil {
			return fmt.Errorf("setup gov kit: ensemble: %w", err)
		}
	}
	if cfg.TribunalID != "" {
		ens.TribunalID = cfg.TribunalID
	}

	// If the consensus seed is a file path, try to load tribunal member app IDs
	// from a sibling tribunal-bootstrap.json so the ensemble votes with the
	// correct member key IDs (multi-member quorum support).
	if cfg.ConsensusSeed != "" {
		seedDir := filepath.Dir(cfg.ConsensusSeed)
		bootstrapPath := filepath.Join(seedDir, "tribunal-bootstrap.json")
		if data, readErr := os.ReadFile(bootstrapPath); readErr == nil {
			var boot struct {
				MemberAppIDs []string `json:"member_app_ids"`
			}
			if json.Unmarshal(data, &boot) == nil && len(boot.MemberAppIDs) > 0 {
				ens.MemberKeyIDs = boot.MemberAppIDs
			}
		}
	}

	prin, err := clientpkg.NewPrincipal(cfg.PrincipalKeyID)
	if err != nil {
		return fmt.Errorf("setup gov kit: principal: %w", err)
	}
	opID := cfg.OperatorSessionID
	opSessionID := ""
	if opID == "" {
		opID, opSessionID = client.DiscoverOperator(ctx)
	} else {
		opSessionID = opID
	}
	scenarios.SetGovKit(&scenarios.GovKit{
		Ensemble: ens, Principal: prin, L3Mode: cfg.L3Mode,
		OperatorID: opID, OperatorSessionID: opSessionID,
	})

	if len(ens.MemberKeyIDs) > 0 {
		for _, appID := range ens.MemberKeyIDs {
			if err := client.RegisterSigner(ctx, appID, ens.PubHex(), "consensus"); err != nil {
				fmt.Fprintf(os.Stderr, "warning: consensus signer registration %s: %v (non-fatal under doctrine)\n", appID, err)
			}
		}
	} else {
		if err := client.RegisterSigner(ctx, ens.KeyID, ens.PubHex(), "consensus"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: consensus signer registration: %v (non-fatal under doctrine)\n", err)
		}
	}
	if err := client.RegisterSigner(ctx, prin.KeyID, prin.PubHex(), "principal"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: principal signer registration: %v (non-fatal under doctrine)\n", err)
	}
	return nil
}

func printAgentHarnessSummary(w io.Writer, results []scenarios.Result) {
	fmt.Fprintln(w, "\n── summary ──")
	ok := 0
	for _, r := range results {
		status := "FAIL"
		if r.OK {
			status = string(constants.GatewayModeStatusOK)
			ok++
		}
		fmt.Fprintf(w, "  %-18s %-9s %-18s %s\n", r.Name, r.RequiresPosture, r.Persona, status)
	}
	fmt.Fprintf(w, "\n%d/%d scenarios ok\n", ok, len(results))
}
