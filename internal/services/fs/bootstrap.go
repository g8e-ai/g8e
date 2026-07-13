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
		{filepath.Join(pkiDir, constants.PkiSubdirApps), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirTrust), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirRevocation), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirBinaries), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirTrustedSigners), constants.PermDirStandard},
		{filepath.Join(pkiDir, constants.PkiSubdirClient), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.SecretsDirname), constants.PermDirPrivate},
		{filepath.Join(fs.runtimeDir, constants.VaultDirname), constants.PermDirPrivate},
		{filepath.Join(fs.runtimeDir, constants.LedgerDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.LedgerDirname, constants.FilesDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.LogDirname), constants.PermDirStandard},
		{filepath.Join(fs.runtimeDir, constants.PidDirname), constants.PermDirStandard},
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
