// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignQueueNextPendingSelectsFirstPendingEntry(t *testing.T) {
	tests := []struct {
		name    string
		queue   *CampaignQueue
		wantTag string
		wantErr error
	}{
		{
			name: "case insensitive first match",
			queue: &CampaignQueue{Models: []CampaignQueueModel{
				{ServedModelTag: "verified:model", Status: "verified"},
				{ServedModelTag: "pending:first", Status: "PENDING"},
				{ServedModelTag: "pending:second", Status: "pending"},
			}},
			wantTag: "pending:first",
		},
		{
			name:    "nil queue",
			wantErr: constants.ErrMissingRequiredField,
		},
		{
			name:    "no pending entry",
			queue:   &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified"}}},
			wantErr: constants.ErrNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry, err := test.queue.NextPending()
			if test.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantTag, entry.ServedModelTag)
		})
	}
}

func TestCampaignQueueFindByTagOrVariantIDResolvesTrimmedQueries(t *testing.T) {
	queue := &CampaignQueue{Models: []CampaignQueueModel{{
		VariantID:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		CampaignID:     "eval-init-qwen3-4b",
	}}}

	tests := []struct {
		name    string
		query   string
		want    string
		wantErr error
	}{
		{name: "served tag", query: "qwen3:4b", want: "eval-init-qwen3-4b"},
		{name: "variant ID", query: " qwen3-4b ", want: "eval-init-qwen3-4b"},
		{name: "blank query", wantErr: constants.ErrMissingRequiredField},
		{name: "unknown model", query: "missing", wantErr: constants.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry, err := queue.FindByTagOrVariantID(test.query)
			if test.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, entry.CampaignID)
		})
	}
}

func TestCampaignIDForVariantUsesCanonicalIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		variant *evalv1.ModelVariant
		want    string
	}{
		{name: "gemma4 compatibility campaign", variant: &evalv1.ModelVariant{ServedModelTag: "gemma4:e4b", VariantId: "gemma4-e4b"}, want: "init-campaign"},
		{name: "standard campaign", variant: &evalv1.ModelVariant{ServedModelTag: "gemma3:4b", VariantId: "gemma3-4b"}, want: "eval-init-gemma3-4b"},
		{name: "nil variant", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, CampaignIDForVariant(test.variant))
		})
	}
}

func TestPrioritizeRolloutIntakeMovesConfiguredVariantsFirst(t *testing.T) {
	variants := []*evalv1.ModelVariant{
		{VariantId: "smollm2-135m", ServedModelTag: "smollm2:135m", ParameterCount: 134_520_000},
		{VariantId: "qwen3-8-27b", ServedModelTag: "qwen3.8:27b", ParameterCount: 27_000_000_000},
		{VariantId: "glm-5-3-air", ServedModelTag: "glm-5.3-air", ParameterCount: 9_000_000_000},
	}
	ordered := PrioritizeRolloutIntake(variants, []string{"glm-5-3-air", "qwen3-8-27b"})
	require.Len(t, ordered, 3)
	assert.Equal(t, "glm-5-3-air", ordered[0].GetVariantId())
	assert.Equal(t, "qwen3-8-27b", ordered[1].GetVariantId())
	assert.Equal(t, "smollm2-135m", ordered[2].GetVariantId())
}

func TestSortModelVariantsForRolloutOrdersByParameterCountAndTag(t *testing.T) {
	variants := []*evalv1.ModelVariant{
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ParameterCount: 4_000_000_000},
		{VariantId: "qwen3-5-9b", ServedModelTag: "qwen3.5:9b", ParameterCount: 9_000_000_000},
		{VariantId: "granite4-2-8b", ServedModelTag: "granite4.2:8b", ParameterCount: 8_000_000_000},
		{VariantId: "qwen3-5-0-8b", ServedModelTag: "qwen3.5:0.8b", ParameterCount: 800_000_000},
		{VariantId: "granite4-2-3b", ServedModelTag: "granite4.2:3b", ParameterCount: 3_000_000_000},
		{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ParameterCount: 4_000_000_000},
	}

	SortModelVariantsForRollout(variants)

	assert.Equal(t, []string{
		"qwen3.5:0.8b",
		"granite4.2:3b",
		"gemma3:4b",
		"qwen3:4b",
		"granite4.2:8b",
		"qwen3.5:9b",
	}, servedModelTags(variants))
}

func TestQueueLogDirAcceptsOneSafeRunName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "queue-run-20260923", want: "eval/logs/queue-run-20260923"},
		{name: "", want: ""},
		{name: "batch/one", want: ""},
		{name: "..", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, QueueLogDir(test.name))
		})
	}
}

func TestSaveAndLoadInitCampaignQueueRoundTrip(t *testing.T) {
	fileSvc := newCampaignQueueFileService()
	want := &CampaignQueue{Models: []CampaignQueueModel{{
		VariantID:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		Status:         "pending",
	}}}

	require.NoError(t, SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath, want))
	got, err := LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

type campaignInventoryPayload struct {
	Variants []json.RawMessage `json:"variants"`
}

func writeCampaignQueue(t *testing.T, fileSvc fs.RuntimeFileService, relPath string, queue *CampaignQueue) {
	t.Helper()
	require.NoError(t, SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, relPath, queue))
}

func writeQueueFrozenVariants(t *testing.T, fileSvc fs.RuntimeFileService, relPath string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	payload := campaignInventoryPayload{Variants: make([]json.RawMessage, 0, len(variants))}
	for _, variant := range variants {
		body, err := protojson.Marshal(variant)
		require.NoError(t, err)
		payload.Variants = append(payload.Variants, body)
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, body, constants.PermFileReadOnly))
}

func servedModelTags(variants []*evalv1.ModelVariant) []string {
	tags := make([]string, 0, len(variants))
	for _, variant := range variants {
		tags = append(tags, variant.GetServedModelTag())
	}
	return tags
}

func variantIDs(variants []*evalv1.ModelVariant) []string {
	ids := make([]string, 0, len(variants))
	for _, variant := range variants {
		ids = append(ids, variant.GetVariantId())
	}
	return ids
}

func queueVariantIDs(queue *CampaignQueue) []string {
	ids := make([]string, 0, len(queue.Models))
	for _, model := range queue.Models {
		ids = append(ids, model.VariantID)
	}
	return ids
}

type campaignQueueFileService struct {
	*campaignMemoryFileService
}

func newCampaignQueueFileService() fs.RuntimeFileService {
	return &campaignQueueFileService{campaignMemoryFileService: newCampaignMemoryFileService()}
}

func (s *campaignQueueFileService) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	body, err := s.campaignMemoryFileService.ReadFile(ctx, relPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, constants.ErrNotFound
	}
	return body, err
}
