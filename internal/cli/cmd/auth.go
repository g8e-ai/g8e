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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	clierrors "github.com/g8e-ai/g8e/internal/cli/errors"
	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and cli/web/operator session management",
		Long:  `Manage mTLS enrollment and Operator sessions via CSR-based authentication.`,
	}

	cmd.AddCommand(
		loginCmd(),
		logoutCmd(),
		enrollWindowsCmd(),
		serveHTTPSCmd(),
	)

	return cmd
}

func loginCmd() *cobra.Command {
	return loginCmdWithConfig(config.Load)
}

func loginCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate CLI with the running Gateway",
		Long:  `Authenticate your local CLI with the running Gateway via CSR-based enrollment. Generates client keypairs, submits CSRs to the Gateway's CA, and saves signed mTLS credentials. The Gateway must already be running (use './g8e gw start' first).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			// Check if platform is already bootstrapped
			bootstrapped, err := auth.CheckBootstrapStatus(cfg)
			if err != nil {
				return fmt.Errorf("failed to check bootstrap status: %w", err)
			}

			// Check if local credentials exist
			hasLocalCreds := true
			if _, err := os.Stat(cfg.CredentialsFile()); os.IsNotExist(err) {
				hasLocalCreds = false
			}
			if _, err := os.Stat(cfg.CLICertFile()); os.IsNotExist(err) {
				hasLocalCreds = false
			}

			// If platform is not bootstrapped, perform first-time bootstrap
			if !bootstrapped {
				cmd.Println("Gateway not bootstrapped. Performing first-time client enrollment...")

				cmd.Println("Generating keys and CSRs...")
				hostname, _ := os.Hostname()
				cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
				if err != nil {
					return fmt.Errorf("failed to generate CLI CSR: %w", err)
				}

				cmd.Println("Bootstrapping with operator...")
				regResp, err := auth.Bootstrap(cfg, "", cliCSR, "")
				if err != nil {
					return err
				}

				if regResp.CLISessionID == "" || regResp.CLICert == "" {
					return fmt.Errorf("unexpected bootstrap response (missing required fields)")
				}

				if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
					return fmt.Errorf("failed to save CLI credentials: %w", err)
				}

				if regResp.HubTrustBundle != "" {
					// Save CA bundle to credentials directory alongside client certs
					hubBundlePath := filepath.Join(cfg.CredentialsDir, "g8eg-ca-bundle.pem")
					if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
						return fmt.Errorf("failed to save hub trust bundle: %w", err)
					}
				}

				creds := &auth.Credentials{
					OperatorSessionID: regResp.OperatorSessionID,
					UserID:            regResp.UserID,
					OperatorID:        regResp.OperatorID,
					CLISessionID:      regResp.CLISessionID,
				}

				if err := auth.SaveCredentials(cfg, creds); err != nil {
					return fmt.Errorf("failed to save credentials: %w", err)
				}

				cmd.Printf("\nClient enrollment complete\n")
				cmd.Printf("User ID: %s\n", regResp.UserID)
				cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)

				// Require passkey registration for first-time bootstrap
				cmd.Println("\nInitializing passkey registration...")
				if err := auth.RegisterPasskeyViaLocalhost(cfg, regResp.UserID); err != nil {
					cmd.Printf("Warning: passkey registration failed: %v\n", err)
					cmd.Println("You can register a passkey later via the web interface.")
				}

				return nil
			}

			// Platform is bootstrapped but local credentials are missing
			if !hasLocalCreds {
				cmd.Println("Gateway already bootstrapped. Performing CLI enrollment...")

				cmd.Println("Generating keys and CSRs...")
				hostname, _ := os.Hostname()
				cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
				if err != nil {
					return fmt.Errorf("failed to generate CLI CSR: %w", err)
				}

				cmd.Println("Enrolling with gateway...")
				regResp, err := auth.CLIEnroll(cfg, cliCSR)
				if err != nil {
					return err
				}

				if regResp.CLISessionID == "" || regResp.CLICert == "" {
					return fmt.Errorf("unexpected CLI enrollment response (missing required fields)")
				}

				if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
					return fmt.Errorf("failed to save CLI credentials: %w", err)
				}

				if regResp.HubTrustBundle != "" {
					hubBundlePath := filepath.Join(cfg.CredentialsDir, "g8eg-ca-bundle.pem")
					if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
						return fmt.Errorf("failed to save hub trust bundle: %w", err)
					}
				}

				creds := &auth.Credentials{
					OperatorSessionID: regResp.OperatorSessionID,
					UserID:            regResp.UserID,
					OperatorID:        regResp.OperatorID,
					CLISessionID:      regResp.CLISessionID,
				}

				if err := auth.SaveCredentials(cfg, creds); err != nil {
					return fmt.Errorf("failed to save credentials: %w", err)
				}

				cmd.Printf("\nClient enrollment complete\n")
				cmd.Printf("User ID: %s\n", regResp.UserID)
				cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)

				return nil
			}

			// Platform already bootstrapped and has local credentials - attempt CSR-based re-enrollment with mTLS
			cmd.Println("Gateway already bootstrapped. Attempting re-enrollment via CSR with mTLS...")

			// Check if operator certificate exists (CLI-only bootstrap has no operator)
			hasOperatorCert := true
			if _, err := os.Stat(cfg.OperatorCertFile()); os.IsNotExist(err) {
				hasOperatorCert = false
			}

			// Check if certificates are expiring soon and auto-renew if needed
			cmd.Println("Checking certificate expiry...")
			if err := auth.AutoRenewCertificate(cfg, "cli", ""); err != nil {
				return fmt.Errorf("CLI certificate auto-renewal failed: %w", err)
			}

			cmd.Println("Generating keys and CSRs...")
			hostname, _ := os.Hostname()
			cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
			if err != nil {
				return fmt.Errorf("failed to generate CLI CSR: %w", err)
			}

			var regResp *auth.RegistrationResponse
			if hasOperatorCert {
				// Full re-enrollment with operator CSR
				cmd.Println("Re-enrolling with operator...")
				regResp, err = auth.ReEnroll(cfg, "", cliCSR, "")
				if err != nil {
					// Check if this is a TLS verification error (stale trust bundle after gateway PKI regeneration)
					if strings.Contains(err.Error(), "certificate signed by unknown authority") ||
						strings.Contains(err.Error(), "x509: certificate") {
						return fmt.Errorf("mTLS re-enrollment failed: trust bundle is stale (gateway PKI was regenerated). To recover, run: ./g8e auth logout && ./g8e auth login. Original error: %w", err)
					}
					return err
				}
			} else {
				// CLI-only re-enrollment (no operator)
				cmd.Println("Re-enrolling CLI credentials...")
				regResp, err = auth.CLIEnroll(cfg, cliCSR)
				if err != nil {
					return err
				}
			}

			if regResp.CLISessionID == "" || regResp.CLICert == "" {
				return fmt.Errorf("unexpected registration response (missing required fields)")
			}

			if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
				return fmt.Errorf("failed to save CLI credentials: %w", err)
			}

			if regResp.HubTrustBundle != "" {
				// Save CA bundle to credentials directory alongside client certs
				hubBundlePath := filepath.Join(cfg.CredentialsDir, "g8eg-ca-bundle.pem")
				if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
					return fmt.Errorf("failed to save hub trust bundle: %w", err)
				}
			}

			creds := &auth.Credentials{
				OperatorSessionID: regResp.OperatorSessionID,
				UserID:            regResp.UserID,
				OperatorID:        regResp.OperatorID,
				CLISessionID:      regResp.CLISessionID,
			}

			if err := auth.SaveCredentials(cfg, creds); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}

			cmd.Printf("\nClient re-enrollment complete\n")
			cmd.Printf("User ID: %s\n", regResp.UserID)
			cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)

			return nil
		},
	}

	return cmd
}

func logoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear local Operator session and credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			creds, err := auth.LoadCredentials(cfg)
			if err != nil {
				return fmt.Errorf("%w: %w", clierrors.ErrFailedToLoadCredentials, err)
			}

			if creds == nil {
				cmd.Println("No active session found")
				return nil
			}

			if err := auth.DeleteCredentials(cfg); err != nil {
				return fmt.Errorf("failed to delete credentials: %w", err)
			}

			cmd.Println("Logged out successfully")
			return nil
		},
	}
	return cmd
}

func enrollWindowsCmd() *cobra.Command {
	var useTPM bool

	cmd := &cobra.Command{
		Use:   "enroll-windows",
		Short: "Enroll via Windows Certificate Store (Windows only)",
		Long:  `Generate an ECDSA P-256 keypair in the Windows Certificate Store, submit a CSR to the Gateway, and import the signed certificate. Chrome/Edge will automatically present this cert. Use --tpm for TPM-backed keys via Windows Hello for Business.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			// Check if platform is already bootstrapped
			bootstrapped, err := auth.CheckBootstrapStatus(cfg)
			if err != nil {
				return fmt.Errorf("failed to check bootstrap status: %w", err)
			}

			cmd.Println("Generating ECDSA P-256 keypair in Windows Certificate Store...")
			hostname, _ := os.Hostname()
			csr, privKey, err := auth.GenerateWindowsCSR(fmt.Sprintf("g8e-windows-%s", hostname), useTPM)
			if err != nil {
				return fmt.Errorf("failed to generate Windows CSR: %w", err)
			}

			var regResp *auth.RegistrationResponse
			if !bootstrapped {
				cmd.Println("Submitting CSR to Gateway for bootstrap...")
				regResp, err = auth.Bootstrap(cfg, csr, "", "")
				if err != nil {
					return fmt.Errorf("failed to submit CSR: %w", err)
				}
			} else {
				cmd.Println("Platform already bootstrapped. Attempting re-enrollment via CSR with mTLS...")
				regResp, err = auth.ReEnroll(cfg, csr, "", "")
				if err != nil {
					// Check if this is a TLS verification error (stale trust bundle after gateway PKI regeneration)
					if strings.Contains(err.Error(), "certificate signed by unknown authority") ||
						strings.Contains(err.Error(), "x509: certificate") {
						return fmt.Errorf("mTLS re-enrollment failed: trust bundle is stale (gateway PKI was regenerated). To recover, run: ./g8e auth logout && ./g8e auth enroll-windows. Original error: %w", err)
					}
					return fmt.Errorf("failed to re-enroll: %w", err)
				}
			}

			if regResp.OperatorCert == "" {
				return fmt.Errorf("unexpected response: missing certificate")
			}

			cmd.Println("Importing signed certificate to Windows Certificate Store...")
			if err := auth.ImportCertificateToWindowsStore(regResp.OperatorCert); err != nil {
				cmd.Printf("Warning: failed to import to Windows store: %v\n", err)
				cmd.Println("Certificate will be saved to local filesystem instead")
			}

			if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, privKey, cfg.OperatorCertFile(), cfg.OperatorKeyFile()); err != nil {
				return fmt.Errorf("failed to save certificate locally: %w", err)
			}

			if regResp.HubTrustBundle != "" {
				// Save CA bundle to credentials directory alongside client certs
				hubBundlePath := filepath.Join(cfg.CredentialsDir, "g8eg-ca-bundle.pem")
				if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0644); err != nil {
					return fmt.Errorf("failed to save hub trust bundle: %w", err)
				}
			}

			creds := &auth.Credentials{
				OperatorSessionID: regResp.OperatorSessionID,
				UserID:            regResp.UserID,
				OperatorID:        regResp.OperatorID,
				CLISessionID:      regResp.CLISessionID,
			}

			if err := auth.SaveCredentials(cfg, creds); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}

			cmd.Printf("\nWindows enrollment complete\n")
			cmd.Printf("User ID: %s\n", regResp.UserID)
			cmd.Printf("Operator Session ID: %s\n", regResp.OperatorSessionID)
			cmd.Printf("Operator ID: %s\n", regResp.OperatorID)
			cmd.Println("\nNEXT STEP: Close and reopen your browser.")
			cmd.Println("Chrome/Edge will automatically present your certificate from the Windows Certificate Store.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&useTPM, "tpm", false, "Use TPM-backed key via Windows Hello for Business")

	return cmd
}

func serveHTTPSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve-https",
		Short: "Start HTTPS server with passkey authentication landing page",
		Long:  `Start a simple HTTPS server on localhost with a landing page for passkey authentication. Uses the operator certificate from './g8e auth enroll-windows'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return auth.ServeHTTPS(cfg, "")
		},
	}

	return cmd
}
