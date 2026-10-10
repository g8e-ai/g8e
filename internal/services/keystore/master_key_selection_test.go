// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestResolveMasterKeyFile(t *testing.T) {
	for _, tc := range []struct{ explicit, env, want string }{
		{"  /explicit  ", "/environment", "/explicit"},
		{"", " /environment ", "/environment"},
		{" \t", " /environment ", "/environment"},
		{" ", " ", ""},
		{"invalid", "/environment", "invalid"},
	} {
		t.Setenv(string(constants.EnvVar.MasterKeyFile), tc.env)
		require.Equal(t, tc.want, ResolveMasterKeyFile(tc.explicit))
	}
}
