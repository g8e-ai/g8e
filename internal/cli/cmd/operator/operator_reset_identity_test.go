// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResetIdentityStopsOnlySelectedDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".g8e/pki/operator.key")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("old-key"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "full.pid"), []byte("123"), 0600))
	selectedSignals, otherSignals, closed := 0, 0, 0
	cmd := operatorResetIdentityCmdWithDiscovery(func() ([]localOperatorProcess, error) {
		return []localOperatorProcess{
			{pid: 123, dir: dir, close: func() { closed++ }, wait: func(time.Duration) (bool, error) { return selectedSignals > 0, nil }, signal: func(bool) error { selectedSignals++; return nil }},
			{pid: 456, dir: t.TempDir(), close: func() { closed++ }, wait: func(time.Duration) (bool, error) { t.Fatal("unrelated worker waited on"); return false, nil }, signal: func(bool) error { otherSignals++; return nil }},
		}, nil
	})
	cmd.SetArgs([]string{"--working-dir", dir, "--yes"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
	require.Equal(t, 1, selectedSignals)
	require.Zero(t, otherSignals)
	require.Equal(t, 2, closed)
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "full.pid"))
	require.True(t, os.IsNotExist(err))
}

func TestResetIdentityDeclinedDoesNotDiscoverOrDelete(t *testing.T) {
	dir := t.TempDir()
	cmd := operatorResetIdentityCmdWithDiscovery(func() ([]localOperatorProcess, error) {
		t.Fatal("discovery before confirmation")
		return nil, nil
	})
	cmd.SetArgs([]string{"--working-dir", dir})
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}
