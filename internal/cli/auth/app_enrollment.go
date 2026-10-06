// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	serviceauth "github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"

	"google.golang.org/protobuf/proto"
)

const (
	appEnrollHTTPTimeout     = 10 * time.Second
	appEnrollPollInitial     = 2 * time.Second
	appEnrollPollMax         = 30 * time.Second
	appEnrollPollJitter      = 500 * time.Millisecond
	appEnrollDefaultDeadline = 30 * time.Minute

	appEnrollSubmitInitial  = 3 * time.Second
	appEnrollSubmitMax      = 30 * time.Second
	appEnrollSubmitJitter   = 1 * time.Second
	appEnrollSubmitDeadline = 30 * time.Minute
)

// AppEnrollmentOptions provides optional overrides for app platform enrollment.
type AppEnrollmentOptions struct {
	GatewayHTTPURL string
	InstanceID     string
	Hostname       string
}

// appPendingState is the resumable pending enrollment attempt,
// persisted to pki/pending-enrollment/app_<name>.json with 0600 permissions.
type appPendingState struct {
	RequestID      string    `json:"request_id"`
	Token          string    `json:"token"`
	AppFingerprint string    `json:"app_fingerprint"`
	KeyPEM         string    `json:"key_pem"`
	ExpiresAt      time.Time `json:"expires_at"`
	AppName        string    `json:"app_name"`
	InstanceID     string    `json:"instance_id"`
	Hostname       string    `json:"hostname"`
}

// AppPlatformEnrollmentClient drives the owner-approved platform
// enrollment protocol for application components.
type AppPlatformEnrollmentClient struct {
	appName        string
	gatewayHTTPURL string
	instanceID     string
	hostname       string
	fileSvc        fs.RuntimeFileService
	cfg            *config.Config
	logger         *slog.Logger
}

// NewAppPlatformEnrollmentClient constructs an app platform enrollment client.
func NewAppPlatformEnrollmentClient(appName string, fileSvc fs.RuntimeFileService, cfg *config.Config, logger *slog.Logger, opts ...AppEnrollmentOptions) (*AppPlatformEnrollmentClient, error) {
	if err := validateAppName(appName); err != nil {
		return nil, err
	}
	if fileSvc == nil {
		return nil, fmt.Errorf("%w: file service is required for app platform enrollment", constants.ErrInternal)
	}
	if cfg == nil {
		return nil, fmt.Errorf("%w: config is required for app platform enrollment", constants.ErrInternal)
	}
	if logger == nil {
		logger = slog.Default()
	}

	var opt AppEnrollmentOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	hostname := opt.Hostname
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			hostname = "localhost"
		} else {
			hostname = h
		}
	}

	instanceID := opt.InstanceID
	if instanceID == "" {
		instanceID = fmt.Sprintf("app-%s-%s", appName, hostname)
	}

	gatewayHTTPURL := opt.GatewayHTTPURL
	if gatewayHTTPURL == "" {
		gatewayHTTPURL = resolveGatewayHTTPURL(cfg)
	}
	gatewayHTTPURL = strings.TrimRight(gatewayHTTPURL, "/")

	return &AppPlatformEnrollmentClient{
		appName:        appName,
		gatewayHTTPURL: gatewayHTTPURL,
		instanceID:     instanceID,
		hostname:       hostname,
		fileSvc:        fileSvc,
		cfg:            cfg,
		logger:         logger,
	}, nil
}

