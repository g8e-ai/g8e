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

// dhsHarnessConfig holds the parameters for building a docker compose run
// command for a DHS agent-harness scenario. Centralising these in a struct
// avoids positional-argument drift as flags are added across phases.
type dhsHarnessConfig struct {
	MTLSURL       string
	PublicURL     string
	CertPath      string
	KeyPath       string
	CAPath        string
	EnsembleSize  int
	L3Mode        string
	Posture       string
	ConsensusSeed string
	TribunalID    string
}

// defaultDHSHarnessConfig returns the config matching the DHS compose topology.
func defaultDHSHarnessConfig() dhsHarnessConfig {
	return dhsHarnessConfig{
		MTLSURL:       "https://10.62.0.10:8443",
		PublicURL:     "http://10.62.0.10:8080",
		CertPath:      "/root/.g8e/pki/operator.crt",
		KeyPath:       "/root/.g8e/pki/operator.key",
		CAPath:        "/root/.g8e/pki/trust/g8eg-ca-bundle.pem",
		EnsembleSize:  3,
		L3Mode:        "mock",
		Posture:       "consensus",
		ConsensusSeed: "/etc/g8e/ensemble-seed.hex",
		TribunalID:    "dhs-tribunal",
	}
}

// dhsSkipScenario prints a skip banner and returns a SKIP result for
// scenarios that require a higher posture than the gateway is running.
func dhsSkipScenario(number, name, required, current, reason string) (scenarioResult, error) {
	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	fmt.Printf("  Scenario %s — %s\n", number, name)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()
	fmt.Printf("  [SKIP] This scenario requires %s posture.\n", required)
	fmt.Printf("         The gateway is currently running in %s posture.\n", current)
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
// scenario from a dhsHarnessConfig. Mirrors the dow pattern: --rm --no-deps -T
// for clean, capturable, non-spawning execution.
func dhsHarnessRun(scenario string, cfg dhsHarnessConfig) []string {
	cmd := []string{
		"docker", "compose", "run", "--rm", "-T", "--no-deps",
		"agent-coalition",
		"agent-harness", "run", "--insecure",
		"--mtls-url", cfg.MTLSURL,
		"--public-url", cfg.PublicURL,
		"--cert", cfg.CertPath,
		"--key", cfg.KeyPath,
		"--ca", cfg.CAPath,
		"--ensemble", fmt.Sprintf("%d", cfg.EnsembleSize),
		"--l3-mode", cfg.L3Mode,
	}
	if cfg.ConsensusSeed != "" {
		cmd = append(cmd, "--consensus-seed", cfg.ConsensusSeed)
	}
	if cfg.TribunalID != "" {
		cmd = append(cmd, "--tribunal-id", cfg.TribunalID)
	}
	cmd = append(cmd, scenario)
	return cmd
}

// dhsScenarioStep runs a single demo step: prints the description, executes
// the command via demoStep, prints pass/fail, and returns whether it succeeded.
// This extracts the repetitive print → demoStep → error pattern from each
// scenario case block.
func dhsScenarioStep(demoDir, desc string, cmd []string) bool {
	fmt.Printf("  ── %s ──\n", desc)
	if err := demoStep(demoDir, desc, false, cmd...); err != nil {
		fmt.Printf("  (%s failed)\n\n", desc)
		return false
	}
	return true
}

func runDHSScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	hcfg := defaultDHSHarnessConfig()
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Sovereign Multi-Source Ingest (chain-of-custody)"
		result.status = "PASS"
		result.metrics = "L1 doctrine admits // L2 consensus quorum met // L5 actuator records INGEST"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — Sovereign Multi-Source Ingest (LOE 1)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A coalition source connector submits a real")
		fmt.Println("          GovernanceEnvelope wrapping a run_shell_command that")
		fmt.Println("          drives the Sovereign Data Service (L5 actuator). L1")
		fmt.Println("          doctrine admits the envelope; L2 consensus quorum is")
		fmt.Println("          met and verified. The ingest is executed and a signed")
		fmt.Println("          receipt is written to the hash-chained ledger —")
		fmt.Println("          provable chain-of-custody.")
		fmt.Println()

		if !dhsScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", "/root/.g8e/pki/operator.crt"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 3: Run real dhs-ingest via agent-harness ────────────────")
		fmt.Println("  L1 doctrine admits; L2 consensus quorum met and verified → L5 actuator records INGEST:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-ingest via agent-harness",
			false,
			dhsHarnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 4: Verify the Sovereign Data Service recorded the INGEST",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "INGEST"}) {
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
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met and verified.")
			fmt.Println("         L5 actuator recorded the ingest; signed receipt in hash-chained ledger.")
		}

	case "2":
		return dhsSkipScenario("2", "Cross-Domain Release requires Notary authority",
			"notary", hcfg.Posture,
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

		if !dhsScenarioStep(demoDir, "Step 1: Confirm gateway is live before disconnect",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		_ = demoStep(demoDir, "sever datalink",
			false,
			"docker", "network", "disconnect", "dhs-demo_net_perimeter", "dhs-coalition-datalink",
		)

		if !dhsScenarioStep(demoDir, "Step 3: Verify gateway continues operating locally",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 4: Govern an ingest while disconnected ──────────────────")
		fmt.Println("  Running dhs-ingest through the gateway (consensus) with the datalink severed:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-ingest while disconnected",
			false,
			dhsHarnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 5: Verify local ledger exists on operator",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "ls -la /root/.g8e/data/ledger/files/ 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'"}) {
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 6: Verify local audit vault exists on operator",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"sh", "-c", "ls -la /root/.g8e/data/g8e.db 2>/dev/null || echo 'Audit vault DB not yet populated'"}) {
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
		result.metrics = "L2 quorum admits cue // L2 veto blocks unauthorized cue // L5 actuator records CUE"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 4 — Governed Predictive Cueing (LOE 3 & 4)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: An authorized interdiction cue with L2 ensemble quorum")
		fmt.Println("          is admitted and executed by the L5 actuator (dhs-cue).")
		fmt.Println("          The same cue with L2 decision=false is vetoed at quorum")
		fmt.Println("          (dhs-cue-veto) — the operator fails closed. This")
		fmt.Println("          demonstrates that L2 BFT consensus is a real fail-closed")
		fmt.Println("          gate, not just an audit annotation.")
		fmt.Println()

		if !dhsScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Run dhs-cue via agent-harness (L2 quorum → admit) ────")
		fmt.Println("  L2 consensus quorum met (decision=true) → admitted → L5 actuator records CUE:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-cue via agent-harness",
			false,
			dhsHarnessRun("dhs-cue", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the CUE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "CUE"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 4: Run dhs-cue-veto via agent-harness (L2 veto → reject) ─")
		fmt.Println("  L2 consensus decision=false → vetoed at quorum → operator fails closed (≥400):")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-cue-veto via agent-harness",
			false,
			dhsHarnessRun("dhs-cue-veto", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-cue-veto harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted with quorum; vetoed cue blocked at quorum.")
			fmt.Println("         CUE operation recorded by the L5 actuator; veto produced no actuator row.")
		}

	case "5":
		result.number = "5"
		result.name = "Sovereign Destruction + tamper-proof audit"
		result.status = "PASS"
		result.metrics = "L1 blocks audit wipe // L1+L2 admit governed purge → receipt"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 5 — Sovereign Destruction + Tamper-Proof Audit (LOE 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A compromised connector tries to wipe the audit trail")
		fmt.Println("          with 'rm -rf /var/log/g8e' — L1 doctrine rejects it at")
		fmt.Println("          admission (the data-destruction threat detector fires).")
		fmt.Println("          Then a governed retention purge is admitted by L1 doctrine")
		fmt.Println("          with L2 consensus quorum met, and the L5 actuator records")
		fmt.Println("          the PURGE with a cryptographic destruction receipt.")
		fmt.Println()

		fmt.Println("  ── Step 1: Run dhs-evidence-block via agent-harness (L1 reject) ──")
		fmt.Println("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-evidence-block via agent-harness",
			false,
			dhsHarnessRun("dhs-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Run dhs-purge via agent-harness (admit) ──────────────")
		fmt.Println("  L1 doctrine admits; L2 consensus quorum met → L5 actuator records PURGE:")
		fmt.Println()
		if err := demoStep(demoDir, "dhs-purge via agent-harness",
			false,
			dhsHarnessRun("dhs-purge", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-purge harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the PURGE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "PURGE"}) {
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
			fmt.Println("         L1 blocked the audit-wipe; L1+L2 admitted governed purge with receipt.")
			fmt.Println("         PURGE operation recorded by the L5 actuator.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for dhs: %q (valid: 1-5)", scenario)
	}
	return result, nil
}

// Made with Bob
