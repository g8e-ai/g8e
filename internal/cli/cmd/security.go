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
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func securityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "security",
		Short: "Security validation checks",
		Long:  `Run security validation and PKI verification checks.`,
	}

	cmd.AddCommand(
		securityValidateCmd(),
		securityPKICmd(),
	)

	return cmd
}

func securityValidateCmd() *cobra.Command {
	var pkiDir string
	var secretsDir string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run security validation checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pkiDir == "" {
				pkiDir = constants.Paths.Infra.PkiDir
			}
			if secretsDir == "" {
				secretsDir = constants.Paths.Infra.SecretsDir
			}

			cmd.Println("Running platform security validation...")
			failed := false

			// Check PKI directory structure
			cmd.Println("\n=== PKI Directory Structure ===")
			pkiFiles := []string{
				filepath.Join(pkiDir, "root", "root_ca.crt"),
				filepath.Join(pkiDir, "root", "root_ca.key"),
				filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"),
				filepath.Join(pkiDir, "warden_pub.pem"),
			}
			for _, file := range pkiFiles {
				if _, err := os.Stat(file); os.IsNotExist(err) {
					cmd.Printf("  [FAIL] %s missing\n", file)
					failed = true
				} else {
					cmd.Printf("  [OK]   %s exists\n", file)
				}
			}

			// Check secrets directory
			cmd.Println("\n=== Secrets Directory ===")
			secretFiles := []string{
				filepath.Join(secretsDir, "session_encryption_key"),
				filepath.Join(secretsDir, "bootstrap_digest.json"),
			}
			for _, file := range secretFiles {
				if _, err := os.Stat(file); os.IsNotExist(err) {
					cmd.Printf("  [FAIL] %s missing\n", file)
					failed = true
				} else {
					cmd.Printf("  [OK]   %s exists\n", file)
				}
			}

			// Validate root CA certificate
			cmd.Println("\n=== Certificate Validation ===")
			rootCAPath := filepath.Join(pkiDir, "root", "root_ca.crt")
			if certData, err := os.ReadFile(rootCAPath); err == nil {
				certPool := x509.NewCertPool()
				if !certPool.AppendCertsFromPEM(certData) {
					cmd.Printf("  [FAIL] root_ca.crt is not a valid PEM certificate\n")
					failed = true
				} else {
					cmd.Printf("  [OK]   root_ca.crt is valid PEM\n")
				}
			} else if !os.IsNotExist(err) {
				cmd.Printf("  [FAIL] failed to read root_ca.crt: %v\n", err)
				failed = true
			}

			// Check port availability (standard ports)
			cmd.Println("\n=== Port Availability ===")
			ports := []int{
				constants.Ports.OperatorHttps,
				constants.Ports.OperatorBootstrapHttps,
				constants.Ports.OperatorPublicHttps,
			}
			for _, port := range ports {
				addr := fmt.Sprintf("127.0.0.1:%d", port)
				listener, err := net.Listen(string(constants.NetworkProtocolTCP), addr)
				if err != nil {
					cmd.Printf("  [WARN] Port %d is in use\n", port)
				} else {
					listener.Close()
					cmd.Printf("  [OK]   Port %d is available\n", port)
				}
			}

			// Check TLS configuration
			cmd.Println("\n=== TLS Configuration ===")
			trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
			if trustData, err := os.ReadFile(trustBundlePath); err == nil {
				certPool := x509.NewCertPool()
				if certPool.AppendCertsFromPEM(trustData) {
					cmd.Printf("  [OK]   Trust bundle is valid PEM\n")
				} else {
					cmd.Printf("  [FAIL] Trust bundle is not valid PEM\n")
					failed = true
				}
			}

			cmd.Println("\n=== Summary ===")
			if failed {
				cmd.Println("[FAIL] Security validation failed")
				return fmt.Errorf("security validation failed")
			}
			cmd.Println("[OK]   Security validation passed")
			return nil
		},
	}

	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "PKI directory (default: "+constants.Paths.Infra.PkiDir+")")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Secrets directory (default: "+constants.Paths.Infra.SecretsDir+")")

	return cmd
}

func securityPKICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pki",
		Short: "PKI management",
		Long:  `Manage PKI certificates and enrollment.`,
	}

	cmd.AddCommand(
		securityPKIEnrollCmd(),
	)

	return cmd
}

