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

//go:build darwin
// +build darwin

package platform

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/g8e-ai/g8e/internal/constants"
)

const (
	// darwinSystemKeychain is the system-wide keychain that browsers and TLS
	// stacks consult for trusted root anchors.
	darwinSystemKeychain = "/Library/Keychains/System.keychain"
)

// isTrustedPlatform enumerates certificates in the system keychain and compares
// SHA-256 fingerprints. No privilege prompt is triggered on the already-trusted
// path because `security find-certificate` reads the keychain without elevation.
func (i *SystemTrustInstaller) isTrustedPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) (bool, error) {
	out, err := i.runner.Run(ctx, nil, "security", "find-certificate", "-a", "-p", darwinSystemKeychain)
	if err != nil {
		// Keychain unreadable or empty — treat as not trusted.
		return false, nil
	}
	return bundleContainsFingerprint(out, fingerprint)
}

// installPlatform writes the root anchor to a restrictive temp file and invokes
// `sudo security add-trusted-cert` with fixed arguments. The temp file is
// cleaned up on every path.
func (i *SystemTrustInstaller) installPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
	dir, certPath, err := writeTempCert(rootPEM)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	_, err = i.runner.Run(ctx, nil, "sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", darwinSystemKeychain, certPath)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	return nil
}
