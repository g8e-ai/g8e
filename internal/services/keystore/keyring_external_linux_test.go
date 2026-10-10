// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux && integration

package keystore

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestExternalKeyring_LinuxModesAndSpecialFiles(t *testing.T) {
	fileSvc, _ := setupTestFileService(t)
	path := filepath.Join(testutil.TempDir(t), "key")
	require.NoError(t, os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(make([]byte, vault.KeySize))), constants.PermFilePrivate))
	kr, err := newExternalFileKeyring(fileSvc, path)
	require.NoError(t, err)
	for _, mode := range []os.FileMode{0400, 0600, 0644, 0440, 0700} {
		require.NoError(t, os.Chmod(path, mode))
		_, err = kr.RetrieveMasterKey()
		if mode == 0400 || mode == 0600 {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, constants.ErrKeyStoreExternalFileInvalid)
		}
	}
	require.NoError(t, os.Remove(path))
	require.NoError(t, syscall.Mkfifo(path, 0600))
	_, err = kr.RetrieveMasterKey() // must return without any FIFO writer
	require.ErrorIs(t, err, constants.ErrKeyStoreExternalFileInvalid)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, constants.PermDirPrivate))
	_, err = kr.RetrieveMasterKey()
	require.ErrorIs(t, err, constants.ErrKeyStoreExternalFileInvalid)
	device, err := newExternalFileKeyring(fileSvc, "/dev/null")
	require.NoError(t, err)
	_, err = device.RetrieveMasterKey()
	require.ErrorIs(t, err, constants.ErrKeyStoreExternalFileInvalid)
}
