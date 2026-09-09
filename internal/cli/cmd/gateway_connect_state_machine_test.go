// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// stubTrustInstaller is a configurable mock auth.SystemTrustInstaller for
// connect state-machine tests. It never touches the real OS trust store.
type stubTrustInstaller struct {
	trusted       bool
	staleAnchors  []platform.StaleAnchor
	isTrustedErr  error
	installErr    error
	listStaleErr  error
	removeErr     error
	installCalled bool
	removeCalled  bool
	removeAnchors []platform.StaleAnchor
}

func (s *stubTrustInstaller) IsTrusted(context.Context, string) (bool, error) {
	if s.isTrustedErr != nil {
		return false, s.isTrustedErr
	}
	return s.trusted, nil
}

func (s *stubTrustInstaller) InstallRoot(_ context.Context, _ *x509.Certificate, _ string) error {
	s.installCalled = true
	return s.installErr
}

func (s *stubTrustInstaller) ListStaleAnchors(context.Context, string) ([]platform.StaleAnchor, error) {
	if s.listStaleErr != nil {
		return nil, s.listStaleErr
	}
	return s.staleAnchors, nil
}

func (s *stubTrustInstaller) RemoveStaleAnchors(_ context.Context, anchors []platform.StaleAnchor) error {
	s.removeCalled = true
	s.removeAnchors = anchors
	return s.removeErr
}

// stubDiscoveryResult builds a TrustDiscoveryResult from a test CA + leaf cert
// so the connect command's trust phase has a valid bundle to work with.
func stubDiscoveryResult(t *testing.T) auth.TrustDiscoveryResult {
	t.Helper()
	caKey, caCert := testutil.GenerateTestCAWithKey(t, "g8e-test-root")
	leafPEM, _ := testutil.GenerateTestSignedCert(t, "g8e-test-leaf", caCert, caKey)
	caPEM := testutil.EncodePEM("CERTIFICATE", caCert.Raw)
	bundlePEM := []byte(leafPEM + caPEM)

	result, err := auth.ValidateTrustBundle(bundlePEM, time.Now)
	require.NoError(t, err)
	return result
}

// urlRewriteTransport is an http.RoundTripper that rewrites the host in each
// request URL to point at a test server while preserving the path and headers.
// This lets the verifier hit https://localhost:8443/api/v1/health in production
// while the test redirects to the httptest.Server's actual listener address.
type urlRewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse the target to get its scheme and host.
	targetURL, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = targetURL.Scheme
	cloned.URL.Host = targetURL.Host
	cloned.Host = targetURL.Host
	return t.base.RoundTrip(cloned)
}

// stubVerifierDeps returns a frontendverify.VerifierDeps with an HTTP client
// factory that targets the given httptest.Server. Requests to the production
// Gateway URL (https://localhost:8443/...) are rewritten to the test server's
// listener address. The server's TLS certificate is added to the root pool so
// the TLS handshake succeeds.
func stubVerifierDeps(t *testing.T, srv *httptest.Server) frontendverify.VerifierDeps {
	t.Helper()
	srvCert := srv.Certificate()
	return frontendverify.VerifierDeps{
		HTTPClientFactory: func(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error) {
			rootPool.AddCert(srvCert)
			client := srv.Client()
			client.Timeout = timeout
			transport := client.Transport.(*http.Transport).Clone()
			transport.TLSClientConfig.RootCAs = rootPool
			client.Transport = &urlRewriteTransport{
				base:   transport,
				target: srv.URL,
			}
			return client, nil
		},
	}
}

// newConnectTestServer starts an httptest.NewTLSServer that responds to GET
// /api/v1/health with a 200 HealthResponse and to OPTIONS /api/v1/health with
// 204 and proper CORS headers reflecting the Origin header. The server is
// registered for cleanup via t.Cleanup.
func newConnectTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			origin := r.Header.Get(constants.HeaderOrigin)
			w.Header().Set(constants.HeaderAccessControlAllowOrigin, origin)
			w.Header().Set(constants.HeaderAccessControlAllowCredentials, "true")
			w.Header().Set(constants.HeaderAccessControlAllowMethods, "POST, GET, OPTIONS")
			w.Header().Set(constants.HeaderAccessControlAllowHeaders, "Content-Type, Authorization")
			w.Header().Set("Vary", "Origin")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","mode":"gateway","posture":"doctrine"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writePIDFile writes a PID file pointing at the current test process PID so
