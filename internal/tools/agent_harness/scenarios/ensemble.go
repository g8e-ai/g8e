// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// The ensemble (g8ee) scenarios exercise the core product path: a user sends
// a chat message to the ensemble, the AI reasons about the request and submits
// governed tool calls (file_create, file_write, document_update, etc.) through
// the operator to the gateway for admission. The gateway runs the 5-layer
// governance gauntlet (L1 doctrine, L2 consensus, L3 notary, L4 warden, L5
// actuator), and the operator executes the admitted command. The scenarios
// verify the end-to-end flow by polling the audit vault for signed
// ActionReceipts matching the expected action type.
//
// The ensemble chat endpoint is non-streaming: it creates case/investigation
// inline (when resource_creation.create_case is set), fires run_chat as a
// background task, and returns immediately with the case/investigation IDs.
// The AI response and tool events are delivered via SSE. The harness scenarios
// poll for side effects (audit vault receipts) rather than waiting for a
// streaming response.
//
// LLM provider selection: scenarios default to the "fake" LLM provider for CI
// determinism (no external LLM dependency). Override via env vars for local
// dev with a real LLM:
//
//	G8E_HARNESS_LLM_PROVIDER=ollama
//	G8E_HARNESS_LLM_MODEL=gemma4:12b
//	G8E_HARNESS_LLM_ENDPOINT=http://192.168.1.2:11434

// ensemblePollTimeout is the maximum time to wait for an audit receipt to
// appear after sending a chat request. The LLM + governance round-trip can
// take 30-60s with ollama; the fake provider is near-instant.
const ensemblePollTimeout = 90 * time.Second

// ensemblePollInterval is the delay between audit vault polls.
const ensemblePollInterval = 3 * time.Second

// ensembleSmokeFile is the path the file-create scenario instructs the AI to
// create. The operator executes the governed file_create tool call against
// this path.
const ensembleSmokeFile = "/tmp/g8e-ensemble-smoke-test.txt"

// ensembleSmokeContent is the content the file-write scenario instructs the
// AI to write.
const ensembleSmokeContent = "g8e ensemble governed file write smoke test"

// ensembleLLMOverrides reads LLM provider config from env vars and returns
// the override fields for the EnsembleChatRequest. Defaults to the "fake"
// provider for CI determinism. Set G8E_HARNESS_LLM_PROVIDER to override
// (e.g., "ollama" for local dev with a real LLM).
//
// When the provider is "fake" and no model is explicitly set, the model
// defaults to "fake" — the ensemble's validate_llm_config requires at least
// one of primary/assistant/lite model to be non-empty, and the FakeProvider
// accepts any model string (it is passed through to the response but not
// used for routing). Without a model default, the ensemble rejects the
// request with "No LLM model configured" even though the provider override
// is present.
func ensembleLLMOverrides() (provider, model, endpoint string) {
	provider = os.Getenv("G8E_HARNESS_LLM_PROVIDER")
	if provider == "" {
		provider = "fake"
	}
	model = os.Getenv("G8E_HARNESS_LLM_MODEL")
	endpoint = os.Getenv("G8E_HARNESS_LLM_ENDPOINT")
	if model == "" && provider == "fake" {
		model = "fake"
	}
	return
}

// ensembleChatRequest builds a typed EnsembleChatRequest with the GovKit
// identity, LLM overrides, and resource creation flags. The persona must
// already have UserID and CLISessionID stamped via withCLIIdentity.
func ensembleChatRequest(persona clientpkg.Persona, message string) clientpkg.EnsembleChatRequest {
	provider, model, endpoint := ensembleLLMOverrides()
	ctx := clientpkg.EnsembleRequestContext{
		CLISessionID:    persona.CLISessionID,
		UserID:          persona.UserID,
		SourceComponent: "CLIENT",
	}
	if kit != nil && kit.OperatorID != "" && kit.OperatorSessionID != "" {
		ctx.BoundOperators = []clientpkg.EnsembleBoundOperator{
			{
				OperatorID:        kit.OperatorID,
				OperatorSessionID: kit.OperatorSessionID,
				Status:            string(constants.OperatorStatusBound),
			},
		}
	}
	return clientpkg.EnsembleChatRequest{
		Context:            ctx,
		Message:            message,
		SentinelMode:       true,
		ResourceCreation:   &clientpkg.EnsembleResourceCreation{CreateCase: true, CaseTitle: "Ensemble harness smoke test"},
		LLMPrimaryProvider: provider,
		LLMPrimaryModel:    model,
		LLMPrimaryEndpoint: endpoint,
	}
}

