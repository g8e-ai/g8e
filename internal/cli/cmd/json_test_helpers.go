// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// enableGlobalJSON attaches cmd under a synthetic root with the global --json
// flag enabled so unit tests can exercise JSON output without full CLI execution.
func enableGlobalJSON(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	root := cmd.Root()
	if root == cmd {
		wrapper := &cobra.Command{Use: "g8e"}
		wrapper.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output")
		wrapper.AddCommand(cmd)
		root = wrapper
	}
	require.NoError(t, root.PersistentFlags().Set("json", "true"))
}

// globalJSONRoot wraps cmd in a synthetic g8e root with --json enabled and
// returns the root for full Execute()-based tests.
func globalJSONRoot(t *testing.T, cmd *cobra.Command) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "g8e"}
	root.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output")
	root.AddCommand(cmd)
	require.NoError(t, root.PersistentFlags().Set("json", "true"))
	return root
}
