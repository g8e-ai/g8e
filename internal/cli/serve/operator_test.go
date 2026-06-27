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

package serve

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeOperatorOptions_ZeroValue(t *testing.T) {
	var opts ServeOperatorOptions

	assert.Equal(t, "", opts.LogLevel)
	assert.Equal(t, "", opts.Endpoint)
	assert.Equal(t, "", opts.TrustBundlePath)
	assert.Equal(t, "", opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert)
	assert.Equal(t, "", opts.WorkingDir)
	assert.Equal(t, "", opts.LaunchDir)
	assert.False(t, opts.CloudMode)
	assert.Equal(t, "", opts.CloudProvider)
	assert.False(t, opts.ExecutionVault)
	assert.False(t, opts.NoGit)
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval)
}

func TestServeOperatorOptions_FullAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:          "debug",
		Endpoint:          "192.168.1.10",
		TrustBundlePath:   "/etc/g8e/ca.pem",
		PrivateKey:        "/etc/g8e/operator.key",
		ClientCert:        "/etc/g8e/operator.crt",
		WorkingDir:        "/var/lib/g8e",
		LaunchDir:         "/opt/g8e",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 30 * time.Second,
	}

	assert.Equal(t, "debug", opts.LogLevel)
	assert.Equal(t, "192.168.1.10", opts.Endpoint)
	assert.Equal(t, "/etc/g8e/ca.pem", opts.TrustBundlePath)
	assert.Equal(t, "/etc/g8e/operator.key", opts.PrivateKey)
	assert.Equal(t, "/etc/g8e/operator.crt", opts.ClientCert)
	assert.Equal(t, "/var/lib/g8e", opts.WorkingDir)
	assert.Equal(t, "/opt/g8e", opts.LaunchDir)
	assert.True(t, opts.CloudMode)
	assert.Equal(t, "aws", opts.CloudProvider)
	assert.True(t, opts.ExecutionVault)
	assert.True(t, opts.NoGit)
	assert.Equal(t, 30*time.Second, opts.HeartbeatInterval)
}

func TestServeOperatorOptions_Equality(t *testing.T) {
	a := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       "localhost",
		PrivateKey:     "/key.pem",
		ClientCert:     "/cert.pem",
		WorkingDir:     "/work",
		LaunchDir:      "/launch",
		CloudMode:      false,
		ExecutionVault: true,
	}
	b := ServeOperatorOptions{
		LogLevel:       "info",
		Endpoint:       "localhost",
		PrivateKey:     "/key.pem",
		ClientCert:     "/cert.pem",
		WorkingDir:     "/work",
		LaunchDir:      "/launch",
		CloudMode:      false,
		ExecutionVault: true,
	}
	c := a
	c.ExecutionVault = false

	require.True(t, a == b, "structs with identical fields should be equal")
	require.False(t, a == c, "structs differing in any field should not be equal")
}

func TestServeOperatorOptions_PartialAssignment(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:   "info",
		Endpoint:   "10.0.0.1",
		PrivateKey: "/tmp/key.pem",
	}

	assert.Equal(t, "info", opts.LogLevel)
	assert.Equal(t, "10.0.0.1", opts.Endpoint)
	assert.Equal(t, "/tmp/key.pem", opts.PrivateKey)
	assert.Equal(t, "", opts.ClientCert, "unassigned ClientCert should be zero value")
	assert.Equal(t, "", opts.WorkingDir, "unassigned WorkingDir should be zero value")
	assert.Equal(t, "", opts.LaunchDir, "unassigned LaunchDir should be zero value")
	assert.False(t, opts.CloudMode, "unassigned CloudMode should be false")
	assert.False(t, opts.ExecutionVault, "unassigned ExecutionVault should be false")
	assert.False(t, opts.NoGit, "unassigned NoGit should be false")
	assert.Equal(t, time.Duration(0), opts.HeartbeatInterval, "unassigned HeartbeatInterval should be zero")
}

