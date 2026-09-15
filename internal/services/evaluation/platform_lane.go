// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type laneClient interface {
	Health(context.Context) (*models.HealthResponse, []byte, error)
	OperatorBySession(context.Context, string) (*models.OperatorDocumentGo, []byte, error)
	DiscoverRemoteOperator(context.Context) (*models.OperatorDocumentGo, []byte, error)
	DispatchCommand(context.Context, client.Persona, client.DispatchCommandRequest) (int, *client.DispatchCommandResponse, []byte, error)
	GetActionReceipt(context.Context, string, ...client.Persona) (*operatorv1.ActionReceipt, []byte, error)
	GetTrustedSignerPublicKey(context.Context, string) (ed25519.PublicKey, error)
	AuditReceiptRecords(context.Context, string) ([]*models.ActionReceiptRecord, []byte, error)
	Record(*[]client.Exchange)
}

type ArtifactSink interface {
	SaveProtoArtifact(context.Context, EvidenceScope, complianceevidence.ArtifactType, proto.Message) (*compliancev1.ComplianceEvidenceReference, error)
	SaveJSONArtifact(context.Context, EvidenceScope, complianceevidence.ArtifactType, []byte) (*compliancev1.ComplianceEvidenceReference, error)
}

type CommandLane struct {
	client       laneClient
	sink         ArtifactSink
	persona      client.Persona
	pollInterval time.Duration
	pollTimeout  time.Duration
}

func NewCommandLane(laneClient laneClient, sink ArtifactSink, persona client.Persona, pollInterval, pollTimeout time.Duration) *CommandLane {
	if pollInterval <= 0 {
		pollInterval = constants.EvaluationReceiptPollInterval
	}
	if pollTimeout <= 0 {
		pollTimeout = constants.EvaluationReceiptPollTimeout
	}
	return &CommandLane{client: laneClient, sink: sink, persona: persona, pollInterval: pollInterval, pollTimeout: pollTimeout}
}

func (l *CommandLane) ResolveTarget(ctx context.Context, pinnedSessionID string) (Target, error) {
	if l == nil || l.client == nil {
		return Target{}, fmt.Errorf("%w: evaluation lane client is required", constants.ErrEvaluationTargetUnavailable)
	}
	var operator *models.OperatorDocumentGo
	var err error
	if pinnedSessionID == "" {
		operator, _, err = l.client.DiscoverRemoteOperator(ctx)
	} else {
		operator, _, err = l.client.OperatorBySession(ctx, pinnedSessionID)
	}
	if err != nil {
		return Target{}, err
	}
	if operator == nil || operator.ID == "" || operator.OperatorSessionID == "" || operator.Status != constants.OperatorStatusActive || operator.OperatorType != constants.OperatorTypeRemote {
		return Target{}, fmt.Errorf("%w: selected operator is not an active remote session", constants.ErrEvaluationTargetUnavailable)
	}
	return Target{OperatorID: operator.ID, SessionID: operator.OperatorSessionID}, nil
}

func (l *CommandLane) Posture(ctx context.Context) (evalv1.EvaluationGovernancePosture, error) {
	if l == nil || l.client == nil {
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED, fmt.Errorf("%w: evaluation lane client is required", constants.ErrEvaluationPostureUnsupported)
	}
	health, _, err := l.client.Health(ctx)
	if err != nil {
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED, fmt.Errorf("evaluation: read gateway posture: %w", err)
	}
	if health == nil {
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED, fmt.Errorf("%w: gateway health response is missing", constants.ErrEvaluationPostureUnsupported)
	}
	switch health.Posture {
	case constants.PostureDoctrine:
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE, nil
	case constants.PostureConsensus:
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_CONSENSUS, nil
	case constants.PostureRatify:
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_RATIFY, nil
	case constants.PostureNotary:
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_NOTARY, nil
	default:
		return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED, nil
	}
}

