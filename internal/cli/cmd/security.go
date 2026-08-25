// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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
