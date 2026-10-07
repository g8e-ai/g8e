// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatorRolesValidateNormalizeAndRejectUnknownValues(t *testing.T) {
	var roles OperatorRoles
	require.NoError(t, roles.Set("observer,data,inference"))
	require.NoError(t, roles.Set("provenance,embedded,data"))
	require.Equal(t, "embedded,data,inference,provenance,observer", roles.String())
	before := roles.String()
	for _, invalid := range []string{"cloud", "system", "node", "", "data,invalid"} {
		require.ErrorIs(t, roles.Set(invalid), ErrOperatorRoleInvalid)
		require.Equal(t, before, roles.String())
	}
	var decoded OperatorRoles
	require.NoError(t, json.Unmarshal([]byte(`["observer","data","observer"]`), &decoded))
	require.Equal(t, "data,observer", decoded.String())
	require.ErrorIs(t, json.Unmarshal([]byte(`["cloud"]`), &decoded), ErrOperatorRoleInvalid)
}
