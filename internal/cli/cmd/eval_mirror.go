// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func evalMirrorCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Run and manage the local public evaluation mirror",
	}
	cmd.AddCommand(
		evalMirrorRunCmd(deps),
		evalMirrorStopCmd(deps),
		evalMirrorStatusCmd(deps),
	)
	return cmd
}

func evalMirrorRunCmd(deps nativeEvalDeps) *cobra.Command {
	var listenAddress string
	var publicListenAddress string
	var sourceID string
	var mirrorOrigin string
	var daemon bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the durable local public mirror for evaluation explorer reads",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			ctx := commandContext(cmd)
			if err := fileSvc.CreateRuntimeTree(ctx); err != nil {
				return err
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return err
			}
			runningPID, err := pm.ReadPIDFile(constants.PublicMirrorPIDFilename)
			if err != nil {
				return err
			}
			if runningPID > 0 && pm.IsProcessRunning(runningPID) {
				return fmt.Errorf("public mirror: already running with pid %d; run `./g8e eval mirror stop` first", runningPID)
			}
			if runningPID > 0 {
				_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
			}
			exportConfig, err := ensureLocalPublicFeed(ctx, fileSvc, sourceID, mirrorOrigin)
			if err != nil {
				return err
			}
			if daemon {
				pid, err := startPublicMirrorDaemon(listenAddress, publicListenAddress, exportConfig.SourceID, exportConfig.MirrorOrigin)
				if err != nil {
					return err
				}
				if err := pm.WritePIDFile(constants.PublicMirrorPIDFilename, pid); err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public mirror started on %s (private) and %s (public) with pid %d\n", listenAddress, publicListenAddress, pid)
				return err
			}
			if err := pm.WritePIDFile(constants.PublicMirrorPIDFilename, os.Getpid()); err != nil {
				return err
			}
			defer func() { _ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename) }()
			runtime, err := newPublicMirrorRuntime(ctx, fileSvc, exportConfig, listenAddress, publicListenAddress)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public mirror listening on %s (private ingest) and %s (public read/SSE)\n", listenAddress, publicListenAddress)
			if err != nil {
				return err
			}
			return runtime.serve(ctx)
		},
	}
	cmd.Flags().StringVar(&listenAddress, "listen", "127.0.0.1:8081", "Private authenticated ingest listen address")
	cmd.Flags().StringVar(&publicListenAddress, "public-listen", "127.0.0.1:8082", "Public anonymous read-only listen address")
	cmd.Flags().StringVar(&sourceID, "source-id", defaultPublicMirrorSourceID, "Public source deployment pseudonym used when initializing a new local feed")
	cmd.Flags().StringVar(&mirrorOrigin, "mirror-origin", defaultPublicMirrorPrivateURL, "Private mirror origin recorded in export config when initializing a new local feed")
	cmd.Flags().BoolVar(&daemon, "daemon", false, "Start the mirror in the background and write a pid file")
	return cmd
}

func evalMirrorStopCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background public mirror started with --daemon",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return err
			}
			pid, err := pm.ReadPIDFile(constants.PublicMirrorPIDFilename)
			if err != nil {
				return err
			}
			if pid == 0 || !pm.IsProcessRunning(pid) {
				_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Public mirror is not running.")
				return err
			}
			process, err := os.FindProcess(pid)
			if err != nil {
				_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
				return fmt.Errorf("public mirror: find pid %d: %w", pid, err)
			}
			if err := process.Signal(syscall.SIGTERM); err != nil {
				_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
				return fmt.Errorf("public mirror: stop pid %d: %w", pid, err)
			}
			deadline := time.Now().Add(platform.ShutdownTimeout)
			for time.Now().Before(deadline) {
				if !pm.IsProcessRunning(pid) {
					_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stopped public mirror (pid %d)\n", pid)
					return err
				}
				time.Sleep(100 * time.Millisecond)
			}
			if err := process.Kill(); err != nil {
				return fmt.Errorf("public mirror: kill pid %d: %w", pid, err)
			}
			_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stopped public mirror (pid %d)\n", pid)
			return err
		},
	}
	return cmd
}

func evalMirrorStatusCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether the local public mirror is running",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			pm, err := platform.NewProcessManager(fileSvc)
			if err != nil {
				return err
			}
			pid, err := pm.ReadPIDFile(constants.PublicMirrorPIDFilename)
			if err != nil {
				return err
			}
			if pid == 0 || !pm.IsProcessRunning(pid) {
				_ = pm.DeletePIDFile(constants.PublicMirrorPIDFilename)
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Public mirror is not running.")
				return err
			}
			healthy := false
			response, err := http.Get("http://127.0.0.1:8082/bootstrap")
			if err == nil {
				response.Body.Close()
				healthy = response.StatusCode == http.StatusOK
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Public mirror running (pid %d, public bootstrap %s)\n", pid, mirrorHealthLabel(healthy))
			return err
		},
	}
	return cmd
}

func mirrorHealthLabel(healthy bool) string {
	if healthy {
		return "ok"
	}
	return "unreachable"
}
