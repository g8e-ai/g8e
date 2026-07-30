// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package client is Agent Harness's thin, faithful client for a real g8e Gateway.
// It speaks the actual wire surfaces (health/state-root, MCP & A2A JSON-RPC,
// the governance envelope admission API, the OOB approve flow, and audit
// receipts) and records every exchange so the run can be audited in detail.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
)

// Persona is the identity Agent Harness wears for a given exchange. This is the ONLY
// thing Agent Harness fakes: it pretends to be whatever AI tool/agent we point at the
// Gateway. The Gateway and Operator are real and treat it like any BYO client.
type Persona struct {
	// ID is a stable handle, e.g. "claude-desktop", "cursor", "langchain-agent".
	ID string
	// UserAgent is sent on the wire so the Gateway/audit log attributes the call.
	UserAgent string
	// OperatorSessionID is the Operator session ID for Bearer token authentication.
	OperatorSessionID string
	// CLISessionID is the CLI session ID for CLI mTLS certificate binding.
	CLISessionID string
	// UserID is the user ID for session binding.
	UserID string
	// OperatorID is the operator ID for session binding.
	OperatorID string
}

// Exchange is a single recorded HTTP round-trip. Slices of these are the spine
// of the detailed audit report.
type Exchange struct {
	Persona   string          `json:"persona"`
	Method    string          `json:"method"`
	URL       string          `json:"url"`
	ReqBody   json.RawMessage `json:"request,omitempty"`
	ReqRaw    string          `json:"request_raw,omitempty"`
	Status    int             `json:"status"`
	RespBody  json.RawMessage `json:"response,omitempty"`
	RespRaw   string          `json:"response_raw,omitempty"`
	LatencyMS int64           `json:"latency_ms"`
	Err       string          `json:"error,omitempty"`
	At        time.Time       `json:"at"`
}

// Client wraps an mTLS-capable http.Client plus the recorder.
type Client struct {
	cfg  config.Config
	http *http.Client
	rec  *[]Exchange // optional sink for the current scenario
}

// New builds a Client with mTLS material loaded per config. The MCP/A2A surface
// also accepts an API key; both are attached automatically when present.
func New(cfg config.Config) (*Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	if cfg.Auth.CABundle != "" {
		if pem, err := os.ReadFile(cfg.Auth.CABundle); err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tlsCfg.RootCAs = pool
			}
		}
	}
	if cfg.Auth.ClientCert != "" && cfg.Auth.ClientKey != "" {
		if _, err := os.Stat(cfg.Auth.ClientCert); err == nil {
			if _, err := os.Stat(cfg.Auth.ClientKey); err == nil {
				crt, err := tls.LoadX509KeyPair(cfg.Auth.ClientCert, cfg.Auth.ClientKey)
				if err != nil {
					return nil, fmt.Errorf("load client cert: %w", err)
				}
				tlsCfg.Certificates = []tls.Certificate{crt}
			}
		}
	}

	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// Config exposes the live config (scenarios need TTLs, key ids, etc.).
func (c *Client) Config() config.Config { return c.cfg }

// Record points the client at a sink; every subsequent call appends an Exchange.
func (c *Client) Record(sink *[]Exchange) { c.rec = sink }

// do executes a JSON request against the mTLS surface and records it.
func (c *Client) do(ctx context.Context, p Persona, method, url string, body []byte) (int, []byte, error) {
	start := time.Now()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
		// Surface the impersonated identity for audit attribution too.
		req.Header.Set("X-G8E-Client-Persona", p.ID)
	}
	if c.cfg.Auth.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Auth.APIKey)
	} else if p.OperatorSessionID != "" {
		req.Header.Set("Authorization", "Bearer "+p.OperatorSessionID)
	}
	if p.CLISessionID != "" {
		req.Header.Set(constants.HeaderCLISessionID, p.CLISessionID)
	}
	if p.UserID != "" {
		req.Header.Set(constants.HeaderUserID, p.UserID)
	}
	if p.OperatorID != "" {
		req.Header.Set(constants.HeaderOperatorID, p.OperatorID)
	}

	resp, err := c.http.Do(req)
	ex := Exchange{Persona: p.ID, Method: method, URL: url, At: start}
	attachBody(&ex.ReqBody, &ex.ReqRaw, body)

	if err != nil {
		ex.Err = err.Error()
		ex.LatencyMS = time.Since(start).Milliseconds()
		c.append(ex, c.cfg.Verbose)
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)

	ex.Status = resp.StatusCode
	ex.LatencyMS = time.Since(start).Milliseconds()
	attachBody(&ex.RespBody, &ex.RespRaw, out)
	c.append(ex, c.cfg.Verbose)
	return resp.StatusCode, out, nil
}

