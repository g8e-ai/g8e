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

package platform

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// StaleAnchor describes a g8e root CA found in the OS trust store that does
// not match the current gateway's root CA fingerprint. These are orphaned
// anchors left behind by a previous gateway instance (e.g., after `gw clean`
// regenerated the CA). The coordinator prompts the user before removing them.
type StaleAnchor struct {
	// Fingerprint is the hex-encoded SHA-256 of the stale anchor's DER.
	Fingerprint string

	// CommonName is the certificate's subject CN, for display.
	CommonName string

	// Handle is the platform-specific identifier used for removal:
	//   - Linux (Debian/RHEL): the managed file path
	//   - Windows: the SHA-1 thumbprint (certutil -delstore)
	//   - Darwin: the SHA-1 hash (security delete-certificate -Z)
	Handle string
}

// g8eRootCommonName is the subject CN used by all gateway root CAs. It is
// the cross-platform filter for identifying g8e-managed anchors in trust
// stores that do not use a filename prefix (Windows, Darwin).
const g8eRootCommonName = "g8e Root CA"

// commandRunner executes an external command with context, returning combined
// stdout/stderr. It is the injection point for unit tests so they never invoke
// sudo, security, certutil, or other real system commands. The optional env
// map is merged onto os.Environ() for the child process; entries with empty
// values unset the variable.
type commandRunner interface {
	Run(ctx context.Context, env map[string]string, name string, args ...string) ([]byte, error)
}

// execRunner is the production commandRunner backed by os/exec.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		base := os.Environ()
		merged := make([]string, 0, len(base)+len(env))
		merged = append(merged, base...)
		for k, v := range env {
			merged = append(merged, k+"="+v)
		}
		cmd.Env = merged
	}
	return cmd.CombinedOutput()
}

// SystemTrustInstaller installs a gateway root CA anchor into the host OS
// trust store and manages stale g8e anchors from previous gateway instances.
// The installer exposes single-purpose methods: IsTrusted reads the trust
// store, InstallRoot writes to it, and ListStaleAnchors/RemoveStaleAnchors
// enumerate and remove orphaned anchors. Bundle parsing, root extraction, and
// chain verification (ExtractRootAnchors, verifyRootUsable) are package-level
// helpers — the coordinator calls them directly to obtain the root +
// fingerprint, then calls IsTrusted/InstallRoot/ListStaleAnchors/
// RemoveStaleAnchors as needed.
type SystemTrustInstaller struct {
	runner commandRunner
	// now is injectable for expiry checks in tests.
	now func() time.Time
}

// NewSystemTrustInstaller returns an installer using real os/exec for command
// execution and the system clock for time checks.
func NewSystemTrustInstaller() *SystemTrustInstaller {
	return &SystemTrustInstaller{
		runner: execRunner{},
		now:    time.Now,
	}
}

// IsTrusted reports whether the OS trust store already contains a root anchor
// with the given SHA-256 fingerprint. It reads only — it never installs,
// removes, or prompts. The platform-specific implementations ignore the root
// certificate argument and compare fingerprints only; the parameter is kept
// for future per-cert inspection without changing the call site.
func (i *SystemTrustInstaller) IsTrusted(ctx context.Context, fingerprint string) (bool, error) {
	return i.isTrustedPlatform(ctx, nil, fingerprint)
}

// InstallRoot writes the root anchor into the OS trust store. It writes only —
// it does not check whether the anchor is already trusted (the caller decides
// via IsTrusted first). Requires elevation on all platforms.
func (i *SystemTrustInstaller) InstallRoot(ctx context.Context, root *x509.Certificate, fingerprint string) error {
	return i.installPlatform(ctx, root, fingerprint)
}

// ListStaleAnchors enumerates g8e root CA anchors in the OS trust store that
// do not match currentFingerprint. These are orphaned anchors from previous
// gateway instances. Returns an empty slice (not nil) when no stale anchors
// are found. The current fingerprint is excluded so the active gateway's root
// is never listed as stale.
func (i *SystemTrustInstaller) ListStaleAnchors(ctx context.Context, currentFingerprint string) ([]StaleAnchor, error) {
	return i.listStaleAnchorsPlatform(ctx, currentFingerprint)
}

// RemoveStaleAnchors removes the given stale anchors from the OS trust store
// and refreshes the trust bundle. Requires elevation on all platforms. An
// error from any anchor removal aborts the operation.
func (i *SystemTrustInstaller) RemoveStaleAnchors(ctx context.Context, anchors []StaleAnchor) error {
	return i.removeStaleAnchorsPlatform(ctx, anchors)
}

