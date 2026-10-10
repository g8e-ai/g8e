// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows && integration

package testcmd

import (
	"context"
	"encoding/csv"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestScaleResourcesWindowsRecordsLiveGatewayAndFleet(t *testing.T) {
	layout, err := prepareScaleRoot(testutil.TempDir(t), false)
	require.NoError(t, err)
	fileSvc, err := fs.NewRuntimeFileService(layout.Run, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(t.Context()))
	pid := []byte(strconv.Itoa(os.Getpid()))
	require.NoError(t, fileSvc.WriteFile(t.Context(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), pid, constants.PermFilePrivate))
	worker := filepath.Join(layout.Fleet, "op-00001")
	require.NoError(t, os.Mkdir(worker, constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(filepath.Join(worker, constants.OperatorPIDFilename), pid, constants.PermFilePrivate))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sampleScaleResources(ctx, layout, time.Second)
	}()
	t.Cleanup(func() { cancel(); <-done })
	paths := []string{"gateway-resources.csv", "fleet-resources.csv"}
	require.Eventually(t, func() bool {
		for _, name := range paths {
			data, err := os.ReadFile(filepath.Join(layout.Out, name))
			if err != nil || strings.Count(string(data), "\n") < 2 {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	for _, name := range paths {
		data, err := os.ReadFile(filepath.Join(layout.Out, name))
		require.NoError(t, err)
		rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(rows), 2)
		values := make(map[string]string)
		for i, column := range rows[0] {
			values[column] = rows[1][i]
		}
		for _, column := range []string{"rss_kb", "threads"} {
			value, err := strconv.ParseInt(values[column], 10, 64)
			require.NoError(t, err)
			require.Positive(t, value, "%s must contain native Windows %s", name, column)
		}
		if name == "fleet-resources.csv" {
			require.Equal(t, "1", values["procs"])
		}
	}
}
