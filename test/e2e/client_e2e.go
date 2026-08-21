// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/httpclient"
)

// dispatchRequestJSON is the typed JSON body for POST
// /api/v1/operators/commands. Mirrors gateway.OperatorCommandRequest. Defined
// locally to keep the E2E package decoupled from internal gateway types.
type dispatchRequestJSON struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	ActionType              string `json:"action_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
}

// dispatchResponseJSON mirrors gateway.DispatchResponse.
type dispatchResponseJSON struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// newE2EClient constructs an E2EClient from the resolved e2eConfig. It loads
// the owner CLI certificate and key from disk, reads the CA bundle from the
// runtime tree, and builds strict mTLS HTTP clients using ServerName derived
// from the validated HTTPS URL. No InsecureSkipVerify — normal Go TLS hostname
// and chain verification is used.
func newE2EClient(ctx context.Context, cfg *e2eConfig) (*E2EClient, error) {
	cliCert, err := tls.LoadX509KeyPair(cfg.cliCertPath, cfg.cliKeyPath)
	if err != nil {
		return nil, fmt.Errorf("e2e: load CLI key pair: %w", err)
	}

	caBundle, err := cfg.readCABundle(ctx)
	if err != nil {
		return nil, fmt.Errorf("e2e: read CA bundle: %w", err)
	}

	caPool, err := parseCAPool(caBundle)
	if err != nil {
		return nil, fmt.Errorf("e2e: parse CA bundle: %w", err)
	}

	serverName, err := extractServerName(cfg.gatewayHTTPSURL)
	if err != nil {
		return nil, fmt.Errorf("e2e: extract server name: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{cliCert},
		RootCAs:          caPool,
		ServerName:       serverName,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
	}

	mtlsClient := &http.Client{
		Transport: httpclient.NewIPv4Transport(tlsConfig),
		Timeout:   defaultClientTimeout,
	}

	publicTLSConfig := &tls.Config{
		RootCAs:          caPool,
		ServerName:       serverName,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
	}

	publicClient := &http.Client{
		Transport: httpclient.NewIPv4Transport(publicTLSConfig),
		Timeout:   defaultClientTimeout,
	}

	return &E2EClient{
		publicClient: publicClient,
		mtlsClient:   mtlsClient,
		cliSessionID: cfg.cliSessionID,
		gatewayHTTPS: cfg.gatewayHTTPSURL,
	}, nil
}