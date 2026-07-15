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

package keystoretest

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

// NewTestFileService creates a RuntimeFileService backed by a temp directory
// with the full .g8e runtime tree created. Returns the file service and the
// absolute path to the secrets directory.
func NewTestFileService(t *testing.T) (fs.RuntimeFileService, string) {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc, svc.Resolve(constants.SecretsDirname)
}
