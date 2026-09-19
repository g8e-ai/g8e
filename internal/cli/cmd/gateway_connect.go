// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.

package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// connectDeps holds the injectable external boundaries for the gw connect
// command. Fields left nil get production defaults via defaultConnectDeps.
// Tests populate the fields they need to control. Internal services
// (ProcessManager, RuntimeFileService, launch-profile read/write, origin
// parsing) are always real — only OS trust, HTTP transport, browser opening,
// and stdin prompting are stubbed.
type connectDeps struct {
	// trustInstaller is the OS trust store interface. The concrete
	// *platform.SystemTrustInstaller satisfies auth.SystemTrustInstaller;
	// tests inject a mock to avoid sudo/exec.
	trustInstaller auth.SystemTrustInstaller

	// discoveryFetcher fetches and validates the live CA bundle from the
	// Gateway's unauthenticated discovery endpoint. Tests inject a stub
	// to avoid real HTTP.
	discoveryFetcher func(ctx context.Context, discoveryURL string, now func() time.Time) (auth.TrustDiscoveryResult, error)

	// verifier performs read-only HTTPS health and CORS preflight checks.
	// Tests inject a Verifier with a stub HTTP client factory.
	verifier *frontendverify.Verifier

	// browserOpener opens a URL in the user's default browser. Tests
	// inject a stub to avoid exec.
	browserOpener func(url string) error

	// confirm prompts the user with a yes/no question. The command layer
	// injects a stdin-reading implementation; tests inject a deterministic
	// stub. Returning false aborts the operation that requested
	// confirmation.
	confirm func(prompt string) bool

	// continueFn prompts the user to press Enter to continue. Used for the
	// browser-restart gate after trust state changes. Tests inject a
	// deterministic stub.
	continueFn func(prompt string) bool

	// now is the clock for time-based certificate validity checks. Defaults
	// to time.Now.
	now func() time.Time

	// logger for diagnostics.
	logger *slog.Logger
}

// defaultConnectDeps returns production defaults for connectDeps. The confirm
// and continueFn defaults are auto-confirm/auto-continue; the interactive
// command layer overrides them with stdin-reading implementations.
func defaultConnectDeps() connectDeps {
	return connectDeps{
		trustInstaller:   platform.NewSystemTrustInstaller(),
		discoveryFetcher: auth.DiscoverLiveTrustBundle,
		verifier:         frontendverify.NewVerifier(frontendverify.VerifierDeps{}),
		browserOpener:    platform.OpenBrowser,
		confirm:          func(string) bool { return true },
		continueFn:       func(string) bool { return true },
		now:              time.Now,
		logger:           slog.Default(),
	}
}

// gatewayConnectCmd is the production constructor for the gw connect command.
func gatewayConnectCmd() *cobra.Command {
	return gatewayConnectCmdWithConfig(loadConfig, newFileSvc, defaultConnectDeps())
}

