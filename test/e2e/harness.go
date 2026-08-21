// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// DockerE2EFixture spins up docker-compose, waits for health, and tears down on cleanup.
// It tests black-box observable gateway and operator behaviors — HTTP health, CA bundle
// discovery, operator liveness, and external mTLS handshakes using enrolled client credentials.
type DockerE2EFixture struct {
	GatewayHTTPURL   string // http://localhost:<httpPort>
	GatewayHTTPSURL  string // https://localhost:<httpsPort> (no client cert for these tests)
	EnsembleHTTPURL  string // http://localhost:<ensemblePort>
	DashboardHTTPURL string // http://localhost:<dashboardPort>
	ComposeFile      string
	ProjectDir       string
	ProjectName      string // unique docker compose project name
	ContainerPrefix  string // unique container name prefix
	HTTPPort         int    // allocated host HTTP port
	HTTPSPort        int    // allocated host HTTPS port
	EnsemblePort     int    // allocated host ensemble port
	DashboardPort    int    // allocated host dashboard port

	// cliMTLSConfig is the owner CLI identity created during bootstrap.
	// It is used for authenticated mTLS calls to the platform enrollment
	// pending and decision endpoints during fixture setup.
	cliMTLSConfig *tls.Config
}

// setupSharedE2EFixture performs the Docker Compose setup without requiring
// a *testing.T, enabling use from TestMain. It allocates ports, builds and
// starts the stack, waits for health, and returns the fixture. On failure it
// tears down any partially-started stack before returning the error.
func setupSharedE2EFixture(composeFile string) (*DockerE2EFixture, error) {
	fixture, err := allocateE2EFixture(composeFile)
	if err != nil {
		return nil, err
	}

	// Owner-approved platform enrollment activation flow. All four services
	// (gateway, dashboard, ensemble, operator) start together as the
	// full-stack default. The gateway becomes healthy immediately, but the
	// operator, dashboard, and ensemble submit platform enrollment requests
	// and stay not-ready until the owner approves them. The flow is:
	//   1. docker compose up -d --build (no --wait: workloads are not ready)
	//   2. Poll the gateway health endpoint until it is healthy.
	//   3. Bootstrap the first user WITH a CLI CSR to obtain mTLS credentials.
	//   4. Wait for pending platform enrollment requests to appear.
	//   5. Approve operator, dashboard, and ensemble in the recommended order.
	//   6. Wait for workload health (operator, dashboard, ensemble).
	if err := fixture.composeUpAndWait(); err != nil {
		fixture.teardownOnErr("compose-up", err)
		return nil, err
	}

	if err := fixture.bootstrapFirstUserWithCLICert(); err != nil {
		fixture.teardownOnErr("bootstrap", err)
		return nil, err
	}
	log.Printf("E2E: First user bootstrapped with CLI mTLS credentials")

	if err := fixture.approvePlatformEnrollments(); err != nil {
		fixture.teardownOnErr("approval", err)
		return nil, err
	}
	log.Printf("E2E: All platform enrollments approved")

	if err := fixture.waitForWorkloadHealth(180 * time.Second); err != nil {
		fixture.teardownOnErr("workload-health", err)
		return nil, err
	}
	log.Printf("E2E: All workloads are healthy")

	return fixture, nil
}

// setupE2EFixtureUpToBootstrap allocates a fixture, starts the full stack,
// waits for gateway health, and bootstraps the first user with CLI mTLS
// credentials — but does NOT approve any platform enrollment requests. The
// returned fixture has pending requests that the caller can approve, deny, or
// inspect. Used by tests that need to exercise the pre-approval state (denial,
// pending discovery). The caller is responsible for teardown via
// fixture.teardown().
func setupE2EFixtureUpToBootstrap(composeFile string) (*DockerE2EFixture, error) {
	fixture, err := allocateE2EFixture(composeFile)
	if err != nil {
		return nil, err
	}
	if err := fixture.composeUpAndWait(); err != nil {
		fixture.teardownOnErr("compose-up", err)
		return nil, err
	}
	if err := fixture.bootstrapFirstUserWithCLICert(); err != nil {
		fixture.teardownOnErr("bootstrap", err)
		return nil, err
	}
	log.Printf("E2E: Fixture ready up to bootstrap (no approvals) — pending requests available")
	return fixture, nil
}

// setupE2EFixtureGatewayOnly allocates a fixture and starts only the gateway
// service (via --no-deps g8e-gateway), then waits for gateway health. No
// workloads are started, so no platform enrollment requests are submitted.
// Used by headless deployment tests. The caller is responsible for teardown.
func setupE2EFixtureGatewayOnly(composeFile string) (*DockerE2EFixture, error) {
	fixture, err := allocateE2EFixture(composeFile)
	if err != nil {
		return nil, err
	}

	log.Printf("E2E: Starting gateway-only (project: %s)", fixture.ProjectName)
	upCmd := exec.Command("docker", "compose", "-p", fixture.ProjectName, "-f", fixture.ComposeFile, "up", "-d", "--build", "--no-deps", "g8e-gateway")
	upCmd.Dir = fixture.ProjectDir
	upCmd.Env = append(os.Environ(), fixture.composeEnv()...)
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		fixture.teardownOnErr("gateway-only-up", err)
		return nil, fmt.Errorf("docker compose up (gateway only) failed: %w\nOutput: %s", err, string(upOutput))
	}

	if err := fixture.waitForGatewayHealth(120 * time.Second); err != nil {
		fixture.teardownOnErr("gateway-health", err)
		return nil, err
	}
	log.Printf("E2E: Gateway-only fixture ready (no workloads)")
	return fixture, nil
}

