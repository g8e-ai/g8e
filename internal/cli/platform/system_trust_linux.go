// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux
// +build linux

package platform

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const (
	// linuxManagedDirDebian is the Debian/Ubuntu CA drop directory scanned by
	// update-ca-certificates.
	linuxManagedDirDebian = "/usr/local/share/ca-certificates"
	// linuxManagedDirRHEL is the RHEL/Fedora/SUSE CA drop directory scanned by
	// update-ca-trust (p11-kit).
	linuxManagedDirRHEL = "/etc/pki/ca-trust/source/anchors"
	// linuxManagedPrefix is the stable filename prefix for g8e root anchors.
	linuxManagedPrefix = "g8e-root-"
)

// linuxTrustFamily identifies which Linux trust tooling family is available.
type linuxTrustFamily int

const (
	linuxFamilyUnknown linuxTrustFamily = iota
	linuxFamilyDebian                   // update-ca-certificates
	linuxFamilyRHEL                     // trust (p11-kit) / update-ca-trust
)

// detectLinuxFamily checks which trust management tool is available on PATH.
func detectLinuxFamily(runner commandRunner, ctx context.Context) linuxTrustFamily {
	if _, err := runner.Run(ctx, nil, "update-ca-certificates", "--help"); err == nil {
		return linuxFamilyDebian
	}
	if _, err := runner.Run(ctx, nil, "trust", "list"); err == nil {
		return linuxFamilyRHEL
	}
	return linuxFamilyUnknown
}

// managedDir returns the CA drop directory for the given family.
func managedDir(family linuxTrustFamily) string {
	switch family {
	case linuxFamilyDebian:
		return linuxManagedDirDebian
	case linuxFamilyRHEL:
		return linuxManagedDirRHEL
	default:
		return ""
	}
}

// managedFilename returns the stable, fingerprint-specific managed filename.
func managedFilename(fingerprint string) string {
	return linuxManagedPrefix + fingerprint + ".crt"
}

// isTrustedPlatform checks whether the root anchor is already present in the
// OS trust store. For Debian it inspects the managed drop file; for RHEL it
// parses p11-kit PEM output and compares fingerprints. No privilege prompt is
// triggered on the already-trusted path.
func (i *SystemTrustInstaller) isTrustedPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) (bool, error) {
	family := detectLinuxFamily(i.runner, ctx)
	if family == linuxFamilyUnknown {
		return false, fmt.Errorf("%w: neither update-ca-certificates nor trust found on PATH", constants.ErrSystemTrustUnsupported)
	}

	switch family {
	case linuxFamilyDebian:
		return i.isTrustedDebian(ctx, fingerprint)
	case linuxFamilyRHEL:
		return i.isTrustedRHEL(ctx, fingerprint)
	default:
		return false, nil
	}
}

func (i *SystemTrustInstaller) isTrustedDebian(ctx context.Context, fingerprint string) (bool, error) {
	managedPath := filepath.Join(linuxManagedDirDebian, managedFilename(fingerprint))
	out, err := i.runner.Run(ctx, nil, "cat", managedPath)
	if err != nil {
		// File does not exist or is unreadable — not trusted yet.
		return false, nil
	}
	return bundleContainsFingerprint(out, fingerprint)
}

