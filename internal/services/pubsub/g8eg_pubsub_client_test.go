// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber:          serial,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func newTestCertsTLSConfig(t *testing.T) *certs.TLSConfig {
	t.Helper()
	trustStore := certs.NewTrustStore(generateTestCAPEM(t))
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	return certs.NewTLSConfig(trustStore, clientIdentity)
}

func newTestCertsTLSConfigForServer(t *testing.T, server *httptest.Server) *certs.TLSConfig {
	t.Helper()
	serverCert := server.Certificate()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw})
	trustStore := certs.NewTrustStore(caPEM)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	return certs.NewTLSConfig(trustStore, clientIdentity)
}

func newTestRawTLSConfigForServer(t *testing.T, server *httptest.Server) *tls.Config {
	t.Helper()
	certsTLSConfig := newTestCertsTLSConfigForServer(t, server)
	tlsCfg, err := certsTLSConfig.GetTLSConfig()
	require.NoError(t, err)
	return tlsCfg
}

func httpsToWss(url string) string {
	return strings.Replace(url, "https://", "wss://", 1)
}

func TestNewOperatorPubSubClient(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("rejects empty baseURL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("", "", logger, tlsCfg)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Error(t, err)
	})

	t.Run("rejects ws:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("ws://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("rejects nil certsTLSConfig", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, nil)
		require.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("accepts wss:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), client.baseURL)
		assert.NotNil(t, client.tlsConfig)
	})

	t.Run("sets serverName for TLS SNI override", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://192.168.1.1:%d", constants.Ports.OperatorHttp), "gateway.local", logger, tlsCfg)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "gateway.local", client.serverName)
		assert.Equal(t, "gateway.local", client.tlsConfig.ServerName)
	})
}

func TestPubSubWSURL(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("returns correct URL for wss://", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("wss://localhost:%d/api/v1/pubsub/stream", constants.Ports.OperatorHttp), client.pubSubWSURL())
	})
}
