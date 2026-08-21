// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
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
	CheckActivationStatus(ctx context.Context, baseURL string) (bool, error)

	// DiscoverGatewayCA fetches the live gateway root CA bundle from the
	// unauthenticated discovery surface (plain-HTTP
	// /.well-known/g8e/pki/ca-bundle). It returns the PEM bundle and the
	// SHA-256 fingerprint of the primary root anchor. The coordinator uses
	// this BEFORE the reuse decision to detect a stale local trust bundle
	// (e.g., after `gw clean` regenerated the gateway PKI).
	//
	// The call is best-effort: a network failure returns a non-nil err and
	// empty bundle/fingerprint. The coordinator decides whether to abort;
	// discovery failure on a complete-reuse identity prints a diagnostic
	// warning and proceeds (the user may be intentionally offline).
	//
	// No fingerprint pin is applied — the live bundle IS the source of
	// truth for the pin, so pinning against the local bundle would be
	// circular.
	DiscoverGatewayCA(ctx context.Context) (bundlePEM []byte, fingerprint string, err error)
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
// trust store and manages stale g8e anchors from previous gateway instances.
// The concrete *platform.SystemTrustInstaller satisfies this interface; tests
// inject a mock to avoid sudo/exec.
type SystemTrustInstaller interface {
	// IsTrusted reports whether the OS trust store already contains a root
	// anchor with the given SHA-256 fingerprint. Reads only.
	IsTrusted(ctx context.Context, fingerprint string) (bool, error)
	// InstallRoot writes the root anchor into the OS trust store. Writes
	// only; the caller checks IsTrusted first.
	InstallRoot(ctx context.Context, root *x509.Certificate, fingerprint string) error
	ListStaleAnchors(ctx context.Context, currentFingerprint string) ([]platform.StaleAnchor, error)
	RemoveStaleAnchors(ctx context.Context, anchors []platform.StaleAnchor) error
}

// ConfirmFunc prompts the user with a yes/no question and returns true if the
// user confirms. The command layer injects a stdin-reading implementation;
// tests inject a deterministic stub. Returning false aborts the operation
// that requested confirmation.
type ConfirmFunc func(prompt string) bool

// ContinueFunc prompts the user to press Enter to continue and returns true
// when the user has done so. The command layer injects a stdin-reading
// implementation; tests inject a deterministic stub. Returning false aborts
// the operation that requested the continue. This is a separate type from
// ConfirmFunc because it encodes a different user intent (Enter-to-continue
// versus y/N confirmation); keeping them distinct lets tests inject one stub
// per dependency and assert on each prompt independently.
type ContinueFunc func(prompt string) bool

// BrowserOpener opens a URL in the user's default browser. Used by the
// coordinator to open the recovery approval URL. The passkey registrar
// manages its own browser launch (Section 8 will harden that path).
type BrowserOpener interface {
	Open(url string) error
}

// PasskeyRegistrar runs the browser-based passkey registration ceremony.
// The default implementation (passkeyRegistrar) prepares the SSE listener
// before browser launch, uses a correct cursor strategy (since_id=0 for
// live-only events), filters events by type/user/session, surfaces
// browser-open errors, and propagates context cancellation.
type PasskeyRegistrar interface {
	// Register opens the gateway console for WebAuthn passkey registration
	// and waits for the passkey.registered SSE event. The userID and
	// cliSessionID identify the newly enrolled (or reused) CLI identity.
	// The userID is used for client-side SSE event filtering; the gateway
	// derives identity from the mTLS certificate context.
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
	gateway    EnrollmentGateway
	store      *CredentialStore
	keys       KeyProvider
	trust      SystemTrustInstaller
	browser    BrowserOpener
	passkey    PasskeyRegistrar
	confirm    ConfirmFunc
	continueFn ContinueFunc
	fileSvc    fs.RuntimeFileService
	cfg        *config.Config
	clock      func() time.Time
	logger     *slog.Logger
	out        OutputFunc
}

