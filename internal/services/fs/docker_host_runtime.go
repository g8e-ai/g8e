// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package fs

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// EnsureDockerHostRuntimeLayout prepares the host-side .g8e tree before Docker
// Compose starts the gateway. The gateway bind-mounts public-feed and
// public-mirror from the repository; when those paths are missing Docker
// creates them as root, which blocks host CLI enrollment from writing
// credentials into .g8e/.
func EnsureDockerHostRuntimeLayout(ctx context.Context, fileSvc RuntimeFileService) error {
	if err := verifyRuntimeDirWritable(fileSvc); err != nil {
		return err
	}
	for _, dir := range []string{constants.PublicFeedDirname, constants.PublicMirrorDirname} {
		if err := fileSvc.MkdirAll(ctx, dir, constants.PermDirPrivate); err != nil {
			return fmt.Errorf("prepare docker host runtime: create %s: %w", dir, err)
		}
	}
	if err := fileSvc.CreateRuntimeTree(ctx); err != nil {
		return fmt.Errorf("prepare docker host runtime: create runtime tree: %w", err)
	}
	return nil
}

func verifyRuntimeDirWritable(fileSvc RuntimeFileService) error {
	runtimeDir := fileSvc.Resolve("")
	info, err := os.Stat(runtimeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%w: stat %s: %w", constants.ErrRuntimeDirNotWritable, runtimeDir, err)
	}
	if info.IsDir() && info.Mode()&0o200 != 0 {
		return probeRuntimeDirWritable(runtimeDir)
	}
	return runtimeDirNotWritableError(runtimeDir)
}

func probeRuntimeDirWritable(runtimeDir string) error {
	probe, err := os.CreateTemp(runtimeDir, ".g8e-write-probe-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return runtimeDirNotWritableError(runtimeDir)
		}
		return fmt.Errorf("%w: probe write in %s: %w", constants.ErrRuntimeDirNotWritable, runtimeDir, err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

func runtimeDirNotWritableError(runtimeDir string) error {
	hint := runtimeDirRepairHint(runtimeDir)
	if hint == "" {
		return fmt.Errorf("%w: %s", constants.ErrRuntimeDirNotWritable, runtimeDir)
	}
	return fmt.Errorf("%w: %s (%s)", constants.ErrRuntimeDirNotWritable, runtimeDir, hint)
}

func runtimeWriteFailureHint(dir string) string {
	if hint := runtimeDirRepairHint(dir); hint != "" {
		return hint
	}
	return "check .g8e ownership and permissions"
}