// allocateE2EFixture resolves the compose file, allocates a unique port set
// and container prefix, and returns an initialized fixture without starting
// any containers. The caller starts containers via composeUpAndWait,
// composeUpGatewayOnly, or a custom docker compose command.
func allocateE2EFixture(composeFile string) (*DockerE2EFixture, error) {
	// Resolve repository root via go list -m
	repoCmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	repoOutput, err := repoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	repoRoot := filepath.Clean(strings.TrimSpace(string(repoOutput)))
	if repoRoot == "" {
		return nil, fmt.Errorf("go list -m returned empty directory")
	}

	// Build absolute path to compose file
	var composePath string
	if filepath.IsAbs(composeFile) {
		composePath = composeFile
	} else {
		composePath = filepath.Join(repoRoot, composeFile)
	}

	// Verify compose file exists
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", composePath)
	}

	// Allocate available ports sequentially starting from 8080/8443/8000/3000.
	httpPort, httpsPort, ensemblePort, dashboardPort := 8080, 8443, 8000, 3000
	for offset := 0; offset < 1000; offset++ {
		candidates := []int{8080 + offset, 8443 + offset, 8000 + offset, 3000 + offset}
		lns := make([]*net.TCPListener, 0, len(candidates))
		ok := true
		for _, p := range candidates {
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if err != nil {
				ok = false
				break
			}
			lns = append(lns, ln.(*net.TCPListener))
		}
		for _, ln := range lns {
			ln.Close()
		}
		if ok {
			httpPort, httpsPort, ensemblePort, dashboardPort = candidates[0], candidates[1], candidates[2], candidates[3]
			break
		}
	}
	if httpPort == 8080 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", httpPort))
		if err != nil {
			return nil, fmt.Errorf("no available port set found in range 8080-9080")
		}
		ln.Close()
	}
	containerPrefix := fmt.Sprintf("g8e-%d", httpPort)
	projectName := containerPrefix

	log.Printf("E2E: Allocated ports HTTP=%d HTTPS=%d Ensemble=%d Dashboard=%d (prefix=%s)",
		httpPort, httpsPort, ensemblePort, dashboardPort, containerPrefix)

	return &DockerE2EFixture{
		GatewayHTTPURL:   fmt.Sprintf("http://localhost:%d", httpPort),
		GatewayHTTPSURL:  fmt.Sprintf("https://localhost:%d", httpsPort),
		EnsembleHTTPURL:  fmt.Sprintf("http://localhost:%d", ensemblePort),
		DashboardHTTPURL: fmt.Sprintf("http://localhost:%d", dashboardPort),
		ComposeFile:      composePath,
		ProjectDir:       repoRoot,
		ProjectName:      projectName,
		ContainerPrefix:  containerPrefix,
		HTTPPort:         httpPort,
		HTTPSPort:        httpsPort,
		EnsemblePort:     ensemblePort,
		DashboardPort:    dashboardPort,
	}, nil
}

// composeEnv returns the docker-compose environment variables for this
// fixture's allocated ports and container prefix.
func (f *DockerE2EFixture) composeEnv() []string {
	return []string{
		"DOCKER_BUILDKIT=1",
		fmt.Sprintf("G8E_HTTP_PORT=%d", f.HTTPPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", f.HTTPSPort),
		fmt.Sprintf("G8E_ENSEMBLE_PORT=%d", f.EnsemblePort),
		fmt.Sprintf("G8E_DASHBOARD_PORT=%d", f.DashboardPort),
		fmt.Sprintf("G8E_PREFIX=%s", f.ContainerPrefix),
	}
}

// composeUpAndWait starts the full stack via docker compose up -d --build and
// waits for the gateway health endpoint to return 200. Workloads are NOT
// healthy at this point — they are pending platform enrollment approval.
func (f *DockerE2EFixture) composeUpAndWait() error {
	log.Printf("E2E: Starting docker-compose full-stack (project: %s), no --wait (workloads pending approval)", f.ProjectName)
	upCmd := exec.Command("docker", "compose", "-p", f.ProjectName, "-f", f.ComposeFile, "up", "-d", "--build")
	upCmd.Dir = f.ProjectDir
	upCmd.Env = append(os.Environ(), f.composeEnv()...)
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\nOutput: %s", err, string(upOutput))
	}
	log.Printf("E2E: Compose stack started, waiting for gateway health")
	if err := f.waitForGatewayHealth(120 * time.Second); err != nil {
		return fmt.Errorf("gateway did not become healthy within 120s: %w", err)
	}
	log.Printf("E2E: Gateway is healthy")
	return nil
}

// teardownOnErr logs a teardown failure after a setup step fails. Used during
// fixture setup where a partial stack may be running.
func (f *DockerE2EFixture) teardownOnErr(step string, err error) {
	if tdErr := f.teardown(); tdErr != nil {
		log.Printf("E2E: teardown after %s failure also failed: %v", step, tdErr)
	}
}