func TestServeOperatorOptions_HeartbeatInterval(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"one second", 1 * time.Second, 1 * time.Second},
		{"thirty seconds", 30 * time.Second, 30 * time.Second},
		{"one minute", time.Minute, 60 * time.Second},
		{"five minutes", 5 * time.Minute, 300 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServeOperatorOptions{HeartbeatInterval: tt.duration}
			assert.Equal(t, tt.expected, opts.HeartbeatInterval)
		})
	}
}

func TestServeOperatorOptions_CloudProviderValues(t *testing.T) {
	providers := []string{"aws", "gcp", "azure", ""}

	for _, p := range providers {
		opts := ServeOperatorOptions{CloudMode: true, CloudProvider: p}
		assert.Equal(t, p, opts.CloudProvider)
		assert.True(t, opts.CloudMode)
	}
}

func TestResolveOperatorEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string returns default", "", "localhost"},
		{"whitespace only returns default", "   ", "localhost"},
		{"tab only returns default", "\t", "localhost"},
		{"simple hostname", "localhost", "localhost"},
		{"ip address", "192.168.1.10", "192.168.1.10"},
		{"hostname with port", "gateway.local:8080", "gateway.local:8080"},
		{"leading whitespace trimmed", "  localhost", "localhost"},
		{"trailing whitespace trimmed", "localhost  ", "localhost"},
		{"surrounding whitespace trimmed", "  10.0.0.1  ", "10.0.0.1"},
		{"fqdn", "operator.example.com", "operator.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOperatorEndpoint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveOperatorEndpoint_DefaultConstant(t *testing.T) {
	result := resolveOperatorEndpoint("")
	assert.Equal(t, "localhost", result, "default endpoint should be localhost")
}

func TestResolveOperatorEndpoint_NoTrimmingForNonWhitespace(t *testing.T) {
	result := resolveOperatorEndpoint("my-endpoint")
	assert.Equal(t, "my-endpoint", result)
}

func TestResolveWorkingDir(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"working dir set, launch dir set", "/var/work", "/opt/launch", "/var/work"},
		{"working dir set, launch dir empty", "/var/work", "", "/var/work"},
		{"working dir empty, launch dir set", "", "/opt/launch", "/opt/launch"},
		{"both empty", "", "", ""},
		{"working dir takes precedence", "/custom", "/default", "/custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkingDir_WorkingDirPrecedence(t *testing.T) {
	workingDir := "/explicit/working"
	launchDir := "/explicit/launch"

	result := resolveWorkingDir(workingDir, launchDir)

	assert.Equal(t, workingDir, result, "WorkingDir should take precedence over LaunchDir")
	assert.NotEqual(t, launchDir, result, "result should not be the LaunchDir when WorkingDir is set")
}

func TestResolveWorkingDir_LaunchDirFallback(t *testing.T) {
	launchDir := "/fallback/launch"

	result := resolveWorkingDir("", launchDir)

	assert.Equal(t, launchDir, result, "should fall back to LaunchDir when WorkingDir is empty")
}

func TestResolveWorkingDir_BothEmpty(t *testing.T) {
	result := resolveWorkingDir("", "")
	assert.Equal(t, "", result)
}

// ---------------------------------------------------------------------------
// resolveOperatorEndpoint — additional edge cases
// ---------------------------------------------------------------------------

func TestResolveOperatorEndpoint_ReturnsConstantsDefault(t *testing.T) {
	result := resolveOperatorEndpoint("")
	assert.Equal(t, constants.DefaultEndpoint, result,
		"empty endpoint should return constants.DefaultEndpoint, not a hardcoded string")
}

func TestResolveOperatorEndpoint_NewlineTrimmed(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"leading newline", "\nlocalhost"},
		{"trailing newline", "localhost\n"},
		{"carriage return", "\rlocalhost\r"},
		{"mixed newline and tab", "\n\tlocalhost\n\t"},
		{"CRLF", "\r\nlocalhost\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveOperatorEndpoint(tt.input)
			assert.Equal(t, "localhost", result)
		})
	}
}

