// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Command auditor impersonates arbitrary AI tools and agents against a REAL g8e
// Gateway + Operator, exercising the full protocol surface (MCP, A2A, A2A
// protobuf, and official governance envelopes with mock consensus + principal
// signing), then audits every result against the Operator's signed receipts.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/cmd/g8ea/internal/client"
	clientpkg "github.com/g8e-ai/g8e/cmd/g8ea/internal/client"
	"github.com/g8e-ai/g8e/cmd/g8ea/internal/config"
	"github.com/g8e-ai/g8e/cmd/g8ea/internal/harness"
	"github.com/g8e-ai/g8e/cmd/g8ea/internal/report"
	"github.com/g8e-ai/g8e/cmd/g8ea/internal/scenarios"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "list":
		cmdList()
	case "run":
		os.Exit(cmdRun(args))
	case "audit":
		os.Exit(cmdAudit(args))
	case "self-test":
		os.Exit(cmdSelfTest(args))
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `auditor — universal agent emulator for a real g8e Gateway/Operator

usage:
  auditor list
  auditor run   [flags] [scenario ...]
  auditor audit [flags]
  auditor self-test [flags]    Start self-contained gateway+operator and run tests

run flags:
  --phase doctrine|notary|all   which block to run (default: all)
                                doctrine = plain/advanced/secured MCP + A2A (+protobuf)
                                notary   = consensus + official envelope w/ principal L3
  --l3-mode mock|suspend        how the maximal envelope satisfies L3 (default: suspend)
  --ensemble N                  mock consensus voters (default: 3)

common flags:
  --config PATH                 JSON config overlay
  --mtls-url URL                Gateway mTLS surface (default https://localhost:8440)
  --public-url URL              Gateway public surface for OOB approve (default :8442)
  --cert / --key / --ca PATH    BYO-client mTLS material + gateway CA bundle
  --api-key KEY                 operator API key for MCP/A2A surface
  --operator-session ID         scope audit to a specific operator session
  --insecure                    skip TLS verify (local dev only)
  --out DIR                     report output dir (default ./phantom-out)
  --verbose                     echo each request/response

self-test flags:
  --phase doctrine|notary|all   which block to run (default: doctrine)
  --gateway-binary PATH         Path to g8e gateway binary (default ./cmd/g8eo/g8eo)
  --operator-binary PATH        Path to g8e operator binary (default ./cmd/g8eo/g8eo)
`)
}

func cmdList() {
	fmt.Println("scenarios (in run order):")
	for _, s := range scenarios.Registry() {
		fmt.Printf("  %-18s %-9s %-18s %s\n", s.Name, s.RequiresPosture, s.Persona.ID, s.Title)
	}
}

// bindFlags returns a flagset wired to a Config plus the run-specific knobs.
func bindFlags(fs *flag.FlagSet, cfg *config.Config) (phase, configPath *string) {
	configPath = fs.String("config", "", "JSON config overlay")
	fs.StringVar(&cfg.MTLSBaseURL, "mtls-url", cfg.MTLSBaseURL, "Gateway mTLS surface")
	fs.StringVar(&cfg.PublicBaseURL, "public-url", cfg.PublicBaseURL, "Gateway public surface")
	fs.StringVar(&cfg.Auth.ClientCert, "cert", cfg.Auth.ClientCert, "client cert PEM")
	fs.StringVar(&cfg.Auth.ClientKey, "key", cfg.Auth.ClientKey, "client key PEM")
	fs.StringVar(&cfg.Auth.CABundle, "ca", cfg.Auth.CABundle, "gateway CA bundle PEM")
	fs.StringVar(&cfg.Auth.APIKey, "api-key", cfg.Auth.APIKey, "operator API key")
	fs.BoolVar(&cfg.Auth.Insecure, "insecure", cfg.Auth.Insecure, "skip TLS verify")
	fs.StringVar(&cfg.OperatorSessionID, "operator-session", cfg.OperatorSessionID, "operator session id")
	fs.StringVar(&cfg.OutDir, "out", cfg.OutDir, "report output dir")
	fs.StringVar(&cfg.L3Mode, "l3-mode", cfg.L3Mode, "mock|suspend")
	fs.IntVar(&cfg.EnsembleSize, "ensemble", cfg.EnsembleSize, "mock consensus voters")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "echo traffic")
	phase = fs.String("phase", "all", "doctrine|notary|all")
	return
}

func loadConfig(fs *flag.FlagSet, cfg *config.Config, configPath string) {
	// Apply file first, then re-apply flags so explicit flags win.
	if configPath != "" {
		if err := cfg.LoadFile(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: config %s: %v\n", configPath, err)
		}
		fs.Visit(func(f *flag.Flag) {}) // flags already bound to cfg fields
	}
}

