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
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ModelProvenanceRemote loads model provenance attestation evidence from a
// remote gateway when local runtime files are unavailable.
type ModelProvenanceRemote interface {
	Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error)
}

// ModelProvenancePolicy controls whether missing provenance attestations are
// verification failures or interim unavailable telemetry.
type ModelProvenancePolicy int

const (
	// ModelProvenancePolicyInterim records missing attestations as unavailable
	// reasons without failing assignment verification.
	ModelProvenancePolicyInterim ModelProvenancePolicy = 0
	// ModelProvenancePolicyStrict requires complete model provenance coverage
	// for every scored inference.
	ModelProvenancePolicyStrict ModelProvenancePolicy = 1
)

// ModelProvenanceReader loads model provenance attestation windows.
type ModelProvenanceReader interface {
	Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error)
}

type localModelProvenanceReader struct {
	store model_provenance.WindowStore
}

// NewLocalModelProvenanceReader constructs a reader over gateway-local
// provenance attestation windows.
func NewLocalModelProvenanceReader(fileSvc fs.RuntimeFileService) (ModelProvenanceReader, error) {
	store, err := model_provenance.NewWindowStore(fileSvc)
	if err != nil {
		return nil, err
	}
	return &localModelProvenanceReader{store: store}, nil
}

func (r *localModelProvenanceReader) Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if r == nil || r.store == nil || providerAttemptID == "" {
		return nil, fmt.Errorf("evaluation: load model provenance: %w", constants.ErrMissingRequiredField)
	}
	return r.store.Load(ctx, providerAttemptID)
}

// VerifyModelProvenanceWindow validates one attestation window against the
// expected campaign model digest when strict policy is enabled.
// CampaignModelProvenanceReader loads model provenance attestation windows
// for campaign verification.
type CampaignModelProvenanceReader struct {
	windows ModelProvenanceReader
}

// NewCampaignModelProvenanceReader constructs one read-only model provenance
// accessor from runtime file services.
func NewCampaignModelProvenanceReader(fileSvc fs.RuntimeFileService) (*CampaignModelProvenanceReader, error) {
	return NewCampaignModelProvenanceReaderWithRemote(fileSvc, nil)
}

// NewCampaignModelProvenanceReaderWithRemote constructs one read-only model
// provenance accessor from local runtime files with optional gateway fallback
// when local evidence is missing.
func NewCampaignModelProvenanceReaderWithRemote(fileSvc fs.RuntimeFileService, remote ModelProvenanceRemote) (*CampaignModelProvenanceReader, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("evaluation: model provenance reader: %w", constants.ErrMissingRequiredField)
	}
	localReader, err := NewLocalModelProvenanceReader(fileSvc)
	if err != nil {
		return nil, err
	}
	windows := localReader
	if remote != nil {
		windows = &fallbackModelProvenanceReader{local: localReader, remote: remote}
	}
	return &CampaignModelProvenanceReader{windows: windows}, nil
}

type fallbackModelProvenanceReader struct {
	local  ModelProvenanceReader
	remote ModelProvenanceRemote
}

func (s *fallbackModelProvenanceReader) Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	window, err := s.local.Load(ctx, providerAttemptID)
	if err == nil || !isModelProvenanceEvidenceNotFound(err) || s.remote == nil {
		return window, err
	}
	return s.remote.Load(ctx, providerAttemptID)
}

func isModelProvenanceEvidenceNotFound(err error) bool {
	return isGatewayEvidenceNotFound(err)
}

// VerifyAssignmentModelProvenance independently checks model provenance
// attestation coverage for every scored inference in one assignment result.
func (r *CampaignModelProvenanceReader) VerifyAssignmentModelProvenance(
	ctx context.Context,
	result *evalv1.EvaluationAssignmentResult,
	policy ModelProvenancePolicy,
) ([]string, []string) {
	if r == nil || result == nil {
		return nil, nil
	}
	failures := make([]string, 0)
	unavailable := make([]string, 0)
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		expectedDigest := inferenceRecord.GetModelVariant().GetModelDigest()
		window, err := r.windows.Load(ctx, attemptID)
		if err != nil {
			if isModelProvenanceEvidenceNotFound(err) {
				reason := fmt.Sprintf("model_provenance_missing:%s", attemptID)
				unavailable = append(unavailable, reason)
				if policy == ModelProvenancePolicyStrict {
					failures = append(failures, fmt.Sprintf("inference %s missing model provenance attestation window", inferenceRecord.GetInferenceRecordId()))
				}
				continue
			}
			failures = append(failures, fmt.Sprintf("inference %s model provenance window load failed: %v", inferenceRecord.GetInferenceRecordId(), err))
			continue
		}
		if err := VerifyModelProvenanceWindow(window, expectedDigest, policy); err != nil {
			failures = append(failures, fmt.Sprintf("inference %s model provenance verification failed: %v", inferenceRecord.GetInferenceRecordId(), err))
		}
	}
	return failures, unavailable
}

func VerifyModelProvenanceWindow(window *evalv1.ModelProvenanceAttestationWindow, expectedModelDigest string, policy ModelProvenancePolicy) error {
	if window == nil {
		if policy == ModelProvenancePolicyStrict {
			return fmt.Errorf("evaluation: model provenance: %w", constants.ErrEvaluationObservationUnavailable)
		}
		return nil
	}
	if err := model_provenance.ValidateAttestationWindow(window); err != nil {
		return err
	}
	if expectedModelDigest != "" && window.GetExpectedModelDigest() != expectedModelDigest {
		return fmt.Errorf("evaluation: model provenance: expected digest mismatch")
	}
	if policy == ModelProvenancePolicyStrict && !window.GetDigestMatch() {
		return fmt.Errorf("evaluation: model provenance: %w", constants.ErrModelProvenanceDigestMismatch)
	}
	return nil
}