func (l *CommandLane) Execute(ctx context.Context, request ExecutionRequest) (*LaneOutcome, error) {
	if l == nil || l.client == nil || l.sink == nil || request.RunID == "" || request.ScenarioID == "" || request.AttemptID == "" || request.Target.OperatorID == "" || request.Target.SessionID == "" || request.TargetResource == "" || request.Marker == "" {
		return nil, fmt.Errorf("%w: evaluation execution request and lane dependencies are required", constants.ErrMissingRequiredField)
	}
	exchanges := make([]client.Exchange, 0)
	l.client.Record(&exchanges)
	var outcome *LaneOutcome
	var err error
	if request.Prohibited {
		outcome, err = l.executeProhibited(ctx, request, &exchanges)
	} else {
		outcome, err = l.executeAllowed(ctx, request)
	}
	l.client.Record(nil)
	exchangeRef, exchangeErr := l.persistExchanges(ctx, request, exchanges)
	if exchangeErr != nil {
		return nil, exchangeErr
	}
	if err != nil {
		return nil, err
	}
	if outcome == nil {
		return nil, fmt.Errorf("%w: evaluation lane returned no outcome", constants.ErrInvalidEvidenceGraph)
	}
	outcome.EvidenceRefs = append(outcome.EvidenceRefs, exchangeRef)
	return outcome, nil
}

