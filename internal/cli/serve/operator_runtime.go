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
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/exitcode"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
)

// OperatorRuntimeDeps are the process-owned dependencies of one Operator
// runtime. ClientIdentity is the single in-memory holder of the Operator's
// mTLS identity; every outbound TLS connection reads it.
type OperatorRuntimeDeps struct {
	Logger         *slog.Logger
	ClientIdentity *certs.ClientIdentity
}

// OperatorRuntime is one started outbound Operator. It owns the service and
// its background work until Stop.
type OperatorRuntime struct {
	service    *services.G8eoService
	logger     *slog.Logger
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	serviceErr chan error
}

// operatorStartError carries the process exit code for a startup failure.
type operatorStartError struct {
	code int
	err  error
}

func (e *operatorStartError) Error() string { return e.err.Error() }
func (e *operatorStartError) Unwrap() error { return e.err }

func startFailure(code int, err error) error {
	return &operatorStartError{code: code, err: err}
}

// operatorExitCode maps a StartOperator failure to the process exit code.
func operatorExitCode(err error) int {
	var startErr *operatorStartError
	if errors.As(err, &startErr) {
		return startErr.code
	}
	return exitcode.FromError(err)
}

// StartOperator resolves the Operator identity, constructs the service, and
// starts it. It returns once the service is starting; Failed reports a start
// failure and Done reports a service-requested shutdown.
func StartOperator(ctx context.Context, opts ServeOperatorOptions, vi VersionInfo, deps OperatorRuntimeDeps) (_ *OperatorRuntime, err error) {
	logger := deps.Logger
	if logger == nil || deps.ClientIdentity == nil {
		return nil, fmt.Errorf("%w: operator runtime requires a logger and client identity", constants.ErrInternal)
	}
	clientIdentity := deps.ClientIdentity

	operatorEndpoint := config.GatewayDialHost(resolveOperatorEndpoint(opts.Endpoint))
	if opts.Endpoint != "" {
		opts.Endpoint = operatorEndpoint
	}

	logger.Info("g8e", "version", vi.Version, "build", vi.BuildID)
	logger.Info("Using Operator endpoint", "endpoint", operatorEndpoint)

	// Resolve the worker root before constructing RuntimeFileService. The
	// --working-dir flag scopes both command execution and the worker's local
	// .g8e evidence stores; using the launch directory here silently split
	// those two concerns and made reports inspect the wrong vault and ledger.
	fileSvc, effectiveWorkDir, err := newOperatorRuntimeFileService(opts, logger)
	if err != nil {
		logger.Error("Failed to create file service", string(constants.ConnectionStateError), err)
		return nil, startFailure(exitcode.FromError(err), err)
	}
	if err := fileSvc.CreateRuntimeTree(ctx); err != nil {
		logger.Error("Failed to create runtime tree", string(constants.ConnectionStateError), err)
		return nil, startFailure(exitcode.FromError(err), err)
	}
	deployment, err := NewOperatorDeploymentRecorder(fileSvc, opts.DeploymentID)
	if err != nil {
		logger.Error("Failed to create deployment recorder", "error", err)
		return nil, startFailure(exitcode.FromError(err), err)
	}
	if err := deployment.Reset(ctx); err != nil {
		logger.Error("Failed to reset deployment state", string(constants.ConnectionStateError), err)
		return nil, startFailure(exitcode.FromError(err), err)
	}

	// Keep early configuration/enrollment failures observable to the deployer.
	recordDeploymentFailure := func() {
		if err := deployment.Record(context.Background(), models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhaseFailed, Error: constants.ErrOperatorDeployFailed.Error()}); err != nil {
			logger.Error("Failed to record deployment failure", "error", err)
		}
	}
	defer func() {
		if err != nil {
			recordDeploymentFailure()
		}
	}()

	trustStore := certs.NewTrustStore(nil)

	trustLoaded := LoadTrustBundle(ctx, logger, opts.TrustBundlePath, fileSvc, trustStore)
	if !trustLoaded {
		if opts.Endpoint != "" {
			baseURL := buildGatewayHTTPBaseURL(opts.Endpoint, opts.HTTPPort)
			trustURL := baseURL + constants.WellKnownPKICABundle
			logger.Info("Fetching trust bundle from Operator PKI endpoint", "url", trustURL)
			pemData, err := certs.FetchTrustBundle(ctx, trustURL, "")
			if err != nil {
				logger.Error("Failed to fetch trust bundle from Operator", "url", trustURL, string(constants.ConnectionStateError), err)
				fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrFetchTrustBundle, err)
				fmt.Fprintf(os.Stderr, "  Ensure the platform is running: ./g8e gw start\n")
				return nil, startFailure(constants.ExitConfigError, fmt.Errorf("%w: %w", constants.ErrFetchTrustBundle, err))
			}
			LogCertBundle(logger, "fetched-trust-bundle", pemData)
			trustStore.SetCA(pemData)
		} else {
			logger.Error("No trust bundle available and no endpoint specified")
			fmt.Fprintf(os.Stderr, "%s. Provide --trust-bundle or --endpoint\n", constants.ErrNoTrustBundle)
			return nil, startFailure(constants.ExitConfigError, constants.ErrNoTrustBundle)
		}
	}
	logger.Info("Trust bundle loaded")

	privateKey := resolveKeyPath(opts.PrivateKey, fileSvc, logger)
	clientCert := resolveCertPath(opts.ClientCert, fileSvc, logger)
	enrolled := false

	// If no installed operator credentials exist and an endpoint is
	// provided, drive the owner-approved platform enrollment protocol
	// to obtain them. The operator submits both an operator CSR and a CLI
	// CSR, waits for owner approval, signs the canonical completion
	// transcript with both private keys, and writes the issued credentials
	// atomically. Pending state is persisted to
	// pki/pending-enrollment/g8eo.json so a kill-and-restart resumes the
	// same request and key material.
	if privateKey == "" && clientCert == "" && opts.Endpoint != "" {
		// Persist an explicitly supplied CA before enrollment so the bootstrap
		// response cannot replace the trust selected by the operator owner.
		if opts.TrustBundlePath != "" {
			pemData, err := os.ReadFile(opts.TrustBundlePath)
			pool := x509.NewCertPool()
			if err != nil || !pool.AppendCertsFromPEM(pemData) {
				logger.Error("Invalid explicit trust bundle", "path", opts.TrustBundlePath)
				return nil, startFailure(constants.ExitConfigError, fmt.Errorf("%w: invalid explicit trust bundle %s", constants.ErrCAParseFailed, opts.TrustBundlePath))
			}
			path := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
			if err := fileSvc.WriteFile(ctx, path, pemData, constants.PermFilePublic); err != nil {
				logger.Error("Failed to persist explicit trust bundle", "error", err)
				return nil, startFailure(constants.ExitConfigError, err)
			}
		}
		logger.Info("No installed operator credentials found; starting platform enrollment", "endpoint", opts.Endpoint)
		gatewayHTTPURL := buildGatewayHTTPBaseURL(opts.Endpoint, opts.HTTPPort)
		hostname, err := os.Hostname()
		if err != nil {
			logger.Error("Failed to resolve hostname for enrollment", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			return nil, startFailure(constants.ExitConfigError, err)
		}
		role := operatorRoles(opts)
		account := auth.ResolveCurrentAccount()
		instanceID := operatorInstanceID(hostname, role, effectiveWorkDir)
		enrollClient, err := NewOperatorPlatformEnrollmentClient(gatewayHTTPURL, instanceID, hostname, fileSvc, logger)
		if err != nil {
			logger.Error("Failed to create enrollment client", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			return nil, startFailure(constants.ExitConfigError, err)
		}
		enrollClient.SetDeploymentRecorder(deployment)
		enrollClient.SetFingerprintOptions(operatorFingerprintOptions(opts, effectiveWorkDir, account))
		result, err := enrollClient.Enroll(ctx)
		if err != nil {
			logger.Error("Platform enrollment failed", string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", opts.Endpoint)
			fmt.Fprintf(os.Stderr, "  Pending state is persisted; restart to resume the same request.\n")
			return nil, startFailure(constants.ExitConfigError, err)
		}

		os.Setenv(string(constants.EnvVar.OperatorSessionID), result.OperatorSessionID)
		if result.Posture != "" {
			opts.Posture = result.Posture
		}
		privateKey = result.OperatorKeyPath
		clientCert = result.OperatorCertPath
		enrolled = true

		// Reload the trust bundle from the newly written file.
		caBundleRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		pemData, err := fileSvc.ReadFile(ctx, caBundleRel)
		if err != nil {
			logger.Error("Failed to reload trust bundle after enrollment", "path", fileSvc.Resolve(caBundleRel), string(constants.ConnectionStateError), err)
			fmt.Fprintf(os.Stderr, "%s: %v\n", constants.ErrFailedToReadTrustBundle, err)
			return nil, startFailure(constants.ExitConfigError, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err))
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
		return nil, startFailure(constants.ExitConfigError, constants.ErrPrivateKeyRequired)
	}

	if clientCert == "" {
		fmt.Fprintf(os.Stderr, "%s (--cert or --client-cert). Expected locations:\n", constants.ErrClientCertRequired)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultOperatorCertDesc)
		fmt.Fprintf(os.Stderr, "  - %s (project directory)\n", constants.DefaultClientCertDesc)
		fmt.Fprintf(os.Stderr, "Or provide --endpoint to perform platform enrollment\n")
		return nil, startFailure(constants.ExitConfigError, constants.ErrClientCertRequired)
	}

	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	var cert tls.Certificate
	var certPEM []byte
	if enrolled {
		cert, certPEM, err = loadClientCertPairViaFileSvc(ctx, fileSvc, clientCert, privateKey)
	} else {
		cert, certPEM, err = loadClientCertPair(clientCert, privateKey)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return nil, startFailure(constants.ExitConfigError, err)
	}
	if enrolled {
		// Enrollment returns runtime-relative paths for file-service writes.
		// Background renewal uses os/x509 APIs and therefore needs the same
		// files expressed as absolute paths.
		clientCert = fileSvc.Resolve(clientCert)
		privateKey = fileSvc.Resolve(privateKey)
	}

	clientIdentity.SetCertificate(cert)
	LogCertBundle(logger, "client-cert", certPEM)
	logger.Info("[TLS-DEBUG] client cert loaded",
		"cert_file", clientCert,
		"key_file", privateKey,
	)

	effectiveWorkDir = resolveWorkingDir(opts.WorkingDir, opts.LaunchDir)

	cfg, err := config.Load(buildOperatorLoadOptions(opts, operatorEndpoint, effectiveWorkDir))
	if err != nil {
		exitCode, actionable := classifyConfigLoadError(err)
		logger.Error("Failed to load configuration", string(constants.ConnectionStateError), err)
		if actionable != "" {
			fmt.Fprintln(os.Stderr, actionable)
		}
		return nil, startFailure(exitCode, err)
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

	g8eoService, err := services.NewG8eoService(cfg, logger, tlsConfig, fileSvc, func(baseURL, serverName string, logger *slog.Logger, tlsConfig *certs.TLSConfig) (pubsub.PubSubClient, error) {
		client, err := pubsub.NewOperatorPubSubClient(baseURL, serverName, logger, tlsConfig)
		if err != nil {
			return nil, err
		}
		client.SetDeploymentID(opts.DeploymentID)
		return client, nil
	})
	if err != nil {
		logger.Error("Failed to create Operator service", string(constants.ConnectionStateError), err)
		return nil, startFailure(exitcode.FromError(err), err)
	}

	if err := g8eoService.SetCommandSubscriptionObserver(deployment.CommandSubscriptionChanged); err != nil {
		logger.Error("Failed to configure deployment observer", "error", err)
		return nil, startFailure(exitcode.FromError(err), err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	rt := &OperatorRuntime{
		service:    g8eoService,
		logger:     logger,
		cancel:     cancel,
		serviceErr: make(chan error, 1),
	}

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		if err := g8eoService.Start(runCtx); err != nil {
			recordDeploymentFailure()
			logger.Error("Failed to start g8e", string(constants.ConnectionStateError), err)
			rt.serviceErr <- err
		}
	}()

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		RunClientCertRenewalLoop(runCtx, cfg, fileSvc, clientCert, privateKey, logger, clientIdentity)
	}()

	return rt, nil
}

// Failed delivers the service start failure, if any.
func (rt *OperatorRuntime) Failed() <-chan error {
	return rt.serviceErr
}

// Done is closed when the Operator service requests shutdown.
func (rt *OperatorRuntime) Done() <-chan struct{} {
	return rt.service.Done()
}

// Stop cancels the runtime, joins its background work, and stops the service.
func (rt *OperatorRuntime) Stop(ctx context.Context) error {
	rt.cancel()
	rt.wg.Wait()
	return rt.service.Stop(ctx)
}
