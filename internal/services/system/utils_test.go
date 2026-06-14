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

package system

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringPtr(t *testing.T) {
	t.Parallel()
	s := "test"
	ptr := StringPtr(s)
	require.NotNil(t, ptr)
	assert.Equal(t, "test", *ptr)
}

func TestStringPtrValue(t *testing.T) {
	t.Parallel()
	s := "hello"
	assert.Equal(t, "hello", StringPtrValue(&s))
	assert.Equal(t, "<nil>", StringPtrValue(nil))
}

func TestIntPtr(t *testing.T) {
	t.Parallel()
	i := 42
	ptr := IntPtr(i)
	require.NotNil(t, ptr)
	assert.Equal(t, 42, *ptr)
}

func TestIntPtrValue(t *testing.T) {
	t.Parallel()
	i := 7
	assert.Equal(t, "7", IntPtrValue(&i))
	assert.Equal(t, "<nil>", IntPtrValue(nil))
}

func TestRealClock_Now(t *testing.T) {
	t.Parallel()
	c := &RealClock{}
	now := c.Now()
	assert.False(t, now.IsZero())
	assert.Equal(t, time.UTC, now.Location())
}

func TestFixedClock_Now(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	c := NewFixedClock(fixedTime)
	now := c.Now()
	assert.Equal(t, fixedTime, now)
}