// bootstrapFirstUserWithCLICert creates the first user via POST
// /api/v1/auth/bootstrap, activating the gateway. Unlike the old
// bootstrapFirstUser (which sent an empty body), this generates a P-256 key
// pair and CLI CSR, submits it with the bootstrap request, and receives back a
// signed CLI certificate and trust bundle. The CLI certificate is stored on
// the fixture as cliMTLSConfig so subsequent calls to the platform enrollment
// pending and decision endpoints can authenticate as the owner via mTLS. No
// operator CSR is submitted — the operator enrolls via the platform enrollment
// protocol, not via bootstrap.
func (f *DockerE2EFixture) bootstrapFirstUserWithCLICert() error {
	cliKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CLI key: %w", err)
	}

	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "e2e-owner",
			Organization: []string{"g8e"},
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, cliKey)
	if err != nil {
		return fmt.Errorf("create CLI CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	reqBody, err := json.Marshal(map[string]string{
		"cli_csr_pem": string(csrPEM),
	})
	if err != nil {
		return fmt.Errorf("marshal bootstrap body: %w", err)
	}

	reqURL := f.GatewayHTTPURL + constants.APIPaths.AuthBootstrap
	resp, err := http.Post(reqURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("bootstrap request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read bootstrap response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("bootstrap returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var bootResp models.BootstrapResponse
	if err := json.Unmarshal(respBody, &bootResp); err != nil {
		return fmt.Errorf("decode bootstrap response: %w", err)
	}
	if bootResp.CLICert == "" {
		return fmt.Errorf("bootstrap response missing cli_cert")
	}

	cliCert, err := tls.X509KeyPair([]byte(bootResp.CLICert), mustEncodeECDSAPrivateKey(cliKey))
	if err != nil {
		return fmt.Errorf("parse CLI X509 key pair: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caBundle := bootResp.HubTrustBundle
	if caBundle == "" {
		caBundle = f.GetCABundleRaw()
	}
	if !caCertPool.AppendCertsFromPEM([]byte(caBundle)) {
		return fmt.Errorf("failed to parse CA bundle into cert pool")
	}

	f.cliMTLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cliCert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true, // Verification handled via VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificates returned by gateway")
			}
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: caCertPool})
			if err != nil {
				return fmt.Errorf("gateway certificate failed verification: %w", err)
			}
			return nil
		},
	}
	return nil
}

// approvePlatformEnrollments waits for pending platform enrollment requests to
// appear, then approves operator, dashboard, and ensemble in the recommended
// order. The harness discovers request IDs via the authenticated pending
// endpoint (GET /api/v1/auth/platform-enrollments/pending) using the owner CLI
// mTLS identity, then posts approve decisions via the decision endpoint (POST
// /api/v1/auth/platform-enrollments/decision). The recommended order is
// operator, dashboard, then ensemble. The order is operational, not a security
// invariant — the gateway does not enforce prerequisite state between
// component approvals.
func (f *DockerE2EFixture) approvePlatformEnrollments() error {
	// Wait for pending requests to appear. The operator, dashboard, and
	// ensemble submit their requests shortly after the gateway becomes
	// healthy, but there is a startup race. Poll up to 60 seconds.
	var pending models.PlatformEnrollmentPendingResponse
	deadline := time.Now().Add(60 * time.Second)
	for {
		pending = f.fetchPendingEnrollments()
		if len(pending.Requests) > 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no pending platform enrollment requests appeared within 60s")
		}
		time.Sleep(2 * time.Second)
	}
	log.Printf("E2E: Discovered %d pending platform enrollment requests", len(pending.Requests))

	// Approve in the recommended order: operator, dashboard, ensemble.
	order := []models.PlatformComponentKind{
		models.PlatformComponentOperator,
		models.PlatformComponentDashboard,
		models.PlatformComponentEnsemble,
	}
	approved := 0
	for _, kind := range order {
		for _, req := range pending.Requests {
			if req.ComponentKind != kind {
				continue
			}
			if req.State != models.PlatformEnrollmentStatePending {
				continue
			}
			if err := f.approveEnrollment(req.RequestID); err != nil {
				return fmt.Errorf("approve %s enrollment %s: %w", kind, req.RequestID, err)
			}
			log.Printf("E2E: Approved %s enrollment (request %s)", kind, req.RequestID)
			approved++
		}
	}
	if approved == 0 {
		return fmt.Errorf("no pending platform enrollment requests were in the pending state")
	}
	return nil
}

// fetchPendingEnrollments calls the authenticated pending endpoint and returns
// the typed response. Uses the owner CLI mTLS identity. Returns an empty
// response (no error) if the endpoint returns no requests yet.
func (f *DockerE2EFixture) fetchPendingEnrollments() models.PlatformEnrollmentPendingResponse {
	if f.cliMTLSConfig == nil {
		return models.PlatformEnrollmentPendingResponse{}
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: f.cliMTLSConfig},
	}
	reqURL := f.GatewayHTTPSURL + constants.APIPaths.AuthPlatformEnrollmentPending
	resp, err := client.Get(reqURL)
	if err != nil {
		return models.PlatformEnrollmentPendingResponse{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.PlatformEnrollmentPendingResponse{}
	}
	var pending models.PlatformEnrollmentPendingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pending); err != nil {
		return models.PlatformEnrollmentPendingResponse{}
	}
	return pending
}

// fetchPendingRaw calls the authenticated pending endpoint and returns the
// raw JSON body as a string. Used by tests that need to verify the wire
// payload does not contain secret fields (token_hash, csr_pem, etc.).
func (f *DockerE2EFixture) fetchPendingRaw(t *testing.T) string {
	t.Helper()
	require.NotNil(t, f.cliMTLSConfig, "CLI mTLS credentials required for pending endpoint")
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: f.cliMTLSConfig},
	}
	reqURL := f.GatewayHTTPSURL + constants.APIPaths.AuthPlatformEnrollmentPending
	resp, err := client.Get(reqURL)
	require.NoError(t, err, "Failed to fetch pending list")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Pending endpoint returned non-200")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read pending list body")
	return string(body)
}

