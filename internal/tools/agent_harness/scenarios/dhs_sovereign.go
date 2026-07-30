// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

// The DHS "Persistent Sovereign Capability" scenarios exercise the REAL g8e
// governance path for a coalition common-operating-picture data plane:
//
//   - a coalition source connector submits a genuine GovernanceEnvelope wrapping
//     a run_shell_command that drives the Sovereign Data Service (the L5 actuator,
//     analogous to the DoW gimbal);
//   - sovereign data operations (ingest / release / cue / purge) are admitted only
//     after L1 doctrine + L2 consensus (+ L3 notary where required) pass;
//   - a cross-domain release is gated on an out-of-band principal (release
//     authority) approval — the real L3 notary suspend/approve flow;
//   - an interdiction cue without ensemble quorum is vetoed by L2;
//   - an attempt to wipe the audit trail is rejected by L1 before it ever reaches
//     the actuator.
//
// Nothing here is mocked except the actor identities (personas) and the data
// content. The Gateway, Operator, hashing, consensus verification, notary flow,
// hash-chained ledger, and signed receipts are all real.

// dhsConnector is the identity a coalition source connector wears.
var dhsConnector = clientpkg.Persona{
	ID:        "dhs-coalition-connector",
	UserAgent: "g8e-dhs-connector/1.x (coalition data plane)",
}

// dhsReleaseAuthority is the identity the U.S. release authority wears when it
// authorizes a cross-domain release out-of-band (the L3 principal).
var dhsReleaseAuthority = clientpkg.Persona{
	ID:        "dhs-release-authority",
	UserAgent: "g8e-dhs-authority/1.x (L3 notary)",
}

// DHSSovereignArgs holds the configurable parameters for the DHS scenarios.
// Defaults target the Sovereign Data Service endpoint on net_secure in the
// dhs demo compose topology; a different topology can override the endpoint.
var DHSSovereignArgs = struct {
	DataSvcEndpoint string
}{
	DataSvcEndpoint: "10.63.0.50:9100",
}

// dataopArgs builds the run_shell_command arguments_json that drives the
// Sovereign Data Service via the `dataop` wrapper (the bridge that lets the
// operator's governed execution reach the actuator without tripping the
// curl/wget denylist — exactly the DoW slew.sh pattern).
func dataopArgs(op, recordID, detail string) string {
	return shellCommandArgs("dataop", op, DHSSovereignArgs.DataSvcEndpoint, recordID, detail)
}

