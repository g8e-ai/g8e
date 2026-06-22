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

package gateway

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

const (
	jwksHTTPTimeout     = 10 * time.Second
	jwksCacheDuration   = 15 * time.Minute
	jwksKeyTypeRSA      = "RSA"
	jwksKeyUseSignature = "sig"
)

type JWKS struct {
	Keys []JWK `json:"keys"`
}

type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSProvider struct {
	url        string
	httpClient *http.Client
	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastFetch  time.Time
}

func NewJWKSProvider(url string) *JWKSProvider {
	return &JWKSProvider{
		url: url,
		httpClient: &http.Client{
			Timeout: jwksHTTPTimeout,
		},
		keys: make(map[string]*rsa.PublicKey),
	}
}

func (p *JWKSProvider) fetchKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrJWKSRequestCreate, err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrJWKSFetchKeys, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d", constants.ErrJWKSUnexpectedStatus, resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrJWKSDecodeResponse, err)
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty != jwksKeyTypeRSA || key.Use != jwksKeyUseSignature {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}

		n := new(big.Int).SetBytes(nBytes)
		var e int
		for _, b := range eBytes {
			e = (e << 8) | int(b)
		}

		newKeys[key.Kid] = &rsa.PublicKey{
			N: n,
			E: e,
		}
	}

	p.mu.Lock()
	p.keys = newKeys
	p.lastFetch = time.Now()
	p.mu.Unlock()

	return nil
}

func (p *JWKSProvider) GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.mu.RLock()
	key, ok := p.keys[kid]
	lastFetch := p.lastFetch
	p.mu.RUnlock()

	if ok && time.Since(lastFetch) < jwksCacheDuration {
		return key, nil
	}

	if err := p.fetchKeys(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrJWKSFetchKeys, err)
	}

	p.mu.RLock()
	key, ok = p.keys[kid]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", constants.ErrJWKSKeyNotFound, kid)
	}

	return key, nil
}
