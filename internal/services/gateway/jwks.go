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
		return fmt.Errorf("jwks: create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: fetch keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: unexpected status code: %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("jwks: decode response: %w", err)
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
		return nil, fmt.Errorf("jwks: fetch keys: %w", err)
	}

	p.mu.RLock()
	key, ok = p.keys[kid]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("jwks: key not found: %s", kid)
	}

	return key, nil
}
