// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows && integration

package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestExecutionService_WindowsCommandHasNoConsole(t *testing.T) {
	const probeEnv = "G8E_EXECUTION_CONSOLE_PROBE"
	if os.Getenv(probeEnv) == "1" {
		console, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
		require.Zero(t, console, "a non-interactive Operator command must not allocate a console")
		fmt.Println("command completed without a console")
		return
	}

	bin, err := os.Executable()
	require.NoError(t, err)
	bin = filepath.ToSlash(bin)
	svc := NewExecutionService(testutil.NewTestConfig(t), testutil.NewTestLogger())
	for _, tc := range []struct{ name, command string }{
		{"direct", bin},
		{"shell", "echo console-probe && " + strconv.Quote(bin)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.ExecuteCommand(context.Background(), &models.ExecutionRequestPayload{
				ExecutionID:    "windows-console-probe-" + tc.name,
				Command:        tc.command,
				Args:           []string{"-test.run=TestExecutionService_WindowsCommandHasNoConsole"},
				Environment:    map[string]string{probeEnv: "1"},
				TimeoutSeconds: 10,
			})
			require.NoError(t, err)
			require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, result.Status, "%s", result.Stderr)
			assert.Contains(t, result.Stdout, "command completed without a console")
		})
	}
}
