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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
)

const cloudflaredBin = "cloudflared"

// tunnelCmd is the parent command for Cloudflare Tunnel management.
func tunnelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Manage Cloudflare Tunnel for public gateway access",
		Long: `Manage a Cloudflare Tunnel that securely exposes the local g8e Gateway
to the internet without opening firewall ports.

The tunnel forwards traffic from a public hostname (e.g. console.g8e.ai)
through Cloudflare's edge to the gateway's HTTPS listener on localhost:8443.

Prerequisites:
  - cloudflared installed (https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/)
  - A Cloudflare account with a registered domain

Subcommands:
  create   Create a named tunnel, route DNS, and generate config.yml
  run      Start the tunnel (foreground, blocks until interrupted)
  status   Check tunnel connectivity and gateway health through the tunnel
`,
	}

	cmd.AddCommand(
		tunnelCreateCmd(),
		tunnelRunCmd(),
		tunnelStatusCmd(),
	)

	return cmd
}

// checkCloudflared verifies that cloudflared is installed and on PATH.
func checkCloudflared() error {
	if _, err := exec.LookPath(cloudflaredBin); err != nil {
		return fmt.Errorf("%w: cloudflared is not installed or not on PATH. "+
			"Install it from https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/",
			constants.ErrServiceUnavailable)
	}
	return nil
}

// tunnelCreateCmd creates a Cloudflare tunnel, routes DNS, and generates config.yml.
func tunnelCreateCmd() *cobra.Command {
	var tunnelName string
	var hostname string
	var configDir string
	var httpsPort int
	var caBundle string
	var originServerName string
	var skipDNS bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Cloudflare tunnel, route DNS, and generate config.yml",
		Long: `Create a named Cloudflare tunnel and generate the cloudflared config.yml
file for the g8e Gateway.

This command:
  1. Authenticates with Cloudflare (if not already authenticated)
  2. Creates a named tunnel
  3. Routes DNS to the tunnel (creates a CNAME record)
  4. Generates ~/.cloudflared/config.yml with the correct ingress rules

After creating the tunnel, run 'g8e gw tunnel run' to start it.

Prerequisites:
  - cloudflared installed
  - A Cloudflare account with a registered domain
  - The gateway running on the configured HTTPS port`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCloudflared(); err != nil {
				return err
			}

			if tunnelName == "" {
				return fmt.Errorf("%w: --name is required", constants.ErrMissingRequiredField)
			}
			if hostname == "" {
				return fmt.Errorf("%w: --hostname is required", constants.ErrMissingRequiredField)
			}

			if httpsPort == 0 {
				httpsPort = constants.Ports.OperatorHttps
			}

			if configDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("%w: cannot determine home directory: %w", constants.ErrInternal, err)
				}
				configDir = filepath.Join(homeDir, ".cloudflared")
			}

			cmd.Println("[g8e] Cloudflare Tunnel Setup")
			cmd.Println()

			// Step 1: Check if already authenticated
			cmd.Println("Step 1: Checking Cloudflare authentication...")
			if !cloudflaredAuthenticated(configDir) {
				cmd.Println("  Running 'cloudflared tunnel login'...")
				cmd.Println("  A browser will open to authenticate with Cloudflare.")
				if err := runCloudflared(cmd, "tunnel", "login"); err != nil {
					return fmt.Errorf("cloudflared login failed: %w", err)
				}
			} else {
				cmd.Println("  Already authenticated.")
			}
			cmd.Println()

			// Step 2: Create the tunnel
			cmd.Printf("Step 2: Creating tunnel '%s'...\n", tunnelName)
			output, err := exec.Command(cloudflaredBin, "tunnel", "create", tunnelName).CombinedOutput()
			if err != nil {
				// Check if tunnel already exists
				if bytes.Contains(output, []byte("already exists")) {
					cmd.Printf("  Tunnel '%s' already exists, continuing.\n", tunnelName)
				} else {
					return fmt.Errorf("cloudflared tunnel create failed: %w\nOutput: %s", err, string(output))
				}
			} else {
				cmd.Printf("  Tunnel created: %s\n", parseTunnelID(output))
			}
			cmd.Println()

			// Step 3: Route DNS
			if !skipDNS {
				cmd.Printf("Step 3: Routing DNS %s -> tunnel '%s'...\n", hostname, tunnelName)
				dnsOutput, dnsErr := exec.Command(cloudflaredBin, "tunnel", "route", "dns", tunnelName, hostname).CombinedOutput()
				if dnsErr != nil {
					if bytes.Contains(dnsOutput, []byte("already exists")) || bytes.Contains(dnsOutput, []byte("record already exists")) {
						cmd.Printf("  DNS record for %s already exists, continuing.\n", hostname)
					} else {
						return fmt.Errorf("cloudflared tunnel route dns failed: %w\nOutput: %s", dnsErr, string(dnsOutput))
					}
				} else {
					cmd.Printf("  DNS CNAME %s -> %s.cfargotunnel.com created.\n", hostname, tunnelName)
				}
			} else {
				cmd.Println("Step 3: Skipping DNS routing (--skip-dns)")
			}
			cmd.Println()

			// Step 4: Generate config.yml
			cmd.Printf("Step 4: Generating config.yml at %s...\n", configDir)
			tunnelID, err := getTunnelID(tunnelName)
			if err != nil {
				cmd.Printf("  Warning: could not determine tunnel ID: %v\n", err)
				cmd.Println("  Using tunnel name as placeholder. Edit config.yml manually.")
				tunnelID = "<tunnel-id>"
			}

			credentialsFile := filepath.Join(configDir, tunnelID+".json")

			configContent := generateTunnelConfig(tunnelID, credentialsFile, hostname, httpsPort, caBundle, originServerName)
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				return fmt.Errorf("%w: create config directory: %w", constants.ErrInternal, err)
			}
			configPath := filepath.Join(configDir, "config.yml")
			if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
				return fmt.Errorf("%w: write config.yml: %w", constants.ErrInternal, err)
			}
			cmd.Printf("  Config written to: %s\n", configPath)
			cmd.Println()

			// Summary
			cmd.Println("Setup complete!")
			cmd.Println()
			cmd.Println("Next steps:")
			cmd.Printf("  1. Start the gateway (if not already running):\n")
			cmd.Printf("     g8e gw start --public-base-url https://%s --passkey-rp-id %s --passkey-rp-origin https://%s --cors-origin https://%s\n",
				hostname, hostname, hostname, hostname)
			cmd.Printf("  2. Start the tunnel:\n")
			cmd.Printf("     g8e gw tunnel run --name %s\n", tunnelName)
			cmd.Printf("  3. Verify connectivity:\n")
			cmd.Printf("     g8e gw tunnel status --hostname %s\n", hostname)
			cmd.Println()
			cmd.Printf("  4. Enroll a frontend:\n")
			cmd.Printf("     g8e gui enroll --origin https://your-app.lovable.app --public-base-url https://%s\n", hostname)

			return nil
		},
	}

	cmd.Flags().StringVar(&tunnelName, "name", "g8e", "Tunnel name")
	cmd.Flags().StringVar(&hostname, "hostname", "", "Public hostname for the tunnel (e.g. console.g8e.ai)")
	cmd.Flags().StringVar(&configDir, "config-dir", "", "cloudflared config directory (default: ~/.cloudflared)")
	cmd.Flags().IntVar(&httpsPort, "https-port", 0, "Gateway HTTPS port (default: 8443)")
	cmd.Flags().StringVar(&caBundle, "ca-bundle", "", "Path to CA bundle for origin TLS verification (default: noTLSVerify)")
	cmd.Flags().StringVar(&originServerName, "origin-server-name", "", "Origin server name for TLS SNI (default: g8e.local)")
	cmd.Flags().BoolVar(&skipDNS, "skip-dns", false, "Skip DNS routing (use if CNAME already exists)")

	return cmd
}

