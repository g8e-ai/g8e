// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package constants

// EnvVarKey is a typed string for environment variable names.
type EnvVarKey string

// EnvVar groups all environment variable name constants consumed by g8eo.
var EnvVar = struct {
	TribunalID            EnvVarKey
	TribunalURL           EnvVarKey
	TribunalBootstrap     EnvVarKey
	VaultDir              EnvVarKey
	VaultKey              EnvVarKey
	OperatorSessionID     EnvVarKey
	PasskeyRpID           EnvVarKey
	PasskeyRpName         EnvVarKey
	PasskeyRpOrigins      EnvVarKey
	PublicBaseURL         EnvVarKey
	AllowedOrigins        EnvVarKey
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
}{
	TribunalID:            EnvVarKey("G8E_TRIBUNAL_ID"),
	TribunalURL:           EnvVarKey("G8E_TRIBUNAL_URL"),
	TribunalBootstrap:     EnvVarKey("G8E_TRIBUNAL_BOOTSTRAP"),
	VaultDir:              EnvVarKey("G8E_VAULT_DIR"),
	VaultKey:              EnvVarKey("G8E_VAULT_KEY"),
	OperatorSessionID:     EnvVarKey("G8E_OPERATOR_SESSION_ID"),
	PasskeyRpID:           EnvVarKey("G8E_PASSKEY_RP_ID"),
	PasskeyRpName:         EnvVarKey("G8E_PASSKEY_RP_NAME"),
	PasskeyRpOrigins:      EnvVarKey("G8E_PASSKEY_RP_ORIGINS"),
	PublicBaseURL:         EnvVarKey("G8E_PUBLIC_BASE_URL"),
	AllowedOrigins:        EnvVarKey("G8E_ALLOWED_ORIGINS"),
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
}
