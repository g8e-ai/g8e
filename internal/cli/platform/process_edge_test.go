package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReExecArgs(t *testing.T) {
	tmpDir := t.TempDir()

	pm, err := NewProcessManager(tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		opts        OperatorStartOptions
		needsDirs   bool
		wantSubstrs []string
	}{
		{
			name: "minimal",
			opts: OperatorStartOptions{
				Posture:    "consensus",
				HTTPPort:   8080,
				HTTPSPort:  8443,
				DataDir:    "/data",
				PKIDir:     "/pki",
				SecretsDir: "/secrets",
				LogLevel:   "info",
			},
			wantSubstrs: []string{
				"--posture consensus",
				"--data-dir /data",
				"--pki-dir /pki",
				"--secrets-dir /secrets",
				"--http-port 8080",
				"--https-port 8443",
				"--log info",
			},
		},
		{
			name: "all options",
			opts: OperatorStartOptions{
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
			},
			wantSubstrs: []string{
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
			},
		},
		{
			name: "with identity data",
			opts: OperatorStartOptions{
				Posture:      "consensus",
				HTTPPort:     8080,
				HTTPSPort:    8443,
				DataDir:      "/data",
				PKIDir:       "/pki",
				SecretsDir:   "/secrets",
				LogLevel:     "info",
				IdentityData: []byte(`{"hostname":"test"}`),
			},
			needsDirs:   true,
			wantSubstrs: []string{"--network-identity-file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsDirs {
				require.NoError(t, pm.CreateDirectories())
			}

			args, err := pm.BuildReExecArgs(tt.opts)
			require.NoError(t, err)

			require.GreaterOrEqual(t, len(args), 2)
			assert.Equal(t, "gateway", args[0])
			assert.Equal(t, "serve", args[1])

			joined := strings.Join(args, " ")
			for _, expected := range tt.wantSubstrs {
				assert.Contains(t, joined, expected)
			}
		})
	}
}
