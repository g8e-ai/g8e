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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignFormationWitnessReader persists and loads independent witness evidence
// for heterogeneous formation assignments.
type CampaignFormationWitnessReader struct {
	providerObservation *CampaignProviderObservationReader
	modelProvenance     *CampaignModelProvenanceReader
}

// NewCampaignFormationWitnessReader wires provider-boundary and provenance readers
// for formation witness persistence and verification.
func NewCampaignFormationWitnessReader(providerObservation *CampaignProviderObservationReader, modelProvenance *CampaignModelProvenanceReader) *CampaignFormationWitnessReader {
	return &CampaignFormationWitnessReader{
		providerObservation: providerObservation,
		modelProvenance:     modelProvenance,
	}
}

// PersistFormationWitnessEvidence stores observer and provenance windows captured
// during one governed formation run into canonical local evidence stores.
func (r *CampaignFormationWitnessReader) PersistFormationWitnessEvidence(ctx context.Context, formationResult *FormationRunResult) error {
	if r == nil || formationResult == nil {
		return nil
	}
	for _, role := range formationResult.Roles {
		if err := r.persistRoleWitnessEvidence(ctx, role); err != nil {
			return fmt.Errorf("formation role %s: %w", role.Role, err)
		}
	}
	return nil
}

func (r *CampaignFormationWitnessReader) persistRoleWitnessEvidence(ctx context.Context, role FormationRoleTelemetry) error {
	if role.ObserverEvidence != nil && role.ObserverEvidence.Window != nil {
		if r.providerObservation == nil {
			return fmt.Errorf("evaluation: persist formation observer evidence: %w", constants.ErrMissingRequiredField)
		}
		if err := r.providerObservation.ImportObservationWindow(ctx, role.ObserverEvidence.Window); err != nil {
			return fmt.Errorf("evaluation: persist formation observer evidence: %w", err)
		}
	}
	if role.ProvenanceEvidence != nil && role.ProvenanceEvidence.Window != nil {
		if r.modelProvenance == nil {
			return fmt.Errorf("evaluation: persist formation provenance evidence: %w", constants.ErrMissingRequiredField)
		}
		if err := r.modelProvenance.ImportProvenanceWindow(ctx, role.ProvenanceEvidence.Window); err != nil {
			return fmt.Errorf("evaluation: persist formation provenance evidence: %w", err)
		}
	}
	return nil
}

// BindFormationWitnessRefs attaches durable witness evidence references to scored
// model inferences when local windows exist.
func (r *CampaignFormationWitnessReader) BindFormationWitnessRefs(ctx context.Context, result *evalv1.EvaluationAssignmentResult) error {
	if r == nil || result == nil || r.providerObservation == nil {
		return nil
	}
	return r.providerObservation.BindProviderBoundaryObservationRefs(ctx, result)
}

