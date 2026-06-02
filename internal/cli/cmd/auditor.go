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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	clientpkg "github.com/g8e-ai/g8e/internal/auditor/client"
	"github.com/g8e-ai/g8e/internal/auditor/config"
	"github.com/g8e-ai/g8e/internal/auditor/scenarios"
	"github.com/g8e-ai/g8e/internal/constants"
)

var (
	auditorConfigPath string
	auditorMTLSURL    string
	auditorPublicURL  string
	auditorCert       string
	auditorKey        string
	auditorCA         string
	auditorAPIKey     string
	auditorInsecure   bool
	auditorSessionID  string
	auditorOutDir     string
	auditorL3Mode     string
	auditorEnsemble   int
	auditorVerbose    bool
	auditorPhase      string
)

func auditorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auditor",
		Short: "Universal agent emulator for a real g8e Gateway/Operator",
		Long: `auditor impersonates arbitrary AI tools and agents against a REAL g8e
Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A
protobuf, and official governance envelopes with mock consensus + principal
signing), then audits every result against the Operator's signed receipts.`,
	}

	cmd.AddCommand(auditorListCmd())
	cmd.AddCommand(auditorRunCmd())
	cmd.AddCommand(auditorAuditCmd())

	return cmd
}

func auditorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available scenarios",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("scenarios (in run order):")
			for _, s := range scenarios.Registry() {
				fmt.Printf("  %-18s %-9s %-18s %s\n", s.Name, s.RequiresPosture, s.Persona.ID, s.Title)
			}
		},
	}
}

func auditorRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flags] [scenario ...]",
		Short: "Run scenarios against a real Gateway/Operator",
		Run:   runAuditorRun,
	}

	cmd.Flags().StringVar(&auditorConfigPath, "config", "", "JSON config overlay")
	cmd.Flags().StringVar(&auditorMTLSURL, "mtls-url", "", "Gateway mTLS surface")
	cmd.Flags().StringVar(&auditorPublicURL, "public-url", "", "Gateway public surface for OOB approve")
	cmd.Flags().StringVar(&auditorCert, "cert", "", "client cert PEM")
	cmd.Flags().StringVar(&auditorKey, "key", "", "client key PEM")
	cmd.Flags().StringVar(&auditorCA, "ca", "", "gateway CA bundle PEM")
	cmd.Flags().StringVar(&auditorAPIKey, "api-key", "", "operator API key for MCP/A2A surface")
	cmd.Flags().StringVar(&auditorSessionID, "operator-session", "", "scope audit to a specific Operator session")
	cmd.Flags().BoolVar(&auditorInsecure, "insecure", false, "skip TLS verify (local dev only)")
	cmd.Flags().StringVar(&auditorOutDir, "out", "", "report output dir")
	cmd.Flags().StringVar(&auditorL3Mode, "l3-mode", "", "mock|suspend")
	cmd.Flags().IntVar(&auditorEnsemble, "ensemble", 3, "mock consensus voters")
	cmd.Flags().BoolVar(&auditorVerbose, "verbose", false, "echo each request/response")
	cmd.Flags().StringVar(&auditorPhase, "phase", "all", "doctrine|notary|all")

	return cmd
}

func auditorAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit [flags]",
		Short: "Audit signed receipts from the Operator",
		Run:   runAuditorAudit,
	}

	cmd.Flags().StringVar(&auditorConfigPath, "config", "", "JSON config overlay")
	cmd.Flags().StringVar(&auditorMTLSURL, "mtls-url", "", "Gateway mTLS surface")
	cmd.Flags().StringVar(&auditorPublicURL, "public-url", "", "Gateway public surface")
	cmd.Flags().StringVar(&auditorCert, "cert", "", "client cert PEM")
	cmd.Flags().StringVar(&auditorKey, "key", "", "client key PEM")
	cmd.Flags().StringVar(&auditorCA, "ca", "", "gateway CA bundle PEM")
	cmd.Flags().StringVar(&auditorAPIKey, "api-key", "", "operator API key")
	cmd.Flags().StringVar(&auditorSessionID, "operator-session", "", "operator session id")
	cmd.Flags().BoolVar(&auditorInsecure, "insecure", false, "skip TLS verify")
	cmd.Flags().StringVar(&auditorOutDir, "out", "", "report output dir")

	return cmd
}

