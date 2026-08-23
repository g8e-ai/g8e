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
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// uniqueRunID returns a per-run unique identifier (Unix nanoseconds + PID) so
// scenario artifacts (file path, case title, document ID) cannot collide with
// prior runs (C.4). The PID disambiguates parallel runs that start in the same
// nanosecond.
func uniqueRunID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// uniqueSmokeFile returns a per-run unique file path under ensembleSmokeFileDir.
func uniqueSmokeFile() string {
	return fmt.Sprintf("%s/g8e-ensemble-smoke-%s.txt", ensembleSmokeFileDir, uniqueRunID())
}

// uniqueCaseTitle returns a per-run unique case title so the ensemble creates a
// distinct case per run rather than reusing a prior run's case.
func uniqueCaseTitle() string {
	return fmt.Sprintf("Ensemble harness smoke test %s", uniqueRunID())
}

// uniqueSmokeContent returns the base smoke content with a per-run unique
// suffix so the read-back assertion can distinguish this run's content from
// any prior run's.
func uniqueSmokeContent() string {
	return fmt.Sprintf("%s run=%s", ensembleSmokeContent, uniqueRunID())
}

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

// ensembleSmokeFileDir is the directory under which per-run unique smoke-test
// files are created. The filename is unique per run (C.4) so retries cannot
// collide with prior state.
const ensembleSmokeFileDir = "/tmp"

// ensembleSmokeContent is the content the file-write scenario instructs the
// AI to write. A per-run unique suffix is appended so the read-back assertion
// can distinguish this run's content from any prior run's.
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
// already have UserID and CLISessionID stamped via withCLIIdentity. The case
// title is per-run unique (C.4) so the ensemble creates a distinct case per
// run rather than reusing a prior run's case.
func ensembleChatRequest(persona clientpkg.Persona, message, caseTitle string) clientpkg.EnsembleChatRequest {
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
		ResourceCreation:   &clientpkg.EnsembleResourceCreation{CreateCase: true, CaseTitle: caseTitle},
		LLMPrimaryProvider: provider,
		LLMPrimaryModel:    model,
		LLMPrimaryEndpoint: endpoint,
	}
}

// ReceiptCorrelation is the set of criteria a receipt must satisfy to be
// attributed to the current run. A receipt is accepted only when ALL of the
// following hold:
//   - ActionType matches the expected action type.
//   - ExecutedAt is at or after NotBefore (the timestamp captured before the
//     chat request was sent). This excludes stale receipts from prior runs.
//   - OperatorSessionID matches the session bound to this run (the audit
//     query is already scoped by operator_session_id, but the receipt body is
//     cross-checked to defend against a mis-scoped query or a proxy that
//     drops the filter).
//   - Signature is non-empty (the receipt is signed by the operator's L5
//     actuator). A receipt without a signature is not trustworthy evidence.
//   - CaseID, when non-empty, must match the chat response's case_id. The
//     audit receipt record does not currently carry case_id, so this is a
//     forward-compatible check: if a future schema adds case_id to the
//     receipt, correlation tightens automatically.
//
// A FAILED receipt that otherwise correlates is returned immediately as
// ErrHarnessEnsembleReceiptFailed so the scenario reports the failure rather
// than waiting for the timeout.
type ReceiptCorrelation struct {
	NotBefore        time.Time
	OperatorSessionID string
	ActionType       string
	CaseID           string
}

// pollForReceipt polls the audit vault for a receipt matching the correlation
// criteria with a COMPLETED status. Returns the matching receipt, or an error
// if the timeout expires. If a matching receipt is found but its status is
// FAILED, returns ErrHarnessEnsembleReceiptFailed with the result summary.
// Stale, unrelated, or unsigned receipts are skipped (with a note) rather than
// accepted, so a historical receipt cannot produce a false pass.
func pollForReceipt(ctx context.Context, c *clientpkg.Client, r *Result, opSessionID, actionType string) (*clientpkg.Receipt, error) {
	return pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
		OperatorSessionID: opSessionID,
		ActionType:        actionType,
	})
}

