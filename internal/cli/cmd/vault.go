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
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/vault"
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

func resolveRuntimePath(fileSvc fs.RuntimeFileService, path, defaultRel string) (string, error) {
	if path == "" {
		return defaultRel, nil
	}
	if filepath.IsAbs(path) {
		rel, err := fileSvc.Rel(path)
		if err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
		}
		return rel, nil
	}
	return path, nil
}

func readKeyFile(fileSvc fs.RuntimeFileService, relKeyPath string) ([]byte, error) {
	data, err := fileSvc.ReadFile(context.Background(), relKeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrVaultKeyReadFailed, err)
	}
	keyHex := strings.TrimSpace(string(data))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrVaultKeyDecodeFailed, err)
	}
	if len(key) != vault.KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", constants.ErrVaultKeyInvalidSize, vault.KeySize, len(key))
	}
	return key, nil
}

func vaultInitCmd() *cobra.Command {
	return vaultInitCmdWithConfig(newFileSvc)
}

func vaultInitCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var vaultDir string
	var keyPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new encryption vault",
		Long:  `Generate a new encryption vault with a random key. The key is saved to the specified key path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir, err = resolveRuntimePath(fileSvc, vaultDir, constants.VaultDirname)
			if err != nil {
				return err
			}
			keyPath, err = resolveRuntimePath(fileSvc, keyPath, vaultDir+"/"+constants.VaultKeyFilename)
			if err != nil {
				return err
			}

			vaultDirAbs := fileSvc.Resolve(vaultDir)
			if vault.VaultHeaderExists(vaultDirAbs) {
				return fmt.Errorf("%w: %s", constants.ErrVaultAlreadyInitialized, vaultDirAbs)
			}

			privateKey := make([]byte, vault.KeySize)
			if _, err := rand.Read(privateKey); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultKeyGenerateFailed, err)
			}

			header, dek, err := vault.NewVaultHeader(privateKey)
			if err != nil {
				vault.SecureZero(privateKey)
				vault.SecureZero(dek)
				return fmt.Errorf("%w: %w", constants.ErrVaultHeaderCreateFailed, err)
			}
			vault.SecureZero(dek)

			if err := fileSvc.MkdirAll(context.Background(), vaultDir, constants.PermDirPrivate); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
			}

			if err := header.Save(vaultDirAbs); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("%w: %w", constants.ErrVaultHeaderSaveFailed, err)
			}

			if err := fileSvc.WriteFile(context.Background(), keyPath, []byte(hex.EncodeToString(privateKey)+"\n"), constants.PermFilePrivate); err != nil {
				vault.SecureZero(privateKey)
				return fmt.Errorf("%w: %w", constants.ErrVaultKeyWriteFailed, err)
			}

			vault.SecureZero(privateKey)

			cmd.Printf("Vault initialized at %s\n", vaultDirAbs)
			cmd.Printf("Key saved to %s\n", fileSvc.Resolve(keyPath))
			cmd.Println("WARNING: Back up this key securely. If lost, all encrypted data is unrecoverable.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.DefaultVaultDirDesc+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to save the vault key")

	return cmd
}

func vaultUnlockCmd() *cobra.Command {
	return vaultUnlockCmdWithConfig(newFileSvc)
}

func vaultUnlockCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var vaultDir string
	var keyPath string

	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Unlock the encryption vault",
		Long:  `Unlock an existing vault using the private key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir, err = resolveRuntimePath(fileSvc, vaultDir, constants.VaultDirname)
			if err != nil {
				return err
			}
			keyPath, err = resolveRuntimePath(fileSvc, keyPath, vaultDir+"/"+constants.VaultKeyFilename)
			if err != nil {
				return err
			}

			vaultDirAbs := fileSvc.Resolve(vaultDir)
			if !vault.VaultHeaderExists(vaultDirAbs) {
				return fmt.Errorf("%w: %s. Run 'g8e vault init' first", constants.ErrVaultNotInitialized, vaultDirAbs)
			}

			privateKey, err := readKeyFile(fileSvc, keyPath)
			if err != nil {
				return fmt.Errorf("vault unlock: %w", err)
			}
			defer vault.SecureZero(privateKey)

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDirAbs,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
			}

			if err := v.Unlock(privateKey); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultUnlockFailed, err)
			}

			cmd.Println("Vault unlocked successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.DefaultVaultDirDesc+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to the vault key")

	return cmd
}

