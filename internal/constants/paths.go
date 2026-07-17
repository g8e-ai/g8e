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

// System paths (Unix) for critical system directories and files.
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
	PathBinBash                                               = "/bin/bash"
	PathUsrBinBash                                            = "/usr/bin/bash"
	PathUsrBinSh                                              = "/usr/bin/sh"
	PathBinSh                                                 = "/bin/sh"
	PathLib                                                   = "/lib"
	PathLib64                                                 = "/lib64"
	PathUsrLib                                                = "/usr/lib"
	PathProc                                                  = "/proc"
	PathSys                                                   = "/sys"
	PathDev                                                   = "/dev"
	PathVar                                                   = "/var"
	PathVarLogDmesg                                           = "/var/log/dmesg"
	PathTmp                                                   = "/tmp"
	PathHome                                                  = "/home"
	PathEtcHostname                                           = "/etc/hostname"
	PathEtcMachineID                                          = "/etc/machine-id"
	PathVarLibDbusMachineID                                   = "/var/lib/dbus/machine-id"
	PathProcSysKernelRandomBootID                             = "/proc/sys/kernel/random/boot_id"
	PathProcMounts                                            = "/proc/mounts"
	PathProcLoadAvg                                           = "/proc/loadavg"
	PathProcMemInfo                                           = "/proc/meminfo"
	PathProcNet                                               = "/proc/net"
	PathProcNetTCP                                            = "/proc/net/tcp"
	PathProcNetUDP                                            = "/proc/net/udp"
	PathProcNetTCP6                                           = "/proc/net/tcp6"
	PathProcNetUDP6                                           = "/proc/net/udp6"
	PathProcNetRaw                                            = "/proc/net/raw"
	PathProcUptime                                            = "/proc/uptime"
	PathProcStat                                              = "/proc/stat"
	PathProcVersion                                           = "/proc/version"
	PathProcOneCmdline                                        = "/proc/1/cmdline"
	PathEtcOSRelease                                          = "/etc/os-release"
	PathEtcTimezone                                           = "/etc/timezone"
	PathEtcLocaltime                                          = "/etc/localtime"
	PathRoot                                                  = "/"
	PathLibraryPreferencesSystemConfigurationPreferencesPlist = "/Library/Preferences/SystemConfiguration/preferences.plist"

	// Relative path components
	PathParentDir = ".."
)

// System paths (Windows) for registry, hosts, and Git Bash locations.
const (
	PathWindowsSystemRoot           = "SystemRoot"
	PathWindowsHostsFile            = "System32\\drivers\\etc\\hosts"
	PathWindowsRegistryCryptography = "SOFTWARE\\Microsoft\\Cryptography"
	PathWindowsRegistryMachineGuid  = "MachineGuid"
	PathWindowsGitBinBash           = "C:\\Program Files\\Git\\bin\\bash.exe"
	PathWindowsGitUsrBinBash        = "C:\\Program Files\\Git\\usr\\bin\\bash.exe"
	PathWindowsGitBinSh             = "C:\\Program Files\\Git\\bin\\sh.exe"
	PathWindowsMsys64Bash           = "C:\\msys64\\usr\\bin\\bash.exe"
	PathWindowsCygwin64Bash         = "C:\\cygwin64\\bin\\bash.exe"

	// Windows temp directory prefixes and filenames for cert store operations
	WindowsTempCertImportPrefix = "g8e-cert-import-*"
	WindowsTempCATrustPrefix    = "g8e-ca-trust-*"
	WindowsTempCertFilename     = "certificate.pem"
)

// SSH path constants for known_hosts and config locations.
const (
	PathEtcSshKnownHosts      = "/etc/ssh/known_hosts"
	PathEtcSshSshKnownHosts   = "/etc/ssh/ssh_known_hosts"
	PathHomeSshKnownHosts     = "$HOME/.ssh/known_hosts"
	PathWindowsSshKnownHosts  = "$USERPROFILE\\.ssh\\known_hosts"
	PathWindowsProgramDataSsh = "C:\\ProgramData\\ssh\\known_hosts"
)

