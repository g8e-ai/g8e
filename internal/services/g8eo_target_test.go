// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
)

func TestG8eoService_ExecutionTarget(t *testing.T) {
	svc := &G8eoService{config: &config.Config{OperatorID: "operator-1"}}
	require.True(t, svc.ExecutesFor("operator-1"))
	require.False(t, svc.ExecutesFor("operator-2"))
	require.False(t, svc.ExecutesFor(""))
	require.False(t, (&G8eoService{}).ExecutesFor("operator-1"))
	require.False(t, (*G8eoService)(nil).ExecutesFor("operator-1"))
	svc.config.OperatorID = ""
	require.False(t, svc.ExecutesFor(""), "an unenrolled runtime owns no execution target")
}
