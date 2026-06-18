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

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/pathutil"
)

// Paths defines canonical G8E filesystem paths.
// All paths are relative to the current working directory by default.
// The binary is fully self-contained and can run from any directory.
var Paths = struct {
	Infra struct {
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
	}
}{
	Infra: struct {
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
	},
}

// InitPaths initializes paths relative to the current working directory.
// This should be called once at program startup.
// All paths are resolved relative to cwd, making the binary fully self-contained.
func InitPaths() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("constants: failed to get working directory: %w", err)
	}
	return InitPathsWithBase(cwd)
}

// InitPathsWithBase initializes paths relative to the specified base directory.
// This allows tests and specific use cases to override the default cwd behavior.
func InitPathsWithBase(baseDir string) error {
	// Resolve all paths relative to baseDir
	Paths.Infra.RuntimeDir = pathutil.SafeJoin(baseDir, ".g8e")
	Paths.Infra.DataDir = pathutil.SafeJoin(baseDir, ".g8e/data")
	Paths.Infra.PkiDir = pathutil.SafeJoin(baseDir, ".g8e/pki")
	Paths.Infra.SecretsDir = pathutil.SafeJoin(baseDir, ".g8e/secrets")
	Paths.Infra.ProtocolDir = pathutil.SafeJoin(baseDir, ".g8e/protocol")
	Paths.Infra.VaultDir = pathutil.SafeJoin(baseDir, ".g8e/vault")
	Paths.Infra.VaultKeyPath = pathutil.SafeJoin(Paths.Infra.VaultDir, "key")

	// Update derived paths
	Paths.Infra.ProtocolConstantsDir = pathutil.SafeJoin(Paths.Infra.ProtocolDir, "constants")
	Paths.Infra.ProtocolModelsDir = pathutil.SafeJoin(Paths.Infra.ProtocolDir, "models")
	Paths.Infra.DbPath = pathutil.SafeJoin(Paths.Infra.DataDir, "g8e.db")
	Paths.Infra.LocalStateDBPath = pathutil.SafeJoin(Paths.Infra.RuntimeDir, "local_state.db")
	Paths.Infra.SuspendedTransactionsDBPath = pathutil.SafeJoin(Paths.Infra.DataDir, "suspended_transactions.db")
	Paths.Infra.AuditVaultDBPath = pathutil.SafeJoin(Paths.Infra.DataDir, "audit_vault.db")
	Paths.Infra.CaCertPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "trust/g8eg-ca-bundle.pem")
	Paths.Infra.AppCertDir = pathutil.SafeJoin(Paths.Infra.PkiDir, "issued/apps")
	Paths.Infra.DocsDir = pathutil.SafeJoin(baseDir, ".g8e/docs")
	Paths.Infra.SshConfigPath = pathutil.SafeJoin(baseDir, ".g8e/ssh_config")
	Paths.Infra.TestVaultDir = pathutil.SafeJoin(baseDir, ".g8e/test-vault")
	Paths.Infra.RootCAPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "root/root_ca.crt")
	Paths.Infra.HubCAPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "authorities/hub_ca.crt")
	Paths.Infra.OperatorCAPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "authorities/operator_ca.crt")
	Paths.Infra.GatewayPeerCAPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "authorities/gateway_peer_ca.crt")
	Paths.Infra.GatewayChainPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "issued/hub/operator-gateway.chain.pem")
	Paths.Infra.TrustDomainJSONPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "trust/trust-domain.json")
	Paths.Infra.ServiceCertPath = pathutil.SafeJoin(Paths.Infra.PkiDir, "issued/hub/operator-gateway.crt")
	Paths.Infra.PkiRootDir = filepath.Join(Paths.Infra.PkiDir, "root")
	Paths.Infra.PkiAuthoritiesDir = filepath.Join(Paths.Infra.PkiDir, "authorities")
	Paths.Infra.PkiIssuedHubDir = filepath.Join(Paths.Infra.PkiDir, "issued/hub")
	Paths.Infra.PkiIssuedGatewayPeerDir = filepath.Join(Paths.Infra.PkiDir, "issued/gateway-peer")
	Paths.Infra.PkiTrustDir = filepath.Join(Paths.Infra.PkiDir, "trust")
	Paths.Infra.PkiRevocationDir = filepath.Join(Paths.Infra.PkiDir, "revocation")
	Paths.Infra.ActuatorPubJSONPath = filepath.Join(Paths.Infra.PkiDir, ActuatorPubJSONFilename)
	Paths.Infra.ActuatorPubPEMPath = filepath.Join(Paths.Infra.PkiDir, ActuatorPubPEMFilename)

	// Update hardcoded path constants
	GatewayIDPath = filepath.Join(Paths.Infra.DataDir, GatewayIDFilename)
	NetworkIdentityPath = filepath.Join(Paths.Infra.PkiDir, NetworkIdentityFilename)
	PeerCertPath = filepath.Join(Paths.Infra.PkiDir, PeerSubdir, PeerCertFilename)
	PeerKeyPath = filepath.Join(Paths.Infra.PkiDir, PeerSubdir, PeerKeyFilename)
	PeerChainPath = filepath.Join(Paths.Infra.PkiDir, PeerSubdir, PeerChainFilename)
	PkiGatewayKeyPath = filepath.Join(Paths.Infra.PkiIssuedHubDir, PkiFileGatewayKey)
	return nil
}

