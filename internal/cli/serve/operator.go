// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/adapters/lattice"
	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/logging"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// ServeOperatorOptions holds the configuration for running the operator in standalone mode.
type ServeOperatorOptions struct {
	OperatorRoles constants.OperatorRoles
	LogLevel      string
	Endpoint      string
	// HTTPPort and HTTPSPort override the Gateway discovery and mTLS ports the
	// Operator dials; zero selects the platform defaults.
	HTTPPort          int
	HTTPSPort         int
	TrustBundlePath   string
	WorkingDir        string
	DeploymentID      string
	LaunchDir         string
	CloudMode         bool
	CloudProvider     string
	ExecutionVault    bool
	NoGit             bool
	HeartbeatInterval time.Duration

	Posture string

	LatticeEndpoint       string
	LatticeClientID       string
	LatticeClientSecret   string
	LatticeSandboxesToken string
	LatticeEntityName     string
	LatticePostureFloor   string

	// Inference (g8ellama). Enabled when the operator runs as an Inference
	// Node calling the configured remote Ollama provider.
	InferenceEnabled        bool
	InferenceOllamaEndpoint string
	InferenceKeepAlive      string

	ProviderBoundaryObserverEnabled bool
	ProviderBoundaryObserverID      string

	ProvenanceOperatorEnabled          bool
	ProvenanceOperatorID               string
	ProvenanceOperatorModelStorageRoot string

	// MasterKeyFile is an operator-provisioned master key outside the runtime
	// directory, required where no OS key store exists.
	MasterKeyFile string
}

// resolveOperatorEndpoint returns the trimmed endpoint if non-empty, otherwise the default endpoint.
func resolveOperatorEndpoint(endpoint string) string {
	if trimmed := strings.TrimSpace(endpoint); trimmed != "" {
		return trimmed
	}
	return constants.DefaultEndpoint
}

// resolveWorkingDir returns workingDir if set, otherwise falls back to launchDir.
func resolveWorkingDir(workingDir, launchDir string) string {
	if workingDir != "" {
		return workingDir
	}
	return launchDir
}

// classifyConfigLoadError inspects a config.Load error and returns the
// appropriate exit code and an actionable user-facing message. All
// config.Load errors return ExitConfigError with no actionable message.
// The operator no longer requires config posture to start (it reads
// posture per-envelope from GovernanceEnvelope.Posture at L4 verification
// time), so the former posture-required enrollment-pending path is gone.
func classifyConfigLoadError(err error) (exitCode int, actionable string) {
	return constants.ExitConfigError, ""
}

// resolveLatticeOpt returns the flag value if set, otherwise falls back to the
// corresponding environment variable.
func resolveLatticeOpt(flagVal string, envKey constants.EnvVarKey) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(string(envKey))
}

