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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestNetworkIdentityArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		identityData []byte
		expectArgs   bool
		checkPerms   bool
	}{
		{
			name:         "writes file with private permissions",
			identityData: []byte(`{"IPs":["192.0.2.10"]}`),
			expectArgs:   true,
			checkPerms:   true,
		},
		{
			name:         "no data returns nil",
			identityData: nil,
			expectArgs:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pm := &ProcessManager{runtimeDir: t.TempDir()}
			args, err := pm.networkIdentityArgs(tt.identityData)
			require.NoError(t, err)

			if !tt.expectArgs {
				assert.Nil(t, args)
				return
			}

			expectedPath := filepath.Join(pm.runtimeDir, constants.NetworkIdentityFilename)
			assert.Equal(t, []string{"--network-identity-file", expectedPath}, args)

			if tt.checkPerms && runtime.GOOS != "windows" {
				info, err := os.Stat(expectedPath)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
			}
		})
	}
}

func TestWriteNetworkIdentityFile_ErrorOnInvalidRuntimeDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runtimeFile := filepath.Join(dir, constants.NetworkIdentityFilename)
	require.NoError(t, os.WriteFile(runtimeFile, []byte("file"), constants.PermFilePrivate))

	pm := &ProcessManager{runtimeDir: runtimeFile}
	_, err := pm.writeNetworkIdentityFile([]byte(`{"IPs":[]}`))
	require.Error(t, err)
}
