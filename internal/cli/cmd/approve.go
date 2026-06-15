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
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func approveCmd() *cobra.Command {
	return approveCmdWithConfig(config.Load)
}

func approveCmdWithConfig(configLoader func(string) (*config.Config, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <transaction_hash>",
		Short: "Approve a suspended L3 transaction with CLI signature",
		Long:  `Approve a suspended transaction by signing the transaction hash with the CLI private key and submitting the cryptographic proof to the Gateway.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txHash := args[0]
			cfg, err := configLoader("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Read CLI private key
			keyData, err := os.ReadFile(cfg.CLIKeyFile())
			if err != nil {
				return fmt.Errorf("approve: read CLI private key: %w", err)
			}

			// Parse PEM-encoded private key
			block, rest := pem.Decode(keyData)
			if block == nil {
				return fmt.Errorf("approve: decode PEM private key: %w", constants.ErrPEMDecodeFailed)
			}
			if len(rest) > 0 {
				return fmt.Errorf("approve: extra data after PEM block")
			}

			// Ed25519 keys are encoded in PKCS8 format
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return fmt.Errorf("approve: parse private key: %w", err)
			}

			privKey, ok := key.(ed25519.PrivateKey)
			if !ok {
				return fmt.Errorf("approve: invalid key type: %w", constants.ErrInvalidKeyType)
			}

			// Sign the transaction hash
			signature := ed25519.Sign(privKey, []byte(txHash))
			signatureHex := hex.EncodeToString(signature)

			// Calculate certificate fingerprint for verification
			certData, err := os.ReadFile(cfg.CLICertFile())
			if err != nil {
				return fmt.Errorf("approve: read CLI certificate: %w", err)
			}

			certBlock, rest := pem.Decode(certData)
			if certBlock == nil {
				return fmt.Errorf("approve: decode PEM certificate: %w", constants.ErrPEMDecodeFailed)
			}
			if len(rest) > 0 {
				return fmt.Errorf("approve: extra data after PEM certificate block")
			}

			cert, err := x509.ParseCertificate(certBlock.Bytes)
			if err != nil {
				return fmt.Errorf("approve: parse certificate: %w", err)
			}

			hash := sha256.Sum256(cert.Raw)
			certFingerprint := hex.EncodeToString(hash[:])

			// Create approval request
			type approvalRequest struct {
				CliSignature        string `json:"cli_signature"`
				MtlsCertFingerprint string `json:"mtls_cert_fingerprint"`
			}
			req := approvalRequest{
				CliSignature:        signatureHex,
				MtlsCertFingerprint: certFingerprint,
			}

			reqBody, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("approve: marshal request: %w", err)
			}

			// Call approval API
			client, err := api.NewClient(cfg)
			if err != nil {
				return fmt.Errorf("approve: create API client: %w", err)
			}

			approvePath := constants.APIPaths.ApprovePagePrefix + txHash
			resp, err := client.Post(approvePath, reqBody)
			if err != nil {
				return fmt.Errorf("approve: approve transaction: %w", err)
			}

			type approvalResponse struct {
				Status        string `json:"status"`
				ResultSummary string `json:"result_summary"`
			}
			var result approvalResponse
			if err := json.Unmarshal(resp, &result); err != nil {
				return fmt.Errorf("approve: parse response: %w", err)
			}

			cmd.Printf("Transaction %s approved successfully\n", txHash)
			if result.Status != "" {
				cmd.Printf("Status: %s\n", result.Status)
			}
			if result.ResultSummary != "" {
				cmd.Printf("Result: %s\n", result.ResultSummary)
			}

			return nil
		},
	}

	return cmd
}