// pollForReceipt polls the audit vault for a receipt matching the expected
// action type with a COMPLETED status. Returns the matching receipt, or an
// error if the timeout expires. If a matching receipt is found but its status
// is FAILED, returns ErrHarnessEnsembleReceiptFailed with the result summary.
func pollForReceipt(ctx context.Context, c *clientpkg.Client, r *Result, opSessionID, actionType string) (*clientpkg.Receipt, error) {
	deadline := time.Now().Add(ensemblePollTimeout)
	ticker := time.NewTicker(ensemblePollInterval)
	defer ticker.Stop()

	r.note("polling audit vault for %s receipt (timeout %s)", actionType, ensemblePollTimeout)

	for {
		receipts, _, err := c.AuditReceipts(ctx, opSessionID)
		if err != nil {
			r.note("audit receipts query error (will retry): %v", err)
		} else {
			for i := range receipts {
				rec := &receipts[i]
				if rec.ActionType != actionType {
					continue
				}
				status, summary := receiptStatusSummary(rec.Raw)
				if status == int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED) {
					r.note("found %s receipt: tx=%s status=COMPLETED", actionType, short(rec.TransactionID))
					return rec, nil
				}
				if status == int(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
					return nil, fmt.Errorf("%w: %s", constants.ErrHarnessEnsembleReceiptFailed, summary)
				}
				r.note("found %s receipt: tx=%s status=%d (waiting for completion)", actionType, short(rec.TransactionID), status)
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: action_type=%s", constants.ErrHarnessEnsembleReceiptTimeout, actionType)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// receiptStatusSummary extracts the numeric execution status and result
// summary from a raw receipt JSON body. The ActionReceiptRecord.Status field
// is operatorv1.ExecutionStatus (int32), serialized as a JSON number by the
// gateway's audit controller. Returns (0, "") if the body is not a parseable
// receipt.
func receiptStatusSummary(raw json.RawMessage) (int, string) {
	if len(raw) == 0 {
		return 0, ""
	}
	var r struct {
		Status        int    `json:"status"`
		ResultSummary string `json:"result_summary"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, ""
	}
	return r.Status, r.ResultSummary
}

// ensembleChat sends a chat request and verifies the ensemble accepted it
// (success=true). Returns the chat response with case_id/investigation_id.
// Fails closed if the chat request itself fails or returns success=false.
func ensembleChat(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, message string) (*clientpkg.EnsembleChatResponse, error) {
	req := ensembleChatRequest(persona, message)
	r.note("sending chat to ensemble: %q", truncateMessage(message))

	resp, err := c.EnsembleChat(ctx, persona, req)
	if err != nil {
		return nil, fmt.Errorf("ensemble chat: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("%w: case_id=%s investigation_id=%s", constants.ErrHarnessEnsembleChatFailed, resp.CaseID, resp.InvestigationID)
	}
	r.note("ensemble accepted chat: case_id=%s investigation_id=%s", short(resp.CaseID), short(resp.InvestigationID))
	return resp, nil
}

func truncateMessage(msg string) string {
	const max = 80
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "..."
}

func ensembleScenarios() []Scenario {
	return []Scenario{
		{
			Name: "ensemble-chat-file-create", Title: "Ensemble chat: AI creates a governed file via file_create tool", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				message := fmt.Sprintf("Create a new file at %s with the content: %s", ensembleSmokeFile, ensembleSmokeContent)

				if _, err := ensembleChat(ctx, c, r, persona, message); err != nil {
					return err
				}
				if _, err := pollForReceipt(ctx, c, r, kit.OperatorSessionID, string(constants.ActionTypeFileEdit)); err != nil {
					return err
				}
				r.note("governed file_create tool call admitted and executed through L1-L5 gauntlet")
				return nil
			},
		},
		{
			Name: "ensemble-chat-file-write", Title: "Ensemble chat: AI writes content to an existing file via file_write tool", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				message := fmt.Sprintf("Write the following content to the file at %s: %s", ensembleSmokeFile, ensembleSmokeContent)

				if _, err := ensembleChat(ctx, c, r, persona, message); err != nil {
					return err
				}
				if _, err := pollForReceipt(ctx, c, r, kit.OperatorSessionID, string(constants.ActionTypeFileEdit)); err != nil {
					return err
				}
				r.note("governed file_write tool call admitted and executed through L1-L5 gauntlet")
				return nil
			},
		},
		{
			Name: "ensemble-document-update", Title: "Ensemble chat: governed document update (case/investigation create) via DOCUMENT_UPDATE", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				message := "Create a new investigation note documenting this smoke test run."

				chatResp, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				// Case creation itself triggers a DOCUMENT_UPDATE envelope
				// (EventAppCaseCreated -> ActionTypeDocumentUpdate). Verify it
				// was admitted by L1 and the document store handler
				// (handleDocumentUpdateSync) persisted it.
				if chatResp.CaseID == "" {
					return fmt.Errorf("ensemble document update: case_id is empty — case creation did not produce a DOCUMENT_UPDATE")
				}
				if _, err := pollForReceipt(ctx, c, r, kit.OperatorSessionID, string(constants.ActionTypeDocumentUpdate)); err != nil {
					return err
				}
				r.note("DOCUMENT_UPDATE envelope admitted by L1; case %s persisted via handleDocumentUpdateSync", short(chatResp.CaseID))
				return nil
			},
		},
		{
			Name: "ensemble-document-delete", Title: "Ensemble chat: governed document delete via DOCUMENT_DELETE", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				message := "Delete the investigation note created in the previous smoke test run."

				if _, err := ensembleChat(ctx, c, r, persona, message); err != nil {
					return err
				}
				if _, err := pollForReceipt(ctx, c, r, kit.OperatorSessionID, string(constants.ActionTypeDocumentDelete)); err != nil {
					return err
				}
				r.note("DOCUMENT_DELETE envelope admitted by L1; document removed via handleDocumentDeleteSync")
				return nil
			},
		},
	}
}