func TestResolveOperatorEndpoint_PreservesInternalWhitespace(t *testing.T) {
	result := resolveOperatorEndpoint("  local  host  ")
	assert.Equal(t, "local  host", result,
		"only leading/trailing whitespace should be trimmed; internal whitespace preserved")
}

func TestResolveOperatorEndpoint_MixedWhitespaceTypes(t *testing.T) {
	result := resolveOperatorEndpoint("\t\n\r  10.0.0.1  \r\n\t")
	assert.Equal(t, "10.0.0.1", result)
}

func TestResolveOperatorEndpoint_LongEndpoint(t *testing.T) {
	long := strings.Repeat("a", 1000)
	result := resolveOperatorEndpoint(long)
	assert.Equal(t, long, result)
}

func TestResolveOperatorEndpoint_WhitespaceOnlyWithNewlines(t *testing.T) {
	result := resolveOperatorEndpoint("\n\r\t  \n\r")
	assert.Equal(t, constants.DefaultEndpoint, result,
		"input that is only whitespace (including newlines) should return default")
}

// ---------------------------------------------------------------------------
// resolveWorkingDir — additional edge cases
// ---------------------------------------------------------------------------

func TestResolveWorkingDir_IdenticalPaths(t *testing.T) {
	path := "/same/path"
	result := resolveWorkingDir(path, path)
	assert.Equal(t, path, result)
}

func TestResolveWorkingDir_RelativePaths(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"relative working dir", "./work", "/abs/launch", "./work"},
		{"relative launch dir fallback", "", "./launch", "./launch"},
		{"both relative", "./work", "./launch", "./work"},
		{"dot as working dir", ".", "/abs", "."},
		{"dotdot as working dir", "..", "/abs", ".."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkingDir_PathsWithSpaces(t *testing.T) {
	result := resolveWorkingDir("/path with spaces/work", "/other/path")
	assert.Equal(t, "/path with spaces/work", result)
}

func TestResolveWorkingDir_TrailingSlashes(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"working dir with trailing slash", "/work/", "/launch", "/work/"},
		{"launch dir with trailing slash", "", "/launch/", "/launch/"},
		{"both with trailing slashes", "/work/", "/launch/", "/work/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveWorkingDir_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		launchDir  string
		expected   string
	}{
		{"tilde", "~/.g8e", "/abs", "~/.g8e"},
		{"env-like", "$HOME/g8e", "/abs", "$HOME/g8e"},
		{"unicode", "/path/数据", "/abs", "/path/数据"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDir(tt.workingDir, tt.launchDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// ServeOperatorOptions — additional edge cases
// ---------------------------------------------------------------------------

func TestServeOperatorOptions_NegativeHeartbeatInterval(t *testing.T) {
	opts := ServeOperatorOptions{HeartbeatInterval: -5 * time.Second}
	assert.Equal(t, -5*time.Second, opts.HeartbeatInterval,
		"negative durations should be stored as-is; validation is the caller's responsibility")
}

func TestServeOperatorOptions_AllBooleansTrue(t *testing.T) {
	opts := ServeOperatorOptions{
		CloudMode:      true,
		ExecutionVault: true,
		NoGit:          true,
	}
	assert.True(t, opts.CloudMode)
	assert.True(t, opts.ExecutionVault)
	assert.True(t, opts.NoGit)
}

func TestServeOperatorOptions_AllBooleansFalse(t *testing.T) {
	opts := ServeOperatorOptions{
		CloudMode:      false,
		ExecutionVault: false,
		NoGit:          false,
	}
	assert.False(t, opts.CloudMode)
	assert.False(t, opts.ExecutionVault)
	assert.False(t, opts.NoGit)
}

func TestServeOperatorOptions_TrustBundlePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"absolute path", "/etc/g8e/ca.pem"},
		{"relative path", "./ca.pem"},
		{"empty", ""},
		{"with spaces", "/path with spaces/ca.pem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ServeOperatorOptions{TrustBundlePath: tt.path}
			assert.Equal(t, tt.path, opts.TrustBundlePath)
		})
	}
}

