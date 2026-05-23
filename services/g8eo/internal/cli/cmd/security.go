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

	"github.com/g8e-ai/g8e/services/g8eo/internal/cli/config"
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
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if pkiDir == "" {
				pkiDir = filepath.Join(cfg.ProjectRoot, ".g8e", "pki")
			}
			if secretsDir == "" {
				secretsDir = filepath.Join(cfg.ProjectRoot, ".g8e", "secrets")
			}

			cmd.Println("Running platform security validation...")
			failed := false

			// Check PKI directory structure
			cmd.Println("\n=== PKI Directory Structure ===")
			pkiFiles := []string{
				filepath.Join(pkiDir, "root", "root_ca.crt"),
				filepath.Join(pkiDir, "root", "root_ca.key"),
				filepath.Join(pkiDir, "trust", "hub-bundle.pem"),
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
			ports := []int{8440, 8443, 9000}
			for _, port := range ports {
				addr := fmt.Sprintf("127.0.0.1:%d", port)
				listener, err := net.Listen("tcp", addr)
				if err != nil {
					cmd.Printf("  [WARN] Port %d is in use\n", port)
				} else {
					listener.Close()
					cmd.Printf("  [OK]   Port %d is available\n", port)
				}
			}

			// Check TLS configuration
			cmd.Println("\n=== TLS Configuration ===")
			trustBundlePath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
			if trustData, err := os.ReadFile(trustBundlePath); err == nil {
				certPool := x509.NewCertPool()
				if certPool.AppendCertsFromPEM(trustData) {
					cmd.Printf("  [OK]   Trust bundle contains %d certificates\n", len(certPool.Subjects()))
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

	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "PKI directory (default: .g8e/pki)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Secrets directory (default: .g8e/secrets)")

	return cmd
}
