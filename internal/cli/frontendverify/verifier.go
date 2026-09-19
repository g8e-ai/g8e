// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package frontendverify provides a read-only verifier that checks HTTPS
// health and CORS preflight against a running Gateway. It accepts a typed
// frontend origin, local API URL, and validated root pool, and returns a
// typed ordered report. The TLS client is always backed by the validated
// root pool — it never uses InsecureSkipVerify.
//
// The verifier is a single-purpose read operation: it never starts, stops,
// restarts, or reconfigures the Gateway. The command layer (gw connect)
// owns process lifecycle and prompting.
package frontendverify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// CheckName identifies a single verification step in the ordered report.
type CheckName string

const (
	CheckHTTPSHealth      CheckName = "https_health"
	CheckCertificateChain CheckName = "certificate_chain"
	CheckCORSOrigin       CheckName = "cors_origin"
	CheckCORSCredentials  CheckName = "cors_credentials"
	CheckCORSMethods      CheckName = "cors_methods"
	CheckCORSHeaders      CheckName = "cors_headers"
	CheckCORSVary         CheckName = "cors_vary"
)

// CheckStatus reports the outcome of a single verification step.
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckFail CheckStatus = "fail"
)

// CheckResult is the outcome of a single verification step. Detail is a
// human-readable explanation of the result; on failure it identifies the
// responsible layer (input, process state, certificate chain, system
// trust, CORS, etc.).
type CheckResult struct {
	Name   CheckName
	Status CheckStatus
	Detail string
}

// Report is the ordered result of a verification run. Checks are appended
// in deterministic order: HTTPS health, certificate chain, CORS origin,
// CORS credentials, CORS methods, CORS headers, CORS Vary. AllPassed is
// false when any check failed; the first failure is the root-cause layer.
type Report struct {
	Checks       []CheckResult
	AllPassed    bool
	FirstFailure *CheckResult
}

// VerifyOptions configures a verification run.
type VerifyOptions struct {
	// FrontendOrigin is the parsed, canonicalized browser origin that
	// the Gateway's CORS middleware must accept.
	FrontendOrigin browserorigin.Origin

	// APIURL is the full HTTPS URL of the Gateway's health endpoint,
	// e.g. https://localhost:8443/api/v1/health.
	APIURL string

	// RootPool is the validated x509 root pool built from the live CA
	// bundle. The TLS client trusts only these roots — never
	// InsecureSkipVerify.
	RootPool *x509.CertPool

	// Timeout is the per-request timeout for HTTPS health and CORS
	// preflight checks. Defaults to 15 seconds when zero.
	Timeout time.Duration
}

// Verifier performs read-only HTTPS health and CORS preflight checks
// against a running Gateway. It holds no mutable state and is safe for
// concurrent use.
type Verifier struct {
	// httpClientFactory builds an *http.Client backed by the provided
	// root pool. The default implementation creates a client with a TLS
	// config that trusts only the given roots. Tests inject a factory
	// that returns a client targeting an httptest.Server.
	httpClientFactory func(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error)
}

// VerifierDeps holds the injectable dependencies for a Verifier. Fields
// left nil get production defaults.
type VerifierDeps struct {
	HTTPClientFactory func(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error)
}

// NewVerifier constructs a Verifier from the given deps. Nil fields get
// production defaults.
func NewVerifier(deps VerifierDeps) *Verifier {
	v := &Verifier{}
	if deps.HTTPClientFactory != nil {
		v.httpClientFactory = deps.HTTPClientFactory
	} else {
		v.httpClientFactory = defaultHTTPClientFactory
	}
	return v
}

