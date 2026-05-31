// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package config holds everything the auditor needs to dial a real Gateway and
// impersonate arbitrary agents against it. Values come from (in order of
// precedence): explicit flags > environment > an optional JSON file > defaults.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// Auth selects how Phantom authenticates to the Gateway's mTLS surface.
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
	// APIKey is the optional operator API key for the MCP/A2A surface.
	APIKey string `json:"api_key"`
	// Insecure skips TLS verification. Local dev only. Never in a real audit.
	Insecure bool `json:"insecure"`
}

// Config is the full Phantom runtime configuration.
type Config struct {
	// MTLSBaseURL is the Gateway mTLS API surface (governance envelope, MCP/A2A,
	// audit).
	MTLSBaseURL string `json:"mtls_base_url"`
	// PublicBaseURL is the Gateway public surface used for the L3
	// suspend/approve out-of-band notary flow.
	PublicBaseURL string `json:"public_base_url"`

	Auth Auth `json:"auth"`

	// UseCLIConfig uses the CLI credentials directory for all paths.
	UseCLIConfig bool `json:"use_cli_config"`

	// OperatorSessionID scopes audit receipt queries to the real Operator that
	// executed the work. If empty, Phantom tries to discover it from /api/operators.
	OperatorSessionID string `json:"operator_session_id"`

	// EnsembleSize is the number of mock consensus agents that "vote" on each
	// maximal envelope. The envelope still carries a single aggregate L2
	// signature from the registered consensus key (KeyID), with one AgentID per voter.
	EnsembleSize int `json:"ensemble_size"`
	// ConsensusKeyID is the trusted-signer id Phantom registers for its L2 key.
	ConsensusKeyID string `json:"consensus_key_id"`
	// PrincipalKeyID identifies the mock L3 principal (the "human" notary).
	PrincipalKeyID string `json:"principal_key_id"`

	// L3Mode controls how the maximal path satisfies L3 in notary posture:
	//   "mock"    - attach a principal Ed25519 signature as governance.l3.proof.signature
	//   "suspend" - submit without L3, follow the real OOB approve flow as the principal
	L3Mode string `json:"l3_mode"`

	// EnvelopeTTL is how long a maximal envelope is valid before expiry.
	EnvelopeTTL time.Duration `json:"envelope_ttl"`

	// OutDir is where the auditor writes its detailed run report and receipt export.
	OutDir string `json:"out_dir"`

	// Verbose echoes every request/response to stderr as it happens.
	Verbose bool `json:"verbose"`
}

// Default returns a config wired for a local two-container dev stack.
func Default() Config {
	cfg := Config{
		MTLSBaseURL:    envOr("G8E_AUDITOR_MTLS_URL", fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorPublicHttps)),
		PublicBaseURL:  envOr("G8E_AUDITOR_PUBLIC_URL", fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorPublicHttps)),
		EnsembleSize:   3,
		ConsensusKeyID: "auditor-ensemble",
		PrincipalKeyID: "auditor-principal",
		L3Mode:         "suspend",
		EnvelopeTTL:    5 * time.Minute,
		OutDir:         "./auditor-out",
		UseCLIConfig:   true,
	}

	if cfg.UseCLIConfig {
		cliCfg, _ := config.Load("")
		if cliCfg != nil {
			cfg.Auth.ClientCert = cliCfg.CLICertFile()
			cfg.Auth.ClientKey = cliCfg.CLIKeyFile()
			cfg.Auth.CABundle = cliCfg.TrustBundlePath()
		}
	}

	// Environment variable overrides
	if cert := os.Getenv("G8E_AUDITOR_CLIENT_CERT"); cert != "" {
		cfg.Auth.ClientCert = cert
	}
	if key := os.Getenv("G8E_AUDITOR_CLIENT_KEY"); key != "" {
		cfg.Auth.ClientKey = key
	}
	if bundle := os.Getenv("G8E_AUDITOR_CA_BUNDLE"); bundle != "" {
		cfg.Auth.CABundle = bundle
	}
	cfg.Auth.APIKey = os.Getenv("G8E_AUDITOR_API_KEY")

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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, _ := os.UserHomeDir()
		if path == "~" {
			return homeDir
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return os.ExpandEnv(path)
}
