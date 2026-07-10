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

package scripts

import (
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderBeforeInit and TestInitAndRender must NOT run in parallel because
// they share package-level template state guarded by sync.Once.

func TestRenderLinuxDeployScript_BeforeInitReturnsNotInitializedError(t *testing.T) {
	// Guard: if Init was already called by another test, skip this test.
	if linuxTemplate != nil {
		t.Skip("templates already initialized by a prior test")
	}

	_, err := RenderLinuxDeployScript(TemplateData{
		GatewayHost: "10.0.0.1",
		GatewayPort: "8080",
	})
	assert.ErrorIs(t, err, constants.ErrScriptTemplateNotInitialized)
}

func TestRenderWindowsDeployScript_BeforeInitReturnsNotInitializedError(t *testing.T) {
	if windowsTemplate != nil {
		t.Skip("templates already initialized by a prior test")
	}

	_, err := RenderWindowsDeployScript(TemplateData{
		GatewayHost: "10.0.0.1",
		GatewayPort: "8080",
	})
	assert.ErrorIs(t, err, constants.ErrScriptTemplateNotInitialized)
}

func TestInit_ParsesEmbeddedTemplatesSuccessfully(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))

	err := Init(logger)
	require.NoError(t, err)

	// Verify templates are now populated
	require.NotNil(t, linuxTemplate)
	require.NotNil(t, windowsTemplate)
}

func TestInit_IsIdempotentViaSyncOnce(t *testing.T) {
	// sync.Once ensures Init only runs once. Calling it again should
	// return the same (nil) error without panicking.
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))

	// Use a separate once to prove the pattern: the package-level initOnce
	// has already fired from TestInit_ParsesEmbeddedTemplatesSuccessfully.
	var once sync.Once
	once.Do(func() {})
	once.Do(func() { t.Fatal("sync.Once should not fire twice") })

	err := Init(logger)
	assert.NoError(t, err)
}

func TestRenderLinuxDeployScript_SubstitutesGatewayHostAndPort(t *testing.T) {
	data := TemplateData{
		GatewayHost: "192.168.1.100",
		GatewayPort: "8080",
	}

	out, err := RenderLinuxDeployScript(data)
	require.NoError(t, err)

	assert.Contains(t, out, "192.168.1.100")
	assert.Contains(t, out, "8080")
}

func TestRenderWindowsDeployScript_SubstitutesGatewayHostAndPort(t *testing.T) {
	data := TemplateData{
		GatewayHost: "192.168.1.100",
		GatewayPort: "8080",
	}

	out, err := RenderWindowsDeployScript(data)
	require.NoError(t, err)

	assert.Contains(t, out, "192.168.1.100")
	assert.Contains(t, out, "8080")
}

func TestRenderLinuxDeployScript_EmptyDataProducesValidScript(t *testing.T) {
	out, err := RenderLinuxDeployScript(TemplateData{})
	require.NoError(t, err)

	assert.NotEmpty(t, out)
	assert.Contains(t, out, "#!/bin/bash")
}

func TestRenderWindowsDeployScript_EmptyDataProducesValidScript(t *testing.T) {
	out, err := RenderWindowsDeployScript(TemplateData{})
	require.NoError(t, err)

	assert.NotEmpty(t, out)
	assert.Contains(t, out, "Deploying g8e")
}

func TestRenderLinuxDeployScript_TemplateContainsDeployLogic(t *testing.T) {
	out, err := RenderLinuxDeployScript(TemplateData{
		GatewayHost: "10.0.0.1",
		GatewayPort: "8080",
	})
	require.NoError(t, err)

	assert.Contains(t, out, "operator run")
	assert.Contains(t, out, "10.0.0.1")
}

func TestRenderWindowsDeployScript_TemplateContainsDeployLogic(t *testing.T) {
	out, err := RenderWindowsDeployScript(TemplateData{
		GatewayHost: "10.0.0.1",
		GatewayPort: "8080",
	})
	require.NoError(t, err)

	assert.Contains(t, out, "g8e.exe")
	assert.Contains(t, out, "10.0.0.1")
}
