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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"

	"google.golang.org/protobuf/proto"
)

// Operator enrollment protocol constants. The operator is the only
// component that submits two CSRs (operator + CLI) and signs the
// completion transcript with both private keys.
const (
	operatorEnrollDefaultDeadline = 30 * time.Minute
)

// OperatorEnrollmentResult is the resolved operator identity after a
// successful platform enrollment. The caller writes OperatorSessionID
// to the G8E_OPERATOR_SESSION_ID env var and sets OperatorID/Posture
// on the config before constructing the g8eo service.
type OperatorEnrollmentResult struct {
	OperatorCertPath  string
	OperatorKeyPath   string
	CLICertPath       string
	CLIKeyPath        string
	TrustBundlePath   string
	OperatorID        string
	OperatorSessionID string
	CLISessionID      string
	Posture           string
}

// operatorPendingState is the resumable pending enrollment attempt,
// persisted to pki/pending-enrollment/g8eo.json with 0600 permissions.
// The private keys and requester token are secret; the request ID,
// fingerprints, and expiry are not.
type operatorPendingState struct {
	RequestID           string    `json:"request_id"`
	Token               string    `json:"token"`
	OperatorFingerprint string    `json:"operator_fingerprint"`
	CLIFingerprint      string    `json:"cli_fingerprint"`
	OperatorKeyPEM      string    `json:"operator_key_pem"`
	CLIKeyPEM           string    `json:"cli_key_pem"`
	ExpiresAt           time.Time `json:"expires_at"`
	InstanceID          string    `json:"instance_id"`
	Hostname            string    `json:"hostname"`
	OperatorCSRPEM      string    `json:"operator_csr_pem"`
	CLICSRPEM           string    `json:"cli_csr_pem"`
	SystemFingerprint   string    `json:"system_fingerprint"`
}

// OperatorPlatformEnrollmentClient drives the owner-approved platform
// enrollment protocol for the operator component. It mirrors the
// ensemble Python client: the same
// nine-step resumable sequence, the same canonical completion
// transcript, and the same atomic credential writes.
//
// The caller (RunOperator) decides whether to load an existing identity
// or enroll. This client does not hide that decision behind an
// ensure* method.
type OperatorPlatformEnrollmentClient struct {
	gatewayHTTPURL  string
	instanceID      string
	hostname        string
	fileSvc         fs.RuntimeFileService
	logger          *slog.Logger
	fingerprintOpts auth.FingerprintOptions
	deployment      *OperatorDeploymentRecorder
}

// NewOperatorPlatformEnrollmentClient constructs an enrollment client.
// gatewayHTTPURL is the gateway's plain-HTTP bootstrap surface (e.g.
// http://g8eg:8080), with no trailing slash.
func NewOperatorPlatformEnrollmentClient(gatewayHTTPURL, instanceID, hostname string, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*OperatorPlatformEnrollmentClient, error) {
	if gatewayHTTPURL == "" {
		return nil, fmt.Errorf("%w: gateway HTTP URL is required for operator platform enrollment", constants.ErrInternal)
	}
	if instanceID == "" || hostname == "" {
		return nil, fmt.Errorf("%w: instance ID and hostname are required for operator platform enrollment", constants.ErrInternal)
	}
	return &OperatorPlatformEnrollmentClient{
		gatewayHTTPURL: trimTrailingSlash(gatewayHTTPURL),
		instanceID:     instanceID,
		hostname:       hostname,
		fileSvc:        fileSvc,
		logger:         logger,
	}, nil
}

// SetFingerprintOptions sets the options that differentiate operators on the same system.
func (c *OperatorPlatformEnrollmentClient) SetFingerprintOptions(opts auth.FingerprintOptions) {
	c.fingerprintOpts = opts
}

// SetDeploymentRecorder makes the client publish non-secret progress for a
// deploying CLI. Without it, enrollment records nothing.
func (c *OperatorPlatformEnrollmentClient) SetDeploymentRecorder(recorder *OperatorDeploymentRecorder) {
	c.deployment = recorder
}

func (c *OperatorPlatformEnrollmentClient) recordDeployment(ctx context.Context, state models.OperatorDeploymentState) error {
	if c.deployment == nil {
		return nil
	}
	return c.deployment.Record(ctx, state)
}

