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

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	vault "github.com/g8e-ai/g8e/internal/services/vault"
)

// handleVaultCommand processes vault management CLI commands
func handleVaultCommand(rekeyVault, verifyVault, resetVault bool, newPrivateKeyStr, oldPrivateKeyStr, logLevel, workDir string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	dataDir := paths.Infra.DataDir

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dataDir,
		Logger:  logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create vault: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}
	defer v.Close()

	switch {
	case rekeyVault:
		handleRekeyVault(v, []byte(oldPrivateKeyStr), []byte(newPrivateKeyStr), logger)
	case verifyVault:
		handleVerifyVault(v, []byte(newPrivateKeyStr), logger)
	case resetVault:
		handleResetVault(v, logger)
	}
}

// handleRekeyVault re-encrypts the vault DEK with a new private key
func handleRekeyVault(v *vault.Vault, oldPrivateKey, newPrivateKey []byte, logger *slog.Logger) {
	if len(oldPrivateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: --old-key is required for --rekey-vault\n")
		fmt.Fprintf(os.Stderr, "Usage: g8e --rekey-vault --old-key <old-key> -k <new-key>\n")
		os.Exit(constants.ExitConfigError)
	}

	if len(newPrivateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: New private key is required (-k)\n")
		os.Exit(constants.ExitConfigError)
	}

	if !v.IsInitialized() {
		fmt.Fprintf(os.Stderr, "Error: No vault found. Nothing to rekey.\n")
		os.Exit(constants.ExitConfigError)
	}

	logger.Info("Re-keying vault")

	if err := v.Rekey(oldPrivateKey, newPrivateKey); err != nil {
		logger.Error("Failed to rekey vault", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault successfully rekeyed")
	os.Exit(constants.ExitSuccess)
}

// handleVerifyVault checks vault integrity
func handleVerifyVault(v *vault.Vault, privateKey []byte, logger *slog.Logger) {
	if len(privateKey) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Private key is required for vault verification\n")
		os.Exit(constants.ExitConfigError)
	}

	if !v.IsInitialized() {
		logger.Info("Vault not initialized")
		os.Exit(constants.ExitSuccess)
	}

	logger.Info("Verifying vault integrity")

	if err := v.VerifyIntegrity(privateKey); err != nil {
		logger.Error("Vault verification failed", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault verification passed")
	os.Exit(constants.ExitSuccess)
}

// handleResetVault destroys the vault (requires confirmation)
func handleResetVault(v *vault.Vault, logger *slog.Logger) {
	if !v.IsInitialized() {
		logger.Info("No vault found, nothing to reset")
		os.Exit(constants.ExitSuccess)
	}

	fmt.Fprint(os.Stderr, "WARNING: This will PERMANENTLY DESTROY all encrypted vault data. Type 'DESTROY' to confirm: ")

	var confirmation string
	_, _ = fmt.Fscan(os.Stdin, &confirmation)

	if confirmation != "DESTROY" {
		logger.Info("Reset cancelled")
		os.Exit(constants.ExitSuccess)
	}

	if err := v.Reset(true); err != nil {
		logger.Error("Failed to reset vault", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitGeneralError)
	}

	logger.Info("Vault has been reset, all encrypted data has been destroyed")
	os.Exit(constants.ExitSuccess)
}
