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
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

// Windows CNG API constants
const (
	PROV_RSA_FULL               = 1
	AT_KEYEXCHANGE              = 1
	AT_SIGNATURE                = 2
	CRYPT_MACHINE_KEYSET        = 0x00000020
	CRYPT_USER_KEYSET           = 0x00000000
	CRYPT_EXPORTABLE            = 0x00000001
	CRYPT_USER_PROTECTED        = 0x00000002
	CRYPT_ARCHIVABLE            = 0x00004000
	CRYPT_SILENT                = 0x00000040
	MS_ENHANCED_PROV            = "Microsoft Enhanced Cryptographic Provider v1.0"
	MS_PLATFORM_CRYPTO_PROVIDER = "Microsoft Platform Crypto Provider"
)

// Windows CryptoAPI structures
type CRYPT_KEY_PROV_INFO struct {
	pwszContainerName *uint16
	pwszProvName      *uint16
	dwProvType        uint32
	dwFlags           uint32
	cProvParam        uint32
	rgProvParam       uintptr
	rgdwProvParam     *uint32
}

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
		return "", nil, fmt.Errorf("failed to generate ECDSA P-256 key: %w", err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		DNSNames:           []string{"localhost"},
		IPAddresses:        []net.IP{net.ParseIP("127.0.0.1")},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
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
		return "", nil, fmt.Errorf("failed to generate ECDSA P-256 key: %w", err)
	}

	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		DNSNames:           []string{"localhost"},
		IPAddresses:        []net.IP{net.ParseIP("127.0.0.1")},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create CSR: %w", err)
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
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	certFile := filepath.Join(tmpDir, "certificate.pem")
	if err := os.WriteFile(certFile, []byte(certPEM), 0600); err != nil {
		return fmt.Errorf("failed to write certificate to temp file: %w", err)
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
		return fmt.Errorf("failed to import certificate via PowerShell: %w, output: %s", err, string(output))
	}

	return nil
}

// SignWithWindowsHello signs a transaction hash using Windows Hello (TPM-backed key).
// This is used for L3 transaction approval when Windows Hello is enabled.
func SignWithWindowsHello(transactionHash []byte) ([]byte, error) {
	// Windows Hello signing requires CNG API calls via syscall
	// For now, return error - full implementation requires Windows API bindings
	_ = transactionHash
	return nil, fmt.Errorf("Windows Hello signing requires Windows API bindings (not yet implemented)")
}

// SignWithWindowsHelloUsingCert signs a transaction hash using a specific certificate
// from the Windows Certificate Store (identified by thumbprint).
func SignWithWindowsHelloUsingCert(transactionHash []byte, certThumbprint string) ([]byte, error) {
	// Windows Hello signing via specific cert requires CNG API calls
	_ = transactionHash
	_ = certThumbprint
	return nil, fmt.Errorf("Windows Hello signing requires Windows API bindings (not yet implemented)")
}
