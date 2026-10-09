// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package operatorcmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	g8econfig "github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
)

const operatorGatewayPreflightTimeout = 10 * time.Second

// operatorGatewayPreflightCmd checks, from the host or container a worker will
// run in, both Gateway paths the worker dials. operator deploy runs it on each
// target before starting workers so an unreachable Gateway stops the rollout.
func operatorGatewayPreflightCmd() *cobra.Command {
	var httpPort, httpsPort int
	cmd := &cobra.Command{
		Use:   "gateway-preflight <gateway-host>",
		Short: "Verify Gateway trust discovery and TLS reachability from this host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !operatorDeployEndpointPattern.MatchString(args[0]) {
				return fmt.Errorf("%w: gateway host %q must match %s", constants.ErrPathValidation, args[0], operatorDeployEndpointPattern)
			}
			if err := validateGatewayPort("gateway-http-port", httpPort); err != nil {
				return err
			}
			if err := validateGatewayPort("gateway-https-port", httpsPort); err != nil {
				return err
			}
			return runGatewayPreflight(cmd.Context(), args[0], httpPort, httpsPort)
		},
	}
	cmd.Flags().IntVar(&httpPort, "gateway-http-port", 0, "Gateway HTTP discovery port to dial (default: platform default)")
	cmd.Flags().IntVar(&httpsPort, "gateway-https-port", 0, "Gateway HTTPS/mTLS port to dial (default: platform default)")
	return cmd
}

// runGatewayPreflight fetches the live trust bundle over plain HTTP, then
// completes a verified TLS handshake on the HTTPS port, checking the same
// certificate identity a worker dialing host checks. Any HTTP response proves
// the channel; its status is not checked.
func runGatewayPreflight(ctx context.Context, host string, httpPort, httpsPort int) error {
	if httpPort == 0 {
		httpPort = constants.Ports.OperatorHttp
	}
	if httpsPort == 0 {
		httpsPort = constants.Ports.OperatorHttps
	}
	ctx, cancel := context.WithTimeout(ctx, operatorGatewayPreflightTimeout)
	defer cancel()

	discoveryURL := "http://" + net.JoinHostPort(host, strconv.Itoa(httpPort)) + constants.APIPaths.WellKnownPKICABundle
	trust, err := auth.DiscoverLiveTrustBundle(ctx, discoveryURL, time.Now)
	if err != nil {
		return fmt.Errorf("gateway preflight: trust discovery at %s: %w", discoveryURL, err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(trust.BundlePEM)
	client := &http.Client{Transport: httpclient.NewIPv4Transport(&tls.Config{
		RootCAs:    roots,
		MinVersion: tls.VersionTLS13,
		ServerName: g8econfig.TLSServerName(host),
	})}

	healthURL := "https://" + net.JoinHostPort(host, strconv.Itoa(httpsPort)) + constants.APIPaths.Health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("%w: gateway preflight: %w", constants.ErrHTTPRequestCreateFailed, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: gateway preflight: TLS request to %s: %w", constants.ErrHTTPRequestExecuteFailed, healthURL, err)
	}
	return resp.Body.Close()
}