// EnrollmentCoordinatorDeps holds the injectable dependencies for an
// EnrollmentCoordinator. Fields left nil get production defaults. Tests
// populate the fields they need to control.
type EnrollmentCoordinatorDeps struct {
	Gateway  EnrollmentGateway
	Store    *CredentialStore
	Keys     KeyProvider
	Trust    SystemTrustInstaller
	Browser  BrowserOpener
	Passkey  PasskeyRegistrar
	Confirm  ConfirmFunc
	Continue ContinueFunc
	FileSvc  fs.RuntimeFileService
	Cfg      *config.Config
	Clock    func() time.Time
	Logger   *slog.Logger
	Out      OutputFunc
}

// NewEnrollmentCoordinator constructs a coordinator from the given deps.
// Nil fields get production defaults:
//   - Gateway: a new *EnrollmentClient (plain HTTP, no mTLS).
//   - Store: a new *CredentialStore over FileSvc/Cfg.
//   - Keys: a FileKeyProvider (file-backed EC P-256 on all platforms).
//   - Trust: a new *platform.SystemTrustInstaller (real os/exec).
//   - Browser: a defaultBrowserOpener wrapping platform.OpenBrowser.
//   - Passkey: a defaultPasskeyRegistrar wrapping the hardened passkeyRegistrar.
//   - Confirm: an auto-confirm stub (always returns true). The interactive
//     `auth enroll user` command overrides this with a stdin-reading impl.
//   - Continue: an auto-continue stub (always returns true). The interactive
//     `auth enroll user` command overrides this with a stdin-reading impl.
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
		passkey = &defaultPasskeyRegistrar{
			registrar: newPasskeyRegistrar(deps.FileSvc, deps.Cfg, PasskeyRegistrarOptions{Out: deps.Out}),
		}
	}
	confirm := deps.Confirm
	if confirm == nil {
		// Default: auto-confirm. The interactive `auth enroll user` command layer
		// overrides this with a stdin-reading implementation. Internal callers
		// (e.g., mcp agent run) that don't supply a ConfirmFunc get the
		// auto-confirm default so they don't block on missing stdin.
		confirm = func(string) bool { return true }
	}
	continueFn := deps.Continue
	if continueFn == nil {
		// Default: auto-continue. The interactive `auth enroll user` command layer
		// overrides this with a stdin-reading implementation. Internal callers
		// (e.g., mcp agent run, demos) that don't supply a ContinueFunc get
		// the auto-continue default so they don't block on missing stdin.
		continueFn = func(string) bool { return true }
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
		gateway:    gateway,
		store:      store,
		keys:       keys,
		trust:      trust,
		browser:    browser,
		passkey:    passkey,
		confirm:    confirm,
		continueFn: continueFn,
		fileSvc:    deps.FileSvc,
		cfg:        deps.Cfg,
		clock:      clock,
		logger:     logger,
		out:        out,
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

	// Discover the live gateway root CA before the state-machine switch.
	// This is best-effort: on network failure we fall through to the
	// existing state machine (offline reuse, absent identity → bootstrap
	// will fail with a clear network error, etc.). We do NOT abort
	// enrollment on discovery failure — that would break the
	// air-gapped/offline case. The BundleStale flag is only meaningful on
	// the complete-reuse path; the new-enrollment paths (bootstrap/
	// recovery/rotation) receive a fresh bundle in their artifacts.
	//
	// Per Open Question 2: run discovery unconditionally at the top of
	// Enroll (one cheap round-trip, best-effort) so the live fingerprint
	// is available for all paths, but only the complete-reuse path uses
	// it for routing. This keeps the state machine simple and avoids a
	// per-branch discovery call.
	liveBundle, liveFingerprint, discoveryErr := c.gateway.DiscoverGatewayCA(ctx)
	discoveryReachable := discoveryErr == nil && liveFingerprint != ""

	// Classify the local identity against the live gateway. Only meaningful
	// when the local identity is complete (has a trust bundle with a
	// primary root fingerprint) AND discovery succeeded.
	if local.State == LocalStateComplete && discoveryReachable {
		if local.TrustBundle != nil && local.TrustBundle.PrimaryRootFingerprint != "" &&
			local.TrustBundle.PrimaryRootFingerprint != liveFingerprint {
			local.BundleStale = true
		}
	}

	// 0. Stale trust-anchor cleanup. This runs BEFORE any enrollment
	// attempt (bootstrap, recovery, rotation, or reuse) so that stale
	// g8e root CAs from a previous gateway instance are removed from the
	// OS trust store before the user starts a recovery approval flow or
	// a passkey ceremony. Without this, the user would create a recovery
	// request, wait for approval, receive new credentials, and only then
	// be prompted about stale anchors — by which point the browser may
	// have already cached the wrong trust state.
	//
	// The cleanup uses the LIVE gateway root CA fingerprint from
	// DiscoverGatewayCA as the "keep" fingerprint. When discovery is
	// unreachable, cleanup is skipped (the coordinator cannot determine
	// which anchors are stale without the live fingerprint). The R5
	// warning in installSystemTrust surfaces this condition on the
	// reuse path.
	//
	// This is a single-purpose method separate from installSystemTrust
	// (which only installs). The two are composed by the caller: cleanup
	// reads/removes, install reads/writes. Neither is an ensure pattern.
	staleRemoved, err := c.cleanupStaleAnchors(ctx, liveFingerprint, discoveryReachable)
	if err != nil {
		return nil, err
	}

	result := &EnrollmentResult{}

	// 1. Credential-preparation: bootstrap, recovery, rotation, or reuse.
	var artifacts EnrollmentArtifacts
	switch local.State {
	case LocalStateAbsent:
		artifacts, err = c.handleAbsent(ctx, local, opts)
	case LocalStateComplete:
		if local.NeedsRecovery() {
			// Complete-but-expired: an expired cert cannot authenticate via
			// mTLS, so rotation is impossible. Route through recovery — but
			// only if the gateway is activated. On a fresh gateway (no
			// users), the recovery endpoint returns 403, so we must
			// bootstrap instead. This mirrors handleAbsent/handlePartialOrCorrupt.
			activated, actErr := c.gateway.CheckActivationStatus(ctx, "")
			if actErr != nil {
				return nil, fmt.Errorf("%w: check activation status: %w", constants.ErrEnrollmentFailed, actErr)
			}
			if !activated {
				c.out("CLI certificate has expired but gateway is not activated; performing initial CLI bootstrap.")
				artifacts, err = c.handleBootstrap(ctx, opts)
			} else {
				c.out("CLI certificate has expired; using recovery flow.")
				artifacts, err = c.handleRecovery(ctx, opts)
			}
		} else if local.BundleStale {
			// Complete-but-stale-bundle: the local CLI cert was issued by
			// the old gateway CA and cannot authenticate to the new
			// gateway via mTLS, so rotation is impossible (rotation uses
			// mTLS). The only valid path on an activated gateway is
			// recovery (human-approved, plain-HTTP, token-scoped), which
			// issues a fresh cert signed by the new CA. On a fresh
			// gateway (no users), the recovery endpoint returns 403, so
			// we must bootstrap instead. This mirrors the CertExpired →
			// recovery routing: a stale bundle is functionally equivalent
			// to an expired cert for mTLS purposes.
			activated, actErr := c.gateway.CheckActivationStatus(ctx, "")
			if actErr != nil {
				return nil, fmt.Errorf("%w: check activation status: %w", constants.ErrEnrollmentFailed, actErr)
			}
			if !activated {
				c.out("Local trust bundle does not match the live gateway root CA but gateway is not activated; performing initial CLI bootstrap.")
				artifacts, err = c.handleBootstrap(ctx, opts)
			} else {
				c.out("Local trust bundle does not match the live gateway root CA; using recovery flow.")
				artifacts, err = c.handleRecovery(ctx, opts)
			}
		} else if local.NeedsRotation() || opts.RotateCLI {
			artifacts, err = c.handleRotation(ctx, opts)
		} else {
			// Reuse the existing identity. No enrollment request, no new
			// certificate. The passkey ceremony still runs.
			result.Reused = true
			result.UserID = local.Credentials.UserID
			result.CLISessionID = local.Credentials.CLISessionID
			c.out("Reusing existing CLI identity (user %s, session %s).", result.UserID, result.CLISessionID)

			// R4: refresh the local trust bundle from the live gateway
			// bundle when they differ in non-root content (e.g.,
			// intermediates rotated but root unchanged). The root
			// fingerprint already matched (BundleStale is false), so this
			// is a minor hardening — the local bundle is still usable for
			// mTLS, but refreshing keeps intermediates current. Best-effort:
			// a write failure is logged but does not abort enrollment.
			if discoveryReachable && len(liveBundle) > 0 && local.TrustBundle != nil {
				if !bytes.Equal(local.TrustBundle.PEM, liveBundle) {
					if wErr := WriteTrustBundleToDisk(ctx, c.fileSvc, c.cfg, liveBundle); wErr != nil {
						c.out("Warning: could not refresh local trust bundle from gateway (%v). Proceeding with the existing bundle.", wErr)
					} else {
						c.out("Refreshed local trust bundle from gateway (intermediate certificates updated).")
					}
				}
			}
		}
	case LocalStatePartial, LocalStateCorrupt:
		artifacts, err = c.handlePartialOrCorrupt(ctx, local, opts)
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

	// 3. Install system trust (local CLI paths only, unless --no-system-trust).
	if result.Source.IsLocalCLI() || result.Reused {
		installed, terr := c.installSystemTrust(ctx, artifacts, result.Reused, local, opts, liveFingerprint, liveBundle, discoveryReachable)
		if terr != nil {
			return nil, terr
		}
		result.SystemTrustInstalled = installed

		// 3b. Blocking browser-restart gate. The trust store changed (a new
		// root was installed or stale anchors were removed), so the user MUST
		// close all open browser windows before the passkey ceremony opens a
		// fresh browser session that recognizes the new trust anchor. The
		// gate is skipped when SkipPasskey is set: those callers (mcp agent
		// run, demos) inject an auto-continue ContinueFunc and open no
		// browser themselves, so the "close all open browser windows" line
		// would pollute their non-interactive output streams with no
		// actionable consequence.
		browserRestartNeeded := staleRemoved || installed
		if browserRestartNeeded && !opts.SkipPasskey {
			c.out("The gateway root CA was installed (or stale anchors were removed).")
			c.out("Browsers cache certificate trust state. You MUST close all open browser windows now,")
			c.out("then press Enter to continue. The passkey enrollment URL will be displayed next.")
			if !c.continueFn("Press Enter to continue after closing all browser windows...") {
				c.out("Browser restart declined. Aborting enrollment before passkey ceremony.")
				return nil, constants.ErrBrowserRestartDeclined
			}
		}
	}

	// 4. Passkey ceremony (unless suppressed by an internal caller or
	// --headless, which opts into a CLI-only identity). Headless implies
	// SkipPasskey; the OR keeps any explicit SkipPasskey from internal
	// callers that also set Headless (none today, but defensive).
	opts.SkipPasskey = opts.SkipPasskey || opts.Headless
	if !opts.SkipPasskey {
		if err := c.runPasskeyCeremony(ctx, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// handleAbsent handles the LocalStateAbsent case. It checks whether the
// gateway has been activated (a human user exists): if not, it bootstraps
// (the first `auth enroll user` creates the first real user/session); if
// so, it creates a CLI recovery request and waits for human approval.
func (c *EnrollmentCoordinator) handleAbsent(ctx context.Context, _ LocalIdentity, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	activated, err := c.gateway.CheckActivationStatus(ctx, "")
	if err != nil {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: check activation status: %w", constants.ErrEnrollmentFailed, err)
	}
	if !activated {
		c.out("Gateway not activated; performing initial CLI bootstrap.")
		return c.handleBootstrap(ctx, opts)
	}
	c.out("Gateway already activated; creating CLI recovery request.")
	return c.handleRecovery(ctx, opts)
}

// handlePartialOrCorrupt handles the LocalStatePartial and
// LocalStateCorrupt cases. It checks whether the gateway has been
// activated (a human user exists): if not, it bootstraps (the gateway's
// recovery endpoint rejects unactivated gateways with 403, so we must
// not attempt recovery); if so, it creates a CLI recovery request and
// waits for human approval, just like handleAbsent on an activated
// gateway.
func (c *EnrollmentCoordinator) handlePartialOrCorrupt(ctx context.Context, local LocalIdentity, opts EnrollmentOptions) (EnrollmentArtifacts, error) {
	activated, err := c.gateway.CheckActivationStatus(ctx, "")
	if err != nil {
		return EnrollmentArtifacts{}, fmt.Errorf("%w: check activation status: %w", constants.ErrEnrollmentFailed, err)
	}
	if !activated {
		c.out("Gateway not activated; performing initial CLI bootstrap despite %s local state.", local.State)
		return c.handleBootstrap(ctx, opts)
	}
	c.out("Local identity is %s; using human-approved recovery flow.", local.State)
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
	// auth enroll user depend on an operator certificate it does not need).
	return c.gateway.Bootstrap(ctx, csrPEM, cliKey, "", opts.CAFingerprint, "")
}

// handleRecovery creates a CLI recovery request, opens the browser for
// human approval (or, under --headless, prints the approve-recovery command
// for an already-enrolled CLI to run), polls until approved/denied/expired,
// then completes the recovery with proof-of-possession to receive the issued
// CLI identity.
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

	// The gateway builds the approval URL from its own PublicBaseURL config,
	// which defaults to localhost. When the user supplied -e/--endpoint (with
	// optional --port), the CLI's HTTPS endpoint override reflects the address
	// the user's browser can actually reach, so rewrite the approval URL base
	// to match. Without this, a remote user who passed -e dev.g8e.local would
	// see an approval URL pointing at localhost and be unable to open it.
	approvalURL = c.rewriteApprovalURLBase(approvalURL)

	if opts.Headless {
		// Headless path: do not open a browser. Print the approve-recovery
		// command for an already-enrolled CLI to run via mTLS. The polling
		// loop below waits for that approval to land.
		c.out("From an already-enrolled CLI, run:")
		c.out("  g8e auth approve-recovery %s", token)
		c.out("Then wait for this command to complete.")
	} else {
		c.out("Approval URL: %s", approvalURL)
		// Open the browser for the user to approve with an existing passkey.
		// A browser-open failure is not fatal — print the URL as a fallback so
		// the user can navigate manually. Section 8 will harden this.
		if openErr := c.browser.Open(approvalURL); openErr != nil {
			c.out("Warning: could not open browser automatically (%v). Please open the approval URL manually.", openErr)
		}
		c.out("Waiting for approval in the console...")
	}

	if err := c.pollRecoveryApproval(ctx, token, ""); err != nil {
		return EnrollmentArtifacts{}, err
	}
	c.out("Recovery request approved. Completing enrollment...")
	return c.gateway.CompleteRecovery(ctx, requestID, token, csrPEM, cliKey, opts.CAFingerprint, "")
}

// rewriteApprovalURLBase replaces the scheme and host:port of the gateway-
// returned approval URL with the CLI's OperatorPublicURL when the user supplied
// -e/--endpoint (with optional --port). The path and fragment (which carries
// the opaque recovery token) are preserved. When no endpoint override is set,
// the approval URL is returned unchanged so the gateway's own PublicBaseURL
// configuration is respected.
func (c *EnrollmentCoordinator) rewriteApprovalURLBase(approvalURL string) string {
	if !config.HasHTTPSEndpointOverride() {
		return approvalURL
	}
	parsed, err := url.Parse(approvalURL)
	if err != nil {
		return approvalURL
	}
	base, err := url.Parse(c.cfg.OperatorPublicURL())
	if err != nil {
		return approvalURL
	}
	parsed.Scheme = base.Scheme
	parsed.Host = base.Host
	return parsed.String()
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

// cleanupStaleAnchors detects and removes stale g8e root CA anchors from
// the OS trust store before any enrollment attempt. It is a single-purpose
// method: it reads the trust store (ListStaleAnchors), prompts the user,
// and removes stale anchors (RemoveStaleAnchors). It does not install
// anything — that is installSystemTrust's job.
//
// keepFingerprint is the SHA-256 fingerprint of the LIVE gateway root CA
// (from DiscoverGatewayCA). It MUST come from the live gateway, not the
// local bundle — otherwise the detector would compare the stale local
// bundle against itself and see nothing wrong. When discovery was
// unreachable (keepFingerprint is empty), cleanup is skipped: the
// coordinator cannot determine which anchors are stale without the live
// fingerprint. The R5 warning in installSystemTrust surfaces this
// condition on the reuse path.
//
// Returns staleRemoved (true when at least one stale anchor was removed
// this run) so the caller can set browserRestartNeeded and gate the
// passkey ceremony behind a blocking browser-restart prompt.
func (c *EnrollmentCoordinator) cleanupStaleAnchors(ctx context.Context, keepFingerprint string, discoveryReachable bool) (staleRemoved bool, err error) {
	if !discoveryReachable || keepFingerprint == "" {
		return false, nil
	}

	// ListStaleAnchors. Best-effort: ErrSystemTrustUnsupported is a no-op;
	// other errors print a warning and proceed (stale detection is a safety
	// improvement, not a gate).
	staleAnchors, listErr := c.trust.ListStaleAnchors(ctx, keepFingerprint)
	if listErr != nil {
		if !errors.Is(listErr, constants.ErrSystemTrustUnsupported) {
			c.out("Warning: could not check for stale trust anchors (%v). Proceeding with enrollment.", listErr)
		}
		return false, nil
	}
	if len(staleAnchors) == 0 {
		return false, nil
	}

	// Print the stale anchors and prompt the user.
	c.out("Found %d stale g8e root CA anchor(s) from a previous gateway instance:", len(staleAnchors))
	for i, a := range staleAnchors {
		c.out("  %d. %s (fingerprint %s)", i+1, a.CommonName, a.Fingerprint)
	}
	prompt := fmt.Sprintf("Remove these %d stale anchor(s) before enrolling? [y/N]: ", len(staleAnchors))
	if !c.confirm(prompt) {
		c.out("Stale anchor removal declined. Aborting enrollment.")
		return false, constants.ErrSystemTrustStaleRemovalDenied
	}
	c.out("Removing stale anchors...")
	if err := c.trust.RemoveStaleAnchors(ctx, staleAnchors); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	c.out("Stale anchors removed.")
	return true, nil
}

// installSystemTrust installs the gateway root CA into the OS trust store
// after the runtime bundle is valid and committed, and before the passkey
// ceremony. It is a single-purpose method: it reads the trust store
// (IsTrusted) and writes to it (InstallRoot). Stale anchor cleanup is
// handled separately by cleanupStaleAnchors, which runs before the
// enrollment state machine. Per §6.5:
//   - Default: any installation error aborts before browser launch.
//   - --no-system-trust: skip the installer after an admin notice.
//     Stale-anchor cleanup still ran earlier (in cleanupStaleAnchors).
//   - Already-trusted roots must not cause a privilege prompt.
//
// liveFingerprint is the SHA-256 fingerprint of the LIVE gateway root CA
// (from DiscoverGatewayCA). When discovery was unreachable,
// liveFingerprint is empty and the coordinator falls back to the bundle's
// own fingerprint (preserving the pre-discovery behavior) after printing a
// diagnostic warning.
//
// liveBundle is the live gateway CA bundle PEM (from DiscoverGatewayCA).
// On the reused-identity path, when discovery succeeded, the live bundle
// is used as the bundlePEM so the install check compares against the live
// root, not the (possibly stale) local one. On the new-enrollment paths,
// the artifacts' bundle IS the live bundle, so liveBundle is not used.
//
// For a reused identity with discovery unreachable, the bundle comes from
// the local trust bundle on disk. For a new enrollment, it comes from the
// artifacts.
//
// The return value is installed (true only when a new root was newly
// installed this run). The caller combines this with the staleRemoved
// return from cleanupStaleAnchors to decide whether a browser restart is
// needed before the passkey ceremony.
func (c *EnrollmentCoordinator) installSystemTrust(ctx context.Context, artifacts EnrollmentArtifacts, reused bool, local LocalIdentity, opts EnrollmentOptions, liveFingerprint string, liveBundle []byte, discoveryReachable bool) (installed bool, err error) {
	// Step 1: Resolve the bundle PEM. On the reused-identity path, prefer
	// the live bundle (when discovery succeeded) so the install check
	// compares against the live root, not the stale local one. On the
	// new-enrollment paths, the artifacts' bundle IS the live bundle.
	//
	// R5: surface a diagnosable warning when discovery is unreachable AND
	// reuse is attempted. The local bundle may be stale (e.g., after
	// `gw clean`); without discovery we cannot tell. Print a clear
	// diagnostic before the passkey ceremony so a subsequent TLS failure
	// has prior context. Do NOT abort — the user may be intentionally
	// offline, or the gateway may be reachable only on the HTTPS port.
	var bundlePEM []byte
	if reused {
		if discoveryReachable && len(liveBundle) > 0 {
			bundlePEM = liveBundle
		} else {
			if !discoveryReachable {
				c.out("Warning: could not reach the gateway discovery endpoint to verify the local trust bundle is current. If the gateway's PKI has been regenerated (e.g., after `gw clean`), the local identity may be stale and the passkey ceremony will fail with a TLS error. Use --endpoint <host> if the gateway is on a remote host.")
			}
			if local.TrustBundle == nil || len(local.TrustBundle.PEM) == 0 {
				// A reused identity with no trust bundle is inconsistent — Inspect
				// should have classified it as partial. This is a defensive guard.
				return false, fmt.Errorf("%w: reused identity has no trust bundle", constants.ErrEnrollmentFailed)
			}
			bundlePEM = local.TrustBundle.PEM
		}
	} else {
		bundlePEM = []byte(artifacts.TrustBundlePEM)
	}

	// Step 2: Extract root anchors + primary fingerprint via the pure
	// platform helpers. This is the KEEP fingerprint — it MUST come from
	// the live bundle when discovery succeeded; the R5 warning above fires
	// when it cannot.
	rootAnchors, err := platform.ExtractRootAnchors(bundlePEM, c.clock)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInvalidAnchor, err)
	}
	if len(rootAnchors) == 0 {
		return false, constants.ErrSystemTrustInvalidAnchor
	}
	if vErr := platform.VerifyRootUsable(rootAnchors, bundlePEM, c.clock); vErr != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInvalidAnchor, vErr)
	}
	primary := rootAnchors[0]
	keepFingerprint := platform.CertFingerprint(primary)
	// Prefer the live fingerprint from discovery (the source of truth) over
	// the bundle's own fingerprint. When discovery was unreachable,
	// keepFingerprint stays as the bundle's own fingerprint — this preserves
	// the pre-discovery behavior but cannot detect a stale bundle (the local
	// bundle and the OS store are stale in lockstep, so they agree with each
	// other). The R5 warning above surfaces this condition to the user.
	if liveFingerprint != "" {
		keepFingerprint = liveFingerprint
	}

	// Step 3: If --no-system-trust, print the skip notice and return.
	// Stale-anchor cleanup already ran in cleanupStaleAnchors before the
	// enrollment state machine.
	if opts.NoSystemTrust {
		c.out("System trust installation skipped (--no-system-trust). The administrator must have pre-installed the gateway root CA.")
		return false, nil
	}

	// Step 4: IsTrusted. If ErrSystemTrustUnsupported, warn and return
	// (do not fail enrollment on a platform we cannot query). If trusted,
	// print "already trusted" and return.
	trusted, err := c.trust.IsTrusted(ctx, keepFingerprint)
	if err != nil {
		if errors.Is(err, constants.ErrSystemTrustUnsupported) {
			c.out("Warning: OS trust store query is unsupported on this platform (%v). Skipping system trust installation.", err)
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	if trusted {
		c.out("System trust: gateway root CA already trusted (fingerprint %s).", keepFingerprint)
		return false, nil
	}

	// Step 5: InstallRoot. If ErrSystemTrustUnsupported, warn and return.
	// On success print "installed". On any other error return wrapped
	// ErrSystemTrustInstallFailed.
	if err := c.trust.InstallRoot(ctx, primary, keepFingerprint); err != nil {
		if errors.Is(err, constants.ErrSystemTrustUnsupported) {
			c.out("Warning: OS trust store installation is unsupported on this platform (%v). Skipping system trust installation.", err)
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	c.out("System trust: installed gateway root CA (fingerprint %s).", keepFingerprint)
	return true, nil
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

// defaultPasskeyRegistrar wraps the hardened passkeyRegistrar. It satisfies
// the PasskeyRegistrar interface and is the production default injected by
// NewEnrollmentCoordinator.
type defaultPasskeyRegistrar struct {
	registrar *passkeyRegistrar
}

// Register delegates to passkeyRegistrar.Register.
func (r *defaultPasskeyRegistrar) Register(ctx context.Context, userID, cliSessionID string) error {
	return r.registrar.Register(ctx, userID, cliSessionID)
}
