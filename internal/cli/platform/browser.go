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

package platform

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
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
		return fmt.Errorf("failed to open browser: URL cannot be empty")
	}

	// Validate URL format
	if _, err := url.Parse(urlStr); err != nil {
		return fmt.Errorf("failed to open browser: invalid URL: %w", err)
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
		return fmt.Errorf("failed to open browser: %w", err)
	}

	return nil
}