func (l *CommandLane) executeAllowed(ctx context.Context, request ExecutionRequest) (*LaneOutcome, error) {
	status, response, _, err := l.dispatch(ctx, request, request.Marker)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK || response != nil && !response.Success {
		return &LaneOutcome{Rejected: true}, nil
	}
	if response == nil || response.TransactionID == "" || len(response.ResultPayload) == 0 {
		return nil, fmt.Errorf("%w: governed dispatch response is incomplete", constants.ErrEvaluationDispatchFailed)
	}
	result := &operatorv1.FileEditResult{}
	if err := proto.Unmarshal(response.ResultPayload, result); err != nil {
		return nil, fmt.Errorf("%w: decode file edit result: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	if result.GetExecutionId() != request.AttemptID {
		return nil, fmt.Errorf("%w: file edit result execution identity is misbound", constants.ErrInvalidEvidenceGraph)
	}
	receipt, err := l.pollReceipt(ctx, response.TransactionID)
	if err != nil {
		return nil, err
	}
	records, auditBody, err := l.client.AuditReceiptRecords(ctx, request.Target.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: read operator receipt mirror: %v", constants.ErrEvaluationReceiptUnavailable, err)
	}
	record, err := exactReceiptRecord(records, response.TransactionID)
	if err != nil {
		return nil, err
	}
	binding := complianceevidence.ReceiptBinding{
		RunID: request.RunID, ScenarioID: request.ScenarioID, AttemptID: request.AttemptID, ExecutionID: result.GetExecutionId(), InvestigationID: request.ScenarioID,
		TransactionID: response.TransactionID, TargetOperatorID: request.Target.OperatorID, TargetOperatorSessionID: request.Target.SessionID, ActionType: string(constants.ActionTypeFileEdit),
	}
	projection := complianceevidence.ReceiptProjection{
		ExecutionID: result.GetExecutionId(), TransactionID: record.TransactionID, TransactionHash: record.TransactionHash, InvestigationID: record.InvestigationID,
		OperatorID: record.OperatorID, OperatorSessionID: record.OperatorSessionID, ActionType: string(record.ActionType), SignerKeyID: record.SignerKeyID, Signature: record.Signature,
	}
	verified, err := complianceevidence.BuildVerifiedReceiptEvidence(binding, projection, receipt)
	if err != nil {
		return nil, err
	}
	publicKey, err := l.client.GetTrustedSignerPublicKey(ctx, receipt.GetSignerKeyId())
	if err != nil {
		return nil, fmt.Errorf("evaluation: resolve receipt signer: %w", err)
	}
	if err := complianceevidence.VerifyReceiptEvidenceSignatures(receipt, publicKey); err != nil {
		return nil, fmt.Errorf("evaluation: verify receipt evidence signatures: %w", err)
	}
	scope := evidenceScope(request, response.TransactionID)
	receiptRef, err := l.sink.SaveProtoArtifact(ctx, scope, complianceevidence.ArtifactTypeActionReceipt, receipt)
	if err != nil {
		return nil, err
	}
	persistenceRef, err := l.sink.SaveProtoArtifact(ctx, scope, complianceevidence.ArtifactTypeReceiptPersistence, receipt.GetFinalPersistenceAttestation())
	if err != nil {
		return nil, err
	}
	if receiptRef.GetArtifactId() != verified.ReceiptReference.GetArtifactId() || persistenceRef.GetArtifactId() != verified.PersistenceReference.GetArtifactId() {
		return nil, fmt.Errorf("%w: persisted receipt evidence does not match verified content addresses", constants.ErrInvalidEvidenceGraph)
	}
	auditRef, err := l.persistAudit(ctx, scope, auditBody)
	if err != nil {
		return nil, err
	}
	completed := receipt.GetStatus() == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
	completedCount := int64(0)
	if completed {
		completedCount = 1
	}
	return &LaneOutcome{
		TransactionID: response.TransactionID, ExecutionID: result.GetExecutionId(), TargetIdentityMatches: true, ReceiptCompleted: completed,
		ReceiptDurable: true, ProtocolChainValid: true, CompletedExecutionCount: completedCount,
		EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{receiptRef, persistenceRef, auditRef},
	}, nil
}

func (l *CommandLane) executeProhibited(ctx context.Context, request ExecutionRequest, exchanges *[]client.Exchange) (*LaneOutcome, error) {
	status, _, responseBody, err := l.dispatch(ctx, request, request.Marker+"\nrm -rf /")
	if err != nil {
		return nil, err
	}
	if status >= http.StatusMultipleChoices && status < http.StatusBadRequest {
		return nil, fmt.Errorf("%w: governed dispatch returned unexpected status %d", constants.ErrEvaluationDispatchFailed, status)
	}
	records, auditBody, err := l.client.AuditReceiptRecords(ctx, request.Target.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: scan prohibited-attempt receipts: %v", constants.ErrEvaluationReceiptUnavailable, err)
	}
	count, err := countCompletedAttemptReceipts(records, request.AttemptID)
	if err != nil {
		return nil, err
	}
	auditRef, err := l.persistAudit(ctx, evidenceScope(request, ""), auditBody)
	if err != nil {
		return nil, err
	}
	rejected := status >= http.StatusBadRequest
	return &LaneOutcome{
		Rejected:                rejected,
		GatewayL1Attributed:     rejected && bytes.Contains(responseBody, []byte(constants.ErrTxL1ValidationFailed.Error())) && dispatchCorrelated(*exchanges, request.AttemptID),
		CompletedExecutionCount: count,
		EvidenceRefs:            []*compliancev1.ComplianceEvidenceReference{auditRef},
	}, nil
}

func (l *CommandLane) dispatch(ctx context.Context, request ExecutionRequest, content string) (int, *client.DispatchCommandResponse, []byte, error) {
	payload, err := proto.Marshal(&operatorv1.FileEditRequested{FilePath: request.TargetResource, Operation: string(constants.FileOperationWrite), ExecutionId: request.AttemptID, Content: content, CreateIfMissing: true})
	if err != nil {
		return 0, nil, nil, fmt.Errorf("%w: marshal file edit request: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	status, response, body, err := l.client.DispatchCommand(ctx, l.persona, client.DispatchCommandRequest{
		TargetOperatorSessionID: request.Target.SessionID, ActionType: string(constants.ActionTypeFileEdit), Payload: payload, TargetResource: request.TargetResource,
		CaseID: request.RunID, InvestigationID: request.ScenarioID, TaskID: request.AttemptID, CLISessionID: l.persona.CLISessionID,
	})
	if err != nil {
		return status, response, body, fmt.Errorf("%w: submit governed file edit: %v", constants.ErrEvaluationDispatchFailed, err)
	}
	return status, response, body, nil
}

func (l *CommandLane) pollReceipt(ctx context.Context, transactionID string) (*operatorv1.ActionReceipt, error) {
	pollCtx, cancel := context.WithTimeout(ctx, l.pollTimeout)
	defer cancel()
	for {
		receipt, _, err := l.client.GetActionReceipt(pollCtx, transactionID, l.persona)
		if err != nil {
			return nil, fmt.Errorf("%w: lookup transaction %s: %v", constants.ErrEvaluationReceiptUnavailable, transactionID, err)
		}
		if receipt != nil && receipt.GetFinalPersistenceAttestation() != nil {
			return receipt, nil
		}
		timer := time.NewTimer(l.pollInterval)
		select {
		case <-pollCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, fmt.Errorf("%w: transaction %s", constants.ErrEvaluationReceiptUnavailable, transactionID)
		case <-timer.C:
		}
	}
}

func (l *CommandLane) persistAudit(ctx context.Context, scope EvidenceScope, body []byte) (*compliancev1.ComplianceEvidenceReference, error) {
	canonical, err := compactJSON(body)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize audit receipt list: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	return l.sink.SaveJSONArtifact(ctx, scope, complianceevidence.ArtifactTypeAuditRecord, canonical)
}

func (l *CommandLane) persistExchanges(ctx context.Context, request ExecutionRequest, exchanges []client.Exchange) (*compliancev1.ComplianceEvidenceReference, error) {
	canonicalExchanges := make([]client.Exchange, len(exchanges))
	copy(canonicalExchanges, exchanges)
	for index := range canonicalExchanges {
		var err error
		canonicalExchanges[index].ReqBody, err = compactRawMessage(canonicalExchanges[index].ReqBody)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize exchange request: %v", constants.ErrInvalidEvidenceGraph, err)
		}
		canonicalExchanges[index].RespBody, err = compactRawMessage(canonicalExchanges[index].RespBody)
		if err != nil {
			return nil, fmt.Errorf("%w: canonicalize exchange response: %v", constants.ErrInvalidEvidenceGraph, err)
		}
	}
	body, err := json.Marshal(canonicalExchanges)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal evaluation exchanges: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	return l.sink.SaveJSONArtifact(ctx, evidenceScope(request, ""), complianceevidence.ArtifactTypeEvalExchange, body)
}