// ExtractRootAnchors parses every PEM certificate in bundlePEM, rejects
// malformed or expired input, and returns the self-signed CA certificates
// that serve as trust anchors. Exported so the enrollment trust-bundle
// helper can derive OS-trust root anchors from the runtime bundle without
// duplicating the parsing/validation logic.
func ExtractRootAnchors(bundlePEM []byte, now func() time.Time) ([]*x509.Certificate, error) {
	return extractRootAnchors(bundlePEM, now)
}

// extractRootAnchors parses every PEM certificate in bundlePEM, rejects
// malformed or expired input, and returns the self-signed CA certificates
// that serve as trust anchors.
func extractRootAnchors(bundlePEM []byte, now func() time.Time) ([]*x509.Certificate, error) {
	certs, err := parseBundleCerts(bundlePEM)
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, constants.ErrEmptyTrustBundle
	}

	currentTime := now()
	var roots []*x509.Certificate
	for _, cert := range certs {
		if currentTime.Before(cert.NotBefore) || currentTime.After(cert.NotAfter) {
			return nil, fmt.Errorf("%w: certificate %q is not valid at this time", constants.ErrSystemTrustInvalidAnchor, cert.Subject.CommonName)
		}
		if !cert.IsCA || !cert.BasicConstraintsValid {
			continue
		}
		if cert.CheckSignatureFrom(cert) != nil {
			continue
		}
		roots = append(roots, cert)
	}
	return roots, nil
}

// VerifyRootUsable confirms that at least one non-root certificate in the
// bundle chains to one of the provided root anchors. This prevents installing
// an unrelated or stale root as a trust anchor. Exported so the enrollment
// coordinator can run the chain check before calling IsTrusted/InstallRoot.
func VerifyRootUsable(roots []*x509.Certificate, bundlePEM []byte, now func() time.Time) error {
	return verifyRootUsable(roots, bundlePEM, now)
}

// verifyRootUsable confirms that at least one non-root certificate in the
// bundle chains to one of the provided root anchors. This prevents installing
// an unrelated or stale root as a trust anchor.
func verifyRootUsable(roots []*x509.Certificate, bundlePEM []byte, now func() time.Time) error {
	allCerts, err := parseBundleCerts(bundlePEM)
	if err != nil {
		return err
	}

	rootPool := x509.NewCertPool()
	for _, r := range roots {
		rootPool.AddCert(r)
	}

	intermediatePool := x509.NewCertPool()
	for _, c := range allCerts {
		if c.IsCA && c.CheckSignatureFrom(c) != nil {
			intermediatePool.AddCert(c)
		}
	}

	opts := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: intermediatePool,
		CurrentTime:   now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	for _, c := range allCerts {
		if c.CheckSignatureFrom(c) == nil {
			continue // skip root anchors themselves
		}
		if _, err := c.Verify(opts); err == nil {
			return nil
		}
	}
	return constants.ErrSystemTrustNoChainToAnchor
}

// ParseBundleCerts decodes all CERTIFICATE PEM blocks from data and parses
// each into an x509.Certificate. Exported for reuse by the enrollment
// trust-bundle helper.
func ParseBundleCerts(data []byte) ([]*x509.Certificate, error) {
	return parseBundleCerts(data)
}

// parseBundleCerts decodes all CERTIFICATE PEM blocks from data and parses
// each into an x509.Certificate.
func parseBundleCerts(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := data
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrCAParseFailed, err)
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

// CertFingerprint returns the hex-encoded SHA-256 hash of the certificate's
// DER encoding. Exported for reuse by the enrollment trust-bundle helper.
func CertFingerprint(cert *x509.Certificate) string {
	return certFingerprint(cert)
}

// certFingerprint returns the hex-encoded SHA-256 hash of the certificate's
// DER encoding.
func certFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// bundleContainsFingerprint parses PEM certificates from data and returns true
// if any certificate's SHA-256 fingerprint matches.
func bundleContainsFingerprint(data []byte, fingerprint string) (bool, error) {
	certs, err := parseBundleCerts(data)
	if err != nil {
		return false, nil
	}
	for _, c := range certs {
		if certFingerprint(c) == fingerprint {
			return true, nil
		}
	}
	return false, nil
}

// writeTempCert writes certPEM to a restrictive temporary directory and returns
// the file path. The caller must clean up the returned directory via
// os.RemoveAll(dir).
func writeTempCert(certPEM []byte) (dir, path string, err error) {
	dir, err = os.MkdirTemp("", "g8e-systrust-*")
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
	}
	path = filepath.Join(dir, "root.pem")
	if err = os.WriteFile(path, certPEM, constants.PermFilePrivate); err != nil {
		os.RemoveAll(dir)
		return "", "", fmt.Errorf("%w: %w", constants.ErrSystemTrustInstallFailed, err)
	}
	return dir, path, nil
}