func (c *Client) append(ex Exchange, echo bool) {
	if c.rec != nil {
		*c.rec = append(*c.rec, ex)
	}
	if echo {
		fmt.Fprintf(os.Stderr, "  %-3s %d %dms %s\n", ex.Method, ex.Status, ex.LatencyMS, ex.URL)
		if ex.Err != "" {
			fmt.Fprintf(os.Stderr, "      err: %s\n", ex.Err)
		}
	}
}

// attachBody stores valid JSON as structured, anything else as raw text.
func attachBody(j *json.RawMessage, raw *string, b []byte) {
	if len(b) == 0 {
		return
	}
	if json.Valid(b) {
		*j = append(json.RawMessage(nil), b...)
		return
	}
	*raw = string(b)
}

// ---- Gateway surfaces -------------------------------------------------------

// StateRoot fetches the current state_merkle_root from /state on the public surface.
// Maximal envelopes must bind to this exact root or the Operator drops them (TOCTOU gap).
func (c *Client) StateRoot(ctx context.Context) (string, error) {
	return c.stateRoot(ctx, c.cfg.PublicBaseURL)
}

// StateRootFromMTLS fetches the current state_merkle_root from /state on the mTLS surface.
// Use this when the gateway is running in full cert mode where all ports require mTLS.
func (c *Client) StateRootFromMTLS(ctx context.Context) (string, error) {
	return c.stateRoot(ctx, c.cfg.MTLSBaseURL)
}

func (c *Client) stateRoot(ctx context.Context, baseURL string) (string, error) {
	_, body, err := c.do(ctx, Persona{ID: "agent-harness"}, http.MethodGet, baseURL+constants.APIPaths.State, nil)
	if err != nil {
		return "", err
	}
	var h struct {
		StateMerkleRoot string `json:"state_merkle_root"`
		StateRoot       string `json:"state_root"`
	}
	_ = json.Unmarshal(body, &h)
	if h.StateMerkleRoot != "" {
		return h.StateMerkleRoot, nil
	}
	return h.StateRoot, nil // tolerate either field name
}

// RegisterSigner registers an Ed25519 public key as a trusted L2/principal
// signer so consensus/notary postures will accept Agent Harness's proofs.
// Best-effort: the exact request shape lives in handleTrustedSigners; the call
// is recorded and non-fatal so the doctrine-posture demos still run if it 404s.
func (c *Client) RegisterSigner(ctx context.Context, keyID, pubHex, role string) error {
	payload, _ := json.Marshal(map[string]any{
		"id":             keyID,
		"public_key_hex": pubHex,
		"enabled":        true,
	})
	status, _, err := c.do(ctx, Persona{ID: "agent-harness"}, http.MethodPost,
		c.cfg.MTLSBaseURL+constants.APIPaths.GovernanceSigners, payload)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("register signer %q: status %d", keyID, status)
	}
	return nil
}

// ApproveWithWebAuthn drives the real out-of-band L3 notary flow: it signs a
// genuine WebAuthn assertion for the given transaction hash using the
// SoftAuthenticator, then POSTs the assertion to
// /api/v1/approvals/{txHash}/verify. The gateway verifies the assertion and
// resumes the suspended transaction with an L3 proof attached.
func (c *Client) ApproveWithWebAuthn(ctx context.Context, p Persona, txHash string, auth *SoftAuthenticator) (int, []byte, error) {
	proof, err := auth.SignAssertion(txHash)
	if err != nil {
		return 0, nil, fmt.Errorf("approve with webauthn: sign assertion: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"id":               proof.CredentialId,
		"rawId":            proof.CredentialId,
		"clientDataJSON":   proof.ClientDataJson,
		"authenticatorData": proof.AuthenticatorData,
		"signature":        proof.Signature,
	})
	return c.do(ctx, p, http.MethodPost,
		c.cfg.PublicBaseURL+constants.APIPaths.ApprovalsByID+txHash+constants.APIPaths.ApprovalsVerifyAction, body)
}
