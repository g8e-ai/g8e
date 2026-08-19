// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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
}

// setupSharedE2EFixture performs the Docker Compose setup without requiring
// a *testing.T, enabling use from TestMain. It allocates ports, builds and
// starts the stack, waits for health, and returns the fixture. On failure it
// tears down any partially-started stack before returning the error.
func setupSharedE2EFixture(composeFile string) (*DockerE2EFixture, error) {
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
	// The four ports share a single offset so the gateway, ensemble, and dashboard
	// in any given run all use the same offset — this keeps the per-run port set
	// contiguous and avoids collisions between concurrent E2E runs.
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

	// Build env for docker-compose (overrides defaults in compose file)
	composeEnv := []string{
		"DOCKER_BUILDKIT=1",
		fmt.Sprintf("G8E_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("G8E_HTTPS_PORT=%d", httpsPort),
		fmt.Sprintf("G8E_ENSEMBLE_PORT=%d", ensemblePort),
		fmt.Sprintf("G8E_DASHBOARD_PORT=%d", dashboardPort),
		fmt.Sprintf("G8E_PREFIX=%s", containerPrefix),
	}

	httpURL := fmt.Sprintf("http://localhost:%d", httpPort)
	httpsURL := fmt.Sprintf("https://localhost:%d", httpsPort)
	ensembleURL := fmt.Sprintf("http://localhost:%d", ensemblePort)
	dashboardURL := fmt.Sprintf("http://localhost:%d", dashboardPort)

	fixture := &DockerE2EFixture{
		GatewayHTTPURL:   httpURL,
		GatewayHTTPSURL:  httpsURL,
		EnsembleHTTPURL:  ensembleURL,
		DashboardHTTPURL: dashboardURL,
		ComposeFile:      composePath,
		ProjectDir:       repoRoot,
		ProjectName:      projectName,
		ContainerPrefix:  containerPrefix,
		HTTPPort:         httpPort,
		HTTPSPort:        httpsPort,
		EnsemblePort:     ensemblePort,
		DashboardPort:    dashboardPort,
	}

	// Spin up docker-compose with `--wait`, which blocks until every service
	// with a healthcheck reaches `healthy` (and services without one reach
	// `running`). Docker performs the readiness wait natively — there is no
	// Go-side health polling. The gateway's compose healthcheck hits
	// /api/v1/health, and the operator's `depends_on: condition:
	// service_healthy` ensures it only starts once the gateway is healthy.
	// `--wait-timeout` caps the total wait at 120s; on timeout or healthcheck
	// failure `up` exits non-zero, we tear down any partial stack, and R1's
	// fail-fast turns the returned error into a fatal suite exit.
	log.Printf("E2E: Starting docker-compose (project: %s), waiting for services to be healthy", projectName)
	upCmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "up", "-d", "--build", "--wait", "--wait-timeout", "120")
	upCmd.Dir = repoRoot
	upCmd.Env = append(os.Environ(), composeEnv...)
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		if tdErr := fixture.teardown(); tdErr != nil {
			log.Printf("E2E: teardown after compose-up failure also failed: %v", tdErr)
		}
		return nil, fmt.Errorf("docker compose up failed (services did not become healthy within 120s): %w\nOutput: %s", err, string(upOutput))
	}
	log.Printf("E2E: Docker compose stack is healthy: %s", string(upOutput))

	return fixture, nil
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
// "Enrollment successful" line during automatic enrollment. Uses windowed
// logs (since the container's current start) so a restart cannot surface a
// stale session ID from a previous start.
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
// ensemble completed self-enrollment against the gateway's public PKI app
// enrollment endpoint.
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
// to the dashboard URL. Because the dashboard has a compose healthcheck, the
// `docker compose up --wait` in setupSharedE2EFixture has already confirmed
// the dashboard is serving before any test runs, so this is a single request
// without require.Eventually retry.
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