// GetSuspendedTransactionsDBPath constructs the suspended transaction database path
// relative to the provided data directory.
func GetSuspendedTransactionsDBPath(dataDir string) string {
	return filepath.Join(dataDir, SuspendedTxFilename)
}

// System path constants for critical system directories and files
const (
	PathEtc                                                   = "/etc"
	PathEtcPasswd                                             = "/etc/passwd"
	PathEtcShadow                                             = "/etc/shadow"
	PathEtcGroup                                              = "/etc/group"
	PathEtcGshadow                                            = "/etc/gshadow"
	PathEtcSudoers                                            = "/etc/sudoers"
	PathEtcSudoersD                                           = "/etc/sudoers.d/"
	PathEtcSshSshdConfig                                      = "/etc/ssh/sshd_config"
	PathEtcSshSshConfig                                       = "/etc/ssh/ssh_config"
	PathEtcPamD                                               = "/etc/pam.d/"
	PathEtcSecurity                                           = "/etc/security/"
	PathEtcLdSoConf                                           = "/etc/ld.so.conf"
	PathEtcLdSoPreload                                        = "/etc/ld.so.preload"
	PathEtcHosts                                              = "/etc/hosts"
	PathEtcResolvConf                                         = "/etc/resolv.conf"
	PathEtcFstab                                              = "/etc/fstab"
	PathEtcCrontab                                            = "/etc/crontab"
	PathEtcCronD                                              = "/etc/cron.d/"
	PathEtcCronDaily                                          = "/etc/cron.daily/"
	PathEtcCronHourly                                         = "/etc/cron.hourly/"
	PathEtcInitD                                              = "/etc/init.d/"
	PathEtcSystemdSystem                                      = "/etc/systemd/system/"
	PathEtcRcLocal                                            = "/etc/rc.local"
	PathEtcProfile                                            = "/etc/profile"
	PathEtcProfileD                                           = "/etc/profile.d/"
	PathEtcBashBashrc                                         = "/etc/bash.bashrc"
	PathEtcEnvironment                                        = "/etc/environment"
	PathEtcSelinux                                            = "/etc/selinux/"
	PathEtcApparmor                                           = "/etc/apparmor/"
	PathEtcApparmorD                                          = "/etc/apparmor.d/"
	PathBoot                                                  = "/boot"
	PathRootSsh                                               = "/root/.ssh/"
	PathRootBashrc                                            = "/root/.bashrc"
	PathRootBashProfile                                       = "/root/.bash_profile"
	PathRootProfile                                           = "/root/.profile"
	PathBin                                                   = "/bin"
	PathSbin                                                  = "/sbin"
	PathUsrBin                                                = "/usr/bin"
	PathUsrSbin                                               = "/usr/sbin"
	PathUsrLocalBin                                           = "/usr/local/bin"
	PathUsrLocalSbin                                          = "/usr/local/sbin"
	PathLib                                                   = "/lib"
	PathLib64                                                 = "/lib64"
	PathUsrLib                                                = "/usr/lib"
	PathProc                                                  = "/proc"
	PathSys                                                   = "/sys"
	PathDev                                                   = "/dev"
	PathVar                                                   = "/var"
	PathTmp                                                   = "/tmp"
	PathVarLib                                                = "/var/lib"
	PathVarWWW                                                = "/var/www"
	PathOpt                                                   = "/opt"
	PathHome                                                  = "/home"
	PathEtcHostname                                           = "/etc/hostname"
	PathEtcMachineID                                          = "/etc/machine-id"
	PathVarLibDbusMachineID                                   = "/var/lib/dbus/machine-id"
	PathProcSysKernelRandomBootID                             = "/proc/sys/kernel/random/boot_id"
	PathProcSelfCgroup                                        = "/proc/self/cgroup"
	PathProcSelfMountinfo                                     = "/proc/self/mountinfo"
	PathLibraryPreferencesSystemConfigurationPreferencesPlist = "/Library/Preferences/SystemConfiguration/preferences.plist"
)

