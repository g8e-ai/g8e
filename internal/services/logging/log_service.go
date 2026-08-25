// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// LogService owns the platform log file lifecycle: path resolution, file
// open (via RuntimeFileService), and logger configuration. Daemon-mode
// callers (RunGateway, StartOperator) go through LogService for all
// .g8e/logs/g8e.log I/O. CLI client commands use NewStdoutLogger directly
// and do not construct a LogService.
type LogService struct {
	fileSvc fs.RuntimeFileService
}

// NewLogService constructs a LogService backed by the given file service.
func NewLogService(fileSvc fs.RuntimeFileService) *LogService {
	return &LogService{fileSvc: fileSvc}
}

// ConfigureFileLogger opens g8e.log via the file service and returns a
// slog.Logger writing to it. The caller must close the returned handle
// on shutdown. Ensures the log directory exists.
func (s *LogService) ConfigureFileLogger(ctx context.Context, level string) (*slog.Logger, *os.File, error) {
	handle, err := s.OpenLogForAppend(ctx)
	if err != nil {
		return nil, nil, err
	}
	logger, err := NewLogger(level, handle)
	if err != nil {
		_ = handle.Close()
		return nil, nil, fmt.Errorf("logging: configure logger: %w", err)
	}
	return logger, handle, nil
}

// OpenLogForAppend opens g8e.log for append via the file service, without
// configuring a logger. Used by the process manager to capture stdout/stderr
// of the re-exec'd child process.
func (s *LogService) OpenLogForAppend(ctx context.Context) (*os.File, error) {
	if err := s.fileSvc.MkdirAll(ctx, constants.LogDirname, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("logging: create log dir: %w", err)
	}
	relPath := filepath.Join(constants.LogDirname, constants.G8eLogFilename)
	handle, err := s.fileSvc.OpenForAppend(ctx, relPath, constants.PermFilePrivate)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file: %w", err)
	}
	return handle, nil
}

// OpenLogForRead opens g8e.log for streaming read (tail/follow). Returns
// constants.ErrNotFound if no log file exists yet.
func (s *LogService) OpenLogForRead(ctx context.Context) (*os.File, error) {
	relPath := filepath.Join(constants.LogDirname, constants.G8eLogFilename)
	handle, err := s.fileSvc.OpenForRead(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("logging: open log for read: %w", err)
	}
	return handle, nil
}

// LogFileExists checks whether g8e.log exists. Returns false, nil if absent.
func (s *LogService) LogFileExists(ctx context.Context) (bool, error) {
	return s.fileSvc.FileExists(ctx, filepath.Join(constants.LogDirname, constants.G8eLogFilename))
}

// LogFilePath returns the absolute path to g8e.log. Used for error messages
// and diagnostics, not for file I/O (use OpenLogForAppend / OpenLogForRead).
func (s *LogService) LogFilePath() string {
	return s.fileSvc.Resolve(filepath.Join(constants.LogDirname, constants.G8eLogFilename))
}
