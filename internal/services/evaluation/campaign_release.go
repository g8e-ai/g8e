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
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ReleaseBasis says how a campaign's platform release is known.
type ReleaseBasis string

const (
	// ReleaseBasisRecorded is the release bound into campaign_digest at freeze time.
	ReleaseBasisRecorded ReleaseBasis = "recorded"
	// ReleaseBasisAsserted is an operator tag on a campaign frozen before
	// releases were recorded. It is not covered by the digest.
	ReleaseBasisAsserted ReleaseBasis = "asserted"
	// ReleaseBasisUnknown means neither a recorded release nor a tag exists.
	ReleaseBasisUnknown ReleaseBasis = "unknown"
)

// CampaignReleaseTag is the operator-asserted release of one campaign frozen
// before releases were recorded. It lives beside the campaign spec and never
// enters campaign_digest.
type CampaignReleaseTag struct {
	CampaignID string    `json:"campaign_id"`
	Release    string    `json:"release"`
	AssertedAt time.Time `json:"asserted_at"`
}

// CampaignRelease is the release a campaign measured and how that is known.
type CampaignRelease struct {
	Release        string       `json:"release"`
	Basis          ReleaseBasis `json:"release_basis"`
	SourceRevision string       `json:"source_revision,omitempty"`
}

func (r CampaignRelease) publicIdentity() CampaignRelease {
	if r.Basis == "" {
		r.Basis = ReleaseBasisUnknown
	}
	return r
}

func (r CampaignRelease) publicBasis() evalv1.PublicReleaseBasis {
	switch r.Basis {
	case ReleaseBasisRecorded:
		return evalv1.PublicReleaseBasis_PUBLIC_RELEASE_BASIS_RECORDED
	case ReleaseBasisAsserted:
		return evalv1.PublicReleaseBasis_PUBLIC_RELEASE_BASIS_ASSERTED
	default:
		return evalv1.PublicReleaseBasis_PUBLIC_RELEASE_BASIS_UNKNOWN
	}
}

// TagCampaignRelease records an operator-asserted release for a campaign that
// did not record one. A campaign that recorded its release cannot be tagged:
// the recorded release is digest-bound. Re-tagging an untagged-by-digest
// campaign replaces the earlier assertion.
func (s *Store) TagCampaignRelease(ctx context.Context, campaignID, release string, assertedAt time.Time) error {
	if s == nil || s.files == nil || !complianceevidence.ValidPathElement(campaignID) || strings.TrimSpace(release) == "" {
		return fmt.Errorf("evaluation: tag campaign release: %w", constants.ErrMissingRequiredField)
	}
	spec, err := s.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("evaluation: tag campaign release %q: %w", campaignID, err)
	}
	if recorded := spec.GetPlatformRelease(); recorded != "" {
		return fmt.Errorf("evaluation: tag campaign release %q: recorded release is %q: %w", campaignID, recorded, constants.ErrEvaluationReleaseRecorded)
	}
	body, err := json.MarshalIndent(CampaignReleaseTag{CampaignID: campaignID, Release: release, AssertedAt: assertedAt.UTC()}, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode campaign release tag: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	if err := s.files.WriteFile(ctx, s.layout.campaignReleaseTagPath(campaignID), body, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: write campaign release tag: %w", constants.ErrEvaluationReportPersistFailed, err)
	}
	return nil
}

// LoadCampaignRelease returns the release a campaign measured: the recorded
// release when the spec binds one, else the operator-asserted tag, else unknown.
func (s *Store) LoadCampaignRelease(ctx context.Context, campaignID string) (CampaignRelease, error) {
	spec, err := s.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return CampaignRelease{}, err
	}
	if recorded := spec.GetPlatformRelease(); recorded != "" {
		return CampaignRelease{Release: recorded, Basis: ReleaseBasisRecorded, SourceRevision: spec.GetSourceRevision()}, nil
	}
	body, err := s.files.ReadFile(ctx, s.layout.campaignReleaseTagPath(campaignID))
	if err != nil {
		if isNotFound(err) {
			return CampaignRelease{Basis: ReleaseBasisUnknown, SourceRevision: spec.GetSourceRevision()}, nil
		}
		return CampaignRelease{}, fmt.Errorf("evaluation: read campaign release tag: %w", err)
	}
	var tag CampaignReleaseTag
	if err := json.Unmarshal(body, &tag); err != nil || tag.CampaignID != campaignID || tag.Release == "" {
		return CampaignRelease{}, fmt.Errorf("%w: campaign release tag", constants.ErrEvidenceArtifactMalformed)
	}
	return CampaignRelease{Release: tag.Release, Basis: ReleaseBasisAsserted, SourceRevision: spec.GetSourceRevision()}, nil
}
