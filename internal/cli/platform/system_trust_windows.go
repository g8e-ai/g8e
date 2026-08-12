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

package platform

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// envCertPath passes the certificate file path to PowerShell via an
// environment variable, avoiding command injection from string interpolation.
const envCertPath = "G8E_SYSTRUST_CERT_PATH"

// isTrustedPlatform enumerates the LocalMachine\Root store and compares
// SHA-256 fingerprints. The fingerprint is passed via environment variable to
// the PowerShell script; no PEM or path data is interpolated into the command
// string.
func (i *SystemTrustInstaller) isTrustedPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) (bool, error) {
	psScript := `$fp = $env:G8E_SYSTRUST_FP
$store = New-Object System.Security.Cryptography.X509Certificates.X509Store("Root","LocalMachine")
$store.Open("ReadOnly")
foreach ($c in $store.Certificates) {
  $hash = $c.GetCertHashString("SHA256")
  if ($hash -eq $fp) { Write-Output "MATCH"; exit 0 }
}
Write-Output "NOMATCH"
$store.Close()`

	out, err := i.runner.Run(ctx, map[string]string{"G8E_SYSTRUST_FP": fingerprint}, "powershell", "-NoProfile", "-Command", psScript)
	if err != nil {
		return false, fmt.Errorf("%w: enumerate LocalMachine\\Root failed: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	return strings.Contains(string(out), "MATCH"), nil
}

// installPlatform imports the root anchor into LocalMachine\Root via certutil.
// The cert path is passed as a plain argument (no shell interpolation). This
// requires elevation; a UAC rejection surfaces as a typed install error.
func (i *SystemTrustInstaller) installPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
	dir, certPath, err := writeTempCert(rootPEM)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	_, err = i.runner.Run(ctx, nil, "certutil", "-addstore", "-f", "Root", certPath)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	return nil
}
