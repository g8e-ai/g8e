//go:build integration

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
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApproveCmd(t *testing.T) {
	t.Run("approve fails with no arguments", func(t *testing.T) {
		cmd := approveCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// The cobra framework will validate args before RunE is called
		// So we test the validation directly
		err := cmd.Args(cmd, []string{})
		require.Error(t, err)
	})

	t.Run("approve fails when not authenticated", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		// Use injectable config loader for hermetic test
		cmd := approveCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		}, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Create valid Ed25519 key and certificate
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)

		privPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), privPEM, constants.PermFilePrivate))

		cert := generateTestCertificate(t, priv)
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))

		// Don't create credentials - should fail on API client creation
		err = cmd.RunE(cmd, []string{"abc123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	})

	t.Run("approve fails with missing CLI key file", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		cmd := approveCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		}, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Save credentials so LoadCredentials passes, but don't create key/cert files
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

		err := cmd.RunE(cmd, []string{"abc123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
	})

	t.Run("approve fails with missing CLI cert file", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		cmd := approveCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		}, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Save credentials and write valid key, but no cert
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)

		privPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), privPEM, constants.PermFilePrivate))

		err = cmd.RunE(cmd, []string{"abc123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
	})

	t.Run("approve fails with invalid key/cert pair", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		cmd := approveCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		}, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Save credentials and write key/cert that don't match
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

		_, priv1, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		privBytes, err := x509.MarshalPKCS8PrivateKey(priv1)
		require.NoError(t, err)
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), privPEM, constants.PermFilePrivate))

		_, priv2, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		cert := generateTestCertificate(t, priv2)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))

		err = cmd.RunE(cmd, []string{"abc123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
	})

	t.Run("approve fails with missing trust bundle", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		// Remove the trust bundle that setupTestConfig creates
		require.NoError(t, fileSvc.Remove(context.Background(), mustRel(t, fileSvc, cfg.TrustBundlePath())))

		// Use injectable config loader for hermetic test
		cmd := approveCmdWithConfig(func(_ string) (*config.Config, error) {
			return cfg, nil
		}, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Create valid Ed25519 key and certificate
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)

		privPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privBytes,
		})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), privPEM, constants.PermFilePrivate))

		cert := generateTestCertificate(t, priv)
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		})
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))

		// Create credentials but don't create trust bundle
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

		err = cmd.RunE(cmd, []string{"abc123"})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
	})
}

// generateTestCertificate creates a test X.509 certificate for testing purposes
func generateTestCertificate(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   "test-cert",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Generate certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	require.NoError(t, err)

	return certBytes
}
