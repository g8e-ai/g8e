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
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/spf13/cobra"
)

func vaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "Manage the encryption vault",
		Long:  `Initialize, unlock, re-key, and manage the g8e encryption vault.`,
	}

	cmd.AddCommand(
		vaultInitCmd(),
		vaultUnlockCmd(),
		vaultRekeyCmd(),
		vaultStatusCmd(),
		vaultResetCmd(),
		vaultExportCmd(),
		vaultImportCmd(),
	)

	return cmd
}

func readKeyFile(keyPath string) ([]byte, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	keyHex := strings.TrimSpace(string(data))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	if len(key) != vault.KeySize {
		return nil, fmt.Errorf("invalid key size: expected %d bytes, got %d", vault.KeySize, len(key))
	}
	return key, nil
}

func vaultInitCmd() *cobra.Command {
	var vaultDir string
	var keyPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new encryption vault",
		Long:  `Generate a new encryption vault with a random key. The key is saved to the specified key path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			if vaultDir == "" {
				vaultDir = constants.Paths.Infra.VaultDir
			}
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if keyPath == "" {
				keyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(projectRoot, keyPath)
			}

			if vault.VaultHeaderExists(vaultDir) {
				return fmt.Errorf("vault already initialized at %s", vaultDir)
			}

			privateKey := make([]byte, vault.KeySize)
			if _, err := rand.Read(privateKey); err != nil {
				return fmt.Errorf("failed to generate vault key: %w", err)
			}

			header, dek, err := vault.NewVaultHeader(privateKey)
			if err != nil {
				vault.SecureZero(privateKey)
				vault.SecureZero(dek)
				return fmt.Errorf("failed to create vault header: %w", err)
			}
			vault.SecureZero(dek)

			if err := os.MkdirAll(vaultDir, 0700); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("failed to create vault directory: %w", err)
			}

			if err := header.Save(vaultDir); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("failed to save vault header: %w", err)
			}

			keyDir := filepath.Dir(keyPath)
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("failed to create key directory: %w", err)
			}

			if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(privateKey)+"\n"), 0600); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("failed to write vault key: %w", err)
			}

			vault.SecureZero(privateKey)

			cmd.Printf("Vault initialized at %s\n", vaultDir)
			cmd.Printf("Key saved to %s\n", keyPath)
			cmd.Println("WARNING: Back up this key securely. If lost, all encrypted data is unrecoverable.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.Paths.Infra.VaultDir+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to save the vault key")

	return cmd
}

func vaultUnlockCmd() *cobra.Command {
	var vaultDir string
	var keyPath string

	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Unlock the encryption vault",
		Long:  `Unlock an existing vault using the private key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			if vaultDir == "" {
				vaultDir = constants.Paths.Infra.VaultDir
			}
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if keyPath == "" {
				keyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(projectRoot, keyPath)
			}

			if !vault.VaultHeaderExists(vaultDir) {
				return fmt.Errorf("vault not initialized at %s. Run 'g8e vault init' first", vaultDir)
			}

			privateKey, err := readKeyFile(keyPath)
			if err != nil {
				return err
			}
			defer vault.SecureZero(privateKey)

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("failed to create vault: %w", err)
			}

			if err := v.Unlock(privateKey); err != nil {
				return fmt.Errorf("failed to unlock vault: %w", err)
			}

			cmd.Println("Vault unlocked successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.Paths.Infra.VaultDir+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to the vault key")

	return cmd
}

func vaultRekeyCmd() *cobra.Command {
	var vaultDir string
	var keyPath string
	var newKeyPath string

	cmd := &cobra.Command{
		Use:   "rekey",
		Short: "Re-key the vault with a new private key",
		Long:  `Re-encrypt the vault's DEK with a new private key. Both old and new keys are required.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			if vaultDir == "" {
				vaultDir = constants.Paths.Infra.VaultDir
			}
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if keyPath == "" {
				keyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(projectRoot, keyPath)
			}
			if newKeyPath == "" {
				newKeyPath = keyPath + ".new"
			}

			if !vault.VaultHeaderExists(vaultDir) {
				return fmt.Errorf("vault not initialized at %s", vaultDir)
			}

			oldKey, err := readKeyFile(keyPath)
			if err != nil {
				return err
			}
			defer vault.SecureZero(oldKey)

			newKey := make([]byte, vault.KeySize)
			if _, err := rand.Read(newKey); err != nil {
				return fmt.Errorf("failed to generate new key: %w", err)
			}

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  nil,
			})
			if err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("failed to create vault: %w", err)
			}

			if err := v.Rekey(oldKey, newKey); err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("failed to rekey vault: %w", err)
			}

			if err := os.WriteFile(newKeyPath, []byte(hex.EncodeToString(newKey)+"\n"), 0600); err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("failed to write new key: %w", err)
			}

			vault.SecureZero(newKey)

			cmd.Println("Vault rekeyed successfully")
			cmd.Printf("New key saved to %s\n", newKeyPath)
			cmd.Println("WARNING: The old key is no longer valid. Remove it after confirming the new key works.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.Paths.Infra.VaultDir+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to the current vault key")
	cmd.Flags().StringVar(&newKeyPath, "new-key-path", "", "Path to save the new vault key (default: <key-path>.new)")

	return cmd
}

func vaultStatusCmd() *cobra.Command {
	var vaultDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show vault status",
		Long:  `Display whether the vault is initialized and unlocked.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			if vaultDir == "" {
				vaultDir = constants.Paths.Infra.VaultDir
			}
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("failed to create vault: %w", err)
			}

			initialized := v.IsInitialized()
			unlocked := v.IsUnlocked()

			cmd.Printf("Vault directory: %s\n", vaultDir)
			if initialized {
				cmd.Println("Status: initialized")
			} else {
				cmd.Println("Status: not initialized")
			}
			if unlocked {
				cmd.Println("Lock state: unlocked")
			} else {
				cmd.Println("Lock state: locked")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.Paths.Infra.VaultDir+")")

	return cmd
}

