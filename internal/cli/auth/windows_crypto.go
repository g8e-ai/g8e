// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package auth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