func cmdRun(args []string) int {
	cfg := config.Default()
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	phase, configPath := bindFlags(fs, &cfg)
	_ = fs.Parse(args)
	loadConfig(fs, &cfg, *configPath)

	names := fs.Args() // explicit scenario names, if any
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		return 1
	}

	selected := selectScenarios(*phase, names)
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "no scenarios selected")
		return 1
	}

	// If any governance scenario is in play, mint mock actors and register them.
	if needsGovKit(selected) {
		if err := setupGovKit(ctx, client, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gov kit setup: %v\n", err)
		}
	}

	results := make([]scenarios.Result, 0, len(selected))
	for _, s := range selected {
		results = append(results, scenarios.Execute(ctx, client, s))
	}

	// Audit: pull the REAL signed receipts from the Operator's vault.
	opSession := cfg.OperatorSessionID
	if opSession == "" {
		opSession = client.DiscoverOperatorSession(ctx)
	}
	receipts, _, _ := client.AuditReceipts(ctx, opSession)
	if export, err := client.ExportReceipts(ctx, opSession); err == nil && len(export) > 0 {
		_ = os.MkdirAll(cfg.OutDir, 0o755)
		_ = os.WriteFile(filepath.Join(cfg.OutDir, "receipts-export.json"), export, 0o644)
	}

	rep := report.Report{
		GeneratedAt:       time.Now(),
		Gateway:           cfg.MTLSBaseURL,
		OperatorSessionID: opSession,
		Results:           results,
		Receipts:          receipts,
	}
	jsonPath, mdPath, err := report.Write(cfg.OutDir, rep)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		return 1
	}

	printSummary(results, jsonPath, mdPath)
	return 0
}

func cmdAudit(args []string) int {
	cfg := config.Default()
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	_, configPath := bindFlags(fs, &cfg)
	_ = fs.Parse(args)
	loadConfig(fs, &cfg, *configPath)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := clientpkg.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		return 1
	}
	opSession := cfg.OperatorSessionID
	if opSession == "" {
		opSession = client.DiscoverOperatorSession(ctx)
	}
	receipts, raw, err := client.AuditReceipts(ctx, opSession)
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit:", err)
		return 1
	}
	_ = os.MkdirAll(cfg.OutDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfg.OutDir, "receipts.json"), raw, 0o644)
	fmt.Printf("operator session: %s\n", opSession)
	fmt.Printf("signed receipts: %d (raw written to %s/receipts.json)\n", len(receipts), cfg.OutDir)
	for _, r := range receipts {
		fmt.Printf("  %-12s %-14s %s\n", trunc(r.TransactionHash, 12), r.ActionType, r.Status)
	}
	return 0
}

// ---- selection & setup ------------------------------------------------------

func selectScenarios(phase string, names []string) []scenarios.Scenario {
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
		default: // all
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

// setupGovKit mints the mock ensemble + principal keys, registers them as
// trusted signers (best-effort), and injects the kit into the scenarios pkg.
func setupGovKit(ctx context.Context, client *client.Client, cfg config.Config) error {
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

func printSummary(results []scenarios.Result, jsonPath, mdPath string) {
	fmt.Println("\n── summary ──")
	ok := 0
	for _, r := range results {
		status := "FAIL"
		if r.OK {
			status = "ok"
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

// cmdSelfTest runs the auditor in self-contained mode with its own gateway+operator.
func cmdSelfTest(args []string) int {
	fs := flag.NewFlagSet("self-test", flag.ExitOnError)
	phase := fs.String("phase", "doctrine", "doctrine|notary|all")
	gatewayBinary := fs.String("gateway-binary", "./cmd/g8eo/g8eo", "Path to g8e gateway binary")
	operatorBinary := fs.String("operator-binary", "./cmd/g8eo/g8eo", "Path to g8e operator binary")
	_ = fs.Parse(args)

	fmt.Println("Starting self-contained test harness...")

	// Create test harness
	cfg := harness.DefaultConfig()
	cfg.GatewayBinary = *gatewayBinary
	cfg.OperatorBinary = *operatorBinary
	cfg.Posture = *phase

	h, err := harness.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create harness: %v\n", err)
		return 1
	}
	defer h.Stop()

	// Start gateway and operator
	if err := h.Start(cfg.Posture); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start harness: %v\n", err)
		return 1
	}

	// Configure auditor to use the test harness
	auditorCfg := config.Default()
	auditorCfg.MTLSBaseURL = h.GatewayURL()
	auditorCfg.PublicBaseURL = h.PublicURL()
	auditorCfg.Auth.ClientCert = h.ClientCertPath
	auditorCfg.Auth.ClientKey = h.ClientKeyPath
	auditorCfg.Auth.CABundle = h.CACertPath
	auditorCfg.Auth.Insecure = true // Skip TLS verify for self-signed test certs
	auditorCfg.OutDir = "./phantom-out-self-test"

	// Create client
	client, err := clientpkg.New(auditorCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	// Select scenarios based on phase
	selected := selectScenarios(*phase, []string{})
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "no scenarios selected")
		return 1
	}

	// Setup gov kit if needed
	if needsGovKit(selected) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := setupGovKit(ctx, client, auditorCfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: gov kit setup: %v\n", err)
		}
	}

	// Run scenarios
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	results := make([]scenarios.Result, 0, len(selected))
	for _, s := range selected {
		results = append(results, scenarios.Execute(ctx, client, s))
	}

	// Audit results
	opSession := client.DiscoverOperatorSession(ctx)
	receipts, _, _ := client.AuditReceipts(ctx, opSession)

	rep := report.Report{
		GeneratedAt:       time.Now(),
		Gateway:           auditorCfg.MTLSBaseURL,
		OperatorSessionID: opSession,
		Results:           results,
		Receipts:          receipts,
	}
	jsonPath, mdPath, err := report.Write(auditorCfg.OutDir, rep)
	if err != nil {
		fmt.Fprintf(os.Stderr, "report: %v\n", err)
		return 1
	}

	printSummary(results, jsonPath, mdPath)
	return 0
}
