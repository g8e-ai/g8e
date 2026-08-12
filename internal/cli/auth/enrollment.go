// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// recoveryPollInterval is the delay between recovery-status polls while
// waiting for the user to approve a CLI recovery request in the console.
const recoveryPollInterval = 2 * time.Second

// OutputFunc writes a progress line to the command layer's output sink.
// The command layer owns where output goes (stdout, a test buffer, etc.).
// The coordinator never writes to stdout/stderr directly.
type OutputFunc func(format string, args ...any)

// EnrollmentGateway is the subset of the enrollment transport used by the
// coordinator. The concrete *EnrollmentClient satisfies this interface;
// tests inject a mock to avoid network I/O.
type EnrollmentGateway interface {
	Bootstrap(ctx context.Context, cliCSR string, cliKey *ecdsa.PrivateKey, operatorCSR, caFingerprint, baseURL string) (EnrollmentArtifacts, error)
	CreateRecoveryRequest(ctx context.Context, cliCSR, baseURL string) (requestID, token, approvalURL string, expiresAt time.Time, err error)
	RecoveryStatus(ctx context.Context, token, baseURL string) (models.CLIRecoveryState, error)
	CompleteRecovery(ctx context.Context, requestID, token string, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint, baseURL string) (EnrollmentArtifacts, error)
	Rotate(ctx context.Context, fileSvc fs.RuntimeFileService, cliCSR string, cliKey *ecdsa.PrivateKey, caFingerprint string) (EnrollmentArtifacts, error)
	CheckBootstrapStatus(ctx context.Context, baseURL string) (bool, error)
}

// KeyProvider generates CLI key pairs and CSRs. Section 7 will replace the
// default file-backed implementation with a build-tag-resolved provider that
// supports both file-backed and platform-backed (Windows CNG/TPM) keys. The
// coordinator never branches on runtime.GOOS — that decision lives in the
// provider.
type KeyProvider interface {
	// GenerateCLIKeyAndCSR creates a new CLI private key and CSR for the
	// given common name. The returned key is staged into EnrollmentArtifacts
	// so CredentialStore can commit it.
	GenerateCLIKeyAndCSR(ctx context.Context, commonName string) (csrPEM string, key *ecdsa.PrivateKey, err error)
}

// SystemTrustInstaller installs the gateway root CA anchor into the host OS
// trust store. The concrete *platform.SystemTrustInstaller satisfies this
// interface; tests inject a mock to avoid sudo/exec.
type SystemTrustInstaller interface {
	EnsureSystemTrust(ctx context.Context, bundlePEM []byte) (platform.SystemTrustResult, error)
}

// BrowserOpener opens a URL in the user's default browser. Used by the
// coordinator to open the recovery approval URL. The passkey registrar
// manages its own browser launch (Section 8 will harden that path).
type BrowserOpener interface {
	Open(url string) error
}

// PasskeyRegistrar runs the browser-based passkey registration ceremony.
// Section 8 will replace the default implementation with a hardened
// PasskeyRegistrar that prepares the SSE listener before browser launch,
// fixes the cursor strategy, and surfaces browser-open errors.
type PasskeyRegistrar interface {
	// Register opens the gateway console for WebAuthn passkey registration
	// and waits for the passkey.registered SSE event. The userID and
	// cliSessionID identify the newly enrolled (or reused) CLI identity.
	Register(ctx context.Context, userID, cliSessionID string) error
}

// EnrollmentCoordinator owns the §3 enrollment state machine. It is the
// single place that decides whether to bootstrap, recover, rotate, or reuse
// the local CLI identity. Callers (auth.go, demos.go, mcp.go) construct it
// with injected dependencies and call Enroll — they do not duplicate the
// state machine, inspect individual credential files, or branch on
// runtime.GOOS.
//
// The coordinator never writes to stdout/stderr directly; all progress goes
// through the OutputFunc. It never opens a browser except via BrowserOpener
// (recovery approval) or PasskeyRegistrar (passkey ceremony). It never
// invokes sudo or mutates an OS certificate store except via
// SystemTrustInstaller.
type EnrollmentCoordinator struct {
	gateway EnrollmentGateway
	store   *CredentialStore
	keys    KeyProvider
	trust   SystemTrustInstaller
	browser BrowserOpener
	passkey PasskeyRegistrar
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
	clock   func() time.Time
	logger  *slog.Logger
	out     OutputFunc
}

