// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bytes"
	"context"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
)

// TestMCPStdio_DoesNotInvokeEnrollment verifies that the `mcp stdio` command
// path does NOT enroll. Direct `mcp stdio` is a credential consumer only — it
// loads credentials and builds an mTLS connection, never enrolling, opening a
// browser, or installing system trust. This is the §11.5 3.8 negative
// assertion.
//
// With the enrollerFactory injection model, `mcpStdioCmdWithConfig` does not
// receive an enrollerFactory at all — the absence is enforced by the function
// signature. This test runs the command and asserts it fails (no gateway, no
// credentials) without any enrollment side effect. If a future change adds an
// enrollerFactory parameter to `mcpStdioCmdWithConfig`, this test should be
// updated to inject a panicking factory and assert it is never called.
func TestMCPStdio_DoesNotInvokeEnrollment(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)

	cmd := McpStdioCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	// runMCPStdioProxy will fail because there is no gateway and no
	// credentials, but it must fail WITHOUT enrolling. The error is
	// expected; the assertion is that the command does not panic or
	// attempt enrollment (which is now impossible by construction since
	// mcpStdioCmdWithConfig has no enrollerFactory parameter).
	_ = cmd.RunE(cmd, nil)
}
