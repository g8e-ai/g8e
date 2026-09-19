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
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
)

// TrustDiscoveryResult carries the validated live CA bundle and derived
// root-anchor metadata. It is the single output of the discovery reader
// and the bundle validator. Both identity enrollment and gw connect
// consume it so there is one implementation of root selection, chain
// verification, and fingerprint formatting.
type TrustDiscoveryResult struct {
	// BundlePEM is the raw PEM bundle fetched from the discovery endpoint
	// or supplied by the caller for offline validation.
	BundlePEM []byte

	// RootAnchors are the self-signed CA certificates extracted from the
	// bundle, validated for time and CA constraints.
	RootAnchors []*x509.Certificate

	// PrimaryRoot is the first root anchor, used as the trust root for
	// installation and TLS client construction.
	PrimaryRoot *x509.Certificate

	// Fingerprint is the hex-encoded SHA-256 of PrimaryRoot's DER encoding.
	Fingerprint string
}

// ValidateTrustBundle extracts root anchors from bundlePEM, verifies that
// at least one non-root certificate chains to a root anchor, and returns
// the root anchors, primary root, and its SHA-256 fingerprint. This is
// the single implementation of root selection, chain verification, and
// fingerprint formatting — both DiscoverLiveTrustBundle and the
// enrollment coordinator's install path call it so the validation logic
// is never duplicated.
//
// now is injectable for time-based validity checks in tests.
func ValidateTrustBundle(bundlePEM []byte, now func() time.Time) (TrustDiscoveryResult, error) {
	rootAnchors, err := platform.ExtractRootAnchors(bundlePEM, now)
	if err != nil {
		return TrustDiscoveryResult{}, fmt.Errorf("%w: %w", constants.ErrSystemTrustInvalidAnchor, err)
	}
	if len(rootAnchors) == 0 {
		return TrustDiscoveryResult{}, constants.ErrSystemTrustInvalidAnchor
	}
	if vErr := platform.VerifyRootUsable(rootAnchors, bundlePEM, now); vErr != nil {
		return TrustDiscoveryResult{}, fmt.Errorf("%w: %w", constants.ErrSystemTrustInvalidAnchor, vErr)
	}
	primary := rootAnchors[0]
	return TrustDiscoveryResult{
		BundlePEM:   bundlePEM,
		RootAnchors: rootAnchors,
		PrimaryRoot: primary,
		Fingerprint: platform.CertFingerprint(primary),
	}, nil
}

// DiscoverLiveTrustBundle fetches the live gateway root CA bundle from
// the unauthenticated discovery endpoint (plain-HTTP
// /.well-known/g8e/pki/ca-bundle), validates it via ValidateTrustBundle,
// and returns the result. The discoveryURL must be the full URL including
// the well-known path.
//
// The fetch uses an IPv4-only transport so localhost resolves to
// 127.0.0.1 on Windows (where the OS resolver returns ::1 first and the
// IDE port-forward only listens on IPv4). The discovery surface is plain
// HTTP, so no TLS config is needed.
//
// A network failure returns a non-nil error with empty result. The
// caller decides whether to abort; discovery failure on a reuse path
// prints a diagnostic warning and proceeds (the user may be intentionally
// offline).
func DiscoverLiveTrustBundle(ctx context.Context, discoveryURL string, now func() time.Time) (TrustDiscoveryResult, error) {
	bundlePEM, err := certs.FetchTrustBundleWithClient(ctx, discoveryURL, "", &http.Client{
		Timeout:   15 * time.Second,
		Transport: httpclient.NewIPv4Transport(nil),
	})
	if err != nil {
		return TrustDiscoveryResult{}, err
	}
	return ValidateTrustBundle(bundlePEM, now)
}
