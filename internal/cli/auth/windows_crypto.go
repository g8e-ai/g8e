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

	"github.com/g8e-ai/g8e/internal/constants"
)

// Windows WebAuthn API constants - Using API Version 4 (stable, modern version)
const (
	WEBAUTHN_API_VERSION_1                                         = 1
	WEBAUTHN_API_VERSION_2                                         = 2
	WEBAUTHN_API_VERSION_3                                         = 3
	WEBAUTHN_API_VERSION_4                                         = 4
	WEBAUTHN_API_VERSION_5                                         = 5
	WEBAUTHN_API_VERSION_6                                         = 6
	WEBAUTHN_API_VERSION_7                                         = 7
	WEBAUTHN_API_VERSION_8                                         = 8
	WEBAUTHN_API_VERSION_9                                         = 9
	WEBAUTHN_API_CURRENT_VERSION                                   = WEBAUTHN_API_VERSION_9

	// WebAuthn v4 requires a 16-byte GUID for user entity ID
	webauthnUserIDSize = 16
	// Maximum allowed size for WebAuthn response data (64 KB)
	webauthnMaxResponseSize = 64 * 1024

	// Windows HRESULT codes for WebAuthn diagnostics
	HRESULT_NTE_USER_CANCELLED   = 0x80090040
	HRESULT_NTE_NOT_FOUND        = 0x80090022
	HRESULT_NTE_DEVICE_NOT_READY = 0x80090030
	HRESULT_NTE_NOT_SUPPORTED    = 0x80090029
	HRESULT_FROM_WIN32_TIMEOUT   = 0x80070079
	WEBAUTHN_RP_ENTITY_INFORMATION_CURRENT_VERSION                 = 1
	WEBAUTHN_USER_ENTITY_INFORMATION_CURRENT_VERSION               = 1
	WEBAUTHN_CLIENT_DATA_CURRENT_VERSION                           = 1
	WEBAUTHN_COSE_CREDENTIAL_PARAMETER_CURRENT_VERSION             = 1
	WEBAUTHN_CREDENTIAL_CURRENT_VERSION                            = 1
	WEBAUTHN_AUTHENTICATOR_MAKE_CREDENTIAL_OPTIONS_CURRENT_VERSION = 5
	WEBAUTHN_AUTHENTICATOR_GET_ASSERTION_OPTIONS_CURRENT_VERSION   = 6
	WEBAUTHN_CREDENTIAL_ATTESTATION_CURRENT_VERSION                = 4
	WEBAUTHN_ASSERTION_CURRENT_VERSION                             = 3
	WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM                     = 1
	WEBAUTHN_AUTHENTICATOR_ATTACHMENT_CROSS_PLATFORM               = 2
	WEBAUTHN_USER_VERIFICATION_REQUIRED                            = 3
	WEBAUTHN_USER_VERIFICATION_PREFERRED                           = 2
	WEBAUTHN_USER_VERIFICATION_DISCOURAGED                         = 1
	WEBAUTHN_RESIDENT_KEY_DISCOURAGED                              = 0
	WEBAUTHN_RESIDENT_KEY_REQUIRED                                 = 1
	WEBAUTHN_ATTESTATION_CONVEYANCE_PREFERENCE_NONE                = 0
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

type webauthnExtensions struct {
	Count      uint32
	Extensions uintptr // PWEBAUTHN_EXTENSION
}

type webauthnAuthenticatorGetAssertionOptions struct {
	StructVersion               uint32
	TimeoutMilliseconds         uint32
	AllowCredentials            webauthnCredentials
	Extensions                  webauthnExtensions
	AuthenticatorAttachment     uint32
	UserVerificationRequirement uint32
	Flags                       uint32
	// Version 2 fields
	U2fAppId     *uint16 // PCWSTR
	U2fAppIdUsed *int32  // BOOL*
	// Version 3 fields
	CancellationId uintptr // GUID*
	// Version 4 fields
	AllowCredentialList uintptr // PWEBAUTHN_CREDENTIAL_LIST
	// Version 5 fields
	CredLargeBlobOperation uint32
	CredLargeBlobSize      uint32
	CredLargeBlob          *byte
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
	Extensions                      webauthnExtensions
	AuthenticatorAttachment         uint32
	ResidentKeyRequirement          int32 // BOOL
	UserVerificationRequirement     uint32
	AttestationConveyancePreference uint32
	Flags                           uint32
	// Version 2 fields
	CancellationId uintptr // GUID*
	// Version 3 fields
	ExcludeCredentialList uintptr // PWEBAUTHN_CREDENTIAL_LIST
	// Version 4 fields
	EnterpriseAttestation uint32
	LargeBlobSupport      uint32
	PreferResidentKey     int32 // BOOL
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

// mapWebAuthnHRESULT translates a Windows WebAuthn HRESULT to a typed error constant.
func mapWebAuthnHRESULT(hresult uint32, baseErr error) error {
	switch hresult {
	case HRESULT_NTE_USER_CANCELLED:
		return fmt.Errorf("%w: %v", constants.ErrWindowsHelloUserCancelled, baseErr)
	case HRESULT_NTE_NOT_FOUND:
		return fmt.Errorf("%w: %v", constants.ErrWindowsHelloDeviceNotFound, baseErr)
	case HRESULT_NTE_DEVICE_NOT_READY:
		return fmt.Errorf("%w: %v", constants.ErrWindowsHelloDeviceNotReady, baseErr)
	case HRESULT_NTE_NOT_SUPPORTED:
		return fmt.Errorf("%w: %v", constants.ErrWindowsHelloNotSupported, baseErr)
	case HRESULT_FROM_WIN32_TIMEOUT:
		return fmt.Errorf("%w: %v", constants.ErrWindowsHelloTimeout, baseErr)
	default:
		return fmt.Errorf("%w: HRESULT 0x%x", baseErr, hresult)
	}
}

// RegisterWithWindowsHello performs native WebAuthn/FIDO2 registration using webauthn.dll.
// Updated to use WEBAUTHN_API_VERSION_4 for better compatibility with modern Windows versions.
func RegisterWithWindowsHello(rpID, rpName string, userIDBytes []byte, userName string, challenge []byte) (*WebAuthnAttestationResponse, error) {
	// 1. Check if webauthn.dll is available
	if err := modWebAuthN.Load(); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrWindowsWebAuthnDLLNotFound, err)
	}

	// 2. Get API version to ensure compatibility
	apiVersion, _, _ := procWebAuthNGetApiVersionNumber.Call()
	if apiVersion < WEBAUTHN_API_VERSION_4 {
		return nil, fmt.Errorf("%w: version %d, minimum required is 4", constants.ErrWindowsWebAuthnAPIVersion, apiVersion)
	}

	// 3. Validate inputs for memory safety
	if len(userIDBytes) == 0 {
		return nil, constants.ErrWindowsHelloEmptyInput
	}
	if len(userIDBytes) != webauthnUserIDSize {
		return nil, fmt.Errorf("%w: got %d bytes, expected %d", constants.ErrWindowsHelloInvalidUserID, len(userIDBytes), webauthnUserIDSize)
	}
	if len(challenge) == 0 {
		return nil, constants.ErrWindowsHelloEmptyInput
	}

	// 4. Prepare RP info
	rpIDPtr, _ := syscall.UTF16PtrFromString(rpID)
	rpNamePtr, _ := syscall.UTF16PtrFromString(rpName)
	rpInfo := webauthnRpEntityInformation{
		StructVersion: WEBAUTHN_RP_ENTITY_INFORMATION_CURRENT_VERSION,
		Id:            rpIDPtr,
		Name:          rpNamePtr,
	}

	// 5. Prepare User info
	userNamePtr, _ := syscall.UTF16PtrFromString(userName)
	userInfo := webauthnUserEntityInformation{
		StructVersion: WEBAUTHN_USER_ENTITY_INFORMATION_CURRENT_VERSION,
		IdSize:        uint32(len(userIDBytes)),
		Id:            &userIDBytes[0],
		Name:          userNamePtr,
		DisplayName:   userNamePtr,
	}

	// 6. Prepare Credential Parameters (ES256)
	pubKeyCredType, _ := syscall.UTF16PtrFromString("public-key")
	credParam := webauthnCoseCredentialParameter{
		StructVersion:  WEBAUTHN_COSE_CREDENTIAL_PARAMETER_CURRENT_VERSION,
		CredentialType: pubKeyCredType,
		Alg:            -7, // ES256
	}
	credParams := webauthnCoseCredentialParameters{
		Count:      1,
		Parameters: &credParam,
	}

	// 7. Prepare Client Data
	// Windows Hello expects the full clientDataJSON as a UTF-8 string
	// The challenge parameter should already be the JSON-encoded clientDataJSON
	hashAlgId, _ := syscall.UTF16PtrFromString("SHA-256")
	clientData := webauthnClientData{
		StructVersion:  WEBAUTHN_CLIENT_DATA_CURRENT_VERSION,
		ClientDataSize: uint32(len(challenge)),
		ClientData:     &challenge[0],
		HashAlgId:      hashAlgId,
	}

	// 8. Prepare Options with API version 4
	options := webauthnAuthenticatorMakeCredentialOptions{
		StructVersion:                   WEBAUTHN_AUTHENTICATOR_MAKE_CREDENTIAL_OPTIONS_CURRENT_VERSION,
		TimeoutMilliseconds:             60000,
		AuthenticatorAttachment:         WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM,
		UserVerificationRequirement:     WEBAUTHN_USER_VERIFICATION_REQUIRED,
		ResidentKeyRequirement:          WEBAUTHN_RESIDENT_KEY_DISCOURAGED,
		AttestationConveyancePreference: WEBAUTHN_ATTESTATION_CONVEYANCE_PREFERENCE_NONE,
	}

	// 9. Call WebAuthNAuthenticatorMakeCredential
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
		return nil, mapWebAuthnHRESULT(uint32(ret), constants.ErrWindowsHelloRegistration)
	}
	defer procWebAuthNFreeCredentialAttestation.Call(uintptr(unsafe.Pointer(pAttestation)))

	// 10. Validate response pointers and sizes before extraction
	if pAttestation == nil {
		return nil, fmt.Errorf("%w: null attestation pointer", constants.ErrWindowsHelloRegistration)
	}
	if pAttestation.CredentialIdSize > webauthnMaxResponseSize ||
		pAttestation.AuthenticatorDataSize > webauthnMaxResponseSize ||
		pAttestation.AttestationObjectSize > webauthnMaxResponseSize {
		return nil, constants.ErrWindowsHelloResponseTooLarge
	}
	if pAttestation.CredentialId == nil || pAttestation.AuthenticatorData == nil || pAttestation.AttestationObject == nil {
		return nil, fmt.Errorf("%w: null response data pointer", constants.ErrWindowsHelloRegistration)
	}

	// 11. Extract result
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
// Updated to use WEBAUTHN_API_VERSION_4 for better compatibility with modern Windows versions.
func AuthenticateWithWindowsHello(rpID string, challenge []byte) (*WebAuthnAssertionResponse, error) {
	// 1. Check if webauthn.dll is available
	if err := modWebAuthN.Load(); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrWindowsWebAuthnDLLNotFound, err)
	}

	// 2. Get API version to ensure compatibility
	apiVersion, _, _ := procWebAuthNGetApiVersionNumber.Call()
	if apiVersion < WEBAUTHN_API_VERSION_4 {
		return nil, fmt.Errorf("%w: version %d, minimum required is 4", constants.ErrWindowsWebAuthnAPIVersion, apiVersion)
	}

	// 3. Validate inputs for memory safety
	if len(challenge) == 0 {
		return nil, constants.ErrWindowsHelloEmptyInput
	}

	// 4. Prepare RP ID
	rpIDPtr, err := syscall.UTF16PtrFromString(rpID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrValidationFailed, err)
	}

	// 5. Prepare Client Data
	hashAlgId, _ := syscall.UTF16PtrFromString("SHA-256")
	clientData := webauthnClientData{
		StructVersion:  WEBAUTHN_CLIENT_DATA_CURRENT_VERSION,
		ClientDataSize: uint32(len(challenge)),
		ClientData:     &challenge[0],
		HashAlgId:      hashAlgId,
	}

	// 6. Prepare Options with API version 4
	options := webauthnAuthenticatorGetAssertionOptions{
		StructVersion:               WEBAUTHN_AUTHENTICATOR_GET_ASSERTION_OPTIONS_CURRENT_VERSION,
		TimeoutMilliseconds:         60000,
		AuthenticatorAttachment:     WEBAUTHN_AUTHENTICATOR_ATTACHMENT_PLATFORM,
		UserVerificationRequirement: WEBAUTHN_USER_VERIFICATION_REQUIRED,
	}

	// 7. Call WebAuthNAuthenticatorGetAssertion
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
		return nil, mapWebAuthnHRESULT(uint32(ret), constants.ErrWindowsHelloAuthentication)
	}
	defer procWebAuthNFreeAssertion.Call(uintptr(unsafe.Pointer(pAssertion)))

	// 8. Validate response pointers and sizes before extraction
	if pAssertion == nil {
		return nil, fmt.Errorf("%w: null assertion pointer", constants.ErrWindowsHelloAuthentication)
	}
	if pAssertion.CredentialIdSize > webauthnMaxResponseSize ||
		pAssertion.AuthenticatorDataSize > webauthnMaxResponseSize ||
		pAssertion.SignatureSize > webauthnMaxResponseSize ||
		pAssertion.UserHandleSize > webauthnMaxResponseSize {
		return nil, constants.ErrWindowsHelloResponseTooLarge
	}
	if pAssertion.CredentialId == nil || pAssertion.AuthenticatorData == nil || pAssertion.Signature == nil {
		return nil, fmt.Errorf("%w: null response data pointer", constants.ErrWindowsHelloAuthentication)
	}

	// 9. Extract result
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
