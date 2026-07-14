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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestWriteNetworkIdentityFile_ErrorOnInvalidRuntimeDir(t *testing.T) {
	t.Parallel()

	dir := testutil.TempDir(t)

	// Create a file where the .g8e/ runtime directory would be expected,
	// causing WriteFile to fail because the path is not a directory.
	runtimeBlockingFile := filepath.Join(dir, constants.RuntimeDirname)
	require.NoError(t, os.WriteFile(runtimeBlockingFile, []byte("blocking"), constants.PermFilePrivate))

	fileSvc := newPlatformTestFileSvc(t, dir)
	pm, err := NewProcessManager(fileSvc)
	require.NoError(t, err)

	_, err = pm.WriteNetworkIdentityFile([]byte(`{"IPs":[]}`))
	require.Error(t, err)
}