// tunnelRunCmd starts the cloudflared tunnel in the foreground.
func tunnelRunCmd() *cobra.Command {
	var tunnelName string
	var configDir string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the Cloudflare tunnel (foreground)",
		Long: `Start the Cloudflare tunnel in the foreground. This command blocks
until interrupted with Ctrl+C or a termination signal.

The tunnel must have been created first with 'g8e gw tunnel create'.

The gateway should be running on localhost:8443 (or the port specified in
config.yml) before starting the tunnel.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCloudflared(); err != nil {
				return err
			}

			if tunnelName == "" {
				return fmt.Errorf("%w: --name is required", constants.ErrMissingRequiredField)
			}

			args2 := []string{"tunnel", "run"}
			if configDir != "" {
				args2 = append(args2, "--config", filepath.Join(configDir, "config.yml"))
			}
			args2 = append(args2, tunnelName)

			cmd.Printf("[g8e] Starting Cloudflare tunnel '%s'...\n", tunnelName)
			cmd.Println("[g8e] Press Ctrl+C to stop.")
			cmd.Println()

			c := exec.Command(cloudflaredBin, args2...)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			c.Stdin = os.Stdin

			if err := c.Start(); err != nil {
				return fmt.Errorf("failed to start cloudflared: %w", err)
			}

			// Handle signals to forward to cloudflared
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				sig := <-sigCh
				if c.Process != nil {
					_ = c.Process.Signal(sig)
				}
			}()

			if err := c.Wait(); err != nil {
				// Exit code 0 or signal termination is not an error
				if c.ProcessState != nil && c.ProcessState.Exited() {
					return nil
				}
				return fmt.Errorf("cloudflared exited with error: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tunnelName, "name", "g8e", "Tunnel name")
	cmd.Flags().StringVar(&configDir, "config-dir", "", "cloudflared config directory (default: ~/.cloudflared)")

	return cmd
}

// tunnelStatusCmd checks tunnel connectivity and gateway health.
func tunnelStatusCmd() *cobra.Command {
	var hostname string
	var tunnelName string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check tunnel connectivity and gateway health",
		Long: `Check the Cloudflare tunnel status and verify that the g8e Gateway
is reachable through the tunnel.

This command:
  1. Checks cloudflared tunnel info
  2. Hits the gateway health endpoint through the public hostname
  3. Reports connection status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCloudflared(); err != nil {
				return err
			}

			if tunnelName == "" {
				return fmt.Errorf("%w: --name is required", constants.ErrMissingRequiredField)
			}

			cmd.Println("[g8e] Cloudflare Tunnel Status")
			cmd.Println()

			// Check tunnel info
			cmd.Printf("Tunnel: %s\n", tunnelName)
			output, err := exec.Command(cloudflaredBin, "tunnel", "info", tunnelName).CombinedOutput()
			if err != nil {
				cmd.Printf("  Status: ERROR — %s\n", strings.TrimSpace(string(output)))
			} else {
				cmd.Println("  Status: ACTIVE")
				// Print relevant lines from tunnel info
				for _, line := range strings.Split(string(output), "\n") {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "Created") {
						cmd.Printf("  %s\n", line)
					}
				}
			}
			cmd.Println()

			// Check gateway health through tunnel
			if hostname != "" {
				cmd.Printf("Gateway health check (through %s):\n", hostname)
				healthURL := fmt.Sprintf("https://%s/api/v1/health", hostname)
				healthOutput, healthErr := exec.Command("curl", "-s", "-m", "10", healthURL).CombinedOutput()
				if healthErr != nil {
					cmd.Printf("  Result: UNREACHABLE — %s\n", strings.TrimSpace(string(healthOutput)))
					cmd.Println()
					cmd.Println("  Possible causes:")
					cmd.Println("    - Tunnel is not running (start with 'g8e gw tunnel run')")
					cmd.Println("    - Gateway is not running (start with 'g8e gw start')")
					cmd.Println("    - DNS has not propagated yet")
				} else {
					outputStr := strings.TrimSpace(string(healthOutput))
					if strings.Contains(outputStr, "\"status\":\"ok\"") {
						cmd.Printf("  Result: HEALTHY\n")
						cmd.Printf("  Response: %s\n", outputStr)
					} else {
						cmd.Printf("  Result: UNEXPECTED RESPONSE\n")
						cmd.Printf("  Response: %s\n", outputStr)
					}
				}
			} else {
				cmd.Println("Gateway health check: skipped (provide --hostname to check)")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "", "Public hostname to check (e.g. console.g8e.ai)")
	cmd.Flags().StringVar(&tunnelName, "name", "g8e", "Tunnel name")

	return cmd
}

