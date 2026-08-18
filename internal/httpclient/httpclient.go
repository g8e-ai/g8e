// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/internal/certs"
)

const (
	DefaultTimeout         = 30 * time.Second
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

// IPv4DialContext is an http.Transport.DialContext function that resolves
// the host using IPv4-only lookup (no AAAA records) and dials over "tcp4".
//
// This forces `localhost` to resolve to 127.0.0.1 on Windows, where the OS
// resolver returns ::1 first and the IDE's port-forward only listens on
// IPv4 127.0.0.1. Even if an IPv6 address somehow slips through the
// lookup, the "tcp4" network prevents the kernel from creating an IPv6
// socket. Literal IPv6 addresses (e.g. "[::1]") fail the ip4 lookup and
// return an error — IPv6 is excluded entirely, not merely deprioritized.
//
// Use this as the DialContext of any CLI HTTP transport that dials the
// gateway via `localhost`. The URL retains `localhost` (for TLS SAN
// verification against the gateway cert) while the dialer connects to
// 127.0.0.1 under the hood.
func IPv4DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ipv4 dial: split host/port %q: %w", addr, err)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, fmt.Errorf("ipv4 dial: resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ipv4 dial: no IPv4 address for %s", host)
	}
	d := &net.Dialer{Timeout: DefaultDialTimeout}
	return d.DialContext(ctx, "tcp4", net.JoinHostPort(ips[0].String(), port))
}

// NewIPv4Transport returns an *http.Transport that dials IPv4 only (see
// IPv4DialContext) with the same timeout/conn-pool tuning as the base
// transport. The optional tlsCfg is set as TLSClientConfig. This is the
// opt-in IPv4 transport for CLI HTTP clients that reach the gateway via
// `localhost`; gateway-side and operator-side transports keep using
// newBaseTransport unless they explicitly need IPv4 restriction.
func NewIPv4Transport(tlsCfg *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig:     tlsCfg,
		DialContext:         IPv4DialContext,
		TLSHandshakeTimeout: DefaultTLSTimeout,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
	}
}

// NewWithTLSConfig creates an HTTP client using the provided TLSConfig (DI pattern).
func NewWithTLSConfig(tlsConfig *certs.TLSConfig) (*http.Client, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}

	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

func NewWithTLS(tlsCfg *tls.Config) *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}
}

func WebSocketDialerWithTLS(tlsCfg *tls.Config) *websocket.Dialer {
	return &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: DefaultTLSTimeout,
	}
}

// NewWithTLSConfigAndServerName creates an HTTP client using the provided TLSConfig and server name (DI pattern).
func NewWithTLSConfigAndServerName(tlsConfig *certs.TLSConfig, serverName string) (*http.Client, error) {
	tlsCfg, err := tlsConfig.GetTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("httpclient: get TLS config: %w", err)
	}
	tlsCfg.ServerName = serverName
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: newBaseTransport(tlsCfg),
	}, nil
}

// ExtractErrorMessage returns a human-readable error string from a raw JSON
// `error` field produced by client, accepting either:
//   - a plain JSON string: "some error"
//   - the standard client error envelope object: {"code": "...", "message": "...", ...}
//
// g8eo HTTP response structs should model `error` as json.RawMessage rather
// than `string`, and call this helper when surfacing the error to the user.
// Modeling it as a bare `string` causes a silent decode failure whenever the
// server returns the object form, masking the real server error.
func ExtractErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Message != "" && obj.Code != "" {
			return fmt.Sprintf("%s: %s", obj.Code, obj.Message)
		}
		if obj.Message != "" {
			return obj.Message
		}
	}
	return string(raw)
}
