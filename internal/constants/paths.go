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
	PathVarLog                                                = "/var/log"
	PathVarLogDmesg                                           = "/var/log/dmesg"
	PathVarWWW                                                = "/var/www"
	PathOpt                                                   = "/opt"
	PathHome                                                  = "/home"
	PathEtcHostname                                           = "/etc/hostname"
	PathEtcMachineID                                          = "/etc/machine-id"
	PathVarLibDbusMachineID                                   = "/var/lib/dbus/machine-id"
	PathProcSysKernelRandomBootID                             = "/proc/sys/kernel/random/boot_id"
	PathProcSelfCgroup                                        = "/proc/self/cgroup"
	PathProcSelfMountinfo                                     = "/proc/self/mountinfo"
	PathProcLoadAvg                                           = "/proc/loadavg"
	PathProcMemInfo                                           = "/proc/meminfo"
	PathProcNet                                               = "/proc/net"
	PathProcNetTCP                                            = "/proc/net/tcp"
	PathProcNetUDP                                            = "/proc/net/udp"
	PathProcNetTCP6                                           = "/proc/net/tcp6"
	PathProcNetUDP6                                           = "/proc/net/udp6"
	PathProcNetRaw                                            = "/proc/net/raw"
	PathLibraryPreferencesSystemConfigurationPreferencesPlist = "/Library/Preferences/SystemConfiguration/preferences.plist"

	// SSH paths
	PathEtcSshKnownHosts      = "/etc/ssh/known_hosts"
	PathEtcSshSshKnownHosts   = "/etc/ssh/ssh_known_hosts"
	PathHomeSshKnownHosts     = "$HOME/.ssh/known_hosts"
	PathWindowsSshKnownHosts  = "$USERPROFILE\\.ssh\\known_hosts"
	PathWindowsProgramDataSsh = "C:\\ProgramData\\ssh\\known_hosts"
	PathWindowsSystemRoot     = "SystemRoot"
	PathWindowsHostsFile      = "System32\\drivers\\etc\\hosts"
)

// Environment variable constants
const (
	EnvPathDefault = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
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

	DemosDirname         = "demos"
	DemosComposeFile     = "compose.yml"
	DemosBinDirname      = "bin"
	DemosBinaryName      = "g8e"
	DemosTargetDataDir   = "target-data"
	DemosDoctrineDir    = "doctrine"
	DemosPARequestsFile  = "pa_requests.json"
	DemosHIPAADoctrineFile = "phi_hipaa_doctrine.json"
	DemosSecureDataDoctrineFile = "secure_data_transfer_doctrine.json"
	DemosOrgHealthcare   = "healthcare"
	DemosOrgFinance      = "finance"
	DemosOrgGov          = "gov"
	DemosOrgSecureData   = "secure-data"

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
	OperatorBinaryFilename  = "g8e-operator"

	// Peer certificate filenames
	PeerCertFilename  = "peer.crt"
	PeerKeyFilename   = "peer.key"
	PeerChainFilename = "peer.chain.pem"
	PeerSubdir        = "peer"
)

// Project root discovery constants for test path initialization
const (
	ProjectRootFromTestDir    = "../../"
	ProjectRootFromCurrentDir = "."

	// Directory names (single path segment, no separators)
	RuntimeDirname        = ".g8e"
	DataDirname           = "data"
	VaultDirname          = "vault"
	SecretsDirname        = "secrets"
	LedgerDirname         = "ledger"
	SshConfigFilename     = "ssh_config"
	SshDirname            = ".ssh"
	SshConfigBasename     = "config"
	SshKnownHostsBasename = "known_hosts"
	PidDirname            = "pids"

	// Ledger-specific directory and file names
	FilesDirname      = "files"
	SessionsDirname   = "sessions"
	GitDirname        = ".git"
	GitignoreFilename = ".gitignore"
	GoModFilename     = "go.mod"

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
	TestFileTxtFilename        = "test.txt"
	TestNonexistentTxtFilename = "nonexistent.txt"
	TestResultsDirname         = "test-results"

	// Test-specific directory names
	TestVaultDirname = "test-vault"
	TestProtocolDirname = "protocol"
	TestDocsDirname = "docs"

	// Test-specific database filenames
	TestLocalStateDBFilename = "local_state.db"
	TestAuditVaultDBFilename = "audit_vault.db"

	// File system listing limits
	FsListMaxDepth       = 3
	FsListDefaultDepth   = 0
	FsListMaxEntries     = 500
	FsListDefaultEntries = 100
	FsListBatchSize      = 100

	// Temporary file suffix for atomic writes
	TmpFileSuffix = ".tmp"

	// Reporting output directory and file names
	ReportsDirname                 = "reports"
	ReportReceiptsFilename         = "receipts.csv"
	ReportSessionsFilename         = "sessions.csv"
	ReportEventsFilename           = "events.csv"
	ReportFileMutationsFilename    = "file_mutations.csv"
	ReportExecutionsFilename       = "executions.csv"
	ReportFileDiffsFilename        = "file_diffs.csv"
	ReportCommitmentsFilename      = "commitments.csv"
	ReportLedgerCommitsFilename    = "ledger_commits.csv"
	ReportLedgerMerkleRootFilename = "ledger_merkle_root.csv"
	ReportReplayNoncesFilename     = "replay_nonces.csv"
	ReportSuspendedTxFilename      = "suspended_transactions.csv"
	ReportVerificationFilename     = "verification_summary.csv"
	ReportManifestFilename         = "manifest.csv"
)
