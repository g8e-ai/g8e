package gateway

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
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
			Timeout: 10 * time.Second,
		},
		keys: make(map[string]*rsa.PublicKey),
	}
}

func (p *JWKSProvider) fetchKeys() error {
	req, err := http.NewRequest(http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code fetching JWKS: %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" || key.Use != "sig" {
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

func (p *JWKSProvider) GetKey(kid string) (*rsa.PublicKey, error) {
	p.mu.RLock()
	key, ok := p.keys[kid]
	lastFetch := p.lastFetch
	p.mu.RUnlock()

	if ok && time.Since(lastFetch) < 15*time.Minute {
		return key, nil
	}

	// Fetch keys if missing or cache expired
	if err := p.fetchKeys(); err != nil {
		// If fetch fails but we have an old key, return it
		if ok {
			return key, nil
		}
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	p.mu.RLock()
	key, ok = p.keys[kid]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key %s not found in JWKS", kid)
	}

	return key, nil
}
