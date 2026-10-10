// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux

package keystore

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

type recordedCall struct {
	args  []string
	stdin []byte
}

func TestLibsecretKeyring_StoreMasterKey_KeyOnStdinNeverArgv(t *testing.T) {
	t.Parallel()

	var calls []recordedCall
	fakeRunner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		var inBytes []byte
		if stdin != nil {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, stdin)
			inBytes = buf.Bytes()
		}
		calls = append(calls, recordedCall{args: args, stdin: inBytes})
		return nil, nil, nil
	}

	kr := &libsecretKeyring{run: fakeRunner}
	assert.Equal(t, "libsecret", kr.Name())

	testKey := make([]byte, vault.KeySize)
	for i := range testKey {
		testKey[i] = byte(i + 1)
	}
	encodedKey := base64.StdEncoding.EncodeToString(testKey)

	err := kr.StoreMasterKey(testKey)
	require.NoError(t, err)
	require.Len(t, calls, 1)

	call := calls[0]
	// Assert key is on stdin
	assert.Equal(t, encodedKey, string(call.stdin), "expected base64 master key on stdin")

	// Assert key is NEVER in argv (neither raw bytes nor base64)
	for _, arg := range call.args {
		assert.False(t, strings.Contains(arg, encodedKey), "master key base64 leaked in argv: %s", arg)
		assert.False(t, strings.Contains(arg, string(testKey)), "raw master key leaked in argv: %s", arg)
	}

	// Assert store args are attribute/value pairs
	// Expected: store --label=g8e-platform <attr1> <val1>
	require.GreaterOrEqual(t, len(call.args), 3)
	assert.Equal(t, "store", call.args[0])
	assert.Equal(t, "--label="+keyStoreName, call.args[1])

	// The remaining args after options must be paired attribute/value
	attrArgs := call.args[2:]
	require.Equal(t, 0, len(attrArgs)%2, "attributes and values must be in pairs, got %d items", len(attrArgs))
	assert.Equal(t, keyStoreName, attrArgs[0])
	assert.Equal(t, masterKeyName, attrArgs[1])
}

func TestLibsecretKeyring_RetrieveMasterKey_Success(t *testing.T) {
	t.Parallel()

	testKey := make([]byte, vault.KeySize)
	for i := range testKey {
		testKey[i] = byte(i + 10)
	}
	encodedKey := base64.StdEncoding.EncodeToString(testKey)

	var calls []recordedCall
	fakeRunner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		calls = append(calls, recordedCall{args: args})
		return []byte(encodedKey + "\n"), nil, nil
	}

	kr := &libsecretKeyring{run: fakeRunner}
	retrieved, err := kr.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrieved)

	require.Len(t, calls, 1)
	assert.Equal(t, []string{"lookup", keyStoreName, masterKeyName}, calls[0].args)
}

func TestLibsecretKeyring_RetrieveMasterKey_NotFound(t *testing.T) {
	t.Parallel()

	fakeRunner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		// secret-tool exits with code 1 and no stderr when not found
		return nil, nil, fakeExitError(1)
	}

	kr := &libsecretKeyring{run: fakeRunner}
	_, err := kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
}

func TestLibsecretKeyring_RetrieveMasterKey_ServiceDown(t *testing.T) {
	t.Parallel()

	fakeRunner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		// secret-tool exits with diagnostic on stderr when service is down
		return nil, []byte("Cannot autolaunch D-Bus without X11 $DISPLAY"), fakeExitError(1)
	}

	kr := &libsecretKeyring{run: fakeRunner}
	_, err := kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreSecretServiceDown)
}

func TestLibsecretKeyring_DeleteMasterKey_Success(t *testing.T) {
	t.Parallel()

	var calls []recordedCall
	fakeRunner := func(stdin io.Reader, args ...string) ([]byte, []byte, error) {
		calls = append(calls, recordedCall{args: args})
		return nil, nil, nil
	}

	kr := &libsecretKeyring{run: fakeRunner}
	err := kr.DeleteMasterKey()
	require.NoError(t, err)

	require.Len(t, calls, 1)
	assert.Equal(t, []string{"clear", keyStoreName, masterKeyName}, calls[0].args)
}

func fakeExitError(exitCode int) error {
	// exec.Command("sh", "-c", "exit <code").Run() gives a real exec.ExitError
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", exitCode))
	return cmd.Run()
}

func TestLibsecretKeyring_CorruptLookupDoesNotMeanMissing(t *testing.T) {
	for _, data := range []string{"", "bad!", base64.StdEncoding.EncodeToString(make([]byte, 16))} {
		kr := &libsecretKeyring{run: func(io.Reader, ...string) ([]byte, []byte, error) { return []byte(data), nil, nil }}
		_, err := kr.RetrieveMasterKey()
		require.Error(t, err)
		require.NotErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)
	}
}
