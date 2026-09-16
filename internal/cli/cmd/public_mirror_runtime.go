// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

const (
	defaultPublicMirrorSourceID   = "opendevops-local"
	defaultPublicMirrorPrivateURL = "http://127.0.0.1:8081"
)

type publicMirrorRuntime struct {
	runtime *gateway.PublicSpectatorRuntime
}

func ensureLocalPublicFeed(ctx context.Context, fileSvc fs.RuntimeFileService, sourceID, mirrorOrigin string) (models.PublicExportConfig, error) {
	return gateway.EnsureLocalPublicFeed(ctx, fileSvc, sourceID, mirrorOrigin)
}

func newPublicMirrorRuntime(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig, listenAddress, publicListenAddress string) (*publicMirrorRuntime, error) {
	cfg := gateway.PublicSpectatorConfig{
		Enabled:              true,
		PrivateListenAddress: listenAddress,
		PublicListenAddress:  publicListenAddress,
		SourceID:             exportConfig.SourceID,
	}
	runtime, err := gateway.NewPublicSpectatorRuntime(cfg, fileSvc, nil)
	if err != nil {
		return nil, err
	}
	return &publicMirrorRuntime{runtime: runtime}, nil
}

func (runtime *publicMirrorRuntime) serve(ctx context.Context) error {
	if runtime == nil || runtime.runtime == nil {
		return fmt.Errorf("public mirror: %w", constants.ErrMissingRequiredField)
	}
	return runtime.runtime.Serve(ctx)
}

func startPublicMirrorDaemon(listenAddress, publicListenAddress, sourceID, mirrorOrigin string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("public mirror: resolve executable: %w", err)
	}
	args := []string{
		"eval", "mirror", "run",
		"--listen", listenAddress,
		"--public-listen", publicListenAddress,
	}
	if strings.TrimSpace(sourceID) != "" {
		args = append(args, "--source-id", sourceID)
	}
	if strings.TrimSpace(mirrorOrigin) != "" {
		args = append(args, "--mirror-origin", mirrorOrigin)
	}
	command := exec.Command(executable, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("public mirror: start daemon: %w", err)
	}
	return command.Process.Pid, nil
}
