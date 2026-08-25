// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package config holds everything the auditor needs to dial a real Gateway and
// impersonate arbitrary agents against it. Values come from (in order of
// precedence): explicit flags > environment > an optional JSON file > defaults.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
)

// Auth selects how Agent Harness authenticates to the Gateway's mTLS surface.
// The MCP/A2A routes are exempt from the main mTLS middleware and can also take
// an API key, but the TLS listener itself still negotiates client certs,
// so a cert is the realistic default.
type Auth struct {
	// ClientCert / ClientKey are the BYO-client mTLS material minted by
	// `g8e auth bootstrap` / `g8e auth login` (PEM paths).
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
	// CABundle is the Gateway root/hub bundle (.well-known/g8e/pki/hub-bundle.pem).
	CABundle string `json:"ca_bundle"`
	// APIKey is the optional Operator API key for the MCP/A2A surface.
	APIKey string `json:"api_key"`
}

// Config is the full Agent Harness runtime configuration.
type Config struct {
	// MTLSBaseURL is the Gateway mTLS API surface (governance envelope, MCP/A2A,
	// audit).
	MTLSBaseURL string `json:"mtls_base_url"`
	// PublicBaseURL is the Gateway public surface used for the L3
	// suspend/approve out-of-band notary flow. The harness dials this
	// URL directly (SSE subscription, approval status verification), so
	// it must be reachable from wherever the harness process runs — in
	// the demo topology that is inside the agent container, hence the
	// container-internal g8e.local address.
	PublicBaseURL string `json:"public_base_url"`
	// ApprovalDisplayURL is the host-reachable base URL used to render
	// the human-facing approval link printed to stderr. It is distinct
	// from PublicBaseURL because the harness process (inside a
	// container) and the human approver (host browser) reach the
	// gateway via different addresses/ports. When empty, the printed
	// link falls back to PublicBaseURL.
	ApprovalDisplayURL string `json:"approval_display_url"`

	// EnsembleBaseURL is the g8ee (ensemble) HTTP surface used by the
	// ensemble chat scenarios. The ensemble is a Python/FastAPI app on
	// its own port; the harness dials POST /api/v1/chat directly. When
	// empty, ensemble scenarios fail closed with a clear error.
	EnsembleBaseURL string `json:"ensemble_base_url"`

	Auth Auth `json:"auth"`

	// UseCLIConfig uses the CLI credentials directory for all paths.
	UseCLIConfig bool `json:"use_cli_config"`

	// CLIAuth holds the host CLI mTLS material used for notary-scenario
	// transaction submits. When populated, the harness client builds a
	// second http.Client with this cert pair so handleCLIAuth stamps the
	// host user's identity onto the suspended transaction. Non-notary MCP
	// calls continue using the operator cert in Auth.
	CLIAuth Auth `json:"cli_auth"`

	// OperatorSessionID scopes audit receipt queries to the real Operator that
	// executed the work. If empty, Agent Harness tries to discover it from /api/operators.
	OperatorSessionID string `json:"operator_session_id"`

	// UserID is the host CLI user_id used to scope the SSE approval subscription.
	// When set, WaitForHumanApproval subscribes with this user_id instead of the
	// operator id, so the harness receives events for transactions tagged with
	// the host user's identity.
	UserID string `json:"user_id"`

	// CLISessionID is the host CLI session id sent as X-CLI-Session-ID on
	// notary submit so handleCLIAuth stamps the host user onto the suspended
	// transaction.
	CLISessionID string `json:"cli_session_id"`

	// EnvelopeTTL is how long a maximal envelope is valid before expiry.
	EnvelopeTTL time.Duration `json:"envelope_ttl"`

	// OutDir is where the auditor writes its detailed run report and receipt export.
	OutDir string `json:"out_dir"`

	// Verbose echoes every request/response to stderr as it happens.
	Verbose bool `json:"verbose"`
}

// CLIIdentity is the host CLI session identity loaded from the credentials
// file by [LoadCLIIdentity]. It is the single source of truth for the
// user_id, cli_session_id, and operator_session_id that the harness stamps
// on authenticated audit/operator requests.
type CLIIdentity struct {
	UserID            string
	CLISessionID      string
	OperatorSessionID string
}

