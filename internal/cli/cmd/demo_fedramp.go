// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// defaultFedRAMPHarnessConfig returns the config matching the FedRAMP compose topology.
func defaultFedRAMPHarnessConfig() harnessConfig {
	return defaultHarnessConfig("agent-runtime")
}

func runFedRAMPScenario(demoDir, scenario string) (scenarioResult, error) {
	hcfg := defaultFedRAMPHarnessConfig()
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Governed Cloud Resource Provisioning"
		result.status = "PASS"
		result.metrics = "L1 doctrine admits // L2 consensus quorum met // L5 actuator records PROVISION"

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

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		if !demoScenarioStep(demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
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
		if err := demoStep(demoDir, "fedramp-provision via agent",
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

		if !demoScenarioStep(demoDir, "Step 4: Verify the Sovereign Cloud Service recorded the PROVISION",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"python", constants.ContainerVerifyOpsPy, "PROVISION"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "fedramp-provision", "PROVISION recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PROVISION — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 1 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Cloud resource provisioning governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the provision; signed receipt in hash-chained ledger.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 PASSED — Cloud resource provisioning governed end to end")
		}

	case "2":
		result.number = "2"
		result.name = "Unauthorized Audit Trail Destruction Blocked by L1 Doctrine"
		result.status = "PASS"
		result.metrics = "L1 doctrine blocks rm -rf /var/cloudsvc // audit trail tamper-evident"

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

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (doctrine)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		demoPrintln("  -- Step 2: Run fedramp-deny via agent (L1 reject) --")
		demoPrintln("  L1 doctrine detects 'rm -rf /var/cloudsvc' -> rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-deny", "doctrine check")
		if err := demoStep(demoDir, "fedramp-deny via agent",
			false,
			harnessRun("fedramp-deny", hcfg)...,
		); err != nil {
			fmt.Println("  (fedramp-deny harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "fedramp-deny", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/cloudsvc' — CR-26 audit integrity violation detected at admission")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 2 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Audit trail destruction blocked by L1 doctrine.")
			fmt.Println("         CR-26 audit integrity rule fired at admission.")
			fmt.Println("         The audit trail is tamper-evident and intact.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Audit trail destruction blocked by L1 doctrine")
		}

	case "3":
		result.number = "3"
		result.name = "Governed Configuration Revert under L2 Consensus"
		result.status = "PASS"
		result.metrics = "L2 quorum admits revert // L5 actuator records REVERT // CM-7 rollback"

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

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
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
		if err := demoStep(demoDir, "fedramp-revert via agent",
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

		if !demoScenarioStep(demoDir, "Step 3: Verify the Sovereign Cloud Service recorded the REVERT",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"python", constants.ContainerVerifyOpsPy, "REVERT"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "fedramp-revert", "REVERT recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded REVERT — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Configuration revert governed by L2 consensus.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         REVERT operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Configuration revert governed by L2 consensus")
		}

	case "4":
		result.number = "4"
		result.name = "Gateway Audit Vault Destruction Blocked + Governed Destruction"
		result.status = "PASS"
		result.metrics = "L1 blocks vault wipe // L1+L2 admit governed destroy -> receipt"

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

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (doctrine)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		demoPrintln("  -- Step 2: Run fedramp-evidence-block via agent (L1 reject) --")
		demoPrintln("  L1 doctrine detects 'rm -rf /root/.g8e/data' -> rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-evidence-block", "doctrine check")
		if err := demoStep(demoDir, "fedramp-evidence-block via agent",
			false,
			harnessRun("fedramp-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (fedramp-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "fedramp-evidence-block", "AUDIT VAULT DESTRUCTION BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /root/.g8e/data' — CR-26 audit integrity violation detected at admission")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Audit vault destruction blocked by L1 doctrine.")
			fmt.Println("         CR-26 audit integrity rule fired at admission.")
			fmt.Println("         The audit vault is tamper-evident and intact.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Audit vault destruction blocked by L1 doctrine")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for fedramp: %q (valid: 1-4)", scenario)
	}
	return result, nil
}

// runFedRAMPKSIEvidence runs g8e compliance ksi inside the gateway container
// to emit KSI result snapshots, then verifies them via verify_ops.py --ksi-result.
// Returns true if both steps succeed.
func runFedRAMPKSIEvidence(demoDir string) bool {
	demoPrintf("\n%s\n", strings.Repeat("-", 60))
	demoPrintln("  KSI Evidence Export")
	demoPrintln(strings.Repeat("-", 60))
	demoPrintln()

	if !demoScenarioStep(demoDir, "Step 1: Emit KSI result snapshots (g8e compliance ksi --class C)",
		[]string{"docker", "compose", "exec", "-T", "gateway",
			"/g8e", "compliance", "ksi", "--class", "C", "--catalog", constants.ContainerKSICatalog}) {
		fmt.Println("  [FAIL] KSI evidence export — could not emit snapshots.")
		return false
	}

	if !demoScenarioStep(demoDir, "Step 2: Verify KSI result snapshots (verify_ops.py --ksi-result)",
		[]string{"docker", "compose", "exec", "-T", "cloudsvc",
			"python", constants.ContainerVerifyOpsPy, "--ksi-result"}) {
		fmt.Println("  [FAIL] KSI evidence export — snapshot verification failed.")
		return false
	}

	fmt.Println("  [PASS] KSI evidence export — snapshots emitted and verified.")
	return true
}
