// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package config holds everything Phantom needs to dial a real Gateway and
// impersonate arbitrary agents against it. Values come from (in order of
// precedence): explicit flags > environment > an optional JSON file > defaults.
package config

import (
	"encoding/json"
	"os"
	"time"
)

// Auth selects how Phantom authenticates to the Gateway's mTLS surface (8440).
// The MCP/A2A routes are exempt from the main mTLS middleware and can also take
// an API key, but the TLS listener itself still negotiates client certs on 8440,
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
	// audit). Default https://localhost:8440.
	MTLSBaseURL string `json:"mtls_base_url"`
	// PublicBaseURL is the Gateway public surface (8442) used for the L3
	// suspend/approve out-of-band notary flow.
	PublicBaseURL string `json:"public_base_url"`

	Auth Auth `json:"auth"`

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

	// OutDir is where Phantom writes its detailed run report and receipt export.
	OutDir string `json:"out_dir"`

	// Verbose echoes every request/response to stderr as it happens.
	Verbose bool `json:"verbose"`
}

// Default returns a config wired for a local two-container dev stack.
func Default() Config {
	return Config{
		MTLSBaseURL:    envOr("PHANTOM_MTLS_URL", "https://localhost:8440"),
		PublicBaseURL:  envOr("PHANTOM_PUBLIC_URL", "https://localhost:8442"),
		EnsembleSize:   3,
		ConsensusKeyID: "phantom-ensemble",
		PrincipalKeyID: "phantom-principal",
		L3Mode:         "suspend",
		EnvelopeTTL:    5 * time.Minute,
		OutDir:         "./phantom-out",
		Auth: Auth{
			ClientCert: envOr("PHANTOM_CLIENT_CERT", ".g8e/pki/client.crt"),
			ClientKey:  envOr("PHANTOM_CLIENT_KEY", ".g8e/pki/client.key"),
			CABundle:   envOr("PHANTOM_CA_BUNDLE", ".g8e/pki/hub-bundle.pem"),
			APIKey:     os.Getenv("PHANTOM_API_KEY"),
		},
	}
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
