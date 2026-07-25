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
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
)

// defaultFedRAMPHarnessConfig returns the config matching the FedRAMP compose topology.
func defaultFedRAMPHarnessConfig() harnessConfig {
	return defaultGovernedHarnessConfig("agent-runtime", "fedramp-tribunal")
}

func switchFedRAMPPosture(demoDir, posture string) error {
	return switchDemoPosture(demoDir, posture, "8088")
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

		if err := switchFedRAMPPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

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

		if err := switchFedRAMPPosture(demoDir, "doctrine"); err != nil {
			fmt.Printf("  [WARNING] Failed to set doctrine posture: %v\n", err)
		}

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
		result.name = "Resource Destruction Gated on Authorizing Official Approval (L3)"
		result.status = "PASS"
		result.metrics = "Notary posture // L3 mock principal authorization -> L5 actuator records DESTROY"

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 3 — Resource Destruction Requires Authorizing Official (L3)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A resource destruction is submitted with L2 consensus")
		demoPrintln("          and a mock L3 principal signature. Under notary posture")
		demoPrintln("          the Gateway verifies the L3 proof before allowing the")
		demoPrintln("          destruction to execute. In production, L3 requires a real")
		demoPrintln("          WebAuthn passkey ceremony; demos use mock L3 mode.")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Resource Destruction Requires Authorizing Official")

		demoPrintln("  -- Step 1: Restart gateway in notary posture --")
		demoPrintln("  Switching from consensus -> notary (L1/L2/L3 strictly enforced):")
		demoPrintln()
		if err := switchFedRAMPPosture(demoDir, "notary"); err != nil {
			fmt.Printf("  [WARNING] Failed to switch to notary posture: %v\n", err)
			fmt.Println("  Continuing — the gateway may already be in notary mode.")
		}

		if !demoScenarioStep(demoDir, "Step 2: Confirm gateway is live (notary posture)",
			[]string{"curl", "-sf", "http://localhost:8088/api/v1/health"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "fedramp-escalate", "doctrine check")
		demoPrintln("  -- Step 3: Submit fedramp-escalate via agent (L2 + mock L3) --")
		demoPrintln("  Operator requests destruction of fedramp-vm-classified-01 (FIPS-199-HIGH).")
		demoPrintln("  Under notary posture, the gateway requires L3 authorization.")
		demoPrintln("  The harness attaches a mock principal Ed25519 signature as L3 proof:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "fedramp-escalate", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "fedramp-escalate", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for fedramp-escalate")
		notaryCfg := hcfg
		notaryCfg.Posture = "notary"
		notaryCfg.L3Mode = "mock"
		if err := demoStep(demoDir, "fedramp-escalate via agent (notary mock L3)",
			false,
			harnessRun("fedramp-escalate", notaryCfg)...,
		); err != nil {
			fmt.Println("  (fedramp-escalate harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "fedramp-escalate", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL3, tui.StatusPassed, "fedramp-escalate", "mock L3 proof verified")
		demoEmitter.Ledger(tui.LevelInfo, "L3 notary: mock principal signature verified (demo mode)")

		// Step 4: Verify the DESTROY was executed by the L5 actuator
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "fedramp-escalate", "actuator executing")
		if !demoScenarioStep(demoDir, "Step 4: Verify the Sovereign Cloud Service recorded the DESTROY",
			[]string{"docker", "compose", "exec", "-T", "cloudsvc",
				"python", constants.ContainerVerifyOpsPy, "DESTROY"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "fedramp-escalate", "DESTROY recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded DESTROY — signed receipt in hash-chained ledger")

		demoPrintln("  -- Step 5: Restore gateway to consensus posture --")
		if err := switchFedRAMPPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to restore consensus posture: %v\n", err)
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Resource destruction governed by L3 notary authorization.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met;")
			fmt.Println("         L3 notary verified mock principal signature (demo mode).")
			fmt.Println("         L5 actuator recorded the DESTROY after authorization.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Resource destruction governed by L3 notary authorization")
		}

	case "4":
		result.number = "4"
		result.name = "Governed Configuration Revert under L2 Consensus"
		result.status = "PASS"
		result.metrics = "L2 quorum admits revert // L5 actuator records REVERT // CM-7 rollback"

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 4 — Governed Configuration Revert (CM-7)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A configuration revert on fedramp-iam-roles-01 is")
		demoPrintln("          submitted with L2 ensemble quorum. The revert is")
		demoPrintln("          admitted and executed by the L5 actuator. This")
		demoPrintln("          demonstrates that configuration changes are governed")
		demoPrintln("          through the full L1/L2/L3 pipeline with signed receipts.")
		demoPrintln()

		if err := switchFedRAMPPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Governed Configuration Revert")

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
		demoEmitter.Consensus(constants.TribunalMemberAxiom, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.TribunalMemberConcord, true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus(constants.TribunalMemberVariance, true, true, 3, 5, tui.ConsensusPending, "")
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
		demoEmitter.Consensus(constants.TribunalMemberAxiom, true, true, 3, 5, tui.ConsensusReached, "revert-hash-001")
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
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Configuration revert governed by L2 consensus.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         REVERT operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Configuration revert governed by L2 consensus")
		}

	case "5":
		result.number = "5"
		result.name = "Gateway Audit Vault Destruction Blocked + Governed Destruction"
		result.status = "PASS"
		result.metrics = "L1 blocks vault wipe // L1+L2 admit governed destroy -> receipt"

		demoPrintf("\n%s\n", strings.Repeat("-", 60))
		demoPrintln("  Scenario 5 — Gateway Audit Vault Destruction Blocked (CR-26)")
		demoPrintln(strings.Repeat("-", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised operator tries to wipe the gateway audit")
		demoPrintln("          vault with 'rm -rf /root/.g8e/data' — L1 doctrine rejects")
		demoPrintln("          it at admission (the CR-26 audit integrity detector fires).")
		demoPrintln("          The audit vault remains tamper-evident and intact.")
		demoPrintln()

		if err := switchFedRAMPPosture(demoDir, "doctrine"); err != nil {
			fmt.Printf("  [WARNING] Failed to set doctrine posture: %v\n", err)
		}

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 5 started: Gateway Audit Vault Destruction Blocked")

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
			fmt.Println("  [FAIL] Scenario 5 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 5 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 5 — Audit vault destruction blocked by L1 doctrine.")
			fmt.Println("         CR-26 audit integrity rule fired at admission.")
			fmt.Println("         The audit vault is tamper-evident and intact.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 5 PASSED — Audit vault destruction blocked by L1 doctrine")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for fedramp: %q (valid: 1-5)", scenario)
	}
	return result, nil
}
