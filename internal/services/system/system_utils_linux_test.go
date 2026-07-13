// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux
// +build linux

package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCPUStat(t *testing.T) {
	t.Parallel()
	stat, err := readCPUStat()

	require.NoError(t, err)
	require.NotNil(t, stat)

	assert.GreaterOrEqual(t, stat.user, int64(0))
	assert.GreaterOrEqual(t, stat.nice, int64(0))
	assert.GreaterOrEqual(t, stat.system, int64(0))
	assert.GreaterOrEqual(t, stat.idle, int64(0))
	assert.GreaterOrEqual(t, stat.iowait, int64(0))
	assert.GreaterOrEqual(t, stat.irq, int64(0))
	assert.GreaterOrEqual(t, stat.softirq, int64(0))

	total := stat.user + stat.nice + stat.system + stat.idle + stat.iowait + stat.irq + stat.softirq
	assert.Positive(t, total)
}

func TestCPUStatConsistency(t *testing.T) {
	t.Parallel()
	stat1, err := readCPUStat()
	require.NoError(t, err)

	stat2, err := readCPUStat()
	require.NoError(t, err)

	total1 := stat1.user + stat1.nice + stat1.system + stat1.idle + stat1.iowait + stat1.irq + stat1.softirq
	total2 := stat2.user + stat2.nice + stat2.system + stat2.idle + stat2.iowait + stat2.irq + stat2.softirq

	assert.GreaterOrEqual(t, total2, total1, "CPU time should be monotonically non-decreasing")
}

func TestReadOSReleaseField_KnownField(t *testing.T) {
	t.Parallel()
	name := readOSReleaseField("NAME")

	assert.NotEmpty(t, name)
	assert.NotEqual(t, "unknown", name)
}
