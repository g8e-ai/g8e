// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// SystemFingerprint represents a unique, stable identifier for the system or specific operator instance.
type SystemFingerprint struct {
	Fingerprint  string                  `json:"fingerprint"`
	OS           string                  `json:"os"`
	Architecture string                  `json:"architecture"`
	CPUCount     int                     `json:"cpu_count"`
	MachineID    string                  `json:"machine_id,omitempty"`
	LocalDir     string                  `json:"local_dir,omitempty"`
	Account      string                  `json:"account,omitempty"`
	Port         int                     `json:"port,omitempty"`
	Roles        constants.OperatorRoles `json:"roles,omitempty"`
}

// FingerprintOptions specifies operator-specific parameters that differentiate
// multiple operators coexisting on the same host system.
type FingerprintOptions struct {
	LocalDir string
	Account  string
	Port     int
	Roles    constants.OperatorRoles
}

// ResolveCurrentAccount returns the current operating system user account username or UID.
func ResolveCurrentAccount() string {
	if u, err := user.Current(); err == nil && u != nil {
		if u.Username != "" {
			return u.Username
		}
		if u.Uid != "" {
			return u.Uid
		}
	}
	return os.Getenv(string(constants.EnvVar.User))
}

// GenerateSystemFingerprint creates a unique fingerprint based on immutable system properties.
// Calling this preserves the canonical 5-property system hash when no operator options are set.
func GenerateSystemFingerprint(logger *slog.Logger) (*SystemFingerprint, error) {
	return GenerateOperatorFingerprint(logger, FingerprintOptions{})
}

// GenerateOperatorFingerprint creates a unique fingerprint incorporating system properties,
// local directory, launching account, port, and operator role. This guarantees that
// multiple operators running on the same host for unique purposes are completely separated.
func GenerateOperatorFingerprint(logger *slog.Logger, opts FingerprintOptions) (*SystemFingerprint, error) {
	if err := opts.Roles.Validate(); err != nil {
		return nil, err
	}
	opts.Roles = opts.Roles.Canonical()

	logger.Info("Generating system fingerprint based on immutable system and operator properties...")

	osType := runtime.GOOS
	arch := runtime.GOARCH
	cpuCount := runtime.NumCPU()

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrFingerprintGetHostname, err)
	}

	machineID, err := getMachineID(logger)
	if err != nil {
		logger.Warn("Failed to get machine ID, using fallback method", "error", err)
		machineID = "fallback"
	}

	components := []string{
		fmt.Sprintf("os:%s", osType),
		fmt.Sprintf("arch:%s", arch),
		fmt.Sprintf("cpu_count:%d", cpuCount),
		fmt.Sprintf("machine_id:%s", machineID),
		fmt.Sprintf("hostname:%s", hostname),
	}

	cleanDir := ""
	if opts.LocalDir != "" {
		cleanDir = filepath.Clean(opts.LocalDir)
		components = append(components, fmt.Sprintf("local_dir:%s", cleanDir))
	}

	account := opts.Account
	if account != "" {
		components = append(components, fmt.Sprintf("account:%s", account))
	}

	if opts.Port > 0 {
		components = append(components, fmt.Sprintf("port:%d", opts.Port))
	}

	if len(opts.Roles) > 0 {
		components = append(components, fmt.Sprintf("role:%s", opts.Roles))
	}

	hasher := sha256.New()
	fingerprintInput := strings.Join(components, "|")
	hasher.Write([]byte(fingerprintInput))
	fingerprintHash := hex.EncodeToString(hasher.Sum(nil))

	fingerprint := &SystemFingerprint{
		Fingerprint:  fingerprintHash,
		OS:           osType,
		Architecture: arch,
		CPUCount:     cpuCount,
		MachineID:    machineID,
		LocalDir:     cleanDir,
		Account:      account,
		Port:         opts.Port,
		Roles:        opts.Roles,
	}

	logger.Info("System fingerprint generated successfully",
		"os", fingerprint.OS,
		"architecture", fingerprint.Architecture,
		"cpu_count", fingerprint.CPUCount,
		"machine_id", machineID,
		"hostname", hostname,
		"local_dir", cleanDir,
		"account", account,
		"port", opts.Port,
		"role", opts.Roles,
		"fingerprint", fingerprintHash[:16])

	return fingerprint, nil
}

// getMachineID retrieves a stable machine identifier based on the OS
func getMachineID(logger *slog.Logger) (string, error) {
	return getMachineIDWithPlatform(logger, runtime.GOOS)
}

// getMachineIDWithPlatform dispatches to the platform-specific machine ID
// implementation. Extracted from getMachineID for testability of the
// unsupported OS branch.
func getMachineIDWithPlatform(logger *slog.Logger, platform string) (string, error) {
	switch constants.Platform(platform) {
	case constants.PlatformLinux:
		return getLinuxMachineID(logger)
	case constants.PlatformDarwin:
		return getDarwinMachineID()
	case constants.PlatformWindows:
		return getWindowsMachineID(logger)
	default:
		return "", fmt.Errorf("%w: %s", constants.ErrFingerprintUnsupportedOS, platform)
	}
}

// getLinuxMachineID reads a stable machine identifier from the kernel.
// For bare metal/VMs, tries persistent identity files first (/etc/machine-id, /var/lib/dbus/machine-id),
// then falls back to /proc/sys/kernel/random/boot_id.
func getLinuxMachineID(logger *slog.Logger) (string, error) {
	// Try persistent identity files
	paths := []string{
		constants.PathEtcMachineID,
		constants.PathVarLibDbusMachineID,
		constants.PathProcSysKernelRandomBootID,
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			machineID := strings.TrimSpace(string(data))
			if machineID != "" {
				logger.Info("Retrieved Linux machine ID", "source", path)
				return machineID, nil
			}
		}
	}

	return "", constants.ErrFingerprintMachineIDRead
}

// getDarwinMachineID uses the system preferences plist as a stable machine identifier on macOS
func getDarwinMachineID() (string, error) {
	data, err := os.ReadFile(constants.PathLibraryPreferencesSystemConfigurationPreferencesPlist)
	if err != nil {
		hostname, err := os.Hostname()
		if err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrFingerprintGetHostname, err)
		}
		return fmt.Sprintf("darwin-%s", hostname), nil
	}

	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))[:32], nil
}

// CSRFingerprint computes the SHA-256 fingerprint of the public key in
// a CSR PEM, matching the gateway's parsePlatformEnrollmentCSR: it
// hashes the SubjectPublicKeyInfo DER bytes and returns hex.
func CSRFingerprint(csrPEM string) (string, error) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", fmt.Errorf("csr fingerprint: %w", constants.ErrPlatformEnrollmentInvalidCSR)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("csr fingerprint: parse: %w", constants.ErrPlatformEnrollmentInvalidCSR)
	}
	if err := csr.CheckSignature(); err != nil {
		return "", fmt.Errorf("csr fingerprint: verify signature: %w", constants.ErrPlatformEnrollmentInvalidCSR)
	}
	publicKey, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return "", constants.ErrPlatformEnrollmentUnsupportedKey
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("csr fingerprint: marshal public key: %w", err)
	}
	digest := sha256.Sum256(publicDER)
	return hex.EncodeToString(digest[:]), nil
}

// CertificateFingerprint computes the SHA-256 fingerprint of a PEM-encoded certificate.
// Returns an empty string on decode or parse failure.
func CertificateFingerprint(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// CertificateSerialNumber extracts the serial number from a PEM-encoded certificate.
// Returns an empty string on decode or parse failure.
func CertificateSerialNumber(certPEM string) string {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	return cert.SerialNumber.String()
}
