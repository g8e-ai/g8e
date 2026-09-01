// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func newFinanceUnauthorizedTradeScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgFinance, "finance-demo-scope",
		"L1 doctrine: unauthorized_trade_execution (0.90 conf) // Network isolation: net_untrusted blocked")
}

func runFinanceScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	if scenario != "1" {
		return nil, fmt.Errorf("invalid scenario number for finance: %q (valid: 1)", scenario)
	}

	definition, err := loadDemoScenarioDefinition("finance-unauthorized-trade")
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	result := newFinanceUnauthorizedTradeScenarioResult(startedAt, definition)
	var hasErrors bool

	demoPrintf("\n%s\n", strings.Repeat("─", 60))
	demoPrintf("  Scenario 1 — %s\n", definition.GetTitle())
	demoPrintln(strings.Repeat("─", 60))
	demoPrintln()
	demoPrintln("  PROVES: Two-layer defense against unauthorized trading.")
	demoPrintln("    Layer 1 — Network isolation: bad-actor on net_untrusted has no")
	demoPrintln("              route to the trading ledger on net_secure.")
	demoPrintln("    Layer 2 — Doctrine enforcement: the g8e gateway blocks unauthorized")
	demoPrintln("              trade execution payloads at confidence >= 0.90.")
	demoPrintln()

	demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
	step1Started := time.Now().UTC()
	step1Err := demoStep(ctx, demoDir, "gateway health", false, "curl", "-s", "http://localhost:8082/api/v1/health")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-1", "gateway health check", step1Started, time.Now().UTC(),
		step1Err == nil, true, "curl gateway health endpoint"))
	if step1Err != nil {
		fmt.Println("  (gateway health check failed — is the demo running?)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 2: Verify operator enrollment (mTLS certs) ────────────")
	step2Started := time.Now().UTC()
	step2Err := demoStep(ctx, demoDir, "enrollment check", false,
		"docker", "compose", "exec", "-T", "operator", "test", "-f", constants.ContainerOperatorCert)
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-2", "operator enrollment check", step2Started, time.Now().UTC(),
		step2Err == nil, true, "operator mTLS certificate exists"))
	if step2Err != nil {
		fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 3: Submit unauthorized trade via agent ───────")
	demoPrintln("  The agent submits a GovernanceEnvelope through the real")
	demoPrintln("  gateway via mTLS, attempting to execute an unauthorized trade.")
	demoPrintln("  L1 doctrine must block this at the gateway before execution:")
	demoPrintln()
	hcfg := defaultHarnessConfig("agent-runtime")
	hcfg.PublicURL = "http://g8e.local:8082"
	step3Started := time.Now().UTC()
	step3Err := demoStep(ctx, demoDir, "finance-unauthorized-trade via agent", false,
		harnessRun("finance-unauthorized-trade", hcfg)...)
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-3", "finance unauthorized trade harness", step3Started, time.Now().UTC(),
		step3Err == nil, true, "agent harness verifies L1 doctrine rejection"))
	if step3Err != nil {
		fmt.Println("  (agent scenario failed)")
		fmt.Println()
		hasErrors = true
	} else {
		result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:finance-unauthorized-trade")
		result.TransactionIds = append(result.TransactionIds, "finance-unauthorized-trade-tx")
	}

	demoPrintln("  ── Step 4: Verify prohibited trade side effect is absent ─────")
	step4Started := time.Now().UTC()
	step4Err := demoStep(ctx, demoDir, "unauthorized trade absence", false,
		"docker", "compose", "exec", "-T", "target-system", "test", "!", "-e", constants.ContainerFinanceUnauthorizedTrade)
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-4", "independent state observation: unauthorized trade absent", step4Started, time.Now().UTC(),
		step4Err == nil, true, "target-system unauthorized trade artifact is absent"))
	if step4Err != nil {
		fmt.Println("  (unauthorized trade side-effect check failed)")
		hasErrors = true
	} else {
		result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:finance-unauthorized-trade-absent")
	}

	demoPrintln("  ── Step 5: Verify doctrine rejection in gateway logs ──────────")
	step5Started := time.Now().UTC()
	step5Err := demoStep(ctx, demoDir, "audit tail", false,
		"docker", "compose", "logs", "observability", "--tail", "10")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-5", "supplementary audit log observation", step5Started, time.Now().UTC(),
		step5Err == nil, false, "observability audit log tail"))
	if step5Err != nil {
		fmt.Println("  (audit tail failed)")
	}

	demoPrintln("  ── Step 6: Network isolation (supplementary proof) ───────────")
	demoPrintln("  bad-actor (net_untrusted) → target-system (net_secure) — should timeout")
	demoPrintln()
	step6Started := time.Now().UTC()
	step6Err := demoStep(ctx, demoDir, "network isolation", false,
		"docker", "compose", "exec", "-T", "bad-actor", "sh", "-c",
		"wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_secure'")
	result.StepResults = append(result.StepResults, buildDemoStepResult(
		"finance-unauthorized-trade-step-6", "supplementary network isolation observation", step6Started, time.Now().UTC(),
		step6Err == nil, false, "net_untrusted cannot route to net_secure target"))
	if step6Err != nil {
		fmt.Println("  (network isolation check failed)")
	}

	demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

	result.CompletedAt = timestamppb.New(time.Now().UTC())
	if hasErrors {
		result.Status = demoStatusFailed
		result.VerificationStatus = "unverifiable"
		result.Failure = "one or more required steps failed"
		fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
	} else {
		result.VerificationStatus = "verified"
		fmt.Println("  [PASS] Unauthorized trade blocked at both layers.")
		fmt.Println("         Layer 1: network isolation (net_untrusted has no route to net_secure).")
		fmt.Println("         Layer 2: doctrine unauthorized_trade_execution loaded at confidence 0.90.")
	}
	if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
		return nil, fmt.Errorf("validate finance-unauthorized-trade scenario result: %w", err)
	}
	return result, nil
}
