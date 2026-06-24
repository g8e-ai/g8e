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

package serve

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/exitcode"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services"
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
}

// RunOperator runs the operator in standalone mode with the given options.
func RunOperator(opts ServeOperatorOptions, vi VersionInfo) {
	logger, err := ConfigureLogger(opts.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", opts.LogLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	operatorEndpoint := constants.DefaultEndpoint
	if strings.TrimSpace(opts.Endpoint) != "" {
		operatorEndpoint = strings.TrimSpace(opts.Endpoint)
	}

	logger.Info("g8e", "version", vi.Version, "build", vi.BuildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	trustStore := certs.NewTrustStore(nil)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})

	trustLoaded := LoadTrustBundle(logger, opts.TrustBundlePath, trustStore)
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

	privateKey := opts.PrivateKey
	clientCert := opts.ClientCert

	if privateKey == "" {
		if _, err := os.Stat(paths.Infra.OperatorKeyPath); err == nil {
			privateKey = paths.Infra.OperatorKeyPath
			logger.Info("Using default Operator key from project directory", "path", privateKey)
		} else {
			if _, err := os.Stat(paths.Infra.ClientOperatorKeyPath); err == nil {
				privateKey = paths.Infra.ClientOperatorKeyPath
				logger.Info("Using default client key from project directory", "path", privateKey)
			}
		}
	}

	if clientCert == "" {
		if _, err := os.Stat(paths.Infra.OperatorCertPath); err == nil {
			clientCert = paths.Infra.OperatorCertPath
			logger.Info("Using default Operator certificate from project directory", "path", clientCert)
		} else {
			if _, err := os.Stat(paths.Infra.ClientOperatorCertPath); err == nil {
				clientCert = paths.Infra.ClientOperatorCertPath
				logger.Info("Using default client certificate from project directory", "path", clientCert)
			}
		}
	}

	if opts.Endpoint != "" {
		logger.Info("Performing automatic enrollment with Gateway", "endpoint", opts.Endpoint)
		if err := PerformAutomaticEnrollment(opts.Endpoint, opts.LaunchDir, logger); err != nil {
			logger.Error("Automatic enrollment failed", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Automatic enrollment failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", opts.Endpoint)
			os.Exit(constants.ExitConfigError)
		}

		privateKey = paths.Infra.OperatorKeyPath
		clientCert = paths.Infra.OperatorCertPath

		if pemData, err := os.ReadFile(paths.Infra.CaCertPath); err == nil {
			trustStore.SetCA(pemData)
			logger.Info("Trust bundle reloaded after enrollment", "path", paths.Infra.CaCertPath)
		}

		logger.Info("Automatic enrollment completed, using enrolled certificates")
	}

	if privateKey == "" {
		fmt.Fprintf(os.Stderr, "%s (-k or --key). Expected locations:\n", constants.ErrPrivateKeyRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorKeyDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientKeyDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "%s (--cert or --client-cert). Expected locations:\n", constants.ErrClientCertRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorCertDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientCertDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform automatic enrollment\n")
		os.Exit(constants.ExitConfigError)
	}

	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	certPEM, err := os.ReadFile(clientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrReadClientCert, err)
		os.Exit(constants.ExitConfigError)
	}

	keyPEM, err := os.ReadFile(privateKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrReadPrivateKey, err)
		os.Exit(constants.ExitConfigError)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrLoadCertKeyPair, err)
		os.Exit(constants.ExitConfigError)
	}

	clientIdentity.SetCertificate(cert)
	LogCertBundle(logger, "client-cert", certPEM)
	logger.Info("[TLS-DEBUG] client cert loaded",
		"cert_file", clientCert,
		"key_file", privateKey,
	)

	effectiveWorkDir := opts.LaunchDir
	if opts.WorkingDir != "" {
		effectiveWorkDir = opts.WorkingDir
	}

	cfg, err := config.Load(config.LoadOptions{
		OperatorEndpoint: operatorEndpoint,
		HTTPPort:         0,
		HTTPSPort:        0,
		CloudMode:        opts.CloudMode,
		CloudProvider:    opts.CloudProvider,
		ExecutionVaultEnabled: opts.ExecutionVault,
		NoGit:            opts.NoGit,
		LogLevel:         opts.LogLevel,
		WorkDir:          effectiveWorkDir,
		PKIDir:           "",
		SecretsDir:       "",
		HeartbeatInterval: opts.HeartbeatInterval,
		Shell:            os.Getenv(string(constants.EnvVar.Shell)),
		Lang:             os.Getenv(string(constants.EnvVar.Lang)),
		Term:             os.Getenv(string(constants.EnvVar.Term)),
		TZ:               os.Getenv(string(constants.EnvVar.TZ)),
		Posture:          "",
	})
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

	g8eoService, err := services.NewG8eoService(cfg, logger, tlsConfig)
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

	if clientCert != "" && privateKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RunClientCertRenewalLoop(ctx, cfg, clientCert, privateKey, logger, clientIdentity)
		}()
	}

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
