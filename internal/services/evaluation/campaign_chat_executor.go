// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"

	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignChatClient submits production chat and reads persisted g8ee traces.
type CampaignChatClient interface {
	EnsembleChat(ctx context.Context, persona harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error)
	GetEvaluationTrace(ctx context.Context, persona harnessclient.Persona, assignmentID, evaluationAttemptID string) (map[string]any, error)
}

// CampaignTraceWaiter polls until one g8ee trace reaches a terminal status.
type CampaignTraceWaiter func(ctx context.Context, fetch func(context.Context) (map[string]any, error)) (map[string]any, error)

// CampaignTraceStore persists imported g8ee trace evidence for one assignment.
type CampaignTraceStore interface {
	SaveAssignmentTrace(ctx context.Context, runID, assignmentID string, body []byte) error
}

// CampaignChatExecutor submits one scored assignment through production
// POST /api/v1/chat, imports the persisted trace, and returns the terminal result.
type CampaignChatExecutor struct {
	client                CampaignChatClient
	persona               harnessclient.Persona
	dataOperatorID        string
	dataOperatorSessionID string
	traceStore            CampaignTraceStore
	waitForTrace          CampaignTraceWaiter
	now                   func() time.Time
	newID                 func(string) string
}

// NewCampaignChatExecutor wires the production chat execution path for one run.
func NewCampaignChatExecutor(client CampaignChatClient, persona harnessclient.Persona, dataOperatorID, dataOperatorSessionID string, traceStore CampaignTraceStore, waitForTrace CampaignTraceWaiter, now func() time.Time, newID func(string) string) *CampaignChatExecutor {
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	return &CampaignChatExecutor{
		client:                client,
		persona:               persona,
		dataOperatorID:        dataOperatorID,
		dataOperatorSessionID: dataOperatorSessionID,
		traceStore:            traceStore,
		waitForTrace:          waitForTrace,
		now:                   now,
		newID:                 newID,
	}
}

// ExecuteAssignment submits one assignment through production chat and imports
// the terminal trace into a canonical assignment result.
func (e *CampaignChatExecutor) ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("evaluation: execute assignment: chat executor is required")
	}
	probeReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{
		GradingMethod:    req.GradingMethod,
		ScenarioGold:     req.ScenarioGold,
		ScenarioTools:    req.ScenarioTools,
		RequiredConcepts: req.RequiredConcepts,
	})
	if err != nil {
		return nil, err
	}
	chatReq, err := BuildChatProbeRequest(probeReq, e.dataOperatorID, e.dataOperatorSessionID)
	if err != nil {
		return nil, err
	}
	chatReq.Context.UserID = e.persona.UserID
	chatReq.Context.CLISessionID = e.persona.CLISessionID
	if _, err := e.client.EnsembleChat(ctx, e.persona, chatReq); err != nil {
		return nil, fmt.Errorf("evaluation: execute assignment: submit chat: %w", err)
	}
	if e.waitForTrace == nil {
		return nil, fmt.Errorf("evaluation: execute assignment: trace waiter is required")
	}
	trace, err := e.waitForTrace(ctx, func(pollCtx context.Context) (map[string]any, error) {
		return e.client.GetEvaluationTrace(pollCtx, e.persona, probeReq.AssignmentID, probeReq.EvaluationAttemptID)
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: execute assignment: wait for trace: %w", err)
	}
	traceBody, err := marshalSortedJSON(trace)
	if err != nil {
		return nil, fmt.Errorf("evaluation: execute assignment: canonicalize trace: %w", err)
	}
	var traceEvidence *compliancev1.ComplianceEvidenceReference
	if e.traceStore != nil {
		if err := e.traceStore.SaveAssignmentTrace(ctx, req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), traceBody); err != nil {
			return nil, fmt.Errorf("evaluation: execute assignment: persist trace evidence: %w", err)
		}
		traceEvidence, err = BuildAssignmentTraceEvidenceReference(req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), req.AttemptID, trace, e.now().UTC())
		if err != nil {
			return nil, fmt.Errorf("evaluation: execute assignment: build trace evidence reference: %w", err)
		}
	}
	return ImportAssignmentResultFromTrace(req, trace, traceEvidence, e.now().UTC(), e.newID)
}
