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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPorts(t *testing.T) {
	t.Run("operator http port has correct value", func(t *testing.T) {
		assert.Equal(t, 8080, Ports.OperatorHttp)
	})

	t.Run("operator https port has correct value", func(t *testing.T) {
		assert.Equal(t, 8443, Ports.OperatorHttps)
	})

	t.Run("local http stdio gateway port has correct value", func(t *testing.T) {
		assert.Equal(t, 18789, Ports.LocalHttpStdioGateway)
	})

	t.Run("all ports are in valid range", func(t *testing.T) {
		assert.GreaterOrEqual(t, Ports.OperatorHttp, 1)
		assert.LessOrEqual(t, Ports.OperatorHttp, 65535)
		assert.GreaterOrEqual(t, Ports.OperatorHttps, 1)
		assert.LessOrEqual(t, Ports.OperatorHttps, 65535)
		assert.GreaterOrEqual(t, Ports.LocalHttpStdioGateway, 1)
		assert.LessOrEqual(t, Ports.LocalHttpStdioGateway, 65535)
	})

	t.Run("ports are distinct", func(t *testing.T) {
		assert.NotEqual(t, Ports.OperatorHttp, Ports.OperatorHttps)
		assert.NotEqual(t, Ports.OperatorHttp, Ports.LocalHttpStdioGateway)
		assert.NotEqual(t, Ports.OperatorHttps, Ports.LocalHttpStdioGateway)
	})
}