// VerifyFormationWitnessEvidence independently checks persisted witness digests
// and telemetry bindings without re-running inference.
func VerifyFormationWitnessEvidence(
	ctx context.Context,
	evidence *FormationRunEvidence,
	providerReader *CampaignProviderObservationReader,
	provenanceReader *CampaignModelProvenanceReader,
	providerPolicy ProviderObservationPolicy,
	provenancePolicy ModelProvenancePolicy,
) []string {
	if evidence == nil {
		return []string{"formation run evidence is required for witness verification"}
	}
	failures := make([]string, 0)
	for index, role := range evidence.Result.Roles {
		if role.ProviderAttemptID == "" {
			failures = append(failures, fmt.Sprintf("formation role %d missing provider attempt id", index))
			continue
		}
		if role.ObserverObservationDigest == "" {
			if providerPolicy == ProviderObservationPolicyStrict {
				failures = append(failures, fmt.Sprintf("formation role %s missing observer observation digest", role.Role))
			}
			continue
		}
		if providerReader == nil {
			if providerPolicy == ProviderObservationPolicyStrict {
				failures = append(failures, fmt.Sprintf("formation role %s observer reader unavailable", role.Role))
			}
			continue
		}
		window, err := providerReader.LoadObservationWindow(ctx, role.ProviderAttemptID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("formation role %s observer window load failed: %v", role.Role, err))
			continue
		}
		if window.GetObservationDigest() != role.ObserverObservationDigest {
			failures = append(failures, fmt.Sprintf("formation role %s observer digest mismatch", role.Role))
		}
		if role.PeakVRAMMiB > 0 {
			observed := observedPeakVRAMMiB(window)
			if observed > 0 && observed != role.PeakVRAMMiB {
				failures = append(failures, fmt.Sprintf("formation role %s peak vram mismatch", role.Role))
			}
		}
		if role.ProvenanceAttestationDigest == "" {
			if role.AttestationStatus != string(FormationAttestationNotNeeded) && provenancePolicy == ModelProvenancePolicyStrict {
				failures = append(failures, fmt.Sprintf("formation role %s missing provenance attestation digest", role.Role))
			}
			continue
		}
		if provenanceReader == nil {
			if provenancePolicy == ModelProvenancePolicyStrict {
				failures = append(failures, fmt.Sprintf("formation role %s provenance reader unavailable", role.Role))
			}
			continue
		}
		provenanceWindow, err := provenanceReader.LoadProvenanceWindow(ctx, role.ProviderAttemptID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("formation role %s provenance window load failed: %v", role.Role, err))
			continue
		}
		if provenanceWindow.GetAttestationDigest() != role.ProvenanceAttestationDigest {
			failures = append(failures, fmt.Sprintf("formation role %s provenance digest mismatch", role.Role))
		}
		if role.ModelDigest != "" && provenanceWindow.GetExpectedModelDigest() != "" && provenanceWindow.GetExpectedModelDigest() != role.ModelDigest {
			failures = append(failures, fmt.Sprintf("formation role %s provenance expected digest mismatch", role.Role))
		}
	}
	return failures
}

func formationRoleWitnessDigests(role FormationRoleTelemetry) (observerDigest, provenanceDigest string) {
	if role.ObserverEvidence != nil && role.ObserverEvidence.Window != nil {
		observerDigest = role.ObserverEvidence.Window.GetObservationDigest()
	}
	if role.ProvenanceEvidence != nil && role.ProvenanceEvidence.Window != nil {
		provenanceDigest = role.ProvenanceEvidence.Window.GetAttestationDigest()
	}
	return observerDigest, provenanceDigest
}

// ImportObservationWindow persists one provider-boundary observation window.
func (r *CampaignProviderObservationReader) ImportObservationWindow(ctx context.Context, window *evalv1.ProviderBoundaryObservationWindow) error {
	if r == nil || window == nil || window.GetProviderAttemptId() == "" {
		return fmt.Errorf("evaluation: import provider observation window: %w", constants.ErrMissingRequiredField)
	}
	if r.localWindows == nil {
		return fmt.Errorf("evaluation: provider observation window store: %w", constants.ErrMissingRequiredField)
	}
	return r.localWindows.Save(ctx, window)
}

// ImportProvenanceWindow persists one model provenance attestation window.
func (r *CampaignModelProvenanceReader) ImportProvenanceWindow(ctx context.Context, window *evalv1.ModelProvenanceAttestationWindow) error {
	if r == nil || window == nil || window.GetProviderAttemptId() == "" {
		return fmt.Errorf("evaluation: import model provenance window: %w", constants.ErrMissingRequiredField)
	}
	if r.local == nil {
		return fmt.Errorf("evaluation: model provenance window store: %w", constants.ErrMissingRequiredField)
	}
	return r.local.Save(ctx, window)
}

// LoadProvenanceWindow returns one model provenance attestation window.
func (r *CampaignModelProvenanceReader) LoadProvenanceWindow(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if r == nil || r.windows == nil || providerAttemptID == "" {
		return nil, fmt.Errorf("evaluation: load model provenance window: %w", constants.ErrMissingRequiredField)
	}
	window, err := r.windows.Load(ctx, providerAttemptID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load model provenance window: %w", err)
	}
	return window, nil
}