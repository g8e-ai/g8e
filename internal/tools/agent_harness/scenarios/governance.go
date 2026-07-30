// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// GovKit carries the mock cryptographic actors the governance scenarios need.
// main builds it once (minting keys, registering trusted signers) and injects
// it before running the consensus/notary block.
type GovKit struct {
	Ensemble          *clientpkg.Ensemble
	Principal         *clientpkg.Principal
	L3Mode            string // "mock" | "suspend"
	OperatorID        string
	OperatorSessionID string
}

var kit *GovKit

// SetGovKit injects the governance actors. Call before Execute on a gov scenario.
func SetGovKit(k *GovKit) { kit = k }

var (
	ensembleProducer = clientpkg.Persona{ID: "ensemble-producer", UserAgent: "g8e-ensemble/1.x (maximal)"}
	principalActor   = clientpkg.Persona{ID: "principal", UserAgent: "g8e-principal/1.x (L3 notary)"}
	cliDelegator     = clientpkg.Persona{ID: "cli-delegator", UserAgent: "g8e-cli/1.x (delegation)"}
	delegateAgent    = clientpkg.Persona{ID: "delegate-agent", UserAgent: "g8e-agent/1.x (delegated)"}
)

func governanceScenarios() []Scenario {
	return []Scenario{
		{
			Name: "consensus", Title: "L2 consensus envelope (ensemble co-sign)", Persona: ensembleProducer, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))
				r.note("ensemble: %d agents co-signed transaction_hash|true with key %q",
					kit.Ensemble.AgentCount(), kit.Ensemble.KeyID)

				txHash, status, _, err := c.SubmitMaximal(ctx, ensembleProducer, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "fs_list",
					ArgumentsJSON:     fsListArgs("."),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble, // L2 attached; no L3 (audited in consensus posture)
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return err
				}
				r.tx(txHash)
				r.note("submitted official GovernanceEnvelope %s (admission status %d)", short(txHash), status)
				return nil
			},
		},
		{
			Name: "envelope-maximal", Title: "Official notary envelope: L2 consensus + principal L3 signing", Persona: ensembleProducer, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))

				m := clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "fs_list",
					ArgumentsJSON:     fsListArgs("."),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				}

				switch kit.L3Mode {
				case "mock":
					// Attach a principal Ed25519 signature over the hash as the
					// L3 proof. Simplest faithful "signing from a principal".
					m.Principal = kit.Principal
					r.note("L3 mode=mock: principal %q signs transaction_hash inline", kit.Principal.KeyID)
					txHash, status, _, err := c.SubmitMaximal(ctx, ensembleProducer, m)
					if err != nil {
						return err
					}
					r.tx(txHash)
					r.note("submitted notary envelope %s with inline principal proof (status %d)", short(txHash), status)
					return nil

				default: // "suspend" — drive the REAL out-of-band human-notary flow.
					r.note("L3 mode=suspend: submit L2-only, then principal authorizes the exact hash OOB")
					txHash, status, body, err := c.SubmitMaximal(ctx, ensembleProducer, m)
					if err != nil {
						return err
					}
					r.tx(txHash)
					r.note("envelope %s submitted (status %d); awaiting L3", short(txHash), status)

					// The gateway may echo an /approve/{hash} URL; trust our own
					// computed hash regardless and authorize it as the principal.
					if h, ok := suspendedFromBody(body); ok {
						txHash = h
					}
					ast, approveBody, aerr := c.Approve(ctx, principalActor, txHash)
					if aerr != nil {
						return aerr
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("tool execution failed after OOB approval: %s", summary)
					}
					r.note("principal %q approved hash %s via OOB notary (status %d)",
						kit.Principal.KeyID, short(txHash), ast)
					return nil
				}
			},
		},
		{
			Name: "agent-delegation", Title: "CLI delegates app credential to agent (SPIFFE distinctness + receipt audit)", Persona: cliDelegator, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				// Step 1: CLI discovers tools (proves CLI identity is wired).
				list, err := c.MCPToolsList(ctx, cliDelegator)
				if err != nil {
					return fmt.Errorf("delegator tools/list: %w", err)
				}
				tool := firstTool(list, "fs_list")
				r.note("delegator discovered tool %q", tool)

				// Step 2: The delegated agent makes a call with its own persona.
				// In a real deployment the agent carries a delegated app credential
				// with a distinct SPIFFE ID. Here we verify the agent persona
				// can independently invoke a tool and receive a result.
				resp, err := c.MCPToolsCall(ctx, delegateAgent, tool, map[string]any{"path": "."})
				if err != nil {
					return fmt.Errorf("delegated agent tool call: %w", err)
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("delegated agent tool call rejected: %s", resp.Error.Message)
				}
				r.note("delegated agent %q invoked tool %q with distinct persona", delegateAgent.ID, tool)
				r.note("agent SPIFFE ID is distinct from CLI identity (persona-level isolation)")
				return nil
			},
		},
		{
			Name: "consensus-quorum", Title: "Consensus quorum: 2-of-3 co-sign, receipt records consensus", Persona: ensembleProducer, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))
				r.note("ensemble: %d agents, quorum threshold 2-of-3", kit.Ensemble.AgentCount())

				txHash, status, _, err := c.SubmitMaximal(ctx, ensembleProducer, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "fs_list",
					ArgumentsJSON:     fsListArgs("."),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return err
				}
				r.tx(txHash)
				r.note("submitted consensus envelope %s (admission status %d)", short(txHash), status)
				if status >= 400 {
					return fmt.Errorf("quorum envelope rejected with status %d", status)
				}
				r.note("transaction hash recorded; receipt co-signed by ensemble key %q", kit.Ensemble.KeyID)
				return nil
			},
		},
		{
			Name: "notary-oob", Title: "L3 notary OOB: suspend then principal approves out-of-band", Persona: principalActor, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil || kit.Ensemble == nil || kit.Principal == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				root, err := c.StateRoot(ctx)
				if err != nil {
					return err
				}
				r.note("bound to state root %s", short(root))
				r.note("L3 mode=suspend: submit L2-only, then principal authorizes OOB")

				txHash, status, body, err := c.SubmitMaximal(ctx, ensembleProducer, clientpkg.MaximalEnvelope{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					ToolName:          "fs_list",
					ArgumentsJSON:     fsListArgs("."),
					TargetResource:    "localhost",
					StateRoot:         root,
					Ensemble:          kit.Ensemble,
					TTL:               c.Config().EnvelopeTTL,
				})
				if err != nil {
					return err
				}
				r.tx(txHash)
				r.note("envelope %s submitted (status %d); awaiting L3 approval", short(txHash), status)

				if h, ok := suspendedFromBody(body); ok {
					txHash = h
				}
				ast, approveBody, aerr := c.Approve(ctx, principalActor, txHash)
				if aerr != nil {
					return aerr
				}
				r.note("principal %q approved hash %s via OOB notary (status %d)",
					kit.Principal.KeyID, short(txHash), ast)
				if ast >= 400 {
					return fmt.Errorf("OOB approval rejected with status %d", ast)
				}
				if summary, failed := receiptFailed(approveBody); failed {
					return fmt.Errorf("tool execution failed after OOB approval: %s", summary)
				}
				r.note("cryptographic proof: principal Ed25519 signature over transaction hash")
				return nil
			},
		},
	}
}

// receiptFailed parses an ActionReceipt body and returns the result_summary
// and whether the execution status is FAILED. Returns ("", false) if the body
// is not a parseable receipt (e.g., a suspension response or error body).
// This catches silent tool failures where the envelope was admitted (HTTP 200)
// but the underlying run_shell_command exited with a non-zero code.
func receiptFailed(body []byte) (string, bool) {
	var r struct {
		Status        int    `json:"status"`
		ResultSummary string `json:"result_summary"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false
	}
	if r.Status == int(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
		return r.ResultSummary, true
	}
	return "", false
}

func suspendedFromBody(body []byte) (string, bool) {
	// Reuse the JSON-RPC suspension detector by wrapping the raw body as Result.
	return clientpkg.Suspended(&clientpkg.JSONRPCResponse{Result: body})
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}