// EnrollmentCoordinatorDeps holds the injectable dependencies for an
// EnrollmentCoordinator. Fields left nil get production defaults. Tests
// populate the fields they need to control.
type EnrollmentCoordinatorDeps struct {
	Gateway EnrollmentGateway
	Store   *CredentialStore
	Keys    KeyProvider
	Trust   SystemTrustInstaller
	Browser BrowserOpener
	Passkey PasskeyRegistrar
	FileSvc fs.RuntimeFileService
	Cfg     *config.Config
	Clock   func() time.Time
	Logger  *slog.Logger
	Out     OutputFunc
}

// NewEnrollmentCoordinator constructs a coordinator from the given deps.
// Nil fields get production defaults:
//   - Gateway: a new *EnrollmentClient (plain HTTP, no mTLS).
//   - Store: a new *CredentialStore over FileSvc/Cfg.
//   - Keys: a FileKeyProvider (file-backed EC P-256 on all platforms).
//   - Trust: a new *platform.SystemTrustInstaller (real os/exec).
//   - Browser: a defaultBrowserOpener wrapping platform.OpenBrowser.
//   - Passkey: a defaultPasskeyRegistrar wrapping RegisterPasskeyViaBrowser.
//   - Clock: time.Now.
//   - Logger: slog.Default().
//   - Out: a no-op writer (the command layer should always supply this).
func NewEnrollmentCoordinator(deps EnrollmentCoordinatorDeps) *EnrollmentCoordinator {
	if deps.FileSvc == nil {
		panic("EnrollmentCoordinator: FileSvc is required")
	}
	if deps.Cfg == nil {
		panic("EnrollmentCoordinator: Cfg is required")
	}
	store := deps.Store
	if store == nil {
		store = NewCredentialStore(deps.FileSvc, deps.Cfg)
	}
	gateway := deps.Gateway
	if gateway == nil {
		gateway = NewEnrollmentClient(deps.Cfg, nil)
	}
	keys := deps.Keys
	if keys == nil {
		keys = FileKeyProvider{}
	}
	trust := deps.Trust
	if trust == nil {
		trust = platform.NewSystemTrustInstaller()
	}
	browser := deps.Browser
	if browser == nil {
		browser = defaultBrowserOpener{}
	}
	passkey := deps.Passkey
	if passkey == nil {
		passkey = &defaultPasskeyRegistrar{fileSvc: deps.FileSvc, cfg: deps.Cfg}
	}
	clock := deps.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	out := deps.Out
	if out == nil {
		out = func(string, ...any) {} // no-op default; command layer should supply
	}
	return &EnrollmentCoordinator{
		gateway: gateway,
		store:   store,
		keys:    keys,
		trust:   trust,
		browser: browser,
		passkey: passkey,
		fileSvc: deps.FileSvc,
		cfg:     deps.Cfg,
		clock:   clock,
		logger:  logger,
		out:     out,
	}
}

// EnrollmentResult is the return value of Enroll. It carries the identity
// info the command layer needs for display and the action the coordinator
// took, so callers (e.g., mcp agent run) can decide whether a passkey
// ceremony is still needed.
type EnrollmentResult struct {
	// Source is the enrollment operation that produced the current identity.
	// EnrollmentSourceBootstrap/Recovery/Rotation mean a new credential set
	// was issued; a zero value means the existing identity was reused.
	Source EnrollmentSource

	// Reused reports whether the coordinator reused the existing local
	// identity without issuing a new certificate.
	Reused bool

	// UserID is the gateway user ID bound to the CLI session.
	UserID string

	// CLISessionID is the gateway-issued CLI session ID.
	CLISessionID string

	// SystemTrustInstalled reports whether OS trust was newly installed
	// during this run. False when the root was already trusted or when
	// --no-system-trust skipped the installer.
	SystemTrustInstalled bool
}

