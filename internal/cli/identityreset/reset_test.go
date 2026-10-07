// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package identityreset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"

	"github.com/stretchr/testify/require"
)

func write(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, ".g8e", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("retain-or-remove"), 0600))
	return path
}

func TestResetPreservesDataAndOtherIdentities(t *testing.T) {
	for _, component := range []string{"operator", "ensemble"} {
		t.Run(component, func(t *testing.T) {
			dir := t.TempDir()
			selected, err := paths(component)
			require.NoError(t, err)
			for _, name := range selected {
				write(t, dir, name)
			}
			retained := []string{"data/g8e.db", "vault/key", "secrets/.master_key", "config.yml", "logs/operator.log", "pki/issued/apps/other.key"}
			for _, name := range retained {
				write(t, dir, name)
			}
			require.NoError(t, Reset(dir, component))
			for _, name := range selected {
				_, err := os.Stat(filepath.Join(dir, ".g8e", name))
				require.True(t, os.IsNotExist(err), name)
			}
			for _, name := range retained {
				data, err := os.ReadFile(filepath.Join(dir, ".g8e", name))
				require.NoError(t, err)
				require.Equal(t, "retain-or-remove", string(data))
			}
			require.NoError(t, Reset(dir, component))
		})
	}
}

func TestResetRefusesGateway(t *testing.T) {
	dir := t.TempDir()
	cert := write(t, dir, "pki/operator.crt")
	write(t, dir, "pki/root/root_ca.crt")
	require.ErrorContains(t, Reset(dir, "operator"), "gateway runtime")
	_, err := os.Stat(cert)
	require.NoError(t, err)
}

func TestResetRefusesSymlinkBeforeRemovingCredentials(t *testing.T) {
	dir := t.TempDir()
	cert := write(t, dir, "pki/operator.crt")
	outside := t.TempDir()
	testutil.Symlink(t, outside, filepath.Join(dir, ".g8e/pki/trust"))
	require.ErrorContains(t, Reset(dir, "operator"), "symlink")
	_, err := os.Stat(cert)
	require.NoError(t, err)
}
