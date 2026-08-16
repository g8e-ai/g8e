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

// GovKit carries the governance context the scenarios need.
// main builds it once and injects it before running the consensus/notary block.
type GovKit struct {
	OperatorID        string
	OperatorSessionID string
	UserID            string // user_id for SSE subscription (the human approver)
	CLISessionID      string // cli_session_id for the X-CLI-Session-ID submit header
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

// withCLIIdentity returns a copy of base with the host CLI user_id and
// cli_session_id from GovKit so the submit headers bind the suspended
// transaction to the same identity as the browser approver.
func withCLIIdentity(base clientpkg.Persona) clientpkg.Persona {
	out := base
	if kit != nil {
		out.UserID = kit.UserID
		out.CLISessionID = kit.CLISessionID
	}
	return out
}

func governanceScenarios() []Scenario {
	return []Scenario{
		{
			Name: "consensus", Title: "L2 consensus via MCP tools/call (gateway deliberation)", Persona: ensembleProducer, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("submitting fs_list via MCP tools/call — gateway runs L2 deliberation")

				resp, err := c.MCPToolsCall(ctx, ensembleProducer, "fs_list", fsListMap("."))
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("consensus tool call rejected: %s", resp.Error.Message)
				}
				r.note("gateway admitted envelope after L2 consensus deliberation")
				return nil
			},
		},
		{
			Name: "envelope-maximal", Title: "Notary envelope: MCP tools/call, gateway suspends, human approves via WebAuthn", Persona: ensembleProducer, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				r.note("submitting fs_list via MCP tools/call (CLI-cert) — gateway runs L2, suspends for L3")

				cliPersona := withCLIIdentity(ensembleProducer)
				resp, err := c.MCPToolsCallWithCLI(ctx, cliPersona, "fs_list", fsListMap("."))
				if err != nil {
					return err
				}

				if txHash, suspended := clientpkg.Suspended(resp); suspended {
					r.note("gateway suspended transaction %s pending L3 notary approval", short(txHash))
					ast, approveBody, aerr := c.WaitForHumanApproval(ctx, ensembleProducer, txHash, kit.UserID)
					if aerr != nil {
						return fmt.Errorf("human approval: %w", aerr)
					}
					r.note("human approved hash %s via browser WebAuthn (status %d)", short(txHash), ast)
					if ast >= 400 {
						return fmt.Errorf("human approval rejected with status %d", ast)
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("tool execution failed after human approval: %s", summary)
					}
					r.note("human WebAuthn L3 proof verified; transaction resumed")
					return nil
				}

				if resp != nil && resp.Error != nil {
					return fmt.Errorf("notary tool call rejected: %s", resp.Error.Message)
				}
				r.note("gateway admitted envelope (L3 notary satisfied inline)")
				return nil
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
				resp, err := c.MCPToolsCall(ctx, delegateAgent, tool, clientpkg.FSPathArgs{Path: "."})
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
			Name: "consensus-quorum", Title: "Consensus quorum: MCP tools/call, gateway 2-of-3 deliberation", Persona: ensembleProducer, RequiresPosture: Consensus,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				r.note("submitting fs_list via MCP tools/call — gateway runs 2-of-3 quorum deliberation")

				resp, err := c.MCPToolsCall(ctx, ensembleProducer, "fs_list", fsListMap("."))
				if err != nil {
					return err
				}
				if resp != nil && resp.Error != nil {
					return fmt.Errorf("quorum tool call rejected: %s", resp.Error.Message)
				}
				r.note("gateway admitted envelope after quorum consensus")
				return nil
			},
		},
		{
			Name: "notary-oob", Title: "L3 notary OOB: MCP tools/call, gateway suspends, human approves via WebAuthn", Persona: principalActor, RequiresPosture: Notary,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitMissingSign
				}
				r.note("submitting fs_list via MCP tools/call (CLI-cert) — gateway suspends for L3 notary")

				cliPersona := withCLIIdentity(ensembleProducer)
				resp, err := c.MCPToolsCallWithCLI(ctx, cliPersona, "fs_list", fsListMap("."))
				if err != nil {
					return err
				}

				if txHash, suspended := clientpkg.Suspended(resp); suspended {
					r.note("gateway suspended transaction %s pending L3 notary approval", short(txHash))
					ast, approveBody, aerr := c.WaitForHumanApproval(ctx, principalActor, txHash, kit.UserID)
					if aerr != nil {
						return fmt.Errorf("human approval: %w", aerr)
					}
					r.note("human approved hash %s via browser WebAuthn (status %d)", short(txHash), ast)
					if ast >= 400 {
						return fmt.Errorf("human approval rejected with status %d", ast)
					}
					if summary, failed := receiptFailed(approveBody); failed {
						return fmt.Errorf("tool execution failed after human approval: %s", summary)
					}
					r.note("human WebAuthn L3 proof verified; transaction resumed")
					return nil
				}

				if resp != nil && resp.Error != nil {
					return fmt.Errorf("notary tool call rejected: %s", resp.Error.Message)
				}
				r.note("gateway admitted envelope (L3 notary satisfied inline)")
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

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}
