// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func resolveEvaluationInventorySource(explicitPath, projectRoot string) (runtimePath string, externalPath string, err error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath == "" {
		return "", "", nil
	}
	if filepath.IsAbs(explicitPath) {
		return "", explicitPath, nil
	}
	normalized := filepath.ToSlash(explicitPath)
	if strings.HasPrefix(normalized, constants.RuntimeDirname+"/") {
		return strings.TrimPrefix(normalized, constants.RuntimeDirname+"/"), "", nil
	}
	if normalized == evaluation.DefaultBaseModelInventoryRelPath {
		return "", filepath.Join(projectRoot, normalized), nil
	}
	return normalized, "", nil
}

func loadEvaluationInventoryVariants(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot, explicitPath string) ([]*evalv1.ModelVariant, error) {
	runtimePath, externalPath, err := resolveEvaluationInventorySource(explicitPath, projectRoot)
	if err != nil {
		return nil, err
	}
	if externalPath != "" {
		return evaluation.LoadFrozenVariantsFromExternalSource(externalPath)
	}
	if runtimePath == "" {
		if exists, err := fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath); err != nil {
			return nil, fmt.Errorf("evaluation: inventory: check runtime freeze: %w", err)
		} else if exists {
			return evaluation.LoadFrozenVariantsFromRuntime(ctx, fileSvc, evaluation.DefaultModelInventoryRelPath)
		}
		return evaluation.LoadFrozenVariantsFromExternalSource(filepath.Join(projectRoot, evaluation.DefaultBaseModelInventoryRelPath))
	}
	return evaluation.LoadFrozenVariantsFromRuntime(ctx, fileSvc, runtimePath)
}

func loadEvaluationInventoryFreeze(ctx context.Context, fileSvc fs.RuntimeFileService, projectRoot, explicitPath string) (*evaluation.ModelInventoryFreeze, error) {
	runtimePath, externalPath, err := resolveEvaluationInventorySource(explicitPath, projectRoot)
	if err != nil {
		return nil, err
	}
	if externalPath != "" {
		return evaluation.LoadModelInventoryFreezeFile(externalPath)
	}
	if runtimePath == "" {
		if exists, err := fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath); err != nil {
			return nil, fmt.Errorf("evaluation: inventory: check runtime freeze: %w", err)
		} else if exists {
			return evaluation.LoadModelInventoryFreezeFromRuntime(ctx, fileSvc, evaluation.DefaultModelInventoryRelPath)
		}
		return evaluation.LoadModelInventoryFreezeFile(filepath.Join(projectRoot, evaluation.DefaultBaseModelInventoryRelPath))
	}
	return evaluation.LoadModelInventoryFreezeFromRuntime(ctx, fileSvc, runtimePath)
}

func writeModelInventoryFreezeFile(path string, freeze *evaluation.ModelInventoryFreeze) error {
	payload, err := modelInventoryFreezeJSON(freeze)
	if err != nil {
		return err
	}
	return writeCatalogInventory(path, payload)
}

func modelInventoryFreezeJSON(freeze *evaluation.ModelInventoryFreeze) ([]byte, error) {
	if freeze == nil {
		return nil, fmt.Errorf("evaluation: inventory freeze json: %w", constants.ErrMissingRequiredField)
	}
	variants := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		body, err := protojson.Marshal(variant)
		if err != nil {
			return nil, fmt.Errorf("evaluation: inventory freeze json: marshal variant: %w", err)
		}
		variants = append(variants, body)
	}
	payload := struct {
		CampaignID           string                              `json:"campaign_id"`
		ModelRegistryDigest  string                              `json:"model_registry_digest"`
		ModelCount           int                                 `json:"model_count"`
		HomogeneousCellCount uint64                              `json:"homogeneous_cell_count"`
		Variants             []json.RawMessage                   `json:"variants"`
		InferenceVariants    []*operatorv1.InferenceModelVariant `json:"variants_inference"`
	}{
		CampaignID:           freeze.CampaignID,
		ModelRegistryDigest:  freeze.RegistryDigest,
		ModelCount:           len(freeze.Variants),
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		Variants:             variants,
		InferenceVariants:    freeze.InferenceVariants,
	}
	return json.MarshalIndent(payload, "", "  ")
}
