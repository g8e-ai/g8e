// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client is a minimal HTTP client for Ollama model maintenance endpoints.
type Client struct {
	base *url.URL
	http *http.Client
}

// ProgressResponse is one NDJSON progress event from /api/pull.
type ProgressResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// NewClient constructs a client for the given Ollama base endpoint.
func NewClient(base *url.URL, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{base: base, http: httpClient}
}

// pullRequest is the /api/pull body. Stream is explicit so a false value
// is not dropped; Ollama defaults a missing stream flag to true.
type pullRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

// Pull downloads a model from the remote Ollama provider.
func (c *Client) Pull(ctx context.Context, model string, fn func(ProgressResponse) error) error {
	return c.stream(ctx, http.MethodPost, "/api/pull", pullRequest{
		Model:  model,
		Stream: false,
	}, func(line []byte) error {
		var progress ProgressResponse
		if err := json.Unmarshal(line, &progress); err != nil {
			return err
		}
		if progress.Error != "" {
			return fmt.Errorf("ollama pull: %s", progress.Error)
		}
		if fn == nil {
			return nil
		}
		return fn(progress)
	})
}

// Copy creates a model alias from an existing local model.
func (c *Client) Copy(ctx context.Context, source, destination string) error {
	return c.do(ctx, http.MethodPost, "/api/copy", map[string]string{
		"source":      source,
		"destination": destination,
	})
}

func (c *Client) do(ctx context.Context, method, path string, reqData any) error {
	body, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, method, c.base.JoinPath(path).String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return checkResponse(response.StatusCode, respBody)
}

func (c *Client) stream(ctx context.Context, method, path string, reqData any, fn func([]byte) error) error {
	body, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, method, c.base.JoinPath(path).String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		respBody, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		return checkResponse(response.StatusCode, respBody)
	}

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		var progress ProgressResponse
		if err := json.Unmarshal(line, &progress); err != nil {
			return err
		}
		if progress.Error != "" {
			return fmt.Errorf("ollama pull: %s", progress.Error)
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func checkResponse(statusCode int, body []byte) error {
	if statusCode < http.StatusBadRequest {
		return nil
	}
	var apiError struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error != "" {
		return fmt.Errorf("ollama api: %s", apiError.Error)
	}
	return fmt.Errorf("ollama api: status %d", statusCode)
}
