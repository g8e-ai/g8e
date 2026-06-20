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

// Package paths manages resolved runtime filesystem paths for the g8e platform.
// All path variables are populated by Init or InitWithBase at program startup.
// String constants (filenames, subdirectory names, system paths) remain in
// internal/constants/paths.go.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pathutil"
)

var mu sync.RWMutex

// Infra holds resolved runtime filesystem paths.
// All paths are relative to the working directory by default.
// Populated by Init or InitWithBase at program startup.
var Infra struct {
	DbPath                      string
	PkiDir                      string
	SecretsDir                  string
	CaCertPath                  string
	AppCertDir                  string
	DocsDir                     string
	ProtocolDir                 string
	ProtocolConstantsDir        string
	ProtocolModelsDir           string
	SshConfigPath               string
	RuntimeDir                  string
	DataDir                     string
	VaultDir                    string
	VaultKeyPath                string
	TestVaultDir                string
	LocalStateDBPath            string
	SuspendedTransactionsDBPath string
	AuditVaultDBPath            string
	RootCAPath                  string
	HubCAPath                   string
	OperatorCAPath              string
	GatewayPeerCAPath           string
	GatewayChainPath            string
	TrustDomainJSONPath         string
	ServiceCertPath             string
	PkiRootDir                  string
	PkiAuthoritiesDir           string
	PkiIssuedHubDir             string
	PkiIssuedGatewayPeerDir     string
	PkiTrustDir                 string
	PkiRevocationDir            string
	PkiBinariesDir              string
	ActuatorPubJSONPath         string
	ActuatorPubPEMPath          string
} = struct {
	DbPath                      string
	PkiDir                      string
	SecretsDir                  string
	CaCertPath                  string
	AppCertDir                  string
	DocsDir                     string
	ProtocolDir                 string
	ProtocolConstantsDir        string
	ProtocolModelsDir           string
	SshConfigPath               string
	RuntimeDir                  string
	DataDir                     string
	VaultDir                    string
	VaultKeyPath                string
	TestVaultDir                string
	LocalStateDBPath            string
	SuspendedTransactionsDBPath string
	AuditVaultDBPath            string
	RootCAPath                  string
	HubCAPath                   string
	OperatorCAPath              string
	GatewayPeerCAPath           string
	GatewayChainPath            string
	TrustDomainJSONPath         string
	ServiceCertPath             string
	PkiRootDir                  string
	PkiAuthoritiesDir           string
	PkiIssuedHubDir             string
	PkiIssuedGatewayPeerDir     string
	PkiTrustDir                 string
	PkiRevocationDir            string
	PkiBinariesDir              string
	ActuatorPubJSONPath         string
	ActuatorPubPEMPath          string
}{
	DbPath:                  ".g8e/data/g8e.db",
	PkiDir:                  ".g8e/pki",
	SecretsDir:              ".g8e/secrets",
	CaCertPath:              ".g8e/pki/trust/g8eg-ca-bundle.pem",
	AppCertDir:              ".g8e/pki/issued/apps",
	DocsDir:                 ".g8e/docs",
	ProtocolDir:             ".g8e/protocol",
	ProtocolConstantsDir:    ".g8e/protocol/constants",
	ProtocolModelsDir:       ".g8e/protocol/models",
	SshConfigPath:           ".g8e/ssh_config",
	RuntimeDir:              ".g8e",
	DataDir:                 ".g8e/data",
	VaultDir:                ".g8e/vault",
	TestVaultDir:            ".g8e/test-vault",
	LocalStateDBPath:        ".g8e/local_state.db",
	AuditVaultDBPath:        ".g8e/audit_vault.db",
	RootCAPath:              ".g8e/pki/root/root_ca.crt",
	HubCAPath:               ".g8e/pki/authorities/hub_ca.crt",
	OperatorCAPath:          ".g8e/pki/authorities/operator_ca.crt",
	GatewayPeerCAPath:       ".g8e/pki/authorities/gateway_peer_ca.crt",
	GatewayChainPath:        ".g8e/pki/issued/hub/operator-gateway.chain.pem",
	TrustDomainJSONPath:     ".g8e/pki/trust/trust-domain.json",
	ServiceCertPath:         ".g8e/pki/issued/hub/operator-gateway.crt",
	PkiRootDir:              ".g8e/pki/root",
	PkiAuthoritiesDir:       ".g8e/pki/authorities",
	PkiIssuedHubDir:         ".g8e/pki/issued/hub",
	PkiIssuedGatewayPeerDir: ".g8e/pki/issued/gateway-peer",
	PkiTrustDir:             ".g8e/pki/trust",
	PkiRevocationDir:        ".g8e/pki/revocation",
	ActuatorPubJSONPath:     ".g8e/pki/Actuator_pub.json",
	ActuatorPubPEMPath:      ".g8e/pki/Actuator_pub.pem",
}

// Mutable path vars that are derived from the base directory at init time.
// These complement Infra for paths accessed as bare variables.
var (
	GatewayIDPath       = ".g8e/data/gateway-id"
	ActuatorPubJSONPath = ".g8e/pki/Actuator_pub.json"
	ActuatorPubPEMPath  = ".g8e/pki/Actuator_pub.pem"
	NetworkIdentityPath = ".g8e/pki/network-identity.json"
	PeerCertPath        = ".g8e/pki/peer/peer.crt"
	PeerKeyPath         = ".g8e/pki/peer/peer.key"
	PeerChainPath       = ".g8e/pki/peer/peer.chain.pem"
	PkiGatewayKeyPath   = ".g8e/pki/issued/hub/operator-gateway.key"
	SwaggerFilePath     = "docs/swagger.json"
	OperatorLogPath     = "operator.log"
)

