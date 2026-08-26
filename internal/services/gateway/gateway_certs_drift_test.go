// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestDetectServiceCertDrift(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(100),
		Subject: pkix.Name{
			CommonName: "operator-gateway",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		DNSNames:              []string{"localhost", "g8e.local", "operator", "host1.example.com"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
	}

	t.Run("Returns expected when cert is empty", func(t *testing.T) {
		emptyCert := tls.Certificate{}
		expDNS := []string{"dev.g8e.local"}
		expIPs := []net.IP{net.ParseIP("192.168.1.62")}

		missingDNS, missingIPs := detectServiceCertDrift(emptyCert, expIPs, expDNS)
		assert.Equal(t, expDNS, missingDNS)
		assert.Equal(t, expIPs, missingIPs)
	})

	t.Run("Returns nil when all expected SANs are present", func(t *testing.T) {
		expDNS := []string{"localhost", "g8e.local", "host1.example.com"}
		expIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")}

		missingDNS, missingIPs := detectServiceCertDrift(tlsCert, expIPs, expDNS)
		assert.Empty(t, missingDNS)
		assert.Empty(t, missingIPs)
	})

	t.Run("Performs case-insensitive DNS name matching", func(t *testing.T) {
		expDNS := []string{"LOCALHOST", "G8E.LOCAL", "HOST1.EXAMPLE.COM"}
		expIPs := []net.IP{net.ParseIP("127.0.0.1")}

		missingDNS, missingIPs := detectServiceCertDrift(tlsCert, expIPs, expDNS)
		assert.Empty(t, missingDNS)
		assert.Empty(t, missingIPs)
	})

	t.Run("Detects missing DNS names", func(t *testing.T) {
		expDNS := []string{"localhost", "dev.g8e.local", "extra.domain"}
		expIPs := []net.IP{net.ParseIP("127.0.0.1")}

		missingDNS, missingIPs := detectServiceCertDrift(tlsCert, expIPs, expDNS)
		assert.Equal(t, []string{"dev.g8e.local", "extra.domain"}, missingDNS)
		assert.Empty(t, missingIPs)
	})

	t.Run("Detects missing IP addresses", func(t *testing.T) {
		expDNS := []string{"localhost"}
		expIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.1.62")}

		missingDNS, missingIPs := detectServiceCertDrift(tlsCert, expIPs, expDNS)
		assert.Empty(t, missingDNS)
		require.Len(t, missingIPs, 1)
		assert.True(t, missingIPs[0].Equal(net.ParseIP("192.168.1.62")))
	})

	t.Run("Ignores transient removal of SANs from expected list", func(t *testing.T) {
		// Cert contains host1.example.com and 10.0.0.1, but expected list does not
		expDNS := []string{"localhost"}
		expIPs := []net.IP{net.ParseIP("127.0.0.1")}

		missingDNS, missingIPs := detectServiceCertDrift(tlsCert, expIPs, expDNS)
		assert.Empty(t, missingDNS)
		assert.Empty(t, missingIPs)
	})
}

func TestPKIAuthority_InitializePKIWithNames_RegeneratesOnDNSNameDrift(t *testing.T) {
	ctx := setupTestPKI(t)
	certRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)

	initialCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	initialSerial := initialCert.SerialNumber

	err := ctx.pki.InitializePKIWithNames(nil, []string{"dev.g8e.local", "dev"})
	require.NoError(t, err)

	updatedCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	assert.NotEqual(t, initialSerial, updatedCert.SerialNumber, "service certificate must regenerate when new DNS names are detected")
	assert.Contains(t, updatedCert.DNSNames, "dev.g8e.local")
	assert.Contains(t, updatedCert.DNSNames, "dev")
}

func TestPKIAuthority_InitializePKIWithNames_RegeneratesOnIPDrift(t *testing.T) {
	ctx := setupTestPKI(t)
	certRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)

	initialCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	initialSerial := initialCert.SerialNumber

	err := ctx.pki.InitializePKIWithNames([]net.IP{net.ParseIP("192.168.1.62")}, nil)
	require.NoError(t, err)

	updatedCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	assert.NotEqual(t, initialSerial, updatedCert.SerialNumber, "service certificate must regenerate when new IP addresses are detected")

	foundIP := false
	for _, ip := range updatedCert.IPAddresses {
		if ip.Equal(net.ParseIP("192.168.1.62")) {
			foundIP = true
			break
		}
	}
	assert.True(t, foundIP, "regenerated certificate must contain the new IP address")
}

func TestPKIAuthority_InitializePKIWithNames_NoRegenWhenSANsMatch(t *testing.T) {
	ctx := setupTestPKI(t)
	certRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)

	extraIPs := []net.IP{net.ParseIP("192.168.1.62")}
	extraDNS := []string{"dev.g8e.local"}

	err := ctx.pki.InitializePKIWithNames(extraIPs, extraDNS)
	require.NoError(t, err)

	firstCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	firstSerial := firstCert.SerialNumber

	err = ctx.pki.InitializePKIWithNames(extraIPs, extraDNS)
	require.NoError(t, err)

	secondCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	assert.Equal(t, firstSerial, secondCert.SerialNumber, "service certificate must not regenerate when SANs match")
}

func TestPKIAuthority_InitializePKIWithNames_NoRegenOnTransientRemoval(t *testing.T) {
	ctx := setupTestPKI(t)
	certRelPath := filepath.Join(constants.PkiDirname, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)

	extraIPs := []net.IP{net.ParseIP("192.168.1.62")}
	extraDNS := []string{"dev.g8e.local"}

	err := ctx.pki.InitializePKIWithNames(extraIPs, extraDNS)
	require.NoError(t, err)

	firstCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	firstSerial := firstCert.SerialNumber

	// Re-initialize with nil/empty (simulating transient network drop)
	err = ctx.pki.InitializePKIWithNames(nil, nil)
	require.NoError(t, err)

	secondCert := loadCertificate(t, ctx.fileSvc, certRelPath)
	assert.Equal(t, firstSerial, secondCert.SerialNumber, "service certificate must not regenerate on transient SAN removal")
	assert.Contains(t, secondCert.DNSNames, "dev.g8e.local", "existing SANs must be retained")
}
