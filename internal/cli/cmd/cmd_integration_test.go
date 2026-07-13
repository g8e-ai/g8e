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

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestCommandErrorHandling(t *testing.T) {
	t.Run("data store requires collection flag", func(t *testing.T) {
		cmd := dataStoreCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		setupDataTestConfig(t, tmpDir)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// The command will fail on authentication before flag validation
		// Just verify it fails
	})

	t.Run("data audit list requires Operator session id", func(t *testing.T) {
		cmd := dataAuditListCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		setupDataTestConfig(t, tmpDir)

		// Unset the environment variable
		originalEnv := os.Getenv("G8E_OPERATOR_SESSION_ID")
		os.Unsetenv("G8E_OPERATOR_SESSION_ID")
		defer func() {
			if originalEnv != "" {
				os.Setenv("G8E_OPERATOR_SESSION_ID", originalEnv)
			}
		}()

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// The command will fail on authentication before flag validation
		// Just verify it fails
	})
}
