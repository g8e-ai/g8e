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

// uniqueDocumentID returns a per-run unique document ID so document scenarios
// cannot collide with prior runs (C.4). The ID is namespaced with the scenario
// kind so update and delete runs are distinguishable in the document store.
func uniqueDocumentID(kind string) string {
	return fmt.Sprintf("%s-%s-%s", harnessDocIDPrefix, kind, uniqueRunID())
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

// ensembleApprovalConnectTimeout is the max time to wait for the
// auto-approver's SSE subscription to establish its first HTTP connection
// before sending the chat request. The SSE handshake against the local
// gateway is fast; 15s is generous and bounds the wait when the gateway is
// unreachable so the scenario fails fast with a clear message rather than
// hanging on the approval gate.
const ensembleApprovalConnectTimeout = 15 * time.Second

// ensembleSmokeFileDir is the directory under which per-run unique smoke-test
// files are created. The filename is unique per run (C.4) so retries cannot
// collide with prior state.
const ensembleSmokeFileDir = "/tmp"

// ensembleSmokeContent is the content the file-write scenario instructs the
// AI to write. A per-run unique suffix is appended so the read-back assertion
// can distinguish this run's content from any prior run's.
const ensembleSmokeContent = "g8e ensemble governed file write smoke test"

// harnessDocIDPrefix is the prefix for per-run unique document IDs created by
// the document-update and document-delete scenarios (C.3/C.4). The prefix
// namespaces harness-created documents so they are distinguishable from
// ensemble-created cases/investigations in the document store.
const harnessDocIDPrefix = "harness"

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
	NotBefore         time.Time
	OperatorSessionID string
	ActionType        string
	CaseID            string
	Persona           clientpkg.Persona
}

