package cmd

import (
	"github.com/g8e-ai/g8e/internal/services/fs"
	"log/slog"
)

// newFileSvc creates a RuntimeFileService rooted at the current working directory
// (the standard .g8e/ location). It is the canonical way to obtain a fileSvc
// in cmd RunE functions.
func newFileSvc() (fs.RuntimeFileService, error) {
	return fs.NewRuntimeFileService("", slog.Default())
}
