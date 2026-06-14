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

package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkIdentityArgs_WritesFileWith0600Permissions(t *testing.T) {
	t.Parallel()

	pm := &ProcessManager{runtimeDir: t.TempDir()}
	args, err := pm.networkIdentityArgs([]byte(`{"IPs":["192.0.2.10"]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"--network-identity-file", filepath.Join(pm.runtimeDir, constants.NetworkIdentityFilename)}, args)

	identityFile := filepath.Join(pm.runtimeDir, constants.NetworkIdentityFilename)
	info, err := os.Stat(identityFile)
	require.NoError(t, err)
	// Verify file permissions on Unix systems
	// Windows uses ACLs and doesn't support Unix-style permissions
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestNetworkIdentityArgs_NoDataReturnsNil(t *testing.T) {
	t.Parallel()

	pm := &ProcessManager{runtimeDir: t.TempDir()}
	args, err := pm.networkIdentityArgs(nil)
	require.NoError(t, err)
	assert.Nil(t, args)
}

func TestWriteNetworkIdentityFile_ErrorOnInvalidRuntimeDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runtimeFile := filepath.Join(dir, "runtime")
	require.NoError(t, os.WriteFile(runtimeFile, []byte("file"), 0600))

	pm := &ProcessManager{runtimeDir: runtimeFile}
	_, err := pm.writeNetworkIdentityFile([]byte(`{"IPs":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write network identity file")
}
