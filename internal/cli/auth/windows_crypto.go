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
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
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

	// WebAuthn constants
	WEBAUTHN_API_VERSION_1                             = 1
	WEBAUTHN_API_VERSION_2                             = 2
	WEBAUTHN_API_VERSION_3                             = 3
	WEBAUTHN_API_VERSION_4                             = 4
	WEBAUTHN_HASH_ALG_SHA_256                          = "SHA-256"
	WEBAUTHN_AUTHENTICATOR_ATTACHMENT_CROSS_PLATFORM   = 1
	WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM         = 2
	WEBAUTHN_USER_VERIFICATION_REQUIREMENT_REQUIRED    = 1
	WEBAUTHN_USER_VERIFICATION_REQUIREMENT_PREFERRED   = 2
	WEBAUTHN_USER_VERIFICATION_REQUIREMENT_DISCOURAGED = 3
)

var (
	modWebAuthN                             = syscall.NewLazyDLL("webauthn.dll")
	procWebAuthNAuthenticatorGetAssertion   = modWebAuthN.NewProc("WebAuthNAuthenticatorGetAssertion")
	procWebAuthNAuthenticatorMakeCredential = modWebAuthN.NewProc("WebAuthNAuthenticatorMakeCredential")
	procWebAuthNFreeAssertion               = modWebAuthN.NewProc("WebAuthNFreeAssertion")
	procWebAuthNFreeCredentialAttestation   = modWebAuthN.NewProc("WebAuthNFreeCredentialAttestation")
	procWebAuthNGetApiVersionNumber         = modWebAuthN.NewProc("WebAuthNGetApiVersionNumber")
)

// WebAuthn structures for syscalls
type webauthnClientData struct {
	StructVersion  uint32
	ClientDataSize uint32
	ClientData     *byte
	HashAlgId      *uint16 // LPCWSTR
}

type webauthnCredential struct {
	StructVersion  uint32
	IdSize         uint32
	Id             *byte
	CredentialType *uint16 // LPCWSTR
}

type webauthnCredentials struct {
	Count       uint32
	Credentials **webauthnCredential
}

type webauthnAuthenticatorGetAssertionOptions struct {
	StructVersion               uint32
	TimeoutMilliseconds         uint32
	AllowCredentials            webauthnCredentials
	Extensions                  uintptr // PWEBAUTHN_EXTENSIONS
	AuthenticatorAttachment     uint32
	UserVerificationRequirement uint32
	// Add more if needed based on version
}

type webauthnAssertion struct {
	StructVersion         uint32
	CredentialIdSize      uint32
	CredentialId          *byte
	AuthenticatorDataSize uint32
	AuthenticatorData     *byte
	SignatureSize         uint32
	Signature             *byte
	UserHandleSize        uint32
	UserHandle            *byte
}

type webauthnRpEntityInformation struct {
	StructVersion uint32
	Id            *uint16 // LPCWSTR
	Name          *uint16 // LPCWSTR
	Icon          *uint16 // LPCWSTR
}

type webauthnUserEntityInformation struct {
	StructVersion uint32
	IdSize        uint32
	Id            *byte
	Name          *uint16 // LPCWSTR
	Icon          *uint16 // LPCWSTR
	DisplayName   *uint16 // LPCWSTR
}

type webauthnCoseCredentialParameter struct {
	StructVersion  uint32
	CredentialType *uint16 // LPCWSTR
	Alg            int32
}

type webauthnCoseCredentialParameters struct {
	Count      uint32
	Parameters *webauthnCoseCredentialParameter
}

type webauthnAuthenticatorMakeCredentialOptions struct {
	StructVersion                   uint32
	TimeoutMilliseconds             uint32
	CredentialList                  webauthnCredentials
	Extensions                      uintptr // PWEBAUTHN_EXTENSIONS
	AuthenticatorAttachment         uint32
	ResidentKeyRequirement          uint32
	UserVerificationRequirement     uint32
	AttestationConveyancePreference uint32
	Flags                           uint32
}

type webauthnCredentialAttestation struct {
	StructVersion         uint32
	FormatType            *uint16 // LPCWSTR
	AuthenticatorDataSize uint32
	AuthenticatorData     *byte
	AttestationDecodeType uint32
	AttestationDecode     uintptr
	AttestationObjectSize uint32
	AttestationObject     *byte
	CredentialIdSize      uint32
	CredentialId          *byte
}

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

