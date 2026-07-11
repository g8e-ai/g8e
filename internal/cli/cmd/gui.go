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

package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	g8econfig "github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
)

// GUIEnrollmentFile is the name of the file storing enrolled GUI origins.
const GUIEnrollmentFile = "gui_enrollments.json"

// GUIEnrollment holds the persisted list of enrolled frontend origins.
type GUIEnrollment struct {
	Origins []string `json:"origins"`
}

// guiCmd is the parent command for GUI/frontend enrollment operations.
func guiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Enroll external frontend applications with the g8e Gateway",
		Long: `Manage GUI/frontend application enrollment with the g8e Gateway.

Enrollment registers a frontend application's origin with the gateway's CORS
allowed origins and passkey relying party (RP) origins, then restarts the
gateway so the new configuration takes effect. After enrollment, the frontend
can authenticate via WebAuthn passkeys and receive SSE events from the gateway.

This enables any external application (Lovable, custom React app, etc.) to
connect securely to the g8e platform.`,
	}

	cmd.AddCommand(
		guiEnrollCmd(),
		guiShowCmd(),
		guiVerifyCmd(),
	)

	return cmd
}

func guiEnrollCmd() *cobra.Command {
	return guiEnrollCmdWithDeps(loadConfig, restartGateway)
}

func guiEnrollCmdWithDeps(configLoader func(string) (*config.Config, error), restarter func(*config.Config, []string, []string) error) *cobra.Command {
	var origin string
	var passkeyRpID string
	var passkeyRpName string
	var publicBaseURL string
	var noRestart bool

	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll a frontend application origin with the gateway",
		Long: `Enroll a frontend application by registering its origin with the g8e Gateway.

This command:
1. Validates the origin URL
2. Persists the origin to the enrollment file (.g8e/gui_enrollments.json)
3. Restarts the gateway with the origin added to CORS allowed origins and passkey RP origins
4. Outputs a configuration snippet for the frontend developer

The gateway must be running before enrollment. Use --no-restart to skip the
gateway restart (you will need to restart manually for the new origin to take
effect).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if origin == "" {
				return fmt.Errorf("%w: --origin is required", constants.ErrMissingRequiredField)
			}

			if err := validateOrigin(origin); err != nil {
				return err
			}

			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gui: load config: %w", err)
			}

			enrollmentPath := guiEnrollmentFilePath(cfg)

			enrollment, err := loadGUIEnrollment(enrollmentPath)
			if err != nil {
				return fmt.Errorf("gui: load enrollment: %w", err)
			}

			for _, existing := range enrollment.Origins {
				if existing == origin {
					cmd.Printf("Origin %s is already enrolled.\n", origin)
					printEnrollConfig(cmd, origin, passkeyRpID, passkeyRpName, publicBaseURL)
					return nil
				}
			}

			enrollment.Origins = append(enrollment.Origins, origin)
			if err := saveGUIEnrollment(enrollmentPath, enrollment); err != nil {
				return fmt.Errorf("gui: save enrollment: %w", err)
			}

			cmd.Printf("Enrolled origin: %s\n", origin)
			cmd.Printf("Saved to: %s\n", enrollmentPath)

			if !noRestart {
				cmd.Println("\nRestarting gateway to apply CORS and passkey RP origins...")
				if err := restarter(cfg, enrollment.Origins, []string{origin}); err != nil {
					cmd.Printf("Warning: gateway restart failed: %v\n", err)
					cmd.Println("The origin has been saved. Restart the gateway manually to apply changes.")
				} else {
					cmd.Println("Gateway restarted successfully.")
				}
			} else {
				cmd.Println("\n--no-restart specified. Restart the gateway manually to apply changes:")
				cmd.Printf("  g8e gw restart\n")
			}

			cmd.Println()
			printEnrollConfig(cmd, origin, passkeyRpID, passkeyRpName, publicBaseURL)

			return nil
		},
	}

	cmd.Flags().StringVar(&origin, "origin", "", "Frontend application origin URL (e.g., https://my-app.lovable.app)")
	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "Passkey RP ID (defaults to gateway's hostname from origin)")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "Passkey RP display name (default: g8e)")
	cmd.Flags().StringVar(&publicBaseURL, "public-base-url", "", "Public base URL for the gateway (e.g., https://console.g8e.ai)")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, "Skip gateway restart (save enrollment only)")

	return cmd
}

func guiShowCmd() *cobra.Command {
	return guiShowCmdWithDeps(loadConfig)
}

func guiShowCmdWithDeps(configLoader func(string) (*config.Config, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display enrolled frontend origins and configuration",
		Long: `Display all enrolled frontend application origins and generate a
configuration snippet for integrating with the g8e Gateway.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gui: load config: %w", err)
			}

			enrollmentPath := guiEnrollmentFilePath(cfg)
			enrollment, err := loadGUIEnrollment(enrollmentPath)
			if err != nil {
				return fmt.Errorf("gui: load enrollment: %w", err)
			}

			if len(enrollment.Origins) == 0 {
				cmd.Println("No frontend applications enrolled.")
				cmd.Println("\nTo enroll a frontend application:")
				cmd.Printf("  g8e gui enroll --origin https://your-app.example.com\n")
				return nil
			}

			cmd.Println("Enrolled Frontend Applications")
			cmd.Println(strings.Repeat("=", 50))
			cmd.Println()
			for i, o := range enrollment.Origins {
				cmd.Printf("  %d. %s\n", i+1, o)
			}
			cmd.Println()
			cmd.Println("Configuration snippet for each origin:")
			cmd.Println()
			for _, o := range enrollment.Origins {
				printEnrollConfig(cmd, o, "", "", "")
				cmd.Println()
			}

			return nil
		},
	}

	return cmd
}

