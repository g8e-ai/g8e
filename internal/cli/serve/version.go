// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

// VersionInfo holds build-time version and provenance metadata passed
// from cmd/g8e/main.go. SourceRevision and SourceTreeStateHash are stamped
// by the Makefile (full commit hash and the canonical source-tree digest);
// a plain `go build` leaves them at their "unknown" defaults while the
// toolchain's own vcs stamping still covers SourceRevision.
type VersionInfo struct {
	Version             string
	BuildID             string
	BuildTime           string
	Platform            string
	SourceRevision      string
	SourceTreeStateHash string
}
