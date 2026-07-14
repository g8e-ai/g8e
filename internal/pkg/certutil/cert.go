package certutil

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
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
