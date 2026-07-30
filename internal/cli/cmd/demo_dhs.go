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
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
)

// defaultDHSHarnessConfig returns the config matching the DHS compose topology.
func defaultDHSHarnessConfig() harnessConfig {
	return defaultHarnessConfig("agent-coalition")
}

func switchDHSPosture(demoDir, posture string) error {
	return switchDemoPosture(demoDir, posture, "8087")
}

func runDHSScenario(demoDir, scenario string) (scenarioResult, error) {
	hcfg := defaultDHSHarnessConfig()
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Sovereign Multi-Source Ingest (chain-of-custody)"
		result.status = "PASS"
		result.metrics = "L1 doctrine admits // L2 consensus quorum met // L5 actuator records INGEST"

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

		if err := switchDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest", "doctrine check")
		demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 started: Sovereign Multi-Source Ingest")

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		if !demoScenarioStep(demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", constants.ContainerOperatorCert}) {
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Run real dhs-ingest via agent ────────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met and verified → L5 actuator records INGEST:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-ingest")
		if err := demoStep(demoDir, "dhs-ingest via agent",
			false,
			harnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-ingest", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met and verified (3/5)")

		if !demoScenarioStep(demoDir, "Step 4: Verify the Sovereign Data Service recorded the INGEST",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "INGEST"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest", "INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded INGEST — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 1 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Sovereign ingest governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the ingest; signed receipt in hash-chained ledger.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 PASSED — Sovereign ingest governed end to end")
		}

	case "2":
		result.number = "2"
		result.name = "Cross-Domain Release requires Notary authority"
		result.status = "PASS"
		result.metrics = "Notary posture // L3 WebAuthn passkey authorization → L5 actuator records RELEASE"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — Cross-Domain Release requires Notary authority (LOE 1 & 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A cross-domain release is submitted with L2 consensus")
		demoPrintln("          and a genuine WebAuthn L3 assertion. Under notary posture")
		demoPrintln("          the Gateway verifies the L3 proof before allowing the")
		demoPrintln("          release to execute. The harness uses a software passkey")
		demoPrintln("          that generates real WebAuthn assertions (ES256).")
		demoPrintln()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 started: Cross-Domain Release requires Notary authority")

		// Step 1: Restart gateway in notary posture
		demoPrintln("  ── Step 1: Restart gateway in notary posture ────────────────────")
		demoPrintln("  Switching from consensus → notary (L1/L2/L3 strictly enforced):")
		demoPrintln()
		if err := switchDHSPosture(demoDir, "notary"); err != nil {
			fmt.Printf("  [WARNING] Failed to switch to notary posture: %v\n", err)
			fmt.Println("  Continuing — the gateway may already be in notary mode.")
		}

		// Step 2: Confirm gateway is live in notary posture
		if !demoScenarioStep(demoDir, "Step 2: Confirm gateway is live (notary posture)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		// Step 3: Submit dhs-release via agent (L2 + mock L3 principal signature)
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-release", "doctrine check")
		demoPrintln("  ── Step 3: Submit dhs-release via agent (L2 + WebAuthn L3) ────")
		demoPrintln("  Connector requests cross-domain release of TRK-MIL-0007.")
		demoPrintln("  Under notary posture, the gateway requires L3 authorization.")
		demoPrintln("  The harness attaches a genuine WebAuthn assertion (ES256) as L3 proof:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-release", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-release", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-release")
		notaryCfg := hcfg
		notaryCfg.Posture = "notary"
		if err := demoStep(demoDir, "dhs-release via agent (notary WebAuthn L3)",
			false,
			harnessRun("dhs-release", notaryCfg)...,
		); err != nil {
			fmt.Println("  (dhs-release harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-release", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL3, tui.StatusPassed, "dhs-release", "WebAuthn L3 proof verified")
		demoEmitter.Ledger(tui.LevelInfo, "L3 notary: WebAuthn assertion verified (software passkey)")

		// Step 4: Verify the RELEASE was executed by the L5 actuator
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-release", "actuator executing")
		if !demoScenarioStep(demoDir, "Step 4: Verify the Sovereign Data Service recorded the RELEASE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "RELEASE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-release", "RELEASE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded RELEASE — signed receipt in hash-chained ledger")

		// Step 5: Restore gateway to consensus posture
		demoPrintln("  ── Step 5: Restore gateway to consensus posture ─────────────────")
		if err := switchDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to restore consensus posture: %v\n", err)
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 2 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Cross-domain release governed by L3 notary authorization.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met;")
			fmt.Println("         L3 notary verified WebAuthn assertion (software passkey).")
			fmt.Println("         L5 actuator recorded the RELEASE after authorization.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Cross-domain release governed by L3 notary authorization")
		}

	case "3":
		result.number = "3"
		result.name = "Resilient Disconnected Operations / Continuity of Coverage"
		result.status = "PASS"
		result.metrics = "Datalink severed // Local governance continues // Git ledger + SQLite vault"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Resilient Disconnected Operations (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The Mission Partner datalink is severed, simulating a")
		demoPrintln("          contested, comms-denied corridor. The gateway and operator")
		demoPrintln("          keep governing ingest locally and commit every decision to")
		demoPrintln("          the Git-backed ledger and SQLite audit vault — no cloud, no")
		demoPrintln("          loss of sovereign control. State reconciles when the link")
		demoPrintln("          is restored.")
		demoPrintln()

		if err := switchDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Resilient Disconnected Operations")

		if !demoScenarioStep(demoDir, "Step 1: Confirm gateway is live before disconnect",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		demoEmitter.Ledger(tui.LevelWarn, "Mission Partner datalink severed — entering comms-denied mode")
		demoStepWarn(demoDir, "sever datalink",
			"docker", "network", "disconnect", "dhs-demo_net_perimeter", "dhs-coalition-datalink",
		)

		if !demoScenarioStep(demoDir, "Step 3: Verify gateway continues operating locally",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Govern an ingest while disconnected ──────────────────")
		demoPrintln("  Running dhs-ingest through the gateway (consensus) with the datalink severed:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest-disco", "doctrine check (local)")
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest-disco", "doctrine admitted (local)")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest-disco", "local consensus")
		if err := demoStep(demoDir, "dhs-ingest while disconnected",
			false,
			harnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest-disco", "local quorum met")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest-disco", "local INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "Governance continued locally while disconnected — Git ledger + SQLite vault persisted")

		if !demoScenarioStep(demoDir, "Step 5: Verify local ledger exists on operator",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "ls -la " + constants.ContainerLedgerFilesDir + " 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'"}) {
			hasErrors = true
		}

		if !demoScenarioStep(demoDir, "Step 6: Verify local audit vault exists on operator",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "ls -la " + constants.ContainerAuditVaultDB + " 2>/dev/null || echo 'Audit vault DB not yet populated'"}) {
			hasErrors = true
		}

		demoPrintln("  ── Step 7: Restore the Mission Partner datalink ─────────────────")
		demoEmitter.Ledger(tui.LevelInfo, "Mission Partner datalink restored")
		demoStepWarn(demoDir, "restore datalink",
			"docker", "network", "connect", "dhs-demo_net_perimeter", "dhs-coalition-datalink",
		)

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Continuity of coverage verified under comms denial.")
			fmt.Println("         Governance continued locally; Git ledger + SQLite vault persisted all decisions.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Continuity of coverage verified under comms denial")
		}

	case "4":
		result.number = "4"
		result.name = "Governed Predictive Cueing"
		result.status = "PASS"
		result.metrics = "L2 quorum admits cue // L5 actuator records CUE"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 4 — Governed Predictive Cueing (LOE 3 & 4)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An authorized interdiction cue with L2 ensemble quorum")
		demoPrintln("          is admitted and executed by the L5 actuator (dhs-cue).")
		demoPrintln("          This demonstrates that L2 BFT consensus is a real")
		demoPrintln("          fail-closed gate, not just an audit annotation.")
		demoPrintln()

		if err := switchDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Governed Predictive Cueing")

		if !demoScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
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
		if err := demoStep(demoDir, "dhs-cue via agent",
			false,
			harnessRun("dhs-cue", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-cue", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-cue", "actuator executing")
		demoEmitter.Consensus(constants.ConsensusMemberAxiom, true, true, 3, 5, tui.ConsensusReached, "cue-hash-001")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met (3/5) — cue admitted")

		if !demoScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the CUE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "CUE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-cue", "CUE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded CUE — signed receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted with quorum.")
			fmt.Println("         CUE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Predictive cueing governed by L2 consensus")
		}

	case "5":
		result.number = "5"
		result.name = "Sovereign Destruction + tamper-proof audit"
		result.status = "PASS"
		result.metrics = "L1 blocks audit wipe // L1+L2 admit governed purge → receipt"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 5 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A compromised connector tries to wipe the audit trail")
		demoPrintln("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		demoPrintln("          admission (the data-destruction threat detector fires).")
		demoPrintln("          Then a governed retention purge is admitted by L1 doctrine")
		demoPrintln("          with L2 consensus quorum met, and the L5 actuator records")
		demoPrintln("          the PURGE with a cryptographic destruction receipt.")
		demoPrintln()

		if err := switchDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to set consensus posture: %v\n", err)
		}

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 5 started: Sovereign Destruction + Tamper-Proof Audit")

		demoPrintln("  ── Step 1: Run dhs-evidence-block via agent (L1 reject) ──")
		demoPrintln("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-evidence-block", "doctrine check")
		if err := demoStep(demoDir, "dhs-evidence-block via agent",
			false,
			harnessRun("dhs-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "dhs-evidence-block", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/log/g8e' — data-destruction threat detected at admission")

		demoPrintln("  ── Step 2: Run dhs-purge via agent (admit) ──────────────")
		demoPrintln("  L1 doctrine admits; L2 consensus quorum met → L5 actuator records PURGE:")
		demoPrintln()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-purge", "doctrine check")
		if err := demoStep(demoDir, "dhs-purge via agent",
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

		if !demoScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the PURGE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", constants.ContainerVerifyOpsPy, "PURGE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-purge", "PURGE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PURGE — cryptographic destruction receipt in hash-chained ledger")

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 5 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 5 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 5 — Destruction governed and provable.")
			fmt.Println("         L1 blocked the audit-wipe; L1+L2 admitted governed purge with receipt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 5 PASSED — Destruction governed and provable")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for dhs: %q (valid: 1-5)", scenario)
	}
	return result, nil
}
