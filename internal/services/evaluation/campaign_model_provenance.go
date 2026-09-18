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

const (
	modelProvenanceArtifactType = "model-provenance-attestation-window"
	modelProvenanceSchemaRef    = "g8e.eval.v1.ModelProvenanceAttestationWindow"
)

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