// Enroll performs the platform enrollment sequence for the application.
// If a resumable pending attempt exists on disk, it resumes from that state.
// Output instructions are written to out (e.g. cmd.OutOrStdout()).
func (c *AppPlatformEnrollmentClient) Enroll(ctx context.Context, out io.Writer) (*models.PlatformEnrollmentCompleteResponse, error) {
	pendingPath := c.pendingStatePath()

	// Step 1: Load persisted pending attempt if it exists.
	pending, err := c.loadPendingState(pendingPath)
	if err != nil {
		return nil, err
	}

	var (
		token, requestID string
		appFP            string
		appKeyPEM        string
		appKey           *ecdsa.PrivateKey
	)

	if pending != nil && pending.Token != "" && pending.RequestID != "" && time.Now().Before(pending.ExpiresAt) {
		token = pending.Token
		requestID = pending.RequestID
		appFP = pending.AppFingerprint
		appKeyPEM = pending.KeyPEM
		appKey, err = parseECPrivateKeyPEM(appKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("app enrollment: resume private key: %w", err)
		}
		c.logger.Info("app enrollment: resuming pending attempt", "app_name", c.appName, "request_id", requestID)
	} else {
		// Step 2: Generate new key and CSR.
		csrPEM, key, err := GenerateCSR(fmt.Sprintf("g8e-app-%s", c.appName))
		if err != nil {
			return nil, err
		}
		appKey = key

		appFP, err = csrFingerprint(csrPEM)
		if err != nil {
			return nil, fmt.Errorf("app enrollment: app csr fingerprint: %w", err)
		}

		appKeyPEM, err = encodeECPrivateKeyPEM(appKey)
		if err != nil {
			return nil, fmt.Errorf("app enrollment: encode app key: %w", err)
		}

		createResp, err := c.submitRequest(ctx, csrPEM)
		if err != nil {
			return nil, err
		}
		requestID = createResp.RequestID
		token = createResp.Token
		if token == "" {
			return nil, fmt.Errorf("app enrollment: gateway returned a deduplicated response with no token; a pending state file is required to resume. Request ID: %s", requestID)
		}

		// Persist pending state with 0600 permissions.
		pending = &appPendingState{
			RequestID:      requestID,
			Token:          token,
			AppFingerprint: appFP,
			KeyPEM:         appKeyPEM,
			ExpiresAt:      createResp.ExpiresAt,
			AppName:        c.appName,
			InstanceID:     c.instanceID,
			Hostname:       c.hostname,
		}
		if err := c.persistPendingState(pendingPath, pending); err != nil {
			return nil, err
		}

		c.logger.Info("app enrollment: request submitted", "app_name", c.appName, "request_id", requestID, "fingerprint", appFP)
		if out != nil {
			fmt.Fprintf(out, "Enrollment request submitted for application %q.\n", c.appName)
			fmt.Fprintf(out, "Approve with: g8e auth enroll approve %s\n", requestID)
		}
	}

	// Step 3: Poll status until approved.
	deadline := appEnrollDefaultDeadline
	if pending != nil && !pending.ExpiresAt.IsZero() {
		deadline = time.Until(pending.ExpiresAt)
	}
	if err := c.pollUntilApproved(ctx, token, deadline); err != nil {
		return nil, err
	}

	// Step 4: Sign the completion transcript.
	tokenHashStr := tokenHash(token)
	transcript, err := buildAppCompletionTranscript(requestID, tokenHashStr, c.instanceID, appFP)
	if err != nil {
		return nil, err
	}
	appProof, err := signTranscript(appKey, transcript)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: sign app proof: %w", err)
	}

	// Step 5: Submit completion.
	completionResp, err := c.submitCompletion(ctx, token, appProof)
	if err != nil {
		return nil, err
	}
	if completionResp.App == nil || completionResp.App.AppCert == "" {
		return nil, fmt.Errorf("app enrollment: completion response missing app credentials")
	}

	// Step 6: Write credentials to disk and clear pending state.
	if err := c.writeCredentials(completionResp.App, appKeyPEM); err != nil {
		return nil, err
	}
	if err := c.removePendingState(pendingPath); err != nil {
		c.logger.Warn("app enrollment: failed to remove pending state", "error", err)
	}

	c.logger.Info("app enrollment: completed successfully", "app_name", c.appName, "app_id", completionResp.App.AppID)
	if out != nil {
		fmt.Fprintf(out, "Application %q enrolled successfully (App ID: %s).\n", c.appName, completionResp.App.AppID)
	}

	return completionResp, nil
}

// LoadIdentity loads and verifies the installed TLS certificate for this client's appName.
func (c *AppPlatformEnrollmentClient) LoadIdentity() (*tls.Certificate, error) {
	return LoadAppIdentity(c.fileSvc, c.cfg, c.appName)
}

// HasValidIdentity checks if this client's application has a valid, non-expired identity.
func (c *AppPlatformEnrollmentClient) HasValidIdentity() bool {
	return HasValidAppIdentity(c.fileSvc, c.cfg, c.appName)
}