// Environment variable constants.
const (
	EnvPathDefault = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// PKI constants for subdirectories, filenames, and extensions.
const (
	PkiDirname              = "pki"
	PkiSubdirRoot           = "root"
	PkiSubdirAuthorities    = "authorities"
	PkiSubdirIssued         = "issued"
	PkiSubdirTrust          = "trust"
	PkiSubdirRevocation     = "revocation"
	PkiSubdirBinaries       = "binaries"
	PkiSubdirClient         = "client"
	PkiSubdirHub            = "hub"
	PkiSubdirGatewayPeer    = "gateway-peer"
	PkiSubdirApps           = "apps"
	PkiSubdirTrustedSigners = "trusted_signers"

	// File extensions
	FileExtCert = ".crt"
	FileExtKey  = ".key"
	FileExtPEM  = ".pem"
	FileExtJSON = ".json"

	// CA and bundle filenames
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
	PkiFileBootstrapCA     = "bootstrap_ca.crt"
	PkiFileBootstrapBundle = "bootstrap-bundle.pem"

	// Operator certificate and key filenames
	PkiFileOperatorCert  = "operator.crt"
	PkiFileOperatorKey   = "operator.key"
	PkiFileOperatorChain = "operator.chain.pem"

	// Gateway certificate and key filenames
	PkiFileGatewayCert  = "operator-gateway.crt"
	PkiFileGatewayKey   = "operator-gateway.key"
	PkiFileGatewayChain = "operator-gateway.chain.pem"

	// Peer certificate filenames
	PeerCertFilename  = "peer.crt"
	PeerKeyFilename   = "peer.key"
	PeerChainFilename = "peer.chain.pem"
	PeerSubdir        = "peer"

	// CLI certificate and key filenames
	CliCertFilename     = "cli.crt"
	CliKeyFilename      = "cli.key"
	CredentialsFilename = "credentials"
)

// Storage constants for database filenames and paths.
const (
	DbFilename             = "g8e.db"
	VaultKeyFilename       = "key"
	VaultNewKeyFilename    = "key.new"
	VaultHeaderFilename    = "vault.header"
	SuspendedTxFilename    = "suspended_transactions.db"
	ReceiptsFilename       = "receipts.json"
	ReceiptsExportFilename = "receipts-export.json"

	// Storage DB filenames (used with filepath.Join)
	ReplayStoreDBFilename    = "replay_store.db"
	ExecutionVaultDBFilename = "execution_vault.db"
	LocalStateDBFilename     = "local_state.db"
	AuditVaultDBFilename     = "audit_vault.db"

	// Full relative storage DB paths (relative to project root, used as config defaults)
	ReplayStoreDBPath          = RuntimeDirname + "/" + ReplayStoreDBFilename
	ExecutionVaultDBPath       = RuntimeDirname + "/" + ExecutionVaultDBFilename
	SuspendedTransactionDBPath = RuntimeDirname + "/" + SuspendedTxFilename

	// Key filenames
	MasterKeyFilename = ".master_key"
	PublicKeySuffix   = ".pub"
)

// Secrets filenames for bootstrap and runtime secret material.
const (
	SecretsFileSessionEncryptionKey    = "session_encryption_key"
	SecretsFileBootstrapDigest         = "bootstrap_digest.json"
	SecretsFileActuatorSigningKey      = "actuator_signing_key"
	SecretsFileActuatorKeyID           = "actuator_key_id"
	SecretsFileAuditorHMACKey          = "auditor_hmac_key"
	SecretsFileNotarySigningKey        = "notary_signing_key"
	SecretsFileOperatorPrivateKey      = "operator_private_key"
	SecretsFileCLIPrivateKey           = "cli_private_key"
	SecretsFileSessionToken            = "session_token"
	SecretsFileTribunalMemberKeyPrefix = "tribunal_member_"
)

// Demos constants for organization names, doctrine files, and compose config.
const (
	DemosDirname                = "demos"
	DemosComposeFile            = "compose.yml"
	DemosBinDirname             = "bin"
	DemosBinaryName             = "g8e"
	DemosTargetDataDir          = "target-data"
	DemosDoctrineDir            = "doctrine"
	DemosPARequestsFile         = "pa_requests.json"
	DemosHIPAADoctrineFile      = "phi_hipaa_doctrine.json"
	DemosSecureDataDoctrineFile = "secure_data_transfer_doctrine.json"
	DemosDoWDoctrineFile        = "dow_tactical_doctrine.json"
	DemosDHSDoctrineFile        = "dhs_sovereign_doctrine.json"
	DemosSwarmDoctrineFile      = "drone_operations_doctrine.json"
	DemosImagesManifestFile     = "images.json"
	DemosOrgHealthcare          = "healthcare"
	DemosOrgFinance             = "finance"
	DemosOrgGov                 = "gov"
	DemosOrgSecureData          = "secure-data"
	DemosOrgDoW                 = "dow"
	DemosOrgDHS                 = "dhs"
	DemosOrgSwarm               = "swarm"
	DemosOrgFrontend            = "frontend"
)

// Container paths for Docker exec commands in demo environments.
// These are paths inside the g8e Docker containers, not local filesystem paths.
const (
	ContainerRootG8E          = "/root/.g8e"
	ContainerPKIDir           = ContainerRootG8E + "/" + PkiDirname
	ContainerOperatorCert     = ContainerPKIDir + "/" + PkiFileOperatorCert
	ContainerOperatorKey      = ContainerPKIDir + "/" + PkiFileOperatorKey
	ContainerCABundle         = ContainerPKIDir + "/" + PkiSubdirTrust + "/" + PkiFileGatewayBundle
	ContainerDataDir          = ContainerRootG8E + "/" + DataDirname
	ContainerAuditVaultDB     = ContainerDataDir + "/" + AuditVaultDBFilename
	ContainerExecutionVaultDB = ContainerDataDir + "/" + ExecutionVaultDBFilename
	ContainerLedgerFilesDir   = ContainerDataDir + "/" + LedgerDirname + "/" + FilesDirname

	ContainerDoctrineDir   = "/etc/g8e/" + DemosDoctrineDir
	ContainerEnsembleSeed  = "/etc/g8e/ensemble-seed.hex"
	ContainerVerifyOpsPy   = "/app/verify_ops.py"
	ContainerInspectRFPy   = "/app/inspect_rf.py"
	ContainerInspectPNTPy  = "/app/inspect_pnt.py"
	ContainerVerifySlewsPy = "/app/verify_slews.py"
)

// Local binary names for the g8e CLI executable.
const (
	LocalBinaryName        = "./g8e"
	LocalBinaryNameWindows = "./g8e.exe"
)

// Binary image names for process detection (no path prefix).
const (
	BinaryImageName        = "g8e"
	BinaryImageNameWindows = "g8e.exe"
)

// Deploy script filenames served by the gateway.
const (
	DeployScriptFilenameLinux   = "g8e-deploy.sh"
	DeployScriptFilenameWindows = "g8e-deploy.ps1"
)

// Component filenames for gateway, operator, and shared services.
const (
	SwaggerFilename          = "swagger.json"
	ComplianceReportFilename = "compliance-report.json"

	// Gateway-specific filenames
	GatewayIDFilename       = "gateway-id"
	ActuatorPubJSONFilename = "Actuator_pub.json"
	ActuatorPubPEMFilename  = "Actuator_pub.pem"
	NetworkIdentityFilename = "network-identity.json"

	// Operator-specific filenames
	OperatorPIDFilename     = "operator.pid"
	OperatorPostureFilename = "operator.posture"
	OperatorBinaryFilename  = "g8e-operator"
	OperatorLogFilename     = "operator.log"
)

// Runtime directory constants for the .g8e/ state tree.
const (
	ProjectRootFromTestDir = "../../"
	PathCurrentDir         = "."

	RuntimeDirname           = ".g8e"
	DataDirname              = "data"
	VaultDirname             = "vault"
	SecretsDirname           = "secrets"
	LedgerDirname            = "ledger"
	PidDirname               = "pids"
	DocsDirname              = "docs"
	ProtocolDirname          = "protocol"
	ProtocolConstantsDirname = "constants"
	ProtocolModelsDirname    = "models"
	BinDirname               = "bin"
	LogDirname               = "logs"

	// Ledger-specific directory and file names
	FilesDirname      = "files"
	SessionsDirname   = "sessions"
	GitDirname        = ".git"
	GitignoreFilename = ".gitignore"
	GoModFilename     = "go.mod"
)

// SSH config constants for basenames and key filenames.
const (
	SshConfigFilename     = "ssh_config"
	SshDirname            = ".ssh"
	SshConfigBasename     = "config"
	SshKnownHostsBasename = "known_hosts"
	SshKeyEd25519         = "id_ed25519"
	SshKeyECDSA           = "id_ecdsa"
	SshKeyRSA             = "id_rsa"
)

// Agent config constants for AI tool config directories and filenames.
const (
	AgentConfigDirGemini    = ".gemini"
	AgentConfigDirGoose     = ".goose"
	AgentConfigFileMCP      = "mcp.json"
	AgentConfigFileSettings = "settings.json"
)

// API path constants for enrollment and well-known endpoints.
const (
	APIPathAuthDeviceEnroll = "/api/v1/auth/device/enroll"
	APIPathPKIDevicesEnroll = "/api/v1/pki/devices/enroll"
	WellKnownPKICABundle    = "/.well-known/g8e/pki/ca-bundle"
)

// CLI default paths for config and help text (derived from primitives).
const (
	DefaultVaultDirDesc     = RuntimeDirname + "/" + VaultDirname
	DefaultVaultKeyDesc     = RuntimeDirname + "/" + SecretsDirname + "/" + VaultKeyFilename
	DefaultOperatorKeyDesc  = RuntimeDirname + "/" + PkiDirname + "/" + PkiFileOperatorKey
	DefaultClientKeyDesc    = RuntimeDirname + "/" + PkiDirname + "/" + CliKeyFilename
	DefaultOperatorCertDesc = RuntimeDirname + "/" + PkiDirname + "/" + PkiFileOperatorCert
	DefaultClientCertDesc   = RuntimeDirname + "/" + PkiDirname + "/" + CliCertFilename

	DefaultDataDir    = RuntimeDirname + "/" + DataDirname
	DefaultPKIDir     = RuntimeDirname + "/" + PkiDirname
	DefaultSecretsDir = RuntimeDirname + "/" + SecretsDirname
)

// File permission modes (octal).
const (
	PermDirPrivate   = 0700 // rwx------
	PermDirStandard  = 0755 // rwxr-xr-x
	PermFilePrivate  = 0600 // rw-------
	PermFilePublic   = 0644 // rw-r--r--
	PermFileReadOnly = 0400 // r--------
)

// Test-specific constants for isolated test environments.
const (
	TestEmptyMachineIDFilename  = "empty-machine-id"
	TestFileTxtFilename         = "test.txt"
	TestNonexistentTxtFilename  = "nonexistent.txt"
	TestResultsDirname          = "test-results"
	TestVaultDirname            = "test-vault"
	TestSecretManagerDBFilename = "secret_manager_test.db"
	TestCertFilename            = "test-cert.pem"
	TestKeyFilename             = "test-key.pem"

	// Cert test filenames for internal/cli/serve/cert_test.go
	TestCertCrtFilename         = "test.crt"
	TestNonExistentCrtFilename  = "nonexistent.crt"
	TestInvalidPEMFilename      = "invalid.pem"
	TestCorruptCrtFilename      = "corrupt.crt"
	TestCABundleFilename        = "ca-bundle.pem"
	TestDoesNotExistPEMFilename = "does-not-exist.pem"
	TestExplicitPEMFilename     = "explicit.pem"
	TestECPrivateKeyFilename    = "key.pem"
	TestClientCrtFilename       = "client.crt"
	TestClientKeyFilename       = "client.key"
	TestNonExistentKeyFilename  = "nonexistent.key"
	TestInvalidCrtFilename      = "invalid.crt"
	TestInvalidKeyFilename      = "invalid.key"
	TestPkiDirname              = "pki"
	TestNestedDirname           = "nested"
	TestDeepDirname             = "deep"

	// TestTempDirname is the CWD-relative base directory for test temp dirs,
	// replacing system TEMP to keep all test artifacts under the project root.
	TestTempDirname = ".g8e-test-tmp"

	// Test path constants for gateway config and tribunal bootstrap tests
	TestPathVarLibDataDir       = "/var/lib/g8e/data"
	TestPathVarLibPKIDir        = "/var/lib/g8e/pki"
	TestPathVarLibSecretsDir    = "/var/lib/g8e/secrets"
	TestPathVarLibVaultDir      = "/var/lib/g8e/vault"
	TestPathVarLibVaultKey      = "/var/lib/g8e/vault/key"
	TestPathEtcNetworkIdentity  = "/etc/g8e/network-identity.json"
	TestPathShortData           = "/data"
	TestPathShortPKI            = "/pki"
	TestPathShortSecrets        = "/secrets"
	TestPathShortVault          = "/vault"
	TestPathShortVaultKey       = "/vault/key"
	TestPathIdentityFile        = "/path/to/identity.json"
	TestPathIdentityFileShort   = "/path/identity.json"
	TestPathNonexistentTribunal = "/nonexistent/path/tribunal.json"
)

// Tribunal bootstrap config filename for declarative tribunal seeding.
const (
	TribunalBootstrapConfigFilename = "tribunal-bootstrap.json"
)

// Operational limits for filesystem, grep, and execution operations.
const (
	FsListMaxDepth       = 3
	FsListDefaultDepth   = 0
	FsListMaxEntries     = 500
	FsListDefaultEntries = 100
	FsListBatchSize      = 100

	FsGrepDefaultMaxMatches     = 100
	FsGrepMaxMatches            = 500
	FsGrepScannerInitialBufSize = 64 * 1024
	FsGrepScannerMaxBufSize     = 1024 * 1024

	ExecutionMaxStreamSize = 10 * 1024 * 1024 // 10MB per stream
	ExecutionMaxLines      = 50               // Max lines for terminal output preview
	ExecutionPreviewLength = 300              // Max characters for log preview
	FileEditMaxSize        = 50 * 1024 * 1024 // 50MB max file size for operations
)

// File suffixes for temp, backup, and SQLite artifacts.
const (
	TmpFileSuffix           = ".tmp"
	BackupFileSuffixPattern = ".backup-%s-%s"
	SQLiteWALSuffix         = "-wal"
	SQLiteSHMSuffix         = "-shm"
)

// Reporting constants for output directory and filenames.
const (
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
