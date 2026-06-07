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

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	clicmd "github.com/g8e-ai/g8e/internal/cli/cmd"
	"github.com/g8e-ai/g8e/internal/cmd"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services"
	"github.com/g8e-ai/g8e/internal/services/execution"
	gateway "github.com/g8e-ai/g8e/internal/services/gateway"
	insecure_mcp "github.com/g8e-ai/g8e/internal/services/insecure_mcp"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
)

// Version information (set via ldflags during build)
var (
	version   string = string(constants.VersionStabilityDev)
	buildID   string = string(constants.SystemHealthUnknown)
	buildTime string = string(constants.SystemHealthUnknown)
	platform  string = string(constants.SystemHealthUnknown)
)

// parseCertPEM parses a PEM-encoded certificate file and returns the x509 certificate.
func parseCertPEM(certFile string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from certificate file")
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("PEM block is not a certificate (type: %s)", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// isCertExpiringSoon checks if a certificate is expiring within the renewal threshold.
// The threshold is set to 24 hours before expiry to allow ample time for renewal.
func isCertExpiringSoon(cert *x509.Certificate) bool {
	renewalThreshold := 24 * time.Hour
	timeUntilExpiry := time.Until(cert.NotAfter)
	return timeUntilExpiry <= renewalThreshold
}

// generateCSR generates a new ECDSA P-256 keypair and CSR for the given common name.
func generateCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

// renewOperatorCertificate performs automatic re-enrollment for the Operator certificate.
// This is a fail-closed operation: if renewal fails, it returns an error.
func renewOperatorCertificate(cfg *config.Config, clientCertFile, clientKeyFile string, clientIdentity *certs.ClientIdentity) error {
	expiringSoon, err := func() (bool, error) {
		cert, err := parseCertPEM(clientCertFile)
		if err != nil {
			return false, err
		}
		return isCertExpiringSoon(cert), nil
	}()
	if err != nil {
		return fmt.Errorf("failed to check certificate expiry: %w", err)
	}

	if !expiringSoon {
		return nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	opCSR, opKey, err := generateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate Operator CSR: %w", err)
	}

	cliCSR, _, err := generateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("failed to generate CLI CSR: %w", err)
	}

	// Load existing CLI certificate for mTLS
	cliCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return fmt.Errorf("failed to load CLI certificate: %w", err)
	}

	// Fetch current trust bundle from operator
	trustBundleURL := fmt.Sprintf("%s/.well-known/g8e/pki/ca-bundle", cfg.Endpoint)
	trustBundleResp, err := http.Get(trustBundleURL)
	if err != nil {
		return fmt.Errorf("failed to fetch trust bundle: %w", err)
	}
	defer trustBundleResp.Body.Close()

	if trustBundleResp.StatusCode != http.StatusOK {
		return fmt.Errorf("trust bundle fetch returned HTTP %d", trustBundleResp.StatusCode)
	}

	currentTrustBundle, err := io.ReadAll(trustBundleResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read trust bundle: %w", err)
	}

	if len(currentTrustBundle) == 0 {
		return fmt.Errorf("fetched trust bundle is empty")
	}

	// Update local trust bundle
	trustBundlePath := filepath.Join(filepath.Dir(clientCertFile), "g8eg-ca-bundle.pem")
	if err := os.WriteFile(trustBundlePath, currentTrustBundle, 0644); err != nil {
		return fmt.Errorf("failed to write trust bundle: %w", err)
	}

	// Create mTLS client
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(currentTrustBundle) {
		return fmt.Errorf("failed to parse CA certificates")
	}

	tlsConfig := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cliCert},
		MinVersion:   tls.VersionTLS13,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{Transport: transport}

	// Submit re-enrollment request
	reqBody := map[string]string{
		"csr_pem":            opCSR,
		"cli_csr_pem":        cliCSR,
		"system_fingerprint": fmt.Sprintf("g8e-operator-%s", hostname),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	enrollURL := fmt.Sprintf("%s/api/v1/pki/devices/enroll", cfg.Endpoint)
	httpReq, err := http.NewRequest("POST", enrollURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to re-enroll: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("re-enrollment failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var regResp struct {
		OperatorCert      string `json:"operator_cert"`
		OperatorCertChain string `json:"operator_cert_chain,omitempty"`
		CLICert           string `json:"cli_cert"`
		CLICertChain      string `json:"cli_cert_chain,omitempty"`
		HubTrustBundle    string `json:"hub_trust_bundle,omitempty"`
		Error             string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &regResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if regResp.Error != "" {
		return fmt.Errorf("re-enrollment failed: %s", regResp.Error)
	}

	if regResp.OperatorCert == "" || regResp.CLICert == "" {
		return fmt.Errorf("unexpected re-enrollment response (missing required fields)")
	}

	// Save renewed certificates
	keyBytes, err := x509.MarshalECPrivateKey(opKey)
	if err != nil {
		return fmt.Errorf("failed to marshal Operator private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	if err := os.WriteFile(clientKeyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write Operator key: %w", err)
	}

	certContent := regResp.OperatorCert
	if regResp.OperatorCertChain != "" {
		certContent += "\n" + regResp.OperatorCertChain
	}

	if err := os.WriteFile(clientCertFile, []byte(certContent), 0600); err != nil {
		return fmt.Errorf("failed to write Operator cert: %w", err)
	}

	// Update the client certificate via DI
	newCert, err := tls.X509KeyPair([]byte(certContent), keyPEM)
	if err != nil {
		return fmt.Errorf("failed to load renewed certificate: %w", err)
	}

	clientIdentity.SetCertificate(newCert)
	// Also set the global for compatibility during migration
	certs.SetClientCertificate(newCert)

	return nil
}

// runClientCertRenewalLoop runs a background goroutine that periodically checks
// and renews the client certificate if it is expiring soon.
func runClientCertRenewalLoop(ctx context.Context, cfg *config.Config, clientCertFile, clientKeyFile string, logger *slog.Logger, clientIdentity *certs.ClientIdentity) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Check immediately on startup
	if err := renewOperatorCertificate(cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
		logger.Error("Failed to renew client certificate on startup", string(constants.ConnectionStateError), err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Client certificate renewal loop stopped")
			return
		case <-ticker.C:
			if err := renewOperatorCertificate(cfg, clientCertFile, clientKeyFile, clientIdentity); err != nil {
				logger.Error("Failed to renew client certificate", string(constants.ConnectionStateError), err)
			} else {
				logger.Info("Client certificate renewal check completed")
			}
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == string(constants.ApprovalTypeStream) {
		cmd.RunStream(os.Args[2:])
		return
	}

	// Check for CLI subcommands (gw, gateway, apps, auth, data, evals, security, setup, vars, test)
	cliSubcommands := map[string]bool{
		"gw":       true,
		"gateway":  true,
		"apps":     true,
		"auth":     true,
		"data":     true,
		"evals":    true,
		"security": true,
		"setup":    true,
		"vars":     true,
		"test":     true,
		"auditor":  true,
		"chaos":    true,
		"mcp":      true,
		"operator": true,
	}

	if len(os.Args) > 1 && cliSubcommands[os.Args[1]] {
		// Delegate to CLI commands
		clicmd.Execute()
		os.Exit(0)
	}

	settings := config.LoadSettings()

	// Capture the launch directory before any flag parsing or os.Chdir calls.
	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to determine working directory: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	var privateKey string
	var clientCert string
	var endpointURL string
	var trustBundlePath string
	var workingDir string
	var cloudMode bool
	var cloudProvider string
	var localStorage bool
	var logLevel string
	var showVersion bool

	var noGit bool

	var httpPort int

	var doctrineMode bool
	var consensusMode bool
	var notaryMode bool
	var gatewayHTTPPort int
	var gatewayHTTPSPort int
	var gatewayDataDir string
	var gatewayPKIDir string
	var gatewaySecretsDir string
	var gatewayVaultDir string
	var gatewayVaultKeyPath string
	var gatewayVaultRequireUnlock bool
	var gatewayPasskeyRpID string
	var gatewayPasskeyRpName string
	var gatewayRateLimitRPS float64
	var gatewayRateLimitBurst int
	var gatewayCertIdentityMode string
	var gatewayNetworkIdentityFile string
	var insecureMode bool
	var insecureURL string
	var insecureToken string
	var insecureNodeID string
	var insecureDisplayName string

	var heartbeatInterval time.Duration

	var rekeyVault bool
	var oldPrivateKeyStr string
	var verifyVault bool
	var resetVault bool
	flag.StringVar(&privateKey, "k", "", "Private key")
	flag.StringVar(&clientCert, "cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&endpointURL, "e", "", "Endpoint (hostname or IP)")
	flag.BoolVar(&cloudMode, "c", true, "Cloud mode")
	flag.StringVar(&cloudProvider, "p", "", "Cloud provider")
	flag.BoolVar(&localStorage, "s", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "l", "info", "Log level")
	flag.BoolVar(&noGit, "G", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "v", false, "Version")
	flag.StringVar(&privateKey, "key", "", "Private key")
	flag.StringVar(&clientCert, "client-cert", "", "Client certificate (for mTLS)")
	flag.StringVar(&endpointURL, "endpoint", "", "Endpoint (hostname or IP)")
	flag.StringVar(&trustBundlePath, "trust-bundle", "", "Path to trust bundle PEM file (default: "+constants.CACertLegacyBundlePath+" or fetch from /.well-known/g8e/pki/ca-bundle)")
	flag.StringVar(&workingDir, "working-dir", "", "Working directory (default: directory Operator was launched from)")
	flag.BoolVar(&cloudMode, string(constants.OperatorTypeCloud), true, "Cloud mode")
	flag.StringVar(&cloudProvider, "provider", "", "Cloud provider")
	flag.BoolVar(&localStorage, "local-storage", true, "Enable local storage (stores data in current directory)")
	flag.StringVar(&logLevel, "log", "info", "Log level")
	flag.BoolVar(&noGit, "no-git", false, "Disable git (ledger)")
	flag.BoolVar(&showVersion, "version", false, "Version")

	flag.BoolVar(&doctrineMode, "doctrine", false, "Gateway mode: L1 enforced, L2/L3 audited (default)")
	flag.BoolVar(&consensusMode, "consensus", false, "Gateway mode: L1/L2 enforced, L3 audited")
	flag.BoolVar(&notaryMode, "notary", false, "Gateway mode: L1/L2/L3 strictly enforced")
	flag.IntVar(&gatewayHTTPPort, "http-port", constants.Ports.OperatorHttp, "HTTP port for bootstrap and MCP routes (default: from paths.json)")
	flag.IntVar(&gatewayHTTPSPort, "https-port", constants.Ports.OperatorHttps, "HTTPS port for mTLS API and public surface (default: from paths.json)")
	flag.StringVar(&gatewayDataDir, "data-dir", "", "Data directory for SQLite database (default: "+constants.Paths.Infra.DataDir+" in working directory)")
	flag.StringVar(&gatewayPKIDir, "pki-dir", "", "Directory for TLS certificates (default: "+constants.Paths.Infra.PkiDir+")")
	flag.StringVar(&gatewaySecretsDir, "secrets-dir", "", "Directory for platform secrets (default: "+constants.Paths.Infra.SecretsDir+")")
	flag.StringVar(&gatewayVaultDir, "vault-dir", "", "Directory for vault data (default: .g8e/vault)")
	flag.StringVar(&gatewayVaultKeyPath, "vault-key", "", "Path to vault private key (default: .g8e/secrets/vault.key)")
	flag.BoolVar(&gatewayVaultRequireUnlock, "vault-require-unlock", false, "Require vault to be unlocked at startup (fail if vault cannot be unlocked)")
	flag.StringVar(&gatewayPasskeyRpID, "passkey-rp-id", "", "RP ID for passkey operations (default: localhost)")
	flag.StringVar(&gatewayPasskeyRpName, "passkey-rp-name", "", "RP Name for passkey operations (default: g8e)")
	flag.Float64Var(&gatewayRateLimitRPS, "rate-limit-rps", 5.0, "Gateway requests per second limit (set to 0 to disable)")
	flag.IntVar(&gatewayRateLimitBurst, "rate-limit-burst", 10, "Gateway rate limit burst size")
	flag.StringVar(&gatewayCertIdentityMode, "cert-mode", "", "Certificate mode: full (all hostnames/IPs), localhost (only localhost)")
	flag.StringVar(&gatewayNetworkIdentityFile, "network-identity-file", "", "Path to JSON file containing pre-detected network identity")
	flag.BoolVar(&rekeyVault, "rekey-vault", false, "Re-encrypt vault with new private key (requires --old-key)")
	flag.StringVar(&oldPrivateKeyStr, "old-key", "", "Old private key for vault re-keying")
	flag.BoolVar(&verifyVault, "verify-vault", false, "Verify vault integrity")
	flag.BoolVar(&resetVault, "reset-vault", false, "Reset vault (DESTROYS ALL DATA)")

	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", 0, "Heartbeat interval (e.g. 60s, 2m); overrides the 30s default")

	flag.BoolVar(&insecureMode, "insecure", false, "INSECURE mode: connect to MCP gateway without governance (DANGEROUS - bypasses all L1/L2/L3 verification)")
	flag.StringVar(&insecureURL, "insecure-url", "", "MCP Gateway WebSocket URL (e.g. ws://"+constants.DefaultEndpoint+":18789)")
	flag.StringVar(&insecureToken, "insecure-token", "", "MCP Gateway auth token")
	flag.StringVar(&insecureNodeID, "insecure-node-id", "", "Node ID to advertise (default: hostname)")
	flag.StringVar(&insecureDisplayName, "insecure-name", "", "Display name shown in MCP gateway UI (default: node ID)")

	// Customize usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: g8e [options]\n")
		fmt.Fprintf(os.Stderr, "   or: g8e <command> [command-options]\n\n")
		fmt.Fprintf(os.Stderr, "Platform Commands:\n")
		fmt.Fprintf(os.Stderr, "  gw          Gateway lifecycle (start, stop, status, logs)\n")
		fmt.Fprintf(os.Stderr, "  auth        Authentication (login, logout)\n")
		fmt.Fprintf(os.Stderr, "  approve     Approve suspended L3 transactions\n")
		fmt.Fprintf(os.Stderr, "  data        Data operations (export, import, query)\n")
		fmt.Fprintf(os.Stderr, "  test        Run platform tests\n")
		fmt.Fprintf(os.Stderr, "  security    Security operations (pki, certificates)\n")
		fmt.Fprintf(os.Stderr, "  auditor     Governance auditor (list, run, audit, self-test)\n")
		fmt.Fprintf(os.Stderr, "  chaos       Chaos testing (generate governance events)\n")
		fmt.Fprintf(os.Stderr, "  mcp         MCP protocol operations\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -k, --key <key>         Private key\n")
		fmt.Fprintf(os.Stderr, "  -e, --endpoint <host>     Operator endpoint: IP address or hostname of the operator\n")
		fmt.Fprintf(os.Stderr, "      --trust-bundle <path> Path to trust bundle PEM file (default: "+constants.CACertLegacyBundlePath+" or fetch from /.well-known/g8e/pki/ca-bundle)\n")
		fmt.Fprintf(os.Stderr, "      --working-dir <dir>   Working directory (default: directory Operator was launched from)\n")
		fmt.Fprintf(os.Stderr, "                            All commands and data storage are anchored to this directory\n")
		fmt.Fprintf(os.Stderr, "  -c, --cloud             Cloud Operator mode (for AWS/cloud CLI)\n")
		fmt.Fprintf(os.Stderr, "  -p, --provider <name>   Cloud provider: aws, gcp, azure\n")
		fmt.Fprintf(os.Stderr, "  -s, --local-storage     Store audit data locally instead of cloud (default: on)\n")
		fmt.Fprintf(os.Stderr, "                          When enabled, data is stored in ./%s/ relative to launch directory\n", constants.Paths.Infra.RuntimeDir)
		fmt.Fprintf(os.Stderr, "  -l, --log <level>       Log level: info, error, debug (default: info)\n")
		fmt.Fprintf(os.Stderr, "  -G, --no-git            Disable ledger (git-backed file versioning)\n")
		fmt.Fprintf(os.Stderr, "      --heartbeat-interval <dur> Heartbeat interval (e.g. 60s, 2m); overrides the 30s default\n")
		fmt.Fprintf(os.Stderr, "  -v, --version           Show version\n")
		fmt.Fprintf(os.Stderr, "\nGateway Mode (platform persistence + pub/sub broker):\n")
		fmt.Fprintf(os.Stderr, "  --doctrine                Gateway mode: L1 enforced, L2/L3 audited (default)\n")
		fmt.Fprintf(os.Stderr, "  --consensus               Gateway mode: L1/L2 enforced, L3 audited\n")
		fmt.Fprintf(os.Stderr, "  --notary                  Gateway mode: L1/L2/L3 strictly enforced\n")
		fmt.Fprintf(os.Stderr, "  --http-port <port>        HTTP port for bootstrap and MCP routes (default: %d)\n", constants.Ports.OperatorHttp)
		fmt.Fprintf(os.Stderr, "  --https-port <port>       HTTPS port for mTLS API and public surface (default: %d)\n", constants.Ports.OperatorHttps)
		fmt.Fprintf(os.Stderr, "  --data-dir <dir>            Data directory for SQLite (default: %s in working directory)\n", constants.Paths.Infra.DataDir)
		fmt.Fprintf(os.Stderr, "  --pki-dir <dir>             Directory for TLS certificates (default: %s)\n", constants.Paths.Infra.PkiDir)
		fmt.Fprintf(os.Stderr, "  --secrets-dir <dir>         Directory for platform secrets (default: %s)\n", constants.Paths.Infra.SecretsDir)
		fmt.Fprintf(os.Stderr, "  --vault-dir <dir>           Directory for vault data (default: .g8e/vault)\n")
		fmt.Fprintf(os.Stderr, "  --vault-key <path>          Path to vault private key (default: .g8e/secrets/vault.key)\n")
		fmt.Fprintf(os.Stderr, "  --vault-require-unlock     Require vault to be unlocked at startup (fail if vault cannot be unlocked)\n")
		fmt.Fprintf(os.Stderr, "  --passkey-rp-id <id>        RP ID for passkey operations (default: localhost)\n")
		fmt.Fprintf(os.Stderr, "  --passkey-rp-name <name>    RP Name for passkey operations (default: g8e)\n")
		fmt.Fprintf(os.Stderr, "  --rate-limit-rps <rps>      Requests per second limit (default: 5.0, set to 0 to disable)\n")
		fmt.Fprintf(os.Stderr, "  --rate-limit-burst <burst>  Rate limit burst size (default: 10)\n")
		fmt.Fprintf(os.Stderr, "  --cert-mode <mode>         Certificate mode: full (all hostnames/IPs), localhost (only localhost)\n")
		fmt.Fprintf(os.Stderr, "  --network-identity-file <path> Path to JSON file containing pre-detected network identity\n")
		fmt.Fprintf(os.Stderr, "\nVault Management:\n")
		fmt.Fprintf(os.Stderr, "  --rekey-vault           Re-encrypt vault with new API key\n")
		fmt.Fprintf(os.Stderr, "  --old-key <key>         Old API key (required for --rekey-vault)\n")
		fmt.Fprintf(os.Stderr, "  --verify-vault          Verify vault integrity\n")
		fmt.Fprintf(os.Stderr, "  --reset-vault           Reset vault (DESTROYS ALL DATA)\n")
		fmt.Fprintf(os.Stderr, "\nInsecure MCP Node Host Mode:\n")
		fmt.Fprintf(os.Stderr, "  --insecure              Connect to MCP gateway without governance (DANGEROUS - bypasses all L1/L2/L3 verification)\n")
		fmt.Fprintf(os.Stderr, "  --insecure-url <url>    MCP Gateway WebSocket URL (e.g. ws://"+constants.DefaultEndpoint+":18789)\n")
		fmt.Fprintf(os.Stderr, "  --insecure-token <tok>  Auth token\n")
		fmt.Fprintf(os.Stderr, "  --insecure-node-id <id> Node ID advertised to the Gateway (default: hostname)\n")
		fmt.Fprintf(os.Stderr, "  --insecure-name <name>  Display name shown in MCP gateway UI (default: node ID)\n")
		fmt.Fprintf(os.Stderr, "\nExample Scenarios:\n")
		fmt.Fprintf(os.Stderr, "  # Initialize Gateway in Doctrine Mode\n")
		fmt.Fprintf(os.Stderr, "  ./g8e gw start\n\n")
		fmt.Fprintf(os.Stderr, "  # Authenticate and bootstrap PKI\n")
		fmt.Fprintf(os.Stderr, "  ./g8e auth login\n\n")
		fmt.Fprintf(os.Stderr, "  # Deploy an Operator on a remote host\n")
		fmt.Fprintf(os.Stderr, "  ./g8e security pki enroll --endpoint <gateway-ip>\n\n")
		fmt.Fprintf(os.Stderr, "  # Verify platform status\n")
		fmt.Fprintf(os.Stderr, "  ./g8e gw status\n\n")
		fmt.Fprintf(os.Stderr, "  # Query audit trail\n")
		fmt.Fprintf(os.Stderr, "  ./g8e data query --collection audit_vault\n\n")
		fmt.Fprintf(os.Stderr, "  # Start Gateway in Notary Mode (L1/L2/L3 enforced)\n")
		fmt.Fprintf(os.Stderr, "  ./g8e gw start --posture notary\n")
	}

	flag.Parse()

	// Show help if no arguments provided
	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	if showVersion {
		printVersion()
		os.Exit(constants.ExitSuccess)
	}

	if rekeyVault || verifyVault || resetVault {
		vaultWorkDir := launchDir
		if workingDir != "" {
			vaultWorkDir = workingDir
		}
		handleVaultCommand(rekeyVault, verifyVault, resetVault, privateKey, oldPrivateKeyStr, logLevel, vaultWorkDir)
		return
	}

	// Check for mutually exclusive posture flags
	postureCount := 0
	var posture config.GatewayPosture
	if doctrineMode {
		postureCount++
		posture = config.PostureDoctrine
	}
	if consensusMode {
		postureCount++
		posture = config.PostureConsensus
	}
	if notaryMode {
		postureCount++
		posture = config.PostureNotary
	}

	if postureCount > 1 {
		fmt.Fprintf(os.Stderr, "Error: Only one of --doctrine, --consensus, or --notary may be specified\n")
		os.Exit(constants.ExitConfigError)
	}

	if postureCount > 0 {
		// Environment variables override CLI flags
		if gatewayVaultDir == "" {
			gatewayVaultDir = os.Getenv("G8E_VAULT_DIR")
		}
		if gatewayVaultKeyPath == "" {
			gatewayVaultKeyPath = os.Getenv("G8E_VAULT_KEY")
		}
		if !gatewayVaultRequireUnlock {
			gatewayVaultRequireUnlock = os.Getenv("G8E_VAULT_REQUIRE_UNLOCK") == "true"
		}
		runGatewayMode(posture, gatewayHTTPPort, gatewayHTTPSPort, gatewayDataDir, gatewayPKIDir, gatewaySecretsDir, gatewayVaultDir, gatewayVaultKeyPath, gatewayVaultRequireUnlock, gatewayPasskeyRpID, gatewayPasskeyRpName, gatewayRateLimitRPS, gatewayRateLimitBurst, logLevel, gatewayCertIdentityMode, gatewayNetworkIdentityFile)
		return
	}

	if insecureMode {
		runInsecureMode(insecureURL, insecureToken, insecureNodeID, insecureDisplayName, os.Getenv("PATH"), logLevel)
		return
	}

	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := constants.DefaultEndpoint
	if strings.TrimSpace(endpointURL) != "" {
		operatorEndpoint = strings.TrimSpace(endpointURL)
	}

	logger.Info("g8e", "version", version, "build", buildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	// Instantiate DI types for trust and client identity
	trustStore := certs.NewTrustStore(nil)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})

	// Load trust bundle for TLS verification. Priority:
	// 1. Explicit --trust-bundle path
	// 2. Local PKI directory ("+constants.CACertLegacyBundlePath+")
	// 3. Fetch from Operator /.well-known/g8e/pki/ca-bundle endpoint
	trustLoaded := loadTrustBundle(logger, trustBundlePath, workingDir, trustStore)
	if !trustLoaded {
		if endpointURL != "" {
			trustURL := fmt.Sprintf("http://%s:%d/.well-known/g8e/pki/ca-bundle", endpointURL, constants.Ports.OperatorHttp)
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			if err := certs.FetchAndSetCA(context.Background(), trustURL, ""); err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, string(constants.ConnectionStateError), err)
				fmt.Fprintf(os.Stderr, "Failed to fetch trust bundle from Operator: %v\n", err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e gw start\n")
				os.Exit(constants.ExitConfigError)
			}
			// Also set in trustStore for DI
			trustStore.SetCA(certs.GetRawCA())
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "Error: No trust bundle available. Provide --trust-bundle or --endpoint\n")
			os.Exit(constants.ExitConfigError)
		}
	}
	logger.Info("Trust bundle loaded")

	// Resolve default client certificate paths if not explicitly provided
	// Priority: 1. Explicit flags, 2. Project-local .g8e/pki/operator.*, 3. Project-local .g8e/pki/client.*
	if privateKey == "" {
		// Try project-local Operator key (created by enrollment)
		projectOperatorKey := filepath.Join(launchDir, ".g8e/pki/operator.key")
		if _, err := os.Stat(projectOperatorKey); err == nil {
			privateKey = projectOperatorKey
			logger.Info("Using default Operator key from project directory", "path", privateKey)
		} else {
			// Try project-local client key
			projectKey := filepath.Join(launchDir, ".g8e/pki/client.key")
			if _, err := os.Stat(projectKey); err == nil {
				privateKey = projectKey
				logger.Info("Using default client key from project directory", "path", privateKey)
			}
		}
	}

	if clientCert == "" {
		// Try project-local Operator cert (created by enrollment)
		projectOperatorCert := filepath.Join(launchDir, ".g8e/pki/operator.crt")
		if _, err := os.Stat(projectOperatorCert); err == nil {
			clientCert = projectOperatorCert
			logger.Info("Using default Operator certificate from project directory", "path", clientCert)
		} else {
			// Try project-local client cert
			projectCert := filepath.Join(launchDir, ".g8e/pki/client.crt")
			if _, err := os.Stat(projectCert); err == nil {
				clientCert = projectCert
				logger.Info("Using default client certificate from project directory", "path", clientCert)
			}
		}
	}

	if privateKey == "" {
		fmt.Fprintf(os.Stderr, "Private key is required (-k or --key). Expected locations:\n")
		fmt.Fprintf(os.Stderr, "  - .g8e/pki/operator.key (project directory)\n")
		fmt.Fprintf(os.Stderr, "  - .g8e/pki/client.key (project directory)\n")
		os.Exit(constants.ExitConfigError)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "Client certificate is required (--cert or --client-cert). Expected locations:\n")
		fmt.Fprintf(os.Stderr, "  - .g8e/pki/operator.crt (project directory)\n")
		fmt.Fprintf(os.Stderr, "  - .g8e/pki/client.crt (project directory)\n")
		os.Exit(constants.ExitConfigError)
	}

	// Load client certificate for mTLS
	certPEM, err := os.ReadFile(clientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read client certificate: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	keyPEM, err := os.ReadFile(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load client certificate/key pair: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	clientIdentity.SetCertificate(cert)
	// Also set the global for compatibility during migration
	certs.SetClientCertificate(cert)

	// Resolve the effective working directory: flag overrides launch dir.
	effectiveWorkDir := launchDir
	if workingDir != "" {
		effectiveWorkDir = workingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		OperatorEndpoint: operatorEndpoint,

		HTTPPort:            httpPort,
		CloudMode:           cloudMode,
		CloudProvider:       cloudProvider,
		LocalStorageEnabled: localStorage,
		NoGit:               noGit,
		LogLevel:            logLevel,
		WorkDir:             effectiveWorkDir,
		PKIDir:              settings.PKIDir,
		SecretsDir:          settings.SecretsDir,
		HeartbeatInterval:   heartbeatInterval,
		Shell:               os.Getenv("SHELL"),
		Lang:                os.Getenv("LANG"),
		Term:                os.Getenv("TERM"),
		TZ:                  os.Getenv("TZ"),
		Posture:             "", // Will default to PostureNotary in Load() since L3Notary is nil
	})
	if err != nil {
		logger.Error("Failed to load configuration", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}

	cfg.Version = version

	if cfg.CloudMode {
		logger.Info("Cloud Operator mode enabled", "provider", cfg.CloudProvider)
	}

	if cfg.LocalStoreEnabled {
		logger.Info("Local storage enabled - data stays in working directory", "db_path", cfg.LocalStoreDBPath, "working_dir", cfg.WorkDir)
	} else {
		logger.Info("Local storage disabled (command output sent to cloud)")
	}

	g8eoService, err := services.NewG8eoService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create Operator service", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := g8eoService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e", string(constants.ConnectionStateError), err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	// Start background client certificate renewal loop
	if clientCert != "" && privateKey != "" {
		go runClientCertRenewalLoop(ctx, cfg, clientCert, privateKey, logger, clientIdentity)
	}

	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)

	if err := g8eoService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", string(constants.ConnectionStateError), err)
	}
	shutdownCancel()

	cancel()
	os.Exit(constants.ExitSuccess)
}

