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

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication and session management",
		Long:  `Manage mTLS enrollment, device-link tokens, and operator sessions.`,
	}

	cmd.AddCommand(
		loginCmd(),
		logoutCmd(),
	)

	return cmd
}

func loginCmd() *cobra.Command {
	var count int
	var ttl int

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and save operator session",
		Long:  `Authenticate and save mTLS credentials to ~/.g8e/credentials. The first login automatically bootstraps the platform.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := auth.CheckOperatorRunning(cfg); err != nil {
				return err
			}

			trustBundle := cfg.TrustBundlePath()
			if _, err := os.Stat(trustBundle); os.IsNotExist(err) {
				return fmt.Errorf("trust bundle not found at %s - start the platform first: ./g8e platform start", trustBundle)
			}

			// Check if platform is already bootstrapped
			bootstrapped, err := auth.CheckBootstrapStatus(cfg)
			if err != nil {
				return fmt.Errorf("failed to check bootstrap status: %w", err)
			}

			if !bootstrapped {
				// First login - perform bootstrap
				cmd.Println("Platform not bootstrapped. Performing first-time bootstrap...")

				cmd.Println("Generating keys and CSRs...")
				hostname, _ := os.Hostname()
				opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
				if err != nil {
					return fmt.Errorf("failed to generate operator CSR: %w", err)
				}

				cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
				if err != nil {
					return fmt.Errorf("failed to generate CLI CSR: %w", err)
				}

				cmd.Println("Bootstrapping with operator...")
				regResp, err := auth.Bootstrap(cfg, opCSR, cliCSR)
				if err != nil {
					return err
				}

				if regResp.OperatorSessionID == "" || regResp.OperatorID == "" || regResp.OperatorCert == "" || regResp.CLISessionID == "" || regResp.CLICert == "" {
					return fmt.Errorf("unexpected bootstrap response (missing required fields)")
				}

				if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
					return fmt.Errorf("failed to save CLI credentials: %w", err)
				}

				if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, opKey, cfg.OperatorCertFile(), cfg.OperatorKeyFile()); err != nil {
					return fmt.Errorf("failed to save operator credentials: %w", err)
				}

				if regResp.HubTrustBundle != "" {
					hubBundlePath := filepath.Join(cfg.CredentialsDir, "hub-bundle.pem")
					if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0600); err != nil {
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

				cmd.Printf("\nBootstrap complete\n")
				cmd.Printf("User ID: %s\n", regResp.UserID)
				cmd.Printf("Operator Session ID: %s\n", regResp.OperatorSessionID)
				cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)
				cmd.Printf("Operator ID: %s\n", regResp.OperatorID)

				return nil
			}

			// Normal device-link flow for subsequent logins
			userID := uuid.New().String()
			cmd.Printf("Requesting device-link token for user %s...\n", userID)
			dlResp, err := auth.RequestDeviceLink(cfg, userID, count, ttl)
			if err != nil {
				return err
			}
			ttlStr := fmt.Sprintf("%dh", ttl/3600)
			cmd.Printf("Device-link token obtained: %s (count=%d, ttl=%s)\n", dlResp.Token, count, ttlStr)

			externalIP := config.GetExternalInterfaceIP()
			bootstrapPort := cfg.OperatorBootstrapHTTPSPort()
			bootstrapURL := fmt.Sprintf("http://%s:%d", externalIP, bootstrapPort)
			cmd.Println("\nTo launch the Operator on a remote system:")
			cmd.Printf("  curl -fsSL %s/g8e | sh -s -- %s\n", bootstrapURL, dlResp.Token)
			cmd.Printf("  G8E_TOKEN=%s curl -fsSL %s/g8e | sh\n", dlResp.Token, bootstrapURL)
			cmd.Println()

			cmd.Println("Generating keys and CSRs...")
			hostname, _ := os.Hostname()
			opCSR, opKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-operator-%s", hostname))
			if err != nil {
				return fmt.Errorf("failed to generate operator CSR: %w", err)
			}

			cliCSR, cliKey, err := auth.GenerateCSR(fmt.Sprintf("g8e-cli-%s", hostname))
			if err != nil {
				return fmt.Errorf("failed to generate CLI CSR: %w", err)
			}

			cmd.Println("Registering with operator...")
			regResp, err := auth.RegisterDeviceLink(cfg, dlResp.Token, opCSR, cliCSR)
			if err != nil {
				return err
			}

			if regResp.OperatorSessionID == "" || regResp.OperatorID == "" || regResp.OperatorCert == "" || regResp.CLISessionID == "" || regResp.CLICert == "" {
				return fmt.Errorf("unexpected registration response (missing required fields)")
			}

			if err := auth.SaveCertAndKey(regResp.CLICert, regResp.CLICertChain, cliKey, cfg.CLICertFile(), cfg.CLIKeyFile()); err != nil {
				return fmt.Errorf("failed to save CLI credentials: %w", err)
			}

			if err := auth.SaveCertAndKey(regResp.OperatorCert, regResp.OperatorCertChain, opKey, cfg.OperatorCertFile(), cfg.OperatorKeyFile()); err != nil {
				return fmt.Errorf("failed to save operator credentials: %w", err)
			}

			if regResp.HubTrustBundle != "" {
				hubBundlePath := filepath.Join(cfg.CredentialsDir, "hub-bundle.pem")
				if err := os.WriteFile(hubBundlePath, []byte(regResp.HubTrustBundle), 0600); err != nil {
					return fmt.Errorf("failed to save hub trust bundle: %w", err)
				}
			}

			creds := &auth.Credentials{
				OperatorSessionID: regResp.OperatorSessionID,
				UserID:            dlResp.UserID,
				OperatorID:        regResp.OperatorID,
				CLISessionID:      regResp.CLISessionID,
			}

			if err := auth.SaveCredentials(cfg, creds); err != nil {
				return fmt.Errorf("failed to save credentials: %w", err)
			}

			cmd.Printf("\nAuthenticated as user %s\n", userID)
			cmd.Printf("Operator Session ID: %s\n", regResp.OperatorSessionID)
			cmd.Printf("CLI Session ID: %s\n", regResp.CLISessionID)
			cmd.Printf("Operator ID: %s\n", regResp.OperatorID)

			return nil
		},
	}

	cmd.Flags().IntVar(&count, "count", 1, "Number of device-link uses")
	cmd.Flags().IntVar(&ttl, "ttl", 3600, "Device-link token TTL in seconds")

	return cmd
}

func logoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear local operator session and credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			creds, err := auth.LoadCredentials(cfg)
			if err != nil {
				return fmt.Errorf("failed to load credentials: %w", err)
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
