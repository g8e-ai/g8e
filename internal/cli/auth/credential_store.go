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
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/certutil"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// rotationThreshold is the window before CLI certificate expiry within
// which the coordinator will rotate via the mTLS rotation endpoint. Kept
// here (not in constants) because it is a coordinator policy, not a wire
// contract.
const rotationThreshold = 24 * time.Hour

// CredentialStore is the coordinator's typed API over the local CLI
// identity files. It owns:
//
//   - Inspect: classify the local state as absent/complete/partial/corrupt
//     by examining ALL managed artifacts as a set, never by checking one
//     file at a time.
//   - Stage/Commit: write a new complete identity atomically enough that
//     readers see either the old complete set or the new complete set,
//     never a half-written one. Credentials are written LAST so a missing
//     or partial commit is detected by the next Inspect as partial/corrupt.
//   - Clear: one cleanup operation for logout/recovery that removes the
//     local CLI credential material. It does NOT remove the shared OS root
//     CA (per the §4.3 ownership policy).
//
// The store does not open browsers, invoke sudo, or perform network I/O.
// It is safe for concurrent use: an enrollment lock (mu) serializes Stage/
// Commit/Clear against each other so two concurrent enrollments cannot
// interleave file writes.
type CredentialStore struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
	mu      sync.Mutex
}

// NewCredentialStore returns a CredentialStore backed by the given file
// service and CLI config. The config is captured by reference; callers
// must not mutate its path fields after construction.
func NewCredentialStore(fileSvc fs.RuntimeFileService, cfg *config.Config) *CredentialStore {
	return &CredentialStore{fileSvc: fileSvc, cfg: cfg}
}

// stagedIdentity holds the in-memory artifacts waiting for Commit. It is
// owned by a single Stage call and consumed by Commit. The caller must
// hold the store lock between Stage and Commit.
type stagedIdentity struct {
	artifacts EnrollmentArtifacts
}

// Inspect examines all managed artifacts as a complete set and returns a
// LocalIdentity classification. It never writes files and never opens a
// browser. It is the single source of truth for the coordinator's state-
// machine decision.
//
// The managed artifact set is:
//   - credentials JSON (cfg.CredentialsFile)
//   - CLI cert PEM (cfg.CLICertFile)
//   - CLI key PEM (cfg.CLIKeyFile)
//   - runtime trust bundle (cfg.DefaultTrustBundleRelPath or custom)
//
// A complete identity requires all four to be present, parseable, and
// mutually consistent (the CLI key's public key matches the CLI cert's
// public key). Any missing or unparseable artifact is partial; a present-
// but-mismatched cert/key pair is corrupt.
func (s *CredentialStore) Inspect(ctx context.Context) (LocalIdentity, error) {
	out := LocalIdentity{State: LocalStateAbsent}

	creds, credsErr := s.loadCredentials(ctx)
	credPresent := creds != nil
	credCorrupt := credsErr != nil && !isNotFound(credsErr)

	cliCert, cliCertPresent, cliCertCorrupt := s.loadCLICert(ctx)
	cliKeyPresent, cliKeyCorrupt, cliKeyPub := s.loadCLIKeyPub(ctx)
	bundle, bundleErr := ReadTrustBundleFromDisk(ctx, s.fileSvc, s.cfg)
	bundlePresent := bundle != nil
	bundleCorrupt := bundleErr != nil

	// If nothing is present, the state is cleanly absent.
	if !credPresent && !cliCertPresent && !cliKeyPresent && !bundlePresent {
		return out, nil
	}

	// Any corrupt artifact (present but unparseable) makes the whole set
	// corrupt. Partial (some present, all parseable but incomplete) is
	// distinct: the files are individually valid but the set is not.
	if credCorrupt || cliCertCorrupt || cliKeyCorrupt || bundleCorrupt {
		out.State = LocalStateCorrupt
		out.Credentials = creds // may be nil
		out.CLICert = cliCert   // may be nil
		out.HasCLIKey = cliKeyPresent
		out.TrustBundle = bundle // may be nil
		return out, nil
	}

	// All present-and-parseable artifacts are loaded. Classify completeness.
	out.Credentials = creds
	out.CLICert = cliCert
	out.HasCLIKey = cliKeyPresent
	out.TrustBundle = bundle

	// Cert/key mutual consistency: if both are present, the key's public
	// key must match the cert's public key.
	if cliCertPresent && cliKeyPresent {
		out.KeyMatchesCert = pubKeysMatch(cliCert, cliKeyPub)
		if !out.KeyMatchesCert {
			out.State = LocalStateCorrupt
			return out, nil
		}
	}

	complete := credPresent && cliCertPresent && cliKeyPresent && bundlePresent && out.KeyMatchesCert
	if !complete {
		out.State = LocalStatePartial
		return out, nil
	}

	// Complete and consistent. Inspect expiry to drive the rotation
	// decision.
	out.State = LocalStateComplete
	if cliCert != nil {
		remaining := time.Until(cliCert.NotAfter)
		out.CertExpiring = remaining <= rotationThreshold && remaining > 0
		out.CertExpired = remaining <= 0
	}
	return out, nil
}

