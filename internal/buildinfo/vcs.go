// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package buildinfo

import "runtime/debug"

// VCSStamp is the source-control identity the Go toolchain embeds in the
// binary at build time (go build -buildvcs, on by default for main
// packages inside a work tree). It is always available at runtime and
// needs no ldflags plumbing.
type VCSStamp struct {
	Revision string
	Time     string
	Modified bool
	Present  bool
}

// ReadVCSStamp returns the VCS identity embedded in the running binary.
// Present is false for binaries built outside a work tree (plain module
// or Docker source-copy builds) and for non-main packages under test.
func ReadVCSStamp() VCSStamp {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return VCSStamp{}
	}
	var stamp VCSStamp
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			stamp.Revision = setting.Value
		case "vcs.time":
			stamp.Time = setting.Value
		case "vcs.modified":
			stamp.Modified = setting.Value == "true"
		}
	}
	stamp.Present = stamp.Revision != ""
	return stamp
}