// Enroll performs the full nine-step platform enrollment sequence and
// returns the resolved operator identity. If a resumable pending
// attempt exists on disk, it resumes from that state rather than
// generating new keys. The context controls cancellation; on
// cancellation the pending state is left on disk so a restart can
// resume the same request.
func (c *OperatorPlatformEnrollmentClient) Enroll(ctx context.Context) (*OperatorEnrollmentResult, error) {
	pendingPath := c.pendingStatePath()

	// Step 2: Load persisted pending attempt if it exists.
	pending, err := c.loadPendingState(pendingPath)
	if err != nil {
		return nil, err
	}

	var (
		token, requestID          string
		operatorFP, cliFP         string
		operatorKeyPEM, cliKeyPEM string
		operatorKey, cliKey       *ecdsa.PrivateKey
	)

	if pending != nil && !pending.ExpiresAt.IsZero() && !time.Now().Before(pending.ExpiresAt) {
		c.logger.Info("operator enrollment: pending attempt expired; starting fresh", "request_id", pending.RequestID)
		_ = c.removePendingState(pendingPath)
		pending = nil
	}

	if pending != nil && pending.Token != "" {
		// Resume the existing pending attempt. Do not generate new keys.
		token = pending.Token
		requestID = pending.RequestID
		operatorFP = pending.OperatorFingerprint
		cliFP = pending.CLIFingerprint
		operatorKeyPEM = pending.OperatorKeyPEM
		cliKeyPEM = pending.CLIKeyPEM
		operatorKey, err = parseECPrivateKeyPEM(operatorKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: resume operator key: %w", err)
		}
		cliKey, err = parseECPrivateKeyPEM(cliKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: resume cli key: %w", err)
		}
		if requestID != "" {
			c.logger.Info("operator enrollment: resuming pending attempt", "request_id", requestID)
		} else {
			createResp, submitErr := c.submitRequest(ctx, pending.OperatorCSRPEM, pending.CLICSRPEM, pending.SystemFingerprint, token)
			if submitErr != nil {
				return nil, submitErr
			}
			requestID = createResp.RequestID
			pending.RequestID = requestID
			pending.ExpiresAt = createResp.ExpiresAt
			if err := c.persistPendingState(pendingPath, pending); err != nil {
				return nil, err
			}
		}
	} else {
		// Step 3: Generate keys and submit a new request.
		operatorCSR, opKey, err := GenerateCSR(fmt.Sprintf("g8e-operator-%s", c.hostname))
		if err != nil {
			return nil, err
		}
		cliCSR, cliK, err := GenerateCSR(fmt.Sprintf("g8e-cli-%s", c.hostname))
		if err != nil {
			return nil, err
		}
		operatorKey = opKey
		cliKey = cliK

		operatorFP, err = csrFingerprint(operatorCSR)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: operator csr fingerprint: %w", err)
		}
		cliFP, err = csrFingerprint(cliCSR)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: cli csr fingerprint: %w", err)
		}

		operatorKeyPEM, err = encodeECPrivateKeyPEM(operatorKey)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: encode operator key: %w", err)
		}
		cliKeyPEM, err = encodeECPrivateKeyPEM(cliKey)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: encode cli key: %w", err)
		}

		systemFp, err := auth.GenerateOperatorFingerprint(c.logger, c.fingerprintOpts)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: system fingerprint: %w", err)
		}

		token, err = models.NewPlatformEnrollmentToken()
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: generate request token: %w", err)
		}

		// Persist pending state atomically with 0600 permissions.
		pending = &operatorPendingState{
			RequestID:           requestID,
			Token:               token,
			OperatorFingerprint: operatorFP,
			CLIFingerprint:      cliFP,
			OperatorKeyPEM:      operatorKeyPEM,
			CLIKeyPEM:           cliKeyPEM,
			InstanceID:          c.instanceID,
			Hostname:            c.hostname,
			OperatorCSRPEM:      operatorCSR,
			CLICSRPEM:           cliCSR,
			SystemFingerprint:   systemFp.Fingerprint,
		}
		if err := c.persistPendingState(pendingPath, pending); err != nil {
			return nil, err
		}
		createResp, err := c.submitRequest(ctx, operatorCSR, cliCSR, systemFp.Fingerprint, token)
		if err != nil {
			return nil, err
		}
		requestID = createResp.RequestID
		pending.RequestID = requestID
		pending.ExpiresAt = createResp.ExpiresAt
		if err := c.persistPendingState(pendingPath, pending); err != nil {
			return nil, err
		}

		// Step 4: Print non-secret approval instructions.
		c.logger.Info("operator enrollment: request submitted", "request_id", requestID)
		c.logger.Info("operator enrollment: operator CSR fingerprint", "fingerprint", operatorFP)
		c.logger.Info("operator enrollment: CLI CSR fingerprint", "fingerprint", cliFP)
		if createResp.ApprovalURL != "" {
			c.logger.Info("operator enrollment: approval URL", "url", createResp.ApprovalURL)
		}
	}
	fmt.Fprintf(os.Stderr, "Approve with: g8e auth enroll approve %s\n", requestID)
	if err := c.recordDeployment(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhasePendingApproval, RequestID: requestID}); err != nil {
		return nil, err
	}

	// Step 5: Hold one status request until the owner decides.
	deadline := operatorEnrollDefaultDeadline
	if pending != nil && !pending.ExpiresAt.IsZero() {
		deadline = time.Until(pending.ExpiresAt)
	}
	if err := c.awaitApproval(ctx, token, deadline); err != nil {
		return nil, err
	}

	// Step 6: Sign the completion transcript with both private keys.
	tokenHashValue := models.PlatformEnrollmentTokenHash(token)
	transcript, err := buildOperatorCompletionTranscript(requestID, tokenHashValue, c.instanceID, operatorFP, cliFP)
	if err != nil {
		return nil, err
	}
	operatorProof, err := signTranscript(operatorKey, transcript)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: sign operator proof: %w", err)
	}
	cliProof, err := signTranscript(cliKey, transcript)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: sign cli proof: %w", err)
	}

	// Step 7: Submit completion and validate the response.
	completionResp, err := c.submitCompletion(ctx, token, operatorProof, cliProof)
	if err != nil {
		return nil, err
	}
	if completionResp.Operator == nil {
		return nil, fmt.Errorf("operator enrollment: completion response missing operator credentials")
	}
	creds := completionResp.Operator
	if creds.OperatorCert == "" || creds.CLICert == "" {
		return nil, fmt.Errorf("operator enrollment: completion response missing certificates")
	}

	// Step 8: Write credentials atomically, then remove pending state.
	if err := c.writeCredentials(creds, operatorKeyPEM, cliKeyPEM); err != nil {
		return nil, err
	}
	if err := c.removePendingState(pendingPath); err != nil {
		c.logger.Warn("operator enrollment: failed to remove pending state", "error", err)
	}

	c.logger.Info("operator enrollment: completed",
		"operator_id", creds.OperatorID,
		"operator_session_id", creds.OperatorSessionID,
		"cli_session_id", creds.CLISessionID,
	)

	if err := c.recordDeployment(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhaseEnrolled, OperatorSessionID: creds.OperatorSessionID}); err != nil {
		return nil, err
	}

	// Step 9: Return the resolved identity. Paths are relative to the
	// runtime tree root; the caller loads them via the fileSvc-aware
	// cert loader (loadClientCertPairViaFileSvc), not os.ReadFile.
	return &OperatorEnrollmentResult{
		OperatorCertPath:  c.operatorCertPath(),
		OperatorKeyPath:   c.operatorKeyPath(),
		CLICertPath:       c.cliCertPath(),
		CLIKeyPath:        c.cliKeyPath(),
		TrustBundlePath:   c.trustBundlePath(),
		OperatorID:        creds.OperatorID,
		OperatorSessionID: creds.OperatorSessionID,
		CLISessionID:      creds.CLISessionID,
		Posture:           creds.Posture,
	}, nil
}

