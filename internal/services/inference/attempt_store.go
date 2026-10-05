// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// AttemptStore persists durable operator-local provider-attempt records.
type AttemptStore interface {
	Begin(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error
	Import(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error
	Complete(ctx context.Context, providerAttemptID, resultDigest string) error
	Fail(ctx context.Context, providerAttemptID, failureSummary string) error
	Get(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceProviderAttemptRecord, error)
	SaveRawResponse(ctx context.Context, providerAttemptID string, response []byte) error
	RawResponse(ctx context.Context, providerAttemptID string) ([]byte, error)
	SaveResult(ctx context.Context, providerAttemptID string, result *operatorv1.InferenceResult) error
	Result(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceResult, error)
}

type fileAttemptStore struct {
	fileSvc fs.RuntimeFileService
}

// NewAttemptStore constructs a durable provider-attempt store under
// data/inference/attempts within the runtime directory.
func NewAttemptStore(fileSvc fs.RuntimeFileService) (AttemptStore, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("inference attempt store: %w", constants.ErrMissingRequiredField)
	}
	return &fileAttemptStore{fileSvc: fileSvc}, nil
}

func (s *fileAttemptStore) attemptsDir() string {
	return filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname)
}

// Raw responses are opaque bytes: decoding or canonicalization must never
// change this artifact. Failed attempts retain it alongside their failure record.
func (s *fileAttemptStore) SaveRawResponse(ctx context.Context, providerAttemptID string, response []byte) error {
	if _, err := s.loadRequired(ctx, providerAttemptID); err != nil {
		return err
	}
	return s.fileSvc.WriteFile(ctx, filepath.Join(s.attemptsDir(), providerAttemptID+constants.InferenceRawResponseSuffix), response, constants.PermFilePrivate)
}

func (s *fileAttemptStore) RawResponse(ctx context.Context, providerAttemptID string) ([]byte, error) {
	if _, err := s.recordPath(providerAttemptID); err != nil {
		return nil, err
	}
	return s.fileSvc.ReadFile(ctx, filepath.Join(s.attemptsDir(), providerAttemptID+constants.InferenceRawResponseSuffix))
}

// SaveResult records the validated application result independently of the
// original provider bytes. Complete binds its digest into the attempt record.
func (s *fileAttemptStore) SaveResult(ctx context.Context, providerAttemptID string, result *operatorv1.InferenceResult) error {
	if result == nil || result.GetProviderAttemptId() != providerAttemptID {
		return fmt.Errorf("inference attempt store: result: %w", constants.ErrInferenceIdentityMismatch)
	}
	if _, err := s.loadRequired(ctx, providerAttemptID); err != nil {
		return err
	}
	data, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("inference attempt store: result marshal: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, filepath.Join(s.attemptsDir(), providerAttemptID+constants.InferenceProcessedResultSuffix), data, constants.PermFilePrivate)
}

func (s *fileAttemptStore) Result(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceResult, error) {
	if _, err := s.recordPath(providerAttemptID); err != nil {
		return nil, err
	}
	data, err := s.fileSvc.ReadFile(ctx, filepath.Join(s.attemptsDir(), providerAttemptID+constants.InferenceProcessedResultSuffix))
	if err != nil {
		return nil, err
	}
	result := &operatorv1.InferenceResult{}
	if err := proto.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("inference attempt store: result unmarshal: %w", err)
	}
	return result, nil
}

func (s *fileAttemptStore) recordPath(providerAttemptID string) (string, error) {
	if err := models.ValidateProviderAttemptID(providerAttemptID); err != nil {
		return "", fmt.Errorf("inference attempt store: %w", err)
	}
	return filepath.Join(s.attemptsDir(), providerAttemptID+constants.FileExtJSON), nil
}

