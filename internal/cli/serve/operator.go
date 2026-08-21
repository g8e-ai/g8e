// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/adapters/lattice"
	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/exitcode"
	"github.com/g8e-ai/g8e/internal/services"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/logging"
)

// ServeOperatorOptions holds the configuration for running the operator in standalone mode.
type ServeOperatorOptions struct {
	LogLevel          string
	Endpoint          string
	TrustBundlePath   string
	PrivateKey        string
	ClientCert        string
	WorkingDir        string
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

// resolveKeyPath returns the explicit key path if set, otherwise checks the default
// operator and client key paths on disk. Returns empty string if none are found.
func resolveKeyPath(privateKey string, fileSvc fs.RuntimeFileService, logger *slog.Logger) string {
	if privateKey != "" {
		return privateKey
	}
	opKeyRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
	if exists, err := fileSvc.FileExists(context.Background(), opKeyRel); err == nil && exists {
		opKeyPath := fileSvc.Resolve(opKeyRel)
		logger.Info("Using default Operator key from project directory", "path", opKeyPath)
		return opKeyPath
	}
	cliKeyRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorKey)
	if exists, err := fileSvc.FileExists(context.Background(), cliKeyRel); err == nil && exists {
		cliKeyPath := fileSvc.Resolve(cliKeyRel)
		logger.Info("Using default client key from project directory", "path", cliKeyPath)
		return cliKeyPath
	}
	return ""
}

// resolveCertPath returns the explicit cert path if set, otherwise checks the default
// operator and client cert paths on disk. Returns empty string if none are found.
func resolveCertPath(clientCert string, fileSvc fs.RuntimeFileService, logger *slog.Logger) string {
	if clientCert != "" {
		return clientCert
	}
	opCertRel := filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
	if exists, err := fileSvc.FileExists(context.Background(), opCertRel); err == nil && exists {
		opCertPath := fileSvc.Resolve(opCertRel)
		logger.Info("Using default Operator certificate from project directory", "path", opCertPath)
		return opCertPath
	}
	cliCertRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient, constants.PkiFileOperatorCert)
	if exists, err := fileSvc.FileExists(context.Background(), cliCertRel); err == nil && exists {
		cliCertPath := fileSvc.Resolve(cliCertRel)
		logger.Info("Using default client certificate from project directory", "path", cliCertPath)
		return cliCertPath
	}
	return ""
}

// loadClientCertPair reads the cert and key PEM files and returns the TLS certificate
// along with the raw cert PEM bytes for logging.
func loadClientCertPair(certPath, keyPath string) (tls.Certificate, []byte, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: %w", constants.ErrReadClientCert, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: %w", constants.ErrReadPrivateKey, err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("%w: %w", constants.ErrLoadCertKeyPair, err)
	}
	return cert, certPEM, nil
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
		OperatorEndpoint:      operatorEndpoint,
		HTTPPort:              0,
		HTTPSPort:             0,
		CloudMode:             opts.CloudMode,
		CloudProvider:         opts.CloudProvider,
		ExecutionVaultEnabled: opts.ExecutionVault,
		NoGit:                 opts.NoGit,
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
	}
}

