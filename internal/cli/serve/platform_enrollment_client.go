// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Bootstrap redial backoff after a dropped socket or a transient Gateway
// error. The request, token, and keys are reused on every redial.
const (
	operatorBootstrapRedialBase = time.Second
	operatorBootstrapRedialMax  = 30 * time.Second
)

// OperatorIdentity is the enrolled Operator's identity and runtime grant. It
// lives in process memory only and is never written to disk: a stop or reboot
// loses it, and the Operator must enroll again.
type OperatorIdentity struct {
	Certificate        tls.Certificate
	CertificatePEM     []byte
	TrustBundlePEM     []byte
	OperatorID         string
	OperatorSessionID  string
	Posture            string
	SystemFingerprint  string
	MaxConcurrentTasks int
	MaxMemoryMB        int
}

// OperatorBootstrapClientConfig configures one Operator enrollment.
type OperatorBootstrapClientConfig struct {
	// URL is the Gateway bootstrap websocket (buildOperatorBootstrapURL).
	URL        string
	InstanceID string
	Hostname   string
	// RuntimeConfig is the canonical JSON OperatorRuntimeConfig the Gateway
	// records for the issued Operator (operatorRuntimeConfig).
	RuntimeConfig      json.RawMessage
	FingerprintOptions auth.FingerprintOptions
	// TrustBundlePEM, when set, is the owner-supplied CA a wss:// bootstrap
	// URL must chain to; otherwise the system roots apply.
	TrustBundlePEM []byte
	Deployment     *OperatorDeploymentRecorder
	Logger         *slog.Logger
}

// OperatorBootstrapClient enrolls the Operator over the Gateway bootstrap
// websocket (models.OperatorBootstrapFrameType). It is the Operator's only
// enrollment path: no HTTP request is made, and nothing it generates or
// receives touches disk.
type OperatorBootstrapClient struct {
	cfg    OperatorBootstrapClientConfig
	dialer *websocket.Dialer
}

func NewOperatorBootstrapClient(cfg OperatorBootstrapClientConfig) (*OperatorBootstrapClient, error) {
	if cfg.URL == "" || cfg.InstanceID == "" || cfg.Hostname == "" || len(cfg.RuntimeConfig) == 0 ||
		cfg.Deployment == nil || cfg.Logger == nil {
		return nil, fmt.Errorf("%w: operator bootstrap requires a URL, instance ID, hostname, runtime config, deployment recorder, and logger", constants.ErrInternal)
	}
	var tlsCfg *tls.Config
	if len(cfg.TrustBundlePEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.TrustBundlePEM) {
			return nil, fmt.Errorf("%w: operator bootstrap trust bundle", constants.ErrCAParseFailed)
		}
		tlsCfg = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &OperatorBootstrapClient{cfg: cfg, dialer: httpclient.WebSocketDialerWithTLS(tlsCfg)}, nil
}

// operatorBootstrapAttempt is the in-memory enrollment request, reused on
// every redial so the Gateway resumes it instead of creating another.
type operatorBootstrapAttempt struct {
	request      models.OperatorBootstrapRequest
	operatorKey  *ecdsa.PrivateKey
	cliKey       *ecdsa.PrivateKey
	fingerprints models.PlatformEnrollmentCSRFingerprints
	requestID    string
}

// operatorBootstrapRetry marks a failure the same attempt survives by
// redialing: a dropped socket, an unreachable Gateway, or a transient
// Gateway error frame.
type operatorBootstrapRetry struct{ err error }

func (e *operatorBootstrapRetry) Error() string { return e.err.Error() }
func (e *operatorBootstrapRetry) Unwrap() error { return e.err }