func (i *SystemTrustInstaller) isTrustedRHEL(ctx context.Context, fingerprint string) (bool, error) {
	out, err := i.runner.Run(ctx, nil, "trust", "list", "--filter=ca-anchors", "--format=pem")
	if err != nil {
		return false, fmt.Errorf("%w: trust list failed: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	return bundleContainsFingerprint(out, fingerprint)
}

// installPlatform writes the root anchor to a restrictive temp file, then uses
// sudo to copy it into the managed CA directory and invoke the trust update
// tool. The temp file is cleaned up on every path.
func (i *SystemTrustInstaller) installPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	family := detectLinuxFamily(i.runner, ctx)
	if family == linuxFamilyUnknown {
		return fmt.Errorf("%w: neither update-ca-certificates nor trust found on PATH", constants.ErrSystemTrustUnsupported)
	}

	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
	dir, certPath, err := writeTempCert(rootPEM)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	managedPath := filepath.Join(managedDir(family), managedFilename(fingerprint))

	// sudo cp <temp> <managed> — argument array, no shell interpolation.
	if _, err := i.runner.Run(ctx, nil, "sudo", "cp", certPath, managedPath); err != nil {
		return wrapElevation(err)
	}

	switch family {
	case linuxFamilyDebian:
		if _, err := i.runner.Run(ctx, nil, "sudo", "update-ca-certificates"); err != nil {
			return wrapElevation(err)
		}
	case linuxFamilyRHEL:
		if _, err := i.runner.Run(ctx, nil, "sudo", "update-ca-trust", "extract"); err != nil {
			return wrapElevation(err)
		}
	}
	return nil
}

// wrapElevation maps common sudo/exec failures to typed errors. A non-zero
// exit from sudo typically means the user cancelled the privilege prompt or
// the command itself failed.
func wrapElevation(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
}

// listStaleAnchorsPlatform enumerates g8e-root-*.crt files in the managed CA
// directory, parses each certificate's fingerprint, and returns those that do
// not match currentFingerprint. Works for both Debian and RHEL families since
// both use the g8e-root-<fingerprint>.crt naming convention.
func (i *SystemTrustInstaller) listStaleAnchorsPlatform(ctx context.Context, currentFingerprint string) ([]StaleAnchor, error) {
	family := detectLinuxFamily(i.runner, ctx)
	if family == linuxFamilyUnknown {
		return nil, fmt.Errorf("%w: neither update-ca-certificates nor trust found on PATH", constants.ErrSystemTrustUnsupported)
	}

	dir := managedDir(family)
	out, err := i.runner.Run(ctx, nil, "ls", dir)
	if err != nil {
		// Directory does not exist or is empty — no stale anchors.
		return []StaleAnchor{}, nil
	}

	var stale []StaleAnchor
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.HasPrefix(name, linuxManagedPrefix) || !strings.HasSuffix(name, ".crt") {
			continue
		}
		filePath := filepath.Join(dir, name)
		certPEM, err := i.runner.Run(ctx, nil, "cat", filePath)
		if err != nil {
			continue // skip unreadable files
		}
		certs, parseErr := parseBundleCerts(certPEM)
		if parseErr != nil || len(certs) == 0 {
			continue // skip unparseable files
		}
		cert := certs[0]
		fp := certFingerprint(cert)
		if fp == currentFingerprint {
			continue // active anchor — not stale
		}
		stale = append(stale, StaleAnchor{
			Fingerprint: fp,
			CommonName:  cert.Subject.CommonName,
			Handle:      filePath,
		})
	}
	if stale == nil {
		return []StaleAnchor{}, nil
	}
	return stale, nil
}

// removeStaleAnchorsPlatform removes each stale anchor file via sudo rm, then
// refreshes the trust bundle with the family-appropriate tool.
func (i *SystemTrustInstaller) removeStaleAnchorsPlatform(ctx context.Context, anchors []StaleAnchor) error {
	if len(anchors) == 0 {
		return nil
	}
	family := detectLinuxFamily(i.runner, ctx)
	if family == linuxFamilyUnknown {
		return fmt.Errorf("%w: neither update-ca-certificates nor trust found on PATH", constants.ErrSystemTrustUnsupported)
	}

	for _, a := range anchors {
		if _, err := i.runner.Run(ctx, nil, "sudo", "rm", "-f", a.Handle); err != nil {
			return wrapElevation(err)
		}
	}

	switch family {
	case linuxFamilyDebian:
		if _, err := i.runner.Run(ctx, nil, "sudo", "update-ca-certificates", "--fresh"); err != nil {
			return wrapElevation(err)
		}
	case linuxFamilyRHEL:
		if _, err := i.runner.Run(ctx, nil, "sudo", "update-ca-trust", "extract"); err != nil {
			return wrapElevation(err)
		}
	}
	return nil
}
