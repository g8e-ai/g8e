package certutil

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ParseCertFromPEM decodes PEM-encoded certificate data and returns the parsed
// x509 certificate. It validates that the PEM block is of type "CERTIFICATE".
func ParseCertFromPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, constants.ErrPEMDecodeFailed
	}

	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: type=%s", constants.ErrInvalidPEMType, block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCertParseFailed, err)
	}

	return cert, nil
}

// EncodeCertAndKey marshals an EC private key to PEM and combines a leaf
// certificate with an optional chain using the canonical "\n" separator.
// Returns the certificate bytes and key bytes. Storage is left to the caller.
func EncodeCertAndKey(certPEM, chainPEM string, key *ecdsa.PrivateKey) ([]byte, []byte, error) {
	if key == nil {
		return nil, nil, constants.ErrKeyParseFailed
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrKeyParseFailed, err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	certContent := certPEM
	if chainPEM != "" {
		certContent += "\n" + chainPEM
	}

	return []byte(certContent), keyPEM, nil
}