// buildOperatorLoadOptions creates config.LoadOptions from ServeOperatorOptions and
// resolved runtime parameters.
func buildOperatorLoadOptions(opts ServeOperatorOptions, operatorEndpoint, effectiveWorkDir string) config.LoadOptions {
	latticeEndpoint := resolveLatticeOpt(opts.LatticeEndpoint, constants.EnvVar.LatticeEndpoint)
	var latticeCfg *lattice.LatticeConfig
	if latticeEndpoint != "" {
		latticeCfg = &lattice.LatticeConfig{
			Enabled:        true,
			Endpoint:       latticeEndpoint,
			ClientID:       resolveLatticeOpt(opts.LatticeClientID, constants.EnvVar.LatticeClientID),
			ClientSecret:   resolveLatticeOpt(opts.LatticeClientSecret, constants.EnvVar.LatticeClientSecret),
			SandboxesToken: resolveLatticeOpt(opts.LatticeSandboxesToken, constants.EnvVar.LatticeSandboxesToken),
			Entity: lattice.EntityConfig{
				Name:         resolveLatticeOpt(opts.LatticeEntityName, constants.EnvVar.LatticeEntityName),
				PlatformType: "g8e-operator",
			},
			PostureFloor: resolveLatticeOpt(opts.LatticePostureFloor, constants.EnvVar.LatticePostureFloor),
		}
	}

	return config.LoadOptions{
		OperatorRoles:         opts.OperatorRoles,
		OperatorEndpoint:      operatorEndpoint,
		HTTPPort:              opts.HTTPPort,
		HTTPSPort:             opts.HTTPSPort,
		CloudMode:             opts.CloudMode,
		CloudProvider:         opts.CloudProvider,
		ExecutionVaultEnabled: opts.ExecutionVault,
		NoGit:                 opts.NoGit,
		MasterKeyFile:         opts.MasterKeyFile,
		LogLevel:              opts.LogLevel,
		WorkDir:               effectiveWorkDir,
		PKIDir:                "",
		SecretsDir:            "",
		HeartbeatInterval:     opts.HeartbeatInterval,
		Shell:                 os.Getenv(string(constants.EnvVar.Shell)),
		Lang:                  os.Getenv(string(constants.EnvVar.Lang)),
		Term:                  os.Getenv(string(constants.EnvVar.Term)),
		TZ:                    os.Getenv(string(constants.EnvVar.TZ)),
		Posture:               config.GatewayPosture(opts.Posture),

		Lattice: latticeCfg,

		InferenceEnabled:        opts.InferenceEnabled,
		InferenceOllamaEndpoint: opts.InferenceOllamaEndpoint,
		InferenceKeepAlive:      opts.InferenceKeepAlive,

		ProviderBoundaryObserverEnabled: opts.ProviderBoundaryObserverEnabled,
		ProviderBoundaryObserverID:      opts.ProviderBoundaryObserverID,

		ProvenanceOperatorEnabled:          opts.ProvenanceOperatorEnabled,
		ProvenanceOperatorID:               opts.ProvenanceOperatorID,
		ProvenanceOperatorModelStorageRoot: opts.ProvenanceOperatorModelStorageRoot,
	}
}

func newOperatorRuntimeFileService(opts ServeOperatorOptions, logger *slog.Logger) (fs.RuntimeFileService, string, error) {
	effectiveWorkDir := resolveWorkingDir(opts.WorkingDir, opts.LaunchDir)
	fileSvc, err := fs.NewRuntimeFileService(effectiveWorkDir, logger)
	return fileSvc, effectiveWorkDir, err
}

// RunOperator runs the operator in standalone mode with the given options.
func RunOperator(opts ServeOperatorOptions, vi VersionInfo) {
	logger, err := logging.NewStdoutLogger(opts.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", opts.LogLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := StartOperator(ctx, opts, vi, OperatorRuntimeDeps{
		Logger:         logger,
		ClientIdentity: certs.NewClientIdentity(tls.Certificate{}),
	})
	if err != nil {
		os.Exit(operatorExitCode(err))
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-rt.Failed():
		logger.Error("Service failed, shutting down", string(constants.ConnectionStateError), err)
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig.String())
	case <-rt.Done():
		logger.Info("Operator service requested shutdown")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Duration(constants.ShutdownTimeout)*time.Second)
	if err := rt.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", string(constants.ConnectionStateError), err)
	}
	shutdownCancel()

	os.Exit(constants.ExitSuccess)
}

func operatorRoles(opts ServeOperatorOptions) constants.OperatorRoles {
	return operatorcapability.ResolveOperatorRoles(&operatorv1.OperatorRuntimeConfig{Roles: models.OperatorRolesToProto(opts.OperatorRoles), InferenceEnabled: opts.InferenceEnabled, ProvenanceOperatorEnabled: opts.ProvenanceOperatorEnabled, ProviderBoundaryObserverEnabled: opts.ProviderBoundaryObserverEnabled})
}

func operatorFingerprintOptions(opts ServeOperatorOptions, localDir, account string) auth.FingerprintOptions {
	return auth.FingerprintOptions{
		LocalDir: localDir,
		Account:  account,
		Port:     constants.Ports.OperatorHttp,
		Roles:    operatorRoles(opts),
	}
}

// Include the canonical runtime directory so same-host, same-role workers enroll independently.
func operatorInstanceID(hostname string, role constants.OperatorRoles, dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if absolute, err := filepath.Abs(dir); err == nil {
		dir = absolute
	}
	digest := sha256.Sum256([]byte(dir))
	roleName := role.String()
	hostnameLimit := constants.PlatformEnrollmentMaxInstanceIDBytes - len("operator--") - len(roleName) - 1 - 32
	if hostnameLimit > 64 {
		hostnameLimit = 64
	}
	return fmt.Sprintf("operator-%.*s-%s-%x", hostnameLimit, hostname, roleName, digest[:16])
}