// approveEnrollment posts an approve decision for the given request ID via the
// authenticated decision endpoint. Uses the owner CLI mTLS identity.
func (f *DockerE2EFixture) approveEnrollment(requestID string) error {
	return f.postEnrollmentDecision(requestID, models.PlatformEnrollmentDecisionApprove)
}

// denyEnrollment posts a deny decision for the given request ID via the
// authenticated decision endpoint. Uses the owner CLI mTLS identity.
func (f *DockerE2EFixture) denyEnrollment(requestID string) error {
	return f.postEnrollmentDecision(requestID, models.PlatformEnrollmentDecisionDeny)
}

// postEnrollmentDecision posts a decision (approve or deny) for the given
// request ID via the authenticated decision endpoint. Uses the owner CLI mTLS
// identity.
func (f *DockerE2EFixture) postEnrollmentDecision(requestID string, decision models.PlatformEnrollmentDecision) error {
	if f.cliMTLSConfig == nil {
		return fmt.Errorf("no CLI mTLS credentials available")
	}
	body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
		RequestID: requestID,
		Decision:  decision,
	})
	if err != nil {
		return fmt.Errorf("marshal decision body: %w", err)
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: f.cliMTLSConfig},
	}
	reqURL := f.GatewayHTTPSURL + constants.APIPaths.AuthPlatformEnrollmentDecision
	resp, err := client.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("decision request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("decision returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// waitForGatewayHealth polls the gateway health endpoint until it returns 200
// or the timeout elapses. The gateway has no enrollment dependency and becomes
// healthy quickly, but the poll accounts for image build + startup time.
func (f *DockerE2EFixture) waitForGatewayHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gateway health endpoint not ready after %s", timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

// waitForWorkloadHealth polls the operator, dashboard, and ensemble containers
// until their healthchecks report healthy, or the timeout elapses. The
// healthchecks are truthful: the operator checks operator.crt existence, the
// dashboard checks its Express server, and the ensemble checks its FastAPI
// /health endpoint. All three transition to healthy only after platform
// enrollment completes.
func (f *DockerE2EFixture) waitForWorkloadHealth(timeout time.Duration) error {
	containers := []string{
		f.ContainerPrefix + "-operator",
		f.ContainerPrefix + "-dashboard",
		f.ContainerPrefix + "-ensemble",
	}
	deadline := time.Now().Add(timeout)
	for {
		allHealthy := true
		for _, c := range containers {
			state, err := f.containerHealthState(c)
			if err != nil || state != "healthy" {
				allHealthy = false
				break
			}
		}
		if allHealthy {
			return nil
		}
		if time.Now().After(deadline) {
			// Report which containers are not healthy for diagnostics.
			for _, c := range containers {
				state, _ := f.containerHealthState(c)
				log.Printf("E2E: %s health=%s (expected healthy)", c, state)
			}
			return fmt.Errorf("workloads not healthy after %s", timeout)
		}
		time.Sleep(3 * time.Second)
	}
}

// containerHealthState returns the Docker healthcheck state for a container
// ("healthy", "starting", "unhealthy", or "none").
func (f *DockerE2EFixture) containerHealthState(container string) (string, error) {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Health.Status}}", container)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// waitForContainerHealth polls a single container until its healthcheck reports
// healthy, or the timeout elapses.
func (f *DockerE2EFixture) waitForContainerHealth(container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		state, err := f.containerHealthState(container)
		if err == nil && state == "healthy" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s not healthy after %s (last state: %s)", container, timeout, state)
		}
		time.Sleep(3 * time.Second)
	}
}

// waitForContainerUnhealthy polls a single container until its healthcheck
// reports unhealthy, or the timeout elapses. Used by denial tests to confirm
// a denied component never becomes ready.
func (f *DockerE2EFixture) waitForContainerUnhealthy(container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		state, err := f.containerHealthState(container)
		if err == nil && state == "unhealthy" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s not unhealthy after %s (last state: %s)", container, timeout, state)
		}
		time.Sleep(3 * time.Second)
	}
}

// pendingRequestByKind returns the first pending platform enrollment request
// matching the given component kind, or false if none is found. Uses the
// authenticated pending endpoint.
func (f *DockerE2EFixture) pendingRequestByKind(kind models.PlatformComponentKind) (models.PlatformEnrollmentPendingRequest, bool) {
	pending := f.fetchPendingEnrollments()
	for _, req := range pending.Requests {
		if req.ComponentKind == kind && req.State == models.PlatformEnrollmentStatePending {
			return req, true
		}
	}
	return models.PlatformEnrollmentPendingRequest{}, false
}

