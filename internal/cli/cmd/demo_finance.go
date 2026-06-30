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

func runFinanceScenario(demoDir, scenario string) error {
	_, err := runFinanceScenarioWithResult(demoDir, scenario)
	return err
}

func runFinanceScenarioWithResult(demoDir, scenario string) (scenarioResult, error) {
	switch scenario {
	case "1":
		return runTwoLayerScenario(demoDir, twoLayerScenarioConfig{
			scenarioName:    "Unauthorized Trade Blocked",
			metrics:         "L1 doctrine: unauthorized_trade_execution (0.90 conf) // Network isolation: net_untrusted blocked",
			httpPort:        "8082",
			harnessScenario: "finance-unauthorized-trade",
			provesDescription: "Two-layer defense against unauthorized trading.\n" +
				"    Layer 1 — Network isolation: bad-actor on net_untrusted has no\n" +
				"              route to the trading ledger on net_secure.\n" +
				"    Layer 2 — Doctrine enforcement: the g8e gateway blocks unauthorized\n" +
				"              trade execution payloads at confidence >= 0.90.",
			step3Label:       "Submit unauthorized trade via agent",
			step3Description: "The agent submits a GovernanceEnvelope through the real\n  gateway via mTLS, attempting to execute an unauthorized trade.\n  L1 doctrine must block this at the gateway before execution:",
			passMessage: "Unauthorized trade blocked at both layers.\n" +
				"         Layer 1: network isolation (net_untrusted has no route to net_secure).\n" +
				"         Layer 2: doctrine unauthorized_trade_execution loaded at confidence 0.90.",
		})
	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for finance: %q (valid: 1)", scenario)
	}
}
