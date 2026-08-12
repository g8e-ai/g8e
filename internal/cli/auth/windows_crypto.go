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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
)

// envCertPath passes the certificate file path to PowerShell scripts via an
// environment variable, avoiding command injection from string interpolation.
const envCertPath = "G8E_CERT_PATH"

// ImportCertificateToWindowsStore imports a signed certificate into the
// Windows CurrentUser Personal ("My") store. It does NOT remove any
// existing certificates — old certs are left in place until they expire
// naturally or are removed by an explicit, verified replacement flow.
// The previous implementation deleted every cert whose subject contained
// "g8e", which could remove unrelated or still-valid certificates.
//
// The system Root trust store (handled by SystemTrustInstaller) is
// separate from this Personal client certificate store and is never
// touched here.
func ImportCertificateToWindowsStore(certPEM string) error {
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