// LoadCLIIdentity loads the host CLI session identity (user_id,
// cli_session_id, operator_session_id) from the CLI credentials file via the
// shared [auth.LoadCredentials] path. It is the single helper used by both
// [Default] and [client.Client.DiscoverOperator] so the two code paths cannot
// drift on which identity source they read (the E.0/E.5 root cause was
// duplicated credential-loading logic that silently dropped the session id).
//
// projectRoot is the base directory containing the .g8e/ runtime tree. When
// empty, the current working directory is used (matching [config.Load]
// behavior). Tests pass a temp directory so no os.Chdir is needed.
//
// Returns a zero-value CLIIdentity and nil error when the CLI config or
// credentials file is absent (the harness can still run with explicit flags).
// A malformed CLI config or credentials file returns the error so the caller
// can log a warning rather than silently falling back to defaults with no
// diagnostic (the E.6 root cause).
func LoadCLIIdentity(projectRoot string) (CLIIdentity, error) {
	cliCfg, err := config.Load(projectRoot)
	if err != nil {
		return CLIIdentity{}, fmt.Errorf("agent_harness: load CLI config: %w", err)
	}
	if cliCfg == nil {
		return CLIIdentity{}, nil
	}
	fileSvc, err := fs.NewRuntimeFileService(projectRoot, slog.Default())
	if err != nil {
		return CLIIdentity{}, fmt.Errorf("agent_harness: init file service: %w", err)
	}
	creds, err := auth.LoadCredentials(fileSvc, cliCfg)
	if err != nil {
		return CLIIdentity{}, fmt.Errorf("agent_harness: load CLI credentials: %w", err)
	}
	if creds == nil {
		return CLIIdentity{}, nil
	}
	return CLIIdentity{
		UserID:            creds.UserID,
		CLISessionID:      creds.CLISessionID,
		OperatorSessionID: creds.OperatorSessionID,
	}, nil
}

// Default returns a config wired for a local two-container dev stack.
func Default() Config {
	cfg := Config{
		MTLSBaseURL:   network.LocalhostHTTPSURL(constants.Ports.OperatorHttps),
		PublicBaseURL: network.LocalhostHTTPURL(constants.Ports.OperatorHttp),
		EnvelopeTTL:   5 * time.Minute,
		OutDir:        "./auditor-out",
		UseCLIConfig:  true,
	}

	if cfg.UseCLIConfig {
		cliCfg, err := config.Load("")
		if err != nil {
			// Log a warning rather than silently discarding — a malformed CLI
			// config causes downstream 401s that give no clue about the root
			// cause (E.6). The harness can still run with explicit flags.
			slog.Warn("agent_harness: CLI config load failed; falling back to defaults", "error", err)
		}
		if cliCfg != nil {
			cfg.Auth.ClientCert = cliCfg.CLICertFile()
			cfg.Auth.ClientKey = cliCfg.CLIKeyFile()
			cfg.Auth.CABundle = cliCfg.ResolvedTrustBundlePath()
			// Load the CLI session identity via the shared helper so this
			// path and DiscoverOperator cannot drift on the identity source
			// (E.5). Explicit flags still win because applyAgentHarnessFlags
			// overlays these fields after Default() returns.
			identity, idErr := LoadCLIIdentity("")
			if idErr != nil {
				slog.Warn("agent_harness: CLI identity load failed; authenticated requests may 401", "error", idErr)
			}
			if cfg.UserID == "" {
				cfg.UserID = identity.UserID
			}
			if cfg.CLISessionID == "" {
				cfg.CLISessionID = identity.CLISessionID
			}
			if cfg.OperatorSessionID == "" {
				cfg.OperatorSessionID = identity.OperatorSessionID
			}
		}
	}

	// When CLIAuth is not explicitly set, default it to the same cert as
	// Auth so the harness works out-of-the-box for non-demo invocations.
	// Demo invocations override CLIAuth via --cli-cert/--cli-key/--cli-ca.
	if cfg.CLIAuth.ClientCert == "" && cfg.Auth.ClientCert != "" {
		cfg.CLIAuth.ClientCert = cfg.Auth.ClientCert
		cfg.CLIAuth.ClientKey = cfg.Auth.ClientKey
		cfg.CLIAuth.CABundle = cfg.Auth.CABundle
	}

	return cfg
}

// LoadFile overlays a JSON config file onto c (fields present in the file win).
func (c *Config) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, c)
}
