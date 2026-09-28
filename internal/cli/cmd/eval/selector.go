// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ModelSelector is the one model selection grammar shared by every eval
// command that operates on a set of models: positional served tags or variant
// IDs, or the --family, --max-params, and --all filters.
//
// Positional IDs are mutually exclusive with every filter flag. --all is
// mutually exclusive with --family and --max-params. --max-params may narrow
// --family and may also stand alone.
type ModelSelector struct {
	IDs       []string
	Family    string
	MaxParams string
	All       bool
}

// bindFlags registers the filter flags on cmd.
func (s *ModelSelector) bindFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.Family, "family", "", "Select models by family (for example gemma4, granite)")
	cmd.Flags().StringVar(&s.MaxParams, "max-params", "", "Select models with at most this many parameters (for example 12b, 1.5B, 350m)")
	cmd.Flags().BoolVar(&s.All, "all", false, "Select every model in scope")
}

// withArgs returns a copy of the selector carrying the positional IDs.
func (s ModelSelector) withArgs(args []string) ModelSelector {
	s.IDs = normalizeSelectorIDs(args)
	return s
}

// IsSet reports whether the selector names anything.
func (s ModelSelector) IsSet() bool {
	return len(s.IDs) > 0 || s.Family != "" || s.MaxParams != "" || s.All
}

func (s ModelSelector) validate() error {
	hasFilter := s.Family != "" || s.MaxParams != ""
	if len(s.IDs) > 0 && (hasFilter || s.All) {
		return fmt.Errorf("evaluation: model selector: positional models cannot be combined with --family, --max-params, or --all")
	}
	if s.All && hasFilter {
		return fmt.Errorf("evaluation: model selector: --all cannot be combined with --family or --max-params")
	}
	return nil
}

func (s ModelSelector) maxParameters() (uint64, error) {
	if s.MaxParams == "" {
		return 0, nil
	}
	maxParams, err := evaluation.ParseParameterCount(s.MaxParams)
	if err != nil {
		return 0, err
	}
	if maxParams == 0 {
		return 0, fmt.Errorf("evaluation: model selector: --max-params must be greater than zero")
	}
	return maxParams, nil
}

// Resolve applies the selector to variants. An empty selector and an empty
// result are both errors wrapping constants.ErrEvaluationSelectionEmpty.
func (s ModelSelector) Resolve(variants []*evalv1.ModelVariant) ([]*evalv1.ModelVariant, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if !s.IsSet() {
		return nil, fmt.Errorf("evaluation: model selector: name models or pass --family, --max-params, or --all: %w", constants.ErrEvaluationSelectionEmpty)
	}
	selected, err := s.apply(variants)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("evaluation: model selector: no models matched: %w", constants.ErrEvaluationSelectionEmpty)
	}
	return selected, nil
}

// Filter applies the selector as a listing filter. An unset selector returns
// every variant and an empty result is not an error.
func (s ModelSelector) Filter(variants []*evalv1.ModelVariant) ([]*evalv1.ModelVariant, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if !s.IsSet() {
		return variants, nil
	}
	return s.apply(variants)
}

func (s ModelSelector) apply(variants []*evalv1.ModelVariant) ([]*evalv1.ModelVariant, error) {
	if len(s.IDs) > 0 {
		return selectVariantsByID(variants, s.IDs)
	}
	maxParams, err := s.maxParameters()
	if err != nil {
		return nil, err
	}
	selected := append([]*evalv1.ModelVariant(nil), variants...)
	if s.Family != "" {
		selected = evaluation.FilterVariantsByFamily(selected, s.Family)
	}
	if maxParams > 0 {
		selected = evaluation.FilterVariantsByMaxParameters(selected, maxParams)
	}
	return selected, nil
}

func selectVariantsByID(variants []*evalv1.ModelVariant, ids []string) ([]*evalv1.ModelVariant, error) {
	selected := make([]*evalv1.ModelVariant, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		var match *evalv1.ModelVariant
		for _, variant := range variants {
			if variant != nil && (variant.GetServedModelTag() == id || variant.GetVariantId() == id) {
				match = variant
				break
			}
		}
		if match == nil {
			return nil, fmt.Errorf("evaluation: model selector: %w: %s", constants.ErrInferenceModelNotFound, id)
		}
		if _, dup := seen[match.GetVariantId()]; dup {
			continue
		}
		seen[match.GetVariantId()] = struct{}{}
		selected = append(selected, match)
	}
	return selected, nil
}

func normalizeSelectorIDs(args []string) []string {
	ids := make([]string, 0, len(args))
	for _, arg := range args {
		if arg = strings.TrimSpace(arg); arg != "" {
			ids = append(ids, arg)
		}
	}
	return ids
}
