// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// FsGrepMatch represents a single grep match result
type FsGrepMatch struct {
	Path       string   `json:"path"`
	LineNumber int      `json:"line_number"`
	Content    string   `json:"content"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}
