// Copyright (c) 2026 Lateralus Labs, LLC.

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
