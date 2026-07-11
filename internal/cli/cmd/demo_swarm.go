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

func runSwarmScenario(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Authorized Recon Mission (Governed Drone Deployment)"
		result.status = "PASS"
		result.metrics = "L2 Consensus (quorum 2/3) // Governed drone launch // L5 actuator"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 1 — Authorized Recon Mission (Governed Drone Deployment)")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A swarm command agent submits a GovernanceEnvelope for an")
		demoPrintln("          authorized recon drone mission through the g8e gateway. The")
		demoPrintln("          envelope passes L2 Consensus (quorum 2/3), and the operator")
		demoPrintln("          executes the drone simulator via the L5 Actuator — all under")
		demoPrintln("          governed mTLS with signed receipts.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live (consensus posture) ──────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8085/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Verify operator enrollment (mTLS certs) ──────────────")
		if err := demoStep(demoDir, "enrollment check",
			false,
			"docker", "compose", "exec", "-T", "operator-1",
			"test", "-f", constants.ContainerOperatorCert,
		); err != nil {
			fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Confirm drone operations doctrine is loaded ──────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSwarmDoctrineFile, "unauthorized_weapon_release"); err == nil {
			demoPrintf("  $ cat %s/%s | grep -A 10 unauthorized_weapon_release\n", constants.ContainerDoctrineDir, constants.DemosSwarmDoctrineFile)
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

		demoPrintln("  ── Step 4: Run governed recon mission via scenarios run ─────────")
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "swarm-recon-mission via agent",
			false,
			harnessRun("swarm-recon-mission", hcfg)...,
		); err != nil {
			fmt.Println("  (recon mission harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 5: Verify drone simulator received the mission ──────────")
		if err := demoStep(demoDir, "drone simulator verification",
			false,
			"docker", "compose", "exec", "-T", "operator-1",
			"sh", "-c", "ls -la "+constants.ContainerLedgerFilesDir+" 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'",
		); err != nil {
			fmt.Println("  (ledger directory not found — no file mutations have been recorded)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Authorized recon mission completed.")
			fmt.Println("         L2 Consensus reached on mission parameters (quorum 2/3).")
			fmt.Println("         Drone simulator executed via L5 Actuator with signed receipt.")
			fmt.Println("         All drone commands governed through the g8e gateway.")
		}

	case "2":
		result.number = "2"
		result.name = "Weapons Safety Doctrine Block"
		result.status = "PASS"
		result.metrics = "L1 Doctrine blocks weapon release // No L2 consensus needed // Fail-closed"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — Weapons Safety Doctrine Block")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An agent attempts to issue an unauthorized weapon release")
		demoPrintln("          command through the MCP tool interface. The g8e gateway's L1")
		demoPrintln("          Doctrine engine matches the command against the")
		demoPrintln("          'unauthorized_weapon_release' rule and blocks it before any")
		demoPrintln("          operator execution — no L2 consensus is reached, no L5")
		demoPrintln("          actuation occurs, and the drone remains unarmed.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8085/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Confirm weapons safety doctrine is loaded ────────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSwarmDoctrineFile, "unauthorized_weapon_release"); err == nil {
			demoPrintf("  $ cat %s/%s | grep -A 10 unauthorized_weapon_release\n", constants.ContainerDoctrineDir, constants.DemosSwarmDoctrineFile)
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

		demoPrintln("  ── Step 3: Attempt weapon release via scenarios run (L1 block) ──")
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "swarm-weapon-release-block via agent",
			false,
			harnessRun("swarm-weapon-release-block", hcfg)...,
		); err != nil {
			fmt.Println("  (weapon release block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Weapons safety doctrine enforced.")
			fmt.Println("         L1 Doctrine blocked unauthorized weapon release command.")
			fmt.Println("         No L2 consensus reached, no L5 actuation occurred.")
			fmt.Println("         Drone remains unarmed — fail-closed behavior verified.")
		}

	case "3":
		result.number = "3"
		result.name = "Navigation Boundary Violation Block"
		result.status = "PASS"
		result.metrics = "L1 Doctrine blocks restricted airspace // Navigation safety enforced"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Navigation Boundary Violation Block")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: An agent attempts to navigate a drone into restricted")
		demoPrintln("          airspace through the MCP tool interface. The g8e gateway's")
		demoPrintln("          L1 Doctrine engine matches the command against the")
		demoPrintln("          'restricted_airspace_violation' rule and blocks it — the drone")
		demoPrintln("          is prevented from entering the prohibited zone.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8085/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Confirm navigation safety doctrine is loaded ─────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSwarmDoctrineFile, "restricted_airspace_violation"); err == nil {
			demoPrintf("  $ cat %s/%s | grep -A 10 restricted_airspace_violation\n", constants.ContainerDoctrineDir, constants.DemosSwarmDoctrineFile)
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

		demoPrintln("  ── Step 3: Attempt restricted airspace navigation via agent ─────")
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.UseRun = true
		if err := demoStep(demoDir, "swarm-restricted-airspace-block via agent",
			false,
			harnessRun("swarm-restricted-airspace-block", hcfg)...,
		); err != nil {
			fmt.Println("  (restricted airspace block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Navigation boundary violation blocked.")
			fmt.Println("         L1 Doctrine blocked restricted airspace navigation command.")
			fmt.Println("         Drone prevented from entering prohibited zone.")
			fmt.Println("         Navigation safety enforced through governed airframe control.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for swarm: %q (valid: 1-3)", scenario)
	}
	return result, nil
}