// OperatorStatus reports the Gateway as running.
func writePIDFile(t *testing.T, fileSvc interface {
	WriteFile(ctx context.Context, relPath string, data []byte, perm os.FileMode) error
}) {
	t.Helper()
	currentPID := os.Getpid()
	pidRelPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidRelPath, []byte(strconv.Itoa(currentPID)), constants.PermFilePrivate))
}

// TestGatewayConnectCmd_InvalidOriginReturnsErrValidationFailed verifies that
// an invalid frontend origin is rejected with ErrValidationFailed before any
// state-changing action.
func TestGatewayConnectCmd_InvalidOriginReturnsErrValidationFailed(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(nil), connectDeps{})
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"not-a-url"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestGatewayConnectCmd_InvalidRPIDOverrideReturnsErrValidationFailed verifies
// that an invalid --passkey-rp-id override is rejected with ErrValidationFailed.
func TestGatewayConnectCmd_InvalidRPIDOverrideReturnsErrValidationFailed(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(nil), connectDeps{})
	require.NoError(t, cmd.Flags().Set("passkey-rp-id", "unrelated.com"))
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestGatewayConnectCmd_NoArgsReturnsUsageError verifies that calling connect
// without a frontend origin argument produces a cobra usage error (ExactArgs
// constraint).
func TestGatewayConnectCmd_NoArgsReturnsUsageError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(nil), connectDeps{})
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
}

// TestGatewayConnectCmd_RunningGatewayWithoutLaunchProfileFailsClosed
// verifies that a running Gateway with no persisted launch profile fails
// closed with ErrLaunchProfileMissing rather than guessing the configuration.
func TestGatewayConnectCmd_RunningGatewayWithoutLaunchProfileFailsClosed(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), connectDeps{})
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLaunchProfileMissing)
	assert.Contains(t, buf.String(), "no complete launch profile")
}

// TestGatewayConnectCmd_RunningGatewayWithMatchingConfigProceedsToVerify
// verifies that a running Gateway whose launch profile matches the requested
// browser configuration does not restart and proceeds directly to trust
// discovery and verification. With stub deps returning success, the command
// prints the frontend prompt.
func TestGatewayConnectCmd_RunningGatewayWithMatchingConfigProceedsToVerify(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	// Write a matching launch profile.
	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	// Set up a test HTTPS server for the verifier.
	srv := newConnectTestServer(t)
	discovery := stubDiscoveryResult(t)

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{trusted: true},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		verifier:      frontendverify.NewVerifier(stubVerifierDeps(t, srv)),
		browserOpener: func(string) error { return nil },
		confirm:       func(string) bool { return false },
		continueFn:    func(string) bool { return true },
		now:           time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "running (configuration matches)")
	assert.Contains(t, output, "already trusted")
	assert.Contains(t, output, "Paste this into your frontend builder")
	assert.NotContains(t, output, "Stopping g8e Gateway")
}

// TestGatewayConnectCmd_RunningGatewayWithDifferingConfigDeclinedRestartReturnsErrManagedRestartDeclined
// verifies that a running Gateway whose launch profile differs from the
// requested browser configuration asks for restart consent and returns
// ErrManagedRestartDeclined when the user declines.
func TestGatewayConnectCmd_RunningGatewayWithDifferingConfigDeclinedRestartReturnsErrManagedRestartDeclined(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	// Write a non-matching launch profile (different CORS origin).
	profileCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{"https://old-app.lovable.app"},
		PasskeyRpOrigins: []string{"https://old-app.lovable.app"},
		PasskeyRpID:      "old-app.lovable.app",
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, profileCfg))

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when restart is declined")
		},
		verifier: frontendverify.NewVerifier(frontendverify.VerifierDeps{
			HTTPClientFactory: func(*x509.CertPool, time.Duration) (*http.Client, error) {
				panic("verifier should not be called when restart is declined")
			},
		}),
		confirm: func(string) bool { return false },
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrManagedRestartDeclined)
	assert.Contains(t, buf.String(), "Restart declined")
}

