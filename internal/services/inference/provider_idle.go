// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const (
	defaultProviderIdlePollInterval = 2 * time.Second
	defaultProviderSettleDuration   = 5 * time.Second
)

// ProviderIdleOptions configures polling against the remote Ollama provider
// before the campaign controller submits the next scored assignment.
type ProviderIdleOptions struct {
	Endpoint       string
	PollInterval   time.Duration
	SettleDuration time.Duration
	HTTPClient     *http.Client
}

// WaitForProviderIdle blocks until the remote provider appears quiescent.
// It polls Ollama /api/ps and requires the response body to remain unchanged
// for SettleDuration so model load/unload or in-flight generation can finish
// before the next assignment starts.
func WaitForProviderIdle(ctx context.Context, opts ProviderIdleOptions) error {
	if opts.Endpoint == "" {
		return fmt.Errorf("inference: wait for provider idle: %w", constants.ErrInferenceEndpointInvalid)
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultProviderIdlePollInterval
	}
	if opts.SettleDuration <= 0 {
		opts.SettleDuration = defaultProviderSettleDuration
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: ProviderStatusTimeout}
	}
	psURL, err := providerPSURL(opts.Endpoint)
	if err != nil {
		return fmt.Errorf("inference: wait for provider idle: %w", err)
	}

	var (
		lastDigest string
		stableAt   time.Time
	)
	for {
		digest, err := fetchProviderPSDigest(ctx, client, psURL)
		if err != nil {
			select {
			case <-ctx.Done():
				return fmt.Errorf("inference: wait for provider idle: %w", ctx.Err())
			case <-time.After(opts.PollInterval):
				continue
			}
		}
		now := time.Now()
		if digest != lastDigest {
			lastDigest = digest
			stableAt = now
		} else if !stableAt.IsZero() && now.Sub(stableAt) >= opts.SettleDuration {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("inference: wait for provider idle: %w", ctx.Err())
		case <-time.After(opts.PollInterval):
		}
	}
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

func fetchProviderPSDigest(ctx context.Context, client *http.Client, psURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, psURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInferenceBackendUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInferenceProviderResponseInvalid, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
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