// Enroll creates the enrollment request, holds the bootstrap socket through
// the owner's decision, proves possession of both keys, and returns the
// delivered identity. Only the context, a denial, an expiry, or a permanent
// Gateway error ends it; everything else redials with the same request.
func (c *OperatorBootstrapClient) Enroll(ctx context.Context) (*OperatorIdentity, error) {
	attempt, err := c.newAttempt()
	if err != nil {
		return nil, err
	}
	delay := operatorBootstrapRedialBase
	for {
		identity, err := c.exchange(ctx, attempt)
		if err == nil {
			return identity, nil
		}
		var retry *operatorBootstrapRetry
		if ctx.Err() != nil || !errors.As(err, &retry) {
			return nil, err
		}
		c.cfg.Logger.Warn("operator enrollment: bootstrap socket ended; redialing",
			"request_id", attempt.requestID, "retry_in", delay, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		delay = min(2*delay, operatorBootstrapRedialMax)
	}
}

func (c *OperatorBootstrapClient) newAttempt() (*operatorBootstrapAttempt, error) {
	operatorCSR, operatorKey, err := GenerateCSR("g8e-operator-" + c.cfg.Hostname)
	if err != nil {
		return nil, err
	}
	cliCSR, cliKey, err := GenerateCSR("g8e-cli-" + c.cfg.Hostname)
	if err != nil {
		return nil, err
	}
	operatorFP, err := auth.CSRFingerprint(operatorCSR)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: operator csr fingerprint: %w", err)
	}
	cliFP, err := auth.CSRFingerprint(cliCSR)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: cli csr fingerprint: %w", err)
	}
	systemFingerprint, err := auth.GenerateOperatorFingerprint(c.cfg.Logger, c.cfg.FingerprintOptions)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: system fingerprint: %w", err)
	}
	token, err := models.NewPlatformEnrollmentToken()
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: generate request token: %w", err)
	}
	return &operatorBootstrapAttempt{
		request: models.OperatorBootstrapRequest{
			Token: token,
			Enrollment: models.PlatformEnrollmentCreateRequest{
				ComponentKind:     models.PlatformComponentOperator,
				InstanceID:        c.cfg.InstanceID,
				Hostname:          c.cfg.Hostname,
				SystemFingerprint: systemFingerprint.Fingerprint,
				DeploymentID:      c.cfg.Deployment.LaunchID(),
				TokenHash:         models.PlatformEnrollmentTokenHash(token),
				Operator: &models.PlatformOperatorCSRPayload{
					OperatorCSRPEM: operatorCSR,
					CLICSRPEM:      cliCSR,
				},
			},
			RuntimeConfig: c.cfg.RuntimeConfig,
		},
		operatorKey:  operatorKey,
		cliKey:       cliKey,
		fingerprints: models.PlatformEnrollmentCSRFingerprints{Operator: operatorFP, CLI: cliFP},
	}, nil
}

// exchange runs one bootstrap socket to the bundle. Transport failures are
// retryable; the Gateway's error frame says whether its failure is.
func (c *OperatorBootstrapClient) exchange(ctx context.Context, attempt *operatorBootstrapAttempt) (*OperatorIdentity, error) {
	conn, resp, err := c.dialer.DialContext(ctx, c.cfg.URL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if errors.Is(err, websocket.ErrBadHandshake) {
		return nil, fmt.Errorf("operator enrollment: Gateway refused the bootstrap websocket (HTTP %d): %w", resp.StatusCode, err)
	}
	if err != nil {
		return nil, &operatorBootstrapRetry{fmt.Errorf("operator enrollment: dial %s: %w", c.cfg.URL, err)}
	}
	defer conn.Close()
	// The socket is held for the owner's decision; cancellation closes it.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := conn.WriteJSON(models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameRequest, Request: &attempt.request}); err != nil {
		return nil, &operatorBootstrapRetry{fmt.Errorf("operator enrollment: send request: %w", err)}
	}

	created, err := readOperatorBootstrapFrame(conn, models.OperatorBootstrapFrameCreated)
	if err != nil {
		return nil, err
	}
	if err := c.observeCreated(ctx, attempt, created.Created); err != nil {
		return nil, err
	}

	decided, err := readOperatorBootstrapFrame(conn, models.OperatorBootstrapFrameDecision)
	if err != nil {
		return nil, err
	}
	if decided.Decision == nil {
		return nil, fmt.Errorf("operator enrollment: decision frame missing its decision")
	}
	switch decided.Decision.State {
	case models.PlatformEnrollmentStateApproved, models.PlatformEnrollmentStateIssuing, models.PlatformEnrollmentStateCompleted:
	case models.PlatformEnrollmentStateDenied:
		return nil, fmt.Errorf("operator enrollment: request %s was denied by the owner", attempt.requestID)
	case models.PlatformEnrollmentStateExpired:
		return nil, fmt.Errorf("operator enrollment: request %s expired before approval", attempt.requestID)
	default:
		return nil, fmt.Errorf("operator enrollment: request %s ended in state %s", attempt.requestID, decided.Decision.State)
	}

	proofs, err := attempt.proofs()
	if err != nil {
		return nil, err
	}
	if err := conn.WriteJSON(models.OperatorBootstrapFrame{Type: models.OperatorBootstrapFrameComplete, Complete: proofs}); err != nil {
		return nil, &operatorBootstrapRetry{fmt.Errorf("operator enrollment: send completion: %w", err)}
	}

	delivered, err := readOperatorBootstrapFrame(conn, models.OperatorBootstrapFrameBundle)
	if err != nil {
		return nil, err
	}
	identity, err := attempt.identity(delivered.Bundle)
	if err != nil {
		return nil, err
	}
	c.cfg.Logger.Info("operator enrollment: completed",
		"request_id", attempt.requestID,
		"operator_id", identity.OperatorID,
		"operator_session_id", identity.OperatorSessionID)
	if err := c.cfg.Deployment.Record(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhaseEnrolled, OperatorSessionID: identity.OperatorSessionID}); err != nil {
		return nil, err
	}
	return identity, nil
}

