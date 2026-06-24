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

	// Operator PKI file paths
	OperatorKeyPath        string
	OperatorCertPath       string
	OperatorChainPath      string
	WardenPubPath          string
	RootCAKeyPath          string
	TrustedSignersDir      string
	ClientPkiDir           string
	ClientOperatorKeyPath  string
	ClientOperatorCertPath string

	// Secrets file paths
	SessionEncKeyPath   string
	BootstrapDigestPath string

	// Log paths
	LogDir          string
	OperatorLogFile string

	// Storage DB paths
	ExecutionVaultDBPath string
	ReplayStoreDBPath    string
	LedgerDir            string

	// Demo paths
	DemosDir                         string
	DemosHealthcareDir               string
	DemosFinanceDir                  string
	DemosGovDir                      string
	DemosSecureDataDir               string
	DemosHealthcareTargetDataDir     string
	DemosHealthcareDoctrineDir       string
	DemosHealthcarePARequestsPath    string
	DemosHealthcareComposePath       string
	DemosHealthcareDoctrineHIPAAPath string
	DemosSecureDataDoctrineDir       string
	DemosSecureDataDoctrinePath      string
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

	OperatorKeyPath        string
	OperatorCertPath       string
	OperatorChainPath      string
	WardenPubPath          string
	RootCAKeyPath          string
	TrustedSignersDir      string
	ClientPkiDir           string
	ClientOperatorKeyPath  string
	ClientOperatorCertPath string

	SessionEncKeyPath   string
	BootstrapDigestPath string

	LogDir          string
	OperatorLogFile string

	ExecutionVaultDBPath string
	ReplayStoreDBPath    string
	LedgerDir            string

	DemosDir                         string
	DemosHealthcareDir               string
	DemosFinanceDir                  string
	DemosGovDir                      string
	DemosSecureDataDir               string
	DemosHealthcareTargetDataDir     string
	DemosHealthcareDoctrineDir       string
	DemosHealthcarePARequestsPath    string
	DemosHealthcareComposePath       string
	DemosHealthcareDoctrineHIPAAPath string
	DemosSecureDataDoctrineDir       string
	DemosSecureDataDoctrinePath      string
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

	OperatorKeyPath:        ".g8e/pki/operator.key",
	OperatorCertPath:       ".g8e/pki/operator.crt",
	OperatorChainPath:      ".g8e/pki/operator.chain.pem",
	WardenPubPath:          ".g8e/pki/warden_pub.pem",
	RootCAKeyPath:          ".g8e/pki/root/root_ca.key",
	TrustedSignersDir:      ".g8e/pki/trusted_signers",
	ClientPkiDir:           ".g8e/pki/client",
	ClientOperatorKeyPath:  ".g8e/pki/client/operator.key",
	ClientOperatorCertPath: ".g8e/pki/client/operator.crt",

	SessionEncKeyPath:   ".g8e/secrets/session_encryption_key",
	BootstrapDigestPath: ".g8e/secrets/bootstrap_digest.json",

	LogDir:          ".g8e/logs",
	OperatorLogFile: ".g8e/logs/operator.log",

	ExecutionVaultDBPath: ".g8e/data/execution_vault.db",
	ReplayStoreDBPath:    ".g8e/data/replay_store.db",
	LedgerDir:            ".g8e/data/ledger",

	DemosDir:                         "demos",
	DemosHealthcareDir:               "demos/healthcare",
	DemosFinanceDir:                  "demos/finance",
	DemosGovDir:                      "demos/gov",
	DemosSecureDataDir:               "demos/secure-data",
	DemosHealthcareTargetDataDir:     "demos/healthcare/target-data",
	DemosHealthcareDoctrineDir:       "demos/healthcare/doctrine",
	DemosHealthcarePARequestsPath:    "demos/healthcare/target-data/pa_requests.json",
	DemosHealthcareComposePath:       "demos/healthcare/compose.yml",
	DemosHealthcareDoctrineHIPAAPath: "demos/healthcare/doctrine/phi_hipaa_doctrine.json",
	DemosSecureDataDoctrineDir:       "demos/secure-data/doctrine",
	DemosSecureDataDoctrinePath:      "demos/secure-data/doctrine/secure_data_transfer_doctrine.json",
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
	Infra.PkiRootDir = pathutil.SafeJoin(Infra.PkiDir, "root")
	Infra.PkiAuthoritiesDir = pathutil.SafeJoin(Infra.PkiDir, "authorities")
	Infra.PkiIssuedHubDir = pathutil.SafeJoin(Infra.PkiDir, "issued/hub")
	Infra.PkiIssuedGatewayPeerDir = pathutil.SafeJoin(Infra.PkiDir, "issued/gateway-peer")
	Infra.PkiTrustDir = pathutil.SafeJoin(Infra.PkiDir, "trust")
	Infra.PkiRevocationDir = pathutil.SafeJoin(Infra.PkiDir, "revocation")
	Infra.ActuatorPubJSONPath = pathutil.SafeJoin(Infra.PkiDir, constants.ActuatorPubJSONFilename)
	Infra.ActuatorPubPEMPath = pathutil.SafeJoin(Infra.PkiDir, constants.ActuatorPubPEMFilename)

	Infra.OperatorKeyPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorKey)
	Infra.OperatorCertPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorCert)
	Infra.OperatorChainPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorChain)
	Infra.WardenPubPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileWardenPub)
	Infra.RootCAKeyPath = pathutil.SafeJoin(Infra.PkiRootDir, constants.PkiFileRootCAKey)
	Infra.TrustedSignersDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrustedSigners)
	Infra.ClientPkiDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirClient)
	Infra.ClientOperatorKeyPath = pathutil.SafeJoin(Infra.ClientPkiDir, constants.PkiFileOperatorKey)
	Infra.ClientOperatorCertPath = pathutil.SafeJoin(Infra.ClientPkiDir, constants.PkiFileOperatorCert)

	Infra.SessionEncKeyPath = pathutil.SafeJoin(Infra.SecretsDir, constants.SecretsFileSessionEncryptionKey)
	Infra.BootstrapDigestPath = pathutil.SafeJoin(Infra.SecretsDir, constants.SecretsFileBootstrapDigest)

	Infra.LogDir = pathutil.SafeJoin(Infra.RuntimeDir, constants.LogDirname)
	Infra.OperatorLogFile = pathutil.SafeJoin(Infra.LogDir, OperatorLogPath)

	Infra.ExecutionVaultDBPath = pathutil.SafeJoin(Infra.DataDir, constants.ExecutionVaultDBFilename)
	Infra.ReplayStoreDBPath = pathutil.SafeJoin(Infra.DataDir, constants.ReplayStoreDBFilename)
	Infra.LedgerDir = pathutil.SafeJoin(Infra.DataDir, constants.LedgerDirname)

	Infra.DemosDir = pathutil.SafeJoin(baseDir, constants.DemosDirname)
	Infra.DemosHealthcareDir = pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgHealthcare)
	Infra.DemosFinanceDir = pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgFinance)
	Infra.DemosGovDir = pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgGov)
	Infra.DemosSecureDataDir = pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgSecureData)
	Infra.DemosHealthcareTargetDataDir = pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosTargetDataDir)
	Infra.DemosHealthcareDoctrineDir = pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosDoctrineDir)
	Infra.DemosHealthcarePARequestsPath = pathutil.SafeJoin(Infra.DemosHealthcareTargetDataDir, constants.DemosPARequestsFile)
	Infra.DemosHealthcareComposePath = pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosComposeFile)
	Infra.DemosHealthcareDoctrineHIPAAPath = pathutil.SafeJoin(Infra.DemosHealthcareDoctrineDir, constants.DemosHIPAADoctrineFile)
	Infra.DemosSecureDataDoctrineDir = pathutil.SafeJoin(Infra.DemosSecureDataDir, constants.DemosDoctrineDir)
	Infra.DemosSecureDataDoctrinePath = pathutil.SafeJoin(Infra.DemosSecureDataDoctrineDir, constants.DemosSecureDataDoctrineFile)

	GatewayIDPath = pathutil.SafeJoin(Infra.DataDir, constants.GatewayIDFilename)
	NetworkIdentityPath = pathutil.SafeJoin(Infra.PkiDir, constants.NetworkIdentityFilename)
	PeerCertPath = pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerCertFilename)
	PeerKeyPath = pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerKeyFilename)
	PeerChainPath = pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerChainFilename)
	PkiGatewayKeyPath = pathutil.SafeJoin(Infra.PkiIssuedHubDir, constants.PkiFileGatewayKey)
	return nil
}

