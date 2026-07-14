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

package governance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActuatorPublicKeyExport(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		err = ExportActuatorPublicKey(fileSvc, pubKey, "test-Actuator-key", logger)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		pemData, err := fileSvc.ReadFile(context.Background(), pemRel)
		require.NoError(t, err)

		block, rest := pem.Decode(pemData)
		require.NotNil(t, block, "failed to decode PEM block")
		require.Empty(t, rest, "unexpected trailing data after PEM block")
		require.Equal(t, "PUBLIC KEY", block.Type)
		require.Equal(t, []byte(pubKey), block.Bytes)

		jsonRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubJSONFilename)
		jsonData, err := fileSvc.ReadFile(context.Background(), jsonRel)
		require.NoError(t, err)

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "test-Actuator-key", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey), parsed.PublicKey)
		assert.Equal(t, "ed25519", parsed.Algorithm)
	})

	t.Run("NilLogger", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		err = ExportActuatorPublicKey(fileSvc, pubKey, "key-id", nil)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		_, err = fileSvc.ReadFile(context.Background(), pemRel)
		require.NoError(t, err, "PEM file should be created even with nil logger")
	})

	t.Run("OverwriteExisting", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)
		pubKey1, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		pubKey2, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		require.NoError(t, ExportActuatorPublicKey(fileSvc, pubKey1, "key-1", logger))
		require.NoError(t, ExportActuatorPublicKey(fileSvc, pubKey2, "key-2", logger))

		jsonRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubJSONFilename)
		jsonData, err := fileSvc.ReadFile(context.Background(), jsonRel)
		require.NoError(t, err)

		var parsed models.ActuatorPublicKeyExport
		require.NoError(t, json.Unmarshal(jsonData, &parsed))
		assert.Equal(t, "key-2", parsed.KeyID)
		assert.Equal(t, hex.EncodeToString(pubKey2), parsed.PublicKey)
	})

	t.Run("FilePermissions", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)
		pubKey, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

		err = ExportActuatorPublicKey(fileSvc, pubKey, "key-id", logger)
		require.NoError(t, err)

		pemRel := filepath.Join(constants.PkiDirname, constants.ActuatorPubPEMFilename)
		info, err := fileSvc.Stat(context.Background(), pemRel)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.True(t, info.Mode().Perm() == constants.PermFilePrivate)
		}
	})
}
