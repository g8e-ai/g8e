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

func agentHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent",
		Aliases: []string{"agent-harness"},
		Short:   "Universal agent harness for a real g8e Gateway/Operator",
		Long: `agent impersonates arbitrary AI tools and agents against a REAL g8e
Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A
protobuf, and official governance envelopes with mock consensus + principal
signing), then audits every result against the Operator's signed receipts.`,
	}

	cmd.AddCommand(agentHarnessListCmd())
	cmd.AddCommand(agentHarnessRunCmd())
	cmd.AddCommand(agentHarnessAuditCmd())

	return cmd
}

func agentHarnessListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available scenarios",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("scenarios (in run order):")
			for _, s := range scenarios.Registry() {
				cmd.Printf("  %-18s %-9s %-18s %s\n", s.Name, s.RequiresPosture, s.Persona.ID, s.Title)
			}
		},
	}
}

func agentHarnessRunCmd() *cobra.Command {
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

func agentHarnessAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit [flags]",
		Short: "Audit signed receipts from the Operator",
		RunE:  runAgentHarnessAudit,
	}

	cmd.Flags().StringVar(&harnessConfigPath, "config", "", "JSON config overlay")
	cmd.Flags().StringVar(&harnessMTLSURL, "mtls-url", "", "Gateway mTLS surface")
	cmd.Flags().StringVar(&harnessPublicURL, "public-url", "", "Gateway public surface")
	cmd.Flags().StringVar(&harnessCert, "cert", "", "client cert PEM")
	cmd.Flags().StringVar(&harnessKey, "key", "", "client key PEM")
	cmd.Flags().StringVar(&harnessCA, "ca", "", "gateway CA bundle PEM")
	cmd.Flags().StringVar(&harnessAPIKey, "api-key", "", "operator API key")
	cmd.Flags().StringVar(&harnessSessionID, "operator-session", "", "operator session id")
	cmd.Flags().StringVar(&harnessOutDir, "out", "", "report output dir")

	return cmd
}

func runAgentHarness(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	applyAgentHarnessFlags(&cfg)

	if harnessConfigPath != "" {
		if err := cfg.LoadFile(harnessConfigPath); err != nil {
			cmd.Printf("warning: config %s: %v\n", harnessConfigPath, err)
		}
	}

	names := args
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		return fmt.Errorf("agent run: client: %w", err)
	}

	selected := selectAgentHarnessScenarios(harnessPhase, names)
	if len(selected) == 0 {
		return fmt.Errorf("agent run: no scenarios selected")
	}

	if needsGovKit(selected) {
		if err := setupGovKit(ctx, client, cfg); err != nil {
			cmd.Printf("warning: gov kit setup: %v\n", err)
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
		_ = os.MkdirAll(cfg.OutDir, 0o755)
		_ = os.WriteFile(filepath.Join(cfg.OutDir, constants.ReceiptsExportFilename), export, 0o644)
	}

	printAgentHarnessSummary(cmd.OutOrStdout(), results, "", "")
	return nil
}

func runAgentHarnessAudit(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	applyAgentHarnessFlags(&cfg)

	if harnessConfigPath != "" {
		if err := cfg.LoadFile(harnessConfigPath); err != nil {
			cmd.Printf("warning: config %s: %v\n", harnessConfigPath, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		return fmt.Errorf("agent audit: client: %w", err)
	}
	opSession := cfg.OperatorSessionID
	if opSession == "" {
		opSession = client.DiscoverOperatorSession(ctx)
	}
	receipts, raw, err := client.AuditReceipts(ctx, opSession)
	if err != nil {
		return fmt.Errorf("agent audit: %w", err)
	}
	_ = os.MkdirAll(cfg.OutDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfg.OutDir, constants.ReceiptsFilename), raw, 0o644)
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "operator session: %s\n", opSession)
	fmt.Fprintf(w, "signed receipts: %d (raw written to %s/receipts.json)\n", len(receipts), cfg.OutDir)
	for _, r := range receipts {
		fmt.Fprintf(w, "  %-12s %-14s %s\n", trunc(r.TransactionHash, 12), r.ActionType, r.Status)
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
		if s.RequiresPosture == scenarios.Consensus || s.RequiresPosture == scenarios.Notary || strings.HasPrefix(s.Name, "dhs-") {
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
			return err
		}
	} else {
		ens, err = clientpkg.NewEnsemble(cfg.ConsensusKeyID, cfg.EnsembleSize)
		if err != nil {
			return err
		}
	}
	if cfg.TribunalID != "" {
		ens.TribunalID = cfg.TribunalID
	}
	prin, err := clientpkg.NewPrincipal(cfg.PrincipalKeyID)
	if err != nil {
		return err
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

	var errs []string
	if err := client.RegisterSigner(ctx, ens.KeyID, ens.PubHex(), "consensus"); err != nil {
		errs = append(errs, "consensus: "+err.Error())
	}
	if err := client.RegisterSigner(ctx, prin.KeyID, prin.PubHex(), "principal"); err != nil {
		errs = append(errs, "principal: "+err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("signer registration (non-fatal under doctrine): %s", strings.Join(errs, "; "))
	}
	return nil
}

func printAgentHarnessSummary(w io.Writer, results []scenarios.Result, jsonPath, mdPath string) {
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
	fmt.Fprintf(w, "report:  %s\n", mdPath)
	fmt.Fprintf(w, "json:    %s\n", jsonPath)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
