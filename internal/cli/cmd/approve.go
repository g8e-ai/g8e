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
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func approveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <transaction_hash>",
		Short: "Approve a suspended L3 transaction with CLI signature",
		Long:  `Approve a suspended transaction by signing the transaction hash with the CLI private key and submitting the cryptographic proof to the Gateway.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txHash := args[0]
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Read CLI private key
			keyData, err := os.ReadFile(cfg.CLIKeyFile())
			if err != nil {
				return fmt.Errorf("failed to read CLI private key: %w", err)
			}

			// Parse PEM-encoded private key
			block, _ := pem.Decode(keyData)
			if block == nil {
				return fmt.Errorf("failed to decode PEM private key")
			}

			// Ed25519 keys are encoded in PKCS8 format
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("failed to parse private key: %w", err)
			}

			privKey, ok := key.(ed25519.PrivateKey)
			if !ok {
				return fmt.Errorf("private key is not Ed25519")
			}

			// Sign the transaction hash
			signature := ed25519.Sign(privKey, []byte(txHash))
			signatureHex := hex.EncodeToString(signature)

			// Calculate certificate fingerprint for verification
			certData, err := os.ReadFile(cfg.CLICertFile())
			if err != nil {
				return fmt.Errorf("failed to read CLI certificate: %w", err)
			}

			certBlock, _ := pem.Decode(certData)
			if certBlock == nil {
				return fmt.Errorf("failed to decode PEM certificate")
			}

			cert, err := x509.ParseCertificate(certBlock.Bytes)
			if err != nil {
				return fmt.Errorf("failed to parse certificate: %w", err)
			}

			hash := sha256.Sum256(cert.Raw)
			certFingerprint := hex.EncodeToString(hash[:])

			// Create approval request
			req := map[string]string{
				"cli_signature":         signatureHex,
				"mtls_cert_fingerprint": certFingerprint,
			}

			reqBody, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to marshal request: %w", err)
			}

			// Call approval API
			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Post(fmt.Sprintf("/api/approve/%s", txHash), reqBody)
			if err != nil {
				return fmt.Errorf("failed to approve transaction: %w", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(resp, &result); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			cmd.Printf("Transaction %s approved successfully\n", txHash)
			if status, ok := result["status"].(string); ok {
				cmd.Printf("Status: %s\n", status)
			}
			if summary, ok := result["result_summary"].(string); ok {
				cmd.Printf("Result: %s\n", summary)
			}

			return nil
		},
	}

	return cmd
}
