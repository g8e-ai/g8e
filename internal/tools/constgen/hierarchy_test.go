// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateOperatorHierarchy(t *testing.T) {
	hierarchy, err := loadOperatorHierarchy("../../..")
	require.NoError(t, err)
	out := generateOperatorHierarchy(hierarchy)
	require.Contains(t, out, "type _EventOperator struct {")
	require.Contains(t, out, "var Event = struct {")
	require.NotContains(t, out, "var Event = struct { _EventOperator")
	require.True(t, strings.HasSuffix(strings.TrimSpace(out), "}"))
}