// Enroll runs the §3 enrollment state machine to completion. It inspects
// the local CLI identity, decides the correct action (bootstrap, recovery,
// rotation, or reuse), stages and commits any new credentials, installs OS
// trust (unless --no-system-trust), and runs the passkey ceremony (unless
// SkipPasskey).
//
// ctx is propagated end-to-end through HTTP, polling, and the passkey
// ceremony. The command layer should pass cmd.Context() so user
// cancellation (Ctrl-C) aborts every phase.
func (c *EnrollmentCoordinator) Enroll(ctx context.Context, opts EnrollmentOptions) (*EnrollmentResult, error) {
	local, err := c.store.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, err)
	}
	c.out("Local identity state: %s", local.State)

	result := &EnrollmentResult{}

	// 1. Credential-preparation: bootstrap, recovery, rotation, or reuse.
	var artifacts EnrollmentArtifacts
	switch local.State {
	case LocalStateAbsent:
		artifacts, err = c.handleAbsent(ctx, local, opts)
	case LocalStateComplete:
		if local.NeedsRecovery() {
			// Complete-but-expired: an expired cert cannot authenticate via
			// mTLS, so rotation is impossible. Route through recovery.
			c.out("CLI certificate has expired; using recovery flow.")
			artifacts, err = c.handleRecovery(ctx, opts)
		} else if local.NeedsRotation() || opts.RotateCLI {
			artifacts, err = c.handleRotation(ctx, opts)
		} else {
			// Reuse the existing identity. No enrollment request, no new
			// certificate. The passkey ceremony still runs.
			result.Reused = true
			result.UserID = local.Credentials.UserID
			result.CLISessionID = local.Credentials.CLISessionID
			c.out("Reusing existing CLI identity (user %s, session %s).", result.UserID, result.CLISessionID)
		}
	case LocalStatePartial, LocalStateCorrupt:
		c.out("Local identity is %s; using human-approved recovery flow.", local.State)
		artifacts, err = c.handleRecovery(ctx, opts)
	default:
		return nil, fmt.Errorf("%w: unknown local state %d", constants.ErrInternal, local.State)
	}
	if err != nil {
		return nil, err
	}

	// 2. Stage + Commit new credentials (bootstrap/recovery/rotation only).
	// Skip when reusing — no new artifacts were produced. The zero-value
	// EnrollmentSource is EnrollmentSourceBootstrap (which IsLocalCLI()
	// returns true for), so we must guard on result.Reused to avoid staging
	// an empty EnrollmentArtifacts.
	if !result.Reused && artifacts.Source.IsLocalCLI() {
		staged, stErr := c.store.Stage(ctx, artifacts)
		if stErr != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, stErr)
		}
		if cmErr := c.store.Commit(ctx, staged); cmErr != nil {
			c.store.Rollback(staged)
			return nil, fmt.Errorf("%w: %w", constants.ErrEnrollmentFailed, cmErr)
		}
		result.Source = artifacts.Source
		result.UserID = artifacts.UserID
		result.CLISessionID = artifacts.CLISessionID
		c.out("%s complete (user %s, session %s).", artifacts.Source, result.UserID, result.CLISessionID)
	}

	// 3. Ensure system trust (local CLI paths only, unless --no-system-trust).
	if result.Source.IsLocalCLI() || result.Reused {
		installed, terr := c.ensureSystemTrust(ctx, artifacts, result.Reused, local, opts)
		if terr != nil {
			return nil, terr
		}
		result.SystemTrustInstalled = installed
	}

	// 4. Passkey ceremony (unless suppressed by an internal caller).
	if !opts.SkipPasskey {
		if err := c.runPasskeyCeremony(ctx, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// handleAbsent handles the LocalStateAbsent case. It checks whether the
// gateway has been bootstrapped: if not, it bootstraps (the first CLI
// creates the first user/session); if so, it creates a CLI recovery
// request and waits for human approval.
func (c *EnrollmentCoordinator) handleAbsent(ctx context.Context, _ LocalIdentity, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	bootstrapped, err := c.gateway.CheckBootstrapStatus(ctx, "")
	if err != nil {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: check bootstrap status: %w", constants.ErrEnrollmentFailed, err)
	}
	if !bootstrapped {
		c.out("Gateway not bootstrapped; performing initial CLI bootstrap.")
		return c.handleBootstrap(ctx, opts)
	}
	c.out("Gateway already bootstrapped; creating CLI recovery request.")
	return c.handleRecovery(ctx, opts)
}

// handleBootstrap generates a CLI CSR and performs the initial gateway
// bootstrap. The gateway creates the first user/session and returns the
// CLI cert plus the full runtime trust bundle.
func (c *EnrollmentCoordinator) handleBootstrap(ctx context.Context, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	csrPEM, cliKey, err := c.generateCLICSR(ctx)
	if err != nil {
		return EnrollmentArtifacts{}, err
	}
	// No operator CSR for local CLI enrollment (per §5.1: do not make
	// auth enroll depend on an operator certificate it does not need).
	return c.gateway.Bootstrap(ctx, csrPEM, cliKey, "", opts.CAFingerprint, "")
}

// handleRecovery creates a CLI recovery request, opens the browser for
// human approval, polls until approved/denied/expired, then completes the
// recovery with proof-of-possession to receive the issued CLI identity.
func (c *EnrollmentCoordinator) handleRecovery(ctx context.Context, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	csrPEM, cliKey, err := c.generateCLICSR(ctx)
	if err != nil {
		return EnrollmentArtifacts{}, err
	}

	requestID, token, approvalURL, expiresAt, err := c.gateway.CreateRecoveryRequest(ctx, csrPEM, "")
	if err != nil {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: %w", constants.ErrCLIRecoveryRequestFailed, err)
	}
	c.out("Recovery request created (expires at %s).", expiresAt.Format(time.RFC3339))
	c.out("Approval URL: %s", approvalURL)

	// Open the browser for the user to approve with an existing passkey.
	// A browser-open failure is not fatal — print the URL as a fallback so
	// the user can navigate manually. Section 8 will harden this.
	if openErr := c.browser.Open(approvalURL); openErr != nil {
		c.out("Warning: could not open browser automatically (%v). Please open the approval URL manually.", openErr)
	}

	c.out("Waiting for approval in the console...")
	if err := c.pollRecoveryApproval(ctx, token, ""); err != nil {
		return EnrollmentArtifacts{}, err
	}
	c.out("Recovery request approved. Completing enrollment...")
	return c.gateway.CompleteRecovery(ctx, requestID, token, csrPEM, cliKey, opts.CAFingerprint, "")
}

// handleRotation generates a new CLI CSR and rotates the existing CLI
// identity through the mTLS rotation endpoint. The gateway derives the
// user and session from the authenticated certificate context.
func (c *EnrollmentCoordinator) handleRotation(ctx context.Context, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	c.out("Rotating CLI certificate via mTLS rotation endpoint.")
	csrPEM, cliKey, err := c.generateCLICSR(ctx)
	if err != nil {
		return EnrollmentArtifacts{}, err
	}
	return c.gateway.Rotate(ctx, c.fileSvc, csrPEM, cliKey, opts.CAFingerprint)
}

// pollRecoveryApproval polls the recovery status endpoint with bounded
// backoff until the request is approved, denied, expired, or the context
// is cancelled. An immediate first check avoids a delay when the user has
// already approved.
func (c *EnrollmentCoordinator) pollRecoveryApproval(ctx context.Context, token, baseURL string) error {
	// Immediate first check.
	state, err := c.gateway.RecoveryStatus(ctx, token, baseURL)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCLIRecoveryRequestFailed, err)
	}
	if done, derr := recoveryStateDone(state); done {
		return derr
	}

	ticker := time.NewTicker(recoveryPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			state, err := c.gateway.RecoveryStatus(ctx, token, baseURL)
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrCLIRecoveryRequestFailed, err)
			}
			if done, derr := recoveryStateDone(state); done {
				return derr
			}
		}
	}
}

