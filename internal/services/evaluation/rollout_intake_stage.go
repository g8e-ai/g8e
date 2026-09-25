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
	"strings"
	"time"

)

// RolloutIntakeStageRequest stages catalog models through the governed Inference
// Operator session.
type RolloutIntakeStageRequest struct {
	Context              context.Context
	CatalogPath          string
	PullTimeout          time.Duration
	VariantIDs           []string
	DryRun               bool
	Progress             func(RolloutIntakeStageEvent)
	Dispatcher           OllamaModelCommandDispatcher
	InferenceSessionID   string
	Environment          map[string]string
	NewID                func(string) string
	CaseID               string
}

// RolloutIntakeStageEvent reports pull/copy progress for one catalog entry.
type RolloutIntakeStageEvent struct {
	VariantID      string
	ServedModelTag string
	Action         string
	Status         string
	Detail         string
}

// RolloutIntakeStageResult summarizes staging output.
type RolloutIntakeStageResult struct {
	Pulled  []string
	Aliased []string
	Skipped []RolloutIntakeStageSkip
	Failed  []RolloutIntakeStageFailure
}

// RolloutIntakeStageSkip records a deliberately skipped catalog entry.
type RolloutIntakeStageSkip struct {
	VariantID      string
	ServedModelTag string
	Reason         string
}

// RolloutIntakeStageFailure records a failed staging attempt.
type RolloutIntakeStageFailure struct {
	VariantID      string
	ServedModelTag string
	Err            string
}

func validateRolloutIntakeStageRequest(req RolloutIntakeStageRequest) error {
	if req.Dispatcher == nil || req.InferenceSessionID == "" || req.NewID == nil {
		return fmt.Errorf("evaluation: stage rollout intake: governed inference session and dispatcher are required")
	}
	return nil
}

func rolloutIntakeMaintenanceContext(req RolloutIntakeStageRequest) OllamaModelMaintenanceContext {
	return OllamaModelMaintenanceContext{
		TargetOperatorSessionID: req.InferenceSessionID,
		Environment:             req.Environment,
		Timeout:                 req.PullTimeout,
		CaseID:                  req.CaseID,
		NewID:                   req.NewID,
	}
}

// StageRolloutIntake pulls Hugging Face GGUF models through the governed
// Inference Operator and applies canonical served-model aliases from the catalog.
func StageRolloutIntake(req RolloutIntakeStageRequest) (*RolloutIntakeStageResult, error) {
	if err := validateRolloutIntakeStageRequest(req); err != nil {
		return nil, err
	}
	catalog, err := LoadRolloutIntakeCatalog(req.CatalogPath)
	if err != nil {
		return nil, err
	}
	pullTimeout := req.PullTimeout
	if pullTimeout <= 0 {
		pullTimeout = catalog.pullTimeout()
	}
	req.PullTimeout = pullTimeout
	if req.Context == nil {
		req.Context = context.Background()
	}
	selected := filterRolloutIntakeModels(catalog.Models, req.VariantIDs)
	return stageRolloutIntakeModels(req, selected)
}

// StageFormationCatalogIntake pulls sovereign ExecutionTopologies served tags
// through the governed Inference Operator session.
func StageFormationCatalogIntake(req RolloutIntakeStageRequest) (*RolloutIntakeStageResult, error) {
	if err := validateRolloutIntakeStageRequest(req); err != nil {
		return nil, err
	}
	pullTimeout := req.PullTimeout
	if pullTimeout <= 0 {
		pullTimeout = DefaultRolloutIntakePullTimeout()
	}
	req.PullTimeout = pullTimeout
	if req.Context == nil {
		req.Context = context.Background()
	}
	models, err := FormationCatalogIntakeModels()
	if err != nil {
		return nil, fmt.Errorf("evaluation: stage formation catalog intake: %w", err)
	}
	selected := filterRolloutIntakeModels(models, req.VariantIDs)
	return stageRolloutIntakeModels(req, selected)
}

