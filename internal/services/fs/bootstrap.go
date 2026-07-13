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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
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

	infra := paths.Infra

	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{fs.runtimeDir, constants.PermDirPrivate},
		{infra.DataDir, constants.PermDirStandard},
		{infra.PkiDir, constants.PermDirStandard},
		{infra.PkiRootDir, constants.PermDirStandard},
		{infra.PkiAuthoritiesDir, constants.PermDirStandard},
		{infra.PkiIssuedDir, constants.PermDirStandard},
		{infra.PkiIssuedHubDir, constants.PermDirStandard},
		{infra.PkiIssuedGatewayPeerDir, constants.PermDirStandard},
		{infra.AppCertDir, constants.PermDirStandard},
		{infra.PkiTrustDir, constants.PermDirStandard},
		{infra.PkiRevocationDir, constants.PermDirStandard},
		{infra.PkiBinariesDir, constants.PermDirStandard},
		{infra.TrustedSignersDir, constants.PermDirStandard},
		{infra.ClientPkiDir, constants.PermDirStandard},
		{infra.SecretsDir, constants.PermDirPrivate},
		{infra.VaultDir, constants.PermDirPrivate},
		{infra.LedgerDir, constants.PermDirStandard},
		{infra.LedgerFilesDir, constants.PermDirStandard},
		{infra.LogDir, constants.PermDirStandard},
		{infra.PidDir, constants.PermDirStandard},
		{infra.ProtocolDir, constants.PermDirStandard},
		{infra.DocsDir, constants.PermDirStandard},
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir.path, dir.mode); err != nil {
			return fmt.Errorf("%w: %s: %w", constants.ErrDirCreateFailed, dir.path, err)
		}
	}

	return nil
}
