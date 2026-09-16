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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignImporter loads persisted model-campaign artifacts into the evidence
// graph without invoking inference or mutation.
type CampaignImporter struct {
	reader fs.RuntimeFileService
	runID  string
	now    func() time.Time
}

func NewCampaignImporter(reader fs.RuntimeFileService, runID string) *CampaignImporter {
	return &CampaignImporter{reader: reader, runID: runID, now: time.Now}
}

// ImportAssignmentResult reads one terminal assignment result and returns a
// verified evidence node for downstream compliance import.
func (i *CampaignImporter) ImportAssignmentResult(ctx context.Context, assignmentID string) (complianceevidence.EvidenceNode, error) {
	if i == nil || i.reader == nil || !complianceevidence.ValidPathElement(i.runID) || !complianceevidence.ValidPathElement(assignmentID) || i.now == nil {
		return complianceevidence.EvidenceNode{}, fmt.Errorf("%w: campaign importer is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	store := NewStore(i.reader)
	result, err := store.LoadAssignmentResult(ctx, i.runID, assignmentID)
	if err != nil {
		return complianceevidence.EvidenceNode{}, err
	}
	body, err := evalv1.MarshalCanonical(result)
	if err != nil {
		return complianceevidence.EvidenceNode{}, fmt.Errorf("%w: canonical assignment result: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	artifactID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalAttempt, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return complianceevidence.EvidenceNode{}, fmt.Errorf("%w: assignment result content address is invalid", constants.ErrEvidenceArtifactMalformed)
	}
	producedAt := i.now().UTC()
	if result.GetCompletedAt() != nil && result.GetCompletedAt().IsValid() {
		producedAt = result.GetCompletedAt().AsTime().UTC()
	}
	return complianceevidence.EvidenceNode{
		ArtifactID:         artifactID,
		ArtifactType:       complianceevidence.ArtifactTypeEvalAttempt,
		SHA256:             digest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.eval.v1.EvaluationAssignmentResult",
		ProducerIdentity:   GraderID,
		ProducedAt:         producedAt,
		ScopeID:            constants.EvalScopePrefix + result.GetCampaignId(),
		RunID:              result.GetRunId(),
		AttemptID:          result.GetAssignmentId(),
		VerificationStatus: complianceevidence.VerificationStatusVerified,
		VerifierID:         constants.EvalRunVerifierID,
		VerifierVersion:    constants.EvalRunVerifierVersion,
		VerifiedAt:         producedAt,
		BundlePath:         assignmentResultPath(result.GetRunId(), result.GetAssignmentId()),
		CanonicalBytes:     body,
	}, nil
}