// --- Path resolution ---

func (c *OperatorPlatformEnrollmentClient) pendingStatePath() string {
	return filepath.Join(constants.PkiDirname, constants.PkiSubdirPendingEnroll, constants.PendingEnrollmentFileOperator)
}

func (c *OperatorPlatformEnrollmentClient) operatorCertPath() string {
	return filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)
}

func (c *OperatorPlatformEnrollmentClient) operatorKeyPath() string {
	return filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)
}

func (c *OperatorPlatformEnrollmentClient) cliCertPath() string {
	return filepath.Join(constants.PkiDirname, constants.CliCertFilename)
}

func (c *OperatorPlatformEnrollmentClient) cliKeyPath() string {
	return filepath.Join(constants.PkiDirname, constants.CliKeyFilename)
}

func (c *OperatorPlatformEnrollmentClient) trustBundlePath() string {
	return filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
}

// --- HTTP ---

func (c *OperatorPlatformEnrollmentClient) submitRequest(ctx context.Context, operatorCSR, cliCSR, systemFingerprint, token string) (*models.PlatformEnrollmentCreateResponse, error) {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentRequest
	payload := models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        c.instanceID,
		Hostname:          c.hostname,
		SystemFingerprint: systemFingerprint,
		DeploymentID:      c.deployment.LaunchID(),
		TokenHash:         models.PlatformEnrollmentTokenHash(token),
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: operatorCSR,
			CLICSRPEM:      cliCSR,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doHTTPRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: submit request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: read response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusConflict {
			_ = c.removePendingState(c.pendingStatePath())
			return nil, fmt.Errorf("operator enrollment: pending request conflicts with gateway state (HTTP 409); pending state cleared, start enrollment again to create a fresh request")
		}
		return nil, fmt.Errorf("operator enrollment: request rejected: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var createResp models.PlatformEnrollmentCreateResponse
	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return nil, fmt.Errorf("operator enrollment: parse response: %w", err)
	}
	return &createResp, nil
}

