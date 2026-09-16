// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

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

// WindowStore persists provider-boundary observation windows.
type WindowStore interface {
	Save(ctx context.Context, window *evalv1.ProviderBoundaryObservationWindow) error
	Load(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error)
}

type fileWindowStore struct {
	fileSvc fs.RuntimeFileService
}

// NewWindowStore constructs a durable provider-boundary observation store.
func NewWindowStore(fileSvc fs.RuntimeFileService) (WindowStore, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("provider observer window store: %w", constants.ErrMissingRequiredField)
	}
	return &fileWindowStore{fileSvc: fileSvc}, nil
}

func (s *fileWindowStore) windowsDir() string {
	return filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceProviderObserverDirname, constants.InferenceProviderObserverWindowsDirname)
}

func (s *fileWindowStore) windowPath(providerAttemptID string) string {
	return filepath.Join(s.windowsDir(), providerAttemptID+constants.FileExtJSON)
}

func (s *fileWindowStore) Save(ctx context.Context, window *evalv1.ProviderBoundaryObservationWindow) error {
	if window == nil || window.GetProviderAttemptId() == "" {
		return fmt.Errorf("provider observer window store: %w", constants.ErrMissingRequiredField)
	}
	if err := ValidateObservationWindow(window); err != nil {
		return err
	}
	body, err := evalv1.MarshalCanonical(window)
	if err != nil {
		return fmt.Errorf("provider observer window store: marshal: %w", err)
	}
	if err := s.fileSvc.MkdirAll(ctx, s.windowsDir(), constants.PermDirStandard); err != nil {
		return fmt.Errorf("provider observer window store: mkdir: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, s.windowPath(window.GetProviderAttemptId()), body, constants.PermFilePrivate)
}

func (s *fileWindowStore) Load(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error) {
	if providerAttemptID == "" {
		return nil, fmt.Errorf("provider observer window store: %w", constants.ErrMissingRequiredField)
	}
	body, err := s.fileSvc.ReadFile(ctx, s.windowPath(providerAttemptID))
	if err != nil {
		return nil, err
	}
	window := &evalv1.ProviderBoundaryObservationWindow{}
	if err := evalv1.UnmarshalCanonical(body, window); err != nil {
		return nil, fmt.Errorf("provider observer window store: unmarshal: %w", err)
	}
	if err := ValidateObservationWindow(window); err != nil {
		return nil, err
	}
	return window, nil
}

// ComputeObservationDigest returns the immutable digest for one observation
// window with observation_digest cleared.
func ComputeObservationDigest(window *evalv1.ProviderBoundaryObservationWindow) (string, error) {
	if window == nil || window.GetProviderAttemptId() == "" || window.GetObserverId() == "" {
		return "", fmt.Errorf("provider observer: compute observation digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(window).(*evalv1.ProviderBoundaryObservationWindow)
	if !ok {
		return "", fmt.Errorf("provider observer: compute observation digest: invalid clone")
	}
	clone.ObservationDigest = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("provider observer: compute observation digest: marshal: %w", err)
	}
	return models.SHA256Hex(data), nil
}

// ValidateObservationWindow verifies digest binding and required fields.
func ValidateObservationWindow(window *evalv1.ProviderBoundaryObservationWindow) error {
	if window == nil || window.GetProviderAttemptId() == "" || window.GetObserverId() == "" {
		return fmt.Errorf("provider observer: validate observation window: %w", constants.ErrMissingRequiredField)
	}
	if window.GetSchemaVersion() != "" && window.GetSchemaVersion() != SchemaVersion {
		return fmt.Errorf("provider observer: validate observation window: unsupported schema version")
	}
	expected, err := ComputeObservationDigest(window)
	if err != nil {
		return err
	}
	if window.GetObservationDigest() != expected {
		return fmt.Errorf("provider observer: validate observation window: digest mismatch")
	}
	return nil
}
