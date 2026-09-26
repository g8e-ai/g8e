// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/protocol"
)

const (
	inferenceHarnessUserID     = "user-inference-python-harness"
	inferenceHarnessOperatorID = "operator-inference-python-harness"
	inferenceHarnessSessionID  = "session-inference-python-harness"
	inferenceHarnessPrompt     = "Python ensemble boundary"
)

type inferenceDispatchHarnessFixture struct {
	ClientURL               string `json:"client_url"`
	CACertPath              string `json:"ca_cert_path"`
	ClientCertPath          string `json:"client_cert_path"`
	ClientKeyPath           string `json:"client_key_path"`
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	PromptText              string `json:"prompt_text"`
	ExpectedResponseText    string `json:"expected_response_text"`
}

type inferenceDispatchHarnessTLS struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
	ServerCert     tls.Certificate
}

func writePEMFile(t *testing.T, dir, name, pemData string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(pemData), 0o600))
	return path
}

func writeECPrivateKeyPEM(t *testing.T, dir, name string, key *ecdsa.PrivateKey) string {
	t.Helper()
	keyBytes, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return writePEMFile(t, dir, name, string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})))
}

func generateHarnessDelegatedAppCert(
	t *testing.T,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	userID string,
) (certPEM string, key *ecdsa.PrivateKey) {
	t.Helper()
	wid := protocol.NewWorkloadIdentity()
	appURI, err := wid.AppSPIFFEURL("g8ee")
	require.NoError(t, err)
	userURI, err := wid.UserSPIFFEURL(userID)
	require.NoError(t, err)

	key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "inference-dispatch-harness-client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{appURI, userURI},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return certPEM, key
}

func generateHarnessServerCert(
	t *testing.T,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
) (certPEM string, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "inference-dispatch-harness-gateway"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return certPEM, key
}

func generateInferenceDispatchHarnessTLS(t *testing.T, dir string, userID string) *inferenceDispatchHarnessTLS {
	t.Helper()
	caKey, caCert := testutil.GenerateTestCAWithKey(t, "inference-dispatch-harness-ca")
	caPEM := testutil.EncodePEM("CERTIFICATE", caCert.Raw)

	serverCertPEM, serverKey := generateHarnessServerCert(t, caCert, caKey)
	clientCertPEM, clientKey := generateHarnessDelegatedAppCert(t, caCert, caKey, userID)

	caPath := writePEMFile(t, dir, "ca.pem", caPEM)
	clientCertPath := writePEMFile(t, dir, "client.pem", clientCertPEM)
	clientKeyPath := writeECPrivateKeyPEM(t, dir, "client.key", clientKey)
	serverCertPath := writePEMFile(t, dir, "server.pem", serverCertPEM)
	serverKeyPath := writeECPrivateKeyPEM(t, dir, "server.key", serverKey)

	serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	require.NoError(t, err)

	return &inferenceDispatchHarnessTLS{
		CACertPath:     caPath,
		ClientCertPath: clientCertPath,
		ClientKeyPath:  clientKeyPath,
		ServerCert:     serverCert,
	}
}

func setupInferenceDispatchHarnessGateway(t *testing.T) (*HTTPHandler, *boundaryInferenceBackend, *inferenceDispatchHarnessTLS) {
	t.Helper()
	h, cfg, infra := setupTestHTTPHandler(t)
	seedActiveUser(t, infra, inferenceHarnessUserID)
	seedAppPolicy(t, infra, protocol.EnsembleAppID)
	seedInferenceOperator(t, infra, inferenceHarnessUserID, inferenceHarnessOperatorID, inferenceHarnessSessionID, true)
	backend, _ := startInferenceOperator(t, infra, infra.StateRootSvc, inferenceHarnessOperatorID, inferenceHarnessSessionID)
	h.inferenceDispatchController = newInferenceDispatchController(InferenceDispatchControllerDeps{
		DispatchSvc: newInferenceBoundaryDispatchService(infra, config.PostureDoctrine),
		Responder:   infra.Responder,
		Logger:      infra.Logger,
		MaxPayload:  cfg.Gateway.MaxPayloadBytes,
	})
	h.router = h.buildPublicRouter()

	dir := t.TempDir()
	tlsMaterials := generateInferenceDispatchHarnessTLS(t, dir, inferenceHarnessUserID)
	return h, backend, tlsMaterials
}

func startInferenceDispatchHarnessServer(t *testing.T, handler http.Handler, tlsMaterials *inferenceDispatchHarnessTLS) (string, func()) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsMaterials.ServerCert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequestClientCert,
	})
	require.NoError(t, err)

	srv := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(listener)
		close(done)
	}()

	baseURL := "https://" + listener.Addr().String()
	cleanup := func() {
		_ = srv.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
	return baseURL, cleanup
}