// gatewayConnectCmdWithConfig is the testable constructor. It accepts
// injectable config loading, file service factory, and external-boundary deps.
func gatewayConnectCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	deps connectDeps,
) *cobra.Command {
	var (
		passkeyRpID   string
		passkeyRpName string
		noSystemTrust bool
		noOpen        bool
		yes           bool
	)

	cmd := &cobra.Command{
		Use:   "connect <frontend-origin>",
		Short: "Connect a browser-hosted frontend to the local Gateway",
		Long: `Connect a browser-hosted frontend (e.g. a Lovable app) to the g8e
Gateway running on the same computer. Supply the frontend origin once; the CLI
validates and normalizes it, derives the safest WebAuthn configuration, starts
or deliberately restarts the Gateway, establishes local certificate trust with
explicit consent, verifies HTTPS and CORS against the running process, and
prints a short frontend prompt.

The successful user journey contains two actions:
  1. Run this command with the frontend origin.
  2. Paste the emitted prompt into your frontend builder, open the app in a new
     browser tab, and approve the browser's local-network permission when
     prompted.

The CLI never silently stops or reconfigures a running Gateway. A restart
requires explicit confirmation (--yes is the non-interactive opt-in).
Certificate trust installation and stale-anchor removal have separate consent
boundaries that --yes does not suppress.

Advanced multi-origin deployments continue to use explicit 'gw start' flags
after confirming that every origin is valid for the selected RP ID.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGatewayConnect(cmd, args[0], passkeyRpID, passkeyRpName, noSystemTrust, noOpen, yes, configLoader, fileSvcFactory, deps)
		},
	}

	cmd.Flags().StringVar(&passkeyRpID, "passkey-rp-id", "", "Advanced override for the exact-host default RP ID; validated against the frontend origin")
	cmd.Flags().StringVar(&passkeyRpName, "passkey-rp-name", "", "Optional passkey RP display name; defaults to g8e")
	cmd.Flags().BoolVar(&noSystemTrust, "no-system-trust", false, "Select manual browser trust instead of OS trust installation")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open the local health URL during manual trust handoff")
	cmd.Flags().BoolVar(&yes, "yes", false, "Approve a required managed Gateway restart non-interactively; does not suppress trust or stale-anchor consent")

	return cmd
}

// runGatewayConnect is the state-machine orchestrator. It is extracted from
// RunE so tests can call it directly with injected deps. Every external
// boundary (OS trust, HTTP transport, browser opening, stdin prompting) goes
// through deps; internal services (ProcessManager, RuntimeFileService,
// launch-profile read/write, origin parsing) are always real.
func runGatewayConnect(
	cmd *cobra.Command,
	originArg string,
	rpIDOverride string,
	rpNameOverride string,
	noSystemTrust bool,
	noOpen bool,
	yes bool,
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	deps connectDeps,
) error {
	// Fill nil deps with production defaults.
	if deps.trustInstaller == nil {
		deps.trustInstaller = platform.NewSystemTrustInstaller()
	}
	if deps.discoveryFetcher == nil {
		deps.discoveryFetcher = auth.DiscoverLiveTrustBundle
	}
	if deps.verifier == nil {
		deps.verifier = frontendverify.NewVerifier(frontendverify.VerifierDeps{})
	}
	if deps.browserOpener == nil {
		deps.browserOpener = platform.OpenBrowser
	}
	if deps.confirm == nil {
		deps.confirm = func(string) bool { return true }
	}
	if deps.continueFn == nil {
		deps.continueFn = func(string) bool { return true }
	}
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.logger == nil {
		deps.logger = slog.Default()
	}

	// 1. Parse and validate the frontend origin.
	origin, err := browserorigin.Parse(originArg)
	if err != nil {
		return fmt.Errorf("gateway connect: parse origin: %w", err)
	}

	// 2. Derive the RP ID (exact-host default or validated override).
	rpID := origin.RPID
	if rpIDOverride != "" {
		rpID, err = browserorigin.ValidateRPID(origin, rpIDOverride)
		if err != nil {
			return fmt.Errorf("gateway connect: validate RP ID override: %w", err)
		}
	}
	rpName := rpNameOverride
	if rpName == "" {
		rpName = "g8e"
	}

	// 3. Load config and create file service.
	if _, err := configLoader(""); err != nil {
		return fmt.Errorf("gateway: load config: %w", err)
	}

	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	pm, err := platform.NewProcessManager(fileSvc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	// 4. Check Gateway process state.
	running, pid, err := pm.OperatorStatus()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
	}

	if !running {
		return connectStoppedGateway(cmd, origin, rpID, rpName, fileSvc, pm, deps, noSystemTrust, noOpen)
	}

	// Gateway is running. Determine whether the persisted launch profile
	// matches the requested browser configuration.
	profile, err := serve.ReadLaunchProfile(fileSvc)
	if err != nil {
		if errors.Is(err, constants.ErrLaunchProfileMissing) {
			cmd.Printf("g8e Gateway is running (PID: %d) but no complete launch profile is available.\n", pid)
			cmd.Println("The CLI cannot safely reconstruct non-default settings from a running process.")
			cmd.Println()
			cmd.Printf("To apply the new frontend origin, stop the Gateway and rerun:\n")
			cmd.Printf("  %s gw stop && %s gw connect %s\n", getBinaryName(), getBinaryName(), origin.URL)
			return fmt.Errorf("%w: no launch profile for running gateway (PID %d)", constants.ErrLaunchProfileMissing, pid)
		}
		return fmt.Errorf("gateway connect: read launch profile: %w", err)
	}

	apiURL := connectAPIBaseURL(profile.Config.HTTPSPort)
	discoveryURL := connectDiscoveryURL(profile.Config.HTTPPort)
	matches, deltas := browserConfigMatches(profile.Config, origin, rpID, rpName)
	if matches {
		renderConnectReview(cmd, origin, rpID, rpName, apiURL, gatewayStateMatching)
		cmd.Println("Gateway is already running with the requested browser configuration.")
		cmd.Println()
		return connectTrustAndVerify(cmd, origin, apiURL, discoveryURL, deps, noSystemTrust, noOpen)
	}

	// Configuration differs. Print the delta and ask for restart consent.
	renderConnectReview(cmd, origin, rpID, rpName, apiURL, gatewayStateRunning)
	renderConfigDelta(cmd, deltas)

	if !yes {
		cmd.Print("Restart the Gateway with the proposed configuration? [y/N]: ")
		if !deps.confirm("") {
			cmd.Println("Restart declined. The current Gateway and launch profile are unchanged.")
			return fmt.Errorf("%w: origin %s", constants.ErrManagedRestartDeclined, origin.URL)
		}
	}

	return connectRestartGateway(cmd, origin, rpID, rpName, profile, fileSvc, pm, deps, noSystemTrust, noOpen)
}

// connectStoppedGateway handles the state where no Gateway process is running.
// It loads the existing launch profile (or defaults), applies the browser
// fields, prints the review, starts the managed Gateway, persists the profile,
// and continues to trust and verification.
func connectStoppedGateway(
	cmd *cobra.Command,
	origin browserorigin.Origin,
	rpID, rpName string,
	fileSvc fs.RuntimeFileService,
	pm *platform.ProcessManager,
	deps connectDeps,
	noSystemTrust, noOpen bool,
) error {
	// Load the existing launch profile when present; otherwise start from
	// resolved defaults.
	var baseCfg serve.GatewayConfig
	profile, err := serve.ReadLaunchProfile(fileSvc)
	if err != nil {
		if errors.Is(err, constants.ErrLaunchProfileMissing) {
			// No prior profile — use default posture and log level.
			baseCfg = defaultServeConfig()
		} else {
			return fmt.Errorf("gateway connect: read launch profile: %w", err)
		}
	} else {
		baseCfg = profile.Config
	}

	// Apply the derived browser fields to a copy of the base config.
	connectCfg := deriveConnectConfig(origin, rpID, rpName, baseCfg)
	apiURL := connectAPIBaseURL(connectCfg.HTTPSPort)
	renderConnectReview(cmd, origin, rpID, rpName, apiURL, gatewayStateStopped)

	// Detect network identity and resolve cert mode.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	identityResult := detectIdentity(cmd.Context(), logger, connectCfg.CertIdentityMode)
	if identityResult.ShouldFallback {
		cmd.Println("Warning: Failed to detect network identity, falling back to localhost-only mode")
	} else if identityResult.Identity != nil {
		cmd.Println(identityResult.Identity.FormatForDisplay())
		cmd.Println()
	}
	connectCfg.CertIdentityMode = identityResult.CertMode

	cmd.Println("[g8e] Starting g8e Gateway service...")
	startOpts := platform.OperatorStartOptions{GatewayConfig: connectCfg}
	if err := pm.StartOperator(&startOpts); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}
	connectCfg = startOpts.GatewayConfig

	// Persist the complete validated launch profile after successful start.
	if err := serve.WriteLaunchProfile(fileSvc, connectCfg); err != nil {
		if stopErr := pm.StopOperator(); stopErr != nil {
			return fmt.Errorf("%w: persist launch profile: %w; stop untracked gateway: %w", constants.ErrInternal, err, stopErr)
		}
		return fmt.Errorf("%w: persist launch profile: %w", constants.ErrInternal, err)
	}

	_, pid, err := pm.OperatorStatus()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
	}
	cmd.Printf("[g8e] Gateway started (PID: %d)\n", pid)
	cmd.Println()

	apiURL = connectAPIBaseURL(connectCfg.HTTPSPort)
	return connectTrustAndVerify(cmd, origin, apiURL, connectDiscoveryURL(connectCfg.HTTPPort), deps, noSystemTrust, noOpen)
}

// connectRestartGateway handles the state where a running Gateway's
// configuration differs from the requested browser configuration. It stops the
// current process, starts with the updated profile, persists, and continues to
// trust and verification. Rollback is attempted if the new process cannot
// start; both failures are reported.
func connectRestartGateway(
	cmd *cobra.Command,
	origin browserorigin.Origin,
	rpID, rpName string,
	profile serve.GatewayLaunchProfile,
	fileSvc fs.RuntimeFileService,
	pm *platform.ProcessManager,
	deps connectDeps,
	noSystemTrust, noOpen bool,
) error {
	// Build the updated config from the existing profile.
	connectCfg := deriveConnectConfig(origin, rpID, rpName, profile.Config)

	// Re-run network identity detection with the profile's cert mode.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	identityResult := detectIdentity(cmd.Context(), logger, connectCfg.CertIdentityMode)
	if identityResult.ShouldFallback {
		cmd.Println("Warning: Failed to detect network identity, falling back to localhost-only mode")
	} else if identityResult.Identity != nil {
		cmd.Println(identityResult.Identity.FormatForDisplay())
		cmd.Println()
	}
	connectCfg.CertIdentityMode = identityResult.CertMode

	cmd.Println("Stopping g8e Gateway...")
	if err := pm.StopOperator(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	cmd.Println("Starting g8e Gateway with updated configuration...")
	startOpts := platform.OperatorStartOptions{GatewayConfig: connectCfg}
	if err := pm.StartOperator(&startOpts); err != nil {
		// Rollback: attempt to restore the previously validated profile.
		cmd.Printf("Failed to start Gateway with new configuration: %v\n", err)
		cmd.Println("Attempting rollback to the previous configuration...")
		rollbackOpts := platform.OperatorStartOptions{GatewayConfig: profile.Config}
		if rbErr := pm.StartOperator(&rollbackOpts); rbErr != nil {
			return fmt.Errorf("%w: start failed: %w; rollback also failed: %w", constants.ErrProcessStartFailed, err, rbErr)
		}
		cmd.Println("Rollback successful. The previous configuration is active.")
		return fmt.Errorf("%w: new configuration could not start; rolled back: %w", constants.ErrProcessStartFailed, err)
	}
	connectCfg = startOpts.GatewayConfig

	// Persist the updated profile only after successful start.
	if err := serve.WriteLaunchProfile(fileSvc, connectCfg); err != nil {
		if stopErr := pm.StopOperator(); stopErr != nil {
			return fmt.Errorf("%w: persist launch profile: %w; stop untracked gateway: %w", constants.ErrInternal, err, stopErr)
		}
		rollbackOpts := platform.OperatorStartOptions{GatewayConfig: profile.Config}
		if rollbackErr := pm.StartOperator(&rollbackOpts); rollbackErr != nil {
			return fmt.Errorf("%w: persist launch profile: %w; rollback: %w", constants.ErrInternal, err, rollbackErr)
		}
		return fmt.Errorf("%w: persist launch profile: %w", constants.ErrInternal, err)
	}

	_, pid, err := pm.OperatorStatus()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
	}
	cmd.Printf("[g8e] Gateway restarted (PID: %d)\n", pid)
	cmd.Println()

	apiURL := connectAPIBaseURL(connectCfg.HTTPSPort)
	return connectTrustAndVerify(cmd, origin, apiURL, connectDiscoveryURL(connectCfg.HTTPPort), deps, noSystemTrust, noOpen)
}

// connectTrustAndVerify runs the trust discovery, trust installation, HTTPS
// health, and CORS preflight phases. It is called after the Gateway is running
// (either freshly started, restarted, or already matching). It prints progress
// and prompts through the injected deps; it never writes to the OS trust store
// without explicit consent.
func connectTrustAndVerify(
	cmd *cobra.Command,
	origin browserorigin.Origin,
	apiURL, discoveryURL string,
	deps connectDeps,
	noSystemTrust, noOpen bool,
) error {
	ctx := cmd.Context()

	// 1. Discover the live CA bundle from the Gateway's discovery endpoint.
	discovery, err := deps.discoveryFetcher(ctx, discoveryURL, deps.now)
	if err != nil {
		return fmt.Errorf("gateway connect: discover trust bundle: %w", err)
	}

	cmd.Printf("Gateway root CA fingerprint (SHA-256): %s\n", discovery.Fingerprint)
	cmd.Println()

	// 2. Inspect system trust state.
	trustState, err := auth.InspectTrustState(ctx, deps.trustInstaller, discovery.Fingerprint)
	if err != nil {
		return fmt.Errorf("gateway connect: inspect trust state: %w", err)
	}

	trustChanged := false

	// 3. Stale anchor detection and removal (separate consent).
	if len(trustState.StaleAnchors) > 0 {
		cmd.Printf("Found %d stale g8e root CA anchor(s) from previous Gateway instances.\n", len(trustState.StaleAnchors))
		cmd.Print("Remove stale anchors? [y/N]: ")
		if !deps.confirm("") {
			cmd.Println("Stale anchor removal declined. Proceeding with current trust state.")
		} else {
			cmd.Println("Removing stale anchors...")
			if err := deps.trustInstaller.RemoveStaleAnchors(ctx, trustState.StaleAnchors); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
			}
			cmd.Println("Stale anchors removed.")
			trustChanged = true
		}
	}

	// 4. Trust installation (separate consent, suppressed by --no-system-trust).
	if noSystemTrust {
		cmd.Println("System trust installation skipped (--no-system-trust).")
		renderManualTrustInstructions(cmd, apiURL, discovery.Fingerprint)
	} else if !trustState.Trusted {
		cmd.Print("Install the g8e root CA into the OS trust store? [y/N]: ")
		if !deps.confirm("") {
			renderManualTrustInstructions(cmd, apiURL, discovery.Fingerprint)
			return fmt.Errorf("%w: user declined OS trust installation for origin %s", constants.ErrManualBrowserTrustRequired, origin.URL)
		}
		cmd.Println("Installing g8e root CA...")
		if err := deps.trustInstaller.InstallRoot(ctx, discovery.PrimaryRoot, discovery.Fingerprint); err != nil {
			if errors.Is(err, constants.ErrSystemTrustUnsupported) {
				cmd.Printf("Warning: OS trust store installation is unsupported on this platform (%v).\n", err)
				renderManualTrustInstructions(cmd, apiURL, discovery.Fingerprint)
				return fmt.Errorf("%w: %w", constants.ErrManualBrowserTrustRequired, err)
			} else {
				return fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
			}
		} else {
			cmd.Println("g8e root CA installed.")
			trustChanged = true
		}
	} else {
		cmd.Println("Gateway root CA is already trusted.")
	}

	// 5. Browser-restart gate when trust state changed.
	if trustChanged {
		cmd.Println()
		cmd.Println("Trust state changed. Close all browser windows before continuing.")
		cmd.Print("Press Enter when all browser windows are closed: ")
		if !deps.continueFn("") {
			cmd.Println("Browser restart gate declined. Browser handoff stopped.")
			return constants.ErrBrowserRestartDeclined
		}
	}
	cmd.Println()

	// 6. Build root pool from the discovered bundle.
	rootPool := x509.NewCertPool()
	for _, anchor := range discovery.RootAnchors {
		rootPool.AddCert(anchor)
	}

	// 7. Run HTTPS health and CORS verification.
	healthURL := apiURL + constants.APIPaths.Health
	report, err := deps.verifier.Verify(ctx, frontendverify.VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         healthURL,
		RootPool:       rootPool,
	})
	if err != nil {
		return fmt.Errorf("gateway connect: verify: %w", err)
	}

	renderVerificationReport(cmd, report)

	if !report.AllPassed {
		if report.FirstFailure != nil {
			switch report.FirstFailure.Name {
			case frontendverify.CheckHTTPSHealth, frontendverify.CheckCertificateChain:
				return fmt.Errorf("%w: %s", constants.ErrHTTPSCertificateVerification, report.FirstFailure.Detail)
			default:
				return fmt.Errorf("%w: %s", constants.ErrCORSPreflightRejected, report.FirstFailure.Detail)
			}
		}
		return fmt.Errorf("%w: verification failed", constants.ErrCORSPreflightRejected)
	}

	// 8. Browser handoff.
	if noSystemTrust && !noOpen {
		if err := deps.browserOpener(apiURL); err != nil {
			cmd.Printf("Warning: could not open browser (%v). The Gateway HTTPS URL is: %s\n", err, apiURL)
		}
	}

	// 9. Print the frontend prompt and next action.
	renderFrontendPrompt(cmd, apiURL)

	return nil
}

// defaultServeConfig returns a minimal serve.GatewayConfig with default
// posture and log level. Used when no launch profile exists and the Gateway
// is stopped.
func defaultServeConfig() serve.GatewayConfig {
	return gatewayFlagsToServeConfig(resolveGatewayFlags(GatewayFlags{
		Posture:  "doctrine",
		LogLevel: "info",
	}))
}

// deriveConnectConfig applies the derived browser fields (CORS origin, passkey
// RP ID, RP name, RP origins) to a copy of the base config. All other fields
// (ports, posture, consensus, downstream, rate limits, doctrine, vault, cert
// mode) are preserved from the base. This is a pure function — it reads the
// base and returns a new config without mutating the input.
func deriveConnectConfig(origin browserorigin.Origin, rpID, rpName string, base serve.GatewayConfig) serve.GatewayConfig {
	cfg := base // copy
	cfg.AllowedOrigins = []string{origin.URL}
	cfg.PasskeyRpOrigins = []string{origin.URL}
	cfg.PasskeyRpID = rpID
	cfg.PasskeyRpName = rpName
	return cfg
}

// browserConfigMatches compares the persisted launch profile's browser-relevant
// fields against the requested frontend origin and RP ID. It returns true when
// every field matches, and a slice of configDelta describing the fields that
// differ. It is a pure read — it never mutates the profile.
func browserConfigMatches(profile serve.GatewayConfig, origin browserorigin.Origin, rpID, rpName string) (bool, []configDelta) {
	var deltas []configDelta
	matched := true

	if !containsString(profile.AllowedOrigins, origin.URL) {
		matched = false
		deltas = append(deltas, configDelta{
			Field:    "CORS origin (--cors-origin)",
			Current:  joinOrigins(profile.AllowedOrigins),
			Proposed: origin.URL,
		})
	}

	if profile.PasskeyRpID != rpID {
		matched = false
		deltas = append(deltas, configDelta{
			Field:    "Passkey RP ID (--passkey-rp-id)",
			Current:  profile.PasskeyRpID,
			Proposed: rpID,
		})
	}

	if profile.PasskeyRpName != rpName {
		matched = false
		deltas = append(deltas, configDelta{
			Field:    "Passkey RP name (--passkey-rp-name)",
			Current:  profile.PasskeyRpName,
			Proposed: rpName,
		})
	}

	if !containsString(profile.PasskeyRpOrigins, origin.URL) {
		matched = false
		deltas = append(deltas, configDelta{
			Field:    "Passkey RP origin (--passkey-rp-origin)",
			Current:  joinOrigins(profile.PasskeyRpOrigins),
			Proposed: origin.URL,
		})
	}

	return matched, deltas
}