// observeCreated announces the request once; a redial resumes the same one.
func (c *OperatorBootstrapClient) observeCreated(ctx context.Context, attempt *operatorBootstrapAttempt, created *models.PlatformEnrollmentCreateResponse) error {
	if created == nil || created.RequestID == "" {
		return fmt.Errorf("operator enrollment: created frame missing its request")
	}
	if created.Fingerprints.Operator != attempt.fingerprints.Operator || created.Fingerprints.CLI != attempt.fingerprints.CLI {
		return fmt.Errorf("operator enrollment: request %s does not carry this Operator's keys", created.RequestID)
	}
	if attempt.requestID == created.RequestID {
		return nil
	}
	if attempt.requestID != "" {
		return fmt.Errorf("operator enrollment: Gateway resumed request %s, expected %s", created.RequestID, attempt.requestID)
	}
	attempt.requestID = created.RequestID
	c.cfg.Logger.Info("operator enrollment: request submitted",
		"request_id", created.RequestID,
		"operator_fingerprint", attempt.fingerprints.Operator,
		"cli_fingerprint", attempt.fingerprints.CLI,
		"approval_url", created.ApprovalURL,
		"expires_at", created.ExpiresAt)
	fmt.Fprintf(os.Stderr, "Approve with: g8e auth enroll approve %s\n", created.RequestID)
	return c.cfg.Deployment.Record(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhasePendingApproval, RequestID: created.RequestID})
}

// readOperatorBootstrapFrame reads the next frame and requires want. A read
// failure is a dropped socket (retryable); an error frame carries the
// Gateway's own verdict.
func readOperatorBootstrapFrame(conn *websocket.Conn, want models.OperatorBootstrapFrameType) (models.OperatorBootstrapFrame, error) {
	var frame models.OperatorBootstrapFrame
	if err := conn.ReadJSON(&frame); err != nil {
		return frame, &operatorBootstrapRetry{fmt.Errorf("operator enrollment: await %s: %w", want, err)}
	}
	if frame.Type == models.OperatorBootstrapFrameError {
		if frame.Error == nil {
			return frame, fmt.Errorf("operator enrollment: Gateway ended the exchange without a reason")
		}
		err := fmt.Errorf("operator enrollment: Gateway: %s", frame.Error.Message)
		if frame.Error.Retryable {
			return frame, &operatorBootstrapRetry{err}
		}
		return frame, err
	}
	if frame.Type != want {
		return frame, fmt.Errorf("operator enrollment: expected %s frame, got %q", want, frame.Type)
	}
	return frame, nil
}

// proofs signs the canonical completion transcript with both private keys.
func (a *operatorBootstrapAttempt) proofs() (*models.PlatformEnrollmentProofs, error) {
	transcript, err := buildOperatorCompletionTranscript(a.requestID, a.request.Enrollment.TokenHash,
		a.request.Enrollment.InstanceID, a.fingerprints.Operator, a.fingerprints.CLI)
	if err != nil {
		return nil, err
	}
	operatorProof, err := signTranscript(a.operatorKey, transcript)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: sign operator proof: %w", err)
	}
	cliProof, err := signTranscript(a.cliKey, transcript)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: sign cli proof: %w", err)
	}
	return &models.PlatformEnrollmentProofs{Operator: operatorProof, CLI: cliProof}, nil
}

// identity pairs the delivered Operator certificate with the in-memory key.
// The companion CLI certificate is not retained: the Operator never uses it.
func (a *operatorBootstrapAttempt) identity(bundle *models.OperatorBootstrapBundle) (*OperatorIdentity, error) {
	if bundle == nil || bundle.Credentials.OperatorCert == "" || bundle.Credentials.HubTrustBundle == "" ||
		bundle.Credentials.OperatorID == "" || bundle.Credentials.OperatorSessionID == "" {
		return nil, fmt.Errorf("operator enrollment: bundle missing the Operator certificate, trust, or session")
	}
	creds := bundle.Credentials
	certPEM := creds.OperatorCert
	if creds.OperatorCertChain != "" {
		certPEM += "\n" + creds.OperatorCertChain
	}
	keyDER, err := x509.MarshalECPrivateKey(a.operatorKey)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: encode operator key: %w", err)
	}
	cert, err := tls.X509KeyPair([]byte(certPEM), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrLoadCertKeyPair, err)
	}
	return &OperatorIdentity{
		Certificate:        cert,
		CertificatePEM:     []byte(certPEM),
		TrustBundlePEM:     []byte(creds.HubTrustBundle),
		OperatorID:         creds.OperatorID,
		OperatorSessionID:  creds.OperatorSessionID,
		Posture:            creds.Posture,
		SystemFingerprint:  a.request.Enrollment.SystemFingerprint,
		MaxConcurrentTasks: bundle.MaxConcurrentTasks,
		MaxMemoryMB:        bundle.MaxMemoryMB,
	}, nil
}

