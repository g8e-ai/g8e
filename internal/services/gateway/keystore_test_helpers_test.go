// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
)

// newTestKeystore creates an initialized keystore using an in-memory keyring.
func newTestKeystore(tb testing.TB, fileSvc fs.RuntimeFileService, logger *slog.Logger) *keystore.Keystore {
	tb.Helper()
	return newTestKeystoreWithKeyring(tb, fileSvc, logger, keystoretest.NewMemoryKeyring())
}

func newTestKeystoreWithKeyring(tb testing.TB, fileSvc fs.RuntimeFileService, logger *slog.Logger, keyring keystore.Keyring) *keystore.Keystore {
	tb.Helper()
	ks, err := keystore.NewWithKeyringAndFS(logger, keyring, fileSvc)
	require.NoError(tb, err)
	require.NoError(tb, ks.Initialize())
	require.NoError(tb, ks.EnforcePermissions())
	return ks
}
