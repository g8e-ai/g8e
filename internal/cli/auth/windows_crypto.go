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

//go:build windows
// +build windows

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
)

// envCertPath passes the certificate file path to PowerShell scripts via an
// environment variable, avoiding command injection from string interpolation.
const envCertPath = "G8E_CERT_PATH"

// GenerateWindowsCSR generates a CSR with an ECDSA P-256 key on Windows.
// If useTPM is true, the caller requests TPM-backed key generation; TPM support
// is not yet implemented and a software-backed key is used in its place.
func GenerateWindowsCSR(commonName string, useTPM bool) (string, *ecdsa.PrivateKey, error) {
	return generateECDSACSR(commonName)
}

// generateECDSACSR generates a CSR with an ECDSA P-256 key, Client Auth + Server Auth EKU.
func generateECDSACSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		DNSNames:           []string{"localhost", "g8e.local"},
		ExtraExtensions: []pkix.Extension{
			{
				Id:       []int{2, 5, 29, 37},
				Critical: false,
				Value:    []byte{0x30, 0x14, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x02, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01},
			},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

// ImportCertificateToWindowsStore imports a signed certificate into the Windows Personal store.
func ImportCertificateToWindowsStore(certPEM string) error {
	// Create a temporary file for the certificate
	tmpDir, err := os.MkdirTemp("", constants.WindowsTempCertImportPrefix)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrWindowsTempDirCreate, err)
	}
	defer os.RemoveAll(tmpDir)

	certFile := filepath.Join(tmpDir, constants.WindowsTempCertFilename)
	if err := os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrWindowsCertWriteFailed, err)
	}

	// Use PowerShell with .NET X509Store to import the certificate.
	// The cert path is passed via environment variable to prevent command injection.
	psScript := `
		$certPath = $env:G8E_CERT_PATH
		$store = New-Object System.Security.Cryptography.X509Certificates.X509Store("My", "CurrentUser")
		$store.Open("ReadWrite")
		
		# Remove existing g8e certificates
		$certs = $store.Certificates
		foreach ($cert in $certs) {
			if ($cert.Subject -like "*g8e*") {
				$store.Remove($cert)
			}
		}
		
		# Import new certificate
		$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($certPath)
		$store.Add($cert)
		$store.Close()
	`

	psCmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	psCmd.Env = append(os.Environ(), envCertPath+"="+certFile)
	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %w, output: %s", constants.ErrWindowsPowerShellImport, err, string(output))
	}

	return nil
}
