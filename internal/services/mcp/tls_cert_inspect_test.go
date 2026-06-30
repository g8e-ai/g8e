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

package mcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLSCertInspectTool_Metadata(t *testing.T) {
	tool := &TLSCertInspectTool{}
	assert.Equal(t, "tls_cert_inspect", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.InputSchema())
}

func TestInspectCertificate(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	notBefore := now.Add(-24 * time.Hour)
	notAfter := now.Add(24 * time.Hour)

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         "test-subject",
			Organization:       []string{"test-org"},
			OrganizationalUnit: []string{"test-ou"},
		},
		Issuer: pkix.Name{
			CommonName: "test-issuer",
		},
		SerialNumber:       big.NewInt(12345),
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		SignatureAlgorithm: x509.SHA256WithRSA,
		PublicKeyAlgorithm: x509.RSA,
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:           []string{"example.com"},
		EmailAddresses:     []string{"test@example.com"},
		IPAddresses:        []net.IP{net.ParseIP("1.2.3.4")},
	}

	result := inspectCertificate(cert)

	assert.Equal(t, "test-subject", result.Subject)
	assert.Equal(t, "test-issuer", result.Issuer)
	assert.Equal(t, "12345", result.SerialNumber)
	assert.Equal(t, notBefore.Format(time.RFC3339), result.NotBefore)
	assert.Equal(t, notAfter.Format(time.RFC3339), result.NotAfter)
	assert.False(t, result.IsExpired)
	assert.True(t, result.DaysUntilExpiry >= 0)
	assert.Equal(t, "SHA256-RSA", result.SignatureAlgorithm)
	assert.Equal(t, "RSA", result.PublicKeyAlgorithm)
	assert.Equal(t, x509.KeyUsageDigitalSignature, result.KeyUsage)
	assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, result.ExtKeyUsage)
	assert.Equal(t, []string{"example.com"}, result.DNSNames)
	assert.Equal(t, []string{"test@example.com"}, result.EmailAddresses)
	assert.Equal(t, []string{"1.2.3.4"}, result.IPAddresses)
	assert.Equal(t, "test-org", result.Organization)
	assert.Equal(t, "test-ou", result.OrganizationalUnit)
}

func TestInspectCertificate_Expired(t *testing.T) {
	now := time.Now()
	cert := &x509.Certificate{
		NotAfter:     now.Add(-24 * time.Hour),
		SerialNumber: big.NewInt(1),
	}

	result := inspectCertificate(cert)
	assert.True(t, result.IsExpired)
	assert.True(t, result.DaysUntilExpiry < 0)
	assert.False(t, result.IsNearExpiry)
}

func TestInspectCertificate_NearExpiry(t *testing.T) {
	now := time.Now()
	// 15 days from now is within the 30-day "near expiry" window
	cert := &x509.Certificate{
		NotAfter:     now.Add(15 * 24 * time.Hour),
		SerialNumber: big.NewInt(1),
	}

	result := inspectCertificate(cert)
	assert.False(t, result.IsExpired)
	assert.True(t, result.IsNearExpiry)
}

func TestTLSCertInspectTool_Execute_Validation(t *testing.T) {
	tool := &TLSCertInspectTool{}
	ctx := context.Background()

	// Empty arguments
	_, err := tool.Execute(ctx, json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPTLSCertInspectRequired))

	// Invalid JSON
	_, err = tool.Execute(ctx, json.RawMessage(`{invalid`))
	assert.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPUnmarshalArguments))

	// Invalid path
	_, err = tool.Execute(ctx, json.RawMessage(`{"cert_path": "/etc/shadow"}`))
	assert.Error(t, err)
	// Note: validation.go likely rejects this
}

func TestTLSCertInspectTool_Execute_File(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	certPath := filepath.Join(tmpDir, "test.crt")
	certPEM, _ := generateTestCert(t)
	err = os.WriteFile(certPath, certPEM, 0644)
	require.NoError(t, err)

	tool := &TLSCertInspectTool{}
	args := json.RawMessage(fmt.Sprintf(`{"cert_path": %q}`, certPath))
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)

	var inspectResult certInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &inspectResult)
	require.NoError(t, err)
	assert.Equal(t, "test-cert", inspectResult.Subject)
}

func TestTLSCertInspectTool_Execute_Host(t *testing.T) {
	certPEM, keyPEM := generateTestCert(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				tlsConn := conn.(*tls.Conn)
				_ = tlsConn.Handshake()
				tlsConn.Close()
			}()
		}
	}()

	host, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	tool := &TLSCertInspectTool{}
	args := json.RawMessage(fmt.Sprintf(`{"host": %q, "port": %d}`, host, port))
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var inspectResult certInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &inspectResult)
	require.NoError(t, err)
	assert.Equal(t, "test-cert", inspectResult.Subject)
}

func generateTestCert(t *testing.T) ([]byte, []byte) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cert",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return certPEM, keyPEM
}
