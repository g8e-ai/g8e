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
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math"
	mathrand "math/rand/v2"
	"net/http"

	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/auth"
	"github.com/g8e-ai/g8e/internal/services/fs"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"

	"google.golang.org/protobuf/proto"
)

// Operator enrollment protocol constants. The operator is the only
// component that submits two CSRs (operator + CLI) and signs the
// completion transcript with both private keys.
const (
	operatorEnrollHTTPTimeout     = 10 * time.Second
	operatorEnrollPollInitial     = 2 * time.Second
	operatorEnrollPollMax         = 30 * time.Second
	operatorEnrollPollJitter      = 500 * time.Millisecond
	operatorEnrollDefaultDeadline = 30 * time.Minute

	// Request submission retry. The gateway starts with zero users; workloads
	// start immediately after the gateway becomes healthy and may submit their
	// enrollment request before the owner has bootstrapped the first user. The
	// gateway returns 403 "platform enrollment requires a bootstrapped gateway"
	// until bootstrap. The client retries with bounded backoff so the workload
	// waits for bootstrap without exiting.
	operatorEnrollSubmitInitial  = 3 * time.Second
	operatorEnrollSubmitMax      = 30 * time.Second
	operatorEnrollSubmitJitter   = 1 * time.Second
	operatorEnrollSubmitDeadline = 30 * time.Minute
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
}

// OperatorPlatformEnrollmentClient drives the owner-approved platform
// enrollment protocol for the operator component. It mirrors the
// dashboard JS client and the ensemble Python client: the same
// nine-step resumable sequence, the same canonical completion
// transcript, and the same atomic credential writes.
//
// The caller (RunOperator) decides whether to load an existing identity
// or enroll. This client does not hide that decision behind an
// ensure* method.
type OperatorPlatformEnrollmentClient struct {
	gatewayHTTPURL string
	instanceID     string
	hostname       string
	fileSvc        fs.RuntimeFileService
	logger         *slog.Logger
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

	if pending != nil && pending.Token != "" && pending.RequestID != "" {
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
		c.logger.Info("operator enrollment: resuming pending attempt", "request_id", requestID)
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

		systemFp, err := auth.GenerateSystemFingerprint(c.logger)
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: system fingerprint: %w", err)
		}

		createResp, err := c.submitRequest(ctx, operatorCSR, cliCSR, systemFp.Fingerprint)
		if err != nil {
			return nil, err
		}
		requestID = createResp.RequestID
		token = createResp.Token
		if token == "" {
			return nil, fmt.Errorf("operator enrollment: gateway returned a deduplicated response with no token; a pending state file is required to resume. Request ID: %s", requestID)
		}

		// Persist pending state atomically with 0600 permissions.
		pending = &operatorPendingState{
			RequestID:           requestID,
			Token:               token,
			OperatorFingerprint: operatorFP,
			CLIFingerprint:      cliFP,
			OperatorKeyPEM:      operatorKeyPEM,
			CLIKeyPEM:           cliKeyPEM,
			ExpiresAt:           createResp.ExpiresAt,
			InstanceID:          c.instanceID,
			Hostname:            c.hostname,
		}
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
		fmt.Fprintf(os.Stderr, "Approve with: g8e auth approve-platform-enrollment %s\n", requestID)
	}

	// Step 5: Poll status until approved.
	deadline := operatorEnrollDefaultDeadline
	if pending != nil && !pending.ExpiresAt.IsZero() {
		deadline = time.Until(pending.ExpiresAt)
	}
	if err := c.pollUntilApproved(ctx, token, deadline); err != nil {
		return nil, err
	}

	// Step 6: Sign the completion transcript with both private keys.
	tokenHash := tokenHash(token)
	transcript, err := buildOperatorCompletionTranscript(requestID, tokenHash, c.instanceID, operatorFP, cliFP)
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

