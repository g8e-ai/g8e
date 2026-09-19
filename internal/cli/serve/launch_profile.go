// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// LaunchProfileVersion is the schema version for the persisted launch
// profile. Read validation rejects any other version so that an older
// profile never silently restarts with an incompatible schema.
const LaunchProfileVersion = 1

// GatewayLaunchProfile is the versioned, typed configuration persisted
// after every successful managed background `gw start`. `gw restart`
// reads and validates the complete profile to reconstruct the prior
// launch configuration rather than falling back to posture-only
// defaults.
//
// The Config field stores the full serve.GatewayConfig as constructed
// from CLI flags, environment overrides, and identity detection. The
// ephemeral NetworkIdentityFile field is cleared before persistence
// because the subprocess re-detects network identity on every start;
// persisting a stale path would point at a file that no longer exists.
type GatewayLaunchProfile struct {
	Version int           `json:"version"`
	Config  GatewayConfig `json:"config"`
}

// launchProfileRelPath returns the runtime-relative path to the launch
// profile file, constructed from path constants. No filepath string
// literals.
func launchProfileRelPath() string {
	return filepath.Join(constants.PidDirname, constants.OperatorLaunchProfileFilename)
}

// WriteLaunchProfile serializes the given gateway config as a versioned
// launch profile with private permissions through RuntimeFileService.
// The NetworkIdentityFile field is cleared before serialization because
// it is ephemeral — the subprocess re-detects network identity on every
// start. Call this only after StartOperator succeeds so the profile
// always reflects a known-good launch.
func WriteLaunchProfile(fileSvc fs.RuntimeFileService, cfg GatewayConfig) error {
	profile := GatewayLaunchProfile{
		Version: LaunchProfileVersion,
		Config:  cfg,
	}
	// Clear ephemeral network identity state before persistence. The
	// subprocess re-detects identity on restart; a stale file path
	// would point at a file that no longer exists.
	profile.Config.NetworkIdentityFile = ""
	if err := ValidateLaunchProfile(profile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: marshal: %w", constants.ErrLaunchProfileInvalid, err)
	}

	if err := fileSvc.WriteFile(context.Background(), launchProfileRelPath(), data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("%w: write: %w", constants.ErrLaunchProfileInvalid, err)
	}
	return nil
}

// ReadLaunchProfile reads, validates, and returns the persisted launch
// profile. Missing, corrupted, unsupported-version, or invalid profiles
// fail closed with typed errors; restart never falls back to defaults.
func ReadLaunchProfile(fileSvc fs.RuntimeFileService) (GatewayLaunchProfile, error) {
	data, err := fileSvc.ReadFile(context.Background(), launchProfileRelPath())
	if err != nil {
		if isNotFound(err) {
			return GatewayLaunchProfile{}, constants.ErrLaunchProfileMissing
		}
		return GatewayLaunchProfile{}, fmt.Errorf("%w: read: %w", constants.ErrLaunchProfileInvalid, err)
	}

	var profile GatewayLaunchProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return GatewayLaunchProfile{}, fmt.Errorf("%w: unmarshal: %w", constants.ErrLaunchProfileCorrupted, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GatewayLaunchProfile{}, fmt.Errorf("%w: multiple JSON values", constants.ErrLaunchProfileCorrupted)
	}

	if profile.Version != LaunchProfileVersion {
		return GatewayLaunchProfile{}, fmt.Errorf("%w: got %d, want %d", constants.ErrLaunchProfileVersionUnsupported, profile.Version, LaunchProfileVersion)
	}

	if err := ValidateLaunchProfile(profile); err != nil {
		return GatewayLaunchProfile{}, err
	}

	return profile, nil
}

