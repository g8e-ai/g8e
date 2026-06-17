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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// PasskeyBootstrapServer handles the localhost HTTP server for passkey registration
// during CLI bootstrap. It serves a simple HTML page that performs WebAuthn registration.
type PasskeyBootstrapServer struct {
	server       *http.Server
	gatewayURL   string
	bootstrapURL string
	userID       string
	userName     string
	cliSessionID string
	done         chan struct{}
	success      bool
	errMessage   string
}

// NewPasskeyBootstrapServer creates a new localhost server for passkey registration
func NewPasskeyBootstrapServer(gatewayURL, bootstrapURL, userID, userName, cliSessionID string) *PasskeyBootstrapServer {
	return &PasskeyBootstrapServer{
		gatewayURL:   gatewayURL,
		bootstrapURL: bootstrapURL,
		userID:       userID,
		userName:     userName,
		cliSessionID: cliSessionID,
		done:         make(chan struct{}),
	}
}

// Start starts the localhost server on a random available port
func (s *PasskeyBootstrapServer) Start() (string, error) {
	// Find an available port on 0.0.0.0 to allow remote access via port forwarding
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return "", fmt.Errorf("failed to find available port: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/register", s.handleRegister)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  1 * time.Minute,
		WriteTimeout: 1 * time.Minute,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Passkey bootstrap server error: %v", err)
		}
	}()

	return fmt.Sprintf("http://0.0.0.0:%d", port), nil
}

