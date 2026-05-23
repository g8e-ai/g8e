// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/g8e-ai/g8e/cmd/g8e/internal/auth"
	"github.com/g8e-ai/g8e/cmd/g8e/internal/config"
)

type Client struct {
	httpClient *http.Client
	cfg        *config.Config
	creds      *auth.Credentials
}

func NewClient(cfg *config.Config) (*Client, error) {
	creds, err := auth.LoadCredentials(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	if creds == nil {
		return nil, fmt.Errorf("not authenticated - run ./g8e login first")
	}

	cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	trustBundle, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return nil, fmt.Errorf("failed to read trust bundle: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(trustBundle) {
		return nil, fmt.Errorf("failed to parse trust bundle")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 30 * time.Second,
	}

	return &Client{
		httpClient: httpClient,
		cfg:        cfg,
		creds:      creds,
	}, nil
}

func (c *Client) DoRequest(method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	url := c.cfg.OperatorHTTPURL() + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("X-Operator-Session-ID", c.creds.OperatorSessionID)
	req.Header.Set("X-CLI-Session-ID", c.creds.CLISessionID)
	req.Header.Set("X-User-ID", c.creds.UserID)
	req.Header.Set("X-Operator-ID", c.creds.OperatorID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) Get(path string) ([]byte, error) {
	return c.DoRequest("GET", path, nil)
}

func (c *Client) Post(path string, body interface{}) ([]byte, error) {
	return c.DoRequest("POST", path, body)
}

func (c *Client) Put(path string, body interface{}) ([]byte, error) {
	return c.DoRequest("PUT", path, body)
}

func (c *Client) Delete(path string) ([]byte, error) {
	return c.DoRequest("DELETE", path, nil)
}
