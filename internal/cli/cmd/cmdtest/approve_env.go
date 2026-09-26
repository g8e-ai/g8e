// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// GenerateApproveTestCertDER builds a short-lived client certificate for tests.
func GenerateApproveTestCertDER(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	require.NoError(t, err)
	return certBytes
}

// SetupApproveAPITestEnv creates a runtime tree with a key, cert, and credentials.
func SetupApproveAPITestEnv(t *testing.T) (*config.Config, ed25519.PrivateKey, fs.RuntimeFileService) {
	t.Helper()
	fileSvc, cfg := NewCmdTestEnv(t)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), MustRel(t, fileSvc, cfg.CLIKeyFile()), keyPEM, constants.PermFilePrivate))

	certDER := GenerateApproveTestCertDER(t, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), MustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-test",
		UserID:            "user-test",
		OperatorID:        "op-test",
		CLISessionID:      "cli-sess-test",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

	return cfg, priv, fileSvc
}

// SetupApproveSSETestEnv extends SetupApproveAPITestEnv with a trust bundle so
// auth.BuildMTLSClient can succeed.
func SetupApproveSSETestEnv(t *testing.T) (*config.Config, ed25519.PrivateKey, fs.RuntimeFileService) {
	t.Helper()
	cfg, priv, fileSvc := SetupApproveAPITestEnv(t)

	certPEM, err := fileSvc.ReadFile(context.Background(), MustRel(t, fileSvc, cfg.CLICertFile()))
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), certPEM, constants.PermFilePrivate))

	return cfg, priv, fileSvc
}

// WithEndpointOverride sets the config endpoint override for the rest of t.
func WithEndpointOverride(t *testing.T, url string) {
	t.Helper()
	config.SetEndpointOverride(url)
	t.Cleanup(func() { config.SetEndpointOverride("") })
}
