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

package auth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
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

	caBundleBytes, err := readFileWithFS(fileSvc, cfg.TrustBundlePath())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBundleBytes) {
		return nil, constants.ErrCAParseFailed
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cliCert},
				RootCAs:      caPool,
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: timeout,
	}, nil
}
