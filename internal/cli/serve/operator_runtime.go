// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

	// An owner-supplied CA is read, never persisted, and wins over the
	// trust delivered in the bundle.
	var explicitTrust []byte
	if opts.TrustBundlePath != "" {
		explicitTrust, err = os.ReadFile(opts.TrustBundlePath)
		if err != nil || !x509.NewCertPool().AppendCertsFromPEM(explicitTrust) {
			logger.Error("Invalid explicit trust bundle", "path", opts.TrustBundlePath)
			return nil, startFailure(constants.ExitConfigError, fmt.Errorf("%w: invalid explicit trust bundle %s", constants.ErrCAParseFailed, opts.TrustBundlePath))
		}
	}

	identity, err := enrollOperator(ctx, opts, cfg, operatorEndpoint, effectiveWorkDir, explicitTrust, deployment, logger)
	if err != nil {
		logger.Error("Operator enrollment failed", string(constants.ConnectionStateError), err)
		fmt.Fprintf(os.Stderr, "Enrollment failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Ensure the Gateway is running and accessible at %s\n", operatorEndpoint)
		return nil, startFailure(constants.ExitConfigError, err)
	}

	trustStore := certs.NewTrustStore(nil)
	if explicitTrust != nil {
		trustStore.SetCA(explicitTrust)
	} else {
		trustStore.SetCA(identity.TrustBundlePEM)
	}
	clientIdentity.SetCertificate(identity.Certificate)
	LogCertBundle(logger, "client-cert", identity.CertificatePEM)
	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)

	cfg.OperatorID = identity.OperatorID
	cfg.OperatorSessionId = identity.OperatorSessionID
	cfg.SystemFingerprint = identity.SystemFingerprint
	if identity.Posture != "" {
		cfg.Posture = config.GatewayPosture(identity.Posture)
	}
	if identity.MaxConcurrentTasks > 0 {
		cfg.MaxConcurrentTasks = identity.MaxConcurrentTasks
	}
	if identity.MaxMemoryMB > 0 {
		cfg.MaxMemoryMB = identity.MaxMemoryMB
	}

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

	return rt, nil
}

// enrollOperator obtains the Operator's in-memory identity over the Gateway
// bootstrap websocket. Every start enrolls: no identity survives the process.
func enrollOperator(ctx context.Context, opts ServeOperatorOptions, cfg *config.Config, endpoint, workDir string, explicitTrust []byte, deployment *OperatorDeploymentRecorder, logger *slog.Logger) (*OperatorIdentity, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("resolve hostname: %w", err)
	}
	runtimeConfig, err := operatorRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}
	client, err := NewOperatorBootstrapClient(OperatorBootstrapClientConfig{
		URL:                buildOperatorBootstrapURL(endpoint, opts.HTTPPort),
		InstanceID:         operatorInstanceID(hostname, operatorRoles(opts), workDir),
		Hostname:           hostname,
		RuntimeConfig:      runtimeConfig,
		FingerprintOptions: operatorFingerprintOptions(opts, workDir, auth.ResolveCurrentAccount()),
		TrustBundlePEM:     explicitTrust,
		Deployment:         deployment,
		Logger:             logger,
	})
	if err != nil {
		return nil, err
	}
	logger.Info("Enrolling Operator over the bootstrap websocket", "endpoint", endpoint)
	return client.Enroll(ctx)
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
