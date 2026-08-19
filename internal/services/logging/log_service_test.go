// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package logging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFileSvc is a minimal RuntimeFileService stub for LogService unit tests.
// It records calls and returns canned responses. Methods not exercised by
// LogService panic to catch unexpected usage.
type stubFileSvc struct {
	mkdirErr       error
	openAppendErr  error
	openAppendFile *os.File
	openReadErr    error
	openReadFile   *os.File
	fileExists     bool
	fileExistsErr  error
	resolvePath    string
}

func (s *stubFileSvc) MkdirAll(ctx context.Context, relPath string, mode os.FileMode) error {
	if s.mkdirErr != nil {
		return s.mkdirErr
	}
	return nil
}

func (s *stubFileSvc) CreateRuntimeTree(ctx context.Context) error { panic("unexpected") }

func (s *stubFileSvc) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	panic("unexpected")
}

func (s *stubFileSvc) FileExists(ctx context.Context, relPath string) (bool, error) {
	return s.fileExists, s.fileExistsErr
}

func (s *stubFileSvc) Stat(ctx context.Context, relPath string) (os.FileInfo, error) {
	panic("unexpected")
}

func (s *stubFileSvc) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	panic("unexpected")
}

func (s *stubFileSvc) OpenForAppend(ctx context.Context, relPath string, mode os.FileMode) (*os.File, error) {
	if s.openAppendErr != nil {
		return nil, s.openAppendErr
	}
	return s.openAppendFile, nil
}

func (s *stubFileSvc) OpenForRead(ctx context.Context, relPath string) (*os.File, error) {
	if s.openReadErr != nil {
		return nil, s.openReadErr
	}
	return s.openReadFile, nil
}

func (s *stubFileSvc) Remove(ctx context.Context, relPath string) error    { panic("unexpected") }
func (s *stubFileSvc) RemoveAll(ctx context.Context, relPath string) error { panic("unexpected") }
func (s *stubFileSvc) ReadDir(ctx context.Context, relPath string) ([]os.DirEntry, error) {
	panic("unexpected")
}
func (s *stubFileSvc) Rename(ctx context.Context, oldPath, newPath string) error { panic("unexpected") }
func (s *stubFileSvc) EnforceDirPermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	panic("unexpected")
}
func (s *stubFileSvc) EnforceFilePermissions(ctx context.Context, relPath string, mode os.FileMode) error {
	panic("unexpected")
}
func (s *stubFileSvc) Resolve(relPath string) string {
	if s.resolvePath != "" {
		return s.resolvePath
	}
	return filepath.Join("/runtime", relPath)
}
func (s *stubFileSvc) Rel(absPath string) (string, error)        { return absPath, nil }
func (s *stubFileSvc) RelFromAbs(absPath string) (string, error) { return absPath, nil }

// Compile-time assertion that stubFileSvc satisfies the interface.
var _ fs.RuntimeFileService = (*stubFileSvc)(nil)

// makeTempFile creates a real *os.File backed by a temp file so the stub
// can return a writable handle for OpenForAppend / OpenForRead.
func makeTempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "logservice-*.log")
	require.NoError(t, err)
	return f
}

func TestLogService_ConfigureFileLogger(t *testing.T) {
	f := makeTempFile(t)
	defer f.Close()

	svc := NewLogService(&stubFileSvc{openAppendFile: f})
	logger, handle, err := svc.ConfigureFileLogger(context.Background(), "info")
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.Same(t, f, handle)

	// The logger writes through newLogHandler to the file handle. We can't
	// easily read back from the same handle (append mode), but the call
	// should not error and should produce a non-nil logger.
	logger.Info("hello")
}

func TestLogService_ConfigureFileLogger_InvalidLevelClosesHandle(t *testing.T) {
	f := makeTempFile(t)

	svc := NewLogService(&stubFileSvc{openAppendFile: f})
	logger, handle, err := svc.ConfigureFileLogger(context.Background(), "trace")
	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Nil(t, handle)
	assert.Contains(t, err.Error(), "logging: configure logger:")
}

func TestLogService_OpenLogForAppend_MkdirError(t *testing.T) {
	mkdirErr := errors.New("disk full")
	svc := NewLogService(&stubFileSvc{mkdirErr: mkdirErr})

	_, err := svc.OpenLogForAppend(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logging: create log dir:")
	assert.ErrorIs(t, err, mkdirErr)
}

func TestLogService_OpenLogForAppend_OpenError(t *testing.T) {
	openErr := errors.New("permission denied")
	svc := NewLogService(&stubFileSvc{openAppendErr: openErr})

	_, err := svc.OpenLogForAppend(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logging: open log file:")
	assert.ErrorIs(t, err, openErr)
}

func TestLogService_OpenLogForRead_NotFound(t *testing.T) {
	svc := NewLogService(&stubFileSvc{openReadErr: constants.ErrNotFound})

	_, err := svc.OpenLogForRead(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotFound), "error should wrap constants.ErrNotFound")
}

func TestLogService_OpenLogForRead_OpenError(t *testing.T) {
	openErr := errors.New("io error")
	svc := NewLogService(&stubFileSvc{openReadErr: openErr})

	_, err := svc.OpenLogForRead(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logging: open log for read:")
	assert.ErrorIs(t, err, openErr)
}

func TestLogService_LogFileExists_True(t *testing.T) {
	svc := NewLogService(&stubFileSvc{fileExists: true})
	exists, err := svc.LogFileExists(context.Background())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestLogService_LogFileExists_False(t *testing.T) {
	svc := NewLogService(&stubFileSvc{fileExists: false})
	exists, err := svc.LogFileExists(context.Background())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestLogService_LogFileExists_Error(t *testing.T) {
	statErr := errors.New("stat failed")
	svc := NewLogService(&stubFileSvc{fileExistsErr: statErr})
	_, err := svc.LogFileExists(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, statErr)
}

func TestLogService_LogFilePath(t *testing.T) {
	svc := NewLogService(&stubFileSvc{})
	path := svc.LogFilePath()
	expected := filepath.Join("/runtime", constants.LogDirname, constants.G8eLogFilename)
	assert.Equal(t, expected, path)
}

func TestLogService_LogFilePath_UsesResolve(t *testing.T) {
	svc := NewLogService(&stubFileSvc{resolvePath: "/custom/root"})
	path := svc.LogFilePath()
	assert.Equal(t, "/custom/root", path)
}

// TestLogService_NewLoggerWritesToHandle verifies the end-to-end path:
// ConfigureFileLogger returns a logger that actually writes to the handle.
func TestLogService_NewLoggerWritesToHandle(t *testing.T) {
	// Use a pipe so we can read what the logger writes.
	r, w, err := os.Pipe()
	require.NoError(t, err)

	svc := NewLogService(&stubFileSvc{openAppendFile: w})
	logger, handle, err := svc.ConfigureFileLogger(context.Background(), "info")
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.Same(t, w, handle)

	logger.Info("pipe-test-message")
	_ = handle.Close()

	got, err := readAll(r)
	require.NoError(t, err)
	assert.Contains(t, got, "pipe-test-message")
}

// readAll drains the reader into a string.
func readAll(r *os.File) (string, error) {
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}
	return string(buf[:n]), nil
}
