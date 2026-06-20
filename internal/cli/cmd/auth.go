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
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and session management",
		Long:  `Manage mTLS enrollment and CLI/web/operator sessions via CSR-based authentication.`,
	}

	cmd.AddCommand(
		enrollCmd(),
		loginCmd(), // hidden alias for backward compatibility
		logoutCmd(),
		enrollWindowsCmd(),
		approveCmd(),
	)

	return cmd
}

func enrollCmd() *cobra.Command {
	return enrollCmdWithConfig(config.Load)
}

// loginCmd is a hidden backward-compatible alias for enrollCmd.
func loginCmd() *cobra.Command {
	cmd := enrollCmdWithConfig(config.Load)
	cmd.Use = "login"
	cmd.Hidden = true
	return cmd
}

func enrollCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll CLI with the running Gateway and register a passkey",
		Long:  `Enroll your local CLI with the running Gateway via CSR-based enrollment, then register a passkey for secure authentication. Generates client keypairs, submits CSRs to the Gateway's CA, saves signed mTLS credentials, and opens a browser to register a WebAuthn/FIDO2 passkey. The Gateway must already be running (use './g8e gw start' first). On Windows, this automatically enrolls via Windows Certificate Store for passkey authentication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			// On Windows, use Windows Certificate Store enrollment for passkey auth
			if runtime.GOOS == "windows" {
				return performWindowsEnroll(cmd, cfg)
			}

			// Non-Windows: use standard CLI enrollment flow
			return performStandardEnroll(cmd, cfg)
		},
	}

	return cmd
}

// performWindowsEnroll handles Windows-specific enrollment with Certificate Store and passkey registration.
func performWindowsEnroll(cmd *cobra.Command, cfg *config.Config) error {
	cmd.Println("Windows detected: Using Windows Certificate Store for passkey authentication")

	// Check if platform is already bootstrapped
	bootstrapped, err := auth.CheckBootstrapStatus(cfg, "")
	if err != nil {
		return err
	}

	// Check if operator credentials exist
	hasOperatorCreds := true
	if _, err := os.Stat(cfg.OperatorCertFile()); os.IsNotExist(err) {
		hasOperatorCreds = false
	}
	if _, err := os.Stat(cfg.OperatorKeyFile()); os.IsNotExist(err) {
		hasOperatorCreds = false
	}

	// If platform is not bootstrapped or operator credentials missing, perform Windows enrollment
	if !bootstrapped || !hasOperatorCreds {
		cmd.Println("Enrolling via Windows Certificate Store...")

		cmd.Println("Generating ECDSA P-256 keypair in Windows Certificate Store...")
		hostname, _ := os.Hostname()
		csr, privKey, err := auth.GenerateWindowsCSR(fmt.Sprintf("g8e-cli-%s", hostname), false)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
		}

		var regResp *auth.RegistrationResponse
		if !bootstrapped {
			cmd.Println("Submitting CSR to Gateway for CLI enrollment...")
			regResp, err = auth.BootstrapWithURL(cfg, "", csr, "", "")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
			}
		} else {
			cmd.Println("Platform already bootstrapped. Attempting CLI re-enrollment...")
			regResp, err = auth.CLIEnroll(cfg, csr, "")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
			}
		}

		if regResp.CLICert == "" {
			return constants.ErrMissingCertificate
		}

		cmd.Println("Importing signed certificate to Windows Certificate Store...")
		if err := auth.ImportCertificateToWindowsStore(regResp.CLICert); err != nil {
			cmd.Printf("Warning: failed to import to Windows store: %v\n", err)
			cmd.Println("Certificate will be saved to local filesystem instead")
		}

		if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, privKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}

		if regResp.HubTrustBundle != "" {
			if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
			}

			// Trust the Root CA in Windows store for local HTTPS server
			if err := auth.TrustRootCAInWindowsStore(regResp.HubTrustBundle); err != nil {
				cmd.Printf("Warning: failed to trust Root CA in Windows store: %v\n", err)
				cmd.Println("You may see 'Not Secure' warnings in your browser.")
			}
		}

		creds := &auth.Credentials{
			UserID:       regResp.UserID,
			CLISessionID: regResp.CLISessionID,
		}

		if err := auth.SaveCredentials(cfg, creds); err != nil {
			return err
		}

		cmd.Printf("\nWindows enrollment complete\n")
		cmd.Printf("User ID: %s\n", regResp.UserID)
		cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)
	}

	// Attempt native Windows Hello authentication
	cmd.Println("\nAttempting native Windows Hello authentication...")
	if err := auth.PerformNativeWindowsAuth(cfg); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrWindowsHelloAuthentication, err)
	}

	cmd.Println("✓ Native authentication successful!")
	cmd.Println("\nLogin complete!")

	return nil
}

// performStandardEnroll handles standard CLI enrollment and passkey registration for non-Windows platforms.
func performStandardEnroll(cmd *cobra.Command, cfg *config.Config) error {
	// Check if local credentials exist
	hasLocalCreds := true
	if _, err := os.Stat(cfg.CredentialsFile()); os.IsNotExist(err) {
		hasLocalCreds = false
	}
	if _, err := os.Stat(cfg.CLICertFile()); os.IsNotExist(err) {
		hasLocalCreds = false
	}

	// No local credentials: first-time bootstrap or new CLI on an existing gateway.
	if !hasLocalCreds {
		cmd.Println("Performing CLI enrollment...")
		if err := auth.EnrollCLI(cfg); err != nil {
			return err
		}
		creds, err := auth.LoadCredentials(cfg)
		if err != nil || creds == nil {
			return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
		}
		cmd.Printf("\nClient enrollment complete\n")
		cmd.Printf("User ID: %s\n", creds.UserID)
		cmd.Printf("CLI Session ID: %s\n", creds.CLISessionID)

		// Register passkey for the newly enrolled user
		cmd.Println("\nRegistering passkey for secure authentication...")
		if err := auth.RegisterPasskeyViaLocalhost(cfg, creds.UserID, creds.CLISessionID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
		}
		return nil
	}

	// Local credentials present — attempt CSR-based re-enrollment with mTLS.
	cmd.Println("Gateway already bootstrapped. Attempting re-enrollment via CSR with mTLS...")

	// Check if operator certificate exists (CLI-only bootstrap has no operator)
	hasOperatorCert := true
	if _, err := os.Stat(cfg.OperatorCertFile()); os.IsNotExist(err) {
		hasOperatorCert = false
	}

	// Check if certificates are expiring soon and auto-renew if needed
	cmd.Println("Checking certificate expiry...")
	if err := auth.AutoRenewCertificate(cfg, "cli", ""); err != nil {
		return err
	}

	cmd.Println("Generating keys and CSRs...")
	hostname, _ := os.Hostname()
	cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	var regResp *auth.RegistrationResponse
	if hasOperatorCert {
		// Full re-enrollment with operator CSR
		cmd.Println("Re-enrolling with operator...")
		regResp, err = auth.ReEnroll(cfg, "", cliCSR, "", "")
		if err != nil {
			// Check if this is a TLS verification error (stale trust bundle after gateway PKI regeneration)
			if errors.Is(err, constants.ErrTrustBundleStale) {
				return fmt.Errorf("%w: %w", constants.ErrTrustBundleStale, err)
			}
			return err
		}
	} else {
		// CLI-only re-enrollment (no operator)
		cmd.Println("Re-enrolling CLI credentials...")
		regResp, err = auth.CLIEnroll(cfg, cliCSR, "")
		if err != nil {
			return err
		}
	}

	if regResp.CLISessionID == "" || regResp.CLICert == "" {
		return constants.ErrMissingRequiredField
	}

	if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	if regResp.HubTrustBundle != "" {
		if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
	}

	creds := &auth.Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := auth.SaveCredentials(cfg, creds); err != nil {
		return err
	}

	cmd.Printf("\nClient re-enrollment complete\n")
	cmd.Printf("User ID: %s\n", regResp.UserID)
	cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)

	return nil
}

func logoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear local Operator session and credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			creds, err := auth.LoadCredentials(cfg)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
			}

			if creds == nil {
				cmd.Println("No active session found")
				return nil
			}

			if err := auth.DeleteCredentials(cfg); err != nil {
				return err
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
		Short: "Enroll via Windows Certificate Store (Windows only - advanced)",
		Long: `Generate an ECDSA P-256 keypair in the Windows Certificate Store, submit a CSR to the Gateway, and import the signed certificate. Chrome/Edge will automatically present this cert. Use --tpm for TPM-backed keys via Windows Hello for Business.