// ValidateLaunchProfile checks that every persisted field is safe to
// pass to StartOperator. Posture must be a recognized value, ports and
// rate limits must be non-negative, and the profile must not carry a
// stale network identity file path.
func ValidateLaunchProfile(profile GatewayLaunchProfile) error {
	cfg := profile.Config

	// Posture must be non-empty and recognized.
	if cfg.Posture == "" {
		return fmt.Errorf("%w: posture is empty", constants.ErrLaunchProfileInvalid)
	}
	if _, valid := constants.GetGovernancePostureRequirements(string(cfg.Posture)); !valid {
		return fmt.Errorf("%w: posture %q is not recognized", constants.ErrLaunchProfileInvalid, cfg.Posture)
	}

	// Ports must be non-negative (0 means "use default").
	if cfg.HTTPPort < 0 {
		return fmt.Errorf("%w: http_port %d is negative", constants.ErrLaunchProfileInvalid, cfg.HTTPPort)
	}
	if cfg.HTTPSPort < 0 {
		return fmt.Errorf("%w: https_port %d is negative", constants.ErrLaunchProfileInvalid, cfg.HTTPSPort)
	}
	if cfg.HTTPPort > 65535 {
		return fmt.Errorf("%w: http_port %d exceeds 65535", constants.ErrLaunchProfileInvalid, cfg.HTTPPort)
	}
	if cfg.HTTPSPort > 65535 {
		return fmt.Errorf("%w: https_port %d exceeds 65535", constants.ErrLaunchProfileInvalid, cfg.HTTPSPort)
	}
	if cfg.HTTPPort != 0 && cfg.HTTPPort == cfg.HTTPSPort {
		return fmt.Errorf("%w: http_port and https_port must differ", constants.ErrLaunchProfileInvalid)
	}

	// Rate limits must be non-negative.
	if cfg.RateLimitRPS < 0 {
		return fmt.Errorf("%w: rate_limit_rps %f is negative", constants.ErrLaunchProfileInvalid, cfg.RateLimitRPS)
	}
	if cfg.RateLimitBurst < 0 {
		return fmt.Errorf("%w: rate_limit_burst %d is negative", constants.ErrLaunchProfileInvalid, cfg.RateLimitBurst)
	}

	// The profile must not carry a stale network identity file path.
	// WriteLaunchProfile clears this field; a non-empty value means
	// the file was tampered with or written by an incompatible version.
	if cfg.NetworkIdentityFile != "" {
		return fmt.Errorf("%w: network_identity_file must be empty in a persisted profile", constants.ErrLaunchProfileInvalid)
	}
	if cfg.LogLevel != constants.LogLevelInfo && cfg.LogLevel != constants.LogLevelError && cfg.LogLevel != constants.LogLevelDebug {
		return fmt.Errorf("%w: log_level %q is not recognized", constants.ErrLaunchProfileInvalid, cfg.LogLevel)
	}
	if cfg.CertIdentityMode != "" && cfg.CertIdentityMode != "full" && cfg.CertIdentityMode != "localhost" {
		return fmt.Errorf("%w: cert_identity_mode %q is not recognized", constants.ErrLaunchProfileInvalid, cfg.CertIdentityMode)
	}
	for _, rawOrigin := range cfg.AllowedOrigins {
		if _, err := browserorigin.Parse(rawOrigin); err != nil {
			return fmt.Errorf("%w: allowed origin %q: %w", constants.ErrLaunchProfileInvalid, rawOrigin, err)
		}
	}
	for _, rawOrigin := range cfg.PasskeyRpOrigins {
		origin, err := browserorigin.Parse(rawOrigin)
		if err != nil {
			return fmt.Errorf("%w: passkey origin %q: %w", constants.ErrLaunchProfileInvalid, rawOrigin, err)
		}
		if _, err := browserorigin.ValidateRPID(origin, cfg.PasskeyRpID); err != nil {
			return fmt.Errorf("%w: passkey RP configuration: %w", constants.ErrLaunchProfileInvalid, err)
		}
	}
	if err := validateLaunchProfileURL(cfg.PublicBaseURL, false); err != nil {
		return fmt.Errorf("%w: public_base_url: %w", constants.ErrLaunchProfileInvalid, err)
	}
	if err := validateLaunchProfileURL(cfg.ConsensusURL, true); err != nil {
		return fmt.Errorf("%w: consensus_url: %w", constants.ErrLaunchProfileInvalid, err)
	}
	if err := validateLaunchProfileURL(cfg.MCPDownstreamURL, false); err != nil {
		return fmt.Errorf("%w: mcp_downstream_url: %w", constants.ErrLaunchProfileInvalid, err)
	}
	if err := validateLaunchProfileURL(cfg.A2ADownstreamURL, false); err != nil {
		return fmt.Errorf("%w: a2a_downstream_url: %w", constants.ErrLaunchProfileInvalid, err)
	}

	return nil
}

func validateLaunchProfileURL(raw string, httpsOnly bool) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return constants.ErrValidationFailed
	}
	if (httpsOnly && parsed.Scheme != "https") || (!httpsOnly && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return constants.ErrValidationFailed
	}
	return nil
}

// DeleteLaunchProfile removes the persisted launch profile. Called by
// `gw clean` indirectly (the runtime tree wipe removes it) and available
// for explicit cleanup if needed.
func DeleteLaunchProfile(fileSvc fs.RuntimeFileService) error {
	return fileSvc.Remove(context.Background(), launchProfileRelPath())
}

// LaunchProfileExists checks whether a launch profile file is present.
func LaunchProfileExists(ctx context.Context, fileSvc fs.RuntimeFileService) (bool, error) {
	return fileSvc.FileExists(ctx, launchProfileRelPath())
}

// isNotFound returns true when the wrapped error is constants.ErrNotFound.
// RuntimeFileService.ReadFile returns ErrNotFound for missing files.
func isNotFound(err error) bool {
	return errors.Is(err, constants.ErrNotFound)
}
