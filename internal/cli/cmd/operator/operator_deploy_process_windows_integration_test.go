// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows && integration

package operatorcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// The deployed fixture runs the actual local launch path with its normal argv.
// It stays alive until killed by test cleanup; no Gateway or credentials exist.
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && os.Args[1] == "operator" && os.Args[2] == "start" {
		dir := os.Args[len(os.Args)-1]
		if err := os.WriteFile(filepath.Join(dir, "ready"), nil, constants.PermFilePrivate); err != nil {
			os.Exit(1)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	os.Exit(m.Run())
}

func TestOperatorDeployLocalWindowsDoesNotWaitForDetachedWorkers(t *testing.T) {
	bin, err := os.Executable()
	require.NoError(t, err)
	root := testutil.TempDir(t)
	before := runtime.NumGoroutine()
	for i := range 8 {
		dir := filepath.Join(root, strconv.Itoa(i))
		require.NoError(t, os.Mkdir(dir, constants.PermDirPrivate))
		require.NoError(t, CopyFile(bin, filepath.Join(dir, "g8e.exe")))
		host := deploySSH{local: true}
		require.NoError(t, host.startOperator(t.Context(), dir, constants.LocalhostIP))
		data, err := os.ReadFile(filepath.Join(dir, constants.OperatorPIDFilename))
		require.NoError(t, err)
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		require.NoError(t, err)
		// Hold an independent handle to join cleanup even after deploy releases its own.
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
		require.NoError(t, err)
		t.Cleanup(func() {
			defer windows.CloseHandle(handle)
			require.NoError(t, windows.TerminateProcess(handle, 0))
			status, err := windows.WaitForSingleObject(handle, 5000)
			require.NoError(t, err)
			require.EqualValues(t, windows.WAIT_OBJECT_0, status)
		})
		require.Eventually(t, func() bool {
			_, err := os.Stat(filepath.Join(dir, "ready"))
			return err == nil
		}, 5*time.Second, 10*time.Millisecond)
	}
	require.LessOrEqual(t, runtime.NumGoroutine(), before+2,
		"detached workers must not leave a Wait goroutine and OS thread in deploy")
}
