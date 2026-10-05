// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package operatorcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatorDeployLocalWorkerDetachesFromLaunchingProcess(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fleet with spaces")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "g8e"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o700))
	host := deploySSH{local: true}
	require.NoError(t, host.startOperator(context.Background(), dir, "localhost"))
	data, err := os.ReadFile(filepath.Join(dir, "operator.pid"))
	require.NoError(t, err)
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	require.NoError(t, err)
	process, err := os.FindProcess(pid)
	require.NoError(t, err)
	t.Cleanup(func() { _ = process.Kill() })
	group, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	require.Equal(t, pid, group, "worker must survive shutdown of the deploying terminal/process group")
	require.NotEqual(t, syscall.Getpgrp(), group)
}