func TestServeOperatorOptions_Equality_DifferInEachField(t *testing.T) {
	base := ServeOperatorOptions{
		LogLevel:          "info",
		Endpoint:          "localhost",
		TrustBundlePath:   "/ca.pem",
		PrivateKey:        "/key.pem",
		ClientCert:        "/cert.pem",
		WorkingDir:        "/work",
		LaunchDir:         "/launch",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 30 * time.Second,
	}

	fields := []struct {
		name string
		mut  func(o *ServeOperatorOptions)
	}{
		{"LogLevel", func(o *ServeOperatorOptions) { o.LogLevel = "debug" }},
		{"Endpoint", func(o *ServeOperatorOptions) { o.Endpoint = "other" }},
		{"TrustBundlePath", func(o *ServeOperatorOptions) { o.TrustBundlePath = "/other" }},
		{"PrivateKey", func(o *ServeOperatorOptions) { o.PrivateKey = "/other" }},
		{"ClientCert", func(o *ServeOperatorOptions) { o.ClientCert = "/other" }},
		{"WorkingDir", func(o *ServeOperatorOptions) { o.WorkingDir = "/other" }},
		{"LaunchDir", func(o *ServeOperatorOptions) { o.LaunchDir = "/other" }},
		{"CloudMode", func(o *ServeOperatorOptions) { o.CloudMode = false }},
		{"CloudProvider", func(o *ServeOperatorOptions) { o.CloudProvider = "gcp" }},
		{"ExecutionVault", func(o *ServeOperatorOptions) { o.ExecutionVault = false }},
		{"NoGit", func(o *ServeOperatorOptions) { o.NoGit = false }},
		{"HeartbeatInterval", func(o *ServeOperatorOptions) { o.HeartbeatInterval = 60 * time.Second }},
	}

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			modified := base
			f.mut(&modified)
			require.False(t, base == modified,
				"structs differing in %s should not be equal", f.name)
		})
	}
}

func TestServeOperatorOptions_Equality_AllFieldsEqual(t *testing.T) {
	a := ServeOperatorOptions{
		LogLevel:          "debug",
		Endpoint:          "10.0.0.1",
		TrustBundlePath:   "/ca.pem",
		PrivateKey:        "/key.pem",
		ClientCert:        "/cert.pem",
		WorkingDir:        "/work",
		LaunchDir:         "/launch",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 45 * time.Second,
	}
	b := a
	require.True(t, a == b, "structs with all 12 fields identical should be equal")
}

// ---------------------------------------------------------------------------
// resolveKeyPath
// ---------------------------------------------------------------------------

func TestResolveKeyPath_ExplicitPath(t *testing.T) {
	result := resolveKeyPath("/explicit/key.pem", testLogger())
	assert.Equal(t, "/explicit/key.pem", result)
}

func TestResolveKeyPath_DefaultOperatorKey(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("fake key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorKeyPath, result)
}

func TestResolveKeyPath_FallsBackToClientKey(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorKeyPath, []byte("fake key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.ClientOperatorKeyPath, result)
}

func TestResolveKeyPath_NoFilesFound(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, "", result)
}

func TestResolveKeyPath_OperatorKeyTakesPrecedenceOverClientKey(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("op key"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorKeyPath, []byte("client key"), 0600))

	result := resolveKeyPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorKeyPath, result,
		"operator key should take precedence over client key when both exist")
}

func TestResolveKeyPath_ExplicitOverridesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorKeyPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorKeyPath, []byte("op key"), 0600))

	result := resolveKeyPath("/explicit/key.pem", testLogger())
	assert.Equal(t, "/explicit/key.pem", result,
		"explicit path should override default file lookup")
}

