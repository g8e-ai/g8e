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
	"crypto/ecdsa"
	"crypto/x509"
	"time"
)

// LocalEnrollmentState is the coordinator's classification of the local CLI
// identity on disk, per the state machine in the enrollment plan §3. It is
// the single source of truth for "what should the coordinator do next" and
// replaces the ad-hoc "do two files exist?" check that performEnroll used.
//
// The classification is derived from a complete-set inspection performed by
// CredentialStore.Inspect — never from individual file existence checks at
// the call site.
type LocalEnrollmentState int

const (
	// LocalStateAbsent means no managed artifacts are present. This is the
	// first-time-CLI state: the coordinator will bootstrap (unbootstrapped
	// gateway) or create a CLI recovery request (bootstrapped gateway).
	LocalStateAbsent LocalEnrollmentState = iota

	// LocalStateComplete means all managed artifacts are present, parseable,
	// and mutually consistent: credentials JSON, CLI cert, CLI key (or
	// platform-key reference), and the runtime trust bundle. The coordinator
	// reuses this identity and does not issue a new certificate unless
	// rotation is required (expiring) or explicitly requested.
	LocalStateComplete

	// LocalStatePartial means some but not all managed artifacts are present,
	// OR the present artifacts fail to parse or are mutually inconsistent.
	// The coordinator never treats partial state as a valid identity; it
	// routes through the human-approved recovery flow and replaces the
	// incomplete set.
	LocalStatePartial

	// LocalStateCorrupt is a specialization of LocalStatePartial where the
	// credentials JSON is present but unparseable, or the CLI cert/key pair
	// is present but the cert does not match the key. Recovery is the only
	// safe path; the coordinator must not overwrite one file at a time.
	LocalStateCorrupt
)

// String returns a human-readable label for the state, suitable for command-
// layer progress output.
func (s LocalEnrollmentState) String() string {
	switch s {
	case LocalStateAbsent:
		return "absent"
	case LocalStateComplete:
		return "complete"
	case LocalStatePartial:
		return "partial"
	case LocalStateCorrupt:
		return "corrupt"
	default:
		return "unknown"
	}
}

// LocalIdentity is the coordinator's typed view of the local CLI identity
// on disk, produced by CredentialStore.Inspect. It carries the parsed
// artifacts needed by the state machine and the rotation/reuse decision —
// never raw file bytes.
//
// State reflects LOCAL file consistency only. It does NOT reflect whether
// the local trust bundle matches the LIVE gateway root CA — that is a
// coordinator-level concern (the coordinator compares
// TrustBundle.PrimaryRootFingerprint against the live fingerprint from
// EnrollmentGateway.DiscoverGatewayCA and sets BundleStale accordingly).
// Inspect has no gateway access and must not perform network I/O.
type LocalIdentity struct {
	State LocalEnrollmentState

	// Credentials is the parsed credentials JSON. Nil when State is
	// LocalStateAbsent or when the JSON failed to parse (LocalStateCorrupt).
	Credentials *Credentials

	// CLICert is the parsed CLI certificate. Nil when absent or unparseable.
	CLICert *x509.Certificate

	// HasCLIKey reports whether the CLI private key file is present and
	// parseable. The key bytes themselves are not loaded here; the mTLS
	// client builder loads them lazily via the key provider.
	HasCLIKey bool

	// KeyMatchesCert reports whether the CLI key's public key matches the
	// CLI certificate's public key. False when either side is missing or
	// when they mismatch (LocalStateCorrupt).
	KeyMatchesCert bool

	// TrustBundle is the parsed runtime trust bundle. Nil when absent or
	// unparseable.
	TrustBundle *TrustBundle

	// CertExpiring reports whether the CLI certificate expires within the
	// coordinator's rotation threshold. Only meaningful when State is
	// LocalStateComplete and CLICert is non-nil.
	CertExpiring bool

	// CertExpired reports whether the CLI certificate has already expired.
	// An expired cert cannot be used for mTLS rotation; the coordinator
	// routes through recovery instead.
	CertExpired bool

	// BundleStale reports whether the local trust bundle's primary root
	// fingerprint does NOT match the live gateway root CA fingerprint
	// (e.g., after `gw clean` regenerated the gateway PKI). Set by the
	// coordinator AFTER comparing TrustBundle.PrimaryRootFingerprint to
	// the live fingerprint returned by DiscoverGatewayCA. Inspect does
	// NOT set this — it has no gateway access.
	//
	// When true on a LocalStateComplete identity, the local CLI cert was
	// issued by the old CA and cannot authenticate to the new gateway via
	// mTLS, so rotation is impossible. The coordinator routes through
	// recovery (human-approved, plain-HTTP, token-scoped), which issues a
	// fresh cert signed by the new CA.
	BundleStale bool
}

// NeedsRotation reports whether the coordinator should perform an mTLS CLI
// rotation before the passkey ceremony. True when the local identity is
// complete but the cert is expiring within the rotation threshold. An
// already-expired cert cannot rotate (it cannot authenticate) and must use
// recovery instead.
func (i LocalIdentity) NeedsRotation() bool {
	return i.State == LocalStateComplete && i.CertExpiring && !i.CertExpired
}

// NeedsRecovery reports whether the coordinator must use the human-approved
// CLI recovery flow. True when the local state is absent on a bootstrapped
// gateway, partial, corrupt, or complete-but-expired (an expired cert
// cannot rotate via mTLS).
func (i LocalIdentity) NeedsRecovery() bool {
	switch i.State {
	case LocalStatePartial, LocalStateCorrupt:
		return true
	case LocalStateComplete:
		return i.CertExpired
	default:
		return false
	}
}

