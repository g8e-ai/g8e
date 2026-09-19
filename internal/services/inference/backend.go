// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package inference owns the backend abstraction, the Ollama HTTP client
// backend, and the governed execution handler for local LLM inference
// (g8ellama). Ollama owns process lifecycle, VRAM management, model
// eviction, and multi-model residency; the Go backend is an HTTP client to
// Ollama's /api/chat endpoint. The Backend interface has no
// LoadModel/Unload methods because Ollama routes by model name in the API
// call, so role routing is a config-and-payload concern, not a
// backend-interface concern.
package inference

import (
	"context"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// Backend is the inference backend abstraction. Implementations send
// generation requests to a local inference daemon (e.g., Ollama) and return
// the generated text, usage metadata, and finish reason. The interface is
// intentionally minimal: Ollama owns model loading, residency, and
// eviction, so the Go backend has no LoadModel/Unload methods.
type Backend interface {
	// Generate sends a generation request to the backend and returns the
	// generated text, usage metadata, and finish reason. The request
	// carries the Ollama model name (which encodes the role), the scrubbed
	// prompt, and generation parameters. Uses context.Context for
	// cancellation and timeout.
	Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error)

	// Status queries the backend's readiness and lists the models available
	// in its store. Used by the readiness check at operator startup.
	Status(ctx context.Context) (*models.BackendStatus, error)
}
