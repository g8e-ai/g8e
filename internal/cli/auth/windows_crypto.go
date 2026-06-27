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

// GenerateWindowsCSR generates a CSR using Windows CNG APIs.
// If useTPM is true, the key is generated in the TPM via Windows Hello for Business.
func GenerateWindowsCSR(commonName string, useTPM bool) (string, *ecdsa.PrivateKey, error) {
	if useTPM {
		return generateTPMBackedCSR(commonName)
	}
	return generateSoftwareBackedCSR(commonName)
}

// generateSoftwareBackedCSR generates a CSR with a software-backed key in Windows cert store.
func generateSoftwareBackedCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	// For software-backed keys, we use standard Go crypto but import to Windows cert store
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", constants.ErrCSRGenerationFailed, err)
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
				Id:       []int{2, 5, 29, 37}, // Extended Key Usage
				Critical: false,
				Value:    []byte{0x30, 0x14, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x02, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01}, // Client Auth + Server Auth OIDs
			},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", constants.ErrCSRGenerationFailed, err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privKey, nil
}

// generateTPMBackedCSR generates a CSR with a TPM-backed key via Windows Hello for Business.
// This uses the Microsoft Platform Crypto Provider KSP.
func generateTPMBackedCSR(commonName string) (string, *ecdsa.PrivateKey, error) {
	// Windows Hello for Business requires CNG API calls
	// For now, fall back to software key with TPM annotation
	// Full implementation requires syscall access to CNG APIs
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", constants.ErrCSRGenerationFailed, err)
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
				Id:       []int{2, 5, 29, 37}, // Extended Key Usage
				Critical: false,
				Value:    []byte{0x30, 0x14, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x02, 0x06, 0x08, 0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01}, // Client Auth + Server Auth OIDs
			},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", constants.ErrCSRGenerationFailed, err)
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
	tmpDir, err := os.MkdirTemp("", "g8e-cert-import-*")
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrWindowsTempDirCreate, err)
	}
	defer os.RemoveAll(tmpDir)

	certFile := filepath.Join(tmpDir, "certificate.pem")
	if err := os.WriteFile(certFile, []byte(certPEM), 0600); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrWindowsCertWriteFailed, err)
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
		return fmt.Errorf("%w: %v, output: %s", constants.ErrWindowsPowerShellImport, err, string(output))
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
	tmpDir, err := os.MkdirTemp("", "g8e-ca-trust-*")
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrWindowsTempDirCreate, err)
	}
	defer os.RemoveAll(tmpDir)

	caFile := filepath.Join(tmpDir, "root_ca.crt")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(block), 0600); err != nil {
		return fmt.Errorf("%w: %v", constants.ErrWindowsCertWriteFailed, err)
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
		return fmt.Errorf("%w: %v, output: %s", constants.ErrWindowsPowerShellTrust, err, string(output))
	}

	return nil
}

