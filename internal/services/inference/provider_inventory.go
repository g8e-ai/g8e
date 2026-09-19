// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const providerClassOllama = "ollama"

// ProviderModelInventoryEntry is one discovered provider model identity with
// normalized metadata used to materialize eval ModelVariant records.
type ProviderModelInventoryEntry struct {
	ProviderClass          string
	ServedModelTag         string
	ModelDigest            string
	ModelFamily            string
	ParameterSize          string
	ParameterCount         uint64
	Quantization           string
	Format                 string
	ContextLimit           uint32
	AdvertisedCapabilities []string
}

type ollamaShowResponse struct {
	Parameters   string                     `json:"parameters"`
	Details      ollamaModelDetails         `json:"details"`
	Capabilities []string                   `json:"capabilities"`
	ModelInfo    map[string]json.RawMessage `json:"model_info"`
}

type ollamaModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

var ollamaNumCtxPattern = regexp.MustCompile(`(?m)^num_ctx\s+(\d+)\s*$`)

// ListProviderModelInventory queries every installed Ollama model tag and
// preserves each served tag as a separate inventory entry even when digests match.
func (b *OllamaBackend) ListProviderModelInventory(ctx context.Context) ([]ProviderModelInventoryEntry, error) {
	variants, err := b.ListModelVariants(ctx)
	if err != nil {
		return nil, fmt.Errorf("inference: list provider model inventory: %w", err)
	}
	entries := make([]ProviderModelInventoryEntry, 0, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetModel() == "" {
			continue
		}
		entry, err := b.describeProviderModel(ctx, variant.GetModel(), variant.GetDigest())
		if err != nil {
			return nil, fmt.Errorf("inference: list provider model inventory: model %q: %w", variant.GetModel(), err)
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("inference: list provider model inventory: %w", constants.ErrInferenceModelNotFound)
	}
	return entries, nil
}

func (b *OllamaBackend) describeProviderModel(ctx context.Context, servedTag, digest string) (ProviderModelInventoryEntry, error) {
	show, err := b.showModel(ctx, servedTag)
	if err != nil {
		return ProviderModelInventoryEntry{}, err
	}
	family := strings.TrimSpace(show.Details.Family)
	if family == "" && len(show.Details.Families) > 0 {
		family = strings.TrimSpace(show.Details.Families[0])
	}
	parameterSize := strings.TrimSpace(show.Details.ParameterSize)
	parameterCount, err := parseProviderParameterCount(parameterSize)
	if err != nil {
		return ProviderModelInventoryEntry{}, fmt.Errorf("parse parameter size %q: %w", parameterSize, err)
	}
	contextLimit := parseProviderContextLimit(show.Parameters, show.ModelInfo)
	capabilities := append([]string(nil), show.Capabilities...)
	return ProviderModelInventoryEntry{
		ProviderClass:          providerClassOllama,
		ServedModelTag:         servedTag,
		ModelDigest:            digest,
		ModelFamily:            family,
		ParameterSize:          parameterSize,
		ParameterCount:         parameterCount,
		Quantization:           strings.TrimSpace(show.Details.QuantizationLevel),
		Format:                 strings.TrimSpace(show.Details.Format),
		ContextLimit:           contextLimit,
		AdvertisedCapabilities: capabilities,
	}, nil
}

func (b *OllamaBackend) showModel(ctx context.Context, model string) (*ollamaShowResponse, error) {
	body, err := json.Marshal(map[string]string{"name": model})
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: show model: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL("api", "show"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: show model: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, transportError("show model", ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("ollama_backend: show model: %w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}
	var show ollamaShowResponse
	if err := b.decodeResponse("show model", resp.Body, &show); err != nil {
		return nil, err
	}
	return &show, nil
}

func parseProviderParameterCount(raw string) (uint64, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0, nil
	}
	var multiplier uint64
	switch {
	case strings.HasSuffix(value, "b"):
		multiplier = 1_000_000_000
		value = strings.TrimSuffix(value, "b")
	case strings.HasSuffix(value, "m"):
		multiplier = 1_000_000
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "k"):
		multiplier = 1_000
		value = strings.TrimSuffix(value, "k")
	default:
		return 0, fmt.Errorf("unsupported parameter size suffix")
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("parameter size must be non-negative")
	}
	return uint64(parsed * float64(multiplier)), nil
}

func parseProviderContextLimit(parameters string, modelInfo map[string]json.RawMessage) uint32 {
	if match := ollamaNumCtxPattern.FindStringSubmatch(parameters); len(match) == 2 {
		if parsed, err := strconv.ParseUint(match[1], 10, 32); err == nil {
			return uint32(parsed)
		}
	}
	for _, key := range []string{".context_length", ".block_count"} {
		raw, ok := modelInfo[key]
		if !ok {
			continue
		}
		var parsed uint64
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed > 0 && parsed <= uint64(^uint32(0)) {
			return uint32(parsed)
		}
	}
	return 0
}

// NormalizeProviderModelVariantID derives a stable variant ID from the served tag.
func NormalizeProviderModelVariantID(servedTag string) string {
	slug := strings.ToLower(servedTag)
	slug = strings.NewReplacer("/", "-", ":", "-", ".", "-", "_", "-").Replace(slug)
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}

// ProviderInventoryDigest computes the campaign registry digest for inventory entries.
func ProviderInventoryDigest(campaignID string, entries []ProviderModelInventoryEntry) (string, error) {
	variants := make([]*operatorv1.InferenceModelVariant, 0, len(entries))
	for _, entry := range entries {
		if entry.ServedModelTag == "" || entry.ModelDigest == "" {
			return "", fmt.Errorf("inference: provider inventory digest: %w", constants.ErrMissingRequiredField)
		}
		variants = append(variants, &operatorv1.InferenceModelVariant{
			Model:  entry.ServedModelTag,
			Digest: entry.ModelDigest,
		})
	}
	return models.ComputeInferenceModelRegistryDigest(campaignID, variants)
}
