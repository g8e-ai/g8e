package cmd

import (
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"log/slog"
)

// newFileSvc creates a RuntimeFileService rooted at the current working directory
// (the standard .g8e/ location). It is the canonical way to obtain a fileSvc
// in cmd RunE functions.
func newFileSvc() (fs.RuntimeFileService, error) {
	return fs.NewRuntimeFileService("", slog.Default())
}

// newFileSvcOrErr creates a RuntimeFileService and wraps the error with
// constants.ErrFileServiceInit if creation fails.
func newFileSvcOrErr() (fs.RuntimeFileService, error) {
	fileSvc, err := newFileSvc()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	return fileSvc, nil
}