// RunOperator runs the operator in standalone mode with the given options.
func RunOperator(opts ServeOperatorOptions, vi VersionInfo) {
	logger, err := logging.NewStdoutLogger(opts.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", opts.LogLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := resolveOperatorEndpoint(opts.Endpoint)

	logger.Info("g8e", "version", vi.Version, "build", vi.BuildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	// Construct RuntimeFileService early so all .g8e/ I/O goes through it
	fileSvc, err := fs.NewRuntimeFileService("", logger)
	if err != nil {
		logger.Error("Failed to create file service", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}
	if err := fileSvc.CreateRuntimeTree(context.Background()); err != nil {
		logger.Error("Failed to create runtime tree", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}

	trustStore := certs.NewTrustStore(nil)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})

	trustLoaded := LoadTrustBundle(context.Background(), logger, opts.TrustBundlePath, fileSvc, trustStore)
	if !trustLoaded {
		if opts.Endpoint != "" {
			trustURL := fmt.Sprintf("http://%s:%d%s", opts.Endpoint, constants.Ports.OperatorHttp, constants.WellKnownPKICABundle)
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			pemData, err := certs.FetchTrustBundle(context.Background(), trustURL, "")
			if err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, string(constants.ConnectionStateError), err)
				fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrFetchTrustBundle, err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e gw start\n")
				os.Exit(constants.ExitConfigError)
			}
			LogCertBundle(logger, "fetched-trust-bundle", pemData)
			trustStore.SetCA(pemData)
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "%s. Provide --trust-bundle or --endpoint\n", constants.ErrNoTrustBundle)
			os.Exit(constants.ExitConfigError)
		}
	}
	logger.Info("Trust bundle loaded")

	privateKey := resolveKeyPath(opts.PrivateKey, fileSvc, logger)
	clientCert := resolveCertPath(opts.ClientCert, fileSvc, logger)

	// If no installed operator credentials exist and an endpoint is
	// provided, drive the owner-approved platform enrollment protocol
	// to obtain them. This replaces the removed bypass
	// PerformAutomaticEnrollment. The operator submits both an operator
	// CSR and a CLI CSR, waits for owner approval, signs the canonical
	// completion transcript with both private keys, and writes the
	// issued credentials atomically. Pending state is persisted to
	// pki/pending-enrollment/g8eo.json so a kill-and-restart resumes
	// the same request and key material.
	if privateKey == "" && clientCert == "" && opts.Endpoint != "" {
		logger.Info("No installed operator credentials found; starting platform enrollment", "endpoint", opts.Endpoint)
		gatewayHTTPURL := fmt.Sprintf("http://%s:%d", opts.Endpoint, constants.Ports.OperatorHttp)
		hostname, err := os.Hostname()
		if err != nil {
			logger.Error("Failed to resolve hostname for enrollment", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			os.Exit(constants.ExitConfigError)
		}
		instanceID := fmt.Sprintf("operator-%s", hostname)
		enrollClient, err := NewOperatorPlatformEnrollmentClient(gatewayHTTPURL, instanceID, hostname, fileSvc, logger)
		if err != nil {
			logger.Error("Failed to create enrollment client", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			os.Exit(constants.ExitConfigError)
		}
		result, err := enrollClient.Enroll(context.Background())
		if err != nil {
			logger.Error("Platform enrollment failed", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", opts.Endpoint)
			fmt.Fprintf(os.Stderr, "  Pending state is persisted; restart to resume the same request.\n")
			os.Exit(constants.ExitConfigError)
		}
		os.Setenv(string(constants.EnvVar.OperatorSessionID), result.OperatorSessionID)
		if result.Posture != "" {
			opts.Posture = result.Posture
		}
		privateKey = result.OperatorKeyPath
		clientCert = result.OperatorCertPath

		// Reload the trust bundle from the newly written file.
		caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		pemData, err := fileSvc.ReadFile(context.Background(), caBundleRel)
		if err != nil {
			logger.Error("Failed to reload trust bundle after enrollment", "path", fileSvc.Resolve(caBundleRel), string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrFailedToReadTrustBundle, err)
			os.Exit(constants.ExitConfigError)
		}
		trustStore.SetCA(pemData)
		logger.Info("Trust bundle reloaded after enrollment", "path", fileSvc.Resolve(caBundleRel))
		logger.Info("Platform enrollment completed, using enrolled certificates")
	}

	if privateKey == "" {
		fmt.Fprintf(os.Stderr, "%s (-k or --key). Expected locations:\n", constants.ErrPrivateKeyRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorKeyDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientKeyDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform platform enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "%s (--cert or --client-cert). Expected locations:\n", constants.ErrClientCertRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorCertDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientCertDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform platform enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	cert, certPEM, err := loadClientCertPair(clientCert, privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	clientIdentity.SetCertificate(cert)
	LogCertBundle(logger, "client-cert", certPEM)
	logger.Info("[TLS-DEBUG] client cert loaded",
		"cert_file", clientCert,
		"key_file", privateKey,
	)

	effectiveWorkDir := resolveWorkingDir(opts.WorkingDir, opts.LaunchDir)

	cfg, err := config.Load(buildOperatorLoadOptions(opts, operatorEndpoint, effectiveWorkDir))
	if err != nil {
		logger.Error("Failed to load configuration", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}

	cfg.Version = vi.Version

	if cfg.CloudMode {
		logger.Info("Cloud Operator mode enabled", "provider", cfg.CloudProvider)
	}

	if cfg.ExecutionVaultEnabled {
		logger.Info("Execution vault enabled - data stays in working directory", "working_dir", cfg.WorkDir)
	} else {
		logger.Info("Execution vault disabled (command output sent to cloud)")
	}

	g8eoService, err := services.NewG8eoService(cfg, logger, tlsConfig, fileSvc)
	if err != nil {
		logger.Error("Failed to create Operator service", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	serviceErr := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := g8eoService.Start(ctx); err != nil {
			logger.Error("Failed to start g8e", string(constants.ConnectionStateError), err)
			serviceErr <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		RunClientCertRenewalLoop(ctx, cfg, fileSvc, clientCert, privateKey, logger, clientIdentity)
	}()

	select {
	case err := <-serviceErr:
		logger.Error("Service failed, shutting down", string(constants.ConnectionStateError), err)
		cancel()
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig.String())
		cancel()
	}

	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Duration(constants.ShutdownTimeout)*time.Second)

	if err := g8eoService.Stop(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", string(constants.ConnectionStateError), err)
	}
	shutdownCancel()

	os.Exit(constants.ExitSuccess)
}
