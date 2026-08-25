package certutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func mustGenerateECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func mustGenerateCertPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
}

func mustGenerateChainPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "chain-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
}

func TestEncodeCertAndKey_LeafWithoutChain(t *testing.T) {
	key := mustGenerateECKey(t)
	certPEM := mustGenerateCertPEM(t, key)

	certBytes, keyBytes, err := EncodeCertAndKey(certPEM, "", key)
	require.NoError(t, err)

	assert.Equal(t, certPEM, string(certBytes))
	assert.Contains(t, string(keyBytes), "EC PRIVATE KEY")

	block, _ := pem.Decode(keyBytes)
	require.NotNil(t, block)
	assert.Equal(t, "EC PRIVATE KEY", block.Type)

	parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)
	assert.True(t, key.Equal(parsedKey))
}

func TestEncodeCertAndKey_LeafWithChain(t *testing.T) {
	key := mustGenerateECKey(t)
	certPEM := mustGenerateCertPEM(t, key)
	chainPEM := mustGenerateChainPEM(t, key)

	certBytes, keyBytes, err := EncodeCertAndKey(certPEM, chainPEM, key)
	require.NoError(t, err)

	assert.Equal(t, certPEM+"\n"+chainPEM, string(certBytes))
	assert.Contains(t, string(keyBytes), "EC PRIVATE KEY")
}

func TestEncodeCertAndKey_ValidECKeyPEMRoundTrip(t *testing.T) {
	key := mustGenerateECKey(t)
	certPEM := mustGenerateCertPEM(t, key)

	_, keyBytes, err := EncodeCertAndKey(certPEM, "", key)
	require.NoError(t, err)

	block, _ := pem.Decode(keyBytes)
	require.NotNil(t, block)

	parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err)
	assert.True(t, key.Equal(parsedKey))
}

func TestParseCertFromPEM_ValidCertificate(t *testing.T) {
	key := mustGenerateECKey(t)
	certPEM := mustGenerateCertPEM(t, key)

	cert, err := ParseCertFromPEM([]byte(certPEM))
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, "test", cert.Subject.CommonName)
	assert.Equal(t, big.NewInt(1), cert.SerialNumber)
}

func TestParseCertFromPEM_InvalidPEMReturnsErrPEMDecodeFailed(t *testing.T) {
	_, err := ParseCertFromPEM([]byte("not a pem block at all"))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPEMDecodeFailed)
}

func TestParseCertFromPEM_WrongPEMTypeReturnsErrInvalidPEMType(t *testing.T) {
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("dummy"),
	})

	_, err := ParseCertFromPEM(keyPEM)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidPEMType)
}

func TestParseCertFromPEM_MalformedCertBytesReturnsErrCertParseFailed(t *testing.T) {
	badCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a real certificate"),
	})

	_, err := ParseCertFromPEM(badCertPEM)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCertParseFailed)
}

func TestEncodeCertAndKey_NilKeyReturnsError(t *testing.T) {
	_, _, err := EncodeCertAndKey("cert", "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyParseFailed)
}

func TestEncodeCertAndKey_SeparatorNoDuplicateNewline(t *testing.T) {
	key := mustGenerateECKey(t)
	certPEM := mustGenerateCertPEM(t, key)
	chainPEM := mustGenerateChainPEM(t, key)

	certBytes, _, err := EncodeCertAndKey(certPEM, chainPEM, key)
	require.NoError(t, err)

	content := string(certBytes)

	assert.True(t, strings.HasPrefix(content, "-----BEGIN CERTIFICATE-----"))
	assert.True(t, strings.Contains(content, "-----END CERTIFICATE-----\n\n-----BEGIN CERTIFICATE-----"))

	assert.False(t, strings.Contains(content, "-----END CERTIFICATE-----\n\n\n"), "no triple newline")
}