// waitForPendingRequestByKind polls the authenticated pending endpoint until a
// pending request matching the given component kind appears, or the timeout
// elapses.
func (f *DockerE2EFixture) waitForPendingRequestByKind(kind models.PlatformComponentKind, timeout time.Duration) (models.PlatformEnrollmentPendingRequest, error) {
	deadline := time.Now().Add(timeout)
	for {
		if req, ok := f.pendingRequestByKind(kind); ok {
			return req, nil
		}
		if time.Now().After(deadline) {
			return models.PlatformEnrollmentPendingRequest{}, fmt.Errorf("no pending %s request appeared within %s", kind, timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

// restartContainer restarts a Docker container by name and returns the
// post-restart StartedAt timestamp (RFC3339). Used by restart-during-pending
// and lost-completion-response tests.
func (f *DockerE2EFixture) restartContainer(container string) (string, error) {
	restartCmd := exec.Command("docker", "restart", container)
	output, err := restartCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("restart %s: %w: %s", container, err, string(output))
	}
	startedAtCmd := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", container)
	startedAtOutput, err := startedAtCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect StartedAt for %s: %w", container, err)
	}
	return strings.TrimSpace(string(startedAtOutput)), nil
}

// GetCABundleRaw fetches the CA bundle from the gateway's well-known endpoint
// without a *testing.T (for use from setupSharedE2EFixture). Returns the raw
// PEM string.
func (f *DockerE2EFixture) GetCABundleRaw() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.GatewayHTTPURL + "/.well-known/g8e/pki/ca-bundle")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	bundle, _ := io.ReadAll(resp.Body)
	return string(bundle)
}

// mustEncodeECDSAPrivateKey encodes an ECDSA private key to SEC1 PEM. Used
// during bootstrap to build the CLI mTLS key pair from the bootstrap-returned
// cert and the locally-generated key.
func mustEncodeECDSAPrivateKey(key *ecdsa.PrivateKey) []byte {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		panic(fmt.Sprintf("marshal ECDSA private key: %v", err))
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})
}

// teardown stops the Docker Compose stack and removes volumes and orphans.
func (f *DockerE2EFixture) teardown() error {
	log.Printf("E2E: Stopping docker-compose (project: %s)...", f.ProjectName)
	composeEnv := []string{
		fmt.Sprintf("G8E_HTTP_PORT=%d", f.HTTPPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", f.HTTPSPort),
		fmt.Sprintf("G8E_ENSEMBLE_PORT=%d", f.EnsemblePort),
		fmt.Sprintf("G8E_DASHBOARD_PORT=%d", f.DashboardPort),
		fmt.Sprintf("G8E_PREFIX=%s", f.ContainerPrefix),
	}
	downCmd := exec.Command("docker", "compose", "-p", f.ProjectName, "-f", f.ComposeFile, "down", "-v", "--remove-orphans")
	downCmd.Dir = f.ProjectDir
	downCmd.Env = append(os.Environ(), composeEnv...)
	output, err := downCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w: %s", err, string(output))
	}
	log.Printf("E2E: Docker compose stopped")
	return nil
}

// NewDockerE2EFixture creates a per-test Docker E2E fixture. It spins up
// docker-compose, waits for health, and registers cleanup via t.Cleanup.
// For shared fixture across all tests, prefer TestMain + sharedFixture.
func NewDockerE2EFixture(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	fixture, err := setupSharedE2EFixture(composeFile)
	if err != nil {
		t.Fatalf("Failed to set up Docker E2E fixture: %v", err)
	}

	// Teardown cleanup: stops and removes the compose stack.
	t.Cleanup(func() {
		if err := fixture.teardown(); err != nil {
			t.Logf("Warning: failed to stop docker-compose: %v", err)
		}
	})

	// Failure-capture cleanup: registered AFTER teardown so it runs FIRST
	// (t.Cleanup is LIFO), while the containers are still up. Only captures
	// when the test actually failed, avoiding diagnostic noise on success.
	t.Cleanup(func() {
		if t.Failed() {
			fixture.captureDiagnostics(t.Logf)
		}
	})

	return fixture
}

// NewDockerE2EFixtureUpToBootstrap creates a per-test fixture that starts the
// full stack and bootstraps the first user with CLI mTLS credentials, but does
// NOT approve any platform enrollment requests. The returned fixture has
// pending requests that the test can approve, deny, or inspect. Cleanup is
// registered via t.Cleanup.
func NewDockerE2EFixtureUpToBootstrap(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	fixture, err := setupE2EFixtureUpToBootstrap(composeFile)
	if err != nil {
		t.Fatalf("Failed to set up Docker E2E fixture (up to bootstrap): %v", err)
	}

	t.Cleanup(func() {
		if err := fixture.teardown(); err != nil {
			t.Logf("Warning: failed to stop docker-compose: %v", err)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			fixture.captureDiagnostics(t.Logf)
		}
	})

	return fixture
}

// NewDockerE2EFixtureGatewayOnly creates a per-test fixture that starts only
// the gateway service (headless mode) and waits for gateway health. No
// workloads are started. Cleanup is registered via t.Cleanup.
func NewDockerE2EFixtureGatewayOnly(t *testing.T, composeFile string) *DockerE2EFixture {
	t.Helper()

	fixture, err := setupE2EFixtureGatewayOnly(composeFile)
	if err != nil {
		t.Fatalf("Failed to set up Docker E2E fixture (gateway only): %v", err)
	}

	t.Cleanup(func() {
		if err := fixture.teardown(); err != nil {
			t.Logf("Warning: failed to stop docker-compose: %v", err)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			fixture.captureDiagnostics(t.Logf)
		}
	})

	return fixture
}

