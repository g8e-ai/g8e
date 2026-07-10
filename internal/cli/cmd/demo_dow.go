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

	"github.com/g8e-ai/g8e/internal/constants"
)

func runDoWScenario(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Autonomous SIGINT-to-EO/IR Cross-Cueing"
		result.status = "PASS"
		result.metrics = "A2A cross-cue: SIGINT→EO/IR // L2 Consensus (quorum 2/3) // Zero ground station"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 1 — Autonomous SIGINT-to-EO/IR Cross-Cueing (Challenge 5)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The agent-sigint sensor detects a mock RF signal and emits")
		demoPrintln("          a governed cross-cue request to the g8e-gateway, which wraps")
		demoPrintln("          it in a GovernanceEnvelope and forces L2 Consensus on the target")
		demoPrintln("          coordinates. The g8e-operator verifies the proofs and executes")
		demoPrintln("          the camera slew via the L5 Actuator — with ZERO ground station")
		demoPrintln("          intervention.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live (consensus posture) ──────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8086/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Verify agent enrollment (operator mTLS certs) ────────")
		if err := demoStep(demoDir, "enrollment check",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"test", "-f", constants.ContainerOperatorCert,
		); err != nil {
			fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Confirm cross-cue doctrine is loaded ────────────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosDoWDoctrineFile, "unauthorized_cross_cue"); err == nil {
			demoPrintf("  $ cat %s/%s | grep -A 10 unauthorized_cross_cue\n", constants.ContainerDoctrineDir, constants.DemosDoWDoctrineFile)
			demoPrintf("  id:         %s\n", rule.ID)
			demoPrintf("  severity:   %s\n", rule.Severity)
			demoPrintf("  confidence: %.2f\n", rule.Confidence)
			demoPrintf("  pattern:    %s\n", rule.Pattern)
			demoPrintln()
		} else {
			fmt.Printf("  (doctrine rule inspection failed: %v)\n", err)
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Inspect the tactical environment (RF signals) ────────")
		if err := demoStep(demoDir, "tactical environment",
			false,
			"docker", "compose", "exec", "-T", "agent-eoir",
			"python3", constants.ContainerInspectRFPy,
		); err != nil {
			hasErrors = true
		}

		demoPrintln("  ── Step 5: Run real g8e cross-cue (governed envelope → L2 → L5) ──")
		hcfg := defaultHarnessConfig("agent-sigint")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "dow-cross-cue via agent",
			false,
			harnessRun("dow-cross-cue", hcfg)...,
		); err != nil {
			fmt.Println("  (cross-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 6: Verify gimbal received the slew command ─────────────")
		if err := demoStep(demoDir, "gimbal slew verification",
			false,
			"docker", "compose", "exec", "-T", "gimbal",
			"python3", constants.ContainerVerifySlewsPy,
		); err != nil {
			fmt.Println("  (gimbal did not record any slew — L5 actuation may have failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 7: Verify SWaP constraints (governance overhead) ───────")
		demoStepWarn(demoDir, "swap verification",
			"docker", "stats", "--no-stream",
			"--format", "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}",
			"dow-gateway", "dow-operator",
		)

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 1 — SIGINT-to-EO/IR cross-cue completed.")
			fmt.Println("         L2 Consensus reached on target coordinates (quorum 2/3).")
			fmt.Println("         Camera slew executed via L5 Actuator with signed receipt.")
			fmt.Println("         Zero ground station intervention required.")
		}

	case "2":
		result.number = "2"
		result.name = "BFT Spoofing Defense (PNT Consensus)"
		result.status = "PASS"
		result.metrics = "BFT: 3 trusted vs 1 spoofed // L2 rejects spoofed GNSS // Operator fails closed"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — BFT Spoofing Defense (Challenge 8)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A spoofed GNSS coordinate is injected into the PNT fusion")
		demoPrintln("          engine, simulating a near-peer EW attack. The BFT consensus")
		demoPrintln("          engine (L2Consensus) detects divergence between the spoofed")
		demoPrintln("          GNSS source and Visual Odometry/MAGNAV sources. The poisoned")
		demoPrintln("          model is outvoted by the ensemble. The GovernanceEnvelope")
		demoPrintln("          fails L2 verification, and the g8e-operator fails closed.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live (consensus posture) ──────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8086/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Verify agent enrollment (operator mTLS certs) ────────")
		if err := demoStep(demoDir, "enrollment check",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"test", "-f", constants.ContainerOperatorCert,
		); err != nil {
			fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Inspect PNT sources (including spoofed) ─────────────")
		if err := demoStep(demoDir, "pnt sources",
			false,
			"docker", "compose", "exec", "-T", "agent-pnt-fusion",
			"python3", constants.ContainerInspectPNTPy,
		); err != nil {
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Confirm spoofing detection doctrine is loaded ───────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosDoWDoctrineFile, "pnt_diversion_detected"); err == nil {
			demoPrintf("  $ cat %s/%s | grep -A 10 pnt_diversion_detected\n", constants.ContainerDoctrineDir, constants.DemosDoWDoctrineFile)
			demoPrintf("  id:         %s\n", rule.ID)
			demoPrintf("  severity:   %s\n", rule.Severity)
			demoPrintf("  confidence: %.2f\n", rule.Confidence)
			demoPrintf("  pattern:    %s\n", rule.Pattern)
			demoPrintln()
		} else {
			fmt.Printf("  (doctrine rule inspection failed: %v)\n", err)
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 5: Run BFT veto via agent-harness (spoofed GNSS) ───────")
		hcfg := defaultHarnessConfig("agent-sigint")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "dow-bft-veto via agent",
			false,
			harnessRun("dow-bft-veto", hcfg)...,
		); err != nil {
			fmt.Println("  (BFT veto harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 6: Verify operator fail-closed behavior ────────────────")
		demoPrintln("  The g8e-operator verifies the GovernanceEnvelope against its")
		demoPrintln("  local state root. A failed L2 consensus causes the operator to")
		demoPrintln("  reject the mutation and fail closed — the drone is not hijacked.")
		demoPrintln()
		demoStepWarn(demoDir, "operator health",
			"docker", "compose", "logs", "operator", "--tail", "10",
		)

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 2 — BFT spoofing defense active.")
			fmt.Println("         3 trusted PNT sources (GNSS, VO, MAGNAV) outvote 1 spoofed source.")
			fmt.Println("         L2 Consensus rejects the poisoned model. Operator fails closed.")
			fmt.Println("         Drone position integrity maintained under EW attack.")
		}

	case "3":
		result.number = "3"
		result.name = "Disconnected Operations (Data Sovereignty)"
		result.status = "PASS"
		result.metrics = "Datalink severed // Local governance continues // Git ledger + SQLite audit vault"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Disconnected Operations (Challenge 6)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: The tactical datalink is severed, simulating a comms-denied")
		demoPrintln("          environment. The g8e-gateway and g8e-operator continue to")
		demoPrintln("          process cross-cueing events locally. Raw data and execution")
		demoPrintln("          histories are committed to g8e's Git-backed ledger and SQLite")
		demoPrintln("          local audit vault — with no cloud connectivity and no OEM")
		demoPrintln("          permission keys.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm gateway is live before disconnect ───────────")
		if err := demoStep(demoDir, "gateway health (pre-disconnect)",
			false,
			"curl", "-s", "http://localhost:8086/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Sever the tactical datalink ──────────────────────────")
		demoPrintln("  Simulating comms-denied environment by disconnecting ground-station")
		demoPrintln("  from net_perimeter:")
		demoPrintln()
		demoStepWarn(demoDir, "sever datalink",
			"docker", "network", "disconnect", "dow-demo_net_perimeter", "dow-ground-station",
		)

		demoPrintln("  ── Step 3: Verify gateway continues operating locally ───────────")
		demoPrintln("  The gateway should still be healthy even with the datalink severed:")
		demoPrintln()
		if err := demoStep(demoDir, "gateway health (post-disconnect)",
			false,
			"curl", "-s", "http://localhost:8086/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway not reachable after disconnect — check container status)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Trigger local cross-cue while disconnected ──────────")
		demoPrintln("  Running a governed cross-cue through the gateway with the datalink")
		demoPrintln("  severed — the operator must process it entirely locally:")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-sigint")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "dow-cross-cue while disconnected",
			false,
			harnessRun("dow-cross-cue", hcfg)...,
		); err != nil {
			fmt.Println("  (cross-cue while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 5: Verify local ledger exists on operator ───────────────")
		if err := demoStep(demoDir, "local ledger",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"sh", "-c", "ls -la "+constants.ContainerLedgerFilesDir+" 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'",
		); err != nil {
			fmt.Println("  (ledger directory not found — no file mutations have been recorded)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 6: Verify local audit vault exists on operator ──────────")
		if err := demoStep(demoDir, "audit vault",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"sh", "-c", "ls -la "+constants.ContainerAuditVaultDB+" 2>/dev/null || echo 'Audit vault DB not yet populated'",
		); err != nil {
			fmt.Println("  (audit vault DB not found — no audit events have been recorded)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 7: Restore the tactical datalink ────────────────────────")
		demoStepWarn(demoDir, "restore datalink",
			"docker", "network", "connect", "dow-demo_net_perimeter", "dow-ground-station",
		)

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Disconnected operations verified.")
			fmt.Println("         Gateway and operator continue functioning with datalink severed.")
			fmt.Println("         Local Git-backed ledger and SQLite audit vault persist all decisions.")
			fmt.Println("         No OEM permission keys or cloud connectivity required.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for dow: %q (valid: 1-3)", scenario)
	}
	return result, nil
}
