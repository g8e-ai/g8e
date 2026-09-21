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
	policy CampaignVerificationPolicy
}

func NewCampaignImporter(reader fs.RuntimeFileService, runID string, policy CampaignVerificationPolicy) *CampaignImporter {
	if policy.AssessmentTime == nil {
		policy.AssessmentTime = time.Now
	}
	return &CampaignImporter{reader: reader, runID: runID, policy: policy}
}

func (i *CampaignImporter) SourceID() string {
	return constants.EvaluationSourceKindCampaign
}

func (i *CampaignImporter) RunID() string {
	if i == nil {
		return ""
	}
	return i.runID
}

func (i *CampaignImporter) Import(ctx context.Context) ([]complianceevidence.EvidenceNode, error) {
	if i == nil || i.reader == nil || !complianceevidence.ValidPathElement(i.runID) || i.policy.VerifierReleaseVersion == "" || i.policy.AssessmentTime == nil {
		return nil, fmt.Errorf("%w: campaign importer is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	result, err := verifyCampaignRunReadOnly(ctx, NewStore(i.reader), i.runID, i.policy)
	if err != nil {
		return nil, err
	}
	body, err := evalv1.MarshalCanonical(result.Report)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical campaign verification report: %v", constants.ErrEvidenceArtifactMalformed, err)
	}
	artifactID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalManifest, body)
	_, digest, ok := complianceevidence.ParseContentAddress(artifactID)
	if !ok {
		return nil, fmt.Errorf("%w: campaign verification report content address is invalid", constants.ErrEvidenceArtifactMalformed)
	}
	verifiedAt := result.Report.GetVerifiedAt().AsTime().UTC()
	return []complianceevidence.EvidenceNode{{
		ArtifactID:         artifactID,
		ArtifactType:       complianceevidence.ArtifactTypeEvalManifest,
		SHA256:             digest,
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.eval.v1.EvaluationVerificationReport",
		ProducerIdentity:   constants.CampaignVerifierID,
		ProducedAt:         verifiedAt,
		ScopeID:            constants.EvalScopePrefix + result.Run.GetCampaignBinding().GetCampaignId(),
		RunID:              i.runID,
		VerificationStatus: complianceevidence.VerificationStatusVerified,
		VerifierID:         constants.CampaignVerifierID,
		VerifierVersion:    constants.CampaignVerifierVersion,
		VerifiedAt:         verifiedAt,
		BundlePath:         constants.CampaignVerificationFilename,
		CanonicalBytes:     body,
	}}, nil
}
