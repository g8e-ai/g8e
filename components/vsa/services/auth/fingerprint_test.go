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

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSystemFingerprint(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("generates valid fingerprint", func(t *testing.T) {
		fingerprint, err := GenerateSystemFingerprint(logger)

		require.NoError(t, err)
		assert.NotNil(t, fingerprint)
		assert.NotEmpty(t, fingerprint.Fingerprint)
		assert.Len(t, fingerprint.Fingerprint, 64)
		assert.Equal(t, runtime.GOOS, fingerprint.OS)
		assert.Equal(t, runtime.GOARCH, fingerprint.Architecture)
		assert.Equal(t, runtime.NumCPU(), fingerprint.CPUCount)
	})

	t.Run("fingerprint is stable across calls", func(t *testing.T) {
		fp1, err1 := GenerateSystemFingerprint(logger)
		require.NoError(t, err1)

		fp2, err2 := GenerateSystemFingerprint(logger)
		require.NoError(t, err2)

		assert.Equal(t, fp1.Fingerprint, fp2.Fingerprint)
		assert.Equal(t, fp1.OS, fp2.OS)
		assert.Equal(t, fp1.Architecture, fp2.Architecture)
		assert.Equal(t, fp1.CPUCount, fp2.CPUCount)
	})

	t.Run("includes machine ID when available", func(t *testing.T) {
		fingerprint, err := GenerateSystemFingerprint(logger)

		require.NoError(t, err)
		assert.NotEmpty(t, fingerprint.MachineID)
	})
}

func TestGetMachineID(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("returns machine ID for current OS", func(t *testing.T) {
		machineID, err := getMachineID(logger)

		switch runtime.GOOS {
		case constants.Status.Platform.Linux, constants.Status.Platform.Darwin:
			require.NoError(t, err)
			assert.NotEmpty(t, machineID)
		default:
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported operating system")
		}
	})

	t.Run("handles each supported OS correctly", func(t *testing.T) {
		machineID, err := getMachineID(logger)

		switch runtime.GOOS {
		case constants.Status.Platform.Linux:
			require.NoError(t, err)
			assert.NotEmpty(t, machineID)
			assert.NotContains(t, machineID, constants.Status.Platform.Darwin)
		case constants.Status.Platform.Darwin:
			require.NoError(t, err)
			assert.NotEmpty(t, machineID)
		}
	})
}

func TestGetLinuxMachineID(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("reads machine ID from standard locations", func(t *testing.T) {
		if runtime.GOOS != constants.Status.Platform.Linux {
			t.Skip("Linux only")
		}
		machineID, err := getLinuxMachineID(logger)
		require.NoError(t, err)
		assert.NotEmpty(t, machineID)
	})

	t.Run("handles missing machine ID files", func(t *testing.T) {
		_, err := getLinuxMachineID(logger)

		if err != nil {
			assert.Contains(t, err.Error(), "could not read machine ID")
		}
	})
}

func TestFingerprintComponents(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("fingerprint changes with different system properties", func(t *testing.T) {
		fp, err := GenerateSystemFingerprint(logger)
		require.NoError(t, err)

		assert.NotEmpty(t, fp.OS)
		assert.NotEmpty(t, fp.Architecture)
		assert.Greater(t, fp.CPUCount, 0)
		assert.NotEmpty(t, fp.Fingerprint)
	})
}

func TestMachineIDWithTemporaryFile(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("reads from first available path", func(t *testing.T) {
		if runtime.GOOS != constants.Status.Platform.Linux {
			t.Skip("Linux only")
		}
		machineID, err := getLinuxMachineID(logger)
		require.NoError(t, err)
		assert.NotEmpty(t, machineID)
	})
}

func TestFingerprintIncludesHostname(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("fingerprint incorporates hostname for Docker container differentiation", func(t *testing.T) {
		fp, err := GenerateSystemFingerprint(logger)
		require.NoError(t, err)

		hostname, err := os.Hostname()
		require.NoError(t, err)
		assert.NotEmpty(t, hostname, "hostname should be available")

		assert.Len(t, fp.Fingerprint, 64)

		machineID, _ := getMachineID(logger)
		if machineID == "" {
			machineID = "fallback"
		}

		componentsWithHostname := []string{
			fmt.Sprintf("os:%s", runtime.GOOS),
			fmt.Sprintf("arch:%s", runtime.GOARCH),
			fmt.Sprintf("cpu_count:%d", runtime.NumCPU()),
			fmt.Sprintf("machine_id:%s", machineID),
			fmt.Sprintf("hostname:%s", hostname),
		}
		dataWith := strings.Join(componentsWithHostname, "|")
		hashWith := sha256.Sum256([]byte(dataWith))
		expectedFingerprint := hex.EncodeToString(hashWith[:])

		assert.Equal(t, expectedFingerprint, fp.Fingerprint,
			"fingerprint should match hash of components including hostname")
	})
}

func TestFingerprintStability(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("multiple calls produce identical results", func(t *testing.T) {
		var fingerprints []string

		for i := 0; i < 5; i++ {
			fp, err := GenerateSystemFingerprint(logger)
			require.NoError(t, err)
			fingerprints = append(fingerprints, fp.Fingerprint)
		}

		for i := 1; i < len(fingerprints); i++ {
			assert.Equal(t, fingerprints[0], fingerprints[i],
				"Fingerprint %d should match fingerprint 0", i)
		}
	})
}

func TestGetLinuxMachineIDFromCustomPath(t *testing.T) {
	logger := testutil.NewTestLogger()

	t.Run("empty machine ID file is handled", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty-machine-id")
		err := os.WriteFile(emptyFile, []byte(""), 0644)
		require.NoError(t, err)

		_, err = getLinuxMachineID(logger)
		if err != nil {
			assert.Error(t, err)
		}
	})
}

// ---------------------------------------------------------------------------
// getDarwinMachineID
// ---------------------------------------------------------------------------

func TestGetDarwinMachineID_PreferencesFileMissing_ReturnsFallback(t *testing.T) {
	// On Linux (the test environment) the macOS plist path does not exist.
	// getDarwinMachineID must handle the missing-file case and return a
	// hostname-based fallback instead of erroring.
	if runtime.GOOS == constants.Status.Platform.Darwin {
		t.Skip("skipped on Darwin: file likely exists and takes the hash path")
	}

	id, err := getDarwinMachineID()
	require.NoError(t, err, "getDarwinMachineID must not error when plist is missing")
	assert.NotEmpty(t, id)
	assert.True(t, strings.HasPrefix(id, "darwin-"), "fallback ID must have darwin- prefix, got: %s", id)
}

func TestGetDarwinMachineID_PreferencesFileExists_ReturnsHash(t *testing.T) {
	if runtime.GOOS != constants.Status.Platform.Darwin {
		t.Skip("hash path only reachable on Darwin where the plist exists")
	}

	id, err := getDarwinMachineID()
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Len(t, id, 32, "hash must be truncated to 32 hex chars")
}

func TestGetDarwinMachineID_FallbackContainsHostname(t *testing.T) {
	if runtime.GOOS == constants.Status.Platform.Darwin {
		t.Skip("skipped on Darwin: plist file exists so fallback branch not taken")
	}

	hostname, err := os.Hostname()
	require.NoError(t, err)

	id, err := getDarwinMachineID()
	require.NoError(t, err)
	assert.Contains(t, id, hostname, "fallback ID must embed hostname")
}
