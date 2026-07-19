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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestBuildMTLSClient_MissingCertFile(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)

	_, err := BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
}

func TestBuildMTLSClient_InvalidCertPair(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte("not a cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte("not a key"), constants.PermFilePrivate))

	_, err = BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
}

func TestBuildMTLSClient_MissingTrustBundle(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)

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

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))

	_, err = BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
}

func TestBuildMTLSClient_InvalidTrustBundle(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)

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

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))

	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte("not a cert"), constants.PermFilePublic))

	_, err = BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCAParseFailed)
}

func TestBuildMTLSClient_ValidCertsAndBundle(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	caCertPEM, _ := generateTestCertificateWithSPIFFE(t, "test-ca", time.Now().Add(time.Hour))
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte(caCertPEM), constants.PermFilePublic))

	client, err := BuildMTLSClient(fileSvc, cfg, 30*time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	tlsConfig := transport.TLSClientConfig
	require.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.NotNil(t, tlsConfig.RootCAs)
	assert.Equal(t, uint16(0x0304), tlsConfig.MinVersion) // tls.VersionTLS13
	assert.Equal(t, 30*time.Second, client.Timeout)
}

func TestBuildMTLSClient_ZeroTimeoutForSSEStreaming(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	caCertPEM, _ := generateTestCertificateWithSPIFFE(t, "test-ca", time.Now().Add(time.Hour))
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte(caCertPEM), constants.PermFilePublic))

	client, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, time.Duration(0), client.Timeout)
}