// GetSuspendedTransactionsDBPath constructs the suspended transaction database path
// relative to the provided data directory.
func GetSuspendedTransactionsDBPath(dataDir string) string {
	return pathutil.SafeJoin(dataDir, constants.SuspendedTxFilename)
}

// AgentConfigPaths holds precomputed agent configuration paths for a given home directory.
// Call once per command with the user's home directory to avoid repeated filepath.Join calls.
type AgentConfigPaths struct {
	CursorConfigDir    string
	CursorConfigPath   string
	DevinConfigDir     string
	DevinConfigPath    string
	GeminiConfigDir    string
	GeminiConfigPath   string
	GooseConfigDir     string
	GooseConfigPath    string
	VSCodeConfigDir    string
	VSCodeConfigPath   string
	CodeiumConfigDir   string
	CodeiumConfigPath  string
	TabbyConfigDir     string
	TabbyConfigPath    string
	ContinueConfigDir  string
	ContinueConfigPath string
}

// GetAgentConfigPaths precomputes all agent configuration paths from the given home directory.
// Used by CLI commands that write agent MCP configurations.
func GetAgentConfigPaths(homeDir string) AgentConfigPaths {
	return AgentConfigPaths{
		CursorConfigDir:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirCursor),
		CursorConfigPath:   pathutil.SafeJoin(homeDir, constants.AgentConfigDirCursor, constants.AgentConfigFileMCP),
		DevinConfigDir:     pathutil.SafeJoin(homeDir, constants.AgentConfigDirDevin),
		DevinConfigPath:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirDevin, constants.AgentConfigFileMCPDevin),
		GeminiConfigDir:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirGemini),
		GeminiConfigPath:   pathutil.SafeJoin(homeDir, constants.AgentConfigDirGemini, constants.AgentConfigFileSettings),
		GooseConfigDir:     pathutil.SafeJoin(homeDir, constants.AgentConfigDirGoose),
		GooseConfigPath:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirGoose, constants.AgentConfigFileSettings),
		VSCodeConfigDir:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirVSCode),
		VSCodeConfigPath:   pathutil.SafeJoin(homeDir, constants.AgentConfigDirVSCode, constants.AgentConfigFileMCP),
		CodeiumConfigDir:   pathutil.SafeJoin(homeDir, constants.AgentConfigDirCodeium),
		CodeiumConfigPath:  pathutil.SafeJoin(homeDir, constants.AgentConfigDirCodeium, constants.AgentConfigFileMCP),
		TabbyConfigDir:     pathutil.SafeJoin(homeDir, constants.AgentConfigDirTabby),
		TabbyConfigPath:    pathutil.SafeJoin(homeDir, constants.AgentConfigDirTabby, constants.AgentConfigFileMCP),
		ContinueConfigDir:  pathutil.SafeJoin(homeDir, constants.AgentConfigDirContinue),
		ContinueConfigPath: pathutil.SafeJoin(homeDir, constants.AgentConfigDirContinue, constants.AgentConfigFileSettings),
	}
}

// SSHConfigPaths holds precomputed SSH configuration paths for a given home directory.
// Call once per command with the user's home directory to avoid repeated filepath.Join calls.
type SSHConfigPaths struct {
	ConfigPath      string
	KnownHostsPath  string
	IDE25519KeyPath string
	IDECDSAKeyPath  string
	IDRSAKeyPath    string
}

// GetSSHConfigPaths precomputes all SSH configuration paths from the given home directory.
// Used by SSH connection resolution functions.
func GetSSHConfigPaths(homeDir string) SSHConfigPaths {
	sshDir := pathutil.SafeJoin(homeDir, ".ssh")
	return SSHConfigPaths{
		ConfigPath:      pathutil.SafeJoin(sshDir, "config"),
		KnownHostsPath:  pathutil.SafeJoin(sshDir, "known_hosts"),
		IDE25519KeyPath: pathutil.SafeJoin(sshDir, "id_ed25519"),
		IDECDSAKeyPath:  pathutil.SafeJoin(sshDir, "id_ecdsa"),
		IDRSAKeyPath:    pathutil.SafeJoin(sshDir, "id_rsa"),
	}
}
