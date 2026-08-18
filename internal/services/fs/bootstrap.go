// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
)

// CreateRuntimeTree creates the full .g8e/ directory tree with correct
// permissions. Called once at startup. Idempotent.
//
// PKI directories use PermDirStandard (0755) to match existing gateway_certs.go
// behavior. SecretsDir and VaultDir use PermDirPrivate (0700) for sensitive
// material.
func (fs *localFS) CreateRuntimeTree(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	pkiDir := filepath.Join(fs.runtimeDir, constants.PkiDirname)

	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{fs.runtimeDir, constants.PermDirPrivate},
		{filepath.Join(fs.runtimeDir, constants.DataDirname), constants.PermDirStandard},
		{pkiDir, constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirRoot), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirAuthorities), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirIssued), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirIssued, constants.PkiSubdirGatewayPeer), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirIssued, constants.PkiSubdirApps), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirTrust), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirRevocation), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirBinaries), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirTrustedSigners), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirClient), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.SecretsDirname), constants.PermDirPrivate},
		{filepath.Join(fs.runtimeDir, constants.VaultDirname), constants.PermDirPrivate},
		{filepath.Join(fs.runtimeDir, constants.DataDirname, constants.LedgerDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.DataDirname, constants.LedgerDirname, constants.FilesDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.LogDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.PidDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.BinDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.ProtocolDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.DocsDirname), constants.PermDirStandard},
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir.path, dir.mode); err != nil {
			return fmt.Errorf("%w: %s: %w", constants.ErrDirCreateFailed, dir.path, err)
		}
	}

	return nil
}
