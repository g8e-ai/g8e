// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package config holds everything the auditor needs to dial a real Gateway and
// impersonate arbitrary agents against it. Values come from (in order of
// precedence): explicit flags > environment > an optional JSON file > defaults.
package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/network"
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
	// suspend/approve out-of-band notary flow.
	PublicBaseURL string `json:"public_base_url"`

	Auth Auth `json:"auth"`

	// UseCLIConfig uses the CLI credentials directory for all paths.
	UseCLIConfig bool `json:"use_cli_config"`

	// OperatorSessionID scopes audit receipt queries to the real Operator that
	// executed the work. If empty, Agent Harness tries to discover it from /api/operators.
	OperatorSessionID string `json:"operator_session_id"`

	// EnsembleSize is the number of mock consensus agents that "vote" on each
	// maximal envelope. The envelope still carries a single aggregate L2
	// signature from the registered consensus key (KeyID), with one AgentID per voter.
	EnsembleSize int `json:"ensemble_size"`
	// ConsensusKeyID is the trusted-signer id Agent Harness registers for its L2 key.
	ConsensusKeyID string `json:"consensus_key_id"`
	// ConsensusSeed is an optional hex-encoded Ed25519 seed for deterministic
	// ensemble key generation. When set, the ensemble is reconstructed from
	// this seed instead of being randomly generated, enabling the gateway to
	// verify L2 votes against a pre-registered trusted signer.
	ConsensusSeed string `json:"consensus_seed"`
	// ConsensusID is the ID of the ConsensusPolicy the gateway uses for L2
	// consensus verification. The harness sets this on the Ensemble so L2
	// votes carry the correct consensus_id. Defaults to "test-consensus".
	ConsensusID string `json:"consensus_id"`
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
		MTLSBaseURL:    network.LocalhostHTTPSURL(constants.Ports.OperatorHttps),
		PublicBaseURL:  network.LocalhostHTTPURL(constants.Ports.OperatorHttp),
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
			cfg.Auth.CABundle = cliCfg.ResolvedTrustBundlePath()
		}
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