// Init initializes paths relative to the current working directory.
// Call once at program startup.
func Init() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("paths: failed to get working directory: %w", err)
	}
	return InitWithBase(cwd)
}

// InitWithBase initializes paths relative to baseDir.
// Used by tests and specific startup contexts to override the default cwd behavior.
func InitWithBase(baseDir string) error {
	mu.Lock()
	defer mu.Unlock()

	Infra.RuntimeDir = pathutil.SafeJoin(baseDir, ".g8e")
	Infra.DataDir = pathutil.SafeJoin(baseDir, ".g8e/data")
	Infra.PkiDir = pathutil.SafeJoin(baseDir, ".g8e/pki")
	Infra.SecretsDir = pathutil.SafeJoin(baseDir, ".g8e/secrets")
	Infra.ProtocolDir = pathutil.SafeJoin(baseDir, ".g8e/protocol")
	Infra.VaultDir = pathutil.SafeJoin(baseDir, ".g8e/vault")
	Infra.VaultKeyPath = pathutil.SafeJoin(Infra.VaultDir, "key")

	Infra.ProtocolConstantsDir = pathutil.SafeJoin(Infra.ProtocolDir, "constants")
	Infra.ProtocolModelsDir = pathutil.SafeJoin(Infra.ProtocolDir, "models")
	Infra.DbPath = pathutil.SafeJoin(Infra.DataDir, "g8e.db")
	Infra.LocalStateDBPath = pathutil.SafeJoin(Infra.RuntimeDir, "local_state.db")
	Infra.SuspendedTransactionsDBPath = pathutil.SafeJoin(Infra.DataDir, "suspended_transactions.db")
	Infra.AuditVaultDBPath = pathutil.SafeJoin(Infra.DataDir, "audit_vault.db")
	Infra.CaCertPath = pathutil.SafeJoin(Infra.PkiDir, "trust/g8eg-ca-bundle.pem")
	Infra.AppCertDir = pathutil.SafeJoin(Infra.PkiDir, "issued/apps")
	Infra.DocsDir = pathutil.SafeJoin(baseDir, ".g8e/docs")
	Infra.SshConfigPath = pathutil.SafeJoin(baseDir, ".g8e/ssh_config")
	Infra.TestVaultDir = pathutil.SafeJoin(baseDir, ".g8e/test-vault")
	Infra.RootCAPath = pathutil.SafeJoin(Infra.PkiDir, "root/root_ca.crt")
	Infra.HubCAPath = pathutil.SafeJoin(Infra.PkiDir, "authorities/hub_ca.crt")
	Infra.OperatorCAPath = pathutil.SafeJoin(Infra.PkiDir, "authorities/operator_ca.crt")
	Infra.GatewayPeerCAPath = pathutil.SafeJoin(Infra.PkiDir, "authorities/gateway_peer_ca.crt")
	Infra.GatewayChainPath = pathutil.SafeJoin(Infra.PkiDir, "issued/hub/operator-gateway.chain.pem")
	Infra.TrustDomainJSONPath = pathutil.SafeJoin(Infra.PkiDir, "trust/trust-domain.json")
	Infra.ServiceCertPath = pathutil.SafeJoin(Infra.PkiDir, "issued/hub/operator-gateway.crt")
	Infra.PkiRootDir = filepath.Join(Infra.PkiDir, "root")
	Infra.PkiAuthoritiesDir = filepath.Join(Infra.PkiDir, "authorities")
	Infra.PkiIssuedHubDir = filepath.Join(Infra.PkiDir, "issued/hub")
	Infra.PkiIssuedGatewayPeerDir = filepath.Join(Infra.PkiDir, "issued/gateway-peer")
	Infra.PkiTrustDir = filepath.Join(Infra.PkiDir, "trust")
	Infra.PkiRevocationDir = filepath.Join(Infra.PkiDir, "revocation")
	Infra.ActuatorPubJSONPath = filepath.Join(Infra.PkiDir, constants.ActuatorPubJSONFilename)
	Infra.ActuatorPubPEMPath = filepath.Join(Infra.PkiDir, constants.ActuatorPubPEMFilename)

	GatewayIDPath = filepath.Join(Infra.DataDir, constants.GatewayIDFilename)
	NetworkIdentityPath = filepath.Join(Infra.PkiDir, constants.NetworkIdentityFilename)
	PeerCertPath = filepath.Join(Infra.PkiDir, constants.PeerSubdir, constants.PeerCertFilename)
	PeerKeyPath = filepath.Join(Infra.PkiDir, constants.PeerSubdir, constants.PeerKeyFilename)
	PeerChainPath = filepath.Join(Infra.PkiDir, constants.PeerSubdir, constants.PeerChainFilename)
	PkiGatewayKeyPath = filepath.Join(Infra.PkiIssuedHubDir, constants.PkiFileGatewayKey)
	return nil
}

// GetSuspendedTransactionsDBPath constructs the suspended transaction database path
// relative to the provided data directory.
func GetSuspendedTransactionsDBPath(dataDir string) string {
	return filepath.Join(dataDir, constants.SuspendedTxFilename)
}