func guiVerifyCmd() *cobra.Command {
	return guiVerifyCmdWithDeps(loadConfig)
}

func guiVerifyCmdWithDeps(configLoader func(string) (*config.Config, error)) *cobra.Command {
	var origin string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify gateway connectivity and CORS configuration for a frontend origin",
		Long: `Verify that the gateway is running and configured to accept requests
from the specified frontend origin. Checks the health endpoint and
CORS preflight.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if origin == "" {
				return fmt.Errorf("%w: --origin is required", constants.ErrMissingRequiredField)
			}

			if err := validateOrigin(origin); err != nil {
				return err
			}

			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("gui: load config: %w", err)
			}

			enrollmentPath := guiEnrollmentFilePath(cfg)
			enrollment, err := loadGUIEnrollment(enrollmentPath)
			if err != nil {
				return fmt.Errorf("gui: load enrollment: %w", err)
			}

			enrolled := false
			for _, o := range enrollment.Origins {
				if o == origin {
					enrolled = true
					break
				}
			}

			if !enrolled {
				cmd.Printf("Warning: origin %s is not in the enrollment list.\n", origin)
				cmd.Println("Run 'g8e gui enroll --origin <origin>' first.")
			}

			httpsURL := netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps)
			httpURL := netutil.LocalhostHTTPURL(constants.Ports.OperatorHttp)

			cmd.Println("Verification Results")
			cmd.Println(strings.Repeat("=", 50))
			cmd.Println()
			cmd.Printf("Origin:     %s\n", origin)
			cmd.Printf("Enrolled:   %v\n", enrolled)
			cmd.Printf("Gateway:    %s\n", httpsURL)
			cmd.Println()

			cmd.Printf("1. Health endpoint (HTTP):  %s/api/v1/health\n", httpURL)
			cmd.Println("   Run in browser DevTools or curl:")
			cmd.Printf("   curl %s/api/v1/health\n", httpURL)
			cmd.Println()

			cmd.Println("2. CORS preflight (HTTPS):")
			cmd.Println("   Run in browser DevTools:")
			cmd.Printf("   fetch('%s/api/v1/health', { credentials: 'include' })\n", httpsURL)
			cmd.Println()

			cmd.Println("3. SSE stream (HTTPS):")
			cmd.Printf("   GET %s/api/v1/sse/stream?web_session_id=<session-id>\n", httpsURL)
			cmd.Println("   (requires authenticated session cookie)")
			cmd.Println()

			cmd.Println("4. WebAuthn passkey endpoints (HTTPS):")
			cmd.Printf("   POST %s/api/v1/auth/passkeys/console/register/challenge\n", httpsURL)
			cmd.Printf("   POST %s/api/v1/auth/passkeys/console/register/verify\n", httpsURL)
			cmd.Printf("   POST %s/api/v1/auth/passkeys/console/authenticate/challenge\n", httpsURL)
			cmd.Printf("   POST %s/api/v1/auth/passkeys/console/authenticate/verify\n", httpsURL)
			cmd.Println()

			cmd.Println("Verification checklist:")
			cmd.Println("  [ ] CORS headers present (Access-Control-Allow-Origin, Allow-Credentials)")
			cmd.Println("  [ ] Passkey registration works (browser WebAuthn dialog appears)")
			cmd.Println("  [ ] Passkey authentication works (login succeeds)")
			cmd.Println("  [ ] Session cookie set (g8e_web_session_cookie, SameSite=None, Secure)")
			cmd.Println("  [ ] SSE stream connects and receives events")
			cmd.Println("  [ ] Authenticated API calls succeed (GET /api/v1/users/me)")

			return nil
		},
	}

	cmd.Flags().StringVar(&origin, "origin", "", "Frontend application origin URL to verify")

	return cmd
}

// printEnrollConfig outputs a configuration snippet for the frontend developer.
func printEnrollConfig(cmd *cobra.Command, origin, rpID, rpName, publicBaseURL string) {
	if rpID == "" {
		if u, err := url.Parse(origin); err == nil && u.Host != "" {
			rpID = u.Host
		} else {
			rpID = "localhost"
		}
	}
	if rpName == "" {
		rpName = "g8e"
	}
	if publicBaseURL == "" {
		publicBaseURL = netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps)
	}

	cmd.Println("Frontend Configuration Snippet")
	cmd.Println(strings.Repeat("=", 50))
	cmd.Println()
	cmd.Println("TypeScript / JavaScript:")
	cmd.Println()
	cmd.Println("```typescript")
	cmd.Printf("const API_BASE_URL = '%s';\n", publicBaseURL)
	cmd.Printf("const PASSKEY_RP_ID = '%s';\n", rpID)
	cmd.Printf("const PASSKEY_RP_NAME = '%s';\n", rpName)
	cmd.Println()
	cmd.Println("// Every fetch call MUST include credentials: 'include'")
	cmd.Println("async function apiFetch(path: string, options: RequestInit = {}) {")
	cmd.Println("  return fetch(`${API_BASE_URL}${path}`, {")
	cmd.Println("    ...options,")
	cmd.Println("    credentials: 'include',")
	cmd.Println("    headers: { 'Content-Type': 'application/json', ...options.headers },")
	cmd.Println("  });")
	cmd.Println("}")
	cmd.Println()
	cmd.Println("// SSE stream (requires authenticated session)")
	cmd.Println("function connectSSE(webSessionId: string) {")
	cmd.Printf("  const es = new EventSource(`${API_BASE_URL}/api/v1/sse/stream?web_session_id=${webSessionId}`, { withCredentials: true });\n")
	cmd.Println("  es.onmessage = (ev) => console.log('SSE event:', JSON.parse(ev.data));")
	cmd.Println("  es.onerror = () => { es.close(); setTimeout(() => connectSSE(webSessionId), 3000); };")
	cmd.Println("  return es;")
	cmd.Println("}")
	cmd.Println()
	cmd.Println("// Key endpoints:")
	cmd.Println("//   GET  /api/v1/health                          - Health check")
	cmd.Println("//   GET  /api/v1/auth/bootstrap/status            - Check if passkey registered")
	cmd.Println("//   POST /api/v1/auth/passkeys/console/register/challenge  - Passkey registration")
	cmd.Println("//   POST /api/v1/auth/passkeys/console/register/verify    - Verify registration")
	cmd.Println("//   POST /api/v1/auth/passkeys/console/authenticate/challenge - Passkey auth")
	cmd.Println("//   POST /api/v1/auth/passkeys/console/authenticate/verify   - Verify auth")
	cmd.Println("//   GET  /api/v1/users/me                         - Get current user")
	cmd.Println("//   GET  /api/v1/sse/stream?web_session_id=<id>   - SSE live events")
	cmd.Println("//   GET  /api/v1/approvals                        - List pending approvals")
	cmd.Println("```")
	cmd.Println()
	cmd.Println("Gateway CLI flags for this origin:")
	cmd.Printf("  --cors-origin %s --passkey-rp-origin %s\n", origin, origin)
}