func (s *fileAttemptStore) Begin(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error {
	if record == nil || record.GetProviderAttemptId() == "" || record.GetTransactionId() == "" {
		return fmt.Errorf("inference attempt store: %w", constants.ErrMissingRequiredField)
	}
	existing, err := s.Get(ctx, record.GetProviderAttemptId())
	if err != nil {
		if !errors.Is(err, constants.ErrNotFound) {
			return fmt.Errorf("inference attempt store: %w", err)
		}
	} else {
		switch existing.GetStatus() {
		case operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS,
			operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
			operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_FAILED:
			return fmt.Errorf("inference attempt store: begin: %w", constants.ErrInferenceProviderAttemptConflict)
		}
	}
	if err := s.fileSvc.MkdirAll(ctx, s.attemptsDir(), constants.PermDirStandard); err != nil {
		return fmt.Errorf("inference attempt store: mkdir: %w", err)
	}
	if record.GetStartedAtUnixMs() == 0 {
		record.StartedAtUnixMs = time.Now().UnixMilli()
	}
	if record.GetStatus() == operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_UNSPECIFIED {
		record.Status = operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS
	}
	return s.write(ctx, record)
}

func (s *fileAttemptStore) Import(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error {
	if record == nil || record.GetProviderAttemptId() == "" || record.GetStartedAtUnixMs() == 0 || record.GetStatus() == operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_UNSPECIFIED {
		return fmt.Errorf("inference attempt store: import: %w", constants.ErrMissingRequiredField)
	}
	if record.GetStatus() != operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS && record.GetCompletedAtUnixMs() == 0 {
		return fmt.Errorf("inference attempt store: import: %w", constants.ErrMissingRequiredField)
	}
	existing, err := s.Get(ctx, record.GetProviderAttemptId())
	if err == nil {
		if proto.Equal(existing, record) {
			return nil
		}
		return fmt.Errorf("inference attempt store: import: %w", constants.ErrInferenceProviderAttemptConflict)
	}
	if !errors.Is(err, constants.ErrNotFound) {
		return fmt.Errorf("inference attempt store: import: %w", err)
	}
	if err := s.fileSvc.MkdirAll(ctx, s.attemptsDir(), constants.PermDirStandard); err != nil {
		return fmt.Errorf("inference attempt store: import mkdir: %w", err)
	}
	return s.write(ctx, record)
}

func (s *fileAttemptStore) Complete(ctx context.Context, providerAttemptID, resultDigest string) error {
	record, err := s.loadRequired(ctx, providerAttemptID)
	if err != nil {
		return err
	}
	if record.GetStatus() != operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS {
		return fmt.Errorf("inference attempt store: complete: %w", constants.ErrInferenceProviderAttemptConflict)
	}
	record.Status = operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED
	record.ResultDigest = resultDigest
	record.CompletedAtUnixMs = time.Now().UnixMilli()
	return s.write(ctx, record)
}

func (s *fileAttemptStore) Fail(ctx context.Context, providerAttemptID, failureSummary string) error {
	record, err := s.loadRequired(ctx, providerAttemptID)
	if err != nil {
		return err
	}
	if record.GetStatus() != operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS {
		return fmt.Errorf("inference attempt store: fail: %w", constants.ErrInferenceProviderAttemptConflict)
	}
	record.Status = operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_FAILED
	record.FailureSummary = failureSummary
	record.CompletedAtUnixMs = time.Now().UnixMilli()
	return s.write(ctx, record)
}

func (s *fileAttemptStore) Get(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceProviderAttemptRecord, error) {
	path, err := s.recordPath(providerAttemptID)
	if err != nil {
		return nil, err
	}
	data, err := s.fileSvc.ReadFile(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("inference attempt store: read: %w", err)
	}
	record := &operatorv1.InferenceProviderAttemptRecord{}
	if err := protojson.Unmarshal(data, record); err != nil {
		return nil, fmt.Errorf("inference attempt store: unmarshal: %w", err)
	}
	return record, nil
}

func (s *fileAttemptStore) loadRequired(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceProviderAttemptRecord, error) {
	record, err := s.Get(ctx, providerAttemptID)
	if err != nil {
		return nil, fmt.Errorf("inference attempt store: %w", err)
	}
	return record, nil
}

func (s *fileAttemptStore) write(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error {
	path, err := s.recordPath(record.GetProviderAttemptId())
	if err != nil {
		return err
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(record)
	if err != nil {
		return fmt.Errorf("inference attempt store: marshal: %w", err)
	}
	return s.fileSvc.WriteFile(ctx, path, data, constants.PermFilePrivate)
}