// operatorRuntimeConfig is the runtime configuration the Operator declares
// at enrollment; the Gateway records it for the issued Operator.
func operatorRuntimeConfig(cfg *config.Config) (json.RawMessage, error) {
	data, err := models.MarshalOperatorRuntimeConfig(&operatorv1.OperatorRuntimeConfig{
		CloudMode:           cfg.CloudMode,
		CloudProvider:       cfg.CloudProvider,
		LocalStorageEnabled: cfg.ExecutionVaultEnabled,
		NoGit:               cfg.NoGit,
		LogLevel:            cfg.LogLevel,

		HttpPort: int32(cfg.HTTPPort),
		Roles:    models.OperatorRolesToProto(cfg.EffectiveOperatorRoles()),
		LocalDir: cfg.WorkDir,
		Account:  auth.ResolveCurrentAccount(),

		InferenceEnabled:                   cfg.Inference.Enabled,
		InferenceOllamaEndpoint:            cfg.Inference.OllamaEndpoint,
		ProviderBoundaryObserverEnabled:    cfg.ProviderBoundaryObserver.Enabled,
		ProvenanceOperatorEnabled:          cfg.ProvenanceOperator.Enabled,
		ProvenanceOperatorModelStorageRoot: cfg.ProvenanceOperator.ModelStorageRoot,
		Platform:                           runtime.GOOS,
		HeartbeatIntervalMs:                uint32(cfg.HeartbeatInterval.Milliseconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrOperatorRuntimeConfigMarshal, err)
	}
	return data, nil
}

// buildOperatorBootstrapURL returns the Gateway bootstrap websocket URL for
// an endpoint flag value. An http(s) scheme maps to ws(s); an endpoint port
// wins, otherwise httpPort (or the Operator HTTP default when zero) is used.
func buildOperatorBootstrapURL(endpoint string, httpPort int) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	var base string
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		base = "wss://" + strings.TrimPrefix(endpoint, "https://")
	case strings.HasPrefix(endpoint, "http://"):
		base = "ws://" + strings.TrimPrefix(endpoint, "http://")
	case strings.Contains(endpoint, "://"):
		base = endpoint
	default:
		if _, _, err := net.SplitHostPort(endpoint); err != nil {
			if httpPort == 0 {
				httpPort = constants.Ports.OperatorHttp
			}
			endpoint = net.JoinHostPort(endpoint, strconv.Itoa(httpPort))
		}
		base = "ws://" + endpoint
	}
	return strings.TrimRight(base, "/") + constants.APIPaths.AuthOperatorBootstrapWebSocket
}

// buildOperatorCompletionTranscript constructs the canonical
// PlatformEnrollmentCompletionTranscript as deterministic protobuf,
// matching the gateway's platformEnrollmentCompletionTranscript. The
// operator transcript includes both the operator and CLI fingerprints.
func buildOperatorCompletionTranscript(requestID, tokenHash, instanceID, operatorFP, cliFP string) ([]byte, error) {
	message := &commonv1.PlatformEnrollmentCompletionTranscript{
		ProtocolVersion: constants.PlatformEnrollmentProtocolVersion,
		RequestId:       requestID,
		TokenHash:       tokenHash,
		ComponentKind:   commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR,
		InstanceId:      instanceID,
		Fingerprints: &commonv1.PlatformEnrollmentFingerprints{
			Operator: operatorFP,
			Cli:      cliFP,
		},
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: marshal completion transcript: %w", err)
	}
	return encoded, nil
}

// signTranscript signs the SHA-256 digest of the transcript with the
// private key and returns the base64url-encoded ASN.1 DER signature.
// Go's ecdsa.SignASN1 produces ASN.1 DER directly (no raw R||S
// conversion needed, unlike WebCrypto's raw R||S signatures).
func signTranscript(privateKey *ecdsa.PrivateKey, transcript []byte) (string, error) {
	digest := sha256.Sum256(transcript)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign transcript: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(signature), nil
}
