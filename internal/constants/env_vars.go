// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// EnvVarKey is a typed string for environment variable names.
type EnvVarKey string

// EnvVar groups all environment variable name constants consumed by g8eo.
var EnvVar = struct {
	ConsensusID           EnvVarKey
	ConsensusURL          EnvVarKey
	ConsensusBootstrap    EnvVarKey
	VaultDir              EnvVarKey
	VaultKey              EnvVarKey
	OperatorSessionID     EnvVarKey
	PasskeyRpID           EnvVarKey
	PasskeyRpName         EnvVarKey
	PasskeyRpOrigins      EnvVarKey
	PublicBaseURL         EnvVarKey
	AllowedOrigins        EnvVarKey
	DoctrineDir           EnvVarKey
	Shell                 EnvVarKey
	Lang                  EnvVarKey
	Term                  EnvVarKey
	TZ                    EnvVarKey
	LatticeEndpoint       EnvVarKey
	LatticeClientID       EnvVarKey
	LatticeClientSecret   EnvVarKey
	LatticeSandboxesToken EnvVarKey
	LatticeEntityName     EnvVarKey
	LatticePostureFloor   EnvVarKey
	DemoRunID             EnvVarKey
	DemoScenarioID        EnvVarKey
	Home                  EnvVarKey
	User                  EnvVarKey
	SSHAuthSock           EnvVarKey
	CloudflareAPIToken    EnvVarKey
	CFAPIToken            EnvVarKey
	OllamaHost            EnvVarKey
	HarnessPollTimeout    EnvVarKey
	HarnessLLMProvider    EnvVarKey
	HarnessLLMModel       EnvVarKey
	HarnessLLMEndpoint    EnvVarKey
	TestReexec            EnvVarKey
}{
	ConsensusID:           EnvVarKey("G8E_CONSENSUS_ID"),
	ConsensusURL:          EnvVarKey("G8E_CONSENSUS_URL"),
	ConsensusBootstrap:    EnvVarKey("G8E_CONSENSUS_BOOTSTRAP"),
	VaultDir:              EnvVarKey("G8E_VAULT_DIR"),
	VaultKey:              EnvVarKey("G8E_VAULT_KEY"),
	OperatorSessionID:     EnvVarKey("G8E_OPERATOR_SESSION_ID"),
	PasskeyRpID:           EnvVarKey("G8E_PASSKEY_RP_ID"),
	PasskeyRpName:         EnvVarKey("G8E_PASSKEY_RP_NAME"),
	PasskeyRpOrigins:      EnvVarKey("G8E_PASSKEY_RP_ORIGINS"),
	PublicBaseURL:         EnvVarKey("G8E_PUBLIC_BASE_URL"),
	AllowedOrigins:        EnvVarKey("G8E_ALLOWED_ORIGINS"),
	DoctrineDir:           EnvVarKey("G8E_DOCTRINE_DIR"),
	Shell:                 EnvVarKey("SHELL"),
	Lang:                  EnvVarKey("LANG"),
	Term:                  EnvVarKey("TERM"),
	TZ:                    EnvVarKey("TZ"),
	LatticeEndpoint:       EnvVarKey("LATTICE_ENDPOINT"),
	LatticeClientID:       EnvVarKey("LATTICE_CLIENT_ID"),
	LatticeClientSecret:   EnvVarKey("LATTICE_CLIENT_SECRET"),
	LatticeSandboxesToken: EnvVarKey("SANDBOXES_TOKEN"),
	LatticeEntityName:     EnvVarKey("LATTICE_ENTITY_NAME"),
	LatticePostureFloor:   EnvVarKey("LATTICE_POSTURE_FLOOR"),
	DemoRunID:             EnvVarKey("G8E_DEMO_RUN_ID"),
	DemoScenarioID:        EnvVarKey("G8E_DEMO_SCENARIO_ID"),
	Home:                  EnvVarKey("HOME"),
	User:                  EnvVarKey("USER"),
	SSHAuthSock:           EnvVarKey("SSH_AUTH_SOCK"),
	CloudflareAPIToken:    EnvVarKey("CLOUDFLARE_API_TOKEN"),
	CFAPIToken:            EnvVarKey("CF_API_TOKEN"),
	OllamaHost:            EnvVarKey("OLLAMA_HOST"),
	HarnessPollTimeout:    EnvVarKey("G8E_HARNESS_POLL_TIMEOUT"),
	HarnessLLMProvider:    EnvVarKey("G8E_HARNESS_LLM_PROVIDER"),
	HarnessLLMModel:       EnvVarKey("G8E_HARNESS_LLM_MODEL"),
	HarnessLLMEndpoint:    EnvVarKey("G8E_HARNESS_LLM_ENDPOINT"),
	TestReexec:            EnvVarKey("G8E_TEST_REEXEC"),
}