// LoadAppIdentity loads and verifies the installed TLS certificate for the given appName.
func LoadAppIdentity(fileSvc fs.RuntimeFileService, cfg *config.Config, appName string) (*tls.Certificate, error) {
	if err := validateAppName(appName); err != nil {
		return nil, err
	}
	if fileSvc == nil || cfg == nil {
		return nil, constants.ErrInternal
	}

	certPath := cfg.AppCertFile(appName)
	keyPath := cfg.AppKeyFile(appName)

	certRel, err := fileSvc.RelFromAbs(certPath)
	if err != nil {
		certRel = certPath
	}
	keyRel, err := fileSvc.RelFromAbs(keyPath)
	if err != nil {
		keyRel = keyPath
	}

	certPEM, err := fileSvc.ReadFile(context.Background(), certRel)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read app cert for %q: %w", constants.ErrAppIdentityNotFound, appName, err)
	}
	keyPEM, err := fileSvc.ReadFile(context.Background(), keyRel)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read app key for %q: %w", constants.ErrAppIdentityNotFound, appName, err)
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cert/key pair for %q: %w", constants.ErrAppIdentityNotFound, appName, err)
	}

	if len(tlsCert.Certificate) == 0 {
		return nil, fmt.Errorf("%w: no certificates found for %q", constants.ErrAppIdentityNotFound, appName)
	}

	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse certificate for %q: %w", constants.ErrAppIdentityNotFound, appName, err)
	}

	now := time.Now()
	if now.After(leaf.NotAfter) {
		return nil, fmt.Errorf("%w: app certificate for %q expired at %s", constants.ErrAppIdentityExpired, appName, leaf.NotAfter.Format(time.RFC3339))
	}
	if now.Before(leaf.NotBefore) {
		return nil, fmt.Errorf("%w: app certificate for %q not yet valid", constants.ErrAppIdentityNotFound, appName)
	}

	// Verify certificate chain against gateway trust bundle if available.
	trustBundlePEM, err := ReadTrustBundle(fileSvc, cfg)
	if err == nil && len(trustBundlePEM) > 0 {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(trustBundlePEM) {
			intermediates := x509.NewCertPool()
			for _, extra := range tlsCert.Certificate[1:] {
				if extraCert, err := x509.ParseCertificate(extra); err == nil {
					intermediates.AddCert(extraCert)
				}
			}
			opts := x509.VerifyOptions{
				Roots:         pool,
				Intermediates: intermediates,
				CurrentTime:   now,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}
			if _, err := leaf.Verify(opts); err != nil {
				return nil, fmt.Errorf("%w: app certificate for %q untrusted: %w", constants.ErrAppIdentityUntrusted, appName, err)
			}
		}
	}

	tlsCert.Leaf = leaf
	return &tlsCert, nil
}

// HasValidAppIdentity returns true if an unexpired, trusted certificate exists for appName.
func HasValidAppIdentity(fileSvc fs.RuntimeFileService, cfg *config.Config, appName string) bool {
	_, err := LoadAppIdentity(fileSvc, cfg, appName)
	return err == nil
}

// --- Internal Implementation ---

func (c *AppPlatformEnrollmentClient) pendingStatePath() string {
	return filepath.Join(constants.PkiDirname, constants.PkiSubdirPendingEnroll, fmt.Sprintf("app_%s.json", c.appName))
}

func (c *AppPlatformEnrollmentClient) loadPendingState(relPath string) (*appPendingState, error) {
	exists, err := c.fileSvc.FileExists(context.Background(), relPath)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: check pending state exists: %w", err)
	}
	if !exists {
		return nil, nil
	}
	data, err := c.fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: read pending state: %w", err)
	}
	var state appPendingState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("app enrollment: parse pending state: %w", err)
	}
	return &state, nil
}

func (c *AppPlatformEnrollmentClient) persistPendingState(relPath string, state *appPendingState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("app enrollment: marshal pending state: %w", err)
	}
	if err := c.fileSvc.WriteFile(context.Background(), relPath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("app enrollment: write pending state: %w", err)
	}
	return nil
}

func (c *AppPlatformEnrollmentClient) removePendingState(relPath string) error {
	return c.fileSvc.Remove(context.Background(), relPath)
}

