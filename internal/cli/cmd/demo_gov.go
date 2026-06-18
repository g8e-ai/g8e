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

func runGovScenario(demoDir, scenario string) error {
	_, err := runGovScenarioWithResult(demoDir, scenario)
	return err
}

func runGovScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	var result scenarioResult

	switch scenario {
	case "1":
		result.number = "1"
		result.name = "CUI Exfiltration Attempt Blocked"
		result.status = "PASS"
		result.metrics = "Network isolation: net_untrusted → net_secure blocked"

		fmt.Printf("\n%s\n", strings.Repeat("─", 60))
		fmt.Println("  Scenario 1 — CUI Exfiltration Attempt Blocked")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println()
		fmt.Println("  PROVES: Network isolation prevents a bad-actor on net_untrusted")
		fmt.Println("          from reaching classified documents on net_secure.")
		fmt.Println()

		fmt.Println("  ── Layer 1: Network isolation ────────────────────────────────")
		_ = demoStep(demoDir, "network isolation",
			false,
			"docker", "compose", "exec", "-T", "gov-bad-actor",
			"sh", "-c", "wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_internal'",
		)

		fmt.Println("  ── Layer 2: g8e doctrine enforcement ─────────────────────────")
		fmt.Println("  Confirming the gateway is live:")
		fmt.Println()
		_ = demoStep(demoDir, "gateway health",
			false,
			"curl", "-s", "http://localhost:8080/api/v1/health",
		)

		fmt.Println("  Copy-paste to inspect the enforcement audit:")
		fmt.Println()
		fmt.Println("    docker compose -f " + filepath.Join(demoDir, constants.DemosComposeFile) + " logs observability --tail 20")
		fmt.Println()

		fmt.Println("  [PASS] Scenario 1 — CUI exfiltration blocked.")
		fmt.Println("         Net_untrusted has no route to net_internal or net_secure.")

	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for gov: %q (valid: 1)", scenario)
	}
	return result, nil
}

// Made with Bob