// defaultHTTPClientFactory builds an *http.Client with a TLS config that
// trusts only the given root pool. The transport uses the IPv4-only
// dialer so localhost resolves to 127.0.0.1 on Windows.
func defaultHTTPClientFactory(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error) {
	tlsConfig := &tls.Config{
		RootCAs: rootPool,
	}
	transport := httpclient.NewIPv4Transport(tlsConfig)
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

// Verify performs the ordered HTTPS health and CORS preflight checks
// against the running Gateway. It returns a typed Report with each check
// result in deterministic order. The first failure identifies the
// responsible layer.
//
// The verifier never uses InsecureSkipVerify. The TLS client trusts only
// the roots in opts.RootPool, which the caller validated from the live
// CA bundle.
func (v *Verifier) Verify(ctx context.Context, opts VerifyOptions) (Report, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	client, err := v.httpClientFactory(opts.RootPool, timeout)
	if err != nil {
		return Report{}, fmt.Errorf("%w: build TLS client: %w", constants.ErrHTTPSCertificateVerification, err)
	}

	report := Report{AllPassed: true}

	// 1. HTTPS health check.
	healthResult := v.checkHTTPSHealth(ctx, client, opts.APIURL)
	report.Checks = append(report.Checks, healthResult)
	if healthResult.Status == CheckFail {
		report.AllPassed = false
		report.FirstFailure = &healthResult
		// Certificate chain failure is embedded in the health check
		// (TLS handshake failure). No point continuing to CORS.
		return report, nil
	}

	// 2. Certificate chain check. The HTTPS health check already
	// performed a TLS handshake against the root pool, so a pass here
	// means the served certificate chains to a trusted root. We record
	// this as a separate check for diagnostic clarity.
	chainResult := CheckResult{
		Name:   CheckCertificateChain,
		Status: CheckPass,
		Detail: "served certificate chains to a validated root anchor",
	}
	report.Checks = append(report.Checks, chainResult)

	// 3-7. CORS preflight checks.
	corsResults := v.checkCORS(ctx, client, opts.APIURL, opts.FrontendOrigin)
	for _, r := range corsResults {
		report.Checks = append(report.Checks, r)
		if r.Status == CheckFail && report.AllPassed {
			report.AllPassed = false
			report.FirstFailure = &r
		}
	}

	return report, nil
}

// checkHTTPSHealth requests the HTTPS health endpoint and requires a
// 200 response with the expected HealthResponse shape. A TLS handshake
// failure is reported as a certificate-chain failure detail.
func (v *Verifier) checkHTTPSHealth(ctx context.Context, client *http.Client, healthURL string) CheckResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return CheckResult{
			Name:   CheckHTTPSHealth,
			Status: CheckFail,
			Detail: fmt.Sprintf("build health request: %v", err),
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Name:   CheckHTTPSHealth,
			Status: CheckFail,
			Detail: fmt.Sprintf("HTTPS health request failed (certificate chain or connectivity): %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{
			Name:   CheckHTTPSHealth,
			Status: CheckFail,
			Detail: fmt.Sprintf("health endpoint returned HTTP %d, expected 200", resp.StatusCode),
		}
	}

	var health models.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return CheckResult{
			Name:   CheckHTTPSHealth,
			Status: CheckFail,
			Detail: fmt.Sprintf("health response parse failed: %v", err),
		}
	}

	if health.Status != constants.GatewayModeStatusOK {
		return CheckResult{
			Name:   CheckHTTPSHealth,
			Status: CheckFail,
			Detail: fmt.Sprintf("health status is %q, expected %q", health.Status, constants.GatewayModeStatusOK),
		}
	}

	return CheckResult{
		Name:   CheckHTTPSHealth,
		Status: CheckPass,
		Detail: fmt.Sprintf("Gateway healthy (mode %s, posture %s)", health.Mode, health.Posture),
	}
}

