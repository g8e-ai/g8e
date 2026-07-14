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
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
		logoutCmd(),
		approveCmd(),
	)

	return cmd
}

func enrollCmd() *cobra.Command {
	return enrollCmdWithConfig(loadConfig)
}

func enrollCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	var useTPM bool

	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll CLI session with the running Gateway and register a passkey",
		Long: `Enroll a CLI session with the running Gateway via CSR-based enrollment, then register a passkey for secure authentication. Generates client keypairs, submits CSRs to the Gateway's CA, saves signed mTLS credentials, and opens a browser to register a WebAuthn/FIDO2 passkey for web session authentication.

On Windows, the CLI key is generated in the Windows Certificate Store and the signed certificate is imported for Windows Hello native API access. Use --tpm for TPM-backed keys via Windows Hello for Business.

The Gateway must already be running (use './g8e gw start' first).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			fileSvc, err := newFileSvc()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			return performEnroll(cmd, fileSvc, cfg, useTPM)
		},
	}

	if runtime.GOOS == "windows" {
		cmd.Flags().BoolVar(&useTPM, "tpm", false, "Use TPM-backed key via Windows Hello for Business")
	}

	return cmd
}

// performEnroll handles CLI session enrollment and browser-based passkey registration.
// On Windows, it uses the Windows Certificate Store for key generation and imports
// the signed cert for Windows Hello native API access.
func performEnroll(cmd *cobra.Command, fileSvc fs.RuntimeFileService, cfg *config.Config, useTPM bool) error {
	ctx := context.Background()

	// Check if local credentials exist
	credsExist, err := fileSvc.FileExists(ctx, relFromAbs(fileSvc, cfg.CredentialsFile()))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	certExists, err := fileSvc.FileExists(ctx, relFromAbs(fileSvc, cfg.CLICertFile()))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	hasLocalCreds := credsExist && certExists

	// No local credentials: first-time bootstrap or new CLI on an existing gateway.
	if !hasLocalCreds {
		cmd.Println("Performing CLI session enrollment...")
		if err := auth.EnrollCLI(fileSvc, cfg, useTPM); err != nil {
			return err
		}
		creds, err := auth.LoadCredentials(fileSvc, cfg)
		if err != nil || creds == nil {
			return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
		}
		cmd.Printf("\nCLI session enrollment complete\n")
		cmd.Printf("User ID: %s\n", creds.UserID)
		cmd.Printf("CLI Session ID: %s\n", creds.CLISessionID)

		// Register passkey for the newly enrolled user via browser (web session)
		cmd.Println("\nRegistering passkey via browser...")
		if err := auth.RegisterPasskeyViaBrowser(fileSvc, cfg, creds.UserID, creds.CLISessionID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
		}
		return nil
	}

	// Local credentials present — attempt CSR-based re-enrollment with mTLS.
	cmd.Println("Gateway already bootstrapped. Attempting re-enrollment via CSR with mTLS...")

	// Check if operator certificate exists (CLI-only bootstrap has no operator)
	opCertExists, err := fileSvc.FileExists(ctx, relFromAbs(fileSvc, cfg.OperatorCertFile()))
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	hasOperatorCert := opCertExists

	// Check if certificates are expiring soon and auto-renew if needed
	cmd.Println("Checking certificate expiry...")
	if err := auth.AutoRenewCertificate(fileSvc, cfg, "cli", ""); err != nil {
		return err
	}

	cmd.Println("Generating keys and CSRs...")
	hostname, _ := os.Hostname()
	var cliCSR string
	var cliKey *ecdsa.PrivateKey
	if runtime.GOOS == "windows" {
		cliCSR, cliKey, err = auth.GenerateWindowsCSR(fmt.Sprintf("g8e-cli-%s", hostname), useTPM)
	} else {
		cliCSR, cliKey, err = auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
	}
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	var regResp *auth.RegistrationResponse
	if hasOperatorCert {
		// Full re-enrollment with operator CSR
		cmd.Println("Re-enrolling with operator...")
		regResp, err = auth.ReEnroll(fileSvc, cfg, "", cliCSR, "", "")
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

	if err := auth.SaveCertAndKey(fileSvc, regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if runtime.GOOS == "windows" {
		if importErr := auth.ImportCertificateToWindowsStore(regResp.CLICert); importErr != nil {
			cmd.Printf("Warning: failed to import CLI cert to Windows Certificate Store: %v\n", importErr)
		}
	}

	if regResp.HubTrustBundle != "" {
		trustRel := relFromAbs(fileSvc, cfg.TrustBundlePath())
		if err := fileSvc.WriteFile(ctx, trustRel, []byte(regResp.HubTrustBundle), constants.PermFilePublic); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
		}
	}

	creds := &auth.Credentials{
		OperatorSessionID: regResp.OperatorSessionID,
		UserID:            regResp.UserID,
		OperatorID:        regResp.OperatorID,
		CLISessionID:      regResp.CLISessionID,
	}

	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return err
	}

	cmd.Printf("\nCLI session re-enrollment complete\n")
	cmd.Printf("User ID: %s\n", regResp.UserID)
	cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)

	cmd.Println("\nRegistering passkey via browser...")
	if err := auth.RegisterPasskeyViaBrowser(fileSvc, cfg, creds.UserID, creds.CLISessionID); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
	}
	return nil
}

func logoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear local Operator session and credentials",
		Long:  `Clear the local Operator session by deleting stored credentials from disk. This does not revoke the session on the gateway side — it only removes the local credential files so the CLI can no longer authenticate.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return err
			}

			fileSvc, err := newFileSvc()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
			}

			if creds == nil {
				cmd.Println("No active session found")
				return nil
			}

			if err := auth.DeleteCredentials(fileSvc, cfg); err != nil {
				return err
			}

			cmd.Println("Logged out successfully")
			return nil
		},
	}
	return cmd
}
