// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errMockEnroll = errors.New("enrollment server error")

// mockRemoteOperatorEnroller is a test double for remoteOperatorEnroller.
type mockRemoteOperatorEnroller struct {
	artifacts auth.EnrollmentArtifacts
	err       error
	called    bool
}

func (m *mockRemoteOperatorEnroller) EnrollRemoteOperator(_ context.Context, _, _ string, _ *ecdsa.PrivateKey, _ string, _ *ecdsa.PrivateKey, _ string) (auth.EnrollmentArtifacts, error) {
	m.called = true
	return m.artifacts, m.err
}

// enrollCmdWithRoot wraps the enroll command under a root command that defines
// the persistent --endpoint flag, matching the real CLI structure so that
// cmd.Flags().GetString("endpoint") can find the inherited flag in tests.
func enrollCmdWithRoot(configLoader func(string) (*config.Config, error), enroller remoteOperatorEnroller) *cobra.Command {
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint (host or host:port)")
	enrollCmd := securityPKIEnrollCmdWithConfig(configLoader, func(*config.Config) remoteOperatorEnroller { return enroller }, newFileSvc)
	root.AddCommand(enrollCmd)
	// Trigger cobra's persistent flag merging so cmd.Flags() can see the inherited --endpoint flag
	_ = enrollCmd.ParseFlags([]string{})
	return enrollCmd
}

// findSubCmd traverses a command tree by subcommand names, returning the leaf
// command or nil if any name in the chain is not found.
func findSubCmd(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}

// generateTestPEMCert creates a self-signed PEM certificate for testing.
func generateTestPEMCert(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-root"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return pemBytes
}

// --- PKI Enroll ---