// Stop stops the server
func (s *PasskeyBootstrapServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

// Wait waits for the registration to complete or timeout.
// It returns (success, timedOut).
func (s *PasskeyBootstrapServer) Wait(timeout time.Duration) (bool, bool) {
	select {
	case <-s.done:
		return s.success, false
	case <-time.After(timeout):
		return false, true
	}
}

// handleIndex serves the registration page
func (s *PasskeyBootstrapServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Register Passkey - g8e</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; line-height: 1.5; }
        .container { border: 1px solid #ddd; border-radius: 8px; padding: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.05); }
        h1 { color: #333; margin-top: 0; }
        .info { background: #f5f5f5; padding: 15px; border-radius: 4px; margin: 20px 0; border: 1px solid #eee; }
        .warning { background: #fff3cd; color: #856404; padding: 15px; border-radius: 4px; margin: 20px 0; border: 1px solid #ffeeba; }
        .label { font-weight: bold; margin-bottom: 5px; }
        .value { margin-bottom: 15px; word-break: break-all; font-family: monospace; }
        .actions { margin-top: 20px; }
        button { padding: 12px 24px; border: none; border-radius: 4px; cursor: pointer; font-size: 16px; font-weight: bold; transition: background 0.2s; }
        .register { background: #4CAF50; color: white; }
        .register:hover { background: #45a049; }
        .status { margin-top: 20px; padding: 15px; border-radius: 4px; display: none; }
        .success { background: #d4edda; color: #155724; border: 1px solid #c3e6cb; }
        .error { background: #f8d7da; color: #721c24; border: 1px solid #f5c6cb; }
        .loading { background: #e2e3e5; color: #383d41; border: 1px solid #d6d8db; }
        code { background: #eee; padding: 2px 4px; border-radius: 3px; font-family: monospace; }
        pre { background: #2d2d2d; color: #ccc; padding: 10px; border-radius: 4px; overflow-x: auto; }
        .trust-link { display: block; margin-top: 10px; color: #0066cc; text-decoration: none; }
        .trust-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Register Passkey for g8e CLI</h1>
        <p>To complete your CLI enrollment, please register a passkey (WebAuthn/FIDO2).</p>
        
        <div class="warning">
            <strong>Self-Signed Certificate Required</strong>
            <p>Since this platform uses a self-signed CA, you must trust the root certificate before proceeding, or your browser will block the passkey registration.</p>
            
            <div class="label">1. Install Trust Script:</div>
            <p>Linux / macOS:</p>
            <pre>curl -fsSL ` + html.EscapeString(s.bootstrapURL) + constants.APIPaths.BootstrapCALinux + ` | sh</pre>
            <p>Windows (PowerShell):</p>
            <pre>irm ` + html.EscapeString(s.bootstrapURL) + constants.APIPaths.BootstrapCAWindows + ` | iex</pre>

            <div class="label">2. Restart Browser:</div>
            <p><strong>RESTART ALL OPEN BROWSERS</strong> after running the script for the new CA to be recognized.</p>
        </div>

        <div class="info">
            <div class="label">User ID:</div>
            <div class="value">` + html.EscapeString(s.userID) + `</div>
        </div>
        
        <div class="actions">
            <button class="register" onclick="registerPasskey()">Register Passkey</button>
        </div>
        
        <div id="status" class="status"></div>
    </div>

    <script>
        // Helper to convert base64url to Uint8Array
        function base64urlToUint8Array(base64url) {
            const padding = '='.repeat((4 - base64url.length % 4) % 4);
            const base64 = (base64url + padding).replace(/\-/g, '+').replace(/_/g, '/');
            const rawData = window.atob(base64);
            const outputArray = new Uint8Array(rawData.length);
            for (let i = 0; i < rawData.length; ++i) {
                outputArray[i] = rawData.charCodeAt(i);
            }
            return outputArray;
        }

        // Helper to convert Uint8Array to base64url
        function bufferToBase64url(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.byteLength; i++) {
                binary += String.fromCharCode(bytes[i]);
            }
            return window.btoa(binary)
                .replace(/\+/g, '-')
                .replace(/\//g, '_')
                .replace(/=/g, '');
        }

        async function registerPasskey() {
            const statusDiv = document.getElementById('status');
            const userID = "` + html.EscapeString(s.userID) + `";
            const userName = "` + html.EscapeString(s.userName) + `";
            const gatewayURL = "` + html.EscapeString(s.gatewayURL) + `";
            const cliSessionID = "` + html.EscapeString(s.cliSessionID) + `";

            // Check if WebAuthn is available
            if (!window.navigator || !window.navigator.credentials) {
                statusDiv.style.display = 'block';
                statusDiv.className = 'status error';
                statusDiv.innerHTML = '<strong>Error:</strong><br/>WebAuthn is not available in this browser.<br/><br/>WebAuthn requires HTTPS (or localhost for HTTP).<br/>Since you are accessing via port forwarding over HTTP, please:<br/>1. Access this page directly on the Linux host using localhost, or<br/>2. Set up HTTPS for the gateway.<br/><br/>Alternatively, you can register a passkey later via the web interface after setting up HTTPS.';
                return;
            }

            try {
                statusDiv.style.display = 'block';
                statusDiv.className = 'status loading';
                statusDiv.textContent = 'Requesting registration challenge...';

                // 1. Get Registration Challenge (use browser CLI bootstrap endpoint - public, no auth required)
                const challengeResp = await fetch(gatewayURL + "/api/v1/auth/passkeys/cli-browser-register/challenge", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        user_id: userID,
                        user_name: userName,
                        cli_session_id: cliSessionID
                    })
                });
                
                if (!challengeResp.ok) {
                    const err = await challengeResp.json();
                    throw new Error(err.error || "Failed to get challenge");
                }
                
                const challengeData = await challengeResp.json();
                const options = challengeData.options;

                // Prepare options for navigator.credentials.create
                options.publicKey.challenge = base64urlToUint8Array(options.publicKey.challenge);
                options.publicKey.user.id = base64urlToUint8Array(options.publicKey.user.id);
                if (options.publicKey.excludeCredentials) {
                    options.publicKey.excludeCredentials.forEach(cred => {
                        cred.id = base64urlToUint8Array(cred.id);
                    });
                }

                statusDiv.textContent = 'Please follow your browser prompts to create a passkey...';

                // 2. WebAuthn Registration
                const credential = await navigator.credentials.create({
                    publicKey: options.publicKey
                });

                statusDiv.textContent = 'Verifying registration...';

                // 3. Verify Registration (use browser CLI bootstrap endpoint - public, no auth required)
                const verifyResp = await fetch(gatewayURL + "/api/v1/auth/passkeys/cli-browser-register/verify", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        user_id: userID,
                        cli_session_id: cliSessionID,
                        attestation_response: {
                            id: credential.id,
                            rawId: bufferToBase64url(credential.rawId),
                            clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                            attestationObject: bufferToBase64url(credential.response.attestationObject),
                            transports: credential.response.getTransports ? credential.response.getTransports() : []
                        }
                    })
                });

                if (!verifyResp.ok) {
                    const err = await verifyResp.json();
                    throw new Error(err.error || "Verification failed");
                }

                const verifyData = await verifyResp.json();
                
                if (!verifyData.success) {
                    throw new Error(verifyData.error || "Registration failed");
                }

                statusDiv.className = 'status success';
                statusDiv.innerHTML = '<strong>Success!</strong><br/>Passkey registered successfully.<br/>You can close this window and return to the CLI.';
                
                // Notify server that registration is complete
                await fetch("/register?status=success", { method: "POST" });
                
            } catch (err) {
                console.error(err);
                statusDiv.className = 'status error';
                statusDiv.textContent = 'Error: ' + err.message;
                
                // Notify server that registration failed
                const errorMsg = encodeURIComponent(err.message);
                await fetch("/register?status=error&error=" + errorMsg, { method: "POST" });
            }
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// handleRegister handles the registration completion callback
func (s *PasskeyBootstrapServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	log.Printf("Passkey bootstrap: Received registration callback")

	// Security: allow requests from any origin when accessed via port forwarding
	// This is safe because the registration callback is a simple completion signal
	// and doesn't expose sensitive data
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		log.Printf("Passkey bootstrap: Invalid method %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	s.errMessage = r.URL.Query().Get("error")
	log.Printf("Passkey bootstrap: Registration status: %s, error: %s", status, s.errMessage)
	s.success = (status == "success")
	close(s.done)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// openBrowser opens the default system browser to the given URL.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default: // linux and other unix-like
		return exec.Command("xdg-open", url).Start()
	}
}

// RegisterPasskeyViaLocalhost starts a localhost server and guides the user through passkey registration
func RegisterPasskeyViaLocalhost(cfg *config.Config, userID, cliSessionID string) error {
	// Use external interface IP for gateway URL to support port forwarding scenarios
	// Use HTTPS (port 8443) for WebAuthn compatibility - WebAuthn requires HTTPS or localhost
	externalIP := config.GetExternalInterfaceIP()
	gatewayURL := fmt.Sprintf("https://%s:%d", externalIP, constants.Ports.OperatorHttps)

	// Get current username for passkey registration
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	userName := currentUser.Username

	bootstrapURL := fmt.Sprintf("http://%s:%d", externalIP, constants.Ports.OperatorHttp)
	server := NewPasskeyBootstrapServer(gatewayURL, bootstrapURL, userID, userName, cliSessionID)

	url, err := server.Start()
	if err != nil {
		return fmt.Errorf("failed to start passkey bootstrap server: %w", err)
	}
	defer server.Stop()

	// Replace 0.0.0.0 with external IP for the display URL
	portStr := strings.TrimPrefix(url, "http://0.0.0.0:")
	displayURL := fmt.Sprintf("http://%s:%s", externalIP, portStr)

	// Trust script URLs for both Unix and Windows platforms
	httpPort := constants.Ports.OperatorHttp
	linuxURL := fmt.Sprintf("http://%s:%d%s", externalIP, httpPort, constants.APIPaths.BootstrapCALinux)
	windowsURL := fmt.Sprintf("http://%s:%d%s", externalIP, httpPort, constants.APIPaths.BootstrapCAWindows)

	// Attempt to auto-open the browser (best-effort)
	if err := openBrowser(displayURL); err != nil {
		log.Printf("Could not auto-open browser: %v", err)
	}

	fmt.Printf("\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  PASSKEY REGISTRATION REQUIRED\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")
	fmt.Printf("To complete your CLI enrollment, you need to register a passkey.\n")
	fmt.Printf("This will enable secure passwordless authentication.\n")
	fmt.Printf("\n")
	fmt.Printf("IMPORTANT: Since we use self-signed certificates, you must:\n")
	fmt.Printf("1. Run the trust script to install the platform CA:\n")
	fmt.Printf("\n")
	fmt.Printf("   Linux / macOS:\n")
	fmt.Printf("   curl -fsSL %s | sh\n", linuxURL)
	fmt.Printf("\n")
	fmt.Printf("   Windows (PowerShell):\n")
	fmt.Printf("   irm %s | iex\n", windowsURL)
	fmt.Printf("\n")
	fmt.Printf("2. RESTART ALL OPEN BROWSERS for the new CA to be recognized.\n")
	fmt.Printf("\n")
	fmt.Printf("Once trusted, open this URL in your browser:\n")
	fmt.Printf("\n")
	fmt.Printf("  %s\n", displayURL)
	fmt.Printf("\n")
	fmt.Printf("The page will guide you through creating a WebAuthn/FIDO2 passkey.\n")
	fmt.Printf("You can use Face ID, Touch ID, Windows Hello, or a security key.\n")
	fmt.Printf("\n")
	fmt.Printf("Waiting for passkey registration...\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")

	// Wait for registration to complete (5 minute timeout for local registration to allow for slow user interaction)
	success, timedOut := server.Wait(5 * time.Minute)
	if timedOut {
		return fmt.Errorf("passkey registration timed out")
	}

	if !success {
		if server.errMessage != "" {
			return fmt.Errorf("passkey registration failed: %s", server.errMessage)
		}
		return fmt.Errorf("passkey registration failed")
	}

	fmt.Printf("\n✓ Passkey registered successfully!\n")
	fmt.Printf("\n")

	return nil
}

// VerifyPasskeyRegistration checks if a user has a passkey registered
func VerifyPasskeyRegistration(cfg *config.Config, userID string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/auth/passkeys?user_id=%s", cfg.OperatorPublicURL(), userID)

	// Load CLI mTLS certificate for authentication
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return false, fmt.Errorf("failed to load CLI certificate: %w", err)
	}

	// Load CA bundle for server verification
	caBundleBytes, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return false, fmt.Errorf("failed to read CA bundle: %w", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caBundleBytes)

	// Create HTTP client with TLS configuration
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cliCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to check passkey status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Success     bool `json:"success"`
		Credentials []struct {
			ID string `json:"id"`
		} `json:"credentials"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	return len(result.Credentials) > 0, nil
}

// PasskeyAttestationResponse represents the attestation response from the client
type PasskeyAttestationResponse struct {
	ID                string   `json:"id"`
	RawID             string   `json:"rawId"`
	ClientDataJSON    string   `json:"clientDataJSON"`
	AttestationObject string   `json:"attestationObject"`
	Transports        []string `json:"transports,omitempty"`
}

// RegisterPasskeyDirectly performs passkey registration directly via API calls
// This is an alternative to the localhost server for automated testing
func RegisterPasskeyDirectly(cfg *config.Config, userID string) error {
	// Get current username for passkey registration
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	userName := currentUser.Username

	// Get challenge
	challengeURL := fmt.Sprintf("%s/api/v1/auth/passkeys/cli-register/challenge", cfg.OperatorDiscoveryURL())
	challengeReq := models.PasskeyRegisterChallengeRequest{
		UserID:   userID,
		UserName: userName,
	}
	challengeBody, err := json.Marshal(challengeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal challenge request: %w", err)
	}

	resp, err := http.Post(challengeURL, "application/json", bytes.NewReader(challengeBody))
	if err != nil {
		return fmt.Errorf("failed to get challenge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("challenge request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var challengeResp struct {
		Success bool `json:"success"`
		Options struct {
			PublicKey struct {
				Challenge string `json:"challenge"`
				User      struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"user"`
			} `json:"publicKey"`
		} `json:"options"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&challengeResp); err != nil {
		return fmt.Errorf("failed to decode challenge response: %w", err)
	}

	if !challengeResp.Success {
		return fmt.Errorf("challenge request was not successful")
	}

	// Note: This is a placeholder for direct registration
	// In practice, WebAuthn requires browser interaction for security
	// This function is mainly for testing infrastructure
	return fmt.Errorf("direct passkey registration requires browser interaction; use RegisterPasskeyViaLocalhost instead")
}