func writeInferenceDispatchHarnessFixture(t *testing.T, path string, fixture inferenceDispatchHarnessFixture) {
	t.Helper()
	body, err := json.Marshal(fixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0o600))
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func pythonExecutable(t *testing.T) string {
	t.Helper()
	repoRoot := repoRootFromTest(t)
	ensembleRoot := filepath.Join(repoRoot, "ensemble")
	protocolPythonRoot := filepath.Join(repoRoot, "protocol", "python")
	pythonPaths := []string{ensembleRoot, protocolPythonRoot, repoRoot}
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		pythonPaths = append(pythonPaths, existing)
	}
	pythonPathEnv := strings.Join(pythonPaths, string(os.PathListSeparator))

	candidates := []string{
		filepath.Join(repoRoot, ".venv", "bin", "python"),
		filepath.Join(repoRoot, "ensemble", ".venv", "bin", "python"),
		"python3",
		"python",
	}
	for _, candidate := range candidates {
		var execPath string
		if candidate == "python3" || candidate == "python" {
			var err error
			execPath, err = exec.LookPath(candidate)
			if err != nil {
				continue
			}
		} else {
			if _, err := os.Stat(candidate); err != nil {
				continue
			}
			execPath = candidate
		}

		// Verify this interpreter has required dependencies installed.
		checkCmd := exec.Command(execPath, "-c", "import pydantic, httpx, cryptography, google.protobuf; from app.models.internal_api import InferenceDispatchRequest")
		checkCmd.Dir = repoRoot
		checkCmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPathEnv)
		if err := checkCmd.Run(); err == nil {
			return execPath
		}
	}
	t.Skip("python interpreter with ensemble dependencies not available for inference dispatch harness")
	return ""
}

func runInferenceDispatchHarnessPython(t *testing.T, fixturePath string) {
	t.Helper()
	repoRoot := repoRootFromTest(t)
	ensembleRoot := filepath.Join(repoRoot, "ensemble")
	protocolPythonRoot := filepath.Join(repoRoot, "protocol", "python")
	script := filepath.Join(ensembleRoot, "tests", "integration", "inference_dispatch_harness_runner.py")
	require.FileExists(t, script)

	pythonPaths := []string{ensembleRoot, protocolPythonRoot, repoRoot}
	if existing := os.Getenv("PYTHONPATH"); existing != "" {
		pythonPaths = append(pythonPaths, existing)
	}

	cmd := exec.Command(pythonExecutable(t), script)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE="+fixturePath,
		"PYTHONPATH="+strings.Join(pythonPaths, string(os.PathListSeparator)),
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "python harness runner failed: %s", string(output))
}

// TestInferenceDispatchPythonHarnessServe blocks with a real TLS gateway and
// stub inference backend until SIGTERM. Python integration tests spawn this via:
//
//	G8E_INFERENCE_DISPATCH_HARNESS=serve go test -tags=integration \
//	  -run '^TestInferenceDispatchPythonHarnessServe$' -count=1 -timeout=0 \
//	  ./internal/services/gateway/
func TestInferenceDispatchPythonHarnessServe(t *testing.T) {
	if os.Getenv("G8E_INFERENCE_DISPATCH_HARNESS") != "serve" {
		t.Skip("only runs when G8E_INFERENCE_DISPATCH_HARNESS=serve")
	}
	fixturePath := os.Getenv("G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE")
	if fixturePath == "" {
		t.Fatal("G8E_INFERENCE_DISPATCH_HARNESS_FIXTURE is required in serve mode")
	}

	h, _, tlsMaterials := setupInferenceDispatchHarnessGateway(t)
	baseURL, cleanup := startInferenceDispatchHarnessServer(t, h, tlsMaterials)
	defer cleanup()

	writeInferenceDispatchHarnessFixture(t, fixturePath, inferenceDispatchHarnessFixture{
		ClientURL:               baseURL,
		CACertPath:              tlsMaterials.CACertPath,
		ClientCertPath:          tlsMaterials.ClientCertPath,
		ClientKeyPath:           tlsMaterials.ClientKeyPath,
		TargetOperatorSessionID: inferenceHarnessSessionID,
		PromptText:              inferenceHarnessPrompt,
		ExpectedResponseText:    "governed boundary response",
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh
}

// TestInferenceDispatch_PythonClientVerifiedRoundTrip exercises the same
// Tier-2 cross-boundary path from the Go side: a real public router, real
// broker/operator boundary, and Python InternalHttpClient.dispatch_inference.
func TestInferenceDispatch_PythonClientVerifiedRoundTrip(t *testing.T) {
	h, backend, tlsMaterials := setupInferenceDispatchHarnessGateway(t)
	baseURL, cleanup := startInferenceDispatchHarnessServer(t, h, tlsMaterials)
	defer cleanup()

	fixturePath := filepath.Join(t.TempDir(), "harness.json")
	writeInferenceDispatchHarnessFixture(t, fixturePath, inferenceDispatchHarnessFixture{
		ClientURL:               baseURL,
		CACertPath:              tlsMaterials.CACertPath,
		ClientCertPath:          tlsMaterials.ClientCertPath,
		ClientKeyPath:           tlsMaterials.ClientKeyPath,
		TargetOperatorSessionID: inferenceHarnessSessionID,
		PromptText:              inferenceHarnessPrompt,
		ExpectedResponseText:    "governed boundary response",
	})

	runInferenceDispatchHarnessPython(t, fixturePath)

	calls, request := backend.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, "assistant-boundary", request.Model)
	assert.Equal(t, inferenceHarnessPrompt, boundaryInferenceText(t, request.Messages))
}
