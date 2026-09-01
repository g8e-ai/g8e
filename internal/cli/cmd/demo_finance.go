// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func runFinanceScenario(ctx context.Context, demoDir, scenario string) (*compliancev1.DemoScenarioResult, error) {
	switch scenario {
	case "1":
		return runTwoLayerScenario(ctx, demoDir, twoLayerScenarioConfig{
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
		return nil, fmt.Errorf("invalid scenario number for finance: %q (valid: 1)", scenario)
	}
}
