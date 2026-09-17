// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
)

func providerObserverEvalCmd(deps nativeEvalDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider-observer",
		Short: "Run the read-only provider-boundary hardware observer",
	}
	cmd.AddCommand(providerObserverRunCmd(deps), providerObserverVerifyCmd(deps))
	return cmd
}

func providerObserverRunCmd(deps nativeEvalDeps) *cobra.Command {
	var observerID string
	var sampleIntervalMS int
	var pollIntervalMS int
	var jsonOutput bool
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
			if observerID == "" {
				observerID = "g8e-provider-boundary-observer"
			}
			runner, err := provider_observer.NewRunner(provider_observer.RunnerConfig{
				ObserverID:     observerID,
				Collector:      provider_observer.DefaultCollector(),
				WindowStore:    windowStore,
				FileSvc:        fileSvc,
				SampleInterval: time.Duration(sampleIntervalMS) * time.Millisecond,
				PollInterval:   time.Duration(pollIntervalMS) * time.Millisecond,
				Now:            deps.now,
			})
			if err != nil {
				return fmt.Errorf("evaluation: provider observer run: %w", err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"observer_id":     observerID,
					"sample_interval": sampleIntervalMS,
					"poll_interval":   pollIntervalMS,
					"collector":       "nvidia-smi+proc-meminfo",
					"windows_dir":     constants.InferenceProviderObserverDirname + "/" + constants.InferenceProviderObserverWindowsDirname,
				}, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
				if err != nil {
					return err
				}
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Provider-boundary observer started\nObserver: %s\nSample interval: %dms\nPoll interval: %dms\n", observerID, sampleIntervalMS, pollIntervalMS)
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
	cmd.Flags().StringVar(&observerID, "observer-id", "", "Stable observer identity pseudonym")
	cmd.Flags().IntVar(&sampleIntervalMS, "sample-interval-ms", 250, "Hardware sample interval in milliseconds")
	cmd.Flags().IntVar(&pollIntervalMS, "poll-interval-ms", 250, "Attempt polling interval in milliseconds")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit startup JSON and run")
	return cmd
}

func providerObserverVerifyCmd(deps nativeEvalDeps) *cobra.Command {
	var providerAttemptID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify provider-boundary observation coverage for one provider attempt",
		RunE: func(cmd *cobra.Command, args []string) error {
			if providerAttemptID == "" {
				return fmt.Errorf("evaluation: provider observer verify: %w", constants.ErrMissingRequiredField)
			}
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
			if jsonOutput {
				payload, err := json.MarshalIndent(map[string]any{
					"provider_attempt_id": report.ProviderAttemptID,
					"complete":            report.Complete,
					"sample_count":        report.SampleCount,
					"gpu_reported":        report.GPUReported,
					"host_ram_reported":   report.HostRAMReported,
					"clock_skew_nanos":    report.ClockSkewNanos,
					"failure_reasons":     report.FailureReasons,
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
	cmd.Flags().StringVar(&providerAttemptID, "provider-attempt-id", "", "Governed inference provider attempt ID")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON status")
	return cmd
}