// cloudflaredAuthenticated checks whether cloudflared has been authenticated
// by looking for the cert.pem file in the config directory.
func cloudflaredAuthenticated(configDir string) bool {
	certPath := filepath.Join(configDir, "cert.pem")
	_, err := os.Stat(certPath)
	return err == nil
}

// runCloudflared runs a cloudflared command, forwarding stdout/stderr to the
// cobra command's output.
func runCloudflared(cmd *cobra.Command, args ...string) error {
	c := exec.Command(cloudflaredBin, args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	return c.Run()
}

// parseTunnelID extracts the tunnel UUID from 'cloudflared tunnel create' output.
func parseTunnelID(output []byte) string {
	// Output looks like: "Created tunnel g8e with id 12345678-1234-1234-1234-123456789abc"
	re := regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// getTunnelID retrieves the tunnel ID by listing tunnels.
func getTunnelID(tunnelName string) (string, error) {
	output, err := exec.Command(cloudflaredBin, "tunnel", "list").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cloudflared tunnel list: %w", err)
	}
	// Output is a table; find the row with our tunnel name
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, tunnelName) {
			id := parseTunnelID([]byte(line))
			if id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("tunnel '%s' not found in list", tunnelName)
}

// generateTunnelConfig produces the cloudflared config.yml content for the
// g8e Gateway.
func generateTunnelConfig(tunnelID, credentialsFile, hostname string, httpsPort int, caBundle, originServerName string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("tunnel: %s\n", tunnelID))
	sb.WriteString(fmt.Sprintf("credentials-file: %s\n", credentialsFile))
	sb.WriteString("\n")
	sb.WriteString("ingress:\n")
	sb.WriteString(fmt.Sprintf("  - hostname: %s\n", hostname))
	sb.WriteString(fmt.Sprintf("    service: https://localhost:%d\n", httpsPort))
	sb.WriteString("    originRequest:\n")

	if caBundle != "" {
		sb.WriteString(fmt.Sprintf("      originCaPool: %s\n", caBundle))
		if originServerName != "" {
			sb.WriteString(fmt.Sprintf("      originServerName: %s\n", originServerName))
		}
	} else {
		sb.WriteString("      noTLSVerify: true\n")
	}

	sb.WriteString("      http2Origin: true\n")
	sb.WriteString("  - service: http_status:404\n")

	return sb.String()
}

// runCloudflaredContext runs cloudflared with context cancellation support.
// This is used by tunnel run to allow graceful shutdown.
func runCloudflaredContext(ctx context.Context, cmd *cobra.Command, args ...string) error {
	c := exec.CommandContext(ctx, cloudflaredBin, args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	return c.Run()
}
