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

	"github.com/g8e-ai/g8e/components/g8eo/certs"
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
		return nil, err
	}

	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

func NewWithTimeout(timeout time.Duration) (*http.Client, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, err
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
		return nil, err
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

func NewWithServerName(serverName string) (*http.Client, error) {
	tlsCfg, err := certs.GetTLSConfig()
	if err != nil {
		return nil, err
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
		return nil, err
	}
	tlsCfg.ServerName = serverName
	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}, nil
}

func MustNew() *http.Client {
	c, err := New()
	if err != nil {
		panic(fmt.Errorf("httpclient: %w", err))
	}
	return c
}

func MustWebSocketDialer() *websocket.Dialer {
	d, err := WebSocketDialer()
	if err != nil {
		panic(fmt.Errorf("httpclient: %w", err))
	}
	return d
}
