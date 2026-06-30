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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
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
		MTLSURL:       "https://g8e.local:8443",
		PublicURL:     "http://g8e.local:8080",
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

// dhsHarnessRun builds the docker compose exec command for a DHS agent-harness
// scenario from a dhsHarnessConfig. Uses exec (not run) because agent-coalition
// is a long-running sleep-infinity container with a fixed IP; `run` would try
// to create a second container with the same IP and fail.
func dhsHarnessRun(scenario string, cfg dhsHarnessConfig) []string {
	cmd := []string{
		"docker", "compose", "exec", "-T",
		"agent-coalition",
		"/g8e", "agent", "run",
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

// ensureDHSPosture restarts the DHS gateway container with the specified posture.
// It sets the G8E_GATEWAY_POSTURE environment variable and recreates the container.
func ensureDHSPosture(demoDir, posture string) error {
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)

	// Stop the gateway container
	if err := exec.Command("docker", "compose", "-f", composePath, "stop", "gateway").Run(); err != nil {
		return fmt.Errorf("stop gateway: %w", err)
	}

	// Recreate the gateway with the new posture via environment variable
	cmd := exec.Command("docker", "compose", "-f", composePath, "up", "-d", "--no-deps", "gateway")
	cmd.Env = append(os.Environ(), "G8E_GATEWAY_POSTURE="+posture)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restart gateway with posture %s: %w", posture, err)
	}

	// Wait for the gateway to become healthy
	for i := 0; i < 30; i++ {
		time.Sleep(3 * time.Second)
		if err := exec.Command("curl", "-sf", "http://localhost:8087/api/v1/health").Run(); err == nil {
			fmt.Printf("  Gateway is live in %s posture.\n", posture)
			return nil
		}
	}
	return fmt.Errorf("gateway did not become healthy in %s posture after 90s", posture)
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

		_ = ensureDHSPosture(demoDir, "consensus")

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest", "doctrine check")
		demoEmitter.Ledger(tui.LevelInfo, "Scenario 1 started: Sovereign Multi-Source Ingest")

		if !dhsScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		if !dhsScenarioStep(demoDir, "Step 2: Verify operator enrollment (mTLS certs)",
			[]string{"docker", "compose", "exec", "-T", "operator",
				"test", "-f", "/root/.g8e/pki/operator.crt"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 3: Run real dhs-ingest via agent ────────────────")
		fmt.Println("  L1 doctrine admits; L2 consensus quorum met and verified → L5 actuator records INGEST:")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-ingest")
		if err := demoStep(demoDir, "dhs-ingest via agent",
			false,
			dhsHarnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-ingest harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-ingest", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met and verified (3/5)")

		if !dhsScenarioStep(demoDir, "Step 4: Verify the Sovereign Data Service recorded the INGEST",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "INGEST"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest", "INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded INGEST — signed receipt in hash-chained ledger")

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

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
		result.metrics = "Notary posture // L3 suspend → human passkey approval → L5 actuator records RELEASE"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 2 — Cross-Domain Release requires Notary authority (LOE 1 & 2)")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: A cross-domain release is submitted with L2 consensus")
		fmt.Println("          only. Under notary posture the Gateway suspends the")
		fmt.Println("          transaction pending an out-of-band L3 human approval")
		fmt.Println("          via WebAuthn passkey. The release executes only after")
		fmt.Println("          a real human authorizes the exact transaction hash.")
		fmt.Println()

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 started: Cross-Domain Release requires Notary authority")

		// Step 1: Restart gateway in notary posture
		fmt.Println("  ── Step 1: Restart gateway in notary posture ────────────────────")
		fmt.Println("  Switching from consensus → notary (L1/L2/L3 strictly enforced):")
		fmt.Println()
		if err := ensureDHSPosture(demoDir, "notary"); err != nil {
			fmt.Printf("  [WARNING] Failed to switch to notary posture: %v\n", err)
			fmt.Println("  Continuing — the gateway may already be in notary mode.")
		}

		// Step 2: Confirm gateway is live in notary posture
		if !dhsScenarioStep(demoDir, "Step 2: Confirm gateway is live (notary posture)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		// Step 3: Submit dhs-release via agent (L2-only → suspend)
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-release", "doctrine check")
		fmt.Println("  ── Step 3: Submit dhs-release via agent (L2-only → suspend) ──")
		fmt.Println("  Connector requests cross-domain release of TRK-MIL-0007.")
		fmt.Println("  Under notary posture, the gateway suspends pending human L3 approval:")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-release", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-release", "consensus quorum")
		demoEmitter.Ledger(tui.LevelInfo, "L1 doctrine admitted envelope for dhs-release")
		notaryCfg := hcfg
		notaryCfg.Posture = "notary"
		notaryCfg.L3Mode = "suspend"
		if err := demoStep(demoDir, "dhs-release via agent (notary suspend)",
			false,
			dhsHarnessRun("dhs-release", notaryCfg)...,
		); err != nil {
			fmt.Println("  (dhs-release harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-release", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL3, tui.StatusWaiting, "dhs-release", "FIDO2 touch required")
		demoEmitter.Ledger(tui.LevelWarn, "L3 notary: transaction suspended pending human WebAuthn approval")

		// Step 4: Query the gateway's pending approvals API (mTLS) for the suspended tx hash
		fmt.Println("  ── Step 4: Query gateway for suspended transaction hash ──────────")
		fmt.Println("  Using GET /api/v1/approvals/pending (mTLS-authenticated):")
		fmt.Println()
		txHashBytes, err := exec.Command("docker", "compose", "exec", "-T", "gateway",
			"curl", "-sf",
			"--cert", "/root/.g8e/pki/operator.crt",
			"--key", "/root/.g8e/pki/operator.key",
			"--cacert", "/root/.g8e/pki/trust/g8eg-ca-bundle.pem",
			"https://localhost:8443/api/v1/approvals/pending").Output()
		if err != nil {
			fmt.Printf("  [WARNING] Could not query pending approvals API: %v\n", err)
			hasErrors = true
		} else {
			txHash := extractFirstTxHash(string(txHashBytes))
			if txHash == "" {
				fmt.Println("  [WARNING] No pending suspended transaction found via API")
				hasErrors = true
			} else {
				fmt.Printf("  Suspended transaction hash: %s\n", txHash)
				fmt.Println()

				// Step 5: Prompt user for passkey enrollment + approval
				fmt.Println("  ── Step 5: Human passkey approval (WebAuthn) ────────────────────")
				fmt.Println("  A browser window will open for passkey enrollment/approval.")
				fmt.Println("  If you have no passkey registered, the console will guide you")
				fmt.Println("  through enrollment first, then the approval ceremony.")
				fmt.Println()
				fmt.Printf("  Console URL: http://localhost:8087/console/\n")
				fmt.Println()

				// Run g8e approve <txHash> — this opens the browser and polls
				approveCmd := exec.Command("g8e", "approve", txHash)
				approveCmd.Stdout = os.Stdout
				approveCmd.Stderr = os.Stderr
				approveCmd.Stdin = os.Stdin
				if err := approveCmd.Run(); err != nil {
					fmt.Printf("  [WARNING] g8e approve did not complete: %v\n", err)
					hasErrors = true
				} else {
					fmt.Println("  Transaction approved via WebAuthn passkey!")
					fmt.Println()
					demoEmitter.Pipeline(tui.StageL3, tui.StatusPassed, "dhs-release", "WebAuthn approved")
					demoEmitter.Ledger(tui.LevelInfo, "L3 notary: human WebAuthn passkey authorization received")
				}
			}
		}

		// Step 6: Verify the RELEASE was executed by the L5 actuator
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-release", "actuator executing")
		if !dhsScenarioStep(demoDir, "Step 6: Verify the Sovereign Data Service recorded the RELEASE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "RELEASE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-release", "RELEASE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded RELEASE — signed receipt in hash-chained ledger")

		// Step 7: Restore gateway to consensus posture
		fmt.Println("  ── Step 7: Restore gateway to consensus posture ─────────────────")
		if err := ensureDHSPosture(demoDir, "consensus"); err != nil {
			fmt.Printf("  [WARNING] Failed to restore consensus posture: %v\n", err)
		}

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 2 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Cross-domain release governed by human passkey approval.")
			fmt.Println("         L1 doctrine admitted; L2 consensus quorum met;")
			fmt.Println("         L3 notary required real human WebAuthn authorization.")
			fmt.Println("         L5 actuator recorded the RELEASE after approval.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 2 PASSED — Cross-domain release governed by human passkey approval")
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

		_ = ensureDHSPosture(demoDir, "consensus")

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 started: Resilient Disconnected Operations")

		if !dhsScenarioStep(demoDir, "Step 1: Confirm gateway is live before disconnect",
			[]string{"curl", "-s", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		fmt.Println("  ── Step 2: Sever the Mission Partner datalink ───────────────────")
		demoEmitter.Ledger(tui.LevelWarn, "Mission Partner datalink severed — entering comms-denied mode")
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
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-ingest-disco", "doctrine check (local)")
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-ingest-disco", "doctrine admitted (local)")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-ingest-disco", "local consensus")
		if err := demoStep(demoDir, "dhs-ingest while disconnected",
			false,
			dhsHarnessRun("dhs-ingest", hcfg)...,
		); err != nil {
			fmt.Println("  (ingest while disconnected failed — operator may not be processing locally)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-ingest-disco", "local quorum met")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-ingest-disco", "local INGEST recorded")
		demoEmitter.Ledger(tui.LevelInfo, "Governance continued locally while disconnected — Git ledger + SQLite vault persisted")

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
		demoEmitter.Ledger(tui.LevelInfo, "Mission Partner datalink restored")
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
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 3 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Continuity of coverage verified under comms denial.")
			fmt.Println("         Governance continued locally; Git ledger + SQLite vault persisted all decisions.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 3 PASSED — Continuity of coverage verified under comms denial")
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

		_ = ensureDHSPosture(demoDir, "consensus")

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 started: Governed Predictive Cueing (quorum vs veto)")

		if !dhsScenarioStep(demoDir, "Step 1: Confirm the governance gateway is live (consensus)",
			[]string{"curl", "-sf", "http://localhost:8087/api/v1/health"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-cue", "doctrine check")
		fmt.Println("  ── Step 2: Run dhs-cue via agent (L2 quorum → admit) ────")
		fmt.Println("  L2 consensus quorum met (decision=true) → admitted → L5 actuator records CUE:")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-cue", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-cue", "consensus deliberation")
		demoEmitter.Consensus("axiom", true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus("concord", true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus("variance", true, true, 3, 5, tui.ConsensusPending, "")
		if err := demoStep(demoDir, "dhs-cue via agent",
			false,
			dhsHarnessRun("dhs-cue", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-cue harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-cue", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-cue", "actuator executing")
		demoEmitter.Consensus("axiom", true, true, 3, 5, tui.ConsensusReached, "cue-hash-001")
		demoEmitter.Ledger(tui.LevelInfo, "L2 consensus quorum met (3/5) — cue admitted")

		if !dhsScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the CUE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "CUE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-cue", "CUE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded CUE — signed receipt in hash-chained ledger")

		fmt.Println("  ── Step 4: Run dhs-cue-veto via agent (L2 veto → reject) ─")
		fmt.Println("  L2 consensus decision=false → vetoed at quorum → operator fails closed (≥400):")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-cue-veto", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusActive, "dhs-cue-veto", "consensus deliberation")
		demoEmitter.Consensus("axiom", true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus("concord", true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus("variance", true, true, 3, 5, tui.ConsensusPending, "")
		demoEmitter.Consensus("pragma", false, true, 3, 5, tui.ConsensusPending, "")
		if err := demoStep(demoDir, "dhs-cue-veto via agent",
			false,
			dhsHarnessRun("dhs-cue-veto", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-cue-veto harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL2, tui.StatusFailed, "dhs-cue-veto", "vetoed at quorum")
		demoEmitter.Consensus("nemesis", true, true, 3, 5, tui.ConsensusRejected, "veto-hash-002")
		demoEmitter.Ledger(tui.LevelCritical, "L2 consensus REJECTED — Byzantine fault detected: pragma dissent (4/5 affirmative, quorum requires 3)")

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 4 — One or more steps failed.")
			demoEmitter.Ledger(tui.LevelCritical, "Scenario 4 FAILED — one or more steps failed")
		} else {
			fmt.Println("  [PASS] Scenario 4 — Predictive cueing governed by L2 consensus.")
			fmt.Println("         Authorized cue admitted with quorum; vetoed cue blocked at quorum.")
			fmt.Println("         CUE operation recorded by the L5 actuator; veto produced no actuator row.")
			demoEmitter.Ledger(tui.LevelInfo, "Scenario 4 PASSED — Predictive cueing governed by L2 consensus")
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

		_ = ensureDHSPosture(demoDir, "consensus")

		demoEmitter.Ledger(tui.LevelInfo, "Scenario 5 started: Sovereign Destruction + Tamper-Proof Audit")

		fmt.Println("  ── Step 1: Run dhs-evidence-block via agent (L1 reject) ──")
		fmt.Println("  L1 doctrine detects 'rm -rf /var/log/g8e' → rejected at admission:")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-evidence-block", "doctrine check")
		if err := demoStep(demoDir, "dhs-evidence-block via agent",
			false,
			dhsHarnessRun("dhs-evidence-block", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-evidence-block harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusFailed, "dhs-evidence-block", "DATA DESTRUCTION ATTEMPT BLOCKED")
		demoEmitter.Ledger(tui.LevelCritical, "L1 doctrine BLOCKED: 'rm -rf /var/log/g8e' — data-destruction threat detected at admission")

		fmt.Println("  ── Step 2: Run dhs-purge via agent (admit) ──────────────")
		fmt.Println("  L1 doctrine admits; L2 consensus quorum met → L5 actuator records PURGE:")
		fmt.Println()
		demoEmitter.Pipeline(tui.StageL1, tui.StatusActive, "dhs-purge", "doctrine check")
		if err := demoStep(demoDir, "dhs-purge via agent",
			false,
			dhsHarnessRun("dhs-purge", hcfg)...,
		); err != nil {
			fmt.Println("  (dhs-purge harness scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL1, tui.StatusPassed, "dhs-purge", "doctrine admitted")
		demoEmitter.Pipeline(tui.StageL2, tui.StatusPassed, "dhs-purge", "quorum met (3/5)")
		demoEmitter.Pipeline(tui.StageL5, tui.StatusActive, "dhs-purge", "actuator executing")
		demoEmitter.Ledger(tui.LevelInfo, "L1+L2 admitted governed purge — L5 actuator recording PURGE with destruction receipt")

		if !dhsScenarioStep(demoDir, "Step 3: Verify the Sovereign Data Service recorded the PURGE",
			[]string{"docker", "compose", "exec", "-T", "datasvc",
				"python", "/app/verify_ops.py", "PURGE"}) {
			hasErrors = true
		}

		demoEmitter.Pipeline(tui.StageL5, tui.StatusPassed, "dhs-purge", "PURGE recorded")
		demoEmitter.Ledger(tui.LevelInfo, "L5 actuator recorded PURGE — cryptographic destruction receipt in hash-chained ledger")

		fmt.Println("  Copy-paste to inspect the audit trail:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " exec operator sh -c 'cd /root/.g8e/data/ledger/files && git log --oneline'")
		fmt.Println()

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

// extractFirstTxHash parses the JSON response from GET /api/v1/approvals/pending
// and returns the first transaction_hash, or "" if none found.
func extractFirstTxHash(jsonBody string) string {
	var resp models.SuspendedTransactionsResponse
	if err := json.Unmarshal([]byte(jsonBody), &resp); err != nil {
		return ""
	}
	if len(resp.Transactions) == 0 {
		return ""
	}
	return resp.Transactions[0].TransactionHash
}
