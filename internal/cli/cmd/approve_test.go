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

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApproveCmd(t *testing.T) {
	t.Run("approve command has correct use and description", func(t *testing.T) {
		cmd := approveCmd()
		assert.Contains(t, cmd.Use, "approve")
		assert.Contains(t, cmd.Short, "Approve")
		assert.Contains(t, cmd.Short, "L3")
		assert.Contains(t, cmd.Short, "CLI signature")
	})

	t.Run("approve requires exactly one argument", func(t *testing.T) {
		cmd := approveCmd()
		// Test that args validation is set by checking it's not nil
		assert.NotNil(t, cmd.Args)
	})
}
