// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux

package keystore

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

type externalKeyFileInfo struct {
	os.FileInfo
	uid  uint32
	mode os.FileMode
}

func (i externalKeyFileInfo) Mode() os.FileMode { return i.mode }
func (i externalKeyFileInfo) Sys() any          { return &syscall.Stat_t{Uid: i.uid} }

func TestExternalKeyMetadata_OwnerAndMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		uid   uint32
		mode  os.FileMode
		valid bool
	}{
		{"service owner", uint32(os.Geteuid()), 0600, true},
		{"root provisioned", 0, 0400, true},
		{"foreign owner", uint32(os.Geteuid()) + 1, 0600, false},
		{"group readable", 0, 0640, false},
		{"world readable", 0, 0444, false},
		{"owner cannot read", 0, 0200, false},
		{"special bits", 0, 0600 | os.ModeSetuid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExternalKeyMetadata(externalKeyFileInfo{uid: tc.uid, mode: tc.mode})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constants.ErrKeyStoreExternalFileInvalid)
			}
		})
	}
}
