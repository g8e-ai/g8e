// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package docker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func toDockerPath(path string) string {
	if runtime.GOOS == "windows" {
		return filepath.ToSlash(path)
	}
	return path
}

func checkDockerAvailable() error {
	if _, err := exec.LookPath(constants.DockerExecutable); err != nil {
		return fmt.Errorf("%w: Docker is not installed or not on PATH", constants.ErrServiceUnavailable)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, constants.DockerExecutable, "info").Run(); err != nil {
		return fmt.Errorf("%w: Docker daemon is not running", constants.ErrServiceUnavailable)
	}
	return nil
}

func forceRemoveLeftovers(cmd *cobra.Command, projectPrefix string) {
	for _, kind := range []string{"volume", "network"} {
		list := exec.Command(constants.DockerExecutable, kind, "ls", "-q", "--filter", "name="+projectPrefix+"_")
		output, err := list.Output()
		if err != nil {
			continue
		}
		for _, name := range strings.Fields(string(output)) {
			remove := exec.Command(constants.DockerExecutable, kind, "rm", "-f", name)
			remove.Stdout = os.Stdout
			remove.Stderr = os.Stderr
			if err := remove.Run(); err != nil {
				cmd.Printf("Warning: could not force-remove %s '%s': %v\n", kind, name, err)
			}
		}
	}
}

func confirmAction(cmd *cobra.Command, prompt string) bool {
	reader := bufio.NewReader(cmd.InOrStdin())
	cmd.Printf("%s [y/N]: ", prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(input))
	return answer == "y" || answer == "yes"
}
