// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTrustBundle_DefaultPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	caPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	relPath := cfg.DefaultTrustBundleRelPath()
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte(caPEM), constants.PermFilePrivate))

	data, err := ReadTrustBundle(fileSvc, cfg)
	require.NoError(t, err)
	assert.Equal(t, caPEM, string(data))
}

func TestReadTrustBundle_CustomPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	customPath := filepath.Join(runtimeDir, "external", "ca-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(customPath), constants.PermDirPrivate))

	caPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")
	require.NoError(t, os.WriteFile(customPath, []byte(caPEM), constants.PermFilePrivate))

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = customPath

	data, err := ReadTrustBundle(fileSvc, cfg)
	require.NoError(t, err)
	assert.Equal(t, caPEM, string(data))
}

func TestReadTrustBundle_MissingDefaultReturnsTypedError(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	_, err := ReadTrustBundle(fileSvc, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestReadTrustBundle_MissingCustomReturnsOSErrNotExist(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	customPath := filepath.Join(runtimeDir, "does-not-exist.pem")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = customPath

	_, err := ReadTrustBundle(fileSvc, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestWriteTrustBundleFS_DefaultPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	data := []byte("test-bundle-content")
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, data, constants.PermFilePrivate))

	read, err := fileSvc.ReadFile(context.Background(), cfg.DefaultTrustBundleRelPath())
	require.NoError(t, err)
	assert.Equal(t, data, read)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(runtimeDir, cfg.DefaultTrustBundleRelPath()))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
	}
}

func TestWriteTrustBundleFS_CustomPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	customPath := filepath.Join(runtimeDir, "external-dir", "ca-bundle.pem")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = customPath

	data := []byte("test-bundle-content")
	require.NoError(t, WriteTrustBundleFS(fileSvc, cfg, data, constants.PermFilePrivate))

	read, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Equal(t, data, read)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(customPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
	}
}

func TestRemoveTrustBundleFS_DefaultPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	relPath := cfg.DefaultTrustBundleRelPath()
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, []byte("data"), constants.PermFilePrivate))

	require.NoError(t, RemoveTrustBundleFS(fileSvc, cfg))

	exists, err := fileSvc.FileExists(context.Background(), relPath)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemoveTrustBundleFS_DefaultPathNonExistentIsNoOp(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	require.NoError(t, RemoveTrustBundleFS(fileSvc, cfg))
}

func TestRemoveTrustBundleFS_CustomPath(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	customPath := filepath.Join(runtimeDir, "external", "ca-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(customPath), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(customPath, []byte("data"), constants.PermFilePrivate))

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = customPath

	require.NoError(t, RemoveTrustBundleFS(fileSvc, cfg))

	_, err := os.Stat(customPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoveTrustBundleFS_CustomPathNonExistentIsNoOp(t *testing.T) {
	t.Parallel()
	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")

	customPath := filepath.Join(runtimeDir, "never-existed.pem")

	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}
	cfg.Paths.Infra.CACertPath = customPath

	require.NoError(t, RemoveTrustBundleFS(fileSvc, cfg))
}

func TestBuildMTLSClient_ReadsTrustBundleFromRuntimeTree(t *testing.T) {
	t.Parallel()
	fileSvc, cfg := newAuthTestEnv(t)

	caPEM, _ := testutil.GenerateTestCertificate(t, "test-ca")

	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte(caPEM), constants.PermFilePrivate))

	writeValidCertPair(t, fileSvc, cfg.CLICertFile(), cfg.CLIKeyFile())

	client, err := BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)
}

// writeValidCertPair generates a self-signed certificate and writes the cert
// and key PEM files to the runtime tree via fileSvc.
func writeValidCertPair(t *testing.T, fileSvc fs.RuntimeFileService, certFile, keyFile string) {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certRel, err := fileSvc.RelFromAbs(certFile)
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(keyFile)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))
}
