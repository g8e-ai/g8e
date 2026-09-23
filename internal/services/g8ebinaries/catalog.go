// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package g8ebinaries

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Target describes one artifact in the g8e deployment matrix.
type Target struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	Checksum string `json:"checksum"`
	Mode     uint32 `json:"mode"`
	FIPS     string `json:"fips_module,omitempty"`
}

var targets = []Target{
	{OS: "linux", Arch: "amd64", Filename: "g8e-linux-amd64", Checksum: "g8e-linux-amd64.sha256", Mode: constants.PermFileExecutable, FIPS: "v1.0.0"},
	{OS: "linux", Arch: "arm64", Filename: "g8e-linux-arm64", Checksum: "g8e-linux-arm64.sha256", Mode: constants.PermFileExecutable},
	{OS: "linux", Arch: "386", Filename: "g8e-linux-386", Checksum: "g8e-linux-386.sha256", Mode: constants.PermFileExecutable},
	{OS: "windows", Arch: "amd64", Filename: "g8e-windows-amd64.exe", Checksum: "g8e-windows-amd64.exe.sha256", Mode: constants.PermFileExecutable},
	{OS: "windows", Arch: "arm64", Filename: "g8e-windows-arm64.exe", Checksum: "g8e-windows-arm64.exe.sha256", Mode: constants.PermFileExecutable},
	{OS: "darwin", Arch: "amd64", Filename: "g8e-darwin-amd64", Checksum: "g8e-darwin-amd64.sha256", Mode: constants.PermFileExecutable},
	{OS: "darwin", Arch: "arm64", Filename: "g8e-darwin-arm64", Checksum: "g8e-darwin-arm64.sha256", Mode: constants.PermFileExecutable},
}

// Targets returns a copy of the supported deployment matrix.
func Targets() []Target {
	result := append([]Target(nil), targets...)
	return result
}

func targetByFilename(name string) (Target, bool) {
	for _, target := range targets {
		if target.Filename == name || target.Checksum == name {
			return target, true
		}
	}
	return Target{}, false
}

func validateFilename(name string) error {
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q", constants.ErrG8eBinaryArtifact, name)
	}
	if _, ok := targetByFilename(name); !ok {
		return fmt.Errorf("%w: unsupported filename %q", constants.ErrG8eBinaryArtifact, name)
	}
	return nil
}

// PlatformTarget returns the catalogued executable for the requested platform.
func PlatformTarget(goos, goarch string) (Target, error) {
	for _, target := range targets {
		if target.OS == goos && target.Arch == goarch {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("%w: unsupported platform %s/%s", constants.ErrG8eBinaryArtifact, goos, goarch)
}

// HostTarget returns the catalogued executable for the current Go platform.
func HostTarget() (Target, error) { return PlatformTarget(runtime.GOOS, runtime.GOARCH) }

func sortedTargets(entries []Target) []Target {
	result := append([]Target(nil), entries...)
	sort.Slice(result, func(i, j int) bool { return result[i].Filename < result[j].Filename })
	return result
}