func stageRolloutIntakeModels(req RolloutIntakeStageRequest, models []RolloutIntakeModel) (*RolloutIntakeStageResult, error) {
	maintenance := rolloutIntakeMaintenanceContext(req)
	result := &RolloutIntakeStageResult{}
	for _, model := range models {
		switch model.Staging.Method {
		case "ollama_hf_pull", "ollama_library_pull":
			if err := stageRolloutIntakePull(req, maintenance, model, result); err != nil {
				result.Failed = append(result.Failed, RolloutIntakeStageFailure{
					VariantID:      model.VariantID,
					ServedModelTag: model.ServedModelTag,
					Err:            err.Error(),
				})
			}
		case "manual_create":
			reason := strings.TrimSpace(model.Staging.OllamaPullError)
			if reason == "" {
				reason = "requires manual ollama create from Hugging Face GGUF"
			}
			result.Skipped = append(result.Skipped, RolloutIntakeStageSkip{
				VariantID:      model.VariantID,
				ServedModelTag: model.ServedModelTag,
				Reason:         reason,
			})
			emitStageEvent(req, RolloutIntakeStageEvent{
				VariantID:      model.VariantID,
				ServedModelTag: model.ServedModelTag,
				Action:         "skip",
				Status:         model.Staging.Method,
				Detail:         reason,
			})
		default:
			reason := strings.TrimSpace(model.Staging.Status)
			if reason == "" {
				reason = "staging method not supported yet"
			}
			result.Skipped = append(result.Skipped, RolloutIntakeStageSkip{
				VariantID:      model.VariantID,
				ServedModelTag: model.ServedModelTag,
				Reason:         reason,
			})
			emitStageEvent(req, RolloutIntakeStageEvent{
				VariantID:      model.VariantID,
				ServedModelTag: model.ServedModelTag,
				Action:         "skip",
				Status:         model.Staging.Method,
				Detail:         reason,
			})
		}
	}
	return result, nil
}

func stageRolloutIntakePull(req RolloutIntakeStageRequest, maintenance OllamaModelMaintenanceContext, model RolloutIntakeModel, result *RolloutIntakeStageResult) error {
	pullTag := strings.TrimSpace(model.Staging.OllamaPull)
	if pullTag == "" {
		return fmt.Errorf("evaluation: stage rollout intake: missing ollama_pull for %q", model.VariantID)
	}
	alias := strings.TrimSpace(model.Staging.Alias)
	if alias == "" {
		alias = model.ServedModelTag
	}

	emitStageEvent(req, RolloutIntakeStageEvent{
		VariantID:      model.VariantID,
		ServedModelTag: model.ServedModelTag,
		Action:         "pull",
		Status:         "starting",
		Detail:         pullTag,
	})
	if req.DryRun {
		result.Skipped = append(result.Skipped, RolloutIntakeStageSkip{
			VariantID:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			Reason:         "dry-run",
		})
		return nil
	}

	if err := PullOllamaModel(req.Context, req.Dispatcher, maintenance, pullTag); err != nil {
		return fmt.Errorf("pull %s: %w", pullTag, err)
	}
	emitStageEvent(req, RolloutIntakeStageEvent{
		VariantID:      model.VariantID,
		ServedModelTag: model.ServedModelTag,
		Action:         "pull",
		Status:         "success",
		Detail:         pullTag,
	})
	result.Pulled = append(result.Pulled, pullTag)

	if alias != "" && alias != pullTag {
		emitStageEvent(req, RolloutIntakeStageEvent{
			VariantID:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			Action:         "alias",
			Status:         "starting",
			Detail:         alias,
		})
		if err := CopyOllamaModel(req.Context, req.Dispatcher, maintenance, pullTag, alias); err != nil {
			return fmt.Errorf("alias %s -> %s: %w", pullTag, alias, err)
		}
		result.Aliased = append(result.Aliased, alias)
		emitStageEvent(req, RolloutIntakeStageEvent{
			VariantID:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			Action:         "alias",
			Status:         "success",
			Detail:         alias,
		})
	}
	return nil
}

func filterRolloutIntakeModels(models []RolloutIntakeModel, variantIDs []string) []RolloutIntakeModel {
	if len(variantIDs) == 0 {
		return append([]RolloutIntakeModel(nil), models...)
	}
	want := make(map[string]struct{}, len(variantIDs))
	for _, id := range variantIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	selected := make([]RolloutIntakeModel, 0, len(want))
	for _, model := range models {
		if _, ok := want[model.VariantID]; ok {
			selected = append(selected, model)
		}
	}
	return selected
}

func emitStageEvent(req RolloutIntakeStageRequest, event RolloutIntakeStageEvent) {
	if req.Progress != nil {
		req.Progress(event)
	}
}
