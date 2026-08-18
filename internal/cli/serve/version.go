// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

// VersionInfo holds build-time version metadata passed from cmd/g8e/main.go.
type VersionInfo struct {
	Version   string
	BuildID   string
	BuildTime string
	Platform  string
}
