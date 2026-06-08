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
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
)

// HTTPSServer handles a simple HTTPS server for passkey authentication
type HTTPSServer struct {
	server     *http.Server
	gatewayURL string
	userID     string
	done       chan struct{}
	success    bool
}

// NewHTTPSServer creates a new HTTPS server for passkey authentication
func NewHTTPSServer(gatewayURL, userID string) *HTTPSServer {
	return &HTTPSServer{
		gatewayURL: gatewayURL,
		userID:     userID,
		done:       make(chan struct{}),
	}
}

// Start starts the HTTPS server on the default port 443 or a random available port
func (s *HTTPSServer) Start(cfg *config.Config) (string, error) {
	// Load operator certificate and key
	certFile := cfg.OperatorCertFile()
	keyFile := cfg.OperatorKeyFile()

	// Check if cert files exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return "", fmt.Errorf("operator certificate not found at %s. Run './g8e auth enroll-windows' first", certFile)
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return "", fmt.Errorf("operator key not found at %s. Run './g8e auth enroll-windows' first", keyFile)
	}

	// Load certificate
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return "", fmt.Errorf("failed to load certificate: %w", err)
	}

	// Try to bind to port 443 first, fall back to random port if not possible
	// Use 127.0.0.1 explicitly to ensure IPv4-only binding for certificate compatibility
	listener, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		// Fall back to random port
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("failed to find available port: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/auth", s.handleAuth)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	s.server = &http.Server{
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	go func() {
		if err := s.server.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS server error: %v", err)
		}
	}()

	return fmt.Sprintf("https://127.0.0.1:%d", port), nil
}

// Stop stops the server
func (s *HTTPSServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	}
}

