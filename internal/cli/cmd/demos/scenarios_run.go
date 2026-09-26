// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package demos

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
)

var (
	HarnessConfigPath   string
	HarnessMTLSURL      string
	HarnessPublicURL    string
	harnessApprovalURL  string
	harnessEnsembleURL  string
	HarnessCert         string
	HarnessKey          string
	HarnessCA           string
	HarnessAPIKey       string
	harnessCLICert      string
	harnessCLIKey       string
	harnessCLICA        string
	HarnessSessionID    string
	harnessUserID       string
	harnessCLISessionID string
	HarnessOutDir       string
	HarnessVerbose      bool
	HarnessPhase        string
)

func demosScenariosRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flags] [scenario ...]",
		Short: "Run scenarios against a real Gateway/Operator",
		RunE:  RunAgentHarness,
	}

	cmd.Flags().StringVar(&HarnessConfigPath, "config", "", "JSON config overlay")
	cmd.Flags().StringVar(&HarnessMTLSURL, "mtls-url", "", "Gateway mTLS surface")
	cmd.Flags().StringVar(&HarnessPublicURL, "public-url", "", "Gateway public surface for OOB approve (must be reachable from the harness process)")
	cmd.Flags().StringVar(&harnessApprovalURL, "approval-url", "", "Host-reachable base URL for the printed human approval link (defaults to --public-url)")
	cmd.Flags().StringVar(&harnessEnsembleURL, "ensemble-url", "", "Ensemble (g8ee) HTTP surface for ensemble chat scenarios")
	cmd.Flags().StringVar(&HarnessCert, "cert", "", "client cert PEM")
	cmd.Flags().StringVar(&HarnessKey, "key", "", "client key PEM")
	cmd.Flags().StringVar(&HarnessCA, "ca", "", "gateway CA bundle PEM")
	cmd.Flags().StringVar(&harnessCLICert, "cli-cert", "", "host CLI client cert PEM for notary submits")
	cmd.Flags().StringVar(&harnessCLIKey, "cli-key", "", "host CLI client key PEM for notary submits")
	cmd.Flags().StringVar(&harnessCLICA, "cli-ca", "", "gateway CA bundle PEM for CLI-cert client (defaults to --ca)")
	cmd.Flags().StringVar(&HarnessAPIKey, "api-key", "", "operator API key for MCP/A2A surface")
	cmd.Flags().StringVar(&HarnessSessionID, "operator-session", "", "scope audit to a specific Operator session")
	cmd.Flags().StringVar(&harnessUserID, "user-id", "", "host CLI user_id for SSE approval subscription")
	cmd.Flags().StringVar(&harnessCLISessionID, "cli-session-id", "", "host CLI session id for X-CLI-Session-ID submit header")
	cmd.Flags().StringVar(&HarnessOutDir, "out", "", "report output dir")
	cmd.Flags().BoolVar(&HarnessVerbose, "verbose", false, "echo each request/response")
	cmd.Flags().StringVar(&HarnessPhase, "phase", "all", "scenario suite: doctrine|consensus|notary|all (ratify has no dedicated suite)")

	return cmd
}

func RunAgentHarness(cmd *cobra.Command, args []string) error {
	cfg := config.Default()
	applyAgentHarnessFlags(&cfg)

	if HarnessConfigPath != "" {
		if err := cfg.LoadFile(HarnessConfigPath); err != nil {
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

	selected := selectAgentHarnessScenarios(HarnessPhase, names)
	if len(selected) == 0 {
		return fmt.Errorf("scenarios run: %w", constants.ErrHarnessNoScenarios)
	}

	if needsGovKit(selected) {
		if err := setupGovKit(ctx, client, cfg, selected); err != nil {
			return fmt.Errorf("scenarios run: gov kit setup: %w", err)
		}
	}

	results := make([]scenarios.Result, 0, len(selected))
	for _, s := range selected {
		result := scenarios.Execute(ctx, client, s)
		if result.RunID != "" && result.ScenarioID != "" && len(result.Receipts) > 0 {
			if err := scenarios.ResolveReceiptEvidence(ctx, client, &result); err != nil {
				result.OK = false
				if result.Err == "" {
					result.Err = err.Error()
				} else {
					result.Err += "; " + err.Error()
				}
			}
		}
		results = append(results, result)
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

	if output.JSONEnabled(cmd) {
		if err := output.WriteJSON(cmd.OutOrStdout(), results); err != nil {
			return fmt.Errorf("scenarios run: encode JSON results: %w", err)
		}
	} else {
		printAgentHarnessSummary(cmd.OutOrStdout(), results)
	}
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
	if HarnessMTLSURL != "" {
		cfg.MTLSBaseURL = HarnessMTLSURL
	}
	if HarnessPublicURL != "" {
		cfg.PublicBaseURL = HarnessPublicURL
	}
	if harnessApprovalURL != "" {
		cfg.ApprovalDisplayURL = harnessApprovalURL
	}
	if harnessEnsembleURL != "" {
		cfg.EnsembleBaseURL = harnessEnsembleURL
	}
	if HarnessCert != "" {
		cfg.Auth.ClientCert = HarnessCert
	}
	if HarnessKey != "" {
		cfg.Auth.ClientKey = HarnessKey
	}
	if HarnessCA != "" {
		cfg.Auth.CABundle = HarnessCA
	}
	if HarnessAPIKey != "" {
		cfg.Auth.APIKey = HarnessAPIKey
	}
	if HarnessSessionID != "" {
		cfg.OperatorSessionID = HarnessSessionID
	}
	if harnessUserID != "" {
		cfg.UserID = harnessUserID
	}
	if harnessCLISessionID != "" {
		cfg.CLISessionID = harnessCLISessionID
	}
	if HarnessOutDir != "" {
		cfg.OutDir = HarnessOutDir
	}
	if HarnessVerbose {
		cfg.Verbose = HarnessVerbose
	}
	if harnessCLICert != "" {
		cfg.CLIAuth.ClientCert = harnessCLICert
	}
	if harnessCLIKey != "" {
		cfg.CLIAuth.ClientKey = harnessCLIKey
	}
	if harnessCLICA != "" {
		cfg.CLIAuth.CABundle = harnessCLICA
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
			strings.HasPrefix(s.Name, scenarios.DhsScenarioPrefix) || strings.HasPrefix(s.Name, scenarios.FedRAMPScenarioPrefix) ||
			strings.HasPrefix(s.Name, scenarios.EnsembleScenarioPrefix) {
			return true
		}
	}
	return false
}

func setupGovKit(ctx context.Context, client *clientpkg.Client, cfg config.Config, selected []scenarios.Scenario) error {
	opID := cfg.OperatorSessionID
	opSessionID := ""
	if opID == "" {
		opID, opSessionID = client.DiscoverOperator(ctx)
	} else {
		opSessionID = opID
	}

	gk := &scenarios.GovKit{
		OperatorID:        opID,
		OperatorSessionID: opSessionID,
		UserID:            cfg.UserID,
		CLISessionID:      cfg.CLISessionID,
	}
	scenarios.SetGovKit(gk)
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
		if r.Err != "" {
			fmt.Fprintf(w, "    error: %s\n", r.Err)
		}
		for _, n := range r.Notes {
			fmt.Fprintf(w, "    note: %s\n", n)
		}
	}
	fmt.Fprintf(w, "\n%d/%d scenarios ok\n", ok, len(results))
}
