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

// GenerateWindowsCSR generates a CSR with an ECDSA P-256 key on Windows.
// If useTPM is true, the caller requests TPM-backed key generation; TPM support
// is not yet implemented and a software-backed key is used in its place.
func GenerateWindowsCSR(commonName string, useTPM bool) (string, *ecdsa.PrivateKey, error) {
	if useTPM {
		return generateTPMBackedCSR(commonName)
	}
	return generateSoftwareBackedCSR(commonName)
}

// generateSoftwareBackedCSR generates a CSR with a software-backed ECDSA P-256 key.
func generateSoftwareBackedCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	return generateECDSACSR(commonName)
}

// generateTPMBackedCSR generates a CSR with a software-backed ECDSA P-256 key.
// TPM-backed key generation via Windows Hello for Business (CNG KSP) is not yet
// implemented; this function delegates to the same software-backed path.
func generateTPMBackedCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
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

	// Use PowerShell with .NET X509Store to import the certificate
	// This is more reliable than the Cert: drive which may not be available
	psScript := fmt.Sprintf(`
		$certPath = "%s"
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
	`, certFile)

	psCmd := exec.Command("powershell", "-Command", psScript)
	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %w, output: %s", constants.ErrWindowsPowerShellImport, err, string(output))
	}

	return nil
}

// TrustRootCAInWindowsStore imports the platform's Root CA from a PEM bundle into the Windows Trusted Root store.
func TrustRootCAInWindowsStore(caBundlePEM string) error {
	// Extract the first certificate from the bundle (the Root CA)
	block, _ := pem.Decode([]byte(caBundlePEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return constants.ErrPEMDecodeFailed
	}

	// Create a temporary file for the certificate
	tmpDir, err := os.MkdirTemp("", constants.WindowsTempCATrustPrefix)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrWindowsTempDirCreate, err)
	}
	defer os.RemoveAll(tmpDir)

	caFile := filepath.Join(tmpDir, constants.PkiFileRootCA)
	if err := os.WriteFile(caFile, pem.EncodeToMemory(block), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrWindowsCertWriteFailed, err)
	}

	// Use PowerShell to import to Trusted Root store
	// Requires Administrator privileges if not already trusted
	psScript := fmt.Sprintf(`
		$certPath = "%s"
		$store = New-Object System.Security.Cryptography.X509Certificates.X509Store("Root", "LocalMachine")
		try {
			$store.Open("ReadWrite")
		} catch {
			# Fall back to CurrentUser if LocalMachine fails (though Root is usually Machine)
			$store = New-Object System.Security.Cryptography.X509Certificates.X509Store("Root", "CurrentUser")
			$store.Open("ReadWrite")
		}
		
		$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($certPath)
		
		# Check if already exists by thumbprint
		$existing = $store.Certificates | Where-Object { $_.Thumbprint -eq $cert.Thumbprint }
		if (-not $existing) {
			$store.Add($cert)
			Write-Host "Root CA trusted successfully"
		} else {
			Write-Host "Root CA already trusted"
		}
		$store.Close()
	`, caFile)

	psCmd := exec.Command("powershell", "-Command", psScript)
	output, err := psCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %w, output: %s", constants.ErrWindowsPowerShellTrust, err, string(output))
	}

	return nil
}
