// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scripts

import (
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

//go:embed g8e-deploy.sh
var deployScriptLinux string

//go:embed g8e-deploy.ps1
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
	initOnce        sync.Once
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
			initErr = fmt.Errorf("%w: %v", constants.ErrScriptTemplateParseFailed, err)
			return
		}

		windowsTemplate, err = template.New("deploy_windows").Parse(deployScriptWindows)
		if err != nil {
			initErr = fmt.Errorf("%w: %v", constants.ErrScriptTemplateParseFailed, err)
			return
		}

		logger.Info("Script templates initialized successfully")
	})
	return initErr
}

// RenderLinuxDeployScript renders the Linux deploy script with the given data.
func RenderLinuxDeployScript(data TemplateData) (string, error) {
	if linuxTemplate == nil {
		return "", constants.ErrScriptTemplateNotInitialized
	}

	var buf strings.Builder
	if err := linuxTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrScriptTemplateRenderFailed, err)
	}

	return buf.String(), nil
}

// RenderWindowsDeployScript renders the Windows deploy script with the given data.
func RenderWindowsDeployScript(data TemplateData) (string, error) {
	if windowsTemplate == nil {
		return "", constants.ErrScriptTemplateNotInitialized
	}

	var buf strings.Builder
	if err := windowsTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: %v", constants.ErrScriptTemplateRenderFailed, err)
	}

	return buf.String(), nil
}