func printVersion() {
	fmt.Printf("g8e\n  Version:   %s\n  Build ID:  %s\n  Build Time: %s\n  Platform:  %s\n", version, buildID, buildTime, platform)
}

// loadTrustBundle attempts to read a trust bundle from:
// 1. Explicit path provided via --trust-bundle
// 2. Working directory PKI path ("+constants.Paths.Infra.CaCertPath+")
// Returns true on the first valid PEM found, which is installed via
// trustStore.SetCA. Returns false if no valid trust bundle is found.
func loadTrustBundle(logger *slog.Logger, explicitPath, workingDir string, trustStore *certs.TrustStore) bool {
	pathsToCheck := []string{}

	if explicitPath != "" {
		pathsToCheck = append(pathsToCheck, explicitPath)
	}

	if workingDir != "" {
		pkiPath := filepath.Join(workingDir, constants.Paths.Infra.CaCertPath)
		pathsToCheck = append(pathsToCheck, pkiPath)
	}

	for _, path := range pathsToCheck {
		pemData, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		logger.Info("Loading trust bundle from local path", "path", path)
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			logger.Warn("CA file exists but contains invalid certificate", "path", path)
			continue
		}
		trustStore.SetCA(pemData)
		// Also set the global for compatibility during migration
		certs.SetCA(pemData)
		logger.Info("CA certificate loaded from local file")
		return true
	}
	return false
}