// ---------------------------------------------------------------------------
// resolveCertPath
// ---------------------------------------------------------------------------

func TestResolveCertPath_ExplicitPath(t *testing.T) {
	result := resolveCertPath("/explicit/cert.pem", testLogger())
	assert.Equal(t, "/explicit/cert.pem", result)
}

func TestResolveCertPath_DefaultOperatorCert(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("fake cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorCertPath, result)
}

func TestResolveCertPath_FallsBackToClientCert(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorCertPath, []byte("fake cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.ClientOperatorCertPath, result)
}

func TestResolveCertPath_NoFilesFound(t *testing.T) {
	require.NoError(t, paths.InitWithBase(t.TempDir()))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, "", result)
}

func TestResolveCertPath_OperatorCertTakesPrecedenceOverClientCert(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("op cert"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.ClientOperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.ClientOperatorCertPath, []byte("client cert"), 0600))

	result := resolveCertPath("", testLogger())
	assert.Equal(t, paths.Infra.OperatorCertPath, result,
		"operator cert should take precedence over client cert when both exist")
}

func TestResolveCertPath_ExplicitOverridesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, paths.InitWithBase(tmpDir))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Infra.OperatorCertPath), 0700))
	require.NoError(t, os.WriteFile(paths.Infra.OperatorCertPath, []byte("op cert"), 0600))

	result := resolveCertPath("/explicit/cert.pem", testLogger())
	assert.Equal(t, "/explicit/cert.pem", result,
		"explicit path should override default file lookup")
}

// ---------------------------------------------------------------------------
// loadClientCertPair
// ---------------------------------------------------------------------------

// generateTestKeyCertPair creates a self-signed cert and matching ECDSA private key
// in PEM format, writing them to temp files. Returns (certPath, keyPath).
func generateTestKeyCertPair(t *testing.T) (string, string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-client"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.crt")
	keyPath := filepath.Join(dir, "client.key")
	require.NoError(t, os.WriteFile(certPath, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))
	return certPath, keyPath
}

func TestLoadClientCertPair_Success(t *testing.T) {
	certPath, keyPath := generateTestKeyCertPair(t)

	cert, certPEM, err := loadClientCertPair(certPath, keyPath)
	require.NoError(t, err)
	assert.NotNil(t, cert.Certificate)
	assert.NotEmpty(t, certPEM)
	assert.Contains(t, string(certPEM), "BEGIN CERTIFICATE")
}

func TestLoadClientCertPair_NonExistentCertFile(t *testing.T) {
	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(filepath.Join(t.TempDir(), "nonexistent.crt"), keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadClientCert))
}

func TestLoadClientCertPair_NonExistentKeyFile(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(certPath, filepath.Join(t.TempDir(), "nonexistent.key"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrReadPrivateKey))
}

