// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	rfs "github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// modelScope names one of the two model-state stores an operator works with.
//
//	catalog   the checked-in inventory of models the project knows about
//	registry  the runtime inventory of frozen models that runs bind to
type modelScope string

const (
	modelScopeCatalog  modelScope = "catalog"
	modelScopeRegistry modelScope = "registry"
	modelScopeAll      modelScope = "all"
)

func parseModelScope(raw string) (modelScope, error) {
	switch scope := modelScope(raw); scope {
	case modelScopeCatalog, modelScopeRegistry, modelScopeAll:
		return scope, nil
	default:
		return "", fmt.Errorf("evaluation: --scope must be catalog, registry, or all (got %q)", raw)
	}
}

// modelInventories reads and writes the two model scopes. The registry lives
// under the runtime tree and goes through RuntimeFileService. The catalog is a
// checked-in repository file addressed from the project root.
type modelInventories struct {
	fileSvc     rfs.RuntimeFileService
	projectRoot string
}

func newModelInventories(fileSvc rfs.RuntimeFileService, projectRoot string) modelInventories {
	return modelInventories{fileSvc: fileSvc, projectRoot: projectRoot}
}

func (m modelInventories) catalogPath() string {
	return filepath.Join(m.projectRoot, evaluation.DefaultBaseModelInventoryRelPath)
}

// load reads one scope. A scope that has not been written yet wraps
// constants.ErrNotFound.
func (m modelInventories) load(ctx context.Context, scope modelScope) (*evaluation.ModelInventoryFreeze, error) {
	switch scope {
	case modelScopeCatalog:
		freeze, err := evaluation.LoadModelInventoryFreezeFile(m.catalogPath())
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("evaluation: model catalog %s: %w", evaluation.DefaultBaseModelInventoryRelPath, constants.ErrNotFound)
			}
			return nil, err
		}
		return freeze, nil
	case modelScopeRegistry:
		exists, err := m.fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath)
		if err != nil {
			return nil, fmt.Errorf("evaluation: check model registry: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("evaluation: model registry is not frozen (run `g8e eval models freeze` or `g8e eval models import`): %w", constants.ErrNotFound)
		}
		return evaluation.LoadModelInventoryFreezeFromRuntime(ctx, m.fileSvc, evaluation.DefaultModelInventoryRelPath)
	default:
		return nil, fmt.Errorf("evaluation: model scope %q is not a single store", scope)
	}
}

// variants returns the variants of one scope for read-only use. A scope that
// has not been written yet is empty rather than an error, and a hand-edited
// inventory without a registry digest still lists.
func (m modelInventories) variants(ctx context.Context, scope modelScope) ([]*evalv1.ModelVariant, error) {
	switch scope {
	case modelScopeCatalog:
		variants, err := evaluation.LoadFrozenVariantsFromExternalSource(m.catalogPath())
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return variants, err
	case modelScopeRegistry:
		exists, err := m.fileSvc.FileExists(ctx, evaluation.DefaultModelInventoryRelPath)
		if err != nil {
			return nil, fmt.Errorf("evaluation: check model registry: %w", err)
		}
		if !exists {
			return nil, nil
		}
		return evaluation.LoadFrozenVariantsFromRuntime(ctx, m.fileSvc, evaluation.DefaultModelInventoryRelPath)
	default:
		return nil, fmt.Errorf("evaluation: model scope %q is not a single store", scope)
	}
}

// freezeOrEmpty returns the scope's freeze, or an empty freeze bound to the
// genesis campaign ID when the scope has not been written yet.
func (m modelInventories) freezeOrEmpty(ctx context.Context, scope modelScope) (*evaluation.ModelInventoryFreeze, error) {
	freeze, err := m.load(ctx, scope)
	if err == nil {
		return freeze, nil
	}
	if !errors.Is(err, constants.ErrNotFound) {
		return nil, err
	}
	return &evaluation.ModelInventoryFreeze{CampaignID: evaluation.DefaultGenesisHomogeneousCampaignID}, nil
}

// save writes one scope. An empty freeze removes the registry and is rejected
// for the catalog.
func (m modelInventories) save(ctx context.Context, scope modelScope, freeze *evaluation.ModelInventoryFreeze) error {
	if freeze == nil || len(freeze.Variants) == 0 {
		if scope == modelScopeRegistry {
			return m.fileSvc.Remove(ctx, evaluation.DefaultModelInventoryRelPath)
		}
		return fmt.Errorf("evaluation: cannot leave the model %s empty: %w", scope, constants.ErrEvaluationSelectionEmpty)
	}
	payload, err := modelInventoryFreezeJSON(freeze)
	if err != nil {
		return err
	}
	switch scope {
	case modelScopeRegistry:
		if err := m.fileSvc.MkdirAll(ctx, constants.EvaluationDirname, constants.PermDirStandard); err != nil {
			return fmt.Errorf("evaluation: write model registry: %w", err)
		}
		if err := m.fileSvc.WriteFile(ctx, evaluation.DefaultModelInventoryRelPath, payload, constants.PermFileReadOnly); err != nil {
			return fmt.Errorf("evaluation: write model registry: %w", err)
		}
		return nil
	case modelScopeCatalog:
		return writeCatalogInventory(m.catalogPath(), payload)
	default:
		return fmt.Errorf("evaluation: model scope %q is not a single store", scope)
	}
}

// writeCatalogInventory replaces the checked-in catalog atomically. The
// catalog is a repository file outside the runtime tree, so it is addressed
// from the project root rather than through RuntimeFileService.
func writeCatalogInventory(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "model-inventory-*.tmp")
	if err != nil {
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("evaluation: write model catalog: %w", err)
	}
	return nil
}