// configureLogger returns a slog logger configured with operator-friendly formatting
func configureLogger(level string) (*slog.Logger, error) {
	parsedLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}

	handler := newOperatorHandler(os.Stdout, parsedLevel)
	logger := slog.New(handler)

	return logger, nil
}

// parseLogLevel validates and converts CLI input into slog levels
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info":
		return slog.LevelInfo, nil
	case string(constants.ConnectionStateError):
		return slog.LevelError, nil
	case "debug":
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, fmt.Errorf("supported values are: info, error, debug")
	}
}

// operatorHandler is a custom slog.Handler for operator-friendly log formatting
type operatorHandler struct {
	level  slog.Level
	output io.Writer
	attrs  []slog.Attr
	groups []string
}

func newOperatorHandler(output io.Writer, level slog.Level) *operatorHandler {
	return &operatorHandler{
		level:  level,
		output: output,
	}
}

func (h *operatorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *operatorHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := r.Time.In(time.Local).Format(time.RFC3339)
	levelStr := strings.ToUpper(r.Level.String())

	msg := fmt.Sprintf("%s %s: %s", timestamp, levelStr, r.Message)

	attrs := make([]slog.Attr, 0, r.NumAttrs()+len(h.attrs))
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	if len(attrs) > 0 {
		for _, attr := range attrs {
			msg += fmt.Sprintf("\n  - %s: %v", attr.Key, attr.Value.Any())
		}
	}

	msg += "\n"
	_, err := h.output.Write([]byte(msg))
	return err
}

