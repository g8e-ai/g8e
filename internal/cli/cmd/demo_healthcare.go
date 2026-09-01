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

func newHealthcareSuccessScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, "healthcare-demo-scope",
		"11 PHI/HIPAA rules evaluated, FHIR PA submission recorded")
}

func newHealthcarePHIBlockedScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgHealthcare, "healthcare-demo-scope",
		"Network isolation verified // L1 doctrine rejection verified at 0.95 confidence")
}

func newHealthcareDisplayScenarioResult(definition *compliancev1.DemoScenarioDefinition, metrics string) *compliancev1.DemoScenarioResult {
	return newDemoScenarioResult(definition.GetDisplayNumber(), definition.GetTitle(), demoStatusPassed, metrics)
}

func runHealthcareScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	var result *compliancev1.DemoScenarioResult

	switch scenario {
	case "1":
		definition, err := loadDemoScenarioDefinition("healthcare-success")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcareSuccessScenarioResult(startedAt, definition)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 1 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An authorized agent on net_internal submits a PA")
		demoPrintln("          request through the g8e gateway via the native")
		demoPrintln("          run_shell_command tool driving the paop wrapper.")
		demoPrintln("          Every request passes through the doctrine engine")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		step1Started := time.Now().UTC()
		step1Err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-1", "gateway health check", step1Started, time.Now().UTC(),
			step1Err == nil, true, "curl gateway health endpoint"))
		if step1Err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Submit PA request through the gateway ───────────")
		demoPrintln("  Request path: agent-runtime → gateway (g8e.local:8443) [Governed run_shell_command]")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8081"
		step2Started := time.Now().UTC()
		step2Err := demoStep(ctx, demoDir, "fhir request", false,
			harnessRun("healthcare-success", hcfg)...)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-2", "healthcare success harness", step2Started, time.Now().UTC(),
			step2Err == nil, true, "agent harness submits PA through governed native tool"))
		if step2Err != nil {
			fmt.Println("  (healthcare-success harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else {
			result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:healthcare-success")
			result.TransactionIds = append(result.TransactionIds, "healthcare-success-tx")
		}

		demoPrintln("  ── Step 3: Independently verify the PA submission record ────")
		step3Started := time.Now().UTC()
		step3Err := demoStep(ctx, demoDir, "PA submission observation", false,
			"docker", "compose", "exec", "-T", "operator", "grep", "-F",
			`"action":"submit","request_id":"PA-2026-0045","resource_type":"ClaimResponse","detail":"preauthorization"`,
			constants.ContainerHealthcarePAOperations)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-3", "independent state observation: PA submission recorded", step3Started, time.Now().UTC(),
			step3Err == nil, true, "operator PA operation log contains the exact submission record"))
		if step3Err != nil {
			fmt.Println("  (PA submission state observation failed)")
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:healthcare-pa-2026-0045-submitted")
		}

		demoPrintln("  ── Step 4: View g8e enforcement audit ───────────────────────")
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()
		step4Started := time.Now().UTC()
		step4Err := demoStep(ctx, demoDir, "audit tail", false,
			"docker", "compose", "logs", "observability", "--tail", "10")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-success-step-4", "supplementary audit log observation", step4Started, time.Now().UTC(),
			step4Err == nil, false, "observability audit log tail"))
		if step4Err != nil {
			fmt.Println("  (warning: audit tail failed)")
		}

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 1 — PA request submitted through governed native tool.")
			fmt.Println("         Doctrine engine evaluated the payload against all 11 PHI/HIPAA rules.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-success scenario result: %w", err)
		}

	case "2":
		definition, err := loadDemoScenarioDefinition("healthcare-gold-card")
		if err != nil {
			return nil, err
		}
		var hasErrors bool
		result = newHealthcareDisplayScenarioResult(definition,
			"Governed gold-card operation recorded // reporting state is a pre-seeded fixture, not run-bound evidence")

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 2 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  DEMONSTRATES: A gold-card operation traverses the governed endpoint.")
		demoPrintln("                The reporting database contains a pre-seeded example")
		demoPrintln("                outcome; this run does not evaluate the threshold or")
		demoPrintln("                produce run-bound terminal-state evidence.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		if err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health"); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Record gold-card operation through the gateway ───")
		demoPrintln("  PA-2026-0043 (Dr. Priya Nair, 96% historic approval rate) is carried")
		demoPrintln("  through the governed endpoint via the native run_shell_command tool.")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8081"
		if err := demoStep(ctx, demoDir, "gold-card PA via agent", false,
			harnessRun("healthcare-gold-card", hcfg)...); err != nil {
			fmt.Println("  (healthcare-gold-card harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: View g8e enforcement audit ───────────────────────")
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()
		demoStepWarn(ctx, demoDir, "audit tail",
			"docker", "compose", "logs", "observability", "--tail", "10")

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Governed gold-card operation recorded.")
			fmt.Println("         Pre-seeded reporting state is display-only and is not persisted as evidence.")
		}

	case "3":
		definition, err := loadDemoScenarioDefinition("healthcare-sla-breach")
		if err != nil {
			return nil, err
		}
		var hasErrors bool
		result = newHealthcareDisplayScenarioResult(definition,
			"Governed SLA query recorded // reporting state is a pre-seeded fixture, not run-bound evidence")

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 3 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  DEMONSTRATES: An SLA query traverses the governed endpoint and the")
		demoPrintln("                reporting dashboard exposes a pre-seeded breach example.")
		demoPrintln("                This run does not calculate or transition the breach state,")
		demoPrintln("                so the fixture is not run-bound terminal evidence.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
		if err := demoStep(ctx, demoDir, "gateway health", false,
			"curl", "-s", "http://localhost:8081/api/v1/health"); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Record SLA query through the gateway ─────────────")
		demoPrintln("  PA-2026-0044 (Dr. James O'Brien, 10 days elapsed) is carried through")
		demoPrintln("  the governed endpoint via the native run_shell_command tool.")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8081"
		if err := demoStep(ctx, demoDir, "SLA breach query via agent", false,
			harnessRun("healthcare-sla-breach", hcfg)...); err != nil {
			fmt.Println("  (healthcare-sla-breach harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Compliance dashboard ──────────────────────────────")
		demoPrintln("  Open in browser:  http://localhost:3001")
		demoPrintln("  Login:            admin@g8e.local / Metabase1!")
		demoPrintln()
		demoPrintln("  Pre-loaded DCBS/OHA queries (under Questions):")
		demoPrintln("    · DCBS March 1 Filing - Denial Rates by Request Type")
		demoPrintln("    · OHA March 31 Filing - Median Decision Time")
		demoPrintln()
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Governed SLA query recorded.")
			fmt.Println("         Pre-seeded reporting state is display-only and is not persisted as evidence.")
		}

	case "4":
		definition, err := loadDemoScenarioDefinition("healthcare-phi-blocked")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newHealthcarePHIBlockedScenarioResult(startedAt, definition)
		var hasErrors bool

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintf("  Scenario 4 — %s\n", definition.GetTitle())
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: Two-layer defense.")
		demoPrintln("    Layer 1 — Network isolation: bad-actor on net_untrusted has no")
		demoPrintln("              route to net_internal or net_secure.")
		demoPrintln("    Layer 2 — Doctrine enforcement: the g8e gateway blocks PHI")
		demoPrintln("              exfiltration payloads at confidence ≥0.95 (phi_exfil_attempt).")
		demoPrintln()

		demoPrintln("  ── Layer 1: Network isolation ────────────────────────────────")
		demoPrintln("  bad-actor (net_untrusted) → gateway (net_internal) — should timeout")
		demoPrintln()
		step1Started := time.Now().UTC()
		step1Err := demoStep(ctx, demoDir, "network isolation", false,
			"docker", "compose", "exec", "-T", "bad-actor",
			"sh", "-c", "! wget -qO- -T 5 http://10.22.0.10:8080/ >/dev/null 2>&1")
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-phi-blocked-step-1", "independent state observation: untrusted network isolated", step1Started, time.Now().UTC(),
			step1Err == nil, true, "net_untrusted cannot route to the net_internal gateway"))
		if step1Err != nil {
			fmt.Println("  (network isolation check failed)")
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:healthcare-untrusted-network-isolated")
		}

		demoPrintln("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		demoPrintln("  Submit a PHI exfiltration attempt through the production-ready")
		demoPrintln("  governed native tool endpoint (mTLS + Protocol Envelopes):")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8081"
		step2Started := time.Now().UTC()
		step2Err := demoStep(ctx, demoDir, "phi exfiltration", false,
			harnessRun("healthcare-phi-blocked", hcfg)...)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"healthcare-phi-blocked-step-2", "healthcare PHI exfiltration harness", step2Started, time.Now().UTC(),
			step2Err == nil, true, "agent harness verifies L1 doctrine rejection"))
		if step2Err != nil {
			fmt.Println("  (healthcare-phi-blocked harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else {
			result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:healthcare-phi-blocked")
			result.TransactionIds = append(result.TransactionIds, "healthcare-phi-blocked-tx")
		}
		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")
		demoPrintln()

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 4 — PHI exfiltration blocked at both layers.")
			fmt.Println("         Layer 1: network isolation (net_untrusted has no route to net_internal).")
			fmt.Println("         Layer 2: doctrine phi_exfil_attempt loaded at confidence 0.95.")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate healthcare-phi-blocked scenario result: %w", err)
		}

	default:
		return nil, fmt.Errorf("invalid scenario number for healthcare: %q (valid: 1-4)", scenario)
	}

	return result, nil
}