// handleIndex serves the landing page
func (s *HTTPSServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>g8e - Passkey Authentication</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; background: #f5f5f5; }
        .container { background: white; border-radius: 8px; padding: 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; margin-bottom: 10px; }
        .subtitle { color: #666; margin-bottom: 30px; }
        .info { background: #e3f2fd; padding: 15px; border-radius: 4px; margin: 20px 0; border-left: 4px solid #2196F3; }
        .label { font-weight: bold; margin-bottom: 5px; color: #555; }
        .value { margin-bottom: 15px; word-break: break-all; color: #333; }
        .actions { margin-top: 30px; text-align: center; }
        button { padding: 12px 30px; border: none; border-radius: 6px; cursor: pointer; font-size: 16px; font-weight: 500; transition: background 0.2s; }
        .auth { background: #4CAF50; color: white; }
        .auth:hover { background: #45a049; }
        .status { margin-top: 20px; padding: 15px; border-radius: 4px; display: none; text-align: center; }
        .success { background: #d4edda; color: #155724; }
        .error { background: #f8d7da; color: #721c24; }
        .loading { background: #fff3cd; color: #856404; }
    </style>
</head>
<body>
    <div class="container">
        <h1>g8e Authentication</h1>
        <p class="subtitle">Secure passkey authentication for g8e</p>
        
        <div class="info">
            <div class="label">User ID:</div>
            <div class="value">` + html.EscapeString(s.userID) + `</div>
        </div>
        
        <div class="actions">
            <button class="auth" onclick="authenticate()">Authenticate with Passkey</button>
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

        async function authenticate() {
            const statusDiv = document.getElementById('status');
            const userID = "` + html.EscapeString(s.userID) + `";
            const gatewayURL = "` + html.EscapeString(s.gatewayURL) + `";
            
            try {
                statusDiv.style.display = 'block';
                statusDiv.className = 'status loading';
                statusDiv.textContent = 'Requesting authentication challenge...';

                // 1. Get Authentication Challenge
                const challengeResp = await fetch(gatewayURL + "/api/v1/auth/passkeys/authenticate/challenge", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ user_id: userID })
                });
                
                if (!challengeResp.ok) {
                    const err = await challengeResp.json();
                    throw new Error(err.error || "Failed to get challenge");
                }
                
                const challengeData = await challengeResp.json();
                if (!challengeData.success) {
                    throw new Error(challengeData.error || "Challenge request failed");
                }
                
                const options = challengeData.options;

                // Prepare options for navigator.credentials.get
                options.publicKey.challenge = base64urlToUint8Array(options.publicKey.challenge);
                if (options.publicKey.allowCredentials) {
                    options.publicKey.allowCredentials.forEach(cred => {
                        cred.id = base64urlToUint8Array(cred.id);
                    });
                }

                statusDiv.textContent = 'Please follow your browser prompts to authenticate with your passkey...';

                // 2. WebAuthn Authentication
                const assertion = await navigator.credentials.get({
                    publicKey: options.publicKey
                });

                statusDiv.textContent = 'Verifying authentication...';

                // 3. Verify Authentication
                const verifyResp = await fetch(gatewayURL + "/api/v1/auth/passkeys/authenticate/verify", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        user_id: userID,
                        assertion_response: {
                            id: assertion.id,
                            rawId: bufferToBase64url(assertion.rawId),
                            clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
                            authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
                            signature: bufferToBase64url(assertion.response.signature),
                            userHandle: assertion.response.userHandle ? bufferToBase64url(assertion.response.userHandle) : null
                        }
                    })
                });

                if (!verifyResp.ok) {
                    const err = await verifyResp.json();
                    throw new Error(err.error || "Verification failed");
                }

                const verifyData = await verifyResp.json();
                
                if (!verifyData.success) {
                    throw new Error(verifyData.error || "Authentication failed");
                }

                statusDiv.className = 'status success';
                statusDiv.innerHTML = '<strong>Success!</strong><br/>Authentication successful.<br/>You can close this window.';
                
                // Notify server that authentication is complete
                await fetch("/auth?status=success", { method: "POST" });
                
            } catch (err) {
                console.error(err);
                statusDiv.className = 'status error';
                statusDiv.textContent = 'Error: ' + err.message;
                
                // Notify server that authentication failed
                await fetch("/auth?status=error", { method: "POST" });
            }
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// handleAuth handles the authentication completion callback
func (s *HTTPSServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	s.success = (status == "success")
	close(s.done)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// ServeHTTPS starts an HTTPS server with passkey authentication landing page
func ServeHTTPS(cfg *config.Config, userID string) error {
	// Load credentials to get userID if not provided
	if userID == "" {
		creds, err := LoadCredentials(cfg)
		if err != nil {
			return fmt.Errorf("failed to load credentials: %w", err)
		}
		if creds == nil {
			return fmt.Errorf("no credentials found. Run './g8e auth enroll-windows' first")
		}
		userID = creds.UserID
	}

	gatewayURL := cfg.OperatorPublicURL()

	server := NewHTTPSServer(gatewayURL, userID)

	url, err := server.Start(cfg)
	if err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", err)
	}
	defer server.Stop()

	// Attempt to auto-open the browser (best-effort)
	if err := openBrowser(url); err != nil {
		log.Printf("Could not auto-open browser: %v", err)
	}

	fmt.Printf("\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  g8e HTTPS AUTHENTICATION SERVER\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")
	fmt.Printf("A browser window should have opened automatically.\n")
	fmt.Printf("If not, please open the following URL manually:\n")
	fmt.Printf("\n")
	fmt.Printf("  %s\n", url)
	fmt.Printf("\n")
	fmt.Printf("The page will guide you through WebAuthn/FIDO2 passkey authentication.\n")
	fmt.Printf("You can use Face ID, Touch ID, Windows Hello, or a security key.\n")
	fmt.Printf("\n")
	fmt.Printf("Press Ctrl+C to stop the server.\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")

	// Wait for authentication to complete or interrupt
	select {
	case <-server.done:
		if server.success {
			fmt.Printf("\n✓ Authentication successful!\n")
			fmt.Printf("\n")
		} else {
			fmt.Printf("\n✗ Authentication failed\n")
			fmt.Printf("\n")
		}
	case <-time.After(30 * time.Minute):
		return fmt.Errorf("authentication timed out")
	}

	return nil
}
