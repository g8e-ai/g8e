package platform

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func netListenForTest() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

func TestBuildReExecArgs_Minimal(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	opts := OperatorStartOptions{
		Posture:    "consensus",
		HTTPPort:   8080,
		HTTPSPort:  8443,
		DataDir:    "/data",
		PKIDir:     "/pki",
		SecretsDir: "/secrets",
		LogLevel:   "info",
	}

	args, err := pm.BuildReExecArgs(opts)
	if err != nil {
		t.Fatalf("BuildReExecArgs failed: %v", err)
	}

	if len(args) < 2 {
		t.Fatalf("expected at least 2 args, got %d", len(args))
	}
	if args[0] != "gateway" || args[1] != "serve" {
		t.Errorf("expected first args to be 'gateway serve', got %s %s", args[0], args[1])
	}

	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"--posture consensus",
		"--data-dir /data",
		"--pki-dir /pki",
		"--secrets-dir /secrets",
		"--http-port 8080",
		"--https-port 8443",
		"--log info",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("expected args to contain %q, got: %s", expected, joined)
		}
	}
}

func TestBuildReExecArgs_AllOptions(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	opts := OperatorStartOptions{
		Posture:            "notary",
		HTTPPort:           9000,
		HTTPSPort:          9443,
		DataDir:            "/data",
		PKIDir:             "/pki",
		SecretsDir:         "/secrets",
		VaultDir:           "/vault",
		VaultKeyPath:       "/vault/key",
		VaultRequireUnlock: true,
		CertIdentityMode:   "spiffe",
		TribunalID:         "trib-1",
		TribunalURL:        "https://trib:8443",
		TribunalBootstrap:  "bootstrap-data",
		MCPDownstreamURL:   "http://mcp:8080",
		A2ADownstreamURL:   "http://a2a:8081",
		PasskeyRpID:        "localhost",
		PasskeyRpName:      "G8E",
		PasskeyRpOrigins:   []string{"http://localhost:8080", "http://127.0.0.1:8080"},
		RateLimitRPS:       100.5,
		RateLimitBurst:     200,
		LogLevel:           "debug",
	}

	args, err := pm.BuildReExecArgs(opts)
	if err != nil {
		t.Fatalf("BuildReExecArgs failed: %v", err)
	}

	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"--vault-dir /vault",
		"--vault-key /vault/key",
		"--vault-require-unlock",
		"--cert-mode spiffe",
		"--tribunal-id trib-1",
		"--tribunal-url https://trib:8443",
		"--tribunal-bootstrap bootstrap-data",
		"--mcp-downstream-url http://mcp:8080",
		"--a2a-downstream-url http://a2a:8081",
		"--passkey-rp-id localhost",
		"--passkey-rp-name G8E",
		"--passkey-rp-origin http://localhost:8080",
		"--passkey-rp-origin http://127.0.0.1:8080",
		"--rate-limit-rps 100.5",
		"--rate-limit-burst 200",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("expected args to contain %q, got: %s", expected, joined)
		}
	}
}

func TestBuildReExecArgs_WithIdentityData(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	opts := OperatorStartOptions{
		Posture:       "consensus",
		HTTPPort:      8080,
		HTTPSPort:     8443,
		DataDir:       "/data",
		PKIDir:        "/pki",
		SecretsDir:    "/secrets",
		LogLevel:      "info",
		IdentityData:  []byte(`{"hostname":"test"}`),
	}

	args, err := pm.BuildReExecArgs(opts)
	if err != nil {
		t.Fatalf("BuildReExecArgs failed: %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--network-identity-file") {
		t.Errorf("expected args to contain --network-identity-file, got: %s", joined)
	}
}

func TestGetOperatorBinary(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	binPath, err := pm.getOperatorBinary()
	if err != nil {
		t.Fatalf("getOperatorBinary failed: %v", err)
	}
	if binPath == "" {
		t.Error("expected non-empty binary path")
	}
}

func TestTailLog_NoFollow(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := TailLog(logPath, false)
	if err != nil {
		t.Errorf("TailLog failed: %v", err)
	}
}

func TestTailLog_NonExistentFile(t *testing.T) {
	err := TailLog("/nonexistent/path/to/log", false)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestClean_RemovesRuntimeDir(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	runtimeDir := pm.runtimeDir
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		t.Fatalf("runtime dir should exist before Clean")
	}

	if err := pm.Clean(); err != nil {
		t.Errorf("Clean failed: %v", err)
	}

	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Error("runtime dir should not exist after Clean")
	}
}

func TestReset_RemovesDataAndSecrets(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	dataDir := pm.dataDir
	secretsDir := pm.secretsDir

	if err := os.WriteFile(filepath.Join(dataDir, "test"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "test"), []byte("secret"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := pm.Reset(); err != nil {
		t.Errorf("Reset failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "test")); !os.IsNotExist(err) {
		t.Error("data file should not exist after Reset")
	}
	if _, err := os.Stat(filepath.Join(secretsDir, "test")); !os.IsNotExist(err) {
		t.Error("secret file should not exist after Reset")
	}

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("data dir should exist after Reset (ensureDirectories recreates it)")
	}
}

func TestGetLogPath_Edge(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	logPath := pm.GetLogPath()
	if logPath == "" {
		t.Error("expected non-empty log path")
	}
	if !strings.Contains(logPath, "operator") {
		t.Errorf("expected log path to contain 'operator', got: %s", logPath)
	}
}

func TestWriteAndDeletePID(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	if err := pm.writePID(constants.OperatorPIDFilename, 12345); err != nil {
		t.Fatalf("writePID failed: %v", err)
	}

	pid, err := pm.readPID(constants.OperatorPIDFilename)
	if err != nil {
		t.Fatalf("readPID failed: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected pid 12345, got %d", pid)
	}

	if err := pm.deletePID(constants.OperatorPIDFilename); err != nil {
		t.Errorf("deletePID failed: %v", err)
	}

	pid, err = pm.readPID(constants.OperatorPIDFilename)
	if err != nil {
		t.Fatalf("readPID after delete failed: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0 after delete, got %d", pid)
	}
}

func TestReadPID_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	if err := pm.ensureDirectories(); err != nil {
		t.Fatalf("ensureDirectories failed: %v", err)
	}

	pidFile := filepath.Join(pm.pidDir, constants.OperatorPIDFilename)
	if err := os.WriteFile(pidFile, []byte("not-a-number"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = pm.readPID(constants.OperatorPIDFilename)
	if err == nil {
		t.Error("expected error for invalid PID data")
	}
}

func TestCheckPortAvailable_FreePort(t *testing.T) {
	tmpDir := t.TempDir()
	pm, err := NewProcessManager(tmpDir)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Find a free port by binding to :0
	ln, err := netListenForTest()
	if err != nil {
		t.Fatalf("failed to get listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if err := pm.checkPortAvailable(port, "test"); err != nil {
		t.Errorf("checkPortAvailable failed for free port %d: %v", port, err)
	}
}
