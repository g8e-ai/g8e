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

package httpclient

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/internal/certs"
)

const (
	DefaultTimeout         = 30 * time.Second
	DefaultShortTimeout    = 10 * time.Second
	DefaultDialTimeout     = 10 * time.Second
	DefaultTLSTimeout      = 10 * time.Second
	DefaultIdleConnTimeout = 90 * time.Second
)

func newBaseTransport(tlsCfg *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig: tlsCfg,
		DialContext: (&net.Dialer{
			Timeout: DefaultDialTimeout,
		}).DialContext,
		TLSHandshakeTimeout: DefaultTLSTimeout,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
	}
}

func New() (*http.Client, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}

	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

// NewWithTLSConfig creates an HTTP client using the provided TLSConfig (DI pattern).
// This is the preferred constructor for new code using dependency injection.
func NewWithTLSConfig(tlsConfig *certs.TLSConfig) (*http.Client, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config from TLSConfig: %w", err)
	}

	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

func NewWithTimeout(timeout time.Duration) (*http.Client, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

// NewWithTLSConfigAndTimeout creates an HTTP client using the provided TLSConfig and timeout (DI pattern).
func NewWithTLSConfigAndTimeout(tlsConfig *certs.TLSConfig, timeout time.Duration) (*http.Client, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config from TLSConfig: %w", err)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

func NewWithTLS(tlsCfg *tls.Config) *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}
}

func WebSocketDialer() (*websocket.Dialer, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}

	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}, nil
}

// WebSocketDialerWithTLSConfig creates a WebSocket dialer using the provided TLSConfig (DI pattern).
func WebSocketDialerWithTLSConfig(tlsConfig *certs.TLSConfig) (*websocket.Dialer, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config from TLSConfig: %w", err)
	}

	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}, nil
}

func WebSocketDialerWithTLS(tlsCfg *tls.Config) *websocket.Dialer {
	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}
}

func MustNew() *http.Client {
	client, err := New()
	if err != nil {
		panic(fmt.Sprintf("httpclient: %v", err))
	}
	return client
}

func MustWebSocketDialer() *websocket.Dialer {
	dialer, err := WebSocketDialer()
	if err != nil {
		panic(fmt.Sprintf("httpclient: %v", err))
	}
	return dialer
}

func NewWithServerName(serverName string) (*http.Client, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}
	tlsCfg.ServerName = serverName
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

// NewWithTLSConfigAndServerName creates an HTTP client using the provided TLSConfig and server name (DI pattern).
func NewWithTLSConfigAndServerName(tlsConfig *certs.TLSConfig, serverName string) (*http.Client, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config from TLSConfig: %w", err)
	}
	tlsCfg.ServerName = serverName
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

func WebSocketDialerWithServerName(serverName string) (*websocket.Dialer, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}
	tlsCfg.ServerName = serverName
	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}, nil
}

// WebSocketDialerWithTLSConfigAndServerName creates a WebSocket dialer using the provided TLSConfig and server name (DI pattern).
func WebSocketDialerWithTLSConfigAndServerName(tlsConfig *certs.TLSConfig, serverName string) (*websocket.Dialer, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config from TLSConfig: %w", err)
	}
	tlsCfg.ServerName = serverName
	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}, nil
}