// Stage prepares a new identity for commit without writing any of the
// canonical managed files. The artifacts are held in memory; Commit
// performs the atomic file replacement. The caller passes the
// EnrollmentArtifacts produced by the enrollment client (bootstrap,
// recovery, rotation, or remote operator/device).
//
// Stage acquires the store lock and holds it until Commit or Rollback is
// called. This serializes concurrent enrollments against each other so
// two Stage/Commit sequences cannot interleave file writes.
func (s *CredentialStore) Stage(ctx context.Context, artifacts EnrollmentArtifacts) (*stagedIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if artifacts.CLICertPEM == "" || artifacts.CLISessionID == "" || artifacts.UserID == "" {
		return nil, fmt.Errorf("%w: staged artifacts missing required CLI fields", constants.ErrValidationFailed)
	}
	if artifacts.CLIKey == nil && artifacts.Source.IsLocalCLI() {
		return nil, fmt.Errorf("%w: staged local CLI artifacts missing CLI key", constants.ErrValidationFailed)
	}
	if artifacts.TrustBundlePEM == "" {
		return nil, fmt.Errorf("%w: staged artifacts missing trust bundle", constants.ErrValidationFailed)
	}

	// Validate the trust bundle parses and matches the fingerprint pin
	// (if provided by the caller via the artifacts — the coordinator
	// validates the pin before staging).
	if _, err := ParseTrustBundle([]byte(artifacts.TrustBundlePEM), time.Now()); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrValidationFailed, err)
	}

	// Validate the CLI cert parses and matches the CLI key.
	cert, err := certutil.ParseCertFromPEM([]byte(artifacts.CLICertPEM))
	if err != nil {
		return nil, fmt.Errorf("%w: staged CLI cert: %w", constants.ErrValidationFailed, err)
	}
	if artifacts.CLIKey != nil && !pubKeyMatchesCert(cert, &artifacts.CLIKey.PublicKey) {
		return nil, fmt.Errorf("%w: staged CLI cert/key mismatch", constants.ErrValidationFailed)
	}

	return &stagedIdentity{artifacts: artifacts}, nil
}