// recoveryStateDone reports whether a recovery state is terminal and
// returns the corresponding error (nil for approved).
func recoveryStateDone(state models.CLIRecoveryState) (bool, error) {
	switch state {
	case models.CLIRecoveryStateApproved:
		return true, nil
	case models.CLIRecoveryStateDenied:
		return true, constants.ErrCLIRecoveryRequestDenied
	case models.CLIRecoveryStateExpired:
		return true, constants.ErrCLIRecoveryRequestExpired
	case models.CLIRecoveryStateCompleted:
		return true, constants.ErrCLIRecoveryRequestConsumed
	default:
		return false, nil
	}
}

// ensureSystemTrust installs the gateway root CA into the OS trust store
// after the runtime bundle is valid and committed, and before the passkey
// ceremony. Per §6.5:
//   - Default: any installation error aborts before browser launch.
//   - --no-system-trust: skip the installer after an admin notice; still
//     run the passkey ceremony and still fail on runtime mTLS/trust-bundle
//     errors (those are checked earlier by Inspect/Stage).
//   - Already-trusted roots must not cause a privilege prompt.
//
// For a reused identity, the bundle comes from the local trust bundle on
// disk. For a new enrollment, it comes from the artifacts.
func (c *EnrollmentCoordinator) ensureSystemTrust(ctx context.Context, artifacts EnrollmentArtifacts, reused bool, local LocalIdentity, opts EnrollmentOptions) (bool, error) {
	if opts.NoSystemTrust {
		c.out("System trust installation skipped (--no-system-trust). The administrator must have pre-installed the gateway root CA.")
		return false, nil
	}

	var bundlePEM []byte
	if reused {
		if local.TrustBundle == nil || len(local.TrustBundle.PEM) == 0 {
			// A reused identity with no trust bundle is inconsistent — Inspect
			// should have classified it as partial. This is a defensive guard.
			return false, fmt.Errorf("%w: reused identity has no trust bundle", constants.ErrEnrollmentFailed)
		}
		bundlePEM = local.TrustBundle.PEM
	} else {
		bundlePEM = []byte(artifacts.TrustBundlePEM)
	}

	result, err := c.trust.EnsureSystemTrust(ctx, bundlePEM)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}

	switch result.Status {
	case platform.SystemTrustInstalled:
		c.out("System trust: installed gateway root CA (fingerprint %s).", result.Fingerprint)
		// Browser-restart note: already-running browsers may cache trust
		// state and not recognize the newly installed root. Per §6.5.
		c.out("Note: if your browser is already open, restart it so it picks up the new trust anchor.")
		return true, nil
	case platform.SystemTrustAlreadyTrusted:
		c.out("System trust: gateway root CA already trusted (fingerprint %s).", result.Fingerprint)
		return false, nil
	default:
		return false, nil
	}
}

