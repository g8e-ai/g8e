// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

// dowPersona is the identity the DoW cross-cue agent wears.
var dowSigintAgent = clientpkg.Persona{
	ID:        "dow-sigint-agent",
	UserAgent: "g8e-dow-sigint/1.x (tactical edge)",
}

// DoWCrossCueArgs holds the configurable parameters for the cross-cue scenario.
// These are set by the demo runner via environment variables or flags so the
// scenario can target the correct gimbal endpoint in different topologies.
var DoWCrossCueArgs = struct {
	GimbalEndpoint string
	Azimuth        string
	Elevation      string
}{
	GimbalEndpoint: "10.43.0.40:9000",
	Azimuth:        "45.0",
	Elevation:      "30.0",
}

func dowScenarios() []Scenario {
	return []Scenario{
		{
			Name: "dow-cross-cue", Title: "DoW: SIGINT→EO/IR cross-cue with governed camera slew", Persona: dowSigintAgent, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))

				argumentsJSON := shellCommandArgs("slew", DoWCrossCueArgs.GimbalEndpoint, DoWCrossCueArgs.Azimuth, DoWCrossCueArgs.Elevation)
				r.note("submitting run_shell_command envelope: slew %s %s %s",
					DoWCrossCueArgs.GimbalEndpoint, DoWCrossCueArgs.Azimuth, DoWCrossCueArgs.Elevation)

				txHash, status, body, err := c.SubmitMaximal(ctx, dowSigintAgent, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     argumentsJSON,
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit envelope: %w", err)
				}
				r.tx(txHash)
				r.note("cross-cue envelope %s submitted (admission status %d)", short(txHash), status)

				if status >= 400 {
					return fmt.Errorf("cross-cue envelope rejected (status %d): %s", status, string(body))
				}

				r.note("governance envelope admitted — operator executing slew via L5 actuator")
				r.note("gimbal endpoint: %s, az=%s, el=%s", DoWCrossCueArgs.GimbalEndpoint, DoWCrossCueArgs.Azimuth, DoWCrossCueArgs.Elevation)
				return nil
			},
		},
		{
			Name: "dow-bft-veto", Title: "DoW: BFT veto rejects spoofed GNSS cross-cue", Persona: dowSigintAgent, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("submitting envelope with L2 decision=false (BFT veto — spoofed GNSS)")

				veto := false
				txHash, status, body, err := c.SubmitMaximal(ctx, dowSigintAgent, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     shellCommandArgs("slew", "10.43.0.40:9000", "99.0", "99.0"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Decision:          &veto,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit veto envelope: %w", err)
				}
				r.tx(txHash)
				r.note("veto envelope %s submitted (status %d)", short(txHash), status)

				if status < 400 {
					return fmt.Errorf("veto envelope was accepted (status %d) — expected rejection", status)
				}
				r.note("envelope correctly rejected by L2 consensus (BFT veto)")
				r.note("response: %s", string(body))
				return nil
			},
		},
	}
}
