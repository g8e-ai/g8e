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
)

func runGovScenario(demoDir, scenario string) error {
	_, err := runGovScenarioWithResult(demoDir, scenario)
	return err
}

func runGovScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	switch scenario {
	case "1":
		return runTwoLayerScenario(demoDir, twoLayerScenarioConfig{
			scenarioName:    "CUI Exfiltration Attempt Blocked",
			metrics:         "L1 doctrine: cui_exfil_attempt (0.95 conf) // Network isolation: net_untrusted blocked",
			httpPort:        "8080",
			harnessScenario: "gov-cui-exfil-block",
			provesDescription: "Two-layer defense against CUI exfiltration.\n" +
				"    Layer 1 — Network isolation: bad-actor on net_untrusted has no\n" +
				"              route to net_internal or net_secure.\n" +
				"    Layer 2 — Doctrine enforcement: the g8e gateway blocks CUI\n" +
				"              exfiltration payloads at confidence >= 0.95 (cui_exfil_attempt).",
			step3Label:       "Submit CUI exfiltration attempt via agent",
			step3Description: "The agent submits a GovernanceEnvelope through the real\n  gateway via mTLS, attempting to read CUI documents for exfiltration.\n  L1 doctrine must block this at the gateway before execution:",
			passMessage: "CUI exfiltration blocked at both layers.\n" +
				"         Layer 1: network isolation (net_untrusted has no route to net_secure).\n" +
				"         Layer 2: doctrine cui_exfil_attempt loaded at confidence 0.95.",
		})
	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for gov: %q (valid: 1)", scenario)
	}
}
