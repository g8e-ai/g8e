// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/httpclient"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// BuildMTLSClient creates an *http.Client configured with CLI mTLS certificates
// and the gateway CA bundle. The timeout parameter controls the client-level
// timeout; pass 0 for context-controlled cancellation (e.g., SSE streaming).
func BuildMTLSClient(fileSvc fs.RuntimeFileService, cfg *config.Config, timeout time.Duration) (*http.Client, error) {
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	caBundleBytes, err := ReadTrustBundle(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBundleBytes) {
		return nil, constants.ErrCAParseFailed
	}

	return &http.Client{
		Transport: httpclient.NewIPv4Transport(&tls.Config{
			Certificates:     []tls.Certificate{cliCert},
			RootCAs:          caPool,
			MinVersion:       tls.VersionTLS13,
			CurvePreferences: certs.FIPSCurvePreferences(),
		}),
		Timeout: timeout,
	}, nil
}
