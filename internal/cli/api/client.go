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
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

type Client struct {
	httpClient *http.Client
	cfg        *config.Config
	creds      *auth.Credentials
	baseURL    string // Optional override for testing
}

func NewClient(fileSvc fs.RuntimeFileService, cfg *config.Config) (*Client, error) {
	return NewClientWithURL(fileSvc, cfg, "")
}

// NewClientWithURL creates a client with an optional base URL override for testing.
// If baseURL is empty, it uses cfg.OperatorHTTPURL().
func NewClientWithURL(fileSvc fs.RuntimeFileService, cfg *config.Config, baseURL string) (*Client, error) {
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}

	if creds == nil {
		return nil, constants.ErrNotAuthenticated
	}

	cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	trustBundle, err := auth.ReadTrustBundle(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(trustBundle) {
		return nil, constants.ErrFailedToParseTrustBundle
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
		Timeout: 5 * time.Second,
	}

	return &Client{
		httpClient: httpClient,
		cfg:        cfg,
		creds:      creds,
		baseURL:    baseURL,
	}, nil
}

func (c *Client) DoRequest(method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestMarshalFailed, err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = c.cfg.OperatorHTTPURL()
	}
	url := baseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set(constants.HeaderOperatorSessionID, c.creds.OperatorSessionID)
	req.Header.Set(constants.HeaderCLISessionID, c.creds.CLISessionID)
	req.Header.Set(constants.HeaderUserID, c.creds.UserID)
	req.Header.Set(constants.HeaderOperatorID, c.creds.OperatorID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status %d: %s", constants.ErrHTTPStatusError, resp.StatusCode, string(respBody))
	}

	// Validate response is valid JSON
	if !json.Valid(respBody) {
		return nil, fmt.Errorf("%w: %s", constants.ErrInvalidJSONResponse, string(respBody))
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
