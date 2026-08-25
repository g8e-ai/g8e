// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

package platform

import (
	"context"
	"crypto/x509"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func (i *SystemTrustInstaller) isTrustedPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) (bool, error) {
	return false, constants.ErrSystemTrustUnsupported
}

func (i *SystemTrustInstaller) installPlatform(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	return constants.ErrSystemTrustUnsupported
}

func (i *SystemTrustInstaller) listStaleAnchorsPlatform(ctx context.Context, currentFingerprint string) ([]StaleAnchor, error) {
	return nil, constants.ErrSystemTrustUnsupported
}

func (i *SystemTrustInstaller) removeStaleAnchorsPlatform(ctx context.Context, anchors []StaleAnchor) error {
	return constants.ErrSystemTrustUnsupported
}