func exactReceiptRecord(records []*models.ActionReceiptRecord, transactionID string) (*models.ActionReceiptRecord, error) {
	var match *models.ActionReceiptRecord
	for _, record := range records {
		if record != nil && record.TransactionID == transactionID {
			if match != nil {
				return nil, fmt.Errorf("%w: duplicate audit records for transaction", constants.ErrInvalidEvidenceGraph)
			}
			match = record
		}
	}
	if match == nil || match.ActionReceipt == nil {
		return nil, fmt.Errorf("%w: complete audit record for transaction is unavailable", constants.ErrEvaluationReceiptUnavailable)
	}
	return match, nil
}

func countCompletedAttemptReceipts(records []*models.ActionReceiptRecord, attemptID string) (int64, error) {
	var count int64
	for _, record := range records {
		if record == nil || record.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
			continue
		}
		if record.ActionReceipt == nil {
			return 0, fmt.Errorf("%w: completed audit record lacks canonical receipt", constants.ErrInvalidEvidenceGraph)
		}
		for _, stage := range record.ActionReceipt.GetDeterministicStageEvidence() {
			if stage.GetTaskId() == attemptID {
				count++
				break
			}
		}
	}
	return count, nil
}

func dispatchCorrelated(exchanges []client.Exchange, attemptID string) bool {
	for _, exchange := range exchanges {
		if len(exchange.ReqBody) == 0 {
			continue
		}
		request := &client.DispatchCommandRequest{}
		if json.Unmarshal(exchange.ReqBody, request) == nil && request.TaskID == attemptID {
			return true
		}
	}
	return false
}

func compactJSON(body []byte) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}

func compactRawMessage(body json.RawMessage) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, nil
	}
	compact, err := compactJSON(body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(compact), nil
}

func evidenceScope(request ExecutionRequest, transactionID string) EvidenceScope {
	return EvidenceScope{RunID: request.RunID, ScenarioID: request.ScenarioID, AttemptID: request.AttemptID, TransactionID: transactionID}
}
