// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build darwin
// +build darwin

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

// listStaleAnchorsPlatform enumerates certificates in the system keychain
// whose subject CN matches constants.RootCACommonName and whose SHA-256
// fingerprint does not match currentFingerprint. Uses `security
// find-certificate -Z` to obtain the SHA-1 hash for later removal.
func (i *SystemTrustInstaller) listStaleAnchorsPlatform(ctx context.Context, currentFingerprint string) ([]StaleAnchor, error) {
	// -Z prints the SHA-1 hash; -c matches by CN; -a returns all matches.
	out, err := i.runner.Run(ctx, nil, "security", "find-certificate", "-c", constants.RootCACommonName, "-a", "-Z", darwinSystemKeychain)
	if err != nil {
		// No matches or keychain unreadable — no stale anchors.
		return []StaleAnchor{}, nil
	}

	var stale []StaleAnchor
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "SHA-1 hash:") {
			continue
		}
		sha1 := strings.TrimSpace(strings.TrimPrefix(line, "SHA-1 hash:"))
		if sha1 == "" {
			continue
		}
		// Fetch the PEM for this cert to compute SHA-256 and get the CN.
		// security find-certificate -Z -c <cn> doesn't give us the PEM per-cert
		// when there are multiple matches, so we use delete-certificate's
		// dry-run alternative: export by SHA-1 hash.
		pemOut, err := i.runner.Run(ctx, nil, "security", "find-certificate", "-Z", "-a", "-p", darwinSystemKeychain)
		if err != nil {
			continue
		}
		certs, parseErr := parseBundleCerts(pemOut)
		if parseErr != nil {
			continue
		}
		for _, cert := range certs {
			if cert.Subject.CommonName != constants.RootCACommonName {
				continue
			}
			fp := certFingerprint(cert)
			if fp == currentFingerprint {
				continue
			}
			stale = append(stale, StaleAnchor{
				Fingerprint: fp,
				CommonName:  cert.Subject.CommonName,
				Handle:      sha1,
			})
		}
		// We only need the PEM once to get all matching certs; break after
		// the first successful parse since subsequent SHA-1 lines would
		// re-fetch the same PEM set.
		break
	}
	if stale == nil {
		return []StaleAnchor{}, nil
	}
	return stale, nil
}

// removeStaleAnchorsPlatform removes each stale anchor from the system
// keychain via `sudo security delete-certificate -Z <sha1>`. Requires
// elevation.
func (i *SystemTrustInstaller) removeStaleAnchorsPlatform(ctx context.Context, anchors []StaleAnchor) error {
	for _, a := range anchors {
		if _, err := i.runner.Run(ctx, nil, "sudo", "security", "delete-certificate", "-Z", a.Handle, darwinSystemKeychain); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
		}
	}
	return nil
}