// PKI filesystem constants for subdirectories and filenames.
const (
	PkiDirname           = "pki"
	PkiSubdirRoot        = "root"
	PkiSubdirAuthorities = "authorities"
	PkiSubdirIssued      = "issued"
	PkiSubdirTrust       = "trust"
	PkiSubdirRevocation  = "revocation"
	PkiSubdirBinaries    = "binaries"
	PkiSubdirClient      = "client"

	// File extensions
	FileExtCert = ".crt"
	FileExtKey  = ".key"
	FileExtPEM  = ".pem"
	FileExtJSON = ".json"

	PkiFileRootCA          = "root_ca.crt"
	PkiFileRootCAKey       = "root_ca.key"
	PkiFileHubCA           = "hub_ca.crt"
	PkiFileOperatorCA      = "operator_ca.crt"
	PkiFileGatewayPeerCA   = "gateway_peer_ca.crt"
	PkiFileGatewayBundle   = "g8eg-ca-bundle.pem"
	PkiFileRootBundle      = "root.pem"
	PkiFileOperatorBundle  = "operator-bundle.pem"
	PkiFileTrustDomainJSON = "trust-domain.json"
	PkiFileWardenPub       = "warden_pub.pem"
	PkiFileOperatorCert    = "operator.crt"
	PkiFileOperatorKey     = "operator.key"
	PkiFileOperatorChain   = "operator.chain.pem"
	PkiFileGatewayCert     = "operator-gateway.crt"
	PkiFileGatewayKey      = "operator-gateway.key"
	PkiFileGatewayChain    = "operator-gateway.chain.pem"
	PkiFileBootstrapCA     = "bootstrap_ca.crt"
	PkiFileBootstrapBundle = "bootstrap-bundle.pem"

	DbFilename             = "g8e.db"
	VaultKeyFilename       = "key"
	VaultNewKeyFilename    = "key.new"
	SuspendedTxFilename    = "suspended_transactions.db"
	ReceiptsFilename       = "receipts.json"
	ReceiptsExportFilename = "receipts-export.json"

	SecretsFileSessionEncryptionKey = "session_encryption_key"
	SecretsFileBootstrapDigest      = "bootstrap_digest.json"
	SecretsFileActuatorSigningKey   = "actuator_signing_key"
	SecretsFileActuatorKeyID        = "actuator_key_id"
	SecretsFileAuditorHMACKey       = "auditor_hmac_key"
	SecretsFileConsensusSigningKey  = "consensus_signing_key"
	SecretsFileNotarySigningKey     = "notary_signing_key"
	SecretsFileOperatorPrivateKey   = "operator_private_key"
	SecretsFileCLIPrivateKey        = "cli_private_key"
	SecretsFileSessionToken         = "session_token"

	DemosDirname     = "demos"
	DemosComposeFile = "compose.yml"
	DemosBinDirname  = "bin"
	DemosBinaryName  = "g8e"

	SwaggerFilename          = "swagger.json"
	ComplianceReportFilename = "compliance-report.json"

	PkiSubdirHub            = "hub"
	PkiSubdirGatewayPeer    = "gateway-peer"
	PkiSubdirApps           = "apps"
	PkiSubdirTrustedSigners = "trusted_signers"

	// Directory names
	BinDirname = "bin"
	LogDirname = "logs"

	// CLI certificate and key filenames
	CliCertFilename     = "cli.crt"
	CliKeyFilename      = "cli.key"
	CredentialsFilename = "credentials"

	// Gateway-specific filenames
	GatewayIDFilename       = "gateway-id"
	ActuatorPubJSONFilename = "Actuator_pub.json"
	ActuatorPubPEMFilename  = "Actuator_pub.pem"
	NetworkIdentityFilename = "network-identity.json"

	// Operator-specific filenames
	OperatorPIDFilename     = "operator.pid"
	OperatorPostureFilename = "operator.posture"

	// Peer certificate filenames
	PeerCertFilename  = "peer.crt"
	PeerKeyFilename   = "peer.key"
	PeerChainFilename = "peer.chain.pem"
	PeerSubdir        = "peer"
)

