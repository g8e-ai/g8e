// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
)

// RoleHandlers holds the independently constructed handlers for a blended Operator.
type RoleHandlers struct {
	Inference                *inference.InferenceExecutionHandler
	InferenceAttemptStore    inference.AttemptStore
	ProviderBoundaryObserver *provider_observer.Handler
	ModelProvenanceOperator  *model_provenance.Handler
}

// NewRoleHandlers builds every enabled role for either embedded or outbound execution.
func NewRoleHandlers(cfg *config.Config, logger *slog.Logger, fileSvc fs.RuntimeFileService, scrubbingService *scrubbing.ScrubbingService, results ResultsPublisher, ollamaBackend *inference.OllamaBackend) (*RoleHandlers, error) {
	if cfg == nil || logger == nil || fileSvc == nil || results == nil {
		return nil, fmt.Errorf("operator role handlers: %w", constants.ErrMissingRequiredField)
	}
	if err := cfg.OperatorRoles.Validate(); err != nil {
		return nil, err
	}
	var err error
	if cfg.Inference.Enabled && ollamaBackend == nil {
		ollamaBackend, err = inference.NewOllamaBackend(cfg.Inference.OllamaEndpoint, logger)
		if err != nil {
			return nil, fmt.Errorf("operator roles: inference backend: %w", err)
		}
	}
	var inferenceHandler *inference.InferenceExecutionHandler
	var inferenceAttemptStore inference.AttemptStore
	if cfg.Inference.Enabled {
		inferenceHandler, err = inference.NewInferenceExecutionHandler(ollamaBackend, cfg, scrubbingService, logger)
		if err != nil {
			return nil, fmt.Errorf("operator roles: inference handler: %w", err)
		}
		inferenceAttemptStore, err = inference.NewAttemptStore(fileSvc)
		if err != nil {
			return nil, fmt.Errorf("operator roles: inference attempt store: %w", err)
		}
		logger.Info("Inference backend initialized",
			"endpoint", cfg.Inference.OllamaEndpoint)
	}

	var providerBoundaryObserver *provider_observer.Handler
	if cfg.ProviderBoundaryObserver.Enabled {
		tracker, err := provider_observer.NewTracker(provider_observer.TrackerConfig{
			ObserverID: cfg.ProviderBoundaryObserver.ObserverID,
			Collector:  provider_observer.DefaultCollector(),
		})
		if err != nil {
			return nil, fmt.Errorf("operator roles: provider boundary observer tracker: %w", err)
		}
		providerBoundaryObserver, err = provider_observer.NewHandler(tracker, results, logger)
		if err != nil {
			return nil, fmt.Errorf("operator roles: provider boundary observer handler: %w", err)
		}
		logger.Info("Provider-boundary observer enabled",
			"observer_id", cfg.ProviderBoundaryObserver.ObserverID)
	}

	var modelProvenanceOperator *model_provenance.Handler
	if cfg.ProvenanceOperator.Enabled {
		attestor, err := model_provenance.NewOllamaStorageAttestor(
			cfg.ProvenanceOperator.ModelStorageRoot,
			cfg.ProvenanceOperator.OperatorID,
		)
		if err != nil {
			return nil, fmt.Errorf("operator roles: model provenance attestor: %w", err)
		}
		tracker, err := model_provenance.NewTracker(model_provenance.TrackerConfig{
			OperatorID: cfg.ProvenanceOperator.OperatorID,
			Attestor:   attestor,
		})
		if err != nil {
			return nil, fmt.Errorf("operator roles: model provenance tracker: %w", err)
		}
		modelProvenanceOperator, err = model_provenance.NewHandler(tracker, results, logger)
		if err != nil {
			return nil, fmt.Errorf("operator roles: model provenance handler: %w", err)
		}
		logger.Info("Model provenance operator enabled",
			"operator_id", cfg.ProvenanceOperator.OperatorID,
			"model_storage_root", cfg.ProvenanceOperator.ModelStorageRoot)
	}

	return &RoleHandlers{Inference: inferenceHandler, InferenceAttemptStore: inferenceAttemptStore, ProviderBoundaryObserver: providerBoundaryObserver, ModelProvenanceOperator: modelProvenanceOperator}, nil
}
