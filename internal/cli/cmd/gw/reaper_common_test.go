// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// withServeReExec sets the test re-exec environment variable to "serve" so the
// test process behaves as a minimal Gateway server when re-executed. The
// env var is restored via t.Cleanup. It also registers a cleanup that reaps
// any remaining child process and removes stale PID files.
func withServeReExec(t *testing.T, fileSvc fs.RuntimeFileService) {
	t.Helper()
	t.Setenv(string(constants.EnvVar.TestReexec), "serve")
	t.Cleanup(func() {
		reapRemainingChild(t, fileSvc)
	})
}

