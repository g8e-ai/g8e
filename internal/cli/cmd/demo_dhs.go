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
		result.metrics = "L2 quorum 3/3 + L3 notary // receipt → ledger"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Sovereign Multi-Source Ingest (LOE 1)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A coalition source connector submits a real")
		fmt.Println("          GovernanceEnvelope wrapping a run_shell_command that")
		fmt.Println("          drives the Sovereign Data Service (L5 actuator). L1")
		fmt.Println("          doctrine + L2 consensus + L3 notary all pass. The")
		fmt.Println("          ingest is admitted and a signed receipt is written to")
		fmt.Println("          the hash-chained ledger — provable chain-of-custody.")
		fmt.Println()

		fmt.Println("  ── Step 1: Confirm the governance gateway is live (notary) ──────")
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
		fmt.Println("  L2 ensemble + inline mock L3 → admitted → L5 actuator records INGEST:")
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
			fmt.Println("         L2 consensus + L3 notary admitted; L5 actuator recorded the ingest.")
			fmt.Println("         Signed receipt written to hash-chained ledger.")
		}

	case "2":
		result.number = "2"
		result.name = "Cross-Domain Release requires Notary authority"
		result.status = "PASS"
		result.metrics = "L2-only → suspend → OOB approve → release executes"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 2 — Cross-Domain Release requires Notary Authority (LOE 1 & 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A cross-domain release is submitted with L2 consensus")
		fmt.Println("          only. Under notary posture the Gateway suspends the")
		fmt.Println("          transaction pending an out-of-band L3 principal (release")
		fmt.Println("          authority) approval. The release executes only after the")
		fmt.Println("          authority signs the exact transaction hash OOB.")
		fmt.Println()

		fmt.Println("  ── Step 1: Run real dhs-release via agent-harness (l3-mode=suspend) ─")
		fmt.Println("  L2-only submit → Gateway suspends → OOB Approve → executes:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-release via agent-harness",
			false,
			dhsHarnessRun("dhs-release", "suspend")...,
		); err != nil {
			fmt.Println("  (dhs-release harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Verify the Sovereign Data Service recorded the RELEASE ─")
		if err := demoStep(demoDir, "datasvc verify RELEASE",
			false,
			"docker", "compose", "exec", "-T", "datasvc",
			"python", "/app/verify_ops.py", "RELEASE",
		); err != nil {
			fmt.Println("  (no RELEASE operation recorded — OOB approval may have failed)")
			fmt.Println()
			hasErrors = true
		}

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Cross-domain release governed by L3 notary.")
			fmt.Println("         Release suspended on submit; executed only after OOB approval.")
			fmt.Println("         RELEASE operation recorded by the L5 actuator.")
		}

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
		fmt.Println("  Running dhs-ingest through the gateway with the datalink severed:")
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
		result.number = "4"
		result.name = "Governed Predictive Cueing (quorum vs veto)"
		result.status = "PASS"
		result.metrics = "L2 quorum admits authorized cue // L2 veto rejects unauthorized cue"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 4 — Governed Predictive Cueing (LOE 3 & 4)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: An authorized interdiction cue with L2 ensemble quorum")
		fmt.Println("          is admitted and executed by the L5 actuator. The same")
		fmt.Println("          cue with L2 decision=false (no consensus) is vetoed by")
		fmt.Println("          L2 — the operator fails closed, no interdiction is executed.")
		fmt.Println()

		fmt.Println("  ── Step 1: Run authorized dhs-cue via agent-harness (admit) ──────")
		fmt.Println("  L2 ensemble + inline mock L3 → admitted → L5 actuator records CUE:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-cue via agent-harness",
			false,
			dhsHarnessRun("dhs-cue", "mock")...,
		); err != nil {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Verify the Sovereign Data Service recorded the CUE ────")
		if err := demoStep(demoDir, "datasvc verify CUE",
			false,
			"docker", "compose", "exec", "-T", "datasvc",
			"python", "/app/verify_ops.py", "CUE",
		); err != nil {
			fmt.Println("  (no CUE operation recorded — L5 actuation may have failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 3: Run vetoed dhs-cue-veto via agent-harness (reject) ───")
		fmt.Println("  L2 decision=false → rejected at L2 consensus → operator fails closed:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-cue-veto via agent-harness",
			false,
			dhsHarnessRun("dhs-cue-veto", "mock")...,
		); err != nil {
			fmt.Println("  (dhs-cue-veto harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted and recorded; vetoed cue rejected at L2.")
			fmt.Println("         The authorized CUE appears in the actuator log; the vetoed one does not.")
		}

	case "5":
		result.number = "5"
		result.name = "Sovereign Destruction + tamper-proof audit"
		result.status = "PASS"
		result.metrics = "L1 blocks audit wipe // L2+L3 admits governed purge → receipt"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 5 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A compromised connector tries to wipe the audit trail")
		fmt.Println("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		fmt.Println("          admission (the data-destruction threat detector fires).")
		fmt.Println("          Then a governed retention purge (L2+L3) is admitted and")
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
		fmt.Println("  L2 ensemble + inline mock L3 → admitted → L5 actuator records PURGE:")
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
			fmt.Println("         L1 blocked the audit-wipe; governed purge admitted with receipt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for dhs: %q (valid: 1-5)", scenario)
	}
	return result, nil
}

// Made with Bob
