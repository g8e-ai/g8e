// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux && integration

package keystore

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestLibsecretKeyring_RealSecretTool(t *testing.T) {
	const marker = "g8e-private-test-bus"
	if os.Args[len(os.Args)-1] != marker {
		if _, err := exec.LookPath("dbus-run-session"); err != nil {
			t.Skip("dbus-run-session unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dbus-run-session", "--", os.Args[0], "-test.run=^TestLibsecretKeyring_RealSecretTool$", "-test.v", "--", marker)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		if bytes.Contains(out, []byte("--- SKIP:")) {
			t.Skipf("isolated Secret Service check skipped: %s", out)
		}
		t.Logf("%s", out)
		return
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool unavailable")
	}
	id := "g8e-test-" + uuid.NewString()
	runner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		args = append([]string(nil), args...)
		for i := range args {
			if args[i] == masterKeyName {
				args[i] = id
			}
		}
		return runSecretTool(stdin, args...)
	}
	kr, err := newLibsecretKeyring(runner)
	if err != nil {
		t.Skipf("private Secret Service unavailable: %v", err)
	}
	key := bytes.Repeat([]byte{7}, vault.KeySize)
	if err := kr.StoreMasterKey(key); err != nil {
		t.Skipf("private Secret Service has no writable collection: %v", err)
	}
	t.Cleanup(func() { require.NoError(t, kr.DeleteMasterKey()) })
	got, err := kr.RetrieveMasterKey()
	require.NoError(t, err)
	require.Equal(t, key, got)
	require.NoError(t, kr.DeleteMasterKey())
	_, err = kr.RetrieveMasterKey()
	require.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
}

func TestLibsecretKeyring_ConstructionNeverWritesAndStoreFailureHasGuidance(t *testing.T) {
	dir := testutil.TempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret-tool"), []byte("#!/bin/sh\nexit 1\n"), 0700))
	t.Setenv("PATH", dir)
	var calls []string
	cause := constants.ErrKeyStoreSecretServiceDown
	kr, err := newLibsecretKeyring(func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		calls = append(calls, args[0])
		if args[0] == "lookup" {
			return nil, nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, nil, cause
	})
	require.NoError(t, err)
	require.Equal(t, []string{"lookup"}, calls)
	fileSvc, _ := setupTestFileService(t)
	ks, err := NewWithKeyringAndFS(testutil.NewTestLogger(), kr, fileSvc)
	require.NoError(t, err)
	err = ks.Initialize()
	require.ErrorIs(t, err, cause)
	require.Contains(t, err.Error(), "--master-key-file")
	require.Contains(t, err.Error(), "G8E_MASTER_KEY_FILE")
	require.Equal(t, []string{"lookup", "lookup", "store"}, calls)
}

func TestKeystore_NoImplicitHomeKeyDiscovery(t *testing.T) {
	dir := testutil.TempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".g8e_master_key"), []byte("unused"), constants.PermFilePrivate))
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("PATH", dir) // no secret-tool, no access to the developer's session
	t.Setenv(string(constants.EnvVar.MasterKeyFile), "")
	fileSvc, _ := setupTestFileService(t)
	_, err := NewWithFS(fileSvc, testutil.NewTestLogger(), Options{})
	require.ErrorIs(t, err, constants.ErrKeyStoreOSKeyringRequired)
	require.ErrorIs(t, err, exec.ErrNotFound)
	require.Contains(t, err.Error(), "--master-key-file")
	require.Contains(t, err.Error(), "G8E_MASTER_KEY_FILE")
}