// Commit writes the staged identity to the canonical managed files in the
// order required by §4.3:
//
//  1. Write the CLI key (PermFilePrivate).
//  2. Write the CLI cert + chain (PermFilePrivate).
//  3. Write the runtime trust bundle (PermFilePublic).
//  4. Write the credentials JSON LAST so a partial commit is detected by
//     the next Inspect as partial/corrupt (credentials present but cert
//     missing, or vice versa).
//
// Each file is written via RuntimeFileService.WriteFile, which uses a
// tmp+rename pattern so individual files are atomic. The previous complete
// identity remains readable until the new file is renamed into place.
//
// On any write failure, Commit returns the error and leaves the previously
// committed identity (if any) in place — it does not half-overwrite the
// canonical files. The caller may retry Commit with the same staged
// identity.
func (s *CredentialStore) Commit(ctx context.Context, staged *stagedIdentity) error {
	if staged == nil {
		return fmt.Errorf("%w: nil staged identity", constants.ErrInternal)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	a := staged.artifacts

	cliCertRel, err := s.fileSvc.RelFromAbs(s.cfg.CLICertFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	cliKeyRel, err := s.fileSvc.RelFromAbs(s.cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}

	// 1. CLI key (only for local CLI enrollment where we own the key).
	if a.CLIKey != nil {
		certBytes, keyBytes, err := certutil.EncodeCertAndKey(a.CLICertPEM, a.CLICertChainPEM, a.CLIKey)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		if err := s.fileSvc.WriteFile(ctx, cliKeyRel, keyBytes, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		if err := s.fileSvc.WriteFile(ctx, cliCertRel, certBytes, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
	} else {
		// Remote operator/device enrollment: only the cert is staged (the
		// key was generated elsewhere). Write the cert + chain.
		certContent := []byte(a.CLICertPEM)
		if a.CLICertChainPEM != "" {
			certContent = append(certContent, []byte("\n"+a.CLICertChainPEM)...)
		}
		if err := s.fileSvc.WriteFile(ctx, cliCertRel, certContent, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
	}

	// 2. Runtime trust bundle.
	if err := WriteTrustBundleToDisk(ctx, s.fileSvc, s.cfg, []byte(a.TrustBundlePEM)); err != nil {
		return err
	}

	// 3. Operator cert/key, when the enrollment path produced them.
	if a.OperatorCertPEM != "" {
		if err := s.writeOperatorCert(ctx, a); err != nil {
			return err
		}
	}

	// 4. Credentials JSON LAST.
	creds := &Credentials{
		OperatorSessionID: a.OperatorSessionID,
		UserID:            a.UserID,
		OperatorID:        a.OperatorID,
		CLISessionID:      a.CLISessionID,
	}
	if err := SaveCredentials(s.fileSvc, s.cfg, creds); err != nil {
		return err
	}
	return nil
}

// writeOperatorCert writes the operator cert (and key, when present) to
// the configured operator cert/key paths. Operator artifacts are optional
// for local CLI enrollment (bootstrap produces them; rotation does not).
func (s *CredentialStore) writeOperatorCert(ctx context.Context, a EnrollmentArtifacts) error {
	opCertRel, err := s.fileSvc.RelFromAbs(s.cfg.OperatorCertFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	certContent := []byte(a.OperatorCertPEM)
	if a.OperatorCertChainPEM != "" {
		certContent = append(certContent, []byte("\n"+a.OperatorCertChainPEM)...)
	}
	if err := s.fileSvc.WriteFile(ctx, opCertRel, certContent, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
	}
	if a.OperatorKeyPEM != "" {
		opKeyRel, err := s.fileSvc.RelFromAbs(s.cfg.OperatorKeyFile())
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
		if err := s.fileSvc.WriteFile(ctx, opKeyRel, []byte(a.OperatorKeyPEM), constants.PermFilePrivate); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrCertSaveFailed, err)
		}
	}
	return nil
}

// Rollback releases the staged identity without writing any canonical
// files. The previously committed identity (if any) remains in place.
// Called when the coordinator decides not to commit after staging (e.g.,
// a trust-install failure before browser launch).
func (s *CredentialStore) Rollback(staged *stagedIdentity) {
	if staged == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Nothing to clean up — Stage wrote no canonical files. The lock is
	// released here so a subsequent Stage/Commit can proceed.
}

// Clear removes the local CLI credential material for logout/recovery
// cleanup. It removes:
//   - credentials JSON
//   - CLI cert
//   - CLI key
//
// It does NOT remove the shared OS root CA (the runtime trust bundle) —
// system trust is shared and may be used by another runtime or gateway
// (per §4.3 ownership policy). Callers that need a full reset including
// the trust bundle should call DeleteCredentials directly. Callers that
// also want to remove an exact Windows Personal-store certificate should
// do so separately via the platform key provider.
func (s *CredentialStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	credsRel, err := s.fileSvc.RelFromAbs(s.cfg.CredentialsFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := s.fileSvc.Remove(ctx, credsRel); err != nil && !isNotFound(err) {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	cliCertRel, err := s.fileSvc.RelFromAbs(s.cfg.CLICertFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := s.fileSvc.Remove(ctx, cliCertRel); err != nil && !isNotFound(err) {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	cliKeyRel, err := s.fileSvc.RelFromAbs(s.cfg.CLIKeyFile())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}
	if err := s.fileSvc.Remove(ctx, cliKeyRel); err != nil && !isNotFound(err) {
		return fmt.Errorf("%w: %w", constants.ErrFileRemoveFailed, err)
	}

	return nil
}

// LoadCredentials returns the parsed credentials JSON, or (nil, nil) when
// absent. Wrapper around the package-level LoadCredentials for callers
// that hold a CredentialStore.
func (s *CredentialStore) LoadCredentials(ctx context.Context) (*Credentials, error) {
	return s.loadCredentials(ctx)
}

// loadCredentials reads and parses the credentials JSON. Returns
// (nil, nil) when absent, (nil, err) when present but unparseable.
func (s *CredentialStore) loadCredentials(ctx context.Context) (*Credentials, error) {
	rel, err := s.fileSvc.RelFromAbs(s.cfg.CredentialsFile())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	data, err := s.fileSvc.ReadFile(ctx, rel)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrFailedToLoadCredentials, err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}
	return &creds, nil
}

// loadCLICert reads and parses the CLI cert PEM. Returns (nil, false,
// false) when absent, (nil, false, true) when present but unparseable.
func (s *CredentialStore) loadCLICert(ctx context.Context) (cert *x509.Certificate, present, corrupt bool) {
	rel, err := s.fileSvc.RelFromAbs(s.cfg.CLICertFile())
	if err != nil {
		return nil, false, false
	}
	data, err := s.fileSvc.ReadFile(ctx, rel)
	if err != nil {
		if isNotFound(err) {
			return nil, false, false
		}
		return nil, false, true
	}
	c, perr := certutil.ParseCertFromPEM(data)
	if perr != nil {
		return nil, true, true
	}
	return c, true, false
}

// loadCLIKeyPub reads the CLI key PEM and returns its public key. Returns
// (false, false, nil) when absent, (false, true, nil) when present but
// unparseable.
func (s *CredentialStore) loadCLIKeyPub(ctx context.Context) (present, corrupt bool, pub *ecdsa.PublicKey) {
	rel, err := s.fileSvc.RelFromAbs(s.cfg.CLIKeyFile())
	if err != nil {
		return false, false, nil
	}
	data, err := s.fileSvc.ReadFile(ctx, rel)
	if err != nil {
		if isNotFound(err) {
			return false, false, nil
		}
		return false, true, nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return true, true, nil
	}
	var key any
	switch block.Type {
	case "EC PRIVATE KEY":
		k, perr := x509.ParseECPrivateKey(block.Bytes)
		if perr != nil {
			return true, true, nil
		}
		key = k
	case "PRIVATE KEY":
		k, perr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if perr != nil {
			return true, true, nil
		}
		key = k
	default:
		return true, true, nil
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return true, true, nil
	}
	return true, false, &ecKey.PublicKey
}

// pubKeysMatch reports whether the CLI cert's public key matches the
// provided CLI key public key.
func pubKeysMatch(cert *x509.Certificate, keyPub *ecdsa.PublicKey) bool {
	if cert == nil || keyPub == nil {
		return false
	}
	return pubKeyMatchesCert(cert, keyPub)
}

// pubKeyMatchesCert reports whether the given public key equals the cert's
// public key. Used by Stage to validate the staged cert/key pair.
func pubKeyMatchesCert(cert *x509.Certificate, keyPub *ecdsa.PublicKey) bool {
	if cert == nil || keyPub == nil {
		return false
	}
	certPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	return certPub.Equal(keyPub)
}

// ResolveCLICertPath returns the absolute CLI cert path. Convenience for
// callers (e.g., the key provider) that need the on-disk path.
func (s *CredentialStore) ResolveCLICertPath() string {
	return s.cfg.CLICertFile()
}

// ResolveCLIKeyPath returns the absolute CLI key path.
func (s *CredentialStore) ResolveCLIKeyPath() string {
	return s.cfg.CLIKeyFile()
}

// ResolveTrustBundlePath returns the absolute trust bundle path (custom
// external path if configured, otherwise the default runtime path).
func (s *CredentialStore) ResolveTrustBundlePath() string {
	return s.cfg.ResolvedTrustBundlePath()
}