// validateOrigin checks that the origin is a valid HTTPS or HTTP URL.
func validateOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("%w: invalid origin URL: %w", constants.ErrValidationFailed, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%w: origin must use http or https scheme, got %s", constants.ErrValidationFailed, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: origin must have a host", constants.ErrValidationFailed)
	}
	return nil
}

// guiEnrollmentFilePath returns the path to the GUI enrollment file.
func guiEnrollmentFilePath(cfg *config.Config) string {
	return filepath.Join(cfg.RuntimeDir, GUIEnrollmentFile)
}

// loadGUIEnrollment reads the enrollment file, returning an empty enrollment if not found.
func loadGUIEnrollment(path string) (*GUIEnrollment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GUIEnrollment{Origins: []string{}}, nil
		}
		return nil, err
	}
	var enrollment GUIEnrollment
	if err := json.Unmarshal(data, &enrollment); err != nil {
		return nil, fmt.Errorf("parse enrollment file: %w", err)
	}
	if enrollment.Origins == nil {
		enrollment.Origins = []string{}
	}
	return &enrollment, nil
}

// saveGUIEnrollment writes the enrollment file.
func saveGUIEnrollment(path string, enrollment *GUIEnrollment) error {
	data, err := json.MarshalIndent(enrollment, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("create enrollment dir: %w", err)
	}
	return os.WriteFile(path, data, constants.PermFilePrivate)
}