func vaultRekeyCmd() *cobra.Command {
	return vaultRekeyCmdWithConfig(newFileSvc)
}

func vaultRekeyCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var vaultDir string
	var keyPath string
	var newKeyPath string

	cmd := &cobra.Command{
		Use:   "rekey",
		Short: "Re-key the vault with a new private key",
		Long:  `Re-encrypt the vault's DEK with a new private key. Both old and new keys are required.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir, err = resolveRuntimePath(fileSvc, vaultDir, constants.VaultDirname)
			if err != nil {
				return err
			}
			keyPath, err = resolveRuntimePath(fileSvc, keyPath, vaultDir+"/"+constants.VaultKeyFilename)
			if err != nil {
				return err
			}
			newKeyPath, err = resolveRuntimePath(fileSvc, newKeyPath, vaultDir+"/"+constants.VaultNewKeyFilename)
			if err != nil {
				return err
			}

			vaultDirAbs := fileSvc.Resolve(vaultDir)
			if !vault.VaultHeaderExists(vaultDirAbs) {
				return fmt.Errorf("%w: %s", constants.ErrVaultNotInitialized, vaultDirAbs)
			}

			oldKey, err := readKeyFile(fileSvc, keyPath)
			if err != nil {
				return fmt.Errorf("vault rekey: %w", err)
			}
			defer vault.SecureZero(oldKey)

			newKey := make([]byte, vault.KeySize)
			if _, err := rand.Read(newKey); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultKeyGenerateFailed, err)
			}

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDirAbs,
				Logger:  nil,
			})
			if err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
			}

			if err := v.Rekey(oldKey, newKey); err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("%w: %w", constants.ErrVaultRekeyFailed, err)
			}

			if err := fileSvc.WriteFile(context.Background(), newKeyPath, []byte(hex.EncodeToString(newKey)+"\n"), constants.PermFilePrivate); err != nil {
				vault.SecureZero(newKey)
				return fmt.Errorf("%w: %w", constants.ErrVaultKeyWriteFailed, err)
			}

			vault.SecureZero(newKey)

			cmd.Println("Vault rekeyed successfully")
			cmd.Printf("New key saved to %s\n", fileSvc.Resolve(newKeyPath))
			cmd.Println("WARNING: The old key is no longer valid. Remove it after confirming the new key works.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.DefaultVaultDirDesc+")")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to the current vault key")
	cmd.Flags().StringVar(&newKeyPath, "new-key-path", "", "Path to save the new vault key (default: <key-path>.new)")

	return cmd
}

func vaultStatusCmd() *cobra.Command {
	return vaultStatusCmdWithConfig(newFileSvc)
}

func vaultStatusCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var vaultDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show vault status",
		Long:  `Display whether the vault is initialized and unlocked.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir, err = resolveRuntimePath(fileSvc, vaultDir, constants.VaultDirname)
			if err != nil {
				return err
			}

			vaultDirAbs := fileSvc.Resolve(vaultDir)
			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDirAbs,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
			}

			initialized := v.IsInitialized()
			unlocked := v.IsUnlocked()

			cmd.Printf("Vault directory: %s\n", vaultDirAbs)
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

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.DefaultVaultDirDesc+")")

	return cmd
}

func vaultResetCmd() *cobra.Command {
	return vaultResetCmdWithConfig(newFileSvc)
}

func vaultResetCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var vaultDir string
	var confirm bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Destroy the vault and all encrypted data",
		Long:  `Reset the vault completely. This is a destructive operation that makes all encrypted data unrecoverable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir, err = resolveRuntimePath(fileSvc, vaultDir, constants.VaultDirname)
			if err != nil {
				return err
			}

			vaultDirAbs := fileSvc.Resolve(vaultDir)
			if !vault.VaultHeaderExists(vaultDirAbs) {
				return fmt.Errorf("%w: %s", constants.ErrVaultNotInitialized, vaultDirAbs)
			}

			if !confirm {
				reader := bufio.NewReader(cmd.InOrStdin())
				cmd.Printf("WARNING: This will destroy the vault at %s and all encrypted data will be unrecoverable.\n", vaultDirAbs)
				cmd.Print("Type 'destroy' to confirm: ")
				input, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrVaultStdinReadFailed, err)
				}
				if strings.TrimSpace(input) != "destroy" {
					cmd.Println("Reset cancelled.")
					return nil
				}
			}

			v, err := vault.NewVault(&vault.VaultConfig{
				DataDir: vaultDirAbs,
				Logger:  nil,
			})
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
			}

			if err := v.Reset(true); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrVaultResetFailed, err)
			}

			cmd.Println("Vault reset complete. All encrypted data has been destroyed.")
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultDir, "vault-dir", "", "Vault directory (default: "+constants.DefaultVaultDirDesc+")")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip interactive confirmation (dangerous)")

	return cmd
}

func vaultExportCmd() *cobra.Command {
	return vaultExportCmdWithConfig(newFileSvc)
}

func vaultExportCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var keyPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the vault key",
		Long:  `Export the vault private key in hex format. Use with extreme caution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir := constants.VaultDirname
			keyPath, err = resolveRuntimePath(fileSvc, keyPath, vaultDir+"/"+constants.VaultKeyFilename)
			if err != nil {
				return err
			}

			key, err := readKeyFile(fileSvc, keyPath)
			if err != nil {
				return fmt.Errorf("vault export: %w", err)
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
	return vaultImportCmdWithConfig(newFileSvc)
}

func vaultImportCmdWithConfig(fileSvcFactory func() (fs.RuntimeFileService, error)) *cobra.Command {
	var keyPath string
	var keyHex string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a vault key",
		Long:  `Import a vault private key from hex string or stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			vaultDir := constants.VaultDirname
			keyPath, err = resolveRuntimePath(fileSvc, keyPath, vaultDir+"/"+constants.VaultKeyFilename)
			if err != nil {
				return err
			}

			var key []byte
			if keyHex != "" {
				key, err = hex.DecodeString(strings.TrimSpace(keyHex))
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrVaultKeyDecodeFailed, err)
				}
			} else {
				reader := bufio.NewReader(cmd.InOrStdin())
				cmd.Print("Enter vault key (hex): ")
				input, readErr := reader.ReadString('\n')
				if readErr != nil {
					return fmt.Errorf("%w: %w", constants.ErrVaultStdinReadFailed, readErr)
				}
				key, err = hex.DecodeString(strings.TrimSpace(input))
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrVaultKeyDecodeFailed, err)
				}
			}

			if len(key) != vault.KeySize {
				vault.SecureZero(key)
				return fmt.Errorf("%w: expected %d bytes, got %d", constants.ErrVaultKeyInvalidSize, vault.KeySize, len(key))
			}

			if err := fileSvc.WriteFile(context.Background(), keyPath, []byte(hex.EncodeToString(key)+"\n"), constants.PermFilePrivate); err != nil {
				vault.SecureZero(key)
				return fmt.Errorf("%w: %w", constants.ErrVaultKeyWriteFailed, err)
			}

			vault.SecureZero(key)

			cmd.Printf("Key imported to %s\n", fileSvc.Resolve(keyPath))
			return nil
		},
	}

	cmd.Flags().StringVar(&keyPath, "key-path", "", "Path to save the vault key")
	cmd.Flags().StringVar(&keyHex, "key-hex", "", "Vault key as hex string (if not provided, reads from stdin)")

	return cmd
}