// pollForReceiptWithCorrelation polls the operator audit vault for a receipt
// matching the correlation criteria with a COMPLETED status, using the
// governed audit_receipt_list native MCP tool. Every poll traverses the
// L1–L5 governance gauntlet and produces its own audit record, replacing the
// previous unauthenticated HTTP audit endpoint polling. Returns the matching
// receipt, or an error if the timeout expires. If a matching receipt is found
// but its status is FAILED, returns ErrHarnessEnsembleReceiptFailed with the
// result summary. Stale, unrelated, or unsigned receipts are skipped (with a
// note) rather than accepted, so a historical receipt cannot produce a false
// pass. The caller captures NotBefore before sending the chat request and
// passes it here so stale receipts from prior runs are rejected.
func pollForReceiptWithCorrelation(ctx context.Context, c *clientpkg.Client, r *Result, corr ReceiptCorrelation) (*clientpkg.Receipt, error) {
	deadline := time.Now().Add(ensemblePollTimeout)
	ticker := time.NewTicker(ensemblePollInterval)
	defer ticker.Stop()

	r.note("polling operator audit vault via audit_receipt_list for %s receipt (not_before=%s, timeout %s)", corr.ActionType, corr.NotBefore.Format(time.RFC3339Nano), ensemblePollTimeout)

	for {
		receipts, err := queryAuditReceiptsViaGovernedTool(ctx, c, corr)
		if err != nil {
			r.note("audit_receipt_list query error (will retry): %v", err)
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

// queryAuditReceiptsViaGovernedTool invokes the audit_receipt_list native MCP
// tool through the gateway's governed tools/call dispatch path and decodes
// the returned ActionReceipt records into harness Receipt values. The tool
// scopes the query by operator_session_id and optionally by action_type and
// not_before; the caller applies additional client-side correlation checks
// for defense in depth.
func queryAuditReceiptsViaGovernedTool(ctx context.Context, c *clientpkg.Client, corr ReceiptCorrelation) ([]clientpkg.Receipt, error) {
	args := clientpkg.AuditReceiptListArgs{
		OperatorSessionID: corr.OperatorSessionID,
		ActionType:        corr.ActionType,
	}
	if !corr.NotBefore.IsZero() {
		args.NotBefore = corr.NotBefore.Format(time.RFC3339Nano)
	}

	resp, err := c.MCPToolsCall(ctx, corr.Persona, "audit_receipt_list", args)
	if err != nil {
		return nil, fmt.Errorf("audit_receipt_list: %w", err)
	}
	if resp != nil && resp.Error != nil {
		return nil, fmt.Errorf("audit_receipt_list rejected: %s", resp.Error.Message)
	}
	if resp == nil || len(resp.Result) == 0 {
		return nil, fmt.Errorf("audit_receipt_list returned empty result")
	}

	// The MCP tools/call result is a CallToolResult JSON:
	// {"content":[{"type":"text","text":"..."}]}
	var cr struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &cr); err != nil {
		return nil, fmt.Errorf("audit_receipt_list: decode CallToolResult: %w", err)
	}
	if cr.IsError {
		var sb strings.Builder
		for _, c := range cr.Content {
			sb.WriteString(c.Text)
		}
		return nil, fmt.Errorf("audit_receipt_list returned error: %s", sb.String())
	}
	if len(cr.Content) == 0 {
		return nil, nil
	}

	// The text content is the JSON-encoded AuditReceiptListResult:
	// {"receipts":[...],"count":N}
	var listResult struct {
		Receipts []json.RawMessage `json:"receipts"`
		Count    int               `json:"count"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &listResult); err != nil {
		return nil, fmt.Errorf("audit_receipt_list: decode receipts payload: %w", err)
	}

	out := make([]clientpkg.Receipt, 0, len(listResult.Receipts))
	for _, raw := range listResult.Receipts {
		var rec clientpkg.Receipt
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		rec.Raw = raw
		out = append(out, rec)
	}
	return out, nil
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

// governedDocumentReadBack retrieves a governed document from the gateway's
// document store via GET /api/v1/data/{collection}/{id}. This is the read-back
// path for document scenarios (C.3): the harness verifies the actual stored
// document, not just a receipt claiming the mutation occurred. Returns
// (nil, nil) when the document does not exist (HTTP 404) so callers can
// verify absence after a delete.
func governedDocumentReadBack(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, collection, documentID string) (clientpkg.DocumentResponse, error) {
	r.note("governed document read-back: GET %s/%s", collection, documentID)
	doc, _, err := c.GetDocument(ctx, persona, collection, documentID)
	if err != nil {
		return nil, fmt.Errorf("governed document read-back: %w", err)
	}
	if doc == nil {
		r.note("governed document read-back: %s/%s not found (404)", collection, documentID)
		return nil, nil
	}
	r.note("governed document read-back: %s/%s found", collection, documentID)
	return doc, nil
}

// submitDocumentUpdateAndCorrelate submits a governed DOCUMENT_UPDATE envelope
// directly to the admission API (bypassing the AI/ensemble layer), captures
// NotBefore before submission, polls for the correlated COMPLETED receipt, and
// verifies the receipt identity. This is the deterministic document-mutation
// path for C.3: no reliance on the LLM choosing the right tool call. The
// caller passes the current state root (fetched via StateRootFromMTLS just
// before submission to close the TOCTOU gap).
func submitDocumentUpdateAndCorrelate(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, req clientpkg.DocumentUpdateRequest) (*clientpkg.Receipt, time.Time, error) {
	notBefore := time.Now()
	r.note("submitting DOCUMENT_UPDATE envelope: %s/%s merge=%v", req.Collection, req.DocumentID, req.Merge)

	txHash, status, body, err := c.SubmitDocumentUpdate(ctx, persona, req)
	if err != nil {
		return nil, notBefore, fmt.Errorf("submit document update: %w", err)
	}
	if status >= 400 {
		return nil, notBefore, fmt.Errorf("submit document update: gateway returned status %d: %s", status, string(body))
	}
	r.note("DOCUMENT_UPDATE envelope admitted: tx=%s status=%d", short(txHash), status)

	receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
		NotBefore:         notBefore,
		OperatorSessionID: req.OperatorSessionID,
		ActionType:        string(constants.ActionTypeDocumentUpdate),
		Persona:           persona,
	})
	if err != nil {
		return nil, notBefore, err
	}
	r.note("correlated DOCUMENT_UPDATE receipt: tx=%s signature_len=%d", short(receipt.TransactionID), len(receipt.Signature))

	if err := verifyReceiptIdentity(r, receipt); err != nil {
		return nil, notBefore, err
	}
	return receipt, notBefore, nil
}

// submitDocumentDeleteAndCorrelate submits a governed DOCUMENT_DELETE envelope
// directly to the admission API, captures NotBefore, polls for the correlated
// COMPLETED receipt, and verifies the receipt identity. This is the
// deterministic document-deletion path for C.3.
func submitDocumentDeleteAndCorrelate(ctx context.Context, c *clientpkg.Client, r *Result, persona clientpkg.Persona, req clientpkg.DocumentDeleteRequest) (*clientpkg.Receipt, time.Time, error) {
	notBefore := time.Now()
	r.note("submitting DOCUMENT_DELETE envelope: %s/%s", req.Collection, req.DocumentID)

	txHash, status, body, err := c.SubmitDocumentDelete(ctx, persona, req)
	if err != nil {
		return nil, notBefore, fmt.Errorf("submit document delete: %w", err)
	}
	if status >= 400 {
		return nil, notBefore, fmt.Errorf("submit document delete: gateway returned status %d: %s", status, string(body))
	}
	r.note("DOCUMENT_DELETE envelope admitted: tx=%s status=%d", short(txHash), status)

	receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
		NotBefore:         notBefore,
		OperatorSessionID: req.OperatorSessionID,
		ActionType:        string(constants.ActionTypeDocumentDelete),
		Persona:           persona,
	})
	if err != nil {
		return nil, notBefore, err
	}
	r.note("correlated DOCUMENT_DELETE receipt: tx=%s signature_len=%d", short(receipt.TransactionID), len(receipt.Signature))

	if err := verifyReceiptIdentity(r, receipt); err != nil {
		return nil, notBefore, err
	}
	return receipt, notBefore, nil
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

				// The ensemble's file_create tool requires human approval before
				// executing the write. In CI/headless mode there is no human to
				// approve, so start an auto-approver that subscribes to the
				// gateway SSE stream and approves file edit approval requests on
				// behalf of the harness persona. The auto-approver runs in a
				// background goroutine and is stopped after the receipt is
				// correlated.
				ap := c.StartApprovalAutoApprover(ctx, persona)
				if ap != nil {
					defer ap.Stop()
					// Block the chat request until the SSE subscription is
					// established. The fake provider is near-instant; without
					// this gate the approval request is pushed before the
					// subscription connects and the gateway delivers it to zero
					// listeners (BLOCKER 6 timing race).
					connectCtx, cancelConnect := context.WithTimeout(ctx, ensembleApprovalConnectTimeout)
					if err := ap.WaitForConnection(connectCtx); err != nil {
						cancelConnect()
						return fmt.Errorf("approval auto-approver SSE connection: %w", err)
					}
					cancelConnect()
					r.note("approval auto-approver SSE subscription connected")
				}

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeFileEdit),
					CaseID:            chatResp.CaseID,
					Persona:           persona,
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

				// Same auto-approval pattern as ensemble-chat-file-create: the
				// file_write tool also goes through the approval gate.
				ap := c.StartApprovalAutoApprover(ctx, persona)
				if ap != nil {
					defer ap.Stop()
					// Block the chat request until the SSE subscription is
					// established. The fake provider is near-instant; without
					// this gate the approval request is pushed before the
					// subscription connects and the gateway delivers it to zero
					// listeners (BLOCKER 6 timing race).
					connectCtx, cancelConnect := context.WithTimeout(ctx, ensembleApprovalConnectTimeout)
					if err := ap.WaitForConnection(connectCtx); err != nil {
						cancelConnect()
						return fmt.Errorf("approval auto-approver SSE connection: %w", err)
					}
					cancelConnect()
					r.note("approval auto-approver SSE subscription connected")
				}

				chatResp, notBefore, err := ensembleChat(ctx, c, r, persona, message)
				if err != nil {
					return err
				}
				receipt, err := pollForReceiptWithCorrelation(ctx, c, r, ReceiptCorrelation{
					NotBefore:         notBefore,
					OperatorSessionID: kit.OperatorSessionID,
					ActionType:        string(constants.ActionTypeFileEdit),
					CaseID:            chatResp.CaseID,
					Persona:           persona,
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
			Name: "ensemble-document-update", Title: "Governed document update: create with merge=false, then merge=true partial update proving untouched fields survive (Bug 10 regression)", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				collection := string(constants.CollectionInvestigations)
				documentID := uniqueDocumentID("update")
				r.note("per-run artifact: collection=%s document_id=%s", collection, documentID)

				// Step 1: Fetch the current state root just before
				// submission to close the TOCTOU gap. The operator rejects
				// envelopes bound to a stale state root.
				stateRoot, err := c.StateRootFromMTLS(ctx)
				if err != nil {
					return fmt.Errorf("document update: fetch state root: %w", err)
				}

				// Step 2: Create the initial document with merge=false (full
				// model). All required fields are set so the merge=true patch
				// in step 3 can prove they survive.
				initialTitle := fmt.Sprintf("Initial investigation %s", uniqueRunID())
				initialFields := map[string]any{
					"case_title":    initialTitle,
					"case_id":       uniqueRunID(),
					"user_id":       kit.UserID,
					"sentinel_mode": true,
					"status":        "open",
				}
				createReq := clientpkg.DocumentUpdateRequest{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					RequestorUserID:   kit.UserID,
					Collection:        collection,
					DocumentID:        documentID,
					Updates:           initialFields,
					Merge:             false,
					StateRoot:         stateRoot,
				}
				if _, _, err := submitDocumentUpdateAndCorrelate(ctx, c, r, persona, createReq); err != nil {
					return err
				}

				// Step 3: Read back the initial document and verify all
				// fields were persisted.
				doc, err := governedDocumentReadBack(ctx, c, r, persona, collection, documentID)
				if err != nil {
					return err
				}
				if doc == nil {
					return fmt.Errorf("document update: initial document not found after create — read-back returned 404")
				}
				if got := doc.GetString("case_title"); got != initialTitle {
					return fmt.Errorf("document update: initial case_title mismatch — expected %q, got %q", initialTitle, got)
				}
				if got := doc.GetString("status"); got != "open" {
					return fmt.Errorf("document update: initial status mismatch — expected %q, got %q", "open", got)
				}
				if !doc.GetBool("sentinel_mode") {
					return fmt.Errorf("document update: initial sentinel_mode mismatch — expected true, got false")
				}
				r.note("initial document verified: case_title=%q status=open sentinel_mode=true", initialTitle)

				// Step 4: Apply a title-only patch with merge=true. Only
				// case_title is sent; all other fields must survive the merge
				// (Bug 10 regression at the scenario layer).
				stateRoot, err = c.StateRootFromMTLS(ctx)
				if err != nil {
					return fmt.Errorf("document update: fetch state root for merge: %w", err)
				}
				refinedTitle := fmt.Sprintf("Refined investigation %s", uniqueRunID())
				mergeReq := clientpkg.DocumentUpdateRequest{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					RequestorUserID:   kit.UserID,
					Collection:        collection,
					DocumentID:        documentID,
					Updates:           map[string]any{"case_title": refinedTitle},
					Merge:             true,
					StateRoot:         stateRoot,
				}
				if _, _, err := submitDocumentUpdateAndCorrelate(ctx, c, r, persona, mergeReq); err != nil {
					return err
				}

				// Step 5: Read back the merged document and verify the title
				// was updated AND all untouched fields survived (Bug 10).
				docAfter, err := governedDocumentReadBack(ctx, c, r, persona, collection, documentID)
				if err != nil {
					return err
				}
				if docAfter == nil {
					return fmt.Errorf("document update: document not found after merge — read-back returned 404")
				}
				if got := docAfter.GetString("case_title"); got != refinedTitle {
					return fmt.Errorf("document update: merged case_title mismatch — expected %q, got %q", refinedTitle, got)
				}
				if got := docAfter.GetString("status"); got != "open" {
					return fmt.Errorf("document update: status did not survive merge — expected %q, got %q", "open", got)
				}
				if !docAfter.GetBool("sentinel_mode") {
					return fmt.Errorf("document update: sentinel_mode did not survive merge — expected true, got false")
				}
				if got := docAfter.GetString("user_id"); got != kit.UserID {
					return fmt.Errorf("document update: user_id did not survive merge — expected %q, got %q", kit.UserID, got)
				}
				r.note("merge=true field survival verified: case_title updated, status/sentinel_mode/user_id preserved (Bug 10 regression clear)")
				return nil
			},
		},
		{
			Name: "ensemble-document-delete", Title: "Governed document delete: create then delete via DOCUMENT_DELETE, verify absence from document store", Persona: ensembleProducer, RequiresPosture: Doctrine,
			Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
				if kit == nil {
					return constants.ErrHarnessGovKitNotInit
				}
				persona := withCLIIdentity(ensembleProducer)
				collection := string(constants.CollectionInvestigations)
				documentID := uniqueDocumentID("delete")
				r.note("per-run artifact: collection=%s document_id=%s", collection, documentID)

				// Step 1: Create a document to delete. Without a prior
				// create, a delete would be a no-op and prove nothing.
				stateRoot, err := c.StateRootFromMTLS(ctx)
				if err != nil {
					return fmt.Errorf("document delete: fetch state root for create: %w", err)
				}
				createReq := clientpkg.DocumentUpdateRequest{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					RequestorUserID:   kit.UserID,
					Collection:        collection,
					DocumentID:        documentID,
					Updates: map[string]any{
						"case_title": fmt.Sprintf("Delete target %s", uniqueRunID()),
						"status":     "open",
					},
					Merge:     false,
					StateRoot: stateRoot,
				}
				if _, _, err := submitDocumentUpdateAndCorrelate(ctx, c, r, persona, createReq); err != nil {
					return err
				}

				// Step 2: Verify the document exists before deletion.
				docBefore, err := governedDocumentReadBack(ctx, c, r, persona, collection, documentID)
				if err != nil {
					return err
				}
				if docBefore == nil {
					return fmt.Errorf("document delete: document not found after create — cannot prove deletion of a non-existent document")
				}
				r.note("document exists before deletion: case_title=%q", docBefore.GetString("case_title"))

				// Step 3: Submit the DOCUMENT_DELETE envelope and correlate
				// the receipt.
				stateRoot, err = c.StateRootFromMTLS(ctx)
				if err != nil {
					return fmt.Errorf("document delete: fetch state root for delete: %w", err)
				}
				deleteReq := clientpkg.DocumentDeleteRequest{
					OperatorID:        kit.OperatorID,
					OperatorSessionID: kit.OperatorSessionID,
					RequestorUserID:   kit.UserID,
					Collection:        collection,
					DocumentID:        documentID,
					StateRoot:         stateRoot,
				}
				if _, _, err := submitDocumentDeleteAndCorrelate(ctx, c, r, persona, deleteReq); err != nil {
					return err
				}

				// Step 4: Verify the document is absent from the document
				// store. A COMPLETED receipt alone does not prove the
				// document was removed — the read-back must return 404.
				docAfter, err := governedDocumentReadBack(ctx, c, r, persona, collection, documentID)
				if err != nil {
					return err
				}
				if docAfter != nil {
					return fmt.Errorf("document delete: document still exists after DOCUMENT_DELETE — read-back returned a document, expected 404")
				}
				r.note("document absence verified: %s/%s returned 404 after DOCUMENT_DELETE", collection, documentID)
				return nil
			},
		},
	}
}