// captureDiagnostics collects gateway/operator container logs and the compose
// ps state into files under a fresh temp dir, then logs the dir path via msg.
// Containers must still be up when called — invoke before teardown. msg is
// log.Printf for the TestMain path (no *testing.T available) and t.Logf for
// the per-test path. Purely diagnostic; changes no assertions.
func (f *DockerE2EFixture) captureDiagnostics(msg func(format string, args ...any)) {
	dir, err := os.MkdirTemp("", "g8e-e2e-diag-*")
	if err != nil {
		msg("E2E: failed to create diagnostics dir: %v", err)
		return
	}

	gatewayContainer := f.ContainerPrefix + "-gateway"
	operatorContainer := f.ContainerPrefix + "-operator"
	ensembleContainer := f.ContainerPrefix + "-ensemble"
	dashboardContainer := f.ContainerPrefix + "-dashboard"

	captures := []struct {
		name string
		cmd  *exec.Cmd
	}{
		{"gateway.log", exec.Command("docker", "logs", gatewayContainer)},
		{"operator.log", exec.Command("docker", "logs", operatorContainer)},
		{"ensemble.log", exec.Command("docker", "logs", ensembleContainer)},
		{"dashboard.log", exec.Command("docker", "logs", dashboardContainer)},
		{"compose-ps.txt", exec.Command("docker", "compose", "-p", f.ProjectName, "-f", f.ComposeFile, "ps")},
	}

	for _, c := range captures {
		out, runErr := c.cmd.CombinedOutput()
		path := filepath.Join(dir, c.name)
		if writeErr := os.WriteFile(path, out, constants.PermFilePublic); writeErr != nil {
			msg("E2E: failed to write %s: %v", c.name, writeErr)
			continue
		}
		if runErr != nil {
			msg("E2E: captured %s (command exited with error, see file)", c.name)
		}
	}

	msg("E2E: failure diagnostics written to %s", dir)
}

// GetHealth returns the health status from the gateway.
func (f *DockerE2EFixture) GetHealth(t *testing.T) map[string]interface{} {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	return health
}

// GetCABundle returns the CA bundle from the gateway.
func (f *DockerE2EFixture) GetCABundle(t *testing.T) string {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.GatewayHTTPURL + "/.well-known/g8e/pki/ca-bundle")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	bundle, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(bundle)
}

// CheckOperatorContainer checks if the operator container is running and has
// authentication success in logs. It waits for the operator to complete bootstrap
// authentication before asserting, since the operator may still be enrolling
// when the gateway first becomes healthy.
//
// Log windowing: uses `docker logs --since <container.StartedAt>` so only logs
// from the current container start are examined. After a restart, StartedAt
// updates to the new start time, excluding pre-restart stale log lines that
// would otherwise produce a false-positive match.
func (f *DockerE2EFixture) CheckOperatorContainer(t *testing.T) {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"

	// Check container status
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", opContainerName)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect operator container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Operator container is not running")

	// Get the container's start time for log windowing. This ensures we only
	// look at logs from the current container start, never stale logs from a
	// previous start (e.g. after a restart).
	startedAt := f.OperatorStartedAt(t)

	// Wait for operator to complete bootstrap authentication, using only
	// logs from the current container start.
	require.Eventually(t, func() bool {
		logsCmd := exec.Command("docker", "logs", "--since", startedAt, opContainerName)
		logsOutput, err := logsCmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(logsOutput), "Authentication successful")
	}, 120*time.Second, 2*time.Second, "Operator logs do not contain authentication success marker")
}

// OperatorStartedAt returns the container's State.StartedAt timestamp (RFC3339)
// for use as a `docker logs --since` argument. This is the canonical way to
// window logs to the current container start.
func (f *DockerE2EFixture) OperatorStartedAt(t *testing.T) string {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", opContainerName)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect operator container start time")
	return strings.TrimSpace(string(output))
}

// OperatorLogs returns the combined stdout/stderr logs of the operator container.
// This is a black-box observation helper — it only reads container logs, never
// accesses files inside the container or opens mTLS connections from the test process.
// Returns the FULL log buffer including pre-restart lines; for windowed access
// after a restart, use OperatorLogsSince.
func (f *DockerE2EFixture) OperatorLogs(t *testing.T) string {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"
	logsCmd := exec.Command("docker", "logs", opContainerName)
	logsOutput, err := logsCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get operator logs")
	return string(logsOutput)
}

// OperatorLogsSince returns the operator container logs since the given
// timestamp (RFC3339 format, as returned by OperatorStartedAt). Use this after
// a restart to examine only post-restart logs, avoiding stale pre-restart
// lines that would produce false-positive matches.
func (f *DockerE2EFixture) OperatorLogsSince(t *testing.T, sinceTS string) string {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"
	logsCmd := exec.Command("docker", "logs", "--since", sinceTS, opContainerName)
	logsOutput, err := logsCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get operator logs since %s", sinceTS)
	return string(logsOutput)
}

// RestartOperator restarts the operator container and waits for it to
// re-authenticate. Log windowing uses the post-restart container StartedAt
// timestamp so the re-auth assertion cannot be satisfied by pre-restart
// stale log lines. This is a black-box helper — it uses `docker restart`,
// HTTP health checks, and windowed log inspection only.
func (f *DockerE2EFixture) RestartOperator(t *testing.T) {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"

	t.Logf("Restarting operator container: %s", opContainerName)
	restartCmd := exec.Command("docker", "restart", opContainerName)
	restartOutput, err := restartCmd.CombinedOutput()
	require.NoError(t, err, "Failed to restart operator container: %s", string(restartOutput))

	// Capture the post-restart start time for log windowing. All subsequent
	// log checks use --since this timestamp, so pre-restart "Authentication
	// successful" lines cannot satisfy the re-auth assertion.
	startedAt := f.OperatorStartedAt(t)
	t.Logf("Operator restarted at %s, waiting for re-authentication", startedAt)

	// Wait for operator to re-authenticate by checking windowed logs for the
	// auth success marker.
	client := &http.Client{Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		// Verify gateway is still healthy
		resp, err := client.Get(f.GatewayHTTPURL + "/api/v1/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}

		// Check windowed operator logs for re-authentication
		logsCmd := exec.Command("docker", "logs", "--since", startedAt, opContainerName)
		logsOutput, err := logsCmd.CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(string(logsOutput), "Authentication successful")
	}, 120*time.Second, 2*time.Second, "Operator did not re-authenticate within 120s after restart")
}