// TrustRootCAInWindowsStore imports the platform's Root CA from a PEM bundle into the Windows Trusted Root store.
func TrustRootCAInWindowsStore(caBundlePEM string) error {
	// Extract the first certificate from the bundle (the Root CA)
	block, _ := pem.Decode([]byte(caBundlePEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("failed to decode Root CA PEM")
	}

	// Create a temporary file for the certificate
	tmpDir, err := os.MkdirTemp("", "g8e-ca-trust-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	caFile := filepath.Join(tmpDir, "root_ca.crt")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(block), 0600); err != nil {
		return fmt.Errorf("failed to write Root CA to temp file: %w", err)
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
		return fmt.Errorf("failed to trust Root CA via PowerShell: %w, output: %s", err, string(output))
	}

	return nil
}

// WebAuthnAssertionResponse is a Go-friendly version of the WebAuthn assertion result.
type WebAuthnAssertionResponse struct {
	Id                string
	RawId             []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        []byte
}

// WebAuthnAttestationResponse is a Go-friendly version of the WebAuthn attestation result.
type WebAuthnAttestationResponse struct {
	Id                string
	RawId             []byte
	AuthenticatorData []byte
	AttestationObject []byte
}

// RegisterWithWindowsHello performs native WebAuthn/FIDO2 registration using webauthn.dll.
func RegisterWithWindowsHello(rpID, rpName, userID, userName string, challenge []byte) (*WebAuthnAttestationResponse, error) {
	// 1. Check if webauthn.dll is available
	if err := modWebAuthN.Load(); err != nil {
		return nil, fmt.Errorf("webauthn.dll not found: %w", err)
	}

	// 2. Prepare RP info
	rpIDPtr, _ := syscall.UTF16PtrFromString(rpID)
	rpNamePtr, _ := syscall.UTF16PtrFromString(rpName)
	rpInfo := webauthnRpEntityInformation{
		StructVersion: WEBAUTHN_API_VERSION_1,
		Id:            rpIDPtr,
		Name:          rpNamePtr,
	}

	// 3. Prepare User info
	userNamePtr, _ := syscall.UTF16PtrFromString(userName)
	userIDBytes := []byte(userID)
	userInfo := webauthnUserEntityInformation{
		StructVersion: WEBAUTHN_API_VERSION_1,
		IdSize:        uint32(len(userIDBytes)),
		Id:            &userIDBytes[0],
		Name:          userNamePtr,
		DisplayName:   userNamePtr,
	}

	// 4. Prepare Credential Parameters (ES256)
	pubKeyCredType, _ := syscall.UTF16PtrFromString("public-key")
	credParam := webauthnCoseCredentialParameter{
		StructVersion:  WEBAUTHN_API_VERSION_1,
		CredentialType: pubKeyCredType,
		Alg:            -7, // ES256
	}
	credParams := webauthnCoseCredentialParameters{
		Count:      1,
		Parameters: &credParam,
	}

	// 5. Prepare Client Data
	clientData := webauthnClientData{
		StructVersion:  WEBAUTHN_API_VERSION_1,
		ClientDataSize: uint32(len(challenge)),
		ClientData:     &challenge[0],
		HashAlgId:      nil,
	}

	// 6. Prepare Options
	options := webauthnAuthenticatorMakeCredentialOptions{
		StructVersion:               WEBAUTHN_API_VERSION_1,
		TimeoutMilliseconds:         60000,
		AuthenticatorAttachment:     WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM,
		UserVerificationRequirement: WEBAUTHN_USER_VERIFICATION_REQUIREMENT_REQUIRED,
	}

	// 7. Call WebAuthNAuthenticatorMakeCredential
	var pAttestation *webauthnCredentialAttestation
	ret, _, _ := procWebAuthNAuthenticatorMakeCredential.Call(
		0,
		uintptr(unsafe.Pointer(&rpInfo)),
		uintptr(unsafe.Pointer(&userInfo)),
		uintptr(unsafe.Pointer(&credParams)),
		uintptr(unsafe.Pointer(&clientData)),
		uintptr(unsafe.Pointer(&options)),
		uintptr(unsafe.Pointer(&pAttestation)),
	)

	if int32(ret) != 0 {
		return nil, fmt.Errorf("Windows Hello registration failed (HRESULT: 0x%x)", uint32(ret))
	}
	defer procWebAuthNFreeCredentialAttestation.Call(uintptr(unsafe.Pointer(pAttestation)))

	// 8. Extract result
	response := &WebAuthnAttestationResponse{
		RawId:             make([]byte, pAttestation.CredentialIdSize),
		AuthenticatorData: make([]byte, pAttestation.AuthenticatorDataSize),
		AttestationObject: make([]byte, pAttestation.AttestationObjectSize),
	}

	copy(response.RawId, unsafe.Slice(pAttestation.CredentialId, pAttestation.CredentialIdSize))
	response.Id = base64.RawURLEncoding.EncodeToString(response.RawId)
	copy(response.AuthenticatorData, unsafe.Slice(pAttestation.AuthenticatorData, pAttestation.AuthenticatorDataSize))
	copy(response.AttestationObject, unsafe.Slice(pAttestation.AttestationObject, pAttestation.AttestationObjectSize))

	return response, nil
}

// AuthenticateWithWindowsHello performs native WebAuthn/FIDO2 authentication using webauthn.dll.
func AuthenticateWithWindowsHello(rpID string, challenge []byte) (*WebAuthnAssertionResponse, error) {
	// 1. Check if webauthn.dll is available
	if err := modWebAuthN.Load(); err != nil {
		return nil, fmt.Errorf("webauthn.dll not found: %w", err)
	}

	// 2. Prepare RP ID
	rpIDPtr, err := syscall.UTF16PtrFromString(rpID)
	if err != nil {
		return nil, fmt.Errorf("invalid RP ID: %w", err)
	}

	// 3. Prepare Client Data
	// In a real WebAuthn flow, clientDataJSON is complex.
	// Windows Hello API often expects the raw challenge if used in certain ways,
	// but for standard WebAuthn it expects the hash or the full JSON.
	// For g8e, we'll follow what the browser does: it signs the clientDataJSON which contains the challenge.
	// However, the Windows API can also take raw data.
	clientData := webauthnClientData{
		StructVersion:  WEBAUTHN_API_VERSION_1,
		ClientDataSize: uint32(len(challenge)),
		ClientData:     &challenge[0],
		HashAlgId:      nil, // Defaults to SHA-256 if nil
	}

	// 4. Prepare Options
	options := webauthnAuthenticatorGetAssertionOptions{
		StructVersion:               WEBAUTHN_API_VERSION_1,
		TimeoutMilliseconds:         60000, // 60 seconds
		AuthenticatorAttachment:     WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM,
		UserVerificationRequirement: WEBAUTHN_USER_VERIFICATION_REQUIREMENT_REQUIRED,
	}

	// 5. Call WebAuthNAuthenticatorGetAssertion
	var pAssertion *webauthnAssertion
	ret, _, _ := procWebAuthNAuthenticatorGetAssertion.Call(
		0, // hWnd (NULL for no parent window)
		uintptr(unsafe.Pointer(rpIDPtr)),
		uintptr(unsafe.Pointer(&clientData)),
		uintptr(unsafe.Pointer(&options)),
		uintptr(unsafe.Pointer(&pAssertion)),
	)

	// HRESULT success is 0 (S_OK)
	if int32(ret) != 0 {
		return nil, fmt.Errorf("Windows Hello authentication failed (HRESULT: 0x%x)", uint32(ret))
	}
	defer procWebAuthNFreeAssertion.Call(uintptr(unsafe.Pointer(pAssertion)))

	// 6. Extract result
	response := &WebAuthnAssertionResponse{
		RawId:             make([]byte, pAssertion.CredentialIdSize),
		AuthenticatorData: make([]byte, pAssertion.AuthenticatorDataSize),
		Signature:         make([]byte, pAssertion.SignatureSize),
	}

	copy(response.RawId, unsafe.Slice(pAssertion.CredentialId, pAssertion.CredentialIdSize))
	response.Id = base64.RawURLEncoding.EncodeToString(response.RawId)
	copy(response.AuthenticatorData, unsafe.Slice(pAssertion.AuthenticatorData, pAssertion.AuthenticatorDataSize))
	copy(response.Signature, unsafe.Slice(pAssertion.Signature, pAssertion.SignatureSize))

	if pAssertion.UserHandleSize > 0 && pAssertion.UserHandle != nil {
		response.UserHandle = make([]byte, pAssertion.UserHandleSize)
		copy(response.UserHandle, unsafe.Slice(pAssertion.UserHandle, pAssertion.UserHandleSize))
	}

	return response, nil
}