func TestSecurityPKIEnrollCmd_API_MockHappyPath(t *testing.T) {
	_, cfg := setupTestConfig(t, testutil.TempDir(t))
	tmpDir := testutil.TempDir(t)

	enroller := &mockRemoteOperatorEnroller{
		artifacts: auth.EnrollmentArtifacts{
			Source:               auth.EnrollmentSourceRemoteOperator,
			OperatorID:           "op-test-123",
			OperatorSessionID:    "sess-test-456",
			OperatorCertPEM:      "-----BEGIN CERTIFICATE-----\nMIIBdummy==\n-----END CERTIFICATE-----",
			OperatorCertChainPEM: "-----BEGIN CERTIFICATE-----\nMIIBchain==\n-----END CERTIFICATE-----",
		},
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := enrollCmdWithRoot(loader, enroller)
	cmd.Flags().Set("endpoint", "127.0.0.1")
	cmd.Flags().Set("output-dir", tmpDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Enrollment complete")
	assert.Contains(t, buf.String(), "op-test-123")
	assert.Contains(t, buf.String(), "sess-test-456")
}

func TestSecurityPKIEnrollCmd_API_MockHappyPathWithTrustBundle(t *testing.T) {
	_, cfg := setupTestConfig(t, testutil.TempDir(t))
	tmpDir := testutil.TempDir(t)

	enroller := &mockRemoteOperatorEnroller{
		artifacts: auth.EnrollmentArtifacts{
			Source:               auth.EnrollmentSourceRemoteOperator,
			OperatorID:           "op-trust",
			OperatorSessionID:    "sess-trust",
			OperatorCertPEM:      "-----BEGIN CERTIFICATE-----\nMIIBdummy==\n-----END CERTIFICATE-----",
			OperatorCertChainPEM: "-----BEGIN CERTIFICATE-----\nMIIBchain==\n-----END CERTIFICATE-----",
			TrustBundlePEM:       "-----BEGIN CERTIFICATE-----\nMIIBtrust==\n-----END CERTIFICATE-----",
		},
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := enrollCmdWithRoot(loader, enroller)
	cmd.Flags().Set("endpoint", "192.168.1.50")
	cmd.Flags().Set("output-dir", tmpDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Trust bundle saved")
}

func TestSecurityPKIEnrollCmd_API_MissingEndpoint(t *testing.T) {
	_, cfg := setupTestConfig(t, testutil.TempDir(t))

	loader := func(string) (*config.Config, error) { return cfg, nil }
	enroller := &mockRemoteOperatorEnroller{
		err: fmt.Errorf("enroll should not be called when endpoint is missing"),
	}

	cmd := enrollCmdWithRoot(loader, enroller)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEndpointRequired)
}

func TestSecurityPKIEnrollCmd_API_ConfigLoadError(t *testing.T) {
	loader := func(string) (*config.Config, error) {
		return nil, constants.ErrConfigLoadFailed
	}
	enroller := &mockRemoteOperatorEnroller{
		err: fmt.Errorf("enroll should not be called on config load failure"),
	}

	cmd := enrollCmdWithRoot(loader, enroller)
	cmd.Flags().Set("endpoint", "127.0.0.1")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
}

func TestSecurityPKIEnrollCmd_API_EnrollError(t *testing.T) {
	_, cfg := setupTestConfig(t, testutil.TempDir(t))

	enroller := &mockRemoteOperatorEnroller{err: errMockEnroll}

	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := enrollCmdWithRoot(loader, enroller)
	cmd.Flags().Set("endpoint", "127.0.0.1")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockEnroll)
}

func TestSecurityPKIEnrollCmd_API_MissingCertInResponse(t *testing.T) {
	_, cfg := setupTestConfig(t, testutil.TempDir(t))

	enroller := &mockRemoteOperatorEnroller{
		artifacts: auth.EnrollmentArtifacts{
			Source:          auth.EnrollmentSourceRemoteOperator,
			OperatorID:      "op-nocert",
			OperatorCertPEM: "",
		},
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }

	cmd := enrollCmdWithRoot(loader, enroller)
	cmd.Flags().Set("endpoint", "127.0.0.1")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingCertificate)
}

func TestSecurityPKIEnrollCmd_API_PKICommandStructure(t *testing.T) {
	t.Run("pki command has correct use", func(t *testing.T) {
		cmd := securityPKICmd()
		assert.Equal(t, "pki", cmd.Use)
	})

	t.Run("enroll command has endpoint and output-dir flags", func(t *testing.T) {
		root := &cobra.Command{Use: "g8e"}
		root.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint (host or host:port)")
		root.AddCommand(securityCmd())
		enrollCmd := findSubCmd(root, "security", "pki", "enroll")
		require.NotNil(t, enrollCmd)
		_ = enrollCmd.ParseFlags([]string{})

		epFlag := enrollCmd.Flags().Lookup("endpoint")
		require.NotNil(t, epFlag)
		assert.Equal(t, "e", epFlag.Shorthand)

		odFlag := enrollCmd.Flags().Lookup("output-dir")
		require.NotNil(t, odFlag)
	})
}

// --- Security Validate with valid PEM ---

func TestSecurityValidateCmd_API_ValidPEMSuccess(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	validCert := generateTestPEMCert(t)

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Security validation passed")
	assert.Contains(t, buf.String(), "root_ca.crt is valid PEM")
	assert.Contains(t, buf.String(), "Trust bundle is valid PEM")
}

func TestSecurityValidateCmd_API_InvalidPEM(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), []byte("not a cert"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), []byte("not a bundle"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, buf.String(), "root_ca.crt is not a valid PEM")
	assert.Contains(t, buf.String(), "Trust bundle is not valid PEM")
}

func TestSecurityValidateCmd_API_PortCheckOutput(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	pkiDir := filepath.Join(tmpDir, "pki")
	secretsDir := filepath.Join(tmpDir, "secrets")

	validCert := generateTestPEMCert(t)

	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "trust"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.crt"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "root", "root_ca.key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem"), validCert, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pkiDir, "warden_pub.pem"), []byte("dummy warden"), 0644))

	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "session_encryption_key"), []byte("dummy key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(secretsDir, "bootstrap_digest.json"), []byte("{}"), 0644))

	cmd := securityValidateCmdWithConfig(newFileSvc)
	cmd.Flags().Set("pki-dir", pkiDir)
	cmd.Flags().Set("secrets-dir", secretsDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Port Availability")
	assert.Contains(t, output, fmt.Sprintf("Port %d", constants.Ports.OperatorHttp))
	assert.Contains(t, output, fmt.Sprintf("Port %d", constants.Ports.OperatorHttps))
}
