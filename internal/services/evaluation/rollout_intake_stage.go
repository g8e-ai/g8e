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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/ollama"
)

// RolloutIntakeStageRequest stages catalog models on a remote Ollama provider.
type RolloutIntakeStageRequest struct {
	Context        context.Context
	CatalogPath    string
	OllamaEndpoint string
	PullTimeout    time.Duration
	VariantIDs     []string
	DryRun         bool
	Progress       func(RolloutIntakeStageEvent)
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

// StageRolloutIntake pulls Hugging Face GGUF models through Ollama's deep HF
// compatibility and applies canonical served-model aliases from the catalog.
func StageRolloutIntake(req RolloutIntakeStageRequest) (*RolloutIntakeStageResult, error) {
	catalog, err := LoadRolloutIntakeCatalog(req.CatalogPath)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(req.OllamaEndpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(catalog.OllamaEndpoint)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("evaluation: stage rollout intake: set ollama endpoint or G8E_OLLAMA_ENDPOINT")
	}
	pullTimeout := req.PullTimeout
	if pullTimeout <= 0 {
		pullTimeout = catalog.pullTimeout()
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	selected := filterRolloutIntakeModels(catalog.Models, req.VariantIDs)
	return stageRolloutIntakeModels(req, endpoint, pullTimeout, selected)
}

// StageFormationCatalogIntake pulls sovereign ExecutionTopologies served tags to
// the approved remote Ollama provider through the official Ollama client.
func StageFormationCatalogIntake(req RolloutIntakeStageRequest) (*RolloutIntakeStageResult, error) {
	endpoint := strings.TrimSpace(req.OllamaEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("evaluation: stage formation catalog intake: set ollama endpoint or G8E_OLLAMA_ENDPOINT")
	}
	pullTimeout := req.PullTimeout
	if pullTimeout <= 0 {
		pullTimeout = DefaultRolloutIntakePullTimeout()
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	models, err := FormationCatalogIntakeModels()
	if err != nil {
		return nil, fmt.Errorf("evaluation: stage formation catalog intake: %w", err)
	}
	selected := filterRolloutIntakeModels(models, req.VariantIDs)
	return stageRolloutIntakeModels(req, endpoint, pullTimeout, selected)
}

func stageRolloutIntakeModels(req RolloutIntakeStageRequest, endpoint string, pullTimeout time.Duration, models []RolloutIntakeModel) (*RolloutIntakeStageResult, error) {
	client, err := newRolloutIntakeOllamaClient(endpoint, pullTimeout)
	if err != nil {
		return nil, err
	}

	result := &RolloutIntakeStageResult{}
	for _, model := range models {
		switch model.Staging.Method {
		case "ollama_hf_pull", "ollama_library_pull":
			if err := stageRolloutIntakePull(req, client, model, result); err != nil {
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

func stageRolloutIntakePull(req RolloutIntakeStageRequest, client *ollama.Client, model RolloutIntakeModel, result *RolloutIntakeStageResult) error {
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

	lastStatus := ""
	if err := client.Pull(req.Context, pullTag, func(progress ollama.ProgressResponse) error {
		status := strings.TrimSpace(progress.Status)
		if status == "" || status == lastStatus {
			return nil
		}
		lastStatus = status
		emitStageEvent(req, RolloutIntakeStageEvent{
			VariantID:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			Action:         "pull",
			Status:         status,
			Detail:         pullTag,
		})
		return nil
	}); err != nil {
		return fmt.Errorf("pull %s: %w", pullTag, err)
	}
	result.Pulled = append(result.Pulled, pullTag)

	if alias != "" && alias != pullTag {
		emitStageEvent(req, RolloutIntakeStageEvent{
			VariantID:      model.VariantID,
			ServedModelTag: model.ServedModelTag,
			Action:         "alias",
			Status:         "starting",
			Detail:         alias,
		})
		if err := client.Copy(req.Context, pullTag, alias); err != nil {
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

func newRolloutIntakeOllamaClient(endpoint string, pullTimeout time.Duration) (*ollama.Client, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("evaluation: stage rollout intake: endpoint: %w: %w", constants.ErrInferenceEndpointInvalid, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("evaluation: stage rollout intake: endpoint %q: %w", endpoint, constants.ErrInferenceEndpointInvalid)
	}
	return ollama.NewClient(base, &http.Client{Timeout: pullTimeout}), nil
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
