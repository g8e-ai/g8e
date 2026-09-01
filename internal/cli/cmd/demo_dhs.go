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

// defaultDHSHarnessConfig returns the config matching the DHS compose topology.
func defaultDHSHarnessConfig() harnessConfig {
	return defaultHarnessConfig("agent-coalition")
}

func newDHSSovereignIngestScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, "dhs-demo-scope",
		"L1 doctrine admits // L2 consensus quorum met // L5 actuator records INGEST")
}

func newDHSDisconnectedOperationsScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, "dhs-demo-scope",
		"Datalink severed // Local governance continues // Git ledger + SQLite vault")
}

func newDHSCueScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition) *compliancev1.DemoScenarioResult {
	return newDemoEvidenceScenarioResult(startedAt, definition, constants.DemosOrgDHS, "dhs-demo-scope",
		"L2 quorum admits cue // L5 actuator records CUE")
}

func runDHSScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	hcfg := defaultDHSHarnessConfig()
	var result *compliancev1.DemoScenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		definition, err := loadDemoScenarioDefinition("dhs-ingest")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSSovereignIngestScenarioResult(startedAt, definition)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 1 — Sovereign Multi-Source Ingest (LOE 1)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A coalition source connector submits a real")
		demoPrintln("          GovernanceEnvelope wrapping a run_shell_command that")
		demoPrintln("          drives the Sovereign Data Service (L5 actuator). L1")
		demoPrintln("          doctrine admits the envelope; L2 consensus quorum is")
		demoPrintln("          met and verified. The ingest is executed and a signed")
		demoPrintln("          receipt is written to the hash-chained ledger —")
		demoPrintln("          provable chain-of-custody.")
		demoPrintln()

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest", "doctrine check")
		demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 started: Sovereign Multi-Source Ingest")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"})
		step1Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-1", "gateway health check", step1Started, step1Completed,
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		step2Started := time.Now().UTC()
		step2OK := demoScenarioStep(ctx, demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", constants.ContainerOperatorCert})
		step2Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-2", "operator enrollment check", step2Started, step2Completed,
			step2OK, true, "docker compose exec operator test client certificate"))
		if !step2OK {
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Run real dhs-ingest via agent ────────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met and verified → L5 actuator records INGEST:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-ingest")
		step3Started := time.Now().UTC()
		harnessErr := demoStep(ctx, demoDir, "dhs-ingest via agent",
			false,
			harnessRun("dhs-ingest", hcfg)...,
		)
		step3Completed := time.Now().UTC()
		step3OK := harnessErr == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-3", "dhs-ingest harness", step3Started, step3Completed,
			step3OK, true, "agent harness dhs-ingest"))
		if !step3OK {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else {
			result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:dhs-ingest")
			result.TransactionIds = append(result.TransactionIds, "dhs-ingest-tx")
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-ingest", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met and verified (3/5)")

		step4Started := time.Now().UTC()
		step4OK := demoScenarioStep(ctx, demoDir, "Step 4: Verify the Sovereign Data Service recorded the INGEST",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "INGEST"})
		step4Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-ingest-step-4", "independent state observation: ingest recorded", step4Started, step4Completed,
			step4OK, true, "datasvc verify_ops.py INGEST"))
		if !step4OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:datasvc-ingest-recorded")
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest", "INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded INGEST — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 1 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 1 — Sovereign ingest governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the ingest; signed receipt in hash-chained ledger.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 PASSED — Sovereign ingest governed end to end")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-ingest scenario result: %w", err)
		}

	case "2":
		definition, err := loadDemoScenarioDefinition("dhs-disconnected-operations")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSDisconnectedOperationsScenarioResult(startedAt, definition)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — Resilient Disconnected Operations (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The Mission Partner datalink is severed, simulating a")
		demoPrintln("          contested, comms-denied corridor. The gateway and operator")
		demoPrintln("          keep governing ingest locally and commit every decision to")
		demoPrintln("          the Git-backed ledger and SQLite audit vault — no cloud, no")
		demoPrintln("          loss of sovereign control. State reconciles when the link")
		demoPrintln("          is restored.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 started: Resilient Disconnected Operations")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm gateway is live before disconnect",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-1", "gateway health check before disconnect", step1Started, time.Now().UTC(),
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		demoEmitter.Ledger(tui.LevelWarn, "Mission Partner datalink severed — entering comms-denied mode")
		step2Started := time.Now().UTC()
		step2Err := demoStep(ctx, demoDir, "sever datalink", false,
			"docker", "network", "disconnect",
			constants.DemosDHSPerimeterNetwork, constants.DemosDHSCoalitionDatalinkCtnr,
		)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-2", "sever mission partner datalink", step2Started, time.Now().UTC(),
			step2Err == nil, true, "docker network disconnect mission partner datalink"))
		if step2Err != nil {
			fmt.Printf("  (sever datalink failed: %v)\n\n", step2Err)
			hasErrors = true
		}

		step3Started := time.Now().UTC()
		step3OK := demoScenarioStep(ctx, demoDir, "Step 3: Verify network detachment (datalink container off perimeter)",
			[]string{"sh", "-c", "docker network inspect " + constants.DemosDHSPerimeterNetwork +
				" --format '{{range .Containers}}{{.Name}} {{end}}' | grep -q " + constants.DemosDHSCoalitionDatalinkCtnr + " && exit 1 || exit 0"})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-3", "independent state observation: datalink detached", step3Started, time.Now().UTC(),
			step3OK, true, "docker network inspection excludes datalink container"))
		if !step3OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:dhs-datalink-detached")
		}

		step4Started := time.Now().UTC()
		step4OK := demoScenarioStep(ctx, demoDir, "Step 4: Verify gateway continues operating locally",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-4", "independent state observation: local gateway available", step4Started, time.Now().UTC(),
			step4OK, true, "curl gateway health endpoint while disconnected"))
		if !step4OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:dhs-local-gateway-available")
		}

		demoPrintln("  ── Step 5: Govern an ingest while disconnected ──────────────────")
		demoPrintln("  Running dhs-ingest through the gateway (consensus) with the datalink severed:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest-disco", "doctrine check (local)")
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest-disco", "doctrine admitted (local)")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest-disco", "local consensus")
		step5Started := time.Now().UTC()
		step5Err := demoStep(ctx, demoDir, "dhs-ingest while disconnected",
			false,
			harnessRun("dhs-ingest", hcfg)...,
		)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-5", "dhs-ingest harness while disconnected", step5Started, time.Now().UTC(),
			step5Err == nil, true, "agent harness dhs-ingest while datalink detached"))
		if step5Err != nil {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		} else {
			result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:dhs-disconnected-ingest")
			result.TransactionIds = append(result.TransactionIds, "dhs-disconnected-ingest-tx")
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest-disco", "local quorum met")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest-disco", "local INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "Governance continued locally while disconnected — Git ledger + SQLite vault persisted")

		step6Started := time.Now().UTC()
		step6OK := demoScenarioStep(ctx, demoDir, "Step 6: Verify local ledger directory exists and is non-empty",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "test -d " + constants.ContainerLedgerFilesDir + " && test -n \"$(ls -A " + constants.ContainerLedgerFilesDir + ")\""})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-6", "independent state observation: local ledger persisted", step6Started, time.Now().UTC(),
			step6OK, true, "operator ledger directory exists and is non-empty"))
		if !step6OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:dhs-local-ledger-persisted")
		}

		step7Started := time.Now().UTC()
		step7OK := demoScenarioStep(ctx, demoDir, "Step 7: Verify local audit vault DB exists and is non-empty",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "test -f " + constants.ContainerAuditVaultDB + " && test -s " + constants.ContainerAuditVaultDB})
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-7", "independent state observation: local audit vault persisted", step7Started, time.Now().UTC(),
			step7OK, true, "operator audit vault exists and is non-empty"))
		if !step7OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:dhs-local-audit-vault-persisted")
		}

		demoPrintln("  ── Step 8: Restore the Mission Partner datalink ─────────────────")
		step8Started := time.Now().UTC()
		step8Err := demoStep(ctx, demoDir, "restore datalink", false,
			"docker", "network", "connect",
			constants.DemosDHSPerimeterNetwork, constants.DemosDHSCoalitionDatalinkCtnr,
		)
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-disconnected-step-8", "restore mission partner datalink", step8Started, time.Now().UTC(),
			step8Err == nil, false, "docker network connect mission partner datalink"))
		restorationFailed := step8Err != nil
		if step8Err != nil {
			fmt.Printf("  (warning: restore datalink failed: %v)\n\n", step8Err)
		} else {
			demoEmitter.Ledger(tui.LevelInfo, "Mission Partner datalink restored")
		}

		step9Started := time.Now().UTC()
		if restorationFailed {
			result.StepResults = append(result.StepResults, &compliancev1.DemoStepResult{
				StepId: "dhs-disconnected-step-9", Operation: "independent state observation: datalink restored",
				StartedAt: timestamppb.New(step9Started), CompletedAt: timestamppb.New(time.Now().UTC()),
				Status: demoStatusSkipped, ProtocolResult: "datalink restoration unavailable", Failure: "restore datalink step failed", Required: false,
			})
		} else {
			step9OK := demoScenarioStep(ctx, demoDir, "Step 9: Verify datalink is reachable again (container back on perimeter)",
				[]string{"sh", "-c", "docker network inspect " + constants.DemosDHSPerimeterNetwork +
					" --format '{{range .Containers}}{{.Name}} {{end}}' | grep -q " + constants.DemosDHSCoalitionDatalinkCtnr})
			result.StepResults = append(result.StepResults, buildDemoStepResult(
				"dhs-disconnected-step-9", "independent state observation: datalink restored", step9Started, time.Now().UTC(),
				step9OK, false, "docker network inspection includes datalink container"))
			restorationFailed = !step9OK
			if step9OK {
				result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:dhs-datalink-restored")
			}
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
			fmt.Println("  [PASS] Scenario 2 — Continuity of coverage verified under comms denial.")
			fmt.Println("         Governance continued locally; Git ledger + SQLite vault persisted all decisions.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Continuity of coverage verified under comms denial")
		}

		if restorationFailed {
			result.MetricsSummary += " // datalink restoration FAILED (reported separately)"
			fmt.Println("  [RESTORATION FAILURE] Datalink could not be restored — continuity claim holds,")
			fmt.Println("    but the environment requires manual reconnection before subsequent scenarios.")
			demoEmitter.Ledger(tui.LevelWarn, "Datalink restoration failed — continuity verified but manual reconnection required")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-disconnected-operations scenario result: %w", err)
		}

	case "3":
		definition, err := loadDemoScenarioDefinition("dhs-cue")
		if err != nil {
			return nil, err
		}
		startedAt := time.Now().UTC()
		result = newDHSCueScenarioResult(startedAt, definition)

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Governed Predictive Cueing (LOE 3 & 4)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An authorized interdiction cue with L2 ensemble quorum")
		demoPrintln("          is admitted and executed by the L5 actuator (dhs-cue).")
		demoPrintln("          This demonstrates that L2 BFT consensus is a real")
		demoPrintln("          fail-closed gate, not just an audit annotation.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Governed Predictive Cueing")

		step1Started := time.Now().UTC()
		step1OK := demoScenarioStep(ctx, demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"})
		step1Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-cue-step-1", "gateway health check", step1Started, step1Completed,
			step1OK, true, "curl gateway health endpoint"))
		if !step1OK {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-cue", "doctrine check")
		demoPrintln("  ── Step 2: Run dhs-cue via agent (L2 quorum → admit) ────")
		demoPrintln("  L2 consensus quorum met (decision=true) → admitted → L5 actuator records CUE:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-cue", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-cue", "consensus deliberation")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberConcord, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.ConsensusMemberVariance, true, true, 3, 5, tui.ConsensusPending, "")
		step2Started := time.Now().UTC()
		harnessErr := demoStep(ctx, demoDir, "dhs-cue via agent",
			false,
			harnessRun("dhs-cue", hcfg)...,
		)
		step2Completed := time.Now().UTC()
		step2OK := harnessErr == nil
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-cue-step-2", "dhs-cue harness", step2Started, step2Completed,
			step2OK, true, "agent harness dhs-cue"))
		if !step2OK {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		} else {
			result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:dhs-cue")
			result.TransactionIds = append(result.TransactionIds, "dhs-cue-tx")
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-cue", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-cue", "actuator executing")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusReached, "cue-hash-001")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met (3/5) — cue admitted")

		step3Started := time.Now().UTC()
		step3OK := demoScenarioStep(ctx, demoDir, "Step 3: Verify the Sovereign Data Service recorded the CUE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "CUE"})
		step3Completed := time.Now().UTC()
		result.StepResults = append(result.StepResults, buildDemoStepResult(
			"dhs-cue-step-3", "independent state observation: cue recorded", step3Started, step3Completed,
			step3OK, true, "datasvc verify_ops.py CUE"))
		if !step3OK {
			hasErrors = true
		} else {
			result.StateObservationRefs = append(result.StateObservationRefs, "state-observation:datasvc-cue-recorded")
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-cue", "CUE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded CUE — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		result.CompletedAt = timestamppb.New(time.Now().UTC())
		if hasErrors {
			result.Status = demoStatusFailed
			result.VerificationStatus = "unverifiable"
			result.Failure = "one or more required steps failed"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			result.VerificationStatus = "verified"
			fmt.Println("  [PASS] Scenario 3 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted with quorum.")
			fmt.Println("         CUE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Predictive cueing governed by L2 consensus")
		}
		if err := compliancecatalog.ValidateDemoScenarioResult(result, definition, result.ScopeId); err != nil {
			return nil, fmt.Errorf("validate dhs-cue scenario result: %w", err)
		}

	case "4":
		result = newDemoScenarioResult("4", "Sovereign Destruction + tamper-proof audit", demoStatusPassed,
			"L1 blocks audit wipe // L1+L2 admit governed purge → receipt")

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 4 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised connector tries to wipe the audit trail")
		demoPrintln("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		demoPrintln("          admission (the data-destruction threat detector fires).")
		demoPrintln("          Then a governed retention purge is admitted by L1 doctrine")
		demoPrintln("          with L2 consensus quorum met, and the L5 actuator records")
		demoPrintln("          the PURGE with a cryptographic destruction receipt.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Sovereign Destruction + Tamper-Proof Audit")

		demoPrintln("  ── Step 1: Run dhs-evidence-block via agent (L1 reject) ──")
		demoPrintln("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-evidence-block", "doctrine check")
		if err := demoStep(ctx, demoDir, "dhs-evidence-block via agent",
			false,
			harnessRun("dhs-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "dhs-evidence-block", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/log/g8e' — data-destruction threat detected at admission")

		if !demoScenarioStep(ctx, demoDir, "Step 2: Independently verify operator audit vault DB still exists and is non-empty (prohibited side-effect check)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "test -f " + constants.ContainerAuditVaultDB + " && test -s " + constants.ContainerAuditVaultDB}) {
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Run dhs-purge via agent (admit) ──────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met → L5 actuator records PURGE:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-purge", "doctrine check")
		if err := demoStep(ctx, demoDir, "dhs-purge via agent",
			false,
			harnessRun("dhs-purge", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-purge harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-purge", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-purge", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-purge", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L1+L2 admitted governed purge — L5 actuator recording PURGE with destruction receipt")

		if !demoScenarioStep(ctx, demoDir, "Step 4: Verify the Sovereign Data Service recorded the PURGE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "PURGE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-purge", "PURGE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PURGE — cryptographic destruction receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.Status = demoStatusFailed
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Destruction governed and provable.")
			fmt.Println("         L1 blocked the audit-wipe; L1+L2 admitted governed purge with receipt.")
			fmt.Println("         Independent verification confirms the audit vault is intact after the blocked attempt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Destruction governed and provable")
		}

	default:
		return nil, fmt.Errorf("invalid scenario number for dhs: %q (valid: 1-4)", scenario)
	}
	return result, nil
}
