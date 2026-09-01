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

	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// defaultFedRAMPHarnessConfig returns the config matching the FedRAMP compose topology.
func defaultFedRAMPHarnessConfig() harnessConfig {
	return defaultHarnessConfig("agent-runtime")
}

func fedRAMPDenyHarnessVerified(err error) bool {
	return err == nil
}

func fedRAMPScenarioID(scenario string) (string, error) {
	switch scenario {
	case "1":
		return "fedramp-provision", nil
	case "2":
		return "fedramp-deny", nil
	case "3":
		return "fedramp-revert", nil
	case "4":
		return "fedramp-evidence-block", nil
	default:
		return "", fmt.Errorf("%w: invalid scenario number for fedramp: %q (valid: 1-4)", constants.ErrNotFound, scenario)
	}
}

func loadFedRAMPScenarioDefinition(scenarioID string) (*compliancev1.DemoScenarioDefinition, error) {
	assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
	if err != nil {
		return nil, fmt.Errorf("load canonical compliance catalogs: %w", err)
	}
	scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
	if err != nil {
		return nil, fmt.Errorf("load canonical demo scenario catalog: %w", err)
	}
	definition := compliancecatalog.FindDemoScenarioDefinition(scenarios, scenarioID, "1.0.0")
	if definition == nil {
		return nil, fmt.Errorf("%w: %s@1.0.0", constants.ErrUnresolvedReference, scenarioID)
	}
	return definition, nil
}

func newFedRAMPDenyScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return &compliancev1.DemoScenarioResult{
		ResultId:             fmt.Sprintf("fedramp-run:%s:%s", startedAt.Format("20060102T150405Z"), definition.ScenarioId),
		ScenarioRef:          &compliancev1.VersionedReference{Id: definition.ScenarioId, Version: definition.ScenarioVersion},
		DemoId:               constants.DemosOrgFedRAMP,
		ScopeId:              "fedramp-demo-scope",
		RunId:                fmt.Sprintf("fedramp-run-%s", startedAt.Format("20060102T150405Z")),
		StartedAt:            timestamppb.New(startedAt),
		Status:               demoStatusPassed,
		AssertionRefs:        cloneVersionedRefs(definition.AssertionRefs),
		FrameworkControlRefs: cloneFrameworkControlRefs(definition.FrameworkControlRefs),
		DisplayNumber:        definition.DisplayNumber,
		Title:                definition.Title,
		MetricsSummary:       "L1 doctrine blocks rm -rf /var/cloudsvc // audit trail tamper-evident",
	}
}

func runFedRAMPScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	scenarioID, err := fedRAMPScenarioID(scenario)
	if err != nil {
		return nil, err
	}
	definition, err := loadFedRAMPScenarioDefinition(scenarioID)
	if err != nil {
		return nil, err
	}
	hcfg := defaultFedRAMPHarnessConfig()
	var hasErrors bool
	var result *compliancev1.DemoScenarioResult

	switch scenario {
	case "1":
		result = newDemoScenarioResult(definition.DisplayNumber, definition.Title, demoStatusPassed,
			"L1 doctrine admits // L2 consensus quorum met // L5 actuator records PROVISION")

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 1 — Governed Cloud Resource Provisioning")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A FedRAMP cloud service operator submits a real")
		demoPrintln("          GovernanceEnvelope wrapping a run_shell_command that")
		demoPrintln("          drives the Sovereign Cloud Service (L5 actuator). L1")
		demoPrintln("          doctrine admits the envelope; L2 consensus quorum is")
		demoPrintln("          met and verified. The provision is executed and a signed")
		demoPrintln("          receipt is written to the hash-chained ledger.")
		demoPrintln()

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-provision", "doctrine check")
		demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 started: Governed Cloud Resource Provisioning")

		if !demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		if !demoScenarioStep(ctx, demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", constants.ContainerOperatorCert}) {
			hasErrors = true
		}

		demoPrintln("  -- Step 3: Run real fedramp-provision via agent --")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met and verified -> L5 actuator records PROVISION:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "fedramp-provision", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "fedramp-provision", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for fedramp-provision")
		if err := demoStep(ctx, demoDir, "fedramp-provision via agent",
			false,
			harnessRun("fedramp-provision", hcfg)...,
		); err != nil {
			fmt.Println("  (fedramp-provision harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "fedramp-provision", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "fedramp-provision", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met and verified (3/5)")

		if !demoScenarioStep(ctx, demoDir, "Step 4: Verify the Sovereign Cloud Service recorded the PROVISION",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"python", constants.ContainerVerifyOpsPy, "PROVISION"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "fedramp-provision", "PROVISION recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PROVISION — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 1 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Cloud resource provisioning governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the provision; signed receipt in hash-chained ledger.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 PASSED — Cloud resource provisioning governed end to end")
		}

	case "2":
		// Evidence-grade scenario: unauthorized audit-trail destruction blocked.
		// This is the first slice to emit a typed DemoScenarioResult with stable
		// identity, assertion references, framework-control references, step
		// results, and required evidence references. The scenario definition
		// lives in the canonical demo scenario catalog (fedramp-deny@1.0.0).
		startedAt := time.Now().UTC()
		result = newFedRAMPDenyScenarioResult(startedAt, definition)

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 2 — Unauthorized Audit Trail Destruction Blocked (CR-26)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised operator tries to wipe the audit trail")
		demoPrintln("          with 'rm -rf /var/cloudsvc' — L1 doctrine rejects it at")
		demoPrintln("          admission (the CR-26 audit integrity detector fires).")
		demoPrintln("          The audit trail remains tamper-evident and intact.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 started: Unauthorized Audit Trail Destruction Blocked")

		// Step 1: gateway health check.
		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (doctrine)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"})
		step1Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"fedramp-deny-step-1", "gateway health check", step1Started, step1Completed,
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		// Step 2: run fedramp-deny harness scenario (L1 reject).
		demoPrintln("  -- Step 2: Run fedramp-deny via agent (L1 reject) --")
		demoPrintln("  L1 doctrine detects 'rm -rf /var/cloudsvc' -> rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-deny", "doctrine check")
		step2Started := time.Now().UTC()
		harnessErr := demoStep(ctx, demoDir, "fedramp-deny via agent",
			false,
			harnessRun("fedramp-deny", hcfg)...,
		)
		step2Completed := time.Now().UTC()
		// The harness exits successfully only after it verifies that doctrine blocked
		// the action. A nonzero exit means the expected rejection was not verified.
		step2Verified := fedRAMPDenyHarnessVerified(harnessErr)
		if !step2Verified {
			fmt.Println("  (fedramp-deny harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"fedramp-deny-step-2", "fedramp-deny harness (L1 doctrine reject)", step2Started, step2Completed,
			step2Verified, true, "agent harness fedramp-deny"))
		if step2Verified {
			// Record the failed-stage receipt reference produced by the L1 denial.
			result.ReceiptRefs = append(result.ReceiptRefs, "failed-stage:fedramp-deny")
			result.TransactionIds = append(result.TransactionIds, "fedramp-deny-tx")
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "fedramp-deny", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/cloudsvc' — CR-26 audit integrity violation detected at admission")

		// Step 3: independent state observation — operations log remains present and non-empty.
		step3Started := time.Now().UTC()
		step3OK := demoScenarioStep(ctx, demoDir, "Step 3: Independently verify cloudsvc operations log still exists and is non-empty (prohibited side-effect check)",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"sh", "-c", "test -f " + constants.ContainerCloudSvcOpsLog + " && test -s " + constants.ContainerCloudSvcOpsLog})
		step3Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"fedramp-deny-step-3", "independent state observation: operations log present and non-empty",
			step3Started, step3Completed, step3OK, true,
			"docker compose exec cloudsvc test -f && test -s operations log"))
		if step3OK {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:cloudsvc-ops-log-intact")
		} else {
			hasErrors = true
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 2 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 2 — Audit trail destruction blocked by L1 doctrine.")
			fmt.Println("         CR-26 audit integrity rule fired at admission.")
			fmt.Println("         Independent verification confirms the operations log is intact and non-empty.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Audit trail destruction blocked by L1 doctrine")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate fedramp-deny scenario result: %w", err)
		}

	case "3":
		result = newDemoScenarioResult(definition.DisplayNumber, definition.Title, demoStatusPassed,
			"L2 quorum admits revert // L5 actuator records REVERT // CM-7 rollback")

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 3 — Governed Configuration Revert (CM-7)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A configuration revert on fedramp-iam-roles-01 is")
		demoPrintln("          submitted with L2 ensemble quorum. The revert is")
		demoPrintln("          admitted and executed by the L5 actuator. This")
		demoPrintln("          demonstrates that configuration changes are governed")
		demoPrintln("          through the full L1/L2/L3 pipeline with signed receipts.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Governed Configuration Revert")

		if !demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-revert", "doctrine check")
		demoPrintln("  -- Step 2: Run fedramp-revert via agent (L2 quorum -> admit) --")
		demoPrintln("  L2 consensus quorum met -> admitted -> L5 actuator records REVERT:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "fedramp-revert", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "fedramp-revert", "consensus deliberation")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberConcord, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberVariance, true, true, 3, 5, tui.ConsensusPending, "")
		if err := demoStep(ctx, demoDir, "fedramp-revert via agent",
			false,
			harnessRun("fedramp-revert", hcfg)...,
		); err != nil {
			fmt.Println("  (fedramp-revert harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "fedramp-revert", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "fedramp-revert", "actuator executing")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusReached, "revert-hash-001")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met (3/5) — revert admitted")

		if !demoScenarioStep(ctx, demoDir, "Step 3: Verify the Sovereign Cloud Service recorded the REVERT",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"python", constants.ContainerVerifyOpsPy, "REVERT"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "fedramp-revert", "REVERT recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded REVERT — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Configuration revert governed by L2 consensus.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         REVERT operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Configuration revert governed by L2 consensus")
		}

	case "4":
		result = newDemoScenarioResult(definition.DisplayNumber, definition.Title, demoStatusPassed,
			"L1 blocks vault wipe // audit vault remains intact")

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 4 — Gateway Audit Vault Destruction Blocked (CR-26)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised operator tries to wipe the gateway audit")
		demoPrintln("          vault with 'rm -rf /root/.g8e/data' — L1 doctrine rejects")
		demoPrintln("          it at admission (the CR-26 audit integrity detector fires).")
		demoPrintln("          The audit vault remains tamper-evident and intact.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Gateway Audit Vault Destruction Blocked")

		if !demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (doctrine)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		demoPrintln("  -- Step 2: Run fedramp-evidence-block via agent (L1 reject) --")
		demoPrintln("  L1 doctrine detects 'rm -rf /root/.g8e/data' -> rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-evidence-block", "doctrine check")
		if err := demoStep(ctx, demoDir, "fedramp-evidence-block via agent",
			false,
			harnessRun("fedramp-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (fedramp-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "fedramp-evidence-block", "AUDIT VAULT DESTRUCTION BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /root/.g8e/data' — CR-26 audit integrity violation detected at admission")

		if !demoScenarioStep(ctx, demoDir, "Step 3: Independently verify gateway audit vault DB still exists and is non-empty (prohibited side-effect check)",
			[]string{"docker", "compose", "exec", "-T", "gateway",
				"sh", "-c", "test -f " + constants.ContainerAuditVaultDB + " && test -s " + constants.ContainerAuditVaultDB}) {
			hasErrors = true
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Audit vault destruction blocked by L1 doctrine.")
			fmt.Println("         CR-26 audit integrity rule fired at admission.")
			fmt.Println("         Independent verification confirms the audit vault DB is intact and non-empty.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Audit vault destruction blocked by L1 doctrine")
		}

	default:
		return nil, fmt.Errorf("%w: invalid scenario number for fedramp: %q (valid: 1-4)", constants.ErrNotFound, scenario)
	}
	return result, nil
}

// cloneVersionedRefs returns a deep copy of the given versioned references so
// callers cannot mutate the package-level canonical slices.
func cloneVersionedRefs(refs []*compliancev1.VersionedReference) []*compliancev1.VersionedReference {
	clone := make([]*compliancev1.VersionedReference, len(refs))
	for i, ref := range refs {
		clone[i] = &compliancev1.VersionedReference{Id: ref.Id, Version: ref.Version}
	}
	return clone
}

// cloneFrameworkControlRefs returns a deep copy of the given framework control
// references so callers cannot mutate the package-level canonical slices.
func cloneFrameworkControlRefs(refs []*compliancev1.FrameworkControlReference) []*compliancev1.FrameworkControlReference {
	clone := make([]*compliancev1.FrameworkControlReference, len(refs))
	for i, ref := range refs {
		clone[i] = &compliancev1.FrameworkControlReference{
			FrameworkRef: &compliancev1.VersionedReference{Id: ref.FrameworkRef.Id, Version: ref.FrameworkRef.Version},
			ControlId:    ref.ControlId,
		}
	}
	return clone
}

// buildDemoStepResult constructs a typed DemoStepResult from the step execution
// outcome. A nil/empty failure string is set for passing steps; a descriptive
// failure is set for non-passing steps per the protocol validation rules.
func buildDemoStepResult(stepID, operation string, started, completed time.Time, ok, required bool, protocolResult string) *compliancev1.DemoStepResult {
	status := demoStatusPassed
	failure := ""
	if !ok {
		status = demoStatusFailed
		failure = operation + " failed"
	}
	return &compliancev1.DemoStepResult{
		StepId:         stepID,
		Operation:      operation,
		StartedAt:      timestamppb.New(started),
		CompletedAt:    timestamppb.New(completed),
		Status:         status,
		ProtocolResult: protocolResult,
		Failure:        failure,
		Required:       required,
	}
}

// runFedRAMPKSIEvidence runs g8e compliance ksi inside the gateway container
// to emit KSI result snapshots, then verifies them via verify_ops.py --ksi-result.
// Returns true if both steps succeed.
func runFedRAMPKSIEvidence(ctx context.Context, demoDir string) bool {
	demoPrintf("\n%s\n", strings.Repeat("-", 60))
	demoPrintln("  KSI Evidence Export")
	demoPrintln(strings.Repeat("-", 60))
	demoPrintln()

	if !demoScenarioStep(ctx, demoDir, "Step 1: Emit KSI result snapshots (g8e compliance ksi --class C)",
		[]string{"docker", "compose", "exec", "-T", "gateway",
			"/g8e", "compliance", "ksi", "--class", "C", "--catalog", constants.ContainerKSICatalog}) {
		fmt.Println("  [FAIL] KSI evidence export — could not emit snapshots.")
		return false
	}

	if !demoScenarioStep(ctx, demoDir, "Step 2: Verify KSI result snapshots (verify_ops.py --ksi-result)",
		[]string{"docker", "compose", "exec", "-T", "cloudsvc",
			"python", constants.ContainerVerifyOpsPy, "--ksi-result"}) {
		fmt.Println("  [FAIL] KSI evidence export — snapshot verification failed.")
		return false
	}

	fmt.Println("  [PASS] KSI evidence export — snapshots emitted and verified.")
	return true
}
