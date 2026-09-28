// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
)

type providerObserverRunJSON struct {
	ObserverID     string `json:"observer_id"`
	SampleInterval int    `json:"sample_interval"`
	PollInterval   int    `json:"poll_interval"`
	Collector      string `json:"collector"`
	WindowsDir     string `json:"windows_dir"`
}

type providerObserverVerifyJSON struct {
	ProviderAttemptID string   `json:"provider_attempt_id"`
	Complete          bool     `json:"complete"`
	SampleCount       int      `json:"sample_count"`
	GPUReported       bool     `json:"gpu_reported"`
	HostRAMReported   bool     `json:"host_ram_reported"`
	ClockSkewNanos    int64    `json:"clock_skew_nanos"`
	FailureReasons    []string `json:"failure_reasons"`
}

func observerEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observer",
		Short: "Provider-boundary hardware observer",
	}
	cmd.AddCommand(
		jsonLeaf(observerRunCmd(deps)),
		jsonLeaf(observerVerifyCmd(deps)),
	)
	return cmd
}

func observerRunCmd(deps nativeEvalDeps) *cobra.Command {
	var name string
	var sampleInterval string
	var pollInterval string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Watch inference provider attempts and record provider-boundary hardware samples",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			windowStore, err := provider_observer.NewWindowStore(fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer run: %w", err)
			}
			if name == "" {
				name = "g8e-provider-boundary-observer"
			}
			sampleDuration, err := time.ParseDuration(sampleInterval)
			if err != nil {
				return fmt.Errorf("evaluation: invalid sample interval: %w", err)
			}
			pollDuration, err := time.ParseDuration(pollInterval)
			if err != nil {
				return fmt.Errorf("evaluation: invalid poll interval: %w", err)
			}
			runner, err := provider_observer.NewRunner(provider_observer.RunnerConfig{
				ObserverID:     name,
				Collector:      provider_observer.DefaultCollector(),
				WindowStore:    windowStore,
				FileSvc:        fileSvc,
				SampleInterval: sampleDuration,
				PollInterval:   pollDuration,
				Now:            deps.now,
			})
			if err != nil {
				return fmt.Errorf("evaluation: provider observer run: %w", err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(providerObserverRunJSON{
					ObserverID:     name,
					SampleInterval: int(sampleDuration.Milliseconds()),
					PollInterval:   int(pollDuration.Milliseconds()),
					Collector:      "nvidia-smi+proc-meminfo",
					WindowsDir:     constants.InferenceProviderObserverDirname + "/" + constants.InferenceProviderObserverWindowsDirname,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Provider-boundary observer started\nObserver: %s\nSample interval: %s\nPoll interval: %s\n", name, sampleInterval, pollInterval)
				if err != nil {
					return err
				}
			}
			if err := runner.Run(ctx); err != nil && err != context.Canceled {
				return fmt.Errorf("evaluation: provider observer run: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Stable observer identity pseudonym")
	cmd.Flags().StringVar(&sampleInterval, "sample-interval", "250ms", "Hardware sample interval (e.g., 250ms, 1s)")
	cmd.Flags().StringVar(&pollInterval, "poll-interval", "250ms", "Attempt polling interval (e.g., 250ms, 1s)")
	return cmd
}

func observerVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <attempt>",
		Short: "Verify provider-boundary observation coverage for one provider attempt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerAttemptID := args[0]
			_, fileSvc, err := nativeEvalEnvironment(cmd, deps)
			if err != nil {
				return err
			}
			windowStore, err := provider_observer.NewWindowStore(fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer verify: %w", err)
			}
			window, err := windowStore.Load(cmd.Context(), providerAttemptID)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer verify: %w", err)
			}
			attemptStore, err := inference.NewAttemptStore(fileSvc)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer verify: %w", err)
			}
			attempt, err := attemptStore.Get(cmd.Context(), providerAttemptID)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer verify: %w", err)
			}
			report, err := provider_observer.VerifyObservationCoverage(window, attempt)
			if err != nil {
				return fmt.Errorf("evaluation: provider observer verify: %w", err)
			}
			if output.JSONEnabled(cmd) {
				payload, err := json.MarshalIndent(providerObserverVerifyJSON{
					ProviderAttemptID: report.ProviderAttemptID,
					Complete:          report.Complete,
					SampleCount:       report.SampleCount,
					GPUReported:       report.GPUReported,
					HostRAMReported:   report.HostRAMReported,
					ClockSkewNanos:    report.ClockSkewNanos,
					FailureReasons:    report.FailureReasons,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Provider attempt %s observation coverage: %s\nSamples: %d\nGPU reported: %t\nHost RAM reported: %t\n",
					report.ProviderAttemptID,
					map[bool]string{true: "complete", false: "incomplete"}[report.Complete],
					report.SampleCount,
					report.GPUReported,
					report.HostRAMReported,
				)
				if err != nil {
					return err
				}
				for _, reason := range report.FailureReasons {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", reason)
				}
			}
			if !report.Complete {
				return constants.ErrEvalRunVerificationFailed
			}
			return nil
		},
	}
	return cmd
}