// runPasskeyCeremony runs the browser-based passkey registration. It uses
// the userID and CLI session ID from the enrollment result (newly issued
// or reused identity).
func (c *EnrollmentCoordinator) runPasskeyCeremony(ctx context.Context, result *EnrollmentResult) error {
	if result.UserID == "" || result.CLISessionID == "" {
		return fmt.Errorf("%w: cannot register passkey without user/session ID", constants.ErrEnrollmentFailed)
	}
	c.out("Registering passkey via browser...")
	if err := c.passkey.Register(ctx, result.UserID, result.CLISessionID); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
	}
	c.out("Passkey registration complete.")
	return nil
}

// generateCLICSR generates a CLI key pair and CSR via the key provider,
// using a hostname-based common name.
func (c *EnrollmentCoordinator) generateCLICSR(ctx context.Context) (string, *ecdsa.PrivateKey, error) {
	hostname, err := osHostname()
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrNetworkGetHostname, err)
	}
	commonName := fmt.Sprintf("g8e-cli-%s", hostname)
	csrPEM, key, err := c.keys.GenerateCLIKeyAndCSR(ctx, commonName)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrCSRGenerationFailed, err)
	}
	return csrPEM, key, nil
}

// Compile-time interface satisfaction checks.
var (
	_ EnrollmentGateway    = (*EnrollmentClient)(nil)
	_ SystemTrustInstaller = (*platform.SystemTrustInstaller)(nil)
	_ KeyProvider          = FileKeyProvider{}
	_ BrowserOpener        = defaultBrowserOpener{}
	_ PasskeyRegistrar     = (*defaultPasskeyRegistrar)(nil)
)

// --- Default implementations ---

// defaultBrowserOpener wraps platform.OpenBrowser.
type defaultBrowserOpener struct{}

// Open opens the URL in the user's default browser.
func (defaultBrowserOpener) Open(url string) error {
	return platform.OpenBrowser(url)
}

// defaultPasskeyRegistrar wraps the existing RegisterPasskeyViaBrowser
// function. Section 8 will replace this with a hardened registrar that
// prepares the SSE listener before browser launch, fixes the cursor
// strategy, and surfaces browser-open errors.
type defaultPasskeyRegistrar struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

// Register delegates to RegisterPasskeyViaBrowser.
func (r *defaultPasskeyRegistrar) Register(_ context.Context, userID, cliSessionID string) error {
	return RegisterPasskeyViaBrowser(r.fileSvc, r.cfg, userID, cliSessionID)
}