func TestLoadClientCertPair_InvalidCertPEM(t *testing.T) {
	dir := t.TempDir()
	invalidCert := filepath.Join(dir, "invalid.crt")
	require.NoError(t, os.WriteFile(invalidCert, []byte("not a PEM file"), 0600))

	_, keyPath := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(invalidCert, keyPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_InvalidKeyPEM(t *testing.T) {
	certPath, _ := generateTestKeyCertPair(t)

	dir := t.TempDir()
	invalidKey := filepath.Join(dir, "invalid.key")
	require.NoError(t, os.WriteFile(invalidKey, []byte("not a PEM file"), 0600))

	_, _, err := loadClientCertPair(certPath, invalidKey)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

func TestLoadClientCertPair_MismatchedKeyCertPair(t *testing.T) {
	certPath1, _ := generateTestKeyCertPair(t)
	_, keyPath2 := generateTestKeyCertPair(t)

	_, _, err := loadClientCertPair(certPath1, keyPath2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrLoadCertKeyPair))
}

// ---------------------------------------------------------------------------
// buildOperatorLoadOptions
// ---------------------------------------------------------------------------

func TestBuildOperatorLoadOptions_BasicMapping(t *testing.T) {
	opts := ServeOperatorOptions{
		LogLevel:          "debug",
		CloudMode:         true,
		CloudProvider:     "aws",
		ExecutionVault:    true,
		NoGit:             true,
		HeartbeatInterval: 45 * time.Second,
	}

	loadOpts := buildOperatorLoadOptions(opts, "10.0.0.1", "/work/dir")

	assert.Equal(t, "10.0.0.1", loadOpts.OperatorEndpoint)
	assert.Equal(t, 0, loadOpts.HTTPPort)
	assert.Equal(t, 0, loadOpts.HTTPSPort)
	assert.True(t, loadOpts.CloudMode)
	assert.Equal(t, "aws", loadOpts.CloudProvider)
	assert.True(t, loadOpts.ExecutionVaultEnabled)
	assert.True(t, loadOpts.NoGit)
	assert.Equal(t, "debug", loadOpts.LogLevel)
	assert.Equal(t, "/work/dir", loadOpts.WorkDir)
	assert.Equal(t, 45*time.Second, loadOpts.HeartbeatInterval)
}

func TestBuildOperatorLoadOptions_EmptyEndpoint(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "", "/work")
	assert.Equal(t, "", loadOpts.OperatorEndpoint)
	assert.Equal(t, "/work", loadOpts.WorkDir)
}

func TestBuildOperatorLoadOptions_ZeroValues(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "localhost", "/launch")

	assert.False(t, loadOpts.CloudMode)
	assert.False(t, loadOpts.ExecutionVaultEnabled)
	assert.False(t, loadOpts.NoGit)
	assert.Equal(t, "", loadOpts.CloudProvider)
	assert.Equal(t, time.Duration(0), loadOpts.HeartbeatInterval)
	assert.Equal(t, "", loadOpts.PKIDir)
	assert.Equal(t, "", loadOpts.SecretsDir)
	assert.Equal(t, config.GatewayPosture(""), loadOpts.Posture)
}

func TestBuildOperatorLoadOptions_EnvVarsPropagated(t *testing.T) {
	t.Setenv(string(constants.EnvVar.Shell), "/bin/zsh")
	t.Setenv(string(constants.EnvVar.Lang), "en_US.UTF-8")
	t.Setenv(string(constants.EnvVar.Term), "xterm-256color")
	t.Setenv(string(constants.EnvVar.TZ), "America/Los_Angeles")

	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "localhost", "/work")

	assert.Equal(t, "/bin/zsh", loadOpts.Shell)
	assert.Equal(t, "en_US.UTF-8", loadOpts.Lang)
	assert.Equal(t, "xterm-256color", loadOpts.Term)
	assert.Equal(t, "America/Los_Angeles", loadOpts.TZ)
}

func TestBuildOperatorLoadOptions_PortAlwaysZero(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "10.0.0.1", "/work")
	assert.Equal(t, 0, loadOpts.HTTPPort, "HTTPPort should always be 0 for operator mode")
	assert.Equal(t, 0, loadOpts.HTTPSPort, "HTTPSPort should always be 0 for operator mode")
}

func TestBuildOperatorLoadOptions_PostureAlwaysEmpty(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "localhost", "/work")
	assert.Equal(t, config.GatewayPosture(""), loadOpts.Posture, "Posture should always be empty for operator mode")
}

func TestBuildOperatorLoadOptions_PKIAndSecretsAlwaysEmpty(t *testing.T) {
	opts := ServeOperatorOptions{}
	loadOpts := buildOperatorLoadOptions(opts, "localhost", "/work")
	assert.Equal(t, "", loadOpts.PKIDir, "PKIDir should always be empty for operator mode")
	assert.Equal(t, "", loadOpts.SecretsDir, "SecretsDir should always be empty for operator mode")
}