func (c *OperatorPlatformEnrollmentClient) submitRequest(ctx context.Context, operatorCSR, cliCSR, systemFingerprint string) (*models.PlatformEnrollmentCreateResponse, error) {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentRequest
	payload := models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        c.instanceID,
		Hostname:          c.hostname,
		SystemFingerprint: systemFingerprint,
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: operatorCSR,
			CLICSRPEM:      cliCSR,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: marshal request: %w", err)
	}

	// Retry with bounded backoff until the gateway is bootstrapped. The gateway
	// starts with zero users and returns 403 "platform enrollment requires a
	// bootstrapped gateway" until the owner bootstraps the first user. Workloads
	// start immediately after the gateway becomes healthy, so the first submit
	// attempt may race with bootstrap.
	delay := operatorEnrollSubmitInitial
	deadline := time.Now().Add(operatorEnrollSubmitDeadline)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("operator enrollment: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := doHTTPRequest(ctx, req)
		if err != nil {
			// Network error: back off and retry.
			if waitErr := c.sleep(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(operatorEnrollSubmitMax)))
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("operator enrollment: submit request: %w", err)
			}
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("operator enrollment: read response: %w", readErr)
		}

		if resp.StatusCode == http.StatusCreated {
			var createResp models.PlatformEnrollmentCreateResponse
			if err := json.Unmarshal(respBody, &createResp); err != nil {
				return nil, fmt.Errorf("operator enrollment: parse response: %w", err)
			}
			return &createResp, nil
		}

		// 403 "requires a bootstrapped gateway": the gateway is not yet
		// bootstrapped. Back off and retry until bootstrap.
		if resp.StatusCode == http.StatusForbidden && strings.Contains(string(respBody), constants.ErrPlatformEnrollmentRequiresBootstrap.Error()) {
			c.logger.Info("operator enrollment: gateway not yet bootstrapped, retrying", "delay", delay.String())
			if waitErr := c.sleep(ctx, delay); waitErr != nil {
				return nil, waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(operatorEnrollSubmitMax)))
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("operator enrollment: gateway not bootstrapped within %s: HTTP %d: %s", operatorEnrollSubmitDeadline, resp.StatusCode, string(respBody))
			}
			continue
		}

		return nil, fmt.Errorf("operator enrollment: request rejected: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
}

func (c *OperatorPlatformEnrollmentClient) pollUntilApproved(ctx context.Context, token string, deadline time.Duration) error {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentStatus
	deadlineTime := time.Now().Add(deadline)
	delay := operatorEnrollPollInitial

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadlineTime) {
			return fmt.Errorf("operator enrollment: polling deadline reached before approval")
		}

		pollCtx, cancel := context.WithTimeout(ctx, operatorEnrollHTTPTimeout)
		u := endpoint + "?token=" + url.QueryEscape(token)
		req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("operator enrollment: create status request: %w", err)
		}
		req.Header.Set("Cache-Control", "no-store")

		resp, err := doHTTPRequest(pollCtx, req)
		if err != nil {
			cancel()
			// Network error: back off and retry.
			if waitErr := c.sleep(ctx, delay); waitErr != nil {
				return waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(operatorEnrollPollMax)))
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			cancel()
			wait := retryAfter
			if wait == 0 {
				wait = delay
			}
			if waitErr := c.sleep(ctx, wait); waitErr != nil {
				return waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(operatorEnrollPollMax)))
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			return fmt.Errorf("operator enrollment: read status response: %w", err)
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
			return fmt.Errorf("operator enrollment: request has expired")
		}

		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		wait := retryAfter
		if wait == 0 {
			wait = delay
		}
		if waitErr := c.sleep(ctx, wait); waitErr != nil {
			return waitErr
		}
		delay = time.Duration(math.Min(float64(delay*2), float64(operatorEnrollPollMax)))
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

	completionCtx, cancel := context.WithTimeout(ctx, operatorEnrollHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(completionCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("operator enrollment: create completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")

	resp, err := doHTTPRequest(completionCtx, req)
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

	// Trust bundle.
	if creds.HubTrustBundle != "" {
		if err := c.atomicWrite(ctx, c.trustBundlePath(), []byte(creds.HubTrustBundle), constants.PermFilePublic); err != nil {
			return fmt.Errorf("operator enrollment: write trust bundle: %w", err)
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
// conversion needed, unlike WebCrypto in the dashboard JS client).
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
// a CSR PEM, matching the gateway's parsePlatformEnrollmentCSR: it
// hashes the SubjectPublicKeyInfo DER bytes and returns hex.
func csrFingerprint(csrPEM string) (string, error) {
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

// --- Helpers ---

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

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

func (c *OperatorPlatformEnrollmentClient) sleep(ctx context.Context, base time.Duration) error {
	jitter := time.Duration(mathrand.Int64N(int64(operatorEnrollPollJitter))) //nolint:gosec // poll jitter, not security-sensitive; keys use crypto/rand
	total := base + jitter
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(total):
		return nil
	}
}

func doHTTPRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := &http.Client{}
	return client.Do(req)
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	// Retry-After can be seconds or an HTTP-date. We only support
	// seconds here, which is what the gateway sends.
	var seconds int
	if _, err := fmt.Sscanf(value, "%d", &seconds); err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