func (c *AppPlatformEnrollmentClient) submitRequest(ctx context.Context, csrPEM string) (*models.PlatformEnrollmentCreateResponse, error) {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentRequest
	payload := models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentApplication,
		AppName:       c.appName,
		InstanceID:    c.instanceID,
		Hostname:      c.hostname,
		App: &models.PlatformAppCSRPayload{
			CSRPEM: csrPEM,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: marshal request: %w", err)
	}

	delay := appEnrollSubmitInitial
	deadline := time.Now().Add(appEnrollSubmitDeadline)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("app enrollment: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := doHTTPRequest(ctx, req)
		if err != nil {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("app enrollment: submit request timed out: %w", err)
			}
			if waitErr := c.sleep(ctx, delay, appEnrollSubmitJitter); waitErr != nil {
				return nil, waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(appEnrollSubmitMax)))
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("app enrollment: read request response: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden && strings.Contains(string(respBody), "bootstrapped") {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("app enrollment: timed out waiting for gateway bootstrap: %s", string(respBody))
			}
			if waitErr := c.sleep(ctx, delay, appEnrollSubmitJitter); waitErr != nil {
				return nil, waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(appEnrollSubmitMax)))
			continue
		}

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("app enrollment: request rejected: HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		var createResp models.PlatformEnrollmentCreateResponse
		if err := json.Unmarshal(respBody, &createResp); err != nil {
			return nil, fmt.Errorf("app enrollment: parse create response: %w", err)
		}
		return &createResp, nil
	}
}

func (c *AppPlatformEnrollmentClient) pollUntilApproved(ctx context.Context, token string, deadline time.Duration) error {
	endpoint := fmt.Sprintf("%s%s?token=%s", c.gatewayHTTPURL, constants.APIPaths.AuthPlatformEnrollmentStatus, url.QueryEscape(token))
	pollCtx, cancelPoll := context.WithTimeout(ctx, deadline)
	defer cancelPoll()

	delay := appEnrollPollInitial
	for {
		if pollCtx.Err() != nil {
			return fmt.Errorf("app enrollment: timed out waiting for owner approval: %w", pollCtx.Err())
		}

		req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("app enrollment: create status request: %w", err)
		}

		resp, err := doHTTPRequest(pollCtx, req)
		if err != nil {
			if waitErr := c.sleep(pollCtx, delay, appEnrollPollJitter); waitErr != nil {
				return waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(appEnrollPollMax)))
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			wait := retryAfter
			if wait == 0 {
				wait = delay
			}
			if waitErr := c.sleep(pollCtx, wait, appEnrollPollJitter); waitErr != nil {
				return waitErr
			}
			delay = time.Duration(math.Min(float64(delay*2), float64(appEnrollPollMax)))
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("app enrollment: read status response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("app enrollment: status query failed: HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		var statusResp models.PlatformEnrollmentStatusResponse
		if err := json.Unmarshal(respBody, &statusResp); err != nil {
			return fmt.Errorf("app enrollment: parse status response: %w", err)
		}

		switch statusResp.State {
		case models.PlatformEnrollmentStateApproved, models.PlatformEnrollmentStateCompleted:
			return nil
		case models.PlatformEnrollmentStateDenied:
			return fmt.Errorf("app enrollment: request was denied by the owner")
		case models.PlatformEnrollmentStateExpired:
			return fmt.Errorf("app enrollment: request has expired")
		}

		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		wait := retryAfter
		if wait == 0 {
			wait = delay
		}
		if waitErr := c.sleep(pollCtx, wait, appEnrollPollJitter); waitErr != nil {
			return waitErr
		}
		delay = time.Duration(math.Min(float64(delay*2), float64(appEnrollPollMax)))
	}
}

