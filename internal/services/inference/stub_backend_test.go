// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// stubBackend is a test-only Backend implementation.
type stubBackend struct {
	generateResp *models.GenerateResponse
	generateErr  error
	statusResp   *models.BackendStatus
	statusErr    error
	lastReq      models.GenerateRequest
	calls        int
}

func (s *stubBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	s.calls++
	s.lastReq = req
	if s.generateErr != nil {
		return nil, s.generateErr
	}
	response := s.generateResp
	if response == nil {
		response = &models.GenerateResponse{Parts: textInferenceResponseParts("stub response"), Model: req.Model}
	}
	if response.NormalizedRequestHash == "" {
		response.NormalizedRequestHash = models.SHA256Hex([]byte("normalized test request"))
	}
	if response.OutputHash == "" {
		outputHash, err := models.ComputeInferenceOutputHash(response.Parts, response.FinishReason)
		if err != nil {
			return nil, err
		}
		response.OutputHash = outputHash
	}
	response.ServedModelDigest = req.ModelDigest
	return response, nil
}

func (s *stubBackend) Status(ctx context.Context) (*models.BackendStatus, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	if s.statusResp != nil {
		return s.statusResp, nil
	}
	return &models.BackendStatus{Available: true, Models: []string{"test-model"}}, nil
}