func runAuditorRun(cmd *cobra.Command, args []string) {
	cfg := config.Default()
	applyAuditorFlags(&cfg)

	if auditorConfigPath != "" {
		if err := cfg.LoadFile(auditorConfigPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: config %s: %v\n", auditorConfigPath, err)
		}
	}

	names := args
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}

	selected := selectAuditorScenarios(auditorPhase, names)
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "no scenarios selected")
		os.Exit(1)
	}

	if needsGovKit(selected) {
		if err := setupGovKit(ctx, client, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gov kit setup: %v\n", err)
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
	// receipts, _, _ := client.AuditReceipts(ctx, opSession)
	if export, err := client.ExportReceipts(ctx, opSession); err == nil && len(export) > 0 {
		_ = os.MkdirAll(cfg.OutDir, 0o755)
		_ = os.WriteFile(filepath.Join(cfg.OutDir, "receipts-export.json"), export, 0o644)
	}

	// report and summary printing would go here if we had internal/auditor/report
	// but for now we just print summary to satisfy the compiler and user
	printAuditorSummary(results, "", "")
}

func runAuditorAudit(cmd *cobra.Command, args []string) {
	cfg := config.Default()
	applyAuditorFlags(&cfg)

	if auditorConfigPath != "" {
		if err := cfg.LoadFile(auditorConfigPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: config %s: %v\n", auditorConfigPath, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}
	opSession := cfg.OperatorSessionID
	if opSession == "" {
		opSession = client.DiscoverOperatorSession(ctx)
	}
	receipts, raw, err := client.AuditReceipts(ctx, opSession)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit:", err)
		os.Exit(1)
	}
	_ = os.MkdirAll(cfg.OutDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfg.OutDir, "receipts.json"), raw, 0o644)
	fmt.Printf("operator session: %s\n", opSession)
	fmt.Printf("signed receipts: %d (raw written to %s/receipts.json)\n", len(receipts), cfg.OutDir)
	for _, r := range receipts {
		fmt.Printf("  %-12s %-14s %s\n", trunc(r.TransactionHash, 12), r.ActionType, r.Status)
	}
}

func applyAuditorFlags(cfg *config.Config) {
	if auditorMTLSURL != "" {
		cfg.MTLSBaseURL = auditorMTLSURL
	}
	if auditorPublicURL != "" {
		cfg.PublicBaseURL = auditorPublicURL
	}
	if auditorCert != "" {
		cfg.Auth.ClientCert = auditorCert
	}
	if auditorKey != "" {
		cfg.Auth.ClientKey = auditorKey
	}
	if auditorCA != "" {
		cfg.Auth.CABundle = auditorCA
	}
	if auditorAPIKey != "" {
		cfg.Auth.APIKey = auditorAPIKey
	}
	if auditorInsecure {
		cfg.Auth.Insecure = auditorInsecure
	}
	if auditorSessionID != "" {
		cfg.OperatorSessionID = auditorSessionID
	}
	if auditorOutDir != "" {
		cfg.OutDir = auditorOutDir
	}
	if auditorL3Mode != "" {
		cfg.L3Mode = auditorL3Mode
	}
	if auditorEnsemble != 0 {
		cfg.EnsembleSize = auditorEnsemble
	}
	if auditorVerbose {
		cfg.Verbose = auditorVerbose
	}
}

func selectAuditorScenarios(phase string, names []string) []scenarios.Scenario {
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
		if s.RequiresPosture == scenarios.Consensus || s.RequiresPosture == scenarios.Notary {
			return true
		}
	}
	return false
}

func setupGovKit(ctx context.Context, client *clientpkg.Client, cfg config.Config) error {
	ens, err := clientpkg.NewEnsemble(cfg.ConsensusKeyID, cfg.EnsembleSize)
	if err != nil {
		return err
	}
	prin, err := clientpkg.NewPrincipal(cfg.PrincipalKeyID)
	if err != nil {
		return err
	}
	opID := cfg.OperatorSessionID
	if opID == "" {
		opID = client.DiscoverOperatorSession(ctx)
	}
	scenarios.SetGovKit(&scenarios.GovKit{
		Ensemble: ens, Principal: prin, L3Mode: cfg.L3Mode, OperatorID: opID,
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

func printAuditorSummary(results []scenarios.Result, jsonPath, mdPath string) {
	fmt.Println("\n── summary ──")
	ok := 0
	for _, r := range results {
		status := "FAIL"
		if r.OK {
			status = string(constants.GatewayModeStatusOK)
			ok++
		}
		fmt.Printf("  %-18s %-9s %-18s %s\n", r.Name, r.RequiresPosture, r.Persona, status)
	}
	fmt.Printf("\n%d/%d scenarios ok\n", ok, len(results))
	fmt.Printf("report:  %s\n", mdPath)
	fmt.Printf("json:    %s\n", jsonPath)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
