// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"fmt"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// WindowStore persists model provenance attestation windows.
type WindowStore interface {
	Save(ctx context.Context, window *evalv1.ModelProvenanceAttestationWindow) error
	Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error)
}

type fileWindowStore struct {
	fileSvc fs.RuntimeFileService
}

// NewWindowStore constructs a durable model provenance attestation store.
func NewWindowStore(fileSvc fs.RuntimeFileService) (WindowStore, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("model provenance window store: %w", constants.ErrMissingRequiredField)
	}
	return &fileWindowStore{fileSvc: fileSvc}, nil
}

func (s *fileWindowStore) windowsDir() string {
	return filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceModelProvenanceDirname, constants.InferenceModelProvenanceWindowsDirname)
}

func (s *fileWindowStore) windowPath(providerAttemptID string) string {
	return filepath.Join(s.windowsDir(), providerAttemptID+constants.FileExtJSON)
}

func (s *fileWindowStore) Save(ctx context.Context, window *evalv1.ModelProvenanceAttestationWindow) error {
	if window == nil || window.GetProviderAttemptId() == "" {
		return fmt.Errorf("model provenance window store: %w", constants.ErrMissingRequiredField)
	}
	if err := ValidateAttestationWindow(window); err != nil {
		return err
	}
	body, err := evalv1.MarshalCanonical(window)
	if err != nil {
		return fmt.Errorf("model provenance window store: marshal: %w", err)
	}
	if err := s.fileSvc.MkdirAll(ctx, s.windowsDir(), constants.PermDirStandard); err != nil {
		return fmt.Errorf("model provenance window store: mkdir: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, s.windowPath(window.GetProviderAttemptId()), body, constants.PermFilePrivate)
}

func (s *fileWindowStore) Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if providerAttemptID == "" {
		return nil, fmt.Errorf("model provenance window store: %w", constants.ErrMissingRequiredField)
	}
	body, err := s.fileSvc.ReadFile(ctx, s.windowPath(providerAttemptID))
	if err != nil {
		return nil, err
	}
	window := &evalv1.ModelProvenanceAttestationWindow{}
	if err := evalv1.UnmarshalCanonical(body, window); err != nil {
		return nil, fmt.Errorf("model provenance window store: unmarshal: %w", err)
	}
	if err := ValidateAttestationWindow(window); err != nil {
		return nil, err
	}
	return window, nil
}

// ComputeAttestationDigest returns the immutable digest for one attestation
// window with attestation_digest cleared.
func ComputeAttestationDigest(window *evalv1.ModelProvenanceAttestationWindow) (string, error) {
	if window == nil || window.GetProviderAttemptId() == "" || window.GetProvenanceOperatorId() == "" {
		return "", fmt.Errorf("model provenance: compute attestation digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(window).(*evalv1.ModelProvenanceAttestationWindow)
	if !ok {
		return "", fmt.Errorf("model provenance: compute attestation digest: invalid clone")
	}
	clone.AttestationDigest = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("model provenance: compute attestation digest: marshal: %w", err)
	}
	return models.SHA256Hex(data), nil
}

// ValidateAttestationWindow verifies digest binding and required fields.
func ValidateAttestationWindow(window *evalv1.ModelProvenanceAttestationWindow) error {
	if window == nil || window.GetProviderAttemptId() == "" || window.GetProvenanceOperatorId() == "" {
		return fmt.Errorf("model provenance: validate attestation window: %w", constants.ErrMissingRequiredField)
	}
	if window.GetSchemaVersion() != "" && window.GetSchemaVersion() != SchemaVersion {
		return fmt.Errorf("model provenance: validate attestation window: unsupported schema version")
	}
	expected, err := ComputeAttestationDigest(window)
	if err != nil {
		return err
	}
	if window.GetAttestationDigest() != expected {
		return fmt.Errorf("model provenance: validate attestation window: digest mismatch")
	}
	return nil
}
