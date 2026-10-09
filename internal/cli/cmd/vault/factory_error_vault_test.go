// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package vaultcmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

var (
	errFactory         = fmt.Errorf("factory boom")
	errKeystoreFactory = fmt.Errorf("keystore factory boom")
)

func failingKeystoreFactory(_ fs.RuntimeFileService, _ keystore.Options, _ bool) (*keystore.Keystore, error) {
	return nil, errKeystoreFactory
}

func dummyKeystoreFactory(_ fs.RuntimeFileService, _ keystore.Options, _ bool) (*keystore.Keystore, error) {
	return nil, nil
}

func TestVaultInitCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultInitCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory), dummyKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultUnlockCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultUnlockCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory), dummyKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultRekeyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultRekeyCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory), dummyKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultStatusCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultResetCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultResetCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	cmd.Flags().Set("confirm", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultInitCmdWithConfig_KeystoreFactoryError(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	cmd := vaultInitCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), failingKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKeystoreFactory)
}

func TestVaultUnlockCmdWithConfig_KeystoreFactoryError(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	realKS, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, realKS.Initialize())
	require.NoError(t, realKS.InitVault())

	cmd := vaultUnlockCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), failingKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err = cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKeystoreFactory)
}

func TestVaultRekeyCmdWithConfig_KeystoreFactoryError(t *testing.T) {
	fileSvc, _ := cmdtest.NewCmdTestEnv(t)
	realKS, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, realKS.Initialize())
	require.NoError(t, realKS.InitVault())

	cmd := vaultRekeyCmdWithConfig(cmdtest.FileSvcFactoryFor(fileSvc), failingKeystoreFactory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err = cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errKeystoreFactory)
}
