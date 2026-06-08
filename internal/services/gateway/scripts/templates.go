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
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"
)

//go:embed deploy.sh
var deployScriptLinux string

//go:embed deploy.ps1
var deployScriptWindows string

// TemplateData holds the data for script template rendering.
type TemplateData struct {
	GatewayHost string
	GatewayPort string
}

// parsedTemplates holds the compiled templates.
var (
	linuxTemplate   *template.Template
	windowsTemplate *template.Template
	initOnce       sync.Once
)

// Init parses and validates the embedded scripts at startup.
// Call this during application initialization to fail fast if templates are invalid.
// It is safe to call this function from multiple goroutines.
func Init(logger *slog.Logger) error {
	var initErr error
	initOnce.Do(func() {
		var err error
		linuxTemplate, err = template.New("deploy_linux").Parse(deployScriptLinux)
		if err != nil {
			initErr = fmt.Errorf("failed to parse Linux deploy script template: %w", err)
			return
		}

		windowsTemplate, err = template.New("deploy_windows").Parse(deployScriptWindows)
		if err != nil {
			initErr = fmt.Errorf("failed to parse Windows deploy script template: %w", err)
			return
		}

		logger.Info("Script templates initialized successfully")
	})
	return initErr
}

// RenderLinuxDeployScript renders the Linux deploy script with the given data.
func RenderLinuxDeployScript(data TemplateData) (string, error) {
	if linuxTemplate == nil {
		return "", errors.New("Linux template not initialized - call Init() first")
	}

	var buf strings.Builder
	if err := linuxTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render Linux deploy script: %w", err)
	}

	return buf.String(), nil
}

// RenderWindowsDeployScript renders the Windows deploy script with the given data.
func RenderWindowsDeployScript(data TemplateData) (string, error) {
	if windowsTemplate == nil {
		return "", errors.New("Windows template not initialized - call Init() first")
	}

	var buf strings.Builder
	if err := windowsTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render Windows deploy script: %w", err)
	}

	return buf.String(), nil
}
