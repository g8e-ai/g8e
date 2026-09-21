// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const (
	defaultProviderResidencyPollInterval = 2 * time.Second
	maxProviderResidencyResponseBytes    = 1 << 20
)

// ProviderResidencyModel is one model currently resident in the provider.
type ProviderResidencyModel struct {
	Name string `json:"name"`
}

// ProviderResidency is the typed Ollama /api/ps response used for residency
// postconditions. It does not claim that a provider is idle or that no request
// can start between samples.
type ProviderResidency struct {
	Models []ProviderResidencyModel `json:"models"`
}

// ProviderResidencyOptions configures typed polling against the remote
// provider.
type ProviderResidencyOptions struct {
	Endpoint     string
	PollInterval time.Duration
	HTTPClient   *http.Client
}

// ReadProviderResidency reads and validates one bounded typed /api/ps response.
func ReadProviderResidency(ctx context.Context, opts ProviderResidencyOptions) (ProviderResidency, error) {
	if opts.Endpoint == "" {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w", constants.ErrInferenceEndpointInvalid)
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: ProviderStatusTimeout}
	}
	psURL, err := providerPSURL(opts.Endpoint)
	if err != nil {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, psURL, nil)
	if err != nil {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: %w", constants.ErrInferenceBackendUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderResidencyResponseBytes+1))
	if err != nil {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: %w", constants.ErrInferenceProviderResponseInvalid, err)
	}
	if len(body) > maxProviderResidencyResponseBytes {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: response exceeds %d bytes", constants.ErrInferenceProviderResponseInvalid, maxProviderResidencyResponseBytes)
	}
	var residency ProviderResidency
	if err := json.Unmarshal(body, &residency); err != nil {
		return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: %w", constants.ErrInferenceProviderResponseInvalid, err)
	}
	for _, model := range residency.Models {
		if model.Name == "" {
			return ProviderResidency{}, fmt.Errorf("inference: read provider residency: %w: model name is required", constants.ErrInferenceProviderResponseInvalid)
		}
	}
	return residency, nil
}

// WaitForProviderModelsAbsent waits for every named model to be absent from a
// typed provider residency response. Stable non-empty responses never satisfy
// this postcondition.
func WaitForProviderModelsAbsent(ctx context.Context, opts ProviderResidencyOptions, modelTags []string) error {
	if len(modelTags) == 0 {
		return nil
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultProviderResidencyPollInterval
	}
	for {
		residency, err := ReadProviderResidency(ctx, opts)
		if err != nil {
			return fmt.Errorf("inference: wait for provider models absent: %w", err)
		}
		if providerModelsAbsent(residency, modelTags) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("inference: wait for provider models absent: %w", ctx.Err())
		case <-time.After(opts.PollInterval):
		}
	}
}

func providerModelsAbsent(residency ProviderResidency, modelTags []string) bool {
	for _, resident := range residency.Models {
		for _, wanted := range modelTags {
			if resident.Name == wanted {
				return false
			}
		}
	}
	return true
}

func providerPSURL(endpoint string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInferenceEndpointInvalid, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return "", fmt.Errorf("%q: %w", endpoint, constants.ErrInferenceEndpointInvalid)
	}
	path, err := url.JoinPath(stringsTrimRightSlash(base.Path), "api", "ps")
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInferenceEndpointInvalid, err)
	}
	base.Path = path
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func stringsTrimRightSlash(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	for len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return path
}