func (c *AppPlatformEnrollmentClient) submitCompletion(ctx context.Context, token, proof string) (*models.PlatformEnrollmentCompleteResponse, error) {
	endpoint := c.gatewayHTTPURL + constants.APIPaths.AuthPlatformEnrollmentComplete
	payload := models.PlatformEnrollmentCompleteRequest{
		Token: token,
		Proofs: models.PlatformEnrollmentProofs{
			App: proof,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: marshal completion: %w", err)
	}

	compCtx, cancel := context.WithTimeout(ctx, appEnrollHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(compCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("app enrollment: create completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")

	resp, err := doHTTPRequest(compCtx, req)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: submit completion: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: read completion response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("app enrollment: completion rejected: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var completionResp models.PlatformEnrollmentCompleteResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("app enrollment: parse completion response: %w", err)
	}
	return &completionResp, nil
}

func (c *AppPlatformEnrollmentClient) writeCredentials(creds *models.PlatformEnrollmentAppCredentials, keyPEM string) error {
	certRel, err := c.fileSvc.RelFromAbs(c.cfg.AppCertFile(c.appName))
	if err != nil {
		certRel = c.cfg.AppCertFile(c.appName)
	}
	keyRel, err := c.fileSvc.RelFromAbs(c.cfg.AppKeyFile(c.appName))
	if err != nil {
		keyRel = c.cfg.AppKeyFile(c.appName)
	}

	fullCertPEM := strings.TrimSpace(creds.AppCert)
	if creds.CertChain != "" {
		fullCertPEM = fmt.Sprintf("%s\n%s", fullCertPEM, strings.TrimSpace(creds.CertChain))
	}

	if err := c.fileSvc.WriteFile(context.Background(), certRel, []byte(fullCertPEM), constants.PermFilePublic); err != nil {
		return fmt.Errorf("app enrollment: write app cert: %w", err)
	}
	if err := c.fileSvc.WriteFile(context.Background(), keyRel, []byte(keyPEM), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("app enrollment: write app key: %w", err)
	}

	if creds.TrustBundle != "" {
		trustBundleRel := c.cfg.DefaultTrustBundleRelPath()
		exists, _ := c.fileSvc.FileExists(context.Background(), trustBundleRel)
		if !exists {
			_ = c.fileSvc.WriteFile(context.Background(), trustBundleRel, []byte(creds.TrustBundle), constants.PermFilePublic)
		}
	}

	return nil
}

func (c *AppPlatformEnrollmentClient) sleep(ctx context.Context, base, maxJitter time.Duration) error {
	jitter := time.Duration(0)
	if maxJitter > 0 {
		jitter = time.Duration(mathrand.Int64N(int64(maxJitter))) //nolint:gosec
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(base + jitter):
		return nil
	}
}

// --- Completion Transcript & Proof ---

func buildAppCompletionTranscript(requestID, tokenHash, instanceID, appFP string) ([]byte, error) {
	message := &commonv1.PlatformEnrollmentCompletionTranscript{
		ProtocolVersion: constants.PlatformEnrollmentProtocolVersion,
		RequestId:       requestID,
		TokenHash:       tokenHash,
		ComponentKind:   commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_APPLICATION,
		InstanceId:      instanceID,
		Fingerprints: &commonv1.PlatformEnrollmentFingerprints{
			App: appFP,
		},
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("app enrollment: marshal completion transcript: %w", err)
	}
	return encoded, nil
}

func signTranscript(privateKey *ecdsa.PrivateKey, transcript []byte) (string, error) {
	digest := sha256.Sum256(transcript)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign transcript: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(signature), nil
}

func csrFingerprint(csrPEM string) (string, error) {
	return serviceauth.CSRFingerprint(csrPEM)
}

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
	return x509.ParseECPrivateKey(keyBytes)
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
	return client.Do(req)
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	var seconds int
	if _, err := fmt.Sscanf(value, "%d", &seconds); err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func resolveGatewayHTTPURL(cfg *config.Config) string {
	if cfg != nil && cfg.Paths != nil && cfg.Paths.Host != "" {
		host := cfg.Paths.Host
		if strings.Contains(host, "://") {
			return strings.TrimRight(host, "/")
		}
		if _, _, err := net.SplitHostPort(host); err == nil {
			return "http://" + host
		}
		return fmt.Sprintf("http://%s:%d", host, constants.Ports.OperatorHttp)
	}
	host := "127.0.0.1"
	if cfg != nil && cfg.Paths != nil && cfg.Paths.Host != "" {
		host = cfg.Paths.Host
	}
	return fmt.Sprintf("http://%s:%d", host, constants.Ports.OperatorHttp)
}

func validateAppName(appName string) error {
	if appName == "" {
		return constants.ErrPlatformEnrollmentAppNameRequired
	}
	if len(appName) > constants.PlatformEnrollmentMaxAppNameBytes {
		return constants.ErrPlatformEnrollmentInvalidAppName
	}
	if !isValidAppNameFormat(appName) || isReservedPlatformName(appName) {
		return constants.ErrPlatformEnrollmentInvalidAppName
	}
	return nil
}

func isValidAppNameFormat(name string) bool {
	matched, err := regexp.MatchString(`^[a-z0-9][a-z0-9._-]*$`, name)
	return err == nil && matched
}

func isReservedPlatformName(name string) bool {
	switch name {
	case "g8e", "g8eg", "g8eo", "g8ee", "g8ed",
		"g8e-gateway", "g8e-operator", "g8e-ensemble", "g8e-dashboard", "g8e-cli",
		"operator", "ensemble", "dashboard", "gateway", "cli", "admin", "system":
		return true
	default:
		return false
	}
}
