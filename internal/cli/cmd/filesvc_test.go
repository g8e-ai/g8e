// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileSvc_ReturnsNonNilService(t *testing.T) {
	svc, err := newFileSvc("", slog.Default())
	require.NoError(t, err)
	assert.NotNil(t, svc)
}
