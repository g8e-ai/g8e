// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ensemble

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/identityreset"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/spf13/cobra"
)

const hostRuntime = ".local.dev/full/ensemble"

func hostResetIdentityCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "reset-identity", Short: "Stop local g8ee and remove its enrollment identity", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			proceed, err := shared.ConfirmDestructive(cmd, shared.DestructiveOptions{Effects: []string{
				"Stop local g8ee in " + hostRuntime, "Remove g8ee certificates, private keys, cached trust, and pending enrollment", "Preserve data, configuration, and logs; fresh enrollment requires gateway approval",
			}, AssumeYes: yes})
			if err != nil || !proceed {
				return err
			}
			stop := hostLifecycleCmd("stop", "")
			stop.SetOut(cmd.OutOrStdout())
			stop.SetErr(cmd.ErrOrStderr())
			stop.SetContext(cmd.Context())
			if err := stop.RunE(stop, nil); err != nil {
				return err
			}
			if err := identityreset.Reset(hostRuntime, "ensemble"); err != nil {
				return err
			}
			cmd.Println("g8ee identity reset. Start g8ee to request fresh enrollment.")
			return nil
		}}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm identity reset")
	return cmd
}

func hostReadCmd(name, short string) *cobra.Command {
	var host bool
	cmd := &cobra.Command{
		Use: name, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "logs" {
				data, err := os.ReadFile(filepath.Join(hostRuntime, "full.log"))
				if err != nil {
					return fmt.Errorf("read g8ee host log: %w", err)
				}
				lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
				if len(lines) > 100 {
					lines = lines[len(lines)-100:]
				}
				cmd.Println(strings.Join(lines, "\n"))
				return nil
			}
			return printHostStatus(cmd, hostRuntime, "http://127.0.0.1:8000/health")
		},
	}
	cmd.Flags().BoolVar(&host, "host", true, "Inspect the local Python service (the default)")
	return cmd
}

func printHostStatus(cmd *cobra.Command, runtimeDir, healthURL string) error {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "full.pid"))
	if os.IsNotExist(err) {
		cmd.Println("  g8ee       stopped (host)")
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return fmt.Errorf("invalid g8ee host PID in %s/full.pid", runtimeDir)
	}
	pm := &platform.ProcessManager{}
	if !pm.IsProcessRunning(pid) {
		cmd.Println("  g8ee       stopped (host)")
		return nil
	}
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		var health struct {
			Status string `json:"status"`
		}
		if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&health) == nil && health.Status == "ok" {
			cmd.Println("  g8ee       ready · http://127.0.0.1:8000")
			return nil
		}
	}
	cmd.Println("  g8ee       running; not ready (check enrollment and logs)")
	return nil
}

func hostLifecycleCmd(name, short string) *cobra.Command {
	return &cobra.Command{
		Use: name, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			python, err := localPython()
			if err != nil {
				return err
			}
			process := exec.CommandContext(cmd.Context(), python, "scripts/full.py", "--ensemble-action", name)
			process.Stdout = cmd.OutOrStdout()
			process.Stderr = cmd.ErrOrStderr()
			return process.Run()
		},
	}
}

func localPython() (string, error) {
	if venv := os.Getenv(string(constants.EnvVar.VirtualEnv)); venv != "" {
		return exec.LookPath(filepath.Join(venv, "bin", "python"))
	}
	if _, err := os.Stat(".venv/bin/python"); err == nil {
		return filepath.Abs(".venv/bin/python")
	}
	return exec.LookPath("python3")
}
