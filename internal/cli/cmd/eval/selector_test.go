// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func selectorFixture() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{VariantId: "qwen3-0-6b", ServedModelTag: "qwen3:0.6b", ModelFamily: "qwen3", ParameterCount: 600_000_000},
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelFamily: "qwen3", ParameterCount: 4_000_000_000},
		{VariantId: "gemma3-12b", ServedModelTag: "gemma3:12b", ModelFamily: "gemma3", ParameterCount: 12_000_000_000},
		{VariantId: "mystery", ServedModelTag: "mystery:latest", ModelFamily: "mystery"},
	}
}

func selectedTags(variants []*evalv1.ModelVariant) []string {
	tags := make([]string, 0, len(variants))
	for _, variant := range variants {
		tags = append(tags, variant.GetServedModelTag())
	}
	return tags
}

func TestModelSelectorResolve(t *testing.T) {
	tests := []struct {
		name     string
		selector ModelSelector
		want     []string
		wantErr  error
		wantMsg  string
	}{
		{name: "served tag", selector: ModelSelector{IDs: []string{"qwen3:4b"}}, want: []string{"qwen3:4b"}},
		{name: "variant id", selector: ModelSelector{IDs: []string{"qwen3-4b"}}, want: []string{"qwen3:4b"}},
		{name: "positional order preserved", selector: ModelSelector{IDs: []string{"gemma3:12b", "qwen3-0-6b"}}, want: []string{"gemma3:12b", "qwen3:0.6b"}},
		{name: "tag and variant dedupe", selector: ModelSelector{IDs: []string{"qwen3:4b", "qwen3-4b"}}, want: []string{"qwen3:4b"}},
		{name: "unknown positional", selector: ModelSelector{IDs: []string{"nope:1b"}}, wantErr: constants.ErrInferenceModelNotFound},
		{name: "family", selector: ModelSelector{Family: "qwen3"}, want: []string{"qwen3:0.6b", "qwen3:4b"}},
		{name: "family narrowed by max params", selector: ModelSelector{Family: "qwen3", MaxParams: "1b"}, want: []string{"qwen3:0.6b"}},
		{name: "max params alone keeps unknown size", selector: ModelSelector{MaxParams: "4b"}, want: []string{"qwen3:0.6b", "qwen3:4b", "mystery:latest"}},
		{name: "max params fractional", selector: ModelSelector{MaxParams: "1.5B"}, want: []string{"qwen3:0.6b", "mystery:latest"}},
		{name: "max params millions narrows to nothing", selector: ModelSelector{MaxParams: "350m", Family: "qwen3"}, wantErr: constants.ErrEvaluationSelectionEmpty},
		{name: "all", selector: ModelSelector{All: true}, want: []string{"qwen3:0.6b", "qwen3:4b", "gemma3:12b", "mystery:latest"}},
		{name: "empty selector", selector: ModelSelector{}, wantErr: constants.ErrEvaluationSelectionEmpty},
		{name: "no match is an error", selector: ModelSelector{Family: "llama"}, wantErr: constants.ErrEvaluationSelectionEmpty},
		{name: "positional with family", selector: ModelSelector{IDs: []string{"qwen3:4b"}, Family: "qwen3"}, wantMsg: "cannot be combined"},
		{name: "positional with max params", selector: ModelSelector{IDs: []string{"qwen3:4b"}, MaxParams: "8b"}, wantMsg: "cannot be combined"},
		{name: "positional with all", selector: ModelSelector{IDs: []string{"qwen3:4b"}, All: true}, wantMsg: "cannot be combined"},
		{name: "all with family", selector: ModelSelector{All: true, Family: "qwen3"}, wantMsg: "--all cannot be combined"},
		{name: "all with max params", selector: ModelSelector{All: true, MaxParams: "8b"}, wantMsg: "--all cannot be combined"},
		{name: "zero max params", selector: ModelSelector{MaxParams: "0"}, wantMsg: "greater than zero"},
		{name: "malformed max params", selector: ModelSelector{MaxParams: "big"}, wantMsg: "parse parameter count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.selector.Resolve(selectorFixture())
			switch {
			case test.wantErr != nil:
				require.ErrorIs(t, err, test.wantErr)
			case test.wantMsg != "":
				require.ErrorContains(t, err, test.wantMsg)
			default:
				require.NoError(t, err)
				assert.Equal(t, test.want, selectedTags(got))
			}
		})
	}
}

func TestModelSelectorFilterAllowsUnsetAndEmptyResults(t *testing.T) {
	all, err := ModelSelector{}.Filter(selectorFixture())
	require.NoError(t, err)
	assert.Len(t, all, 4)

	none, err := ModelSelector{Family: "llama"}.Filter(selectorFixture())
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = ModelSelector{All: true, Family: "qwen3"}.Filter(selectorFixture())
	require.Error(t, err)
}

func TestModelSelectorWithArgsTrimsBlanks(t *testing.T) {
	selector := ModelSelector{}.withArgs([]string{" qwen3:4b ", "", "  "})
	assert.Equal(t, []string{"qwen3:4b"}, selector.IDs)
	assert.True(t, selector.IsSet())
	assert.False(t, ModelSelector{}.withArgs([]string{" "}).IsSet())
}

func TestModelSelectorBindFlags(t *testing.T) {
	var selector ModelSelector
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	selector.bindFlags(cmd)
	cmd.SetArgs([]string{"--family", "qwen3", "--max-params", "8b"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "qwen3", selector.Family)
	assert.Equal(t, "8b", selector.MaxParams)
	assert.False(t, selector.All)
	assert.Nil(t, cmd.Flags().Lookup("params"), "--params is removed")
}