func dhsScenarios() []Scenario {
	return []Scenario{
		{
			Name: "dhs-ingest", Title: "DHS: governed multi-source ingest into the sovereign data plane", Persona: dhsConnector, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("connector submits ingest of TRK-CBP-0001 (CUI//LES, NIPR) — L2 consensus + L3 notary")

				txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     dataopArgs("ingest", "TRK-CBP-0001", "NIPR"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal, // inline L3 (mock notary) — admit + execute
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit ingest envelope: %w", err)
				}
				r.tx(txHash)
				r.note("ingest envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("ingest envelope rejected (status %d): %s", status, string(body))
				}
				if summary, failed := receiptFailed(body); failed {
					return fmt.Errorf("ingest tool execution failed: %s", summary)
				}
				r.note("admitted — operator executing dataop ingest via L5 actuator; provenance receipt written to ledger")
				return nil
			},
		},
		{
			Name: "dhs-release", Title: "DHS: cross-domain release gated on out-of-band release-authority approval (L3)", Persona: dhsReleaseAuthority, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("connector requests cross-domain release of TRK-MIL-0007 to the Mission Partner COP")

				m := clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     dataopArgs("release", "TRK-MIL-0007", "MISSION_PARTNER_COP"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				}

				switch kit.L3Mode {
				case "mock":
					r.note("L3 mode=mock: release authority %q signs transaction_hash inline", kit.Principal.KeyID)
					m.Principal = kit.Principal
					txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, m)
					if err != nil {
						return fmt.Errorf("submit release envelope: %w", err)
					}
					r.tx(txHash)
					r.note("release envelope %s submitted with inline principal proof (status %d)", short(txHash), status)
					if status >= 400 {
						return fmt.Errorf("release envelope rejected (status %d): %s", status, string(body))
					}
					if summary, failed := receiptFailed(body); failed {
						return fmt.Errorf("release tool execution failed: %s", summary)
					}
					r.note("release authority %q approved inline (mock L3); release executed", kit.Principal.KeyID)
					return nil

				default:
					r.note("L3 mode=suspend: submit L2-only; data stays under U.S. authority until the release authority signs")
					txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, m)
					if err != nil {
						return fmt.Errorf("submit release envelope: %w", err)
					}
					r.tx(txHash)
					r.note("release envelope %s submitted (status %d); awaiting release-authority approval", short(txHash), status)

					suspendedHash, ok := suspendedFromBody(body)
					if !ok {
						if status >= 400 {
							return fmt.Errorf("release envelope rejected (status %d): %s", status, string(body))
						}
						return fmt.Errorf("release envelope was not suspended (status %d): expected suspension pending L3 notary approval", status)
					}
					txHash = suspendedHash
					ast, approveBody, aerr := c.Approve(ctx, dhsReleaseAuthority, txHash)
					if aerr != nil {
						return fmt.Errorf("release authority approve: %w", aerr)
					}
					if ast >= 400 {
						return fmt.Errorf("release approval rejected (status %d)", ast)
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("release tool execution failed: %s", summary)
					}
					r.note("release authority %q approved hash %s out-of-band (status %d)", kit.Principal.KeyID, short(txHash), ast)
					r.note("cryptographic proof: principal Ed25519 signature over the exact transaction hash — release now executes")
					return nil
				}
			},
		},
		{
			Name: "dhs-cue", Title: "DHS: authorized interdiction cue admitted under L2 consensus quorum", Persona: dhsConnector, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("predictive-analytics agent cues interdiction on TRK-CBP-0001 with signed tasking — quorum %d", kit.Ensemble.AgentCount())

				txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     dataopArgs("cue", "TRK-CBP-0001", "TASKING-DHS-2026-077"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit cue envelope: %w", err)
				}
				r.tx(txHash)
				r.note("cue envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("authorized cue rejected (status %d): %s", status, string(body))
				}
				if summary, failed := receiptFailed(body); failed {
					return fmt.Errorf("cue tool execution failed: %s", summary)
				}
				r.note("admitted — ensemble quorum reached; cue executed and recorded with full data lineage")
				return nil
			},
		},
		{
			Name: "dhs-cue-veto", Title: "DHS: interdiction cue without quorum is vetoed by L2 consensus", Persona: dhsConnector, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("submitting cue with L2 decision=false (no consensus — unauthorized targeting)")

				veto := false
				txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     dataopArgs("cue", "TRK-CBP-0001", "UNAUTHORIZED"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Decision:          &veto,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit veto cue: %w", err)
				}
				r.tx(txHash)
				r.note("veto cue %s submitted (status %d)", short(txHash), status)
				if status < 400 {
					return fmt.Errorf("unauthorized cue was accepted (status %d) — expected L2 rejection", status)
				}
				r.note("correctly rejected by L2 consensus — operator fails closed, no interdiction cue executed")
				r.note("response: %s", string(body))
				return nil
			},
		},
		{
			Name: "dhs-evidence-block", Title: "DHS: attempt to wipe the audit trail is rejected by L1 doctrine", Persona: dhsConnector, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("compromised connector tries to destroy the audit trail: run_shell_command 'rm -rf /var/log/g8e'")

				// Even with valid L2 + L3 proofs attached, L1 doctrine is the hard
				// gate and runs first — the audit-wipe command is blocked at admission.
				txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     shellCommandArgs("rm -rf /var/log/g8e"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit evidence-wipe envelope: %w", err)
				}
				r.tx(txHash)
				r.note("evidence-wipe envelope %s submitted (status %d)", short(txHash), status)
				if status < 400 {
					return fmt.Errorf("audit-wipe was accepted (status %d) — expected L1 rejection", status)
				}
				r.note("correctly rejected by L1 doctrine (data-destruction threat) — audit trail is tamper-evident")
				r.note("response: %s", string(body))
				return nil
			},
		},
		{
			Name: "dhs-purge", Title: "DHS: governed retention destruction with cryptographic receipt", Persona: dhsConnector, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return fmt.Errorf("state root: %w", err)
				}
				r.note("bound to state root %s", short(root))
				r.note("U.S.-person record VPR-0001 hit its 30-day retention limit — governed purge under L2+L3")

				txHash, status, body, err := c.SubmitMaximal(ctx, dhsConnector, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "run_shell_command",
					ArgumentsJSON:     dataopArgs("purge", "VPR-0001", "RETENTION-30D"),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					Principal:         kit.Principal,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return fmt.Errorf("submit purge envelope: %w", err)
				}
				r.tx(txHash)
				r.note("purge envelope %s submitted (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("governed purge rejected (status %d): %s", status, string(body))
				}
				if summary, failed := receiptFailed(body); failed {
					return fmt.Errorf("purge tool execution failed: %s", summary)
				}
				r.note("admitted — operator executed dataop purge; cryptographic destruction receipt written to ledger")
				return nil
			},
		},
	}
}
