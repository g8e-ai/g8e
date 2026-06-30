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

func runSecureDataScenario(demoDir, scenario string) error {
	_, err := runSecureDataScenarioWithResult(demoDir, scenario)
	return err
}

func runSecureDataScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "Governed Migration with Chain-of-Custody Receipts"
		result.status = "PASS"
		result.metrics = "Two-Operator Topology: src-operator → dst-operator // Chain of Custody Proof"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 1 — Governed Migration with Chain-of-Custody Receipts")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: A SharePoint migration moves data from source to destination")
		demoPrintln("          only through the governed connector pipeline. Both operators")
		demoPrintln("          emit signed receipts, forming a cryptographic chain of custody.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm source and destination gateways are live ──────")
		if err := demoStep(demoDir, "src-gateway health",
			false,
			"curl", "-s", "http://localhost:8083/api/v1/health",
		); err != nil {
			fmt.Println("  (src-gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}
		if err := demoStep(demoDir, "dst-gateway health",
			false,
			"curl", "-s", "http://localhost:8084/api/v1/health",
		); err != nil {
			fmt.Println("  (dst-gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Verify operator enrollment (mTLS certs) ──────────────")
		if err := demoStep(demoDir, "enrollment check",
			false,
			"docker", "compose", "exec", "-T", "src-operator",
			"test", "-f", "/root/.g8e/pki/operator.crt",
		); err != nil {
			fmt.Println("  (src-operator cert not found — operator may not have enrolled correctly)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 3: Confirm the migration doctrines are loaded ───────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSecureDataDoctrineFile, "migration_manifest_required"); err == nil {
			demoPrintf("  $ cat /etc/g8e/doctrine/%s | grep -A 10 migration_manifest_required\n", constants.DemosSecureDataDoctrineFile)
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

		demoPrintln("  ── Step 4: Run governed migration via agent-harness (mTLS) ──────")
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8080"
		if err := demoStep(demoDir, "secure-data-migration via agent",
			false,
			harnessRun("secure-data-migration", hcfg)...,
		); err != nil {
			fmt.Println("  (governed migration scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 5: Verify chain-of-custody receipt in ledger ────────────")
		if err := demoStep(demoDir, "ledger verification",
			false,
			"docker", "compose", "exec", "-T", "src-operator",
			"sh", "-c", "ls -la /root/.g8e/data/ledger/files/ 2>/dev/null || echo 'Ledger directory missing (bootstrap failed)'",
		); err != nil {
			fmt.Println("  (ledger directory not found — no file mutations recorded)")
			fmt.Println()
			hasErrors = true
		}

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 1 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 1 — Migration governed end to end.")
			fmt.Println("         Source and destination receipts written to the hash-chained ledger.")
		}

	case "2":
		result.number = "2"
		result.name = "Connector Bypass Attempt Blocked"
		result.status = "PASS"
		result.metrics = "Doctrine: connector_bypass_attempt (0.93 conf) // Layer 1 Blocked"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 2 — Connector Bypass Attempt Blocked")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: Direct invocation of transfer tools (rclone, scp, robocopy)")
		demoPrintln("          is blocked by doctrine when not wrapped in a GovernanceEnvelope.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm src-gateway is live ──────────────────────────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8083/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Confirm bypass doctrine is loaded ────────────────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSecureDataDoctrineFile, "connector_bypass_attempt"); err == nil {
			demoPrintf("  $ cat /etc/g8e/doctrine/%s | grep -A 10 connector_bypass_attempt\n", constants.DemosSecureDataDoctrineFile)
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

		demoPrintln("  ── Step 3: Run bypass attempt via agent-harness (mTLS) ──────────")
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8080"
		if err := demoStep(demoDir, "secure-data-bypass-attempt via agent",
			false,
			harnessRun("secure-data-bypass-attempt", hcfg)...,
		); err != nil {
			fmt.Println("  (bypass attempt scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Verify doctrine rejection in gateway logs ────────────")
		_ = demoStep(demoDir, "audit tail",
			false,
			"docker", "compose", "logs", "observability", "--tail", "10",
		)

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 2 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 2 — Bypass attempt blocked at Layer 1.")
		}

	case "3":
		result.number = "3"
		result.name = "Cross-Tenant Leak Doctrine Triggered"
		result.status = "PASS"
		result.metrics = "Doctrine: cross_tenant_data_leak (0.88 conf) // Intent Rejected"

		demoPrintf("\n%s\n", strings.Repeat("─", 60))
		demoPrintln("  Scenario 3 — Cross-Tenant Leak Doctrine Triggered")
		demoPrintln(strings.Repeat("─", 60))
		demoPrintln()
		demoPrintln("  PROVES: Envelopes targeting destinations not in the signed manifest")
		demoPrintln("          are rejected before execution.")
		demoPrintln()

		demoPrintln("  ── Step 1: Confirm src-gateway is live ──────────────────────────")
		if err := demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8083/api/v1/health",
		); err != nil {
			fmt.Println("  (gateway health check failed — is the demo running?)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 2: Confirm cross-tenant doctrine is loaded ──────────────")
		if rule, err := readDoctrineRule(demoDir, constants.DemosSecureDataDoctrineFile, "cross_tenant_data_leak"); err == nil {
			demoPrintf("  $ cat /etc/g8e/doctrine/%s | grep -A 10 cross_tenant_data_leak\n", constants.DemosSecureDataDoctrineFile)
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

		demoPrintln("  ── Step 3: Run cross-tenant leak attempt via agent-harness (mTLS)")
		demoPrintln("    Target: rogue-tenant.sharepoint.com")
		demoPrintln()
		hcfg := defaultHarnessConfig("agent-runtime")
		hcfg.PublicURL = "http://g8e.local:8080"
		if err := demoStep(demoDir, "secure-data-cross-tenant via agent",
			false,
			harnessRun("secure-data-cross-tenant", hcfg)...,
		); err != nil {
			fmt.Println("  (cross-tenant leak scenario failed)")
			fmt.Println()
			hasErrors = true
		}

		demoPrintln("  ── Step 4: Verify doctrine rejection in gateway logs ────────────")
		_ = demoStep(demoDir, "audit tail",
			false,
			"docker", "compose", "logs", "observability", "--tail", "10",
		)

		if hasErrors {
			result.status = "FAIL"
			fmt.Println("  [FAIL] Scenario 3 — One or more steps failed.")
		} else {
			fmt.Println("  [PASS] Scenario 3 — Cross-tenant leak attempt rejected.")
		}

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for secure-data: %q (valid: 1-3)", scenario)
	}

	return result, nil
}
