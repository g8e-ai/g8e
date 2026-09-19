// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version.0.

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TrustState is the read-only result of inspecting the OS trust store
// against a live gateway root fingerprint. It reports whether the live
// root is already trusted and lists stale g8e anchors from previous
// gateway instances. Both identity enrollment and gw connect consume it
// so there is one implementation of trust-state inspection.
type TrustState struct {
	// Trusted reports whether the OS trust store already contains a root
	// anchor with the given SHA-256 fingerprint.
	Trusted bool

	// StaleAnchors are orphaned g8e root CA anchors from previous gateway
	// instances that do not match the live fingerprint. Empty (not nil)
	// when none are found or when the platform does not support stale
	// detection.
	StaleAnchors []platform.StaleAnchor
}

// InspectTrustState reads the OS trust store to determine whether the
// live root fingerprint is already trusted and enumerates stale g8e
// anchors. It is a single-purpose read operation: it never installs,
// removes, or prompts. The caller (enrollment coordinator or gw connect
// command) composes the result with explicit consent before calling
// InstallRoot or RemoveStaleAnchors.
//
// ErrSystemTrustUnsupported from either sub-operation is treated as a
// benign "unsupported platform" result: Trusted is false and
// StaleAnchors is empty. The caller prints a diagnostic warning and
// proceeds without failing enrollment or connection.
func InspectTrustState(ctx context.Context, installer SystemTrustInstaller, fingerprint string) (TrustState, error) {
	trusted, err := installer.IsTrusted(ctx, fingerprint)
	if err != nil {
		if errors.Is(err, constants.ErrSystemTrustUnsupported) {
			return TrustState{Trusted: false, StaleAnchors: nil}, nil
		}
		return TrustState{}, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}

	staleAnchors, err := installer.ListStaleAnchors(ctx, fingerprint)
	if err != nil {
		if errors.Is(err, constants.ErrSystemTrustUnsupported) {
			return TrustState{Trusted: trusted, StaleAnchors: nil}, nil
		}
		return TrustState{}, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}

	return TrustState{
		Trusted:      trusted,
		StaleAnchors: staleAnchors,
	}, nil
}