func (c *OperatorPlatformEnrollmentClient) awaitApproval(ctx context.Context, token string, deadline time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline <= 0 {
		_ = c.removePendingState(c.pendingStatePath())
		return fmt.Errorf("operator enrollment: approval deadline reached before approval")
	}
	waitCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentStatus + "?wait=true&token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(waitCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("operator enrollment: create status request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-store")
	resp, err := doHTTPRequest(waitCtx, req)
	if err != nil {
		return fmt.Errorf("operator enrollment: await approval: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("operator enrollment: read status response: %w", err)
	}
	if resp.StatusCode == http.StatusGone {
		_ = c.removePendingState(c.pendingStatePath())
		return fmt.Errorf("operator enrollment: request has expired (HTTP 410)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("operator enrollment: status query failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var statusResp models.PlatformEnrollmentStatusResponse
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		return fmt.Errorf("operator enrollment: parse status response: %w", err)
	}
	switch statusResp.State {
	case models.PlatformEnrollmentStateApproved, models.PlatformEnrollmentStateCompleted:
		return nil
	case models.PlatformEnrollmentStateDenied:
		return fmt.Errorf("operator enrollment: request was denied by the owner")
	case models.PlatformEnrollmentStateExpired:
		_ = c.removePendingState(c.pendingStatePath())
		return fmt.Errorf("operator enrollment: request has expired")
	default:
		return fmt.Errorf("operator enrollment: unexpected approval state %s", statusResp.State)
	}
}

func (c *OperatorPlatformEnrollmentClient) submitCompletion(ctx context.Context, token, operatorProof, cliProof string) (*models.PlatformEnrollmentCompleteResponse, error) {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentComplete
	payload := models.PlatformEnrollmentCompleteRequest{
		Token: token,
		Proofs: models.PlatformEnrollmentProofs{
			Operator: operatorProof,
			CLI:      cliProof,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: marshal completion: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: create completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")

	resp, err := doHTTPRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: submit completion: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: read completion response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("operator enrollment: completion rejected: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var completionResp models.PlatformEnrollmentCompleteResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("operator enrollment: parse completion response: %w", err)
	}
	return &completionResp, nil
}

// --- Credential writes ---

func (c *OperatorPlatformEnrollmentClient) writeCredentials(creds *models.PlatformEnrollmentOperatorCredentials, operatorKeyPEM, cliKeyPEM string) error {
	ctx := context.Background()

	// Operator cert + chain.
	operatorCertContent := creds.OperatorCert
	if creds.OperatorCertChain != "" {
		operatorCertContent = operatorCertContent + "\n" + creds.OperatorCertChain
	}
	if err := c.atomicWrite(ctx, c.operatorCertPath(), []byte(operatorCertContent), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator enrollment: write operator cert: %w", err)
	}
	if err := c.atomicWrite(ctx, c.operatorKeyPath(), []byte(operatorKeyPEM), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator enrollment: write operator key: %w", err)
	}

	// CLI cert + chain.
	cliCertContent := creds.CLICert
	if creds.CLICertChain != "" {
		cliCertContent = cliCertContent + "\n" + creds.CLICertChain
	}
	if err := c.atomicWrite(ctx, c.cliCertPath(), []byte(cliCertContent), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator enrollment: write cli cert: %w", err)
	}
	if err := c.atomicWrite(ctx, c.cliKeyPath(), []byte(cliKeyPEM), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("operator enrollment: write cli key: %w", err)
	}

	// Preserve installed trust (including an explicitly authorized recovery CA).
	// Enrollment over the bootstrap surface must not replace a pinned CA.
	if creds.HubTrustBundle != "" {
		exists, err := c.fileSvc.FileExists(ctx, c.trustBundlePath())
		if err != nil {
			return fmt.Errorf("operator enrollment: check pinned trust: %w", err)
		}
		if !exists {
			if err := c.atomicWrite(ctx, c.trustBundlePath(), []byte(creds.HubTrustBundle), constants.PermFilePublic); err != nil {
				return fmt.Errorf("operator enrollment: write trust bundle: %w", err)
			}
		}
	}

	// Actuator public key (if issued).
	if creds.ActuatorKeyID != "" && creds.ActuatorPubKey != "" {
		signersDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners)
		if err := c.fileSvc.MkdirAll(ctx, signersDir, constants.PermDirPrivate); err != nil {
			return fmt.Errorf("operator enrollment: create trusted_signers dir: %w", err)
		}
		signerPath := filepath.Join(signersDir, creds.ActuatorKeyID+constants.PublicKeySuffix)
		if err := c.atomicWrite(ctx, signerPath, []byte(creds.ActuatorPubKey), constants.PermFilePrivate); err != nil {
			return fmt.Errorf("operator enrollment: write actuator pub key: %w", err)
		}
	}

	c.logger.Info("operator enrollment: credentials saved",
		"operator_cert", c.fileSvc.Resolve(c.operatorCertPath()),
		"cli_cert", c.fileSvc.Resolve(c.cliCertPath()),
		"trust_bundle", c.fileSvc.Resolve(c.trustBundlePath()),
	)
	return nil
}

// atomicWrite writes data to a relative path under the runtime tree
// using temp-file-plus-rename for atomicity.
func (c *OperatorPlatformEnrollmentClient) atomicWrite(ctx context.Context, relPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(relPath)
	if err := c.fileSvc.MkdirAll(ctx, dir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := c.fileSvc.WriteFile(ctx, relPath, data, perm); err != nil {
		return err
	}
	return nil
}

// --- Pending state ---

func (c *OperatorPlatformEnrollmentClient) persistPendingState(relPath string, state *operatorPendingState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("operator enrollment: marshal pending state: %w", err)
	}
	return c.atomicWrite(context.Background(), relPath, data, constants.PermFilePrivate)
}

func (c *OperatorPlatformEnrollmentClient) loadPendingState(relPath string) (*operatorPendingState, error) {
	exists, err := c.fileSvc.FileExists(context.Background(), relPath)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: check pending state: %w", err)
	}
	if !exists {
		return nil, nil
	}
	data, err := c.fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: read pending state: %w", err)
	}
	var state operatorPendingState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("operator enrollment: parse pending state: %w", err)
	}
	return &state, nil
}

func (c *OperatorPlatformEnrollmentClient) removePendingState(relPath string) error {
	return c.fileSvc.Remove(context.Background(), relPath)
}

// --- Transcript construction and signing ---

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

// --- CSR fingerprint ---

// csrFingerprint computes the SHA-256 fingerprint of the public key in
// a CSR PEM.
func csrFingerprint(csrPEM string) (string, error) {
	return auth.CSRFingerprint(csrPEM)
}

// --- Helpers ---

func parseECPrivateKeyPEM(keyPEM string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("parse EC private key: no PEM block found")
	}
	var keyBytes []byte
	switch block.Type {
	case "EC PRIVATE KEY":
		keyBytes = block.Bytes
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		ecKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("parse PKCS8 private key: not an EC key")
		}
		return ecKey, nil
	default:
		return nil, fmt.Errorf("parse EC private key: unexpected PEM type %q", block.Type)
	}
	key, err := x509.ParseECPrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse EC private key: %w", err)
	}
	return key, nil
}

func encodeECPrivateKeyPEM(key *ecdsa.PrivateKey) (string, error) {
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
	return string(pemBytes), nil
}

func doHTTPRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := &http.Client{}
	return client.Do(req.WithContext(ctx))
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