func vaultResetCmd() *cobra.Command {
	var vaultDir string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Destroy the vault and all encrypted data",
		Long:  `Reset the vault completely. This is a destructive operation that makes all encrypted data unrecoverable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			if vaultDir == "" {
				vaultDir = constants.Paths.Infra.VaultDir
			}
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if !vault.VaultHeaderExists(vaultDir) {
				return fmt.Errorf("vault not initialized at %s", vaultDir)
			}

			if !confirm {
				reader := bufio.NewReader(cmd.InOrStdin())
				cmd.Printf("WARNING: This will destroy the vault at %s and all encrypted data will be unrecoverable.\n", vaultDir)
				cmd.Print("Type 'destroy' to confirm: ")
				input, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if strings.TrimSpace(input) != "destroy" {
					cmd.Println("Reset cancelled.")
					return nil
				}
			}

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDir,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("failed to create vault: %w", err)
			}

			if err := v.Reset(true); err != nil {
				return fmt.Errorf("failed to reset vault: %w", err)
			}

			cmd.Println("Vault reset complete. All encrypted data has been destroyed.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.Paths.Infra.VaultDir+")")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip interactive confirmation (dangerous)")

	return cmd
}

func vaultExportCmd() *cobra.Command {
	var keyPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the vault key",
		Long:  `Export the vault private key in hex format. Use with extreme caution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			vaultDir := constants.Paths.Infra.VaultDir
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if keyPath == "" {
				keyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(projectRoot, keyPath)
			}

			key, err := readKeyFile(keyPath)
			if err != nil {
				return err
			}
			defer vault.SecureZero(key)

			cmd.Println(hex.EncodeToString(key))
			return nil
		},
	}

	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to the vault key")

	return cmd
}

func vaultImportCmd() *cobra.Command {
	var keyPath string
	var keyHex string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a vault key",
		Long:  `Import a vault private key from hex string or stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize paths relative to current working directory
			if err := constants.InitPaths(); err != nil {
				return fmt.Errorf("failed to initialize paths: %w", err)
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			vaultDir := constants.Paths.Infra.VaultDir
			if !filepath.IsAbs(vaultDir) {
				vaultDir = filepath.Join(projectRoot, vaultDir)
			}

			if keyPath == "" {
				keyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
			}
			if !filepath.IsAbs(keyPath) {
				keyPath = filepath.Join(projectRoot, keyPath)
			}

			var key []byte
			if keyHex != "" {
				key, err = hex.DecodeString(strings.TrimSpace(keyHex))
				if err != nil {
					return fmt.Errorf("failed to decode key: %w", err)
				}
			} else {
				reader := bufio.NewReader(cmd.InOrStdin())
				cmd.Print("Enter vault key (hex): ")
				input, readErr := reader.ReadString('\n')
				if readErr != nil {
					return fmt.Errorf("failed to read key: %w", readErr)
				}
				key, err = hex.DecodeString(strings.TrimSpace(input))
				if err != nil {
					return fmt.Errorf("failed to decode key: %w", err)
				}
			}

			if len(key) != vault.KeySize {
				vault.SecureZero(key)
				return fmt.Errorf("invalid key size: expected %d bytes, got %d", vault.KeySize, len(key))
			}

			keyDir := filepath.Dir(keyPath)
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				vault.SecureZero(key)
				return fmt.Errorf("failed to create key directory: %w", err)
			}

			if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
				vault.SecureZero(key)
				return fmt.Errorf("failed to write key: %w", err)
			}

			vault.SecureZero(key)

			cmd.Printf("Key imported to %s\n", keyPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to save the vault key")
	cmd.Flags().StringVar(&keyHex, "key-hex", "", "Vault key as hex string (if not provided, reads from stdin)")

	return cmd
}