func securityPKIEnrollCmd() *cobra.Command {
	var endpoint string
	var outputDir string

	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll a device with the Gateway via CSR",
		Long:  `Generate a CSR and enroll with the Gateway to obtain mTLS certificates.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if endpoint == "" {
				return fmt.Errorf("--endpoint is required")
			}

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use outputDir if specified, otherwise use project root
			var pkiDir string
			if outputDir != "" {
				pkiDir = filepath.Join(outputDir, ".g8e/pki")
			} else {
				pkiDir = constants.Paths.Infra.PkiDir
			}

			cmd.Println("Generating CSR for enrollment...")
			hostname, _ := os.Hostname()
			opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
			if err != nil {
				return fmt.Errorf("failed to generate operator CSR: %w", err)
			}

			cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
			if err != nil {
				return fmt.Errorf("failed to generate CLI CSR: %w", err)
			}

			cmd.Printf("Enrolling with Gateway at %s...\n", endpoint)
			regResp, err := auth.EnrollWithGateway(cfg, endpoint, opCSR, cliCSR, "")
			if err != nil {
				return fmt.Errorf("failed to enroll: %w", err)
			}

			if regResp.OperatorCert == "" {
				return fmt.Errorf("unexpected response: missing certificate")
			}

			if err := os.MkdirAll(pkiDir, 0700); err != nil {
				return fmt.Errorf("failed to create PKI directory: %w", err)
			}

			certPath := filepath.Join(pkiDir, "operator.crt")
			keyPath := filepath.Join(pkiDir, "operator.key")
			chainPath := filepath.Join(pkiDir, "operator.chain.pem")

			if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, opKey, certPath, keyPath); err != nil {
				return fmt.Errorf("failed to save operator certificate: %w", err)
			}

			// Save CLI cert separately
			cliCertPath := filepath.Join(pkiDir, "cli.crt")
			cliKeyPath := filepath.Join(pkiDir, "cli.key")
			if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cliCertPath, cliKeyPath); err != nil {
				return fmt.Errorf("failed to save CLI certificate: %w", err)
			}

			if err := os.WriteFile(chainPath, []byte(regResp.OperatorCertChain), 0600); err != nil {
				return fmt.Errorf("failed to save certificate chain: %w", err)
			}

			if regResp.HubTrustBundle != "" {
				trustDir := filepath.Join(pkiDir, "trust")
				if err := os.MkdirAll(trustDir, 0700); err != nil {
					return fmt.Errorf("failed to create trust directory: %w", err)
				}
				bundlePath := filepath.Join(trustDir, "g8eg-ca-bundle.pem")
				if err := os.WriteFile(bundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
					return fmt.Errorf("failed to save trust bundle: %w", err)
				}
			}

			cmd.Printf("\nEnrollment complete\n")
			cmd.Printf("Operator ID: %s\n", regResp.OperatorID)
			cmd.Printf("Operator Session ID: %s\n", regResp.OperatorSessionID)
			cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)
			cmd.Printf("Certificate saved to: %s\n", certPath)
			cmd.Printf("Key saved to: %s\n", keyPath)
			cmd.Printf("CLI Certificate saved to: %s\n", cliCertPath)
			cmd.Printf("CLI Key saved to: %s\n", cliKeyPath)
			if regResp.HubTrustBundle != "" {
				cmd.Printf("Trust bundle saved to: %s\n", filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"))
			}

			cmd.Printf("\n=== MCP Server Configuration ===\n")
			mcpConfig := map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"g8e-gateway": map[string]interface{}{
						"serverUrl": fmt.Sprintf("https://%s", endpoint),
						"headers": map[string]string{
							"Content-Type": "application/json",
						},
						"clientCertPath": certPath,
						"clientKeyPath":  keyPath,
						"caCertPath":     filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"),
						"operatorId":     regResp.OperatorID,
						"description":    "g8e Gateway - BFT-governed MCP/A2A protocol translator with L1/L2/L3 verification",
						"notes": []string{
							"Universal HTTP endpoint - no stdio bridge required",
							"Requires mTLS client certificate issued by the g8e Gateway CA",
							"Obtain client certificate via: ./g8e security pki enroll --endpoint <gateway-address>",
							"All tool calls are wrapped in GovernanceEnvelope and verified through the 3-layer governance sequence",
							"Local 'read_field' tool is available for JIT field resolution from governed collections",
						},
					},
				},
			}

			configJSON, err := json.MarshalIndent(mcpConfig, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal MCP config: %w", err)
			}
			cmd.Println(string(configJSON))

			return nil
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Gateway endpoint (e.g., 192.168.1.62:8441)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for certificates (default: project root)")

	return cmd
}