// operatorSessionIDRe matches the slog-rendered structured field from the
// "Enrollment successful" log line (internal/cli/serve/cert.go:247), which
// logs operator_session_id as a key-value pair rendered as
// "  - operator_session_id: <uuid>". This is the authoritative source for
// the session ID: the operator sets it in its own process env via os.Setenv
// (internal/cli/serve/operator.go:250), which is NOT visible to `docker exec
// ... printenv` (a new process does not inherit the operator process's
// runtime os.Setenv, only the container's env metadata set by compose).
var operatorSessionIDRe = regexp.MustCompile(`operator_session_id:\s*([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// GetOperatorSessionID extracts the operator session ID from the operator's
// bootstrap logs. The session ID is logged as a structured field on the
// "Enrollment successful" line during platform enrollment completion. Uses
// windowed logs (since the container's current start) so a restart cannot
// surface a stale session ID from a previous start.
func (f *DockerE2EFixture) GetOperatorSessionID(t *testing.T) string {
	t.Helper()

	startedAt := f.OperatorStartedAt(t)
	logs := f.OperatorLogsSince(t, startedAt)
	m := operatorSessionIDRe.FindStringSubmatch(logs)
	require.Len(t, m, 2, "Operator logs since %s do not contain an operator_session_id; logs:\n%s", startedAt, logs)
	sessionID := strings.TrimSpace(m[1])
	require.NotEmpty(t, sessionID, "Operator session ID parsed from logs is empty")
	return sessionID
}

// GetOperatorBySession queries the gateway's GET /api/v1/operators/session/{id}
// endpoint and returns the operator document. The session-lookup route is
// registered on the full HTTPS handler (gateway_http_router.go:137), not the
// HTTP-only bootstrap router (buildHTTPRouter), so this helper hits the HTTPS
// port. The route defaults to RouteAuthMTLS (fail-closed for any path not
// explicitly public), so the client must present a valid client certificate —
// this helper uses the operator's own enrolled cert via operatorMTLSConfig.
func (f *DockerE2EFixture) GetOperatorBySession(t *testing.T, sessionID string) *models.OperatorDocumentGo {
	t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: f.operatorMTLSConfig(t),
		},
	}
	reqURL := f.GatewayHTTPSURL + constants.APIPaths.OperatorsSession + sessionID
	resp, err := client.Get(reqURL)
	require.NoError(t, err, "Failed to query operator session")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET /api/v1/operators/session/%s returned unexpected status", sessionID)

	var opResp models.OperatorResponse
	err = json.NewDecoder(resp.Body).Decode(&opResp)
	require.NoError(t, err, "Failed to decode operator response")
	require.True(t, opResp.Success, "Gateway returned success=false for operator session lookup")
	require.NotNil(t, opResp.Operator, "Gateway returned nil operator for session lookup")

	return opResp.Operator
}

// operatorMTLSConfig builds a *tls.Config using the operator's enrolled
// certificate and key (read from the operator container) and the gateway's CA
// bundle (fetched from the well-known PKI endpoint). The returned config
// presents the operator cert as the client certificate and verifies the
// gateway's server cert against the CA bundle via VerifyConnection. This is
// the exact identity the operator uses to communicate with the gateway, so
// requests made with this config are authenticated as the enrolled operator.
func (f *DockerE2EFixture) operatorMTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	opContainerName := f.ContainerPrefix + "-operator"

	// Read operator's enrolled cert and key from container
	certCmd := exec.Command("docker", "exec", opContainerName, "cat", constants.ContainerOperatorCert)
	certPEM, err := certCmd.CombinedOutput()
	require.NoError(t, err, "Failed to read operator cert from container: %s", string(certPEM))

	keyCmd := exec.Command("docker", "exec", opContainerName, "cat", constants.ContainerOperatorKey)
	keyPEM, err := keyCmd.CombinedOutput()
	require.NoError(t, err, "Failed to read operator key from container: %s", string(keyPEM))

	cliCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err, "Failed to parse operator X509 key pair")

	// Read gateway CA bundle from well-known endpoint
	caBundlePEM := f.GetCABundle(t)
	caCertPool := x509.NewCertPool()
	require.True(t, caCertPool.AppendCertsFromPEM([]byte(caBundlePEM)), "Failed to parse CA bundle into cert pool")

	return &tls.Config{
		Certificates:       []tls.Certificate{cliCert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true, // Verification handled via VerifyConnection against CA bundle
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificates returned by gateway")
			}
			opts := x509.VerifyOptions{
				Roots: caCertPool,
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			if err != nil {
				return fmt.Errorf("gateway certificate failed verification against CA bundle: %w", err)
			}
			return nil
		},
	}
}

// DialGatewayMTLS completes a real mTLS TLS handshake against the gateway's HTTPS port
// using the operator's enrolled certificate and key read from the operator container.
func (f *DockerE2EFixture) DialGatewayMTLS(t *testing.T) {
	t.Helper()

	tlsConfig := f.operatorMTLSConfig(t)

	addr := fmt.Sprintf("127.0.0.1:%d", f.HTTPSPort)
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	require.NoError(t, err, "mTLS handshake against gateway HTTPS port failed")
	defer conn.Close()

	state := conn.ConnectionState()
	require.True(t, state.HandshakeComplete, "TLS handshake did not complete")
	require.NotEmpty(t, state.PeerCertificates, "No peer certificates received from gateway")
}

// ensembleEnrolledMarkerRe matches the AppEnrollmentService success log line
// emitted from ensemble/app/services/infra/app_enrollment_service.py. Either
// the fresh-enrollment marker ("enrolled successfully") or the reuse marker
// ("reusing existing valid app cert") indicates the ensemble reached the
// identity-ready state. The "app cert saved" marker from _write_credentials is
// also accepted — it fires on every fresh enrollment before the success line.
var ensembleEnrolledMarkerRe = regexp.MustCompile(
	`AppEnrollmentService: (enrolled successfully|reusing existing valid app cert|app cert saved)`)

// GetEnsembleHealth queries the ensemble's public /health endpoint and asserts
// a 200 with status == "ok". The endpoint is served by FastAPI once the
// lifespan completes — including the AppEnrollmentService phase — so reaching
// it with status "ok" is indirect proof that enrollment succeeded and the app
// is serving.
func (f *DockerE2EFixture) GetEnsembleHealth(t *testing.T) map[string]interface{} {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.EnsembleHTTPURL + "/health")
	require.NoError(t, err, "Failed to query ensemble /health")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "ensemble /health returned non-200")

	var health map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health), "Failed to decode ensemble /health body")
	require.Equal(t, "ok", health["status"], "ensemble /health status != ok")
	return health
}

// GetEnsembleDetailedHealth queries the ensemble's /health/details endpoint and
// asserts a 200 with status == "ok". The `clients` map in the response reports
// service-object existence on app.state — `up` means the startup phase that
// creates the service object completed without raising, not that the underlying
// mTLS connection is live. Reaching this endpoint with all services `up` is
// indirect proof that enrollment succeeded (the lifespan exception handler
// re-raises on enrollment failure, so FastAPI never starts serving and this
// endpoint would be unreachable). For a direct mTLS connection probe, a future
// workstream could add a live connectivity check to the health endpoint.
func (f *DockerE2EFixture) GetEnsembleDetailedHealth(t *testing.T) map[string]interface{} {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.EnsembleHTTPURL + "/health/details")
	require.NoError(t, err, "Failed to query ensemble /health/details")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "ensemble /health/details returned non-200")

	var detailed map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detailed), "Failed to decode ensemble /health/details body")
	require.Equal(t, "ok", detailed["status"], "ensemble /health/details status != ok")
	return detailed
}

// CheckEnsembleContainer inspects the ensemble container, asserts it is
// running, and checks the container logs (windowed to the current container
// start) for an AppEnrollmentService success marker. The marker proves the
// ensemble completed platform enrollment (owner-approved) and has valid
// credentials.
func (f *DockerE2EFixture) CheckEnsembleContainer(t *testing.T) {
	t.Helper()

	ensembleContainer := f.ContainerPrefix + "-ensemble"

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", ensembleContainer)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect ensemble container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Ensemble container is not running")

	startedAt := f.EnsembleStartedAt(t)
	logs := f.EnsembleLogsSince(t, startedAt)
	require.True(t,
		ensembleEnrolledMarkerRe.MatchString(logs),
		"Ensemble logs since %s do not contain an AppEnrollmentService success marker; logs:\n%s",
		startedAt, logs,
	)
}

// EnsembleStartedAt returns the ensemble container's State.StartedAt timestamp
// (RFC3339) for use as a `docker logs --since` argument.
func (f *DockerE2EFixture) EnsembleStartedAt(t *testing.T) string {
	t.Helper()

	ensembleContainer := f.ContainerPrefix + "-ensemble"
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.StartedAt}}", ensembleContainer)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect ensemble container start time")
	return strings.TrimSpace(string(output))
}

// EnsembleLogsSince returns the ensemble container logs since the given
// RFC3339 timestamp (as returned by EnsembleStartedAt).
func (f *DockerE2EFixture) EnsembleLogsSince(t *testing.T, sinceTS string) string {
	t.Helper()

	ensembleContainer := f.ContainerPrefix + "-ensemble"
	logsCmd := exec.Command("docker", "logs", "--since", sinceTS, ensembleContainer)
	logsOutput, err := logsCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get ensemble logs since %s", sinceTS)
	return string(logsOutput)
}

// CheckDashboardContainer inspects the dashboard container, asserts it is
// running, and verifies the dashboard serves its index page via a single GET
// to the dashboard URL. The shared fixture's waitForWorkloadHealth has already
// confirmed the dashboard healthcheck is healthy before any test runs, so this
// is a single request without require.Eventually retry.
func (f *DockerE2EFixture) CheckDashboardContainer(t *testing.T) {
	t.Helper()

	dashboardContainer := f.ContainerPrefix + "-dashboard"

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", dashboardContainer)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to inspect dashboard container")
	status := strings.TrimSpace(string(output))
	require.Equal(t, "running", status, "Dashboard container is not running")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(f.DashboardHTTPURL + "/")
	require.NoError(t, err, "Failed to GET dashboard index page")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Dashboard index page returned non-200")
}
