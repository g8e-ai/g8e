// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/certutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// remoteOperatorEnroller is the subset of auth.EnrollmentClient used by
// `auth enroll operator`. Injectable for tests.
type remoteOperatorEnroller interface {
	EnrollRemoteOperator(ctx context.Context, gatewayEndpoint, operatorCSR string, operatorKey *ecdsa.PrivateKey, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint string) (auth.EnrollmentArtifacts, error)
}

// defaultRemoteOperatorEnroller builds the production enrollment client for
// `auth enroll operator`. The client uses plain HTTP (no mTLS) and performs no
// OS trust installation or passkey registration — `auth enroll operator` is a
// headless operator-only enrollment path.
func defaultRemoteOperatorEnroller(cfg *config.Config) remoteOperatorEnroller {
	return auth.NewEnrollmentClient(cfg, nil)
}

func enrollOperatorCmd() *cobra.Command {
	return enrollOperatorCmdWithConfig(loadConfig, defaultRemoteOperatorEnroller, newFileSvc)
}

func enrollOperatorCmdWithConfig(configLoader func(string) (*config.Config, error), enrollerFactory func(*config.Config) remoteOperatorEnroller, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Enroll an operator with the Gateway via CSR",
		Long: `Generate a CSR and enroll with the Gateway to obtain Operator mTLS certificates.

This is the remote operator/device enrollment path: it generates an operator
CSR, calls the gateway's device enrollment endpoint, and writes the resulting
operator cert/key/chain (and optional trust bundle) to the PKI directory. It is
headless and operator-only — it does not install OS trust, register a passkey,
or produce a CLI session. For the local human CLI/user enrollment path
(passkey ceremony, OS trust installation, CLI session), use ` + "`auth enroll user`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, _ := cmd.Flags().GetString("endpoint")
			if endpoint == "" {
				return fmt.Errorf("auth enroll operator: %w", constants.ErrEndpointRequired)
			}

			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("auth enroll operator: load config: %w", err)
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
				return fmt.Errorf("auth enroll operator: get hostname: %w: %w", constants.ErrNetworkGetHostname, err)
			}
			opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
			if err != nil {
				return fmt.Errorf("auth enroll operator: generate CSR: %w", err)
			}

			// Append default HTTP port
			gatewayEndpoint := fmt.Sprintf("%s:%d", endpoint, constants.Ports.OperatorHttp)
			cmd.Printf("Enrolling with Gateway at %s...\n", gatewayEndpoint)
			enroller := enrollerFactory(cfg)
			// auth enroll operator is a headless operator-only path: no CLI CSR,
			// no OS trust installation, no passkey ceremony. The enrollment
			// client validates the operator cert and (when present) the trust
			// bundle / CA fingerprint pin.
			artifacts, err := enroller.EnrollRemoteOperator(ctx, gatewayEndpoint, opCSR, opKey, "", nil, "")
			if err != nil {
				return fmt.Errorf("auth enroll operator: %w", err)
			}

			if artifacts.OperatorCertPEM == "" {
				return fmt.Errorf("auth enroll operator: %w", constants.ErrMissingCertificate)
			}

			if outputDir == "" {
				if err := fileSvc.MkdirAll(ctx, constants.PkiDirname, constants.PermDirPrivate); err != nil {
					return fmt.Errorf("auth enroll operator: create PKI dir: %w: %w", constants.ErrDirCreateFailed, err)
				}
			} else {
				if err := os.MkdirAll(pkiDir, constants.PermDirPrivate); err != nil {
					return fmt.Errorf("auth enroll operator: create PKI dir: %w: %w", constants.ErrDirCreateFailed, err)
				}
			}

			certPath := filepath.Join(pkiDir, constants.PkiFileOperatorCert)
			keyPath := filepath.Join(pkiDir, constants.PkiFileOperatorKey)
			chainPath := filepath.Join(pkiDir, constants.PkiFileOperatorChain)

			if outputDir == "" {
				certRel, err := fileSvc.RelFromAbs(certPath)
				if err != nil {
					return fmt.Errorf("auth enroll operator: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				keyRel, err := fileSvc.RelFromAbs(keyPath)
				if err != nil {
					return fmt.Errorf("auth enroll operator: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := auth.SaveCertAndKey(fileSvc, artifacts.OperatorCertPEM, artifacts.OperatorCertChainPEM, opKey, certRel, keyRel); err != nil {
					return fmt.Errorf("auth enroll operator: save cert and key: %w", err)
				}
			} else {
				certBytes, keyBytes, err := certutil.EncodeCertAndKey(artifacts.OperatorCertPEM, artifacts.OperatorCertChainPEM, opKey)
				if err != nil {
					return fmt.Errorf("auth enroll operator: save cert and key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := os.WriteFile(keyPath, keyBytes, constants.PermFilePrivate); err != nil {
					return fmt.Errorf("auth enroll operator: save key: %w: %w", constants.ErrCertSaveFailed, err)
				}
				if err := os.WriteFile(certPath, certBytes, constants.PermFilePrivate); err != nil {
					return fmt.Errorf("auth enroll operator: save cert: %w: %w", constants.ErrCertSaveFailed, err)
				}
			}

			if outputDir == "" {
				chainRelPath := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorChain)
				if err := fileSvc.WriteFile(ctx, chainRelPath, []byte(artifacts.OperatorCertChainPEM), constants.PermFilePrivate); err != nil {
					return fmt.Errorf("auth enroll operator: save chain: %w: %w", constants.ErrChainSaveFailed, err)
				}
			} else {
				if err := os.WriteFile(chainPath, []byte(artifacts.OperatorCertChainPEM), constants.PermFilePrivate); err != nil {
					return fmt.Errorf("auth enroll operator: save chain: %w: %w", constants.ErrChainSaveFailed, err)
				}
			}

			if artifacts.TrustBundlePEM != "" {
				trustDir := filepath.Join(pkiDir, constants.PkiSubdirTrust)
				if outputDir == "" {
					trustRelDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust)
					if err := fileSvc.MkdirAll(ctx, trustRelDir, constants.PermDirPrivate); err != nil {
						return fmt.Errorf("auth enroll operator: create trust dir: %w: %w", constants.ErrDirCreateFailed, err)
					}
					bundleRelPath := filepath.Join(trustRelDir, constants.PkiFileGatewayBundle)
					if err := fileSvc.WriteFile(ctx, bundleRelPath, []byte(artifacts.TrustBundlePEM), constants.PermFilePublic); err != nil {
						return fmt.Errorf("auth enroll operator: save trust bundle: %w: %w", constants.ErrTrustSaveFailed, err)
					}
				} else {
					if err := os.MkdirAll(trustDir, constants.PermDirPrivate); err != nil {
						return fmt.Errorf("auth enroll operator: create trust dir: %w: %w", constants.ErrDirCreateFailed, err)
					}
					bundlePath := filepath.Join(trustDir, constants.PkiFileGatewayBundle)
					if err := os.WriteFile(bundlePath, []byte(artifacts.TrustBundlePEM), constants.PermFilePublic); err != nil {
						return fmt.Errorf("auth enroll operator: save trust bundle: %w: %w", constants.ErrTrustSaveFailed, err)
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
