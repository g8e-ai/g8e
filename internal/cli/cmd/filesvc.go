package cmd

import (
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// newFileSvc is the canonical file service factory for cmd RunE functions.
// It is a package-level var so tests can swap it for a mock factory.
var newFileSvc = fs.NewRuntimeFileService
