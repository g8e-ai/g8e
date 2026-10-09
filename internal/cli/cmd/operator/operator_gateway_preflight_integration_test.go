// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package operatorcmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// startPreflightGateway serves a trust bundle on plain HTTP and a TLS listener
// whose certificate names dnsName, and returns both ports.
func startPreflightGateway(t *testing.T, dnsName string) (httpPort, httpsPort int) {
	t.Helper()
	caKey, ca := testutil.GenerateTestCAWithKey(t, "preflight-root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	bundle := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})...)

	discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPaths.WellKnownPKICABundle {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(bundle)
	}))
	t.Cleanup(discovery.Close)

	public := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	public.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}}
	public.StartTLS()
	t.Cleanup(public.Close)

	return listenerPort(t, discovery.Listener), listenerPort(t, public.Listener)
}

func listenerPort(t *testing.T, l net.Listener) int {
	t.Helper()
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	n, err := strconv.Atoi(port)
	require.NoError(t, err)
	return n
}

func TestGatewayPreflightVerifiesDiscoveryAndTLSWithTheGatewayIdentity(t *testing.T) {
	httpPort, httpsPort := startPreflightGateway(t, constants.GatewayInternalHostname)
	require.NoError(t, runGatewayPreflight(context.Background(), constants.LocalhostIP, httpPort, httpsPort))
}

func TestGatewayPreflightVerifiesAHostnameEndpointAgainstThatHostname(t *testing.T) {
	httpPort, httpsPort := startPreflightGateway(t, "localhost")
	require.NoError(t, runGatewayPreflight(context.Background(), "localhost", httpPort, httpsPort))
}

func TestGatewayPreflightRejectsACertificateForAnotherIdentity(t *testing.T) {
	httpPort, httpsPort := startPreflightGateway(t, "not-the-gateway.example")
	err := runGatewayPreflight(context.Background(), constants.LocalhostIP, httpPort, httpsPort)
	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
}

func TestGatewayPreflightFailsWhenDiscoveryIsUnreachable(t *testing.T) {
	_, httpsPort := startPreflightGateway(t, constants.GatewayInternalHostname)
	closed, err := net.Listen("tcp", net.JoinHostPort(constants.LocalhostIP, "0"))
	require.NoError(t, err)
	unusedPort := listenerPort(t, closed)
	require.NoError(t, closed.Close())

	err = runGatewayPreflight(context.Background(), constants.LocalhostIP, unusedPort, httpsPort)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trust discovery")
}