// restartGateway stops and restarts the gateway with the given CORS origins
// and passkey RP origins added to the existing configuration.
func restartGateway(cfg *config.Config, corsOrigins, passkeyRpOrigins []string) error {
	pm, err := platform.NewProcessManager(cfg.ProjectRoot)
	if err != nil {
		return fmt.Errorf("create process manager: %w", err)
	}

	running, _, err := pm.OperatorStatus()
	if err != nil {
		return fmt.Errorf("check gateway status: %w", err)
	}

	if running {
		if err := pm.StopOperator(); err != nil {
			return fmt.Errorf("stop gateway: %w", err)
		}
	}

	posture, err := pm.ReadPosture()
	if err != nil {
		return fmt.Errorf("read posture: %w", err)
	}
	if posture == "" {
		posture = "doctrine"
	}

	gatewayCfg := serve.GatewayConfig{
		Posture:          g8econfig.GatewayPosture(posture),
		LogLevel:         "info",
		AllowedOrigins:   corsOrigins,
		PasskeyRpOrigins: passkeyRpOrigins,
	}

	if err := pm.StartOperator(platform.OperatorStartOptions{
		GatewayConfig: gatewayCfg,
	}); err != nil {
		return fmt.Errorf("start gateway: %w", err)
	}

	return nil
}
