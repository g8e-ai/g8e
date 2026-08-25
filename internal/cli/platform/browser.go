// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// browserCommandExecutor is an interface for executing commands, allowing dependency injection for testing.
type browserCommandExecutor interface {
	start(name string, args ...string) error
}

// realBrowserCommandExecutor is the production implementation that uses os/exec.
type realBrowserCommandExecutor struct{}

func (e *realBrowserCommandExecutor) start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

var defaultBrowserExecutor browserCommandExecutor = &realBrowserCommandExecutor{}

// OpenBrowser opens the default web browser to the specified URL.
func OpenBrowser(urlStr string) error {
	return openBrowserWithExecutor(urlStr, defaultBrowserExecutor)
}

// openBrowserWithExecutor opens the browser using the provided command executor.
func openBrowserWithExecutor(urlStr string, executor browserCommandExecutor) error {
	if urlStr == "" {
		return constants.ErrBrowserURLEmpty
	}

	// Validate URL format
	if _, err := url.Parse(urlStr); err != nil {
		return fmt.Errorf("platform: parse URL: %w", err)
	}

	var cmdName string
	var cmdArgs []string

	switch runtime.GOOS {
	case "windows":
		cmdName = "rundll32"
		cmdArgs = []string{"url.dll,FileProtocolHandler", urlStr}
	case "darwin":
		cmdName = "open"
		cmdArgs = []string{urlStr}
	default: // linux, bsd, etc.
		cmdName = "xdg-open"
		cmdArgs = []string{urlStr}
	}

	if err := executor.start(cmdName, cmdArgs...); err != nil {
		return fmt.Errorf("platform: start browser: %w", err)
	}

	return nil
}
