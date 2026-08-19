// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// NewLogger returns a slog.Logger writing to the given writer with g8e
// log formatting. Used by LogService.ConfigureFileLogger and by CLI
// clients that log to stdout.
func NewLogger(level string, output io.Writer) (*slog.Logger, error) {
	parsedLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, fmt.Errorf("logging: parse level: %w", err)
	}
	return slog.New(newLogHandler(output, parsedLevel)), nil
}

// NewStdoutLogger returns a slog.Logger writing to stdout. Used by CLI
// client commands (g8e operator, g8e auth, etc.) that do not run as
// daemons and do not write to .g8e/logs/g8e.log.
func NewStdoutLogger(level string) (*slog.Logger, error) {
	return NewLogger(level, os.Stdout)
}
