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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// secretToolTimeout bounds each secret-tool call; D-Bus activation of a
// missing or locked Secret Service can otherwise block startup indefinitely.
const secretToolTimeout = 15 * time.Second

// secretToolRunner runs secret-tool with args, feeding stdin, and returns its
// stdout, stderr, and exit error. Tests replace it to observe the exact argv.
type secretToolRunner func(stdin io.Reader, args ...string) (stdout, stderr []byte, err error)

func runSecretTool(stdin io.Reader, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), secretToolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// libsecretKeyring stores the master key in the Secret Service (GNOME Keyring,
// KWallet) through secret-tool. The item is identified by the single attribute
// pair keyStoreName=masterKeyName.
type libsecretKeyring struct {
	run secretToolRunner
}

// newLibsecretKeyring returns a libsecret keyring only when secret-tool is
// installed and the Secret Service answers a lookup. Headless hosts without a
// D-Bus session fail here rather than on first store.
func newLibsecretKeyring(run secretToolRunner) (Keyring, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, fmt.Errorf("%w: secret-tool not found (install libsecret-tools): %w", constants.ErrKeyStoreSecretServiceDown, err)
	}
	k := &libsecretKeyring{run: run}
	key, err := k.RetrieveMasterKey()
	vault.SecureZero(key)
	if err != nil && !errors.Is(err, constants.ErrKeyStoreKeyNotFound) {
		return nil, err
	}
	if errors.Is(err, constants.ErrKeyStoreKeyNotFound) {
		probeKey := "probe-session-check"
		_, stderr, storeErr := run(strings.NewReader("probe"), "store", "--label=probe", keyStoreName, probeKey)
		if storeErr != nil {
			return nil, fmt.Errorf("%w: Secret Service not writable: %w: %s", constants.ErrKeyStoreSecretServiceDown, storeErr, strings.TrimSpace(string(stderr)))
		}
		_, _, _ = run(nil, "clear", keyStoreName, probeKey)
	}
	return k, nil
}

func (l *libsecretKeyring) Name() string {
	return "libsecret"
}

func (l *libsecretKeyring) RetrieveMasterKey() ([]byte, error) {
	stdout, stderr, err := l.run(nil, "lookup", keyStoreName, masterKeyName)
	if err != nil {
		// secret-tool exits 1 with no output when the item does not exist,
		// and exits 1 with a diagnostic on stderr when the service fails.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(bytes.TrimSpace(stderr)) == 0 {
			return nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, fmt.Errorf("%w: lookup master key: %w: %s", constants.ErrKeyStoreSecretServiceDown, err, strings.TrimSpace(string(stderr)))
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(stdout)))
	if err != nil {
		return nil, fmt.Errorf("%w: libsecret: %w", constants.ErrKeyStoreDecodeFailed, err)
	}
	if len(key) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	return key, nil
}

// StoreMasterKey passes the secret on stdin, which is where secret-tool reads
// it from; it must never appear in argv.
func (l *libsecretKeyring) StoreMasterKey(key []byte) error {
	encoded := base64.StdEncoding.EncodeToString(key)
	_, stderr, err := l.run(strings.NewReader(encoded), "store", "--label="+keyStoreName, keyStoreName, masterKeyName)
	if err != nil {
		return fmt.Errorf("%w: libsecret: %w: %s", constants.ErrKeyStoreStoreFailed, err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

func (l *libsecretKeyring) DeleteMasterKey() error {
	_, stderr, err := l.run(nil, "clear", keyStoreName, masterKeyName)
	if err != nil {
		// clear exits non-zero with no diagnostic when nothing matched.
		if len(bytes.TrimSpace(stderr)) == 0 {
			return nil
		}
		return fmt.Errorf("%w: libsecret: %w: %s", constants.ErrKeyStoreDeleteFailed, err, strings.TrimSpace(string(stderr)))
	}
	return nil
}
