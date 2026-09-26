// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package shared

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
)

type versionInfoKey struct{}

// ContextWithVersionInfo stores build metadata on ctx for command handlers.
func ContextWithVersionInfo(ctx context.Context, vi serve.VersionInfo) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, versionInfoKey{}, vi)
}

// VersionInfoFromCmd returns build metadata attached by the root command.
func VersionInfoFromCmd(cmd *cobra.Command) serve.VersionInfo {
	if cmd == nil || cmd.Context() == nil {
		return serve.VersionInfo{}
	}
	if vi, ok := cmd.Context().Value(versionInfoKey{}).(serve.VersionInfo); ok {
		return vi
	}
	return serve.VersionInfo{}
}