// checkCORS sends an HTTPS OPTIONS preflight with the exact frontend
// Origin, requested method, and requested headers, then checks each CORS
// response header independently. Returns results in deterministic order:
// origin, credentials, methods, headers, vary.
func (v *Verifier) checkCORS(ctx context.Context, client *http.Client, healthURL string, origin browserorigin.Origin) []CheckResult {
	originStr := origin.URL

	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, healthURL, nil)
	if err != nil {
		return []CheckResult{
			{Name: CheckCORSOrigin, Status: CheckFail, Detail: fmt.Sprintf("build CORS preflight request: %v", err)},
		}
	}
	req.Header.Set(constants.HeaderOrigin, originStr)
	req.Header.Set(constants.HeaderAccessControlRequestMethod, "POST")
	req.Header.Set(constants.HeaderAccessControlRequestHeaders, "Content-Type, Authorization")

	resp, err := client.Do(req)
	if err != nil {
		return []CheckResult{
			{Name: CheckCORSOrigin, Status: CheckFail, Detail: fmt.Sprintf("CORS preflight request failed: %v", err)},
		}
	}
	defer resp.Body.Close()

	var results []CheckResult

	// CORS origin: must be 204 and reflect the exact origin.
	if resp.StatusCode != http.StatusNoContent {
		results = append(results, CheckResult{
			Name:   CheckCORSOrigin,
			Status: CheckFail,
			Detail: fmt.Sprintf("preflight returned HTTP %d, expected 204 No Content", resp.StatusCode),
		})
		return results
	}

	allowOrigin := resp.Header.Get(constants.HeaderAccessControlAllowOrigin)
	if allowOrigin != originStr {
		results = append(results, CheckResult{
			Name:   CheckCORSOrigin,
			Status: CheckFail,
			Detail: fmt.Sprintf("Access-Control-Allow-Origin is %q, expected exact %q", allowOrigin, originStr),
		})
	} else {
		results = append(results, CheckResult{
			Name:   CheckCORSOrigin,
			Status: CheckPass,
			Detail: fmt.Sprintf("origin %s reflected exactly", originStr),
		})
	}

	// CORS credentials: must be "true".
	allowCreds := resp.Header.Get(constants.HeaderAccessControlAllowCredentials)
	if allowCreds != "true" {
		results = append(results, CheckResult{
			Name:   CheckCORSCredentials,
			Status: CheckFail,
			Detail: fmt.Sprintf("Access-Control-Allow-Credentials is %q, expected \"true\"", allowCreds),
		})
	} else {
		results = append(results, CheckResult{
			Name:   CheckCORSCredentials,
			Status: CheckPass,
			Detail: "credentials allowed",
		})
	}

	// CORS methods: must include the requested method (POST).
	allowMethods := resp.Header.Get(constants.HeaderAccessControlAllowMethods)
	if !methodAllowed(allowMethods, "POST") {
		results = append(results, CheckResult{
			Name:   CheckCORSMethods,
			Status: CheckFail,
			Detail: fmt.Sprintf("Access-Control-Allow-Methods is %q, expected to include POST", allowMethods),
		})
	} else {
		results = append(results, CheckResult{
			Name:   CheckCORSMethods,
			Status: CheckPass,
			Detail: "requested method POST allowed",
		})
	}

	// CORS headers: must include the requested headers.
	allowHeaders := resp.Header.Get(constants.HeaderAccessControlAllowHeaders)
	missingHeaders := headersMissing(allowHeaders, "Content-Type", "Authorization")
	if len(missingHeaders) > 0 {
		results = append(results, CheckResult{
			Name:   CheckCORSHeaders,
			Status: CheckFail,
			Detail: fmt.Sprintf("Access-Control-Allow-Headers is %q, missing %s", allowHeaders, strings.Join(missingHeaders, ", ")),
		})
	} else {
		results = append(results, CheckResult{
			Name:   CheckCORSHeaders,
			Status: CheckPass,
			Detail: "requested headers Content-Type, Authorization allowed",
		})
	}

	vary := strings.Join(resp.Header.Values(constants.HeaderVary), ",")
	if !methodAllowed(vary, constants.HeaderOrigin) {
		results = append(results, CheckResult{
			Name:   CheckCORSVary,
			Status: CheckFail,
			Detail: fmt.Sprintf("Vary is %q, expected to include Origin", vary),
		})
	} else {
		results = append(results, CheckResult{
			Name:   CheckCORSVary,
			Status: CheckPass,
			Detail: "responses vary by Origin",
		})
	}

	return results
}

// methodAllowed reports whether the comma-separated allowMethods list
// contains the given method (case-insensitive).
func methodAllowed(allowMethods, method string) bool {
	for _, m := range strings.Split(allowMethods, ",") {
		if strings.EqualFold(strings.TrimSpace(m), method) {
			return true
		}
	}
	return false
}

// headersMissing returns the requested headers that are not present in
// the comma-separated allowHeaders list (case-insensitive).
func headersMissing(allowHeaders string, requested ...string) []string {
	allowedSet := make(map[string]bool)
	for _, h := range strings.Split(allowHeaders, ",") {
		allowedSet[strings.ToLower(strings.TrimSpace(h))] = true
	}
	var missing []string
	for _, h := range requested {
		if !allowedSet[strings.ToLower(h)] {
			missing = append(missing, h)
		}
	}
	return missing
}
