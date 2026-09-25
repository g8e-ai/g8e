// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// AssignmentAuditProofInput carries one built assignment audit export package
// ready for proof-catalog ingest.
type AssignmentAuditProofInput struct {
	CampaignID       string
	CampaignRevision string
	RunID            string
	AssignmentID     string
	IndexDigest      string
	Artifacts        AssignmentAuditSliceArtifacts
	DeferMirrorPush  bool
}

// CampaignProofPublisher ingests assignment audit exports into the public proof
// catalog. Nil publishers skip proof publication.
type CampaignProofPublisher interface {
	IngestAssignmentAuditSlices(ctx context.Context, inputs []AssignmentAuditProofInput, deferMirrorPush bool) error
	PruneRunProofCatalog(ctx context.Context, runID string) error
	FlushProofCatalog(ctx context.Context) error
}

// AssignmentAuditProofIdempotencyKey returns the publication idempotency key for
// one assignment audit proof package.
func AssignmentAuditProofIdempotencyKey(runID, assignmentID string) string {
	return runID + ":" + assignmentID + ":audit-proof"
}

// CollectAssignmentAuditEventsFromBodies maps signed public live-event bodies
// into audit slice rows. Only record_type "event" payloads are accepted.
func CollectAssignmentAuditEventsFromBodies(bodies [][]byte) ([]AssignmentAuditSliceEvent, error) {
	events := make([]AssignmentAuditSliceEvent, 0, len(bodies))
	for _, body := range bodies {
		if len(body) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return nil, fmt.Errorf("evaluation: collect assignment audit events: decode body: %w", err)
		}
		kindRaw, ok := fields["kind"]
		if !ok {
			return nil, fmt.Errorf("evaluation: collect assignment audit events: missing kind: %w", constants.ErrEvidenceArtifactMalformed)
		}
		var kind string
		if err := json.Unmarshal(kindRaw, &kind); err != nil || kind == "" {
			return nil, fmt.Errorf("evaluation: collect assignment audit events: invalid kind: %w", constants.ErrEvidenceArtifactMalformed)
		}
		observedRaw, ok := fields["observed_at"]
		if !ok {
			return nil, fmt.Errorf("evaluation: collect assignment audit events: missing observed_at: %w", constants.ErrEvidenceArtifactMalformed)
		}
		var observedAt string
		if err := json.Unmarshal(observedRaw, &observedAt); err != nil || observedAt == "" {
			return nil, fmt.Errorf("evaluation: collect assignment audit events: invalid observed_at: %w", constants.ErrEvidenceArtifactMalformed)
		}
		events = append(events, AssignmentAuditSliceEvent{
			Type:      kind,
			Timestamp: observedAt,
			Payload:   append([]byte(nil), body...),
		})
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("evaluation: collect assignment audit events: no events: %w", constants.ErrMissingRequiredField)
	}
	return events, nil
}

func collectAssignmentAuditEventBodies(requests []campaignFeedPublishRequest) [][]byte {
	bodies := make([][]byte, 0, len(requests))
	for _, request := range requests {
		if request.RecordType != "" && request.RecordType != models.PublicFeedRecordTypeEvent {
			continue
		}
		if len(request.Body) == 0 {
			continue
		}
		bodies = append(bodies, request.Body)
	}
	return bodies
}

// BuildAssignmentAuditBindings builds one assignment audit export package from
// the live events about to be published for a terminal assignment. When
// ingestProofs is true, the proof publisher ingests the export package.
func (c *CampaignPublicationCoordinator) BuildAssignmentAuditBindings(
	ctx context.Context,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	liveRequests []campaignFeedPublishRequest,
	ingestProofs bool,
) ([]*evalv1.PublicEvidenceBinding, error) {
	if c == nil || assignment == nil || result == nil {
		return nil, nil
	}
	bodies := collectAssignmentAuditEventBodies(liveRequests)
	events, err := CollectAssignmentAuditEventsFromBodies(bodies)
	if err != nil {
		return nil, err
	}
	artifacts, err := BuildAssignmentAuditSlice(assignment.GetAssignmentId(), events)
	if err != nil {
		return nil, err
	}
	if !ingestProofs || c.proofPublisher == nil {
		return AssignmentAuditEvidenceBindings(artifacts), nil
	}
	spec, err := c.store.LoadCampaignSpec(ctx, assignment.GetCampaignId())
	if err != nil {
		return nil, fmt.Errorf("evaluation: build assignment audit bindings: load campaign spec: %w", err)
	}
	revision := spec.GetCampaignDigest()
	if revision == "" {
		revision = assignment.GetCampaignId()
	}
	indexDigest := result.GetResultDigest()
	if indexDigest == "" {
		indexDigest = assignment.GetAssignmentId()
	}
	input := AssignmentAuditProofInput{
		CampaignID:       assignment.GetCampaignId(),
		CampaignRevision: revision,
		RunID:            assignment.GetRunId(),
		AssignmentID:     assignment.GetAssignmentId(),
		IndexDigest:      indexDigest,
		Artifacts:        artifacts,
	}
	if c.deferProofMirrorPush {
		c.pendingProofInputs = append(c.pendingProofInputs, input)
		c.pendingProofMirrorPush = true
		return AssignmentAuditEvidenceBindings(artifacts), nil
	}
	if err := c.proofPublisher.IngestAssignmentAuditSlices(ctx, []AssignmentAuditProofInput{input}, false); err != nil {
		return nil, err
	}
	if err := c.recordPublishedProofArtifacts(ctx, input); err != nil {
		return nil, err
	}
	return AssignmentAuditEvidenceBindings(artifacts), nil
}
