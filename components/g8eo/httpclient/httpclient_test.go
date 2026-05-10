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

package httpclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/certs"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

func TestMain(m *testing.M) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "g8e Test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	certs.SetCA(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	os.Exit(m.Run())
}

func assertEmbeddedCATransport(t *testing.T, transport *http.Transport) {
	t.Helper()
	require.NotNil(t, transport.TLSClientConfig)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Empty(t, transport.TLSClientConfig.Certificates)
	require.Len(t, transport.TLSClientConfig.CurvePreferences, 3)
	assert.Equal(t, tls.X25519, transport.TLSClientConfig.CurvePreferences[0])
	assert.Equal(t, tls.CurveP384, transport.TLSClientConfig.CurvePreferences[1])
	assert.Equal(t, tls.CurveP256, transport.TLSClientConfig.CurvePreferences[2])
}

func assertBaseTransportTimeouts(t *testing.T, transport *http.Transport) {
	t.Helper()
	assert.Equal(t, DefaultTLSTimeout, transport.TLSHandshakeTimeout)
	assert.Equal(t, DefaultIdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, 10, transport.MaxIdleConns)
	assert.Equal(t, 5, transport.MaxIdleConnsPerHost)
	assert.NotNil(t, transport.DialContext)
}

func assertEmbeddedCADialer(t *testing.T, dialer *websocket.Dialer) {
	t.Helper()
	require.NotNil(t, dialer.TLSClientConfig)
	assert.NotNil(t, dialer.TLSClientConfig.RootCAs)
	assert.Equal(t, uint16(tls.VersionTLS13), dialer.TLSClientConfig.MinVersion)
	assert.False(t, dialer.TLSClientConfig.InsecureSkipVerify)
	assert.Empty(t, dialer.TLSClientConfig.Certificates)
	require.Len(t, dialer.TLSClientConfig.CurvePreferences, 3)
	assert.Equal(t, tls.X25519, dialer.TLSClientConfig.CurvePreferences[0])
	assert.Equal(t, tls.CurveP384, dialer.TLSClientConfig.CurvePreferences[1])
	assert.Equal(t, tls.CurveP256, dialer.TLSClientConfig.CurvePreferences[2])
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 30*time.Second, DefaultTimeout)
	assert.Equal(t, 10*time.Second, DefaultShortTimeout)
	assert.Equal(t, 10*time.Second, DefaultDialTimeout)
	assert.Equal(t, 10*time.Second, DefaultTLSTimeout)
	assert.Equal(t, 90*time.Second, DefaultIdleConnTimeout)
}

func TestNew(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, DefaultTimeout, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assertEmbeddedCATransport(t, transport)
	assertBaseTransportTimeouts(t, transport)
}

func TestNew_InvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	client, err := New()
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestNewWithTimeout(t *testing.T) {
	timeout := 5 * time.Second
	client, err := NewWithTimeout(timeout)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, timeout, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assertEmbeddedCATransport(t, transport)
	assertBaseTransportTimeouts(t, transport)
}

func TestNewWithTimeout_InvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	client, err := NewWithTimeout(5 * time.Second)
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestNewWithTLS(t *testing.T) {
	customTLS := &tls.Config{
		MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{
			{},
		},
	}

	client := NewWithTLS(customTLS)
	require.NotNil(t, client)

	assert.Equal(t, DefaultTimeout, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, customTLS, transport.TLSClientConfig)
	assertBaseTransportTimeouts(t, transport)
}

func TestWebSocketDialer(t *testing.T) {
	dialer, err := WebSocketDialer()
	require.NoError(t, err)
	require.NotNil(t, dialer)

	assert.Equal(t, DefaultTLSTimeout, dialer.HandshakeTimeout)
	assertEmbeddedCADialer(t, dialer)
}

func TestWebSocketDialer_InvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	dialer, err := WebSocketDialer()
	require.Error(t, err)
	assert.Nil(t, dialer)
}

func TestWebSocketDialerWithTLS(t *testing.T) {
	customTLS := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	dialer := WebSocketDialerWithTLS(customTLS)
	require.NotNil(t, dialer)

	assert.Equal(t, customTLS, dialer.TLSClientConfig)
	assert.Equal(t, DefaultTLSTimeout, dialer.HandshakeTimeout)
}

func TestNewWithServerName(t *testing.T) {
	client, err := NewWithServerName(constants.DefaultEndpoint)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, DefaultTimeout, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assertEmbeddedCATransport(t, transport)
	assertBaseTransportTimeouts(t, transport)
	assert.Equal(t, constants.DefaultEndpoint, transport.TLSClientConfig.ServerName)
}

func TestNewWithServerName_InvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	client, err := NewWithServerName(constants.DefaultEndpoint)
	require.Error(t, err)
	assert.Nil(t, client)
}

func TestWebSocketDialerWithServerName(t *testing.T) {
	dialer, err := WebSocketDialerWithServerName(constants.DefaultEndpoint)
	require.NoError(t, err)
	require.NotNil(t, dialer)

	assert.Equal(t, DefaultTLSTimeout, dialer.HandshakeTimeout)
	assertEmbeddedCADialer(t, dialer)
	assert.Equal(t, constants.DefaultEndpoint, dialer.TLSClientConfig.ServerName)
}

func TestWebSocketDialerWithServerName_InvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	dialer, err := WebSocketDialerWithServerName(constants.DefaultEndpoint)
	require.Error(t, err)
	assert.Nil(t, dialer)
}

func TestMustNew(t *testing.T) {
	client := MustNew()
	require.NotNil(t, client)
	assert.Equal(t, DefaultTimeout, client.Timeout)
}

func TestMustNew_PanicsOnInvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	assert.Panics(t, func() {
		MustNew()
	})
}

func TestMustWebSocketDialer(t *testing.T) {
	dialer := MustWebSocketDialer()
	require.NotNil(t, dialer)
	assert.Equal(t, DefaultTLSTimeout, dialer.HandshakeTimeout)
}

func TestMustWebSocketDialer_PanicsOnInvalidCert(t *testing.T) {
	old := certs.GetRawCA()
	t.Cleanup(func() { certs.SetCA(old) })
	certs.SetCA([]byte("not a valid pem block"))

	assert.Panics(t, func() {
		MustWebSocketDialer()
	})
}
