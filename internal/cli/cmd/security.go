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
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/certutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
	return securityValidateCmdWithConfig(newFileSvc)
}

func securityValidateCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var pkiDir string
	var secretsDir string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run security validation checks",
		Long: `Run security validation checks against the local g8e installation. Verifies
PKI certificate existence and validity, checks that the HTTPS port is active,
and confirms that the CA bundle is properly configured for mTLS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Running platform security validation...")
			failed := false

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := context.Background()

			// Check PKI directory structure
			cmd.Println("\n=== PKI Directory Structure ===")
			type pathEntry struct {
				displayPath string
				relPath     string
			}
			var pkiEntries []pathEntry
			if pkiDir == "" {
				pkiEntries = []pathEntry{
					{fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)},
					{fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCAKey)), filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCAKey)},
					{fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)},
					{fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiFileWardenPub)), filepath.Join(constants.PkiDirname, constants.PkiFileWardenPub)},
				}
			} else {
				pkiEntries = []pathEntry{
					{filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA), ""},
					{filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCAKey), ""},
					{filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), ""},
					{filepath.Join(pkiDir, constants.PkiFileWardenPub), ""},
				}
			}
			for _, entry := range pkiEntries {
				var exists bool
				if entry.relPath != "" {
					exists, err = fileSvc.FileExists(ctx, entry.relPath)
					if err != nil {
						cmd.Printf("  [FAIL] %s: %v\n", entry.displayPath, err)
						failed = true
						continue
					}
				} else {
					_, statErr := os.Stat(entry.displayPath)
					exists = statErr == nil
				}
				if exists {
					cmd.Printf("  [OK]   %s exists\n", entry.displayPath)
				} else {
					cmd.Printf("  [FAIL] %s missing\n", entry.displayPath)
					failed = true
				}
			}

			// Check secrets directory
			cmd.Println("\n=== Secrets Directory ===")
			var secretEntries []pathEntry
			if secretsDir == "" {
				secretEntries = []pathEntry{
					{fileSvc.Resolve(filepath.Join(constants.SecretsDirname, constants.SecretsFileSessionEncryptionKey)), filepath.Join(constants.SecretsDirname, constants.SecretsFileSessionEncryptionKey)},
					{fileSvc.Resolve(filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)), filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)},
				}
			} else {
				secretEntries = []pathEntry{
					{filepath.Join(secretsDir, constants.SecretsFileSessionEncryptionKey), ""},
					{filepath.Join(secretsDir, constants.SecretsFileBootstrapDigest), ""},
				}
			}
			for _, entry := range secretEntries {
				var exists bool
				if entry.relPath != "" {
					exists, err = fileSvc.FileExists(ctx, entry.relPath)
					if err != nil {
						cmd.Printf("  [FAIL] %s: %v\n", entry.displayPath, err)
						failed = true
						continue
					}
				} else {
					_, statErr := os.Stat(entry.displayPath)
					exists = statErr == nil
				}
				if exists {
					cmd.Printf("  [OK]   %s exists\n", entry.displayPath)
				} else {
					cmd.Printf("  [FAIL] %s missing\n", entry.displayPath)
					failed = true
				}
			}

			// Validate root CA certificate
			cmd.Println("\n=== Certificate Validation ===")
			var rootCARelPath string
			var rootCADisplayPath string
			if pkiDir == "" {
				rootCARelPath = filepath.Join(constants.PkiDirname, constants.PkiSubdirRoot, constants.PkiFileRootCA)
				rootCADisplayPath = fileSvc.Resolve(rootCARelPath)
			} else {
				rootCADisplayPath = filepath.Join(pkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)
			}
			var certData []byte
			if rootCARelPath != "" {
				certData, err = fileSvc.ReadFile(ctx, rootCARelPath)
				if err != nil && !errors.Is(err, constants.ErrNotFound) {
					cmd.Printf("  [FAIL] failed to read root_ca.crt: %v\n", err)
					failed = true
				}
			} else {
				certData, err = os.ReadFile(rootCADisplayPath)
				if err != nil && !os.IsNotExist(err) {
					cmd.Printf("  [FAIL] failed to read root_ca.crt: %v\n", err)
					failed = true
				}
			}
			if certData != nil {
				certPool := x509.NewCertPool()
				if !certPool.AppendCertsFromPEM(certData) {
					cmd.Printf("  [FAIL] root_ca.crt is not a valid PEM certificate\n")
					failed = true
				} else {
					cmd.Printf("  [OK]   root_ca.crt is valid PEM\n")
				}
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
			var trustBundleRelPath string
			var trustBundleDisplayPath string
			if pkiDir == "" {
				trustBundleRelPath = filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
				trustBundleDisplayPath = fileSvc.Resolve(trustBundleRelPath)
			} else {
				trustBundleDisplayPath = filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
			}
			var trustData []byte
			if trustBundleRelPath != "" {
				trustData, err = fileSvc.ReadFile(ctx, trustBundleRelPath)
				if err != nil && !errors.Is(err, constants.ErrNotFound) {
					cmd.Printf("  [FAIL] failed to read trust bundle: %v\n", err)
					failed = true
				}
			} else {
				trustData, err = os.ReadFile(trustBundleDisplayPath)
				if err != nil && !os.IsNotExist(err) {
					cmd.Printf("  [FAIL] failed to read trust bundle: %v\n", err)
					failed = true
				}
			}
			if trustData != nil {
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
				return fmt.Errorf("security: validate: %w", constants.ErrValidationFailed)
			}
			cmd.Println("[OK]   Security validation passed")
			return nil
		},
	}

	cmd.Flags().StringVar(&pkiDir, "pki-dir", "", "PKI directory (default: "+constants.RuntimeDirname+"/"+constants.PkiDirname+")")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", "", "Secrets directory (default: "+constants.RuntimeDirname+"/"+constants.SecretsDirname+")")

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

// remoteOperatorEnroller is the subset of auth.EnrollmentClient used by
// `security pki enroll`. Injectable for tests.
type remoteOperatorEnroller interface {
	EnrollRemoteOperator(ctx context.Context, gatewayEndpoint, operatorCSR string, operatorKey *ecdsa.PrivateKey, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint string) (auth.EnrollmentArtifacts, error)
}

// defaultRemoteOperatorEnroller builds the production enrollment client for
// `security pki enroll`. The client uses plain HTTP (no mTLS) and performs no
// OS trust installation or passkey registration — `security pki enroll` is a
// headless operator-only enrollment path.
func defaultRemoteOperatorEnroller(cfg *config.Config) remoteOperatorEnroller {
	return auth.NewEnrollmentClient(cfg, nil)
}

func securityPKIEnrollCmd() *cobra.Command {
	return securityPKIEnrollCmdWithConfig(loadConfig, defaultRemoteOperatorEnroller, newFileSvc)
}

func securityPKIEnrollCmdWithConfig(configLoader func(string) (*config.Config, error), enrollerFactory func(*config.Config) remoteOperatorEnroller, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll an operator with the Gateway via CSR",
		Long:  `Generate a CSR and enroll with the Gateway to obtain Operator mTLS certificates.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, _ := cmd.Flags().GetString("endpoint")
			if endpoint == "" {
				return fmt.Errorf("security: enroll: %w", constants.ErrEndpointRequired)
			}

			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("security: load config: %w", err)
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			ctx := context.Background()

			var pkiDir string
			if outputDir != "" {
				pkiDir = filepath.Join(outputDir, constants.RuntimeDirname, constants.PkiDirname)
			} else {
				pkiDir = fileSvc.Resolve(constants.PkiDirname)
			}

			cmd.Println("Generating CSR for enrollment...")
			hostname, err := os.Hostname()
			if err != nil {
				return fmt.Errorf("security: get hostname: %w: %w", constants.ErrNetworkGetHostname, err)
			}
			opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
			if err != nil {
				return fmt.Errorf("security: generate CSR: %w", err)
			}

			// Append default HTTP port
			gatewayEndpoint := fmt.Sprintf("%s:%d", endpoint, constants.Ports.OperatorHttp)
			cmd.Printf("Enrolling with Gateway at %s...\n", gatewayEndpoint)
			enroller := enrollerFactory(cfg)
			// security pki enroll is a headless operator-only path: no CLI CSR,
			// no OS trust installation, no passkey ceremony. The enrollment
			// client validates the operator cert and (when present) the trust
			// bundle / CA fingerprint pin.
			artifacts, err := enroller.EnrollRemoteOperator(ctx, gatewayEndpoint, opCSR, opKey, "", nil, "")
			if err != nil {
				return fmt.Errorf("security: enroll: %w", err)
			}

			if artifacts.OperatorCertPEM == "" {
				return fmt.Errorf("security: enroll: %w", constants.ErrMissingCertificate)
			}

			if outputDir == "" {
				if err := fileSvc.MkdirAll(ctx, constants.PkiDirname, constants.PermDirPrivate); err != nil {
					return fmt.Errorf("security: create PKI dir: %w: %w", constants.ErrDirCreateFailed, err)
				}
			} else {
				if err := os.MkdirAll(pkiDir, constants.PermDirPrivate); err != nil {
					return fmt.Errorf("security: create PKI dir: %w: %w", constants.ErrDirCreateFailed, err)
				}
			}

			certPath := filepath.Join(pkiDir, constants.PkiFileOperatorCert)
			keyPath := filepath.Join(pkiDir, constants.PkiFileOperatorKey)
			chainPath := filepath.Join(pkiDir, constants.PkiFileOperatorChain)

			if outputDir == "" {
				certRel, err := fileSvc.RelFromAbs(certPath)
				if err != nil {
					return fmt.Errorf("security: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				keyRel, err := fileSvc.RelFromAbs(keyPath)
				if err != nil {
					return fmt.Errorf("security: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := auth.SaveCertAndKey(fileSvc, artifacts.OperatorCertPEM, artifacts.OperatorCertChainPEM, opKey, certRel, keyRel); err != nil {
					return fmt.Errorf("security: save cert and key: %w", err)
				}
			} else {
				certBytes, keyBytes, err := certutil.EncodeCertAndKey(artifacts.OperatorCertPEM, artifacts.OperatorCertChainPEM, opKey)
				if err != nil {
					return fmt.Errorf("security: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := os.WriteFile(keyPath, keyBytes, constants.PermFilePrivate); err != nil {
					return fmt.Errorf("security: save key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := os.WriteFile(certPath, certBytes, constants.PermFilePrivate); err != nil {
					return fmt.Errorf("security: save cert: %w: %w", constants.ErrCertSaveFailed, err)
				}
			}

			if outputDir == "" {
				chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorChain)
				if err := fileSvc.WriteFile(ctx, chainRelPath, []byte(artifacts.OperatorCertChainPEM), constants.PermFilePrivate); err != nil {
					return fmt.Errorf("security: save chain: %w: %w", constants.ErrChainSaveFailed, err)
				}
			} else {
				if err := os.WriteFile(chainPath, []byte(artifacts.OperatorCertChainPEM), constants.PermFilePrivate); err != nil {
					return fmt.Errorf("security: save chain: %w: %w", constants.ErrChainSaveFailed, err)
				}
			}

			if artifacts.TrustBundlePEM != "" {
				trustDir := filepath.Join(pkiDir, constants.PkiSubdirTrust)
				if outputDir == "" {
					trustRelDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust)
					if err := fileSvc.MkdirAll(ctx, trustRelDir, constants.PermDirPrivate); err != nil {
						return fmt.Errorf("security: create trust dir: %w: %w", constants.ErrDirCreateFailed, err)
					}
					bundleRelPath := filepath.Join(trustRelDir, constants.PkiFileGatewayBundle)
					if err := fileSvc.WriteFile(ctx, bundleRelPath, []byte(artifacts.TrustBundlePEM), constants.PermFilePublic); err != nil {
						return fmt.Errorf("security: save trust bundle: %w: %w", constants.ErrTrustSaveFailed, err)
					}
				} else {
					if err := os.MkdirAll(trustDir, constants.PermDirPrivate); err != nil {
						return fmt.Errorf("security: create trust dir: %w: %w", constants.ErrDirCreateFailed, err)
					}
					bundlePath := filepath.Join(trustDir, constants.PkiFileGatewayBundle)
					if err := os.WriteFile(bundlePath, []byte(artifacts.TrustBundlePEM), constants.PermFilePublic); err != nil {
						return fmt.Errorf("security: save trust bundle: %w: %w", constants.ErrTrustSaveFailed, err)
					}
				}
			}

			cmd.Printf("\nEnrollment complete\n")
			cmd.Printf("Operator ID: %s\n", artifacts.OperatorID)
			cmd.Printf("Operator Session ID: %s\n", artifacts.OperatorSessionID)
			cmd.Printf("Certificate saved to: %s\n", certPath)
			cmd.Printf("Key saved to: %s\n", keyPath)
			if artifacts.TrustBundlePEM != "" {
				cmd.Printf("Trust bundle saved to: %s\n", filepath.Join(pkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for certificates (default: project root)")

	return cmd
}