// TestGatewayConnectCmd_StoppedGatewayAttemptsStart verifies that a stopped
// Gateway with no launch profile attempts to start. The test makes .g8e/bin
// read-only so StartOperator fails, proving the command got past validation
// and tried to start.
func TestGatewayConnectCmd_StoppedGatewayAttemptsStart(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	// Make .g8e/bin read-only so StartOperator fails at binary copy.
	binAbsPath := fileSvc.Resolve(constants.BinDirname)
	require.NoError(t, os.MkdirAll(binAbsPath, constants.PermDirStandard))
	require.NoError(t, os.Chmod(binAbsPath, 0500))
	t.Cleanup(func() { _ = os.Chmod(binAbsPath, constants.PermDirStandard) })

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when StartOperator fails")
		},
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed)
	assert.Contains(t, buf.String(), "Starting g8e Gateway service")
}

// TestGatewayConnectCmd_TrustDeclinedReturnsErrManualBrowserTrustRequired
// verifies that declining OS trust installation returns
// ErrManualBrowserTrustRequired and prints manual trust instructions.
func TestGatewayConnectCmd_TrustDeclinedReturnsErrManualBrowserTrustRequired(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	discovery := stubDiscoveryResult(t)

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{trusted: false},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		confirm: func(string) bool { return false },
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrManualBrowserTrustRequired)
	assert.Contains(t, buf.String(), "Manual browser trust is required")
}

// TestGatewayConnectCmd_NoSystemTrustSkipsInstallAndProceedsToManualTrust
// verifies that --no-system-trust skips OS trust installation, prints manual
// trust instructions, and proceeds to verification. With stub deps returning
// success, the command prints the frontend prompt.
func TestGatewayConnectCmd_NoSystemTrustSkipsInstallAndProceedsToManualTrust(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	srv := newConnectTestServer(t)
	discovery := stubDiscoveryResult(t)
	browserOpened := false

	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{trusted: false},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		verifier:      frontendverify.NewVerifier(stubVerifierDeps(t, srv)),
		browserOpener: func(string) error { browserOpened = true; return nil },
		confirm:       func(string) bool { return false },
		continueFn:    func(string) bool { return true },
		now:           time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	require.NoError(t, cmd.Flags().Set("no-system-trust", "true"))
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "System trust installation skipped (--no-system-trust)")
	assert.Contains(t, output, "Manual browser trust is required")
	assert.Contains(t, output, "Paste this into your frontend builder")
	assert.True(t, browserOpened, "browser should be opened in manual trust mode")
}