// EnrollmentArtifacts is the coordinator's internal result type for a
// successful credential-preparation step (bootstrap, recovery completion,
// rotation, or remote operator/device enrollment). It replaces the
// catch-all auth.RegistrationResponse union that callers previously used.
//
// Wire models in internal/models are mapped into this type by the
// enrollment client after centralized response validation. The coordinator
// then stages and commits these artifacts via CredentialStore.
type EnrollmentArtifacts struct {
	// Source identifies which enrollment operation produced these artifacts.
	Source EnrollmentSource

	// CLISessionID is the gateway-issued CLI session ID. Required for all
	// local CLI enrollment paths.
	CLISessionID string

	// UserID is the gateway user ID bound to the CLI session.
	UserID string

	// OperatorSessionID is the operator session ID, when the enrollment
	// path created one (bootstrap, recovery, remote operator/device). Empty
	// for CLI-only rotation.
	OperatorSessionID string

	// OperatorID is the operator document ID, when applicable. Empty for
	// CLI-only rotation.
	OperatorID string

	// CLICertPEM is the signed CLI certificate PEM. Required.
	CLICertPEM string

	// CLICertChainPEM is the CLI certificate chain PEM (intermediates),
	// when provided by the gateway.
	CLICertChainPEM string

	// CLIKey is the CLI private key. The coordinator owns this in-memory
	// only for the duration of staging; it is never logged. Nil for remote
	// operator/device enrollment where the CLI key was generated elsewhere.
	CLIKey *ecdsa.PrivateKey

	// TrustBundlePEM is the full gateway runtime trust bundle PEM. Used by
	// CredentialStore to refresh the local runtime bundle and by
	// ExtractRootAnchors/InstallRoot to install root anchors for OS trust.
	TrustBundlePEM string

	// OperatorCertPEM is the operator certificate PEM, when the enrollment
	// path produced one (remote operator/device enrollment). Empty for
	// local CLI enrollment.
	OperatorCertPEM string

	// OperatorCertChainPEM is the operator certificate chain PEM, when
	// applicable.
	OperatorCertChainPEM string

	// OperatorKeyPEM is the operator private key PEM, when applicable and
	// when the caller generated it locally. Empty for remote enrollment
	// where the operator key was generated elsewhere.
	OperatorKeyPEM string
}

// EnrollmentSource identifies which gateway enrollment operation produced
// an EnrollmentArtifacts value. Used for logging, validation, and to drive
// commit-time decisions (e.g., remote operator enrollment does not install
// OS trust or register a passkey).
type EnrollmentSource int

const (
	// EnrollmentSourceBootstrap is the initial unbootstrapped-gateway path
	// (POST /api/v1/auth/bootstrap).
	EnrollmentSourceBootstrap EnrollmentSource = iota

	// EnrollmentSourceRecovery is the human-approved CLI recovery path
	// (POST /api/v1/auth/cli/recovery/complete).
	EnrollmentSourceRecovery

	// EnrollmentSourceRotation is the mTLS CLI rotation path
	// (POST /api/v1/auth/cli/rotate).
	EnrollmentSourceRotation

	// EnrollmentSourceRemoteOperator is the remote operator/device
	// enrollment path (POST /api/v1/auth/device/enroll), used by
	// `security pki enroll`. It is NOT a local human enrollment.
	EnrollmentSourceRemoteOperator
)

// String returns a human-readable label for the enrollment source.
func (s EnrollmentSource) String() string {
	switch s {
	case EnrollmentSourceBootstrap:
		return "bootstrap"
	case EnrollmentSourceRecovery:
		return "recovery"
	case EnrollmentSourceRotation:
		return "rotation"
	case EnrollmentSourceRemoteOperator:
		return "remote-operator"
	default:
		return "unknown"
	}
}

// IsLocalCLI reports whether the source is a local human CLI enrollment
// path (bootstrap, recovery, or rotation). Remote operator enrollment is
// not a local CLI path and must not trigger OS trust installation or
// passkey registration.
func (s EnrollmentSource) IsLocalCLI() bool {
	switch s {
	case EnrollmentSourceBootstrap, EnrollmentSourceRecovery, EnrollmentSourceRotation:
		return true
	default:
		return false
	}
}

// EnrollmentOptions is the input to EnrollmentCoordinator.Enroll. It is
// constructed by the command layer (auth.go, demos.go, mcp.go) and carries
// only options — dependencies are injected on the coordinator itself.
type EnrollmentOptions struct {
	// NoSystemTrust skips the OS trust installer after printing an
	// administrator notice. The passkey ceremony still runs and runtime
	// mTLS/trust-bundle errors still fail the enrollment. Per §6.5.
	NoSystemTrust bool

	// RotateCLI forces an mTLS CLI rotation even when the local identity is
	// complete and not expiring. Idempotent at the coordinator level: at
	// most one rotation per run.
	RotateCLI bool

	// CAFingerprint, when non-empty, pins the expected SHA-256 fingerprint
	// of the gateway root CA. The coordinator verifies any received trust
	// bundle against it before committing.
	CAFingerprint string

	// PasskeyTimeout, when zero, uses the registrar's default. Allows
	// callers (e.g., demos) to shorten or extend the browser ceremony
	// wait.
	PasskeyTimeout time.Duration

	// SkipPasskey suppresses the passkey ceremony. This is NOT exposed as a
	// CLI flag (per the fixed decision "no --skip-passkey"); it exists for
	// internal callers that already hold a valid passkey (e.g., mcp agent
	// run's ensure path when a passkey already exists). The command layer
	// must never set this from a user flag.
	SkipPasskey bool
}