NOTE: This is now handled automatically by './g8e auth enroll' on Windows. This command is for advanced use cases or manual re-enrollment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			// Check if platform is already bootstrapped
			bootstrapped, err := auth.CheckBootstrapStatus(cfg, "")
			if err != nil {
				return err
			}

			cmd.Println("Generating ECDSA P-256 keypair in Windows Certificate Store...")
			hostname, _ := os.Hostname()
			csr, privKey, err := auth.GenerateWindowsCSR(fmt.Sprintf("g8e-windows-%s", hostname), useTPM)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
			}

			var regResp *auth.RegistrationResponse
			if !bootstrapped {
				cmd.Println("Submitting CSR to Gateway for bootstrap...")
				regResp, err = auth.BootstrapWithURL(cfg, csr, "", "", "")
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
				}
			} else {
				cmd.Println("Platform already bootstrapped. Attempting re-enrollment via CSR with mTLS...")
				regResp, err = auth.ReEnroll(cfg, csr, "", "", "")
				if err != nil {
					// Check if this is a TLS verification error (stale trust bundle after gateway PKI regeneration)
					if errors.Is(err, constants.ErrTrustBundleStale) {
						return fmt.Errorf("%w: %w", constants.ErrTrustBundleStale, err)
					}
					return fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
				}
			}

			if regResp.OperatorCert == "" {
				return constants.ErrMissingCertificate
			}

			cmd.Println("Importing signed certificate to Windows Certificate Store...")
			if err := auth.ImportCertificateToWindowsStore(regResp.OperatorCert); err != nil {
				cmd.Printf("Warning: failed to import to Windows store: %v\n", err)
				cmd.Println("Certificate will be saved to local filesystem instead")
			}

			if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, privKey, cfg.OperatorCertFile(), cfg.OperatorKeyFile()); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
			}

			if regResp.HubTrustBundle != "" {
				if err := os.WriteFile(cfg.TrustBundleFile(), []byte(regResp.HubTrustBundle), 0644); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
				}
			}

			creds := &auth.Credentials{
				OperatorSessionID: regResp.OperatorSessionID,
				UserID:            regResp.UserID,
				OperatorID:        regResp.OperatorID,
				CLISessionID:      regResp.CLISessionID,
			}

			if err := auth.SaveCredentials(cfg, creds); err != nil {
				return err
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
