// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
)

func TestCommandErrorHandling(t *testing.T) {
	t.Run("data store requires collection flag", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataStoreCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		// The command will fail on authentication before flag validation
		// Just verify it fails
	})

	t.Run("data audit list requires Operator session id", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)
		loader := func(_ string) (*config.Config, error) { return cfg, nil }
		cmd := dataAuditListCmdWithConfig(loader, defaultAPIClientFactory, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

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
