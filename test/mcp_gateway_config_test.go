// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
TestMCPGateway_ConfigTemplate validates that the config template file
exists and contains the expected HTTP transport structure.
*/
func TestMCPGateway_ConfigTemplate(t *testing.T) {
	t.Run("http template exists", func(t *testing.T) {
		repoRoot := ResolveRepoRootFromTestDir(t)
		fullPath := filepath.Join(repoRoot, "protocol/examples/mcp_server/g8e_gateway_mcp_config.json")
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "http template should exist at %s", fullPath)
		assert.Contains(t, string(content), `"type": "http"`)
		assert.Contains(t, string(content), `"url"`)
		assert.Contains(t, string(content), `"tls"`)
		assert.Contains(t, string(content), `"clientCertificate"`)
		assert.Contains(t, string(content), `"clientKey"`)
		assert.Contains(t, string(content), `"caCertificate"`)
	})
}
