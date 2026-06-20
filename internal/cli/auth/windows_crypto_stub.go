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

//go:build !windows
// +build !windows

package auth

import (
	"crypto/ecdsa"

	"github.com/g8e-ai/g8e/internal/constants"
)

// GenerateWindowsCSR is a stub for non-Windows platforms.
func GenerateWindowsCSR(commonName string, useTPM bool) (string, *ecdsa.PrivateKey, error) {
	return "", nil, constants.ErrWindowsSpecificEnrollment
}

// ImportCertificateToWindowsStore is a stub for non-Windows platforms.
func ImportCertificateToWindowsStore(certPEM string) error {
	return constants.ErrWindowsCertStoreImport
}

// SignWithWindowsHello is a stub for non-Windows platforms.
func SignWithWindowsHello(transactionHash []byte) ([]byte, error) {
	return nil, constants.ErrWindowsHelloSigning
}

// AuthenticateWithWindowsHello is a stub for non-Windows platforms.
func AuthenticateWithWindowsHello(rpID string, challenge []byte) (*WebAuthnAssertionResponse, error) {
	return nil, constants.ErrWindowsHelloAuthentication
}

// RegisterWithWindowsHello is a stub for non-Windows platforms.
func RegisterWithWindowsHello(rpID, rpName string, userIDBytes []byte, userName string, challenge []byte) (*WebAuthnAttestationResponse, error) {
	return nil, constants.ErrWindowsHelloRegistration
}

// WebAuthnAttestationResponse is a stub for non-Windows platforms.
type WebAuthnAttestationResponse struct {
	Id                string
	RawId             []byte
	AuthenticatorData []byte
	AttestationObject []byte
}

// WebAuthnAssertionResponse is a stub for non-Windows platforms.
type WebAuthnAssertionResponse struct {
	Id                string
	RawId             []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        []byte
}

// TrustRootCAInWindowsStore is a stub for non-Windows platforms.
func TrustRootCAInWindowsStore(caBundlePEM string) error {
	return constants.ErrWindowsCertStoreTrust
}
