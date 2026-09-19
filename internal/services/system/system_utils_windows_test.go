// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCPUPercent(t *testing.T) {
	t.Parallel()

	cpuPercent := GetCPUPercent()

	assert.GreaterOrEqual(t, cpuPercent, 0.0)
	assert.LessOrEqual(t, cpuPercent, 100.0)
	rounded := float64(int(cpuPercent*100+0.5)) / 100
	assert.InDelta(t, cpuPercent, rounded, 0.005, "GetCPUPercent() should be rounded to 2 decimal places")
}

func TestReadSystemCPUTimes(t *testing.T) {
	t.Parallel()

	idle, total, ok := readSystemCPUTimes()
	assert.True(t, ok)
	assert.NotZero(t, total)
	assert.LessOrEqual(t, idle, total)
}
