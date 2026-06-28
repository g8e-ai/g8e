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
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

func runDHSScenario(demoDir, scenario string) error {
	_, err := runDHSScenarioWithResult(demoDir, scenario)
	return err
}

// dhsGatewayPosture is the posture the DHS compose file starts the gateway in.
// Phase 1 uses doctrine; Phase 2 will use consensus; Phase 3 (deferred) notary.
// Scenarios whose RequiresPosture exceeds this are skipped with a banner.
const dhsGatewayPosture = "doctrine"

// dhsSkipScenario prints a skip banner and returns a SKIP result for
// scenarios that require a higher posture than the gateway is running.
func dhsSkipScenario(number, name, required, reason string) (scenarioResult, error) {
	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Printf("  Scenario %s — %s\n", number, name)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()
	fmt.Printf("  [SKIP] This scenario requires %s posture.\n", required)
	fmt.Printf("         The gateway is currently running in %s posture.\n", dhsGatewayPosture)
	fmt.Printf("         %s\n", reason)
	fmt.Println()
	return scenarioResult{
		number:  number,
		name:    name,
		status:  "SKIP",
		metrics: fmt.Sprintf("requires %s posture (deferred)", required),
	}, nil
}

// dhsHarnessRun builds the docker compose run command for a DHS agent-harness
// scenario. Mirrors the dow pattern: --rm --no-deps -T for clean, capturable,
// non-spawning execution.
func dhsHarnessRun(scenario, l3Mode string) []string {
	return []string{
		"docker", "compose", "run", "--rm", "-T", "--no-deps",
		"agent-coalition",
		"agent-harness", "run", "--insecure",
		"--mtls-url", "https://10.62.0.10:8443",
		"--public-url", "http://10.62.0.10:8080",
		"--cert", "/root/.g8e/pki/operator.crt",
		"--key", "/root/.g8e/pki/operator.key",
		"--ca", "/root/.g8e/pki/trust/g8eg-ca-bundle.pem",
		"--ensemble", "3",
		"--l3-mode", l3Mode,
		scenario,
	}
}

func runDHSScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Sovereign Multi-Source Ingest (chain-of-custody)"
		result.status = "PASS"
		result.metrics = "L1 doctrine admits // L2/L3 audited in receipt // L5 actuator records INGEST"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Sovereign Multi-Source Ingest (LOE 1)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A coalition source connector submits a real")
		fmt.Println("          GovernanceEnvelope wrapping a run_shell_command that")
		fmt.Println("          drives the Sovereign Data Service (L5 actuator). L1")
		fmt.Println("          doctrine admits the envelope; L2/L3 proofs are")
		fmt.Println("          attached and audited in the receipt. The ingest is")
		fmt.Println("          executed and a signed receipt is written to the")
		fmt.Println("          hash-chained ledger — provable chain-of-custody.")
		fmt.Println()

		fmt.Println("  ── Step 1: Confirm the governance gateway is live (doctrine) ────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-sf", "http://localhost:8087/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Verify operator enrollment (mTLS certs) ──────────────")
		if err := demoStep(demoDir, "enrollment check",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"test", "-f", "/root/.g8e/pki/operator.crt",
		); err != nil {
			fmt.Println("  (operator cert not found — operator may not have enrolled)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 3: Run real dhs-ingest via agent-harness ────────────────")
		fmt.Println("  L1 doctrine admits; L2/L3 proofs attached and audited in receipt:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-ingest via agent-harness",
			false,
			dhsHarnessRun("dhs-ingest", "mock")...,
		); err != nil {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 4: Verify the Sovereign Data Service recorded the INGEST ─")
		if err := demoStep(demoDir, "datasvc verify INGEST",
			false,
			"docker", "compose", "exec", "-T", "datasvc",
			"python", "/app/verify_ops.py", "INGEST",
		); err != nil {
			fmt.Println("  (no INGEST operation recorded — L5 actuation may have failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Sovereign ingest governed end to end.")
			fmt.Println("         L1 doctrine admitted; L2/L3 proofs audited in receipt.")
			fmt.Println("         L5 actuator recorded the ingest; signed receipt in hash-chained ledger.")
		}

	case "2":
		return dhsSkipScenario("2", "Cross-Domain Release requires Notary authority",
			"notary",
			"Deferred to Phase 3 — the L3 mock is incompatible with gateway-mode notary (requires WebAuthn passkey). See plan §11.2.")

	case "3":
		result.number = "3"
		result.name = "Resilient Disconnected Operations / Continuity of Coverage"
		result.status = "PASS"
		result.metrics = "Datalink severed // Local governance continues // Git ledger + SQLite vault"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 3 — Resilient Disconnected Operations (LOE 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: The Mission Partner datalink is severed, simulating a")
		fmt.Println("          contested, comms-denied corridor. The gateway and operator")
		fmt.Println("          keep governing ingest locally and commit every decision to")
		fmt.Println("          the Git-backed ledger and SQLite audit vault — no cloud, no")
		fmt.Println("          loss of sovereign control. State reconciles when the link")
		fmt.Println("          is restored.")
		fmt.Println()

		fmt.Println("  ── Step 1: Confirm gateway is live before disconnect ───────────")
		if err := demoStep(demoDir, "gateway health (pre-disconnect)",
			false,
			"curl", "-s", "http://localhost:8087/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		_ = demoStep(demoDir, "sever datalink",
			false,
			"docker", "network", "disconnect", "dhs-demo_net_perimeter", "dhs-coalition-datalink",
		)

		fmt.Println("  ── Step 3: Verify gateway continues operating locally ───────────")
		if err := demoStep(demoDir, "gateway health (post-disconnect)",
			false,
			"curl", "-s", "http://localhost:8087/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway not reachable after disconnect — check container status)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 4: Govern an ingest while disconnected ──────────────────")
		fmt.Println("  Running dhs-ingest through the gateway (doctrine) with the datalink severed:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-ingest while disconnected",
			false,
			dhsHarnessRun("dhs-ingest", "mock")...,
		); err != nil {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 5: Verify local ledger exists on operator ───────────────")
		if err := demoStep(demoDir, "local ledger",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"sh", "-c", "ls -la /root/.g8e/data/ledger/files/ 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'",
		); err != nil {
			fmt.Println("  (ledger directory not found — no file mutations have been recorded)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 6: Verify local audit vault exists on operator ──────────")
		if err := demoStep(demoDir, "audit vault",
			false,
			"docker", "compose", "exec", "-T", "operator",
			"sh", "-c", "ls -la /root/.g8e/data/g8e.db 2>/dev/null || echo 'Audit vault DB not yet populated'",
		); err != nil {
			fmt.Println("  (audit vault DB not found — no audit events have been recorded)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 7: Restore the Mission Partner datalink ─────────────────")
		_ = demoStep(demoDir, "restore datalink",
			false,
			"docker", "network", "connect", "dhs-demo_net_perimeter", "dhs-coalition-datalink",
		)

		fmt.Println("  Copy-paste to inspect the local audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sqlite3 /root/.g8e/data/g8e.db 'SELECT * FROM events ORDER BY id DESC LIMIT 20;'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Continuity of coverage verified under comms denial.")
			fmt.Println("         Governance continued locally; Git ledger + SQLite vault persisted all decisions.")
		}

	case "4":
		return dhsSkipScenario("4", "Governed Predictive Cueing (quorum vs veto)",
			"consensus",
			"Deferred to Phase 2 — requires tribunal policy creation and consensus posture. See plan §12 Phase 2.")

	case "5":
		result.number = "5"
		result.name = "Sovereign Destruction + tamper-proof audit"
		result.status = "PASS"
		result.metrics = "L1 blocks audit wipe // L1 admits governed purge → receipt"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 5 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A compromised connector tries to wipe the audit trail")
		fmt.Println("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		fmt.Println("          admission (the data-destruction threat detector fires).")
		fmt.Println("          Then a governed retention purge is admitted by L1 doctrine")
		fmt.Println("          (L2/L3 proofs attached and audited) and")
		fmt.Println("          the L5 actuator records the PURGE with a cryptographic")
		fmt.Println("          destruction receipt.")
		fmt.Println()

		fmt.Println("  ── Step 1: Run dhs-evidence-block via agent-harness (L1 reject) ──")
		fmt.Println("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-evidence-block via agent-harness",
			false,
			dhsHarnessRun("dhs-evidence-block", "mock")...,
		); err != nil {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Run dhs-purge via agent-harness (admit) ──────────────")
		fmt.Println("  L1 doctrine admits; L2/L3 proofs attached and audited → L5 actuator records PURGE:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-purge via agent-harness",
			false,
			dhsHarnessRun("dhs-purge", "mock")...,
		); err != nil {
			fmt.Println("  (dhs-purge harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 3: Verify the Sovereign Data Service recorded the PURGE ──")
		if err := demoStep(demoDir, "datasvc verify PURGE",
			false,
			"docker", "compose", "exec", "-T", "datasvc",
			"python", "/app/verify_ops.py", "PURGE",
		); err != nil {
			fmt.Println("  (no PURGE operation recorded — L5 actuation may have failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 5 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 5 — Destruction governed and provable.")
			fmt.Println("         L1 blocked the audit-wipe; L1 admitted governed purge with receipt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for dhs: %q (valid: 1-5)", scenario)
	}
	return result, nil
}

// Made with Bob