// FULL path constants (relative from runtime directory)
// These are variables to allow test path overrides via InitPathsWithBase
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

// Project root discovery constants for test path initialization
const (
	ProjectRootFromTestDir    = "../../"
	ProjectRootFromCurrentDir = "."

	// Directory names (single path segment, no separators)
	RuntimeDirname    = ".g8e"
	DataDirname       = "data"
	VaultDirname      = "vault"
	SecretsDirname    = "secrets"
	LedgerDirname     = "ledger"
	SshConfigFilename = "ssh_config"
	PidDirname        = "pids"

	// Ledger-specific directory and file names
	FilesDirname      = "files"
	SessionsDirname   = "sessions"
	GitDirname        = ".git"
	GitignoreFilename = ".gitignore"

	// Key filenames
	MasterKeyFilename = ".master_key"
	PublicKeySuffix   = ".pub"

	// Storage DB filenames (used with filepath.Join)
	TokenStoreDBFilename     = "token_store.db"
	ReplayStoreDBFilename    = "replay_store.db"
	ExecutionVaultDBFilename = "execution_vault.db"

	// Full relative storage DB paths (relative to project root, used as config defaults)
	TokenStoreDBPath           = RuntimeDirname + "/token_store.db"
	ReplayStoreDBPath          = RuntimeDirname + "/replay_store.db"
	ExecutionVaultDBPath       = RuntimeDirname + "/execution_vault.db"
	SuspendedTransactionDBPath = RuntimeDirname + "/" + SuspendedTxFilename

	// Agent config directory and file names (relative to home directory)
	AgentConfigDirCursor    = ".cursor"
	AgentConfigDirDevin     = ".codeium/windsurf"
	AgentConfigDirGemini    = ".gemini"
	AgentConfigDirGoose     = ".goose"
	AgentConfigDirVSCode    = ".vscode"
	AgentConfigDirCodeium   = ".codeium"
	AgentConfigDirTabby     = ".tabby"
	AgentConfigDirContinue  = ".continue"
	AgentConfigFileMCP      = "mcp.json"
	AgentConfigFileMCPDevin = "mcp_config.json"
	AgentConfigFileSettings = "settings.json"
	AgentConfigFileAider    = ".aider.conf.yml"

	// File permission modes (octal)
	PermDirPrivate  = 0700 // rwx------
	PermFilePrivate = 0600 // rw-------
	PermFilePublic  = 0644 // rw-r--r--

	// API path constants
	APIPathAuthDeviceEnroll = "/api/v1/auth/device/enroll"
	APIPathPKIDevicesEnroll = "/api/v1/pki/devices/enroll"
	WellKnownPKICABundle    = "/.well-known/g8e/pki/ca-bundle"

	// Default path descriptions for CLI help text
	DefaultVaultDirDesc     = ".g8e/vault"
	DefaultVaultKeyDesc     = ".g8e/secrets/vault.key"
	DefaultOperatorKeyDesc  = ".g8e/pki/operator.key"
	DefaultClientKeyDesc    = ".g8e/pki/client.key"
	DefaultOperatorCertDesc = ".g8e/pki/operator.crt"
	DefaultClientCertDesc   = ".g8e/pki/client.crt"

	// Default path constants for CLI config (relative paths)
	DefaultDataDir    = RuntimeDirname + "/" + DataDirname
	DefaultPKIDir     = RuntimeDirname + "/" + PkiDirname
	DefaultSecretsDir = RuntimeDirname + "/" + SecretsDirname

	// Test-specific filename constants (for isolated test environments)
	TestEmptyMachineIDFilename = "empty-machine-id"
	TestDBSubdirName           = "db"
	TestLedgerDirname          = "ledger"
	TestGitDirname             = ".git"
)
