// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// TrustBundle is the parsed runtime trust bundle: the full gateway CA
// chain (root + intermediates) used by CLI mTLS. It is distinct from the
// OS trust store, which receives only the self-signed root anchor(s).
type TrustBundle struct {
	// PEM is the raw bundle PEM bytes as read from disk or received from
	// the gateway.
	PEM []byte

	// Certificates is the parsed certificate set, in PEM order.
	Certificates []*x509.Certificate

	// RootAnchors is the subset of self-signed CA certificates suitable
	// for OS trust installation. Derived via platform.ExtractRootAnchors.
	RootAnchors []*x509.Certificate

	// PrimaryRootFingerprint is the SHA-256 fingerprint (hex) of the first
	// root anchor, used as the idempotency key for IsTrusted/InstallRoot.
	PrimaryRootFingerprint string
}

// ParseTrustBundle parses PEM bundle bytes into a TrustBundle, extracting
// root anchors and computing the primary root fingerprint. It rejects
// empty, malformed, or expired input via the shared platform helpers so
// the validation rules are identical to ExtractRootAnchors/InstallRoot.
//
// now is injectable for tests; pass time.Now in production.
func ParseTrustBundle(bundlePEM []byte, now time.Time) (*TrustBundle, error) {
	if len(bundlePEM) == 0 {
		return nil, constants.ErrEmptyTrustBundle
	}

	certs, err := platform.ParseBundleCerts(bundlePEM)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrCAParseFailed, err)
	}
	if len(certs) == 0 {
		return nil, constants.ErrEmptyTrustBundle
	}

	roots, err := platform.ExtractRootAnchors(bundlePEM, func() time.Time { return now })
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, constants.ErrSystemTrustInvalidAnchor
	}

	return &TrustBundle{
		PEM:                    bundlePEM,
		Certificates:           certs,
		RootAnchors:            roots,
		PrimaryRootFingerprint: platform.CertFingerprint(roots[0]),
	}, nil
}

// ContainsFingerprint reports whether any certificate in the bundle has the
// given SHA-256 fingerprint (hex). Used to detect an already-trusted root
// without re-parsing.
func (b *TrustBundle) ContainsFingerprint(fingerprint string) bool {
	if b == nil || fingerprint == "" {
		return false
	}
	for _, c := range b.Certificates {
		if platform.CertFingerprint(c) == fingerprint {
			return true
		}
	}
	return false
}

// VerifyFingerprintPin checks the bundle against an expected root CA
// fingerprint pin. An empty expected fingerprint is a no-op (no pin
// configured). The pin must match at least one certificate in the bundle.
func (b *TrustBundle) VerifyFingerprintPin(expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return nil
	}
	if b == nil {
		return constants.ErrValidationFailed
	}
	if !b.ContainsFingerprint(expectedFingerprint) {
		return constants.ErrValidationFailed
	}
	return nil
}

// ReadTrustBundleFromDisk reads the configured runtime trust bundle and
// parses it. Returns (nil, nil) when the bundle is absent — the caller
// (CredentialStore.Inspect) treats absence as part of partial local state.
func ReadTrustBundleFromDisk(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (*TrustBundle, error) {
	raw, err := ReadTrustBundle(fileSvc, cfg)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	parsed, perr := ParseTrustBundle(raw, time.Now())
	if perr != nil {
		// A present-but-unparseable bundle is corrupt state, not a clean
		// absent state. Surface the parse error so Inspect can classify
		// the local identity as corrupt.
		return nil, perr
	}
	return parsed, nil
}

// WriteTrustBundleToDisk writes the bundle PEM to the configured runtime
// trust bundle path via the file service (or os.WriteFile for a configured
// external path). The mode should be PermFilePublic for the runtime bundle
// — it contains only public certificates.
func WriteTrustBundleToDisk(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config, bundlePEM []byte) error {
	if len(bundlePEM) == 0 {
		return constants.ErrEmptyTrustBundle
	}
	if err := WriteTrustBundleFS(fileSvc, cfg, bundlePEM, constants.PermFilePublic); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrTrustSaveFailed, err)
	}
	return nil
}

// RemoveTrustBundleFromDisk removes the configured runtime trust bundle.
// No-op if the file does not exist. Used by CredentialStore.Clear for
// logout/recovery cleanup.
func RemoveTrustBundleFromDisk(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) error {
	if err := RemoveTrustBundleFS(fileSvc, cfg); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	return nil
}