// TestGatewayConnectCmd_TrustInstallWithConsentProceedsToVerify verifies that
// approving trust installation with consent installs the root and proceeds to
// verification. The stub trust installer records the install call.
func TestGatewayConnectCmd_TrustInstallWithConsentProceedsToVerify(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	srv := newConnectTestServer(t)
	discovery := stubDiscoveryResult(t)
	trustInst := &stubTrustInstaller{trusted: false}

	deps := connectDeps{
		trustInstaller: trustInst,
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		verifier:      frontendverify.NewVerifier(stubVerifierDeps(t, srv)),
		browserOpener: func(string) error { return nil },
		confirm:       func(string) bool { return true },
		continueFn:    func(string) bool { return true },
		now:           time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.NoError(t, err)

	assert.True(t, trustInst.installCalled, "InstallRoot should be called after consent")
	assert.Contains(t, buf.String(), "g8e root CA installed")
	assert.Contains(t, buf.String(), "Paste this into your frontend builder")
}

// TestGatewayConnectCmd_StaleAnchorRemovalWithConsent verifies that stale
// anchors are removed when the user consents, and the browser-restart gate
// is triggered.
func TestGatewayConnectCmd_StaleAnchorRemovalWithConsent(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	srv := newConnectTestServer(t)
	discovery := stubDiscoveryResult(t)
	trustInst := &stubTrustInstaller{
		trusted:      true,
		staleAnchors: []platform.StaleAnchor{{Fingerprint: "stale-fp", CommonName: "old-g8e-root", Handle: "/tmp/old.pem"}},
	}
	continueCalled := false

	deps := connectDeps{
		trustInstaller: trustInst,
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		verifier:      frontendverify.NewVerifier(stubVerifierDeps(t, srv)),
		browserOpener: func(string) error { return nil },
		confirm:       func(string) bool { return true },
		continueFn:    func(string) bool { continueCalled = true; return true },
		now:           time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.NoError(t, err)

	assert.True(t, trustInst.removeCalled, "RemoveStaleAnchors should be called after consent")
	assert.Len(t, trustInst.removeAnchors, 1)
	assert.True(t, continueCalled, "browser-restart gate should be triggered after trust change")
	assert.Contains(t, buf.String(), "Stale anchors removed")
	assert.Contains(t, buf.String(), "Close all browser windows")
}

// TestGatewayConnectCmd_VerificationFailureReturnsErrCORSPreflightRejected
// verifies that a CORS verification failure returns ErrCORSPreflightRejected
// and does not print the frontend prompt.
func TestGatewayConnectCmd_UnsupportedSystemTrustRequiresExplicitManualMode(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}))

	discovery := stubDiscoveryResult(t)
	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{installErr: constants.ErrSystemTrustUnsupported},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		confirm: func(string) bool { return true },
		now:     time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{origin.URL})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrManualBrowserTrustRequired)
	assert.NotContains(t, buf.String(), "Paste this into your frontend builder")
}

func TestGatewayConnectCmd_DeclinedBrowserRestartGateStopsHandoff(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}))

	discovery := stubDiscoveryResult(t)
	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		confirm:    func(string) bool { return true },
		continueFn: func(string) bool { return false },
		now:        time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{origin.URL})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrBrowserRestartDeclined)
	assert.NotContains(t, buf.String(), "Paste this into your frontend builder")
}

func TestGatewayConnectCmd_VerificationFailureReturnsErrCORSPreflightRejected(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	writePIDFile(t, fileSvc)

	origin, err := browserorigin.Parse("https://your-app.lovable.app")
	require.NoError(t, err)

	matchCfg := serve.GatewayConfig{
		Posture:          "doctrine",
		LogLevel:         "info",
		CertIdentityMode: "localhost",
		AllowedOrigins:   []string{origin.URL},
		PasskeyRpOrigins: []string{origin.URL},
		PasskeyRpID:      origin.RPID,
		PasskeyRpName:    "g8e",
	}
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, matchCfg))

	discovery := stubDiscoveryResult(t)

	// Verifier with a stub HTTP client factory that returns an all-fail report.
	deps := connectDeps{
		trustInstaller: &stubTrustInstaller{trusted: true},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			return discovery, nil
		},
		verifier: frontendverify.NewVerifier(frontendverify.VerifierDeps{
			HTTPClientFactory: func(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error) {
				return &http.Client{Timeout: timeout}, nil
			},
		}),
		confirm:    func(string) bool { return false },
		continueFn: func(string) bool { return true },
		now:        time.Now,
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc), deps)
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	// The HTTPS health check will fail because there's no server listening,
	// so the error is ErrHTTPSCertificateVerification (the first failure is
	// HTTPS health).
	assert.True(t,
		errors.Is(err, constants.ErrHTTPSCertificateVerification) || errors.Is(err, constants.ErrCORSPreflightRejected),
		"error should be a verification failure, got: %v", err,
	)
	assert.NotContains(t, buf.String(), "Paste this into your frontend builder")
}