func (h *operatorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *operatorHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &operatorHandler{
		level:  h.level,
		output: h.output,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// runGatewayMode starts the Operator in gateway mode - the platform's central
// persistence (operator) and pub/sub broker. In this mode, the Operator also
// runs an in-process command service to act as the sovereign execution Gateway.
func runGatewayMode(posture config.GatewayPosture, httpPort, httpsPort int, dataDir, pkiDir, secretsDir, vaultDir, vaultKeyPath string, vaultRequireUnlock bool, passkeyRpID, passkeyRpName string, rateLimitRPS float64, rateLimitBurst int, logLevel, certIdentityMode, networkIdentityFile string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	// Initialize paths relative to current working directory
	constants.InitPaths()

	// Apply defaults for empty directory flags (constants are now absolute)
	if dataDir == "" {
		dataDir = constants.Paths.Infra.DataDir
	}
	if pkiDir == "" {
		pkiDir = constants.Paths.Infra.PkiDir
	}
	if secretsDir == "" {
		secretsDir = constants.Paths.Infra.SecretsDir
	}

	logger.Info("Gateway paths configured", "data_dir", dataDir, "pki_dir", pkiDir, "secrets_dir", secretsDir)

	logger.Info("g8e - Gateway Mode",
		"posture", posture,
		"version", version,
		"build", buildID)

	cfg, err := config.LoadGateway(config.GatewayOptions{
		Posture:             posture,
		HTTPPort:            httpPort,
		HTTPSPort:           httpsPort,
		DataDir:             dataDir,
		PKIDir:              pkiDir,
		SecretsDir:          secretsDir,
		PasskeyRpID:         passkeyRpID,
		PasskeyRpName:       passkeyRpName,
		RateLimitRPS:        rateLimitRPS,
		RateLimitBurst:      rateLimitBurst,
		CertMode:            certIdentityMode,
		NetworkIdentityFile: networkIdentityFile,
		MCPDownstreamURL:    "",
		A2ADownstreamURL:    "",
		AllowTestPortZero:   false,
	})
	if err != nil {
		logger.Error("Failed to load gateway configuration", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}
	cfg.Version = version

	svc, err := gateway.NewGatewayModeService(cfg, logger)
	if err != nil {
		logger.Error("Failed to create gateway service", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize In-Process Execution Gateway
	logger.Info("Initializing in-process execution Gateway...")
	execSvc := execution.NewExecutionService(cfg, logger)
	fileSvc := execution.NewFileEditService(cfg, logger)

	// Resolve Git for ledger
	gitPath := system.ResolveGitBinary(logger)
	cfg.GitPath = gitPath
	cfg.GitAvailable = gitPath != ""

	// Use the gateway-mode database for everything
	govDeps := svc.GetGovernanceDeps()
	sm, err := svc.GetSecretManager()
	if err != nil {
		logger.Error("Failed to get secret manager", string(constants.ConnectionStateError), err)
		cancel()
		os.Exit(constants.ExitConfigError)
	}

	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	if err != nil {
		logger.Error("Failed to load Actuator signing key - mutations will fail", string(constants.ConnectionStateError), err)
		cancel()
		os.Exit(constants.ExitConfigError)
	}

	ConsensusPriv, err := sm.GetConsensusKey()
	if err != nil {
		logger.Error("Failed to load Consensus signing key - L2 consensus will fail", string(constants.ConnectionStateError), err)
		cancel()
		os.Exit(constants.ExitConfigError)
	}

	// Export Actuator public key for receipt verification by evals harness
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	logger.Info("Exporting Actuator public key", "pki_dir", cfg.PKIDir, "key_id", ActuatorKeyID)
	if err := exportActuatorPublicKey(cfg.PKIDir, ActuatorPub, ActuatorKeyID, logger); err != nil {
		logger.Warn("Failed to export Actuator public key for evals harness receipt verification", "error", err)
	}

	// Loopback Pub/Sub for in-process command dispatch
	loopbackClient := pubsub.NewInProcessPubSubClient(svc.GetHTTPHandler().GetPubSubBroker())

	// Resolve the MCP gateway up-front so the pubsub command service can
	// reach it for Actuator egress dispatch on verified MCP_CALL transactions.
	mcpSvc := svc.GetHTTPHandler().GetMCPGateway()

	// Get the GatewayDBService's AuditVault for full audit vault storage
	// This ensures ActionReceipts are persisted in the receipts table
	var auditVault *storage.AuditVaultService
	if svc.GetDB() != nil && svc.GetDB().AuditVault != nil && svc.GetDB().AuditVault.IsEnabled() {
		auditVault = svc.GetDB().AuditVault
		logger.Info("Gateway AuditVault enabled for full audit vault storage")
	} else {
		logger.Warn("Gateway AuditVault not available or disabled - ActionReceipts will not be stored in audit vault")
	}

	psConfig := pubsub.CommandServiceConfig{
		Config:              cfg,
		Logger:              logger,
		Execution:           execSvc,
		FileEdit:            fileSvc,
		PubSubClient:        loopbackClient,
		ResultsService:      nil, // Results handled via direct loopback publish if needed
		LocalStore:          nil, // Not used in gateway mode
		AuditVault:          auditVault,
		Ledger:              nil, // P1: Ledger in gateway mode
		HistoryHandler:      nil, // P1: History in gateway mode
		Scrubbing:           scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil),
		ReplayStore:         govDeps.ReplayStore,
		StateRootProvider:   govDeps.StateRootProvider,
		TransactionAudit:    govDeps.TransactionAudit,
		SignerStore:         govDeps.SignerStore,
		AppPolicyStore:      govDeps.AppPolicyStore,
		L3Notary:            govDeps.L3Notary,
		ActuatorSigningKey:  ActuatorPriv,
		ActuatorKeyID:       ActuatorKeyID,
		ConsensusSigningKey: ConsensusPriv,
		MCPGateway:          mcpSvc,
	}

	cmdSvc, err := pubsub.NewPubSubCommandService(psConfig)
	if err != nil {
		logger.Error("Failed to initialize in-process command service", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	// Wire the synchronous fail-closed mutation gate into the gateway HTTP
	// surface. Once set, BYO clients can POST GovernanceEnvelope envelopes to
	// /api/v1/governance/envelopes and receive a signed ActionReceipt.
	svc.SetEnvelopeProcessor(cmdSvc)

	// Wire MCP gateway -> Gateway processor. Egress dispatch back to the
	// downstream MCP server is invoked from the Actuator via cmdSvc, so the
	// gateway only needs the Gateway processor + signing identity here.
	if mcpSvc != nil {
		mcpSvc.SetDependencies(cmdSvc, govDeps.StateRootProvider, ActuatorPriv, ActuatorKeyID, cfg.Gateway.MCPDownstreamURL)
	}

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("Gateway service failed", string(constants.ConnectionStateError), err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	// Start the command service once the gateway service is ready
	go func() {
		for !svc.IsReady() {
			time.Sleep(100 * time.Millisecond)
			if ctx.Err() != nil {
				return
			}
		}
		logger.Info("Gateway service ready, starting in-process command service")
		if err := cmdSvc.Start(ctx); err != nil {
			logger.Error("In-process command service failed to start", string(constants.ConnectionStateError), err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if cmdSvc != nil {
		if cmdSvc.Actuator() != nil {
			logger.Info("Waiting for in-flight transactions to drain...")
			cmdSvc.Actuator().Wait()
		}
		if err := cmdSvc.Stop(); err != nil {
			logger.Error("Command service stop error", string(constants.ConnectionStateError), err)
		}
	}

	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("Gateway shutdown error", string(constants.ConnectionStateError), err)
	}
	logger.Info("Gateway mode stopped")
}

// handleVaultCommand processes vault management CLI commands
func handleVaultCommand(rekeyVault, verifyVault, resetVault bool, newPrivateKeyStr, oldPrivateKeyStr, logLevel, workDir string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	dataDir := filepath.Join(workDir, constants.Paths.Infra.DataDir)

	vault, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dataDir,
		Logger:  logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create vault: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}
	defer vault.Close()

	switch {
	case rekeyVault:
		handleRekeyVault(vault, []byte(oldPrivateKeyStr), []byte(newPrivateKeyStr), logger)
	case verifyVault:
		handleVerifyVault(vault, []byte(newPrivateKeyStr), logger)
	case resetVault:
		handleResetVault(vault, logger)
	}
}

// handleRekeyVault re-encrypts the vault DEK with a new private key
func handleRekeyVault(vault *vault.Vault, oldPrivateKey, newPrivateKey []byte, logger *slog.Logger) {
	if len(oldPrivateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: --old-key is required for --rekey-vault\n")
		fmt.Fprintf(os.Stderr, "Usage: g8e --rekey-vault --old-key <old-key> -k <new-key>\n")
		os.Exit(constants.ExitConfigError)
	}

	if len(newPrivateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: New private key is required (-k)\n")
		os.Exit(constants.ExitConfigError)
	}

	if !vault.IsInitialized() {
		fmt.Fprintf(os.Stderr, "Error: No vault found. Nothing to rekey.\n")
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("Re-keying vault")

	if err := vault.Rekey(oldPrivateKey, newPrivateKey); err != nil {
		logger.Error("Failed to rekey vault", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault successfully rekeyed")
	os.Exit(constants.ExitSuccess)
}

// handleVerifyVault checks vault integrity
func handleVerifyVault(vault *vault.Vault, privateKey []byte, logger *slog.Logger) {
	if len(privateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Private key is required for vault verification\n")
		os.Exit(constants.ExitConfigError)
	}

	if !vault.IsInitialized() {
		logger.Info("Vault not initialized")
		os.Exit(constants.ExitSuccess)
	}

	logger.Info("Verifying vault integrity")

	if err := vault.VerifyIntegrity(privateKey); err != nil {
		logger.Error("Vault verification failed", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault verification passed")
	os.Exit(constants.ExitSuccess)
}

// runInsecureMode starts the Operator in INSECURE MCP gateway mode.
// The Operator connects to an MCP gateway via WebSocket without any governance.
// This mode bypasses all L1/L2/L3 verification and is DANGEROUS.
// No g8e infrastructure (agent, client) is required.
func runInsecureMode(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	cfg, err := config.LoadInsecureMcp(config.InsecureMcpOptions{
		GatewayURL:  gatewayURL,
		Token:       token,
		NodeID:      nodeID,
		DisplayName: displayName,
		PathEnv:     pathEnv,
		LogLevel:    logLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "INSECURE MCP configuration error: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("g8e - INSECURE MCP Gateway Mode", "version", version, "build", buildID)

	svc, err := insecure_mcp.NewInsecureMcpNodeService(
		cfg.GatewayURL,
		cfg.Token,
		cfg.NodeID,
		cfg.DisplayName,
		cfg.PathEnv,
		logger,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create INSECURE MCP node service: %v\n", err)
		os.Exit(constants.ExitCodeFromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("INSECURE MCP node service failed", string(constants.ConnectionStateError), err)
			os.Exit(constants.ExitCodeFromError(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()
	svc.Stop()
	logger.Info("INSECURE MCP node host stopped")
}

// handleResetVault destroys the vault (requires confirmation)
func handleResetVault(vault *vault.Vault, logger *slog.Logger) {
	if !vault.IsInitialized() {
		logger.Info("No vault found, nothing to reset")
		os.Exit(constants.ExitSuccess)
	}

	fmt.Fprint(os.Stderr, "WARNING: This will PERMANENTLY DESTROY all encrypted vault data. Type 'DESTROY' to confirm: ")

	var confirmation string
	_, _ = fmt.Fscan(os.Stdin, &confirmation)

	if confirmation != "DESTROY" {
		logger.Info("Reset cancelled")
		os.Exit(constants.ExitSuccess)
	}

	if err := vault.Reset(true); err != nil {
		logger.Error("Failed to reset vault", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault has been reset, all encrypted data has been destroyed")
	os.Exit(constants.ExitSuccess)
}

// exportActuatorPublicKey writes the Actuator's public key to both PEM and JSON formats
// in the PKI directory for receipt verification by the evals harness.
func exportActuatorPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if pkiDir == "" {
		return fmt.Errorf("pkiDir cannot be empty")
	}
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return fmt.Errorf("create PKI directory: %w", err)
	}

	// Write PEM format
	pemPath := filepath.Join(pkiDir, "Actuator_pub.pem")
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKey,
	})
	if err := os.WriteFile(pemPath, pemData, 0600); err != nil {
		return fmt.Errorf("write Actuator_pub.pem: %w", err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", pemPath, "format", "PEM")
	}

	// Write JSON format
	jsonPath := filepath.Join(pkiDir, "Actuator_pub.json")
	jsonData := map[string]string{
		"key_id":     keyID,
		"public_key": hex.EncodeToString(pubKey),
		"algorithm":  "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Actuator_pub.json: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0600); err != nil {
		return fmt.Errorf("write Actuator_pub.json: %w", err)
	}
	if logger != nil {
		logger.Info("Actuator public key exported", "path", jsonPath, "format", "JSON")
	}

	return nil
}
