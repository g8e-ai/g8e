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
	ClientCert            EnvVarKey
	ClientKey             EnvVarKey
	CABundle              EnvVarKey
	GatewayURL            EnvVarKey
	AppID                 EnvVarKey
	AppCert               EnvVarKey
	AppKey                EnvVarKey
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
	ClientCert:            EnvVarKey("G8E_CLIENT_CERT"),
	ClientKey:             EnvVarKey("G8E_CLIENT_KEY"),
	CABundle:              EnvVarKey("G8E_CA_BUNDLE"),
	GatewayURL:            EnvVarKey("G8E_GATEWAY_URL"),
	AppID:                 EnvVarKey("G8E_APP_ID"),
	AppCert:               EnvVarKey("G8E_APP_CERT"),
	AppKey:                EnvVarKey("G8E_APP_KEY"),
}