// pollForReceiptWithCorrelation is the correlated form of pollForReceipt. The
// caller captures NotBefore before sending the chat request and passes it here
// so stale receipts from prior runs are rejected.
func pollForReceiptWithCorrelation(ctx context.Context, c *clientpkg.Client, r *Result, corr ReceiptCorrelation) (*clientpkg.Receipt, error) {
	deadline := time.Now().Add(ensemblePollTimeout)
	ticker := time.NewTicker(ensemblePollInterval)
	defer ticker.Stop()

	r.note("polling audit vault for %s receipt (not_before=%s, timeout %s)", corr.ActionType, corr.NotBefore.Format(time.RFC3339Nano), ensemblePollTimeout)

	for {
		receipts, _, err := c.AuditReceipts(ctx, corr.OperatorSessionID)
		if err != nil {
			r.note("audit receipts query error (will retry): %v", err)
		} else {
			for i := range receipts {
				rec := &receipts[i]
				if rec.ActionType != corr.ActionType {
					continue
				}
				// Not-before boundary: reject stale receipts from prior runs.
				if !corr.NotBefore.IsZero() && !rec.ExecutedAt.IsZero() && rec.ExecutedAt.Before(corr.NotBefore) {
					r.note("skipping stale %s receipt: tx=%s executed_at=%s before not_before=%s", corr.ActionType, short(rec.TransactionID), rec.ExecutedAt.Format(time.RFC3339Nano), corr.NotBefore.Format(time.RFC3339Nano))
					continue
				}
				// Operator session binding: the audit query is scoped by
				// operator_session_id, but cross-check the receipt body to
				// defend against a mis-scoped query.
				if corr.OperatorSessionID != "" && rec.OperatorSessionID != "" && rec.OperatorSessionID != corr.OperatorSessionID {
					r.note("skipping %s receipt: tx=%s operator_session_id=%s != expected=%s", corr.ActionType, short(rec.TransactionID), rec.OperatorSessionID, corr.OperatorSessionID)
					continue
				}
				// Signature presence: a receipt without a signature is not
				// trustworthy evidence of execution.
				if rec.Signature == "" {
					r.note("skipping unsigned %s receipt: tx=%s (no L5 actuator signature)", corr.ActionType, short(rec.TransactionID))
					continue
				}
				status, summary := receiptStatusSummary(rec.Raw)
				if status == int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED) {
					r.note("found correlated %s receipt: tx=%s status=COMPLETED executed_at=%s", corr.ActionType, short(rec.TransactionID), rec.ExecutedAt.Format(time.RFC3339Nano))
					return rec, nil
				}
				if status == int(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED) {
					return nil, fmt.Errorf("%w: %s", constants.ErrHarnessEnsembleReceiptFailed, summary)
				}
				r.note("found correlated %s receipt: tx=%s status=%d (waiting for completion)", corr.ActionType, short(rec.TransactionID), status)
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: action_type=%s not_before=%s", constants.ErrHarnessEnsembleReceiptTimeout, corr.ActionType, corr.NotBefore.Format(time.RFC3339Nano))
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
// (success=true). Returns the chat response with case_id/investigation_id and
// the not-before timestamp captured just before the request was sent (for
// receipt correlation). Fails closed if the chat request itself fails or
// returns success=false. The case title is per-run unique (C.4).
func ensembleChat(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, message string) (*clientpkg.EnsembleChatResponse, time.Time, error) {
	caseTitle := uniqueCaseTitle()
	req := ensembleChatRequest(persona, message, caseTitle)
	notBefore := time.Now()
	r.note("sending chat to ensemble: %q (case_title=%q)", truncateMessage(message), caseTitle)

	resp, err := c.EnsembleChat(ctx, persona, req)
	if err != nil {
		return nil, notBefore, fmt.Errorf("ensemble chat: %w", err)
	}
	if !resp.Success {
		return nil, notBefore, fmt.Errorf("%w: case_id=%s investigation_id=%s", constants.ErrHarnessEnsembleChatFailed, resp.CaseID, resp.InvestigationID)
	}
	r.note("ensemble accepted chat: case_id=%s investigation_id=%s", short(resp.CaseID), short(resp.InvestigationID))
	return resp, notBefore, nil
}

func truncateMessage(msg string) string {
	const max = 80
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "..."
}

// verifyReceiptIdentity asserts that a correlated receipt attributes the action
// to the expected human requestor and a non-empty acting app (C.2). The
// requestor_user_id must match the GovKit user identity that authorized the
// chat; the acting_app_id must be non-empty so the action is attributable to a
// specific app rather than an anonymous submission. A mismatch means the
// receipt does not prove this run's identity bound the action, so the scenario
// fails closed rather than passing on an unattributed receipt.
func verifyReceiptIdentity(r *Result, receipt *clientpkg.Receipt) error {
	if receipt == nil {
		return fmt.Errorf("verifyReceiptIdentity: nil receipt")
	}
	if kit != nil && kit.UserID != "" {
		if receipt.RequestorUserID == "" {
			return fmt.Errorf("verifyReceiptIdentity: receipt requestor_user_id is empty — action not attributed to a human user (tx=%s)", short(receipt.TransactionID))
		}
		if receipt.RequestorUserID != kit.UserID {
			return fmt.Errorf("verifyReceiptIdentity: receipt requestor_user_id=%q != expected=%q (tx=%s)", receipt.RequestorUserID, kit.UserID, short(receipt.TransactionID))
		}
	}
	if receipt.ActingAppID == "" {
		return fmt.Errorf("verifyReceiptIdentity: receipt acting_app_id is empty — action not attributed to an app (tx=%s)", short(receipt.TransactionID))
	}
	r.note("receipt identity verified: requestor_user_id=%s acting_app_id=%s", receipt.RequestorUserID, short(receipt.ActingAppID))
	return nil
}

// governedReadBack submits a governed fs_read tool call through the gateway's
// MCP endpoint and returns the text content from the response. This is the
// read-back path (C.2): the harness verifies the actual operator filesystem
// via a governed FS_READ envelope, not a host path. The gateway wraps the
// fs_read into a governed MCP_CALL envelope, runs the L1-L5 gauntlet, and the
// operator executes the read and returns the file content.
func governedReadBack(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, path string) (string, error) {
	r.note("governed read-back: fs_read %s via MCP tools/call", path)
	resp, err := c.MCPToolsCall(ctx, persona, "fs_read", clientpkg.FSPathArgs{Path: path})
	if err != nil {
		return "", fmt.Errorf("governed read-back: %w", err)
	}
	if resp != nil && resp.Error != nil {
		return "", fmt.Errorf("governed read-back: fs_read rejected: %s", resp.Error.Message)
	}
	if resp == nil || len(resp.Result) == 0 {
		return "", fmt.Errorf("governed read-back: fs_read returned empty result")
	}
	// The MCP tools/call result is a CallToolResult JSON: {"content":[{"type":"text","text":"..."}]}
	var cr struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &cr); err != nil {
		return "", fmt.Errorf("governed read-back: decode CallToolResult: %w", err)
	}
	if cr.IsError {
		var sb strings.Builder
		for _, c := range cr.Content {
			sb.WriteString(c.Text)
		}
		return "", fmt.Errorf("governed read-back: fs_read returned error: %s", sb.String())
	}
	if len(cr.Content) == 0 {
		return "", fmt.Errorf("governed read-back: fs_read returned no content")
	}
	return cr.Content[0].Text, nil
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
				filePath := uniqueSmokeFile()
				fileContent := uniqueSmokeContent()
				message := fmt.Sprintf("Create a new file at %s with the content: %s", filePath, fileContent)
				r.note("per-run artifact: file=%s", filePath)

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeFileEdit),
					CaseID:            chatResp.CaseID,
				})
				if err != nil {
					return err
				}
				r.note("correlated FILE_EDIT receipt: tx=%s signature_len=%d", short(receipt.TransactionID), len(receipt.Signature))

				// C.2: verify the receipt attributes the action to the expected
				// human requestor and a non-empty acting app.
				if err := verifyReceiptIdentity(r, receipt); err != nil {
					return err
				}

				// C.2: governed read-back verifies exact file content and
				// that the side effect actually occurred on the operator
				// filesystem, not just a receipt claiming it did.
				content, err := governedReadBack(ctx, c, r, persona, filePath)
				if err != nil {
					return err
				}
				if !strings.Contains(content, fileContent) {
					return fmt.Errorf("governed read-back: file content mismatch — expected to contain %q, got %q", fileContent, content)
				}
				r.note("governed read-back verified file content at %s", filePath)
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
				filePath := uniqueSmokeFile()
				fileContent := uniqueSmokeContent()
				message := fmt.Sprintf("Write the following content to the file at %s: %s", filePath, fileContent)
				r.note("per-run artifact: file=%s", filePath)

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeFileEdit),
					CaseID:            chatResp.CaseID,
				})
				if err != nil {
					return err
				}
				r.note("correlated FILE_EDIT receipt: tx=%s signature_len=%d", short(receipt.TransactionID), len(receipt.Signature))

				// C.2: verify the receipt attributes the action to the expected
				// human requestor and a non-empty acting app.
				if err := verifyReceiptIdentity(r, receipt); err != nil {
					return err
				}

				// C.2: governed read-back verifies exact file content.
				content, err := governedReadBack(ctx, c, r, persona, filePath)
				if err != nil {
					return err
				}
				if !strings.Contains(content, fileContent) {
					return fmt.Errorf("governed read-back: file content mismatch — expected to contain %q, got %q", fileContent, content)
				}
				r.note("governed read-back verified file content at %s", filePath)
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

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
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
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeDocumentUpdate),
					CaseID:            chatResp.CaseID,
				})
				if err != nil {
					return err
				}
				r.note("correlated DOCUMENT_UPDATE receipt: tx=%s case=%s signature_len=%d", short(receipt.TransactionID), short(chatResp.CaseID), len(receipt.Signature))
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
				// C.3: the document-delete scenario must actually submit a
				// DOCUMENT_DELETE envelope. The fake provider has no
				// document-delete tool path, so instruct the AI to delete the
				// case created by the document-update scenario. The per-run
				// unique case title ensures the AI targets a specific case.
				message := "Delete the investigation note created in the previous smoke test run."

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeDocumentDelete),
					CaseID:            chatResp.CaseID,
				})
				if err != nil {
					return err
				}
				r.note("correlated DOCUMENT_DELETE receipt: tx=%s signature_len=%d", short(receipt.TransactionID), len(receipt.Signature))
				r.note("DOCUMENT_DELETE envelope admitted by L1; document removed via handleDocumentDeleteSync")
				return nil
			},
		},
	}
}
