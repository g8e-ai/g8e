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
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
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
				pkiDir = paths.Infra.PkiDir
			}
			if secretsDir == "" {
				secretsDir = paths.Infra.SecretsDir
			}

			cmd.Println("Running platform security validation...")
			failed := false

			// Check PKI directory structure
			cmd.Println("\n=== PKI Directory Structure ===")
			pkiFiles := []string{
				filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA),
				filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCAKey),
				filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle),
				filepath.Join(pkiDir, constants.PkiFileWardenPub),
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
				filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey),
				filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest),
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
			rootCAPath := filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)
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
				constants.Ports.OperatorHttp,
				constants.Ports.OperatorHttps,
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
			trustBundlePath := filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
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
				return fmt.Errorf("security: %w", constants.ErrValidationFailed)
			}
			cmd.Println("[OK]   Security validation passed")
			return nil
		},
	}

	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "PKI directory (default: "+paths.Infra.PkiDir+")")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Secrets directory (default: "+paths.Infra.SecretsDir+")")

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
		Short: "Enroll an operator with the Gateway via CSR",
		Long:  `Generate a CSR and enroll with the Gateway to obtain Operator mTLS certificates.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if endpoint == "" {
				return fmt.Errorf("security: %w", constants.ErrEndpointRequired)
			}

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("security: %w", err)
			}

			// Use outputDir if specified, otherwise use project root
			var pkiDir string
			if outputDir != "" {
				pkiDir = filepath.Join(outputDir, paths.Infra.PkiDir)
			} else {
				pkiDir = paths.Infra.PkiDir
			}

			cmd.Println("Generating CSR for enrollment...")
			hostname, err := os.Hostname()
			if err != nil {
				return fmt.Errorf("security: failed to get hostname: %w", err)
			}
			opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
			if err != nil {
				return fmt.Errorf("security: %w", err)
			}

			// Append default HTTP port
			gatewayEndpoint := fmt.Sprintf("%s:%d", endpoint, constants.Ports.OperatorHttp)
			cmd.Printf("Enrolling with Gateway at %s...\n", gatewayEndpoint)
			regResp, err := auth.EnrollWithGateway(cfg, gatewayEndpoint, opCSR, "", "")
			if err != nil {
				return fmt.Errorf("security: %w", err)
			}

			if regResp.OperatorCert == "" {
				return fmt.Errorf("security: %w", constants.ErrMissingCertificate)
			}

			if err := os.MkdirAll(pkiDir, constants.PermDirPrivate); err != nil {
				return fmt.Errorf("security: failed to create PKI directory: %w", err)
			}

			certPath := filepath.Join(pkiDir, constants.PkiFileOperatorCert)
			keyPath := filepath.Join(pkiDir, constants.PkiFileOperatorKey)
			chainPath := filepath.Join(pkiDir, constants.PkiFileOperatorChain)

			if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, opKey, certPath, keyPath); err != nil {
				return fmt.Errorf("security: %w", err)
			}

			if err := os.WriteFile(chainPath, []byte(regResp.OperatorCertChain), constants.PermFilePrivate); err != nil {
				return fmt.Errorf("security: failed to save certificate chain: %w", err)
			}

			if regResp.HubTrustBundle != "" {
				trustDir := filepath.Join(pkiDir, constants.PkiSubdirTrust)
				if err := os.MkdirAll(trustDir, constants.PermDirPrivate); err != nil {
					return fmt.Errorf("security: failed to create trust directory: %w", err)
				}
				bundlePath := filepath.Join(trustDir, constants.PkiFileGatewayBundle)
				if err := os.WriteFile(bundlePath, []byte(regResp.HubTrustBundle), constants.PermFilePublic); err != nil {
					return fmt.Errorf("security: failed to save trust bundle: %w", err)
				}
			}

			cmd.Printf("\nEnrollment complete\n")
			cmd.Printf("Operator ID: %s\n", regResp.OperatorID)
			cmd.Printf("Operator Session ID: %s\n", regResp.OperatorSessionID)
			cmd.Printf("Certificate saved to: %s\n", certPath)
			cmd.Printf("Key saved to: %s\n", keyPath)
			if regResp.HubTrustBundle != "" {
				cmd.Printf("Trust bundle saved to: %s\n", filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&endpoint, "endpoint", "e", "", "Gateway IP address (e.g., 192.168.1.62)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for certificates (default: project root)")

	return cmd
}
