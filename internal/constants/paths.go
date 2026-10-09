// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	PathEtcHostsHost                                          = "/etc/hosts.host"
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
	PathEtcHostnameHost                                       = "/etc/hostname.host"
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
	PathSysClassDMIIDProductUUID                              = "/sys/class/dmi/id/product_uuid"
	PathSysClassDMIIDSysVendor                                = "/sys/class/dmi/id/sys_vendor"
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
	PkiSubdirPendingEnroll  = "pending-enrollment"

	// File extensions
	FileExtCert = ".crt"
	FileExtKey  = ".key"
	FileExtPEM  = ".pem"
	FileExtJSON = ".json"
	FileExtText = ".txt"

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

	// Pending platform enrollment state filenames (one per component,
	// persisted under pki/pending-enrollment/ with 0600 permissions).
	PendingEnrollmentFileOperator  = "g8eo.json"
	PendingEnrollmentFileDashboard = "g8ed.json"
	PendingEnrollmentFileEnsemble  = "g8ee.json"

	// Non-secret Operator deployment state, persisted under the runtime tree
	// so a deploying CLI can discover enrollment progress without parsing logs.
	DeploymentDirname           = "deployment"
	DeploymentStateFileOperator = "operator.json"
)

// Storage constants for database filenames and paths.
const (
	DbFilename             = "g8e.db"
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
	OperatorDeploymentStatePath = RuntimeDirname + "/" + DeploymentDirname + "/" + DeploymentStateFileOperator
	ReplayStoreDBPath           = RuntimeDirname + "/" + ReplayStoreDBFilename
	ExecutionVaultDBPath        = RuntimeDirname + "/" + ExecutionVaultDBFilename
	SuspendedTransactionDBPath  = RuntimeDirname + "/" + SuspendedTxFilename

	// Runtime-relative database paths (relative to RuntimeFileService root).
	CanonicalDBRelPath            = DataDirname + "/" + DbFilename
	SuspendedTransactionDBRelPath = DataDirname + "/" + SuspendedTxFilename
	ExecutionVaultDBRelPath       = DataDirname + "/" + ExecutionVaultDBFilename
	ReplayStoreDBRelPath          = DataDirname + "/" + ReplayStoreDBFilename

	// Key filenames. MasterKeyDPAPIFilename holds the Windows DPAPI-sealed
	// master key; no platform stores the master key in the clear.
	MasterKeyDPAPIFilename = ".master_key.dpapi"
	PublicKeySuffix        = ".pub"
)

// Secrets filenames for bootstrap and runtime secret material.
const (
	SecretsFileSessionEncryptionKey     = "session_encryption_key"
	SecretsFileBootstrapDigest          = "bootstrap_digest.json"
	SecretsFileActuatorSigningKey       = "actuator_signing_key"
	SecretsFileActuatorKeyID            = "actuator_key_id"
	SecretsFileAuditorSigningKey        = "auditor_signing_key"
	SecretsFileAuditorKeyID             = "auditor_key_id"
	SecretsFileAuditorHMACKey           = "auditor_hmac_key"
	SecretsFileNotarySigningKey         = "notary_signing_key"
	SecretsFileOperatorPrivateKey       = "operator_private_key"
	SecretsFileCLIPrivateKey            = "cli_private_key"
	SecretsFileSessionToken             = "session_token"
	SecretsFileConsensusMemberKeyPrefix = "consensus_member_"
	SecretsFileVaultKey                 = "vault_key"
	SecretsFileVaultKeyStaged           = "vault_key.rekey"
)

// Docker constants for the root unified-stack compose deployment.
const (
	DockerExecutable                 = "docker"
	DockerComposeFile                = "docker-compose.yml"
	DockerNativeEvalComposeFile      = "eval/native-boundary-compose.yml"
	DockerBootstrappedProfile        = "bootstrapped"
	DockerCrossEnrollProfile         = "cross-enrollment"
	DockerEvaluationProfile          = "evaluation"
	DockerG8ellamaProfile            = "g8ellama"
	DockerGatewayContainer           = "g8e-gateway"
	DockerDataOperatorContainer      = "g8e-data-operator"
	DockerEnsembleContainer          = "ensemble"
	DockerDashboardContainer         = "dashboard"
	DockerNativeTargetReaderService  = "g8e-native-target-reader"
	DockerProjectPrefix              = "g8e"
	EvaluationTargetContainerDir     = "/tmp"
	EvaluationTargetFilenamePrefix   = "g8e-eval-"
	EvaluationObserverTargetEnv      = "G8E_EVAL_TARGET"
	EvaluationObserverAbsentExitCode = 3
	// EvaluationWorkspaceDirname is the directory under the Data Operator's
	// reported working directory that holds attempt-scoped scenario fixture
	// workspaces. Names are neutral so the model is not told it is evaluated.
	EvaluationWorkspaceDirname = "workspaces"
	// EvaluationWorkspacePrefix prefixes the per-attempt workspace directory,
	// which is completed by a digest of the run and attempt identity.
	EvaluationWorkspacePrefix = "ws-"
)

// Container paths for Docker exec commands in container environments.
// These are paths inside the g8e Docker containers, not local filesystem paths.
const (
	ContainerBinaryPath               = "/g8e"
	ContainerRootG8E                  = "/root/.g8e"
	ContainerPKIDir                   = ContainerRootG8E + "/" + PkiDirname
	ContainerOperatorCert             = ContainerPKIDir + "/" + PkiFileOperatorCert
	ContainerOperatorKey              = ContainerPKIDir + "/" + PkiFileOperatorKey
	ContainerCABundle                 = ContainerPKIDir + "/" + PkiSubdirTrust + "/" + PkiFileGatewayBundle
	ContainerDataDir                  = ContainerRootG8E + "/" + DataDirname
	ContainerAuditVaultDB             = ContainerDataDir + "/" + DbFilename
	ContainerExecutionVaultDB         = ContainerDataDir + "/" + ExecutionVaultDBFilename
	ContainerLedgerFilesDir           = ContainerDataDir + "/" + LedgerDirname + "/" + FilesDirname
	ContainerFinanceTargetDir         = "/var/g8e/target"
	ContainerFinanceUnauthorizedTrade = ContainerFinanceTargetDir + "/unauthorized_trade_execution.log"
	ContainerHealthcarePAOperations   = "/var/pa_operations.log"

	ContainerEnsembleSeed  = "/etc/g8e/ensemble-seed.hex"
	ContainerVerifyOpsPy   = "/app/verify_ops.py"
	ContainerVerifyPAPy    = "/app/verify_pa.py"
	ContainerInspectRFPy   = "/app/inspect_rf.py"
	ContainerInspectPNTPy  = "/app/inspect_pnt.py"
	ContainerVerifySlewsPy = "/app/verify_slews.py"
	ContainerKSICatalog    = "/docs/reference/" + KSICatalogFilename

	// FedRAMP cloudsvc operations log path inside the cloudsvc container.
	// Used for independent prohibited-side-effect verification after blocked
	// destruction attempts.
	ContainerCloudSvcOpsLog = "/var/cloudsvc/operations.jsonl"
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

// G8eBinariesDir is the optional image-baked directory for a complete
// cross-platform binary manifest. Standard runtime images contain only /g8e;
// custom distribution images can populate this directory for the Gateway's
// /.well-known/g8e/bin/{filename} endpoint. The path remains outside .g8e/
// runtime state.
const (
	G8eBinariesDir                  = "/opt/g8e/bin"
	G8eBinariesManifestFilename     = "g8e-binaries.json"
	G8eBinariesExportRecordFilename = "g8e-binaries-export.json"
	G8eBinaryChecksumSuffix         = ".sha256"
	G8eBinariesArchiveRoot          = "/opt/g8e/bin"
	G8eBinariesStagingPrefix        = ".g8e-binaries-export-"
	G8eBinariesPreviousSuffix       = ".previous"

	// OCI labels copied from the build provenance embedded in g8e-binaries.json.
	G8eImageVersionLabel   = "org.opencontainers.image.version"
	G8eBuildIDLabel        = "io.g8e.build.id"
	G8eBuildTimeLabel      = "io.g8e.build.time"
	G8eSourceRevisionLabel = "io.g8e.source.revision"
	G8eSourceTreeHashLabel = "io.g8e.source.tree.hash"
)

// Deploy script filenames served by the gateway.
const (
	DeployScriptFilenameLinux   = "g8e-deploy.sh"
	DeployScriptFilenameWindows = "g8e-deploy.ps1"
)

// Gateway-served API reference routes. The spec route is read by the Console
// API view, so it is classified for web-session or mTLS callers in
// gateway_auth.go.
const (
	SwaggerUIPath  = "/swagger/"
	SwaggerDocPath = "/swagger/doc.json"
)

// Component filenames for gateway, operator, and shared services.
const (
	SwaggerFilename          = "swagger.json"
	ComplianceReportFilename = "compliance-report.json"
	G8eLogFilename           = "g8e.log"

	// Gateway-specific filenames
	GatewayIDFilename       = "gateway-id"
	ActuatorPubJSONFilename = "Actuator_pub.json"
	ActuatorPubPEMFilename  = "Actuator_pub.pem"
	NetworkIdentityFilename = "network-identity.json"

	// Operator-specific filenames
	OperatorPIDFilename     = "operator.pid"
	OperatorPostureFilename = "operator.posture"
	OperatorBinaryFilename  = "g8e-operator"

	// Launch profile persisted after every successful managed background
	// `gw start`. Read by `gw restart` to reconstruct the complete prior
	// launch configuration (CORS, passkey, ports, posture, downstream
	// routes, rate limits, doctrine, consensus, vault, cert mode, public
	// base URL). Ephemeral network identity is re-detected on restart, not
	// persisted. Stored under .g8e/pids/ alongside the PID file.
	OperatorLaunchProfileFilename = "operator-launch-profile.json"
)

// RuntimeArchiveTimestampLayout is the MMDDHHMM suffix appended to
// RuntimeDirname when a destructive clean renames the runtime tree aside
// (.g8e-09301401) instead of deleting it.
const RuntimeArchiveTimestampLayout = "01021504"

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

	// Inference runtime directory names under data/ and the runtime root.
	InferenceDirname                        = "inference"
	InferenceAttemptsDirname                = "attempts"
	InferenceProviderObserverDirname        = "provider-observer"
	InferenceProviderObserverWindowsDirname = "windows"
	InferenceModelProvenanceDirname         = "model-provenance"
	InferenceModelProvenanceWindowsDirname  = "windows"

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
	AgentConfigDirDevin      = ".config/devin"
	AgentConfigDirGemini     = ".gemini"
	AgentConfigDirGoose      = ".config/goose"
	AgentConfigFileMCP       = "mcp.json"
	AgentConfigFileMCPDevin  = "config.json"
	AgentConfigFileSettings  = "settings.json"
	AgentConfigFileGooseYAML = "config.yaml"
)

// API path constants for enrollment and well-known endpoints.
const (
	APIPathPKIDevicesEnroll = "/api/v1/pki/devices/enroll"
	WellKnownPKICABundle    = "/.well-known/g8e/pki/ca-bundle"
)

// CLI default paths for config and help text (derived from primitives).
const (
	DefaultVaultDirDesc     = RuntimeDirname + "/" + VaultDirname
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
	PermDirPrivate     = 0700 // rwx------
	PermDirStandard    = 0755 // rwxr-xr-x
	PermFilePrivate    = 0600 // rw-------
	PermFilePublic     = 0644 // rw-r--r--
	PermFileReadOnly   = 0400 // r--------
	PermFileExecutable = 0755 // rwxr-xr-x
)

// Test-specific constants for isolated test environments.
const (
	TestEmptyMachineIDFilename     = "empty-machine-id"
	TestFileTxtFilename            = "test.txt"
	TestNonexistentTxtFilename     = "nonexistent.txt"
	TestResultsDirname             = "test-results"
	TestVaultDirname               = "test-vault"
	TestSecretManagerDBFilename    = "secret_manager_test.db"
	TestCommitmentLedgerDBFilename = "commitment_ledger_test.db"
	TestGatewayDatabaseFilename    = "gateway_test.db"
	TestCertFilename               = "test-cert.pem"
	TestKeyFilename                = "test-key.pem"

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

	// Test path constants for gateway config and consensus bootstrap tests
	TestPathVarLibDataDir                 = "/var/lib/g8e/data"
	TestPathVarLibPKIDir                  = "/var/lib/g8e/pki"
	TestPathVarLibSecretsDir              = "/var/lib/g8e/secrets"
	TestPathVarLibVaultDir                = "/var/lib/g8e/vault"
	TestPathEtcNetworkIdentity            = "/etc/g8e/network-identity.json"
	TestPathShortData                     = "/data"
	TestPathShortPKI                      = "/pki"
	TestPathShortSecrets                  = "/secrets"
	TestPathShortVault                    = "/vault"
	TestPathIdentityFile                  = "/path/to/identity.json"
	TestPathIdentityFileShort             = "/path/identity.json"
	TestPathNonexistentConsensus          = "/nonexistent/path/consensus.json"
	TestPathNonexistentCatalog            = "/nonexistent/path/ksi-catalog.json"
	TestPathNonexistentOverlayDir         = "/nonexistent/path/overlays"
	TestCustomComplianceOutDir            = "custom-compliance-out"
	TestInvalidJSONFilename               = "invalid.json"
	TestPublicFeedRecordsFilename         = "public-feed-records.jsonl"
	TestQualificationCandidateFilename    = "qualification-candidate.json"
	TestPublicLoopEvidenceFilename        = "public-loop-evidence.json"
	TestOverlaysDirname                   = "test-overlays"
	TestPathRepoRootFromCompliancePackage = "../../.."
	TestDataDirname                       = "testdata"
	TestEvaluationTargetFilename          = "evaluation-target.txt"
	TestReadOnlyDatabaseFilename          = "read-only.db"

	// Source-tree protocol path constants for contract tests in
	// internal/constants and internal/models. These resolve canonical
	// protocol JSON files relative to the package directory so tests do not
	// hand-roll "../../protocol/..." literals.
	ProtocolSourceTreeRootFromInternalPkg            = "../../"
	ProtocolEventsJSONFilename                       = "events.json"
	ProtocolEventDashboardClassificationJSONFilename = "event_dashboard_classification.json"
	ProtocolObserveAPIJSONFilename                   = "observe_api.json"
	ProtocolObserveEventPayloadsJSONFilename         = "observe_event_payloads.json"
	ProtocolPublicFeedJSONFilename                   = "public_feed.json"

	// Evaluation contract vectors under protocol/vectors/eval, resolved from
	// internal/services/evaluation. The ensemble pins the same file, so the
	// g8ee trace and the Go grader cannot drift apart.
	ProtocolSourceTreeRootFromEvaluationPkg = "../../../"
	ProtocolVectorsDirname                  = "vectors"
	ProtocolEvalVectorsDirname              = "eval"
	ProtocolPlayerStepsVectorFilename       = "player_steps.json"
)

// Consensus bootstrap config filename for declarative consensus seeding.
const (
	ConsensusBootstrapConfigFilename = "consensus-bootstrap.json"
)

// Lattice adapter filename for persisted entity ID.
const (
	LatticeEntityIDFilename = "lattice_entity_id"
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

// Compliance constants for KSI catalog, OSCAL output, and KSI history snapshots.
const (
	ComplianceDirname              = "compliance"
	KSICatalogFilename             = "ksi-catalog.json"
	COSAiSOverlaysFilename         = "cosais-overlays.json"
	OSCALAssessmentResultsFilename = "assessment-results.json"
	OSCALComponentDefFilename      = "component-definition.json"
	KSIHistoryDirname              = "ksi-history"
	KSIHistoryFilenamePrefix       = "ksi-result-"
	DefaultKSICatalogPath          = DocsDirname + "/reference/" + KSICatalogFilename
	DefaultOverlayDirPath          = DocsDirname + "/reference"
	KSIHistoryRetentionDays        = 90

	ComplianceBundlesDirname                     = "reports"
	ComplianceBundleManifestFilename             = "manifest.json"
	ComplianceBundleScopeFilename                = "scope.json"
	ComplianceBundleFrameworkCatalogsDirname     = "framework-catalogs"
	ComplianceBundleFrameworkCatalogFilename     = "framework-catalog.json"
	ComplianceBundleAssertionsDirname            = "assertions"
	ComplianceBundleAssertionCatalogFilename     = "assertion-catalog.json"
	ComplianceBundleCrosswalksDirname            = "crosswalks"
	ComplianceBundleCrosswalkFilename            = "fedramp-nist-crosswalk.json"
	ComplianceBundleAssessmentsDirname           = "assessments"
	ComplianceBundleAssertionAssessmentsFilename = "assertion-assessments.jsonl"
	ComplianceBundleControlAssessmentsFilename   = "control-assessments.jsonl"
	ComplianceBundleEvidenceDirname              = "evidence"
	ComplianceBundleEvidenceIndexFilename        = "evidence-index.jsonl"
	ComplianceBundleEvalEvidenceDirname          = "eval"
	ComplianceBundleEvalManifestsFilename        = "manifests.jsonl"
	ComplianceBundleEvalTasksFilename            = "tasks.jsonl"
	ComplianceBundleEvalAttemptsFilename         = "attempts.jsonl"
	ComplianceBundleEvalReceiptsFilename         = "receipts.jsonl"
	ComplianceBundleEvalStagesFilename           = "stages.jsonl"
	ComplianceBundleEvalMetricsFilename          = "metrics.jsonl"
	ComplianceBundleEvalStateFilename            = "state-observations.jsonl"
	ComplianceBundlePlatformEvidenceDirname      = "platform"
	ComplianceBundleKSIResultsFilename           = "ksi-results.json"
	ComplianceBundleKSIHistoryFilename           = "ksi-history.jsonl"
	ComplianceBundleCommitmentsFilename          = "commitments.jsonl"
	ComplianceBundleAttestationsFilename         = "attestations.jsonl"
	ComplianceBundleRestrictedDirname            = "restricted"
	ComplianceBundleOSCALDirname                 = "oscal"
	ComplianceBundleAnalysisFilename             = "analysis.json"
	ComplianceBundleGapsFilename                 = "gaps.json"
	ComplianceBundleMarkdownFilename             = "report.md"
	ComplianceBundleHTMLFilename                 = "report.html"
	ComplianceBundleChecksumsFilename            = "checksums.json"
	ComplianceBundlePublicKeysFilename           = "public-keys.json"
	ComplianceBundleSignaturesFilename           = "signatures.json"
	ComplianceOperationalExportDirname           = "operational"
	ComplianceOperationalInventoryFilename       = "inventory.json"
	ComplianceOperationalReceiptsDirname         = "receipts"
	ComplianceOperationalPersistenceDirname      = "persistence"
	ComplianceOperationalCommitmentsDirname      = "commitments"
	ComplianceOperationalAuditChainDirname       = "audit-chain"
)

// Release evidence output filename suffixes. The per-release compliance
// evidence artifacts are written to docs/release_notes/vX.Y.x/ as
// vX.Y.Z-compliance-evidence.md and vX.Y.Z-compliance-evidence.csv. These are
// source-tree documentation paths, not .g8e/ runtime paths.
const (
	ReleaseEvidenceMarkdownSuffix = "-compliance-evidence.md"
	ReleaseEvidenceCSVSuffix      = "-compliance-evidence.csv"
)

// Evidence verifier identities, reference prefixes, collection limits, and
// attestation metadata. These describe runtime evidence handling, distinct
// from the ComplianceBundle* constants which describe the generated report
// bundle layout.
const (
	EvalRunVerificationCheck                      = "eval_evidence_graph"
	AssertionGraderID                             = "assertion_assessment"
	AssertionGraderVersion                        = "2.2.0"
	FrameworkGraderID                             = "framework_assessment"
	FrameworkGraderVersion                        = "1.0.0"
	ReceiptEvidenceVerifierID                     = "g8e-receipt-evidence-importer"
	ReceiptEvidenceVerifierVersion                = "1.0.0"
	ActionReceiptReferencePrefix                  = "action-receipt"
	ReceiptPersistenceReferencePrefix             = "receipt-persistence"
	DeterministicStagesReferencePrefix            = "deterministic-stages"
	AuditRecordReferencePrefix                    = "audit-record"
	AuditRecordsDirname                           = "audit-records"
	CommitmentReferencePrefix                     = "commitment"
	CommitmentsDirname                            = "commitments"
	CommitmentEvidenceVerifierID                  = "g8e-commitment-evidence-importer"
	CommitmentEvidenceVerifierVersion             = "1.0.0"
	AuditChainEvidenceVerifierID                  = "g8e-audit-chain-evidence-importer"
	AuditChainEvidenceVerifierVersion             = "1.0.0"
	LedgerCommitReferencePrefix                   = "ledger-commit"
	LedgerCommitCollectionReferencePrefix         = "ledger-commit-collection"
	LedgerStateReferencePrefix                    = "ledger-state"
	LedgerEvidenceDirname                         = "ledger"
	LedgerCommitsFilename                         = "commits.jsonl"
	LedgerStateFilename                           = "state.json"
	LedgerEvidenceSchemaVersion                   = "1.0.0"
	LedgerEvidenceMaxCommits                      = 4096
	KSIHistoryReferencePrefix                     = "ksi-history"
	KSIResultReferencePrefix                      = "ksi-result"
	KSIHistoryMaxSnapshots                        = 4096
	BuildAttestationReferencePrefix               = "build-attestation"
	ConfigAttestationReferencePrefix              = "config-attestation"
	BuildConfigAttestationsFilename               = "build-config-attestations.jsonl"
	BuildAttestationSchemaVersion                 = "1.0.0"
	BuildAttestationMaxRecords                    = 4096
	AttestationCollectionReferencePrefix          = "attestation-collection"
	CustomerAttestationReferencePrefix            = "customer-attestation"
	AssessorAttestationReferencePrefix            = "assessor-attestation"
	AttestationSchemaVersion                      = "1.0.0"
	AttestationMaxRecords                         = 4096
	AttestationEvidenceVerifierID                 = "g8e-attestation-evidence-importer"
	AttestationEvidenceVerifierVersion            = "1.0.0"
	EvidenceMaxArtifactsPerDirectory              = 4096
	EvaluationDirname                             = "eval"
	EvaluationInventoriesDirname                  = "inventories"
	EvaluationCampaignsDirname                    = "campaigns"
	EvaluationSuitesDirname                       = "suites"
	EvaluationRunsDirname                         = "runs"
	EvaluationArchiveDirname                      = "archive"
	EvaluationArchiveManifestSuffix               = ".archive.json"
	EvaluationRunLeaseFilename                    = "lease.json"
	EvaluationAssignmentsDirname                  = "assignments"
	EvaluationActiveRunFilename                   = "active-run.json"
	EvaluationActiveRunPath                       = DataDirname + "/" + EvaluationDirname + "/" + EvaluationActiveRunFilename
	EvaluationDataPath                            = DataDirname + "/" + EvaluationDirname
	EvaluationBackupManifestFilename              = "eval-backup.json"
	EvaluationBackupDirPrefix                     = "eval-backup-"
	EvaluationBackupTimestampLayout               = "20060102T150405Z"
	EvaluationBackupDefaultDir                    = EvaluationDirname + "/backups"
	EvaluationModelInventoryPath                  = EvaluationDirname + "/model-inventory.json"
	EvaluationInitCampaignQueuePath               = EvaluationDirname + "/init-campaign-queue.json"
	EvaluationQueueLogsDirname                    = EvaluationDirname + "/logs"
	EvaluationCampaignExportDirname               = "export"
	EvaluationCampaignExportSQLiteFilename        = "campaign_export.sqlite"
	EvaluationExportSchemaFilename                = "export_schema.json"
	EvaluationRunSummaryFilename                  = "run_summary.json"
	EvaluationAssignmentsJSONLFilename            = "assignments.jsonl"
	EvaluationModelSummariesJSONLFilename         = "model_summaries.jsonl"
	EvaluationAssignmentsCSVFilename              = "assignments.csv"
	EvaluationModelSummariesCSVFilename           = "model_summaries.csv"
	EvaluationReportFilename                      = "report.json"
	EvaluationVerificationFilename                = "verification.json"
	EvaluationCampaignSpecFilename                = "campaign-spec.json"
	EvaluationCampaignReleaseTagFilename          = "release-tag.json"
	EvaluationHeterogeneousStackSetFilename       = "heterogeneous-stack-set.json"
	EvaluationScenarioCatalogFilename             = "scenario-catalog.json"
	EvaluationScenarioArtifactsDirname            = "scenario-artifacts"
	EvaluationRunStateFilename                    = "run.json"
	EvaluationPublicationStateFilename            = "public-projection-state.json"
	EvaluationEvidenceDirname                     = "evidence"
	EvalRunVerifierID                             = "g8e-native-evaluation-verifier"
	EvalRunVerifierVersion                        = "1.0.0"
	EvaluationSourceKindNative                    = "native-evaluation"
	EvaluationSourceKindCampaign                  = "campaign-evaluation"
	EvaluationSourceVersion                       = "1.0.0"
	CampaignVerifierID                            = "g8e-campaign-verifier"
	CampaignVerifierVersion                       = "2.0.0"
	CampaignVerificationFilename                  = "campaign-verification.json"
	CampaignExportEvaluationSummaryFilename       = "evaluation_summary.json"
	CampaignSourceInventoryFilename               = "campaign-source-inventory.json"
	CampaignSourceInventoryVersion                = "1.0.0"
	EvaluationSelectionDiagnosticsFilename        = "evaluation-selection-diagnostics.json"
	EvalScopePrefix                               = "eval:"
	EvalRestrictedEvidenceScope                   = "restricted_evaluation_evidence"
	EvalEvidenceEncryptionAES256GCM               = "aes-256-gcm"
	EvalEncryptedEvidenceVersion                  = 1
	EvalEncryptedEvidenceNonceBytes               = 12
	MediaTypeJSON                                 = "application/json"
	MediaTypeOctetStream                          = "application/octet-stream"
	MediaTypeOSCALJSON                            = "application/oscal+json"
	MediaTypeMarkdown                             = "text/markdown; charset=utf-8"
	MediaTypeHTML                                 = "text/html; charset=utf-8"
	MediaTypeCSV                                  = "text/csv; charset=utf-8"
	MediaTypeText                                 = "text/plain; charset=utf-8"
	ObserveEventPayloadSchemaVersion              = "1.0.0"
	ObserveMeasurementSchemaVersion               = "1.0.0"
	ObserveAPIReadModelSchemaVersion              = "1.0.0"
	ObserveDownloadsDirname                       = "observe-downloads"
	ObserveDownloadArtifactMaxBytes               = 64 << 20
	EvidenceGraphVerifierID                       = "g8e-evidence-graph-verifier"
	EvidenceGraphVerifierVersion                  = "1.0.0"
	EvidenceGraphMaxBytes                         = 64 << 20
	AnalysisBuilderID                             = "g8e-compliance-analysis-builder"
	AnalysisBuilderVersion                        = "1.2.0"
	AnalysisSchemaVersion                         = "1.2.0"
	FrameworkProfileVersion                       = "1.0.0"
	ComplianceReportSignatureAlgorithm            = "ed25519"
	ComplianceReportSigningPurpose                = "compliance-report-bundle"
	ComplianceBundleAssemblerID                   = "g8e-compliance-bundle-assembler"
	ComplianceBundleAssemblerVersion              = "1.0.0"
	ComplianceBundleVerifierID                    = "compliance_bundle"
	ComplianceBundleVerifierVersion               = "2.0.0"
	ComplianceBundleCheckCatalog                  = "bundle_catalog"
	ComplianceBundleCheckManifestDigest           = "manifest_digest"
	ComplianceBundleCheckBindings                 = "bundle_bindings"
	ComplianceBundleCheckDirectoryInventory       = "directory_inventory"
	ComplianceBundleCheckArtifactBodies           = "artifact_bodies"
	ComplianceBundleCheckChecksumRoot             = "checksum_root"
	ComplianceBundleCheckSignatures               = "bundle_signatures"
	ComplianceBundleCheckTypedArtifacts           = "typed_artifacts"
	ComplianceBundleCheckSourceVerification       = "source_verification"
	ComplianceBundleCheckDecisionReplay           = "decision_replay"
	ComplianceBundleCheckRenderedFormats          = "rendered_formats"
	ComplianceBundleSchemaVersion                 = "1.0.0"
	ComplianceBundleProfilePublic                 = "public"
	ComplianceBundleProfileRestricted             = "restricted"
	ComplianceBundleMaxArtifacts                  = 8192
	ComplianceBundleMaxArtifactBytes              = 64 << 20
	ComplianceOperationalExportDefaultMaxRows     = 4096
	ComplianceEvidenceTrustMaxKeys                = 4096
	ComplianceBundleMaxDirectoryDepth             = 32
	ComplianceBundleMaxEnumeratedEntries          = ComplianceBundleMaxArtifacts * 2
	ComplianceBundleAnalysisPath                  = "analysis.json"
	ComplianceBundleProfilesDirname               = "profiles"
	ComplianceBundleOSCALPath                     = "oscal/assessment-results.json"
	ComplianceBundleMarkdownPath                  = "report.md"
	ComplianceBundleHTMLPath                      = "report.html"
	ComplianceBundleCSVPath                       = "report.csv"
	ComplianceBundleCLIPath                       = "report.txt"
	ComplianceBundleJSONPath                      = "analysis.json"
	ComplianceBundleChecksumsPath                 = "checksums.json"
	ComplianceBundleManifestPath                  = "manifest.json"
	ComplianceBundleSourcesDirname                = "sources"
	ComplianceBundleSourceEvalsDirname            = "evals"
	ComplianceBundleSourceRuntimeDirname          = "runtime"
	ComplianceBundleSourceProvenanceDirname       = "provenance"
	ComplianceBundleSourceArtifactsDirname        = "artifacts"
	ComplianceBundleSourceVerificationFilename    = "verification.json"
	ComplianceBundleFrameworkProfileTestPath      = "profiles/profile:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.json"
	ComplianceBundleRestrictedEvidenceTestPath    = "restricted/evidence.json"
	ComplianceBundleMisnestedRestrictedTestPath   = "sources/restricted/evidence.json"
	ComplianceBundleUnexpectedTestPath            = "unexpected.json"
	ComplianceReportSigningMetadataTestFilename   = "signing-metadata.json"
	ComplianceReportSigningPrivateKeyTestFilename = "signing-private-key.hex"
	ComplianceReportTrustPolicyTestFilename       = "assessed-trust.json"
	ComplianceEvidenceTrustPolicyTestFilename     = "assessed-evidence-trust.json"
	AuditRecordTestFilename                       = "audit-record.json"
	KSIMethodDefinitionVersion                    = "1.0.0"
	KSIMethodVerifierID                           = "g8e-ksi-method-verifier"
	KSIMethodVerifierVersion                      = "1.0.0"
	KSIEvaluatorID                                = "g8e-ksi-evaluator"
	KSIEvaluatorVersion                           = "1.0.0"
	KSIHistorySchemaVersion                       = "1.3.0"
	KSIUnavailableIntervalsFilename               = "ksi-unavailable-intervals.jsonl"
	KSIUnavailableIntervalReferencePrefix         = "ksi-unavailable"

	// OSCAL validator identity and embedded schema metadata. The official
	// unmodified NIST OSCAL 1.1.2 assessment-results JSON Schema is embedded
	// into the protocol library so validation is offline and available in
	// air-gapped builds. The pinned SHA-256 rejects any schema bytes that do
	// not match the authenticated NIST release artifact.
	OSCALValidatorID               = "g8e-oscal-validator"
	OSCALValidatorVersion          = "1.0.0"
	OSCALSchemaVersion             = "1.1.2"
	OSCALSchemaID                  = "http://csrc.nist.gov/ns/oscal/1.1.2/oscal-ar-schema.json"
	OSCALSchemaJSONDraft           = "draft-07"
	OSCALSchemaByteLength          = 133015
	OSCALSchemaSHA256              = "d033da70154cf6625ae46a746199e88e58f2928b1387dfac051d381b92f41b0d"
	OSCALSchemaFilename            = "oscal_assessment-results_schema.json"
	OSCALSchemaProvenanceFilename  = "provenance.json"
	OSCALSchemaRelativePath        = "oscal/v1.1.2/" + "oscal_assessment-results_schema.json"
	OSCALProvenanceRelativePath    = "oscal/v1.1.2/" + "provenance.json"
	OSCALValidatorMaxDocumentBytes = 16 << 20
	OSCALValidatorMaxDepth         = 256
	OSCALValidatorMaxProperties    = 4096
	OSCALValidatorMaxItems         = 4096
	OSCALValidatorMaxRefDepth      = 64
	OSCALValidatorMaxFailures      = 256
)

// Cloudflare Tunnel configuration path constants.
const (
	CloudflaredDirname                 = ".cloudflared"
	CloudflaredConfigFilename          = "config.yml"
	CloudflaredOriginCertFilename      = "cert.pem"
	CloudflaredCredentialFileExtension = ".json"
)

// Supervisor runtime path constants (O1-supervisor: continuous campaign
// supervisor with hash-linked cycle ledger and persisted spec).
const (
	SupervisorDirname        = "supervisor"
	SupervisorSpecFilename   = "spec.json"
	SupervisorLedgerFilename = "cycle-ledger.jsonl"
	SupervisorLockFilename   = "supervisor.lock"
	SupervisorStateFilename  = "state.json"
)

// Scale test scratch root constants. `g8e test scale` without --root writes
// each run under <working dir>/.local.dev/scale/<UTC timestamp>, never under
// the OS temporary directory.
const (
	ScaleRunsDirPath        = ".local.dev/scale"
	ScaleRunTimestampFormat = "2006-01-02T15-04-05Z"
)

// Public loop test scratch root constants. `g8e test public-loop` writes
// each run under <working dir>/.local.dev/public-loop/<UTC timestamp>, never under
// the OS temporary directory.
const (
	PublicLoopRunsDirPath        = ".local.dev/public-loop"
	PublicLoopRunTimestampFormat = "2006-01-02T15-04-05Z"
)

// Public feed runtime path constants (O3-public-feed: outbound publisher,
// signed append-only batches, ordered outbox, and content-addressed proof
// packages).
const (
	PublicFeedDirname                        = "public-feed"
	PublicFeedOutboxFilename                 = "outbox.jsonl"
	PublicFeedOutboxPath                     = "public-feed/outbox.jsonl"
	PublicFeedSnapshotFilename               = "snapshot.json"
	PublicFeedSnapshotPath                   = "public-feed/snapshot.json"
	PublicFeedExportConfigFilename           = "export-config.json"
	PublicFeedExportConfigPath               = "public-feed/export-config.json"
	PublicFeedSigningKeyFilename             = "signing-key.ed25519"
	PublicFeedSigningKeyPath                 = "public-feed/signing-key.ed25519"
	PublicFeedIngestTokenFilename            = "ingest-token"
	PublicFeedIngestTokenPath                = "public-feed/ingest-token"
	PublicFeedKeyRotationFilename            = "key-rotation.json"
	PublicFeedKeyRotationPath                = "public-feed/key-rotation.json"
	PublicFeedArchiveDirname                 = "public-feed-archive"
	PublicFeedArchiveGenerationsDirname      = "public-feed-archive/generations"
	PublicFeedLegacyArchiveExportConfigPath  = "public-feed-archive/export-config.json"
	PublicFeedLegacyArchiveSigningKeyPath    = "public-feed-archive/signing-key.ed25519"
	PublicFeedLegacyArchiveIngestTokenPath   = "public-feed-archive/ingest-token"
	PublicFeedLegacyArchiveOutboxPath        = "public-feed-archive/outbox.jsonl"
	PublicFeedLegacyArchiveSnapshotPath      = "public-feed-archive/snapshot.json"
	PublicFeedLegacyArchiveKeyRotationPath   = "public-feed-archive/key-rotation.json"
	PublicFeedLegacyArchiveProofsPath        = "public-feed-archive/public-proofs"
	PublicFeedLegacyArchiveProofCatalogPath  = "public-feed-archive/proof-catalog.json"
	PublicFeedLegacyArchiveProofManifestPath = "public-feed-archive/proof-manifest.json"
	PublicMirrorDirname                      = "public-mirror"
	PublicMirrorStateFilename                = "state.json"
	PublicMirrorStatePath                    = "public-mirror/state.json"
	PublicProofsDirname                      = "public-proofs"
	PublicProofCatalogFilename               = "proof-catalog.json"
	PublicProofManifestFilename              = "proof-manifest.json"
	PublicProofMirrorSyncFilename            = "proof-mirror-sync.json"
)

// Public feed schema and protocol version constants.
const (
	PublicFeedProtocolVersion          = "1.0.0"
	PublicFeedSchemaVersion            = "1.0.0"
	PublicProofManifestSchemaVersion   = "1.0.0"
	PublicProofCatalogSchemaVersion    = "1.0.0"
	PublicProofMirrorSyncSchemaVersion = "1.0.0"
	PublicFeedBatchMaxRecords          = 100
	PublicFeedBatchMaxBytes            = 4 << 20
	PublicFeedRetryMaxAttempts         = 5
	PublicFeedRetryInitialBackoff      = 1
	PublicFeedRetryMaxBackoff          = 60
	PublicFeedAckWindowSeconds         = 300
	PublicFeedMaxArtifactBytes         = 64 << 20
	PublicFeedProofIngestMaxBytes      = 512 << 20
	PublicFeedKeyRegistrationMaxBytes  = 16 << 10
	// Init-campaign restore publishes two proof artifacts per assignment across
	// many verified runs; keep headroom above ~3k assignment-audit artifacts.
	PublicFeedProofMaxArtifacts       = 8192
	PublicFeedFreshnessDelayedSeconds = 60
	PublicFeedFreshnessStaleSeconds   = 300
	PublicFeedFreshnessOfflineSeconds = 900
	PublicFeedAnonymousRatePerWindow  = 600
	PublicFeedAnonymousRateWindowSecs = 60
	PublicFeedAnonymousRateMaxClients = 10000
	PublicFeedSSEMaxSubscribers       = 1000
	// Cap SSE backlog replay so reconnects do not resend the full retained feed.
	PublicFeedSSEReplayMaxRecords = 100
	// Retain enough batches for a full homogeneous smoke matrix (~2.6k cells
	// plus lifecycle, result, and aggregate projections) without pruning the
	// prefix the evaluation explorer replays on cold load.
	PublicFeedMirrorRetainedBatches = 25000
	PublicFeedIngestTokenBytes      = 32
	PublicFeedZeroHashHex           = "0000000000000000000000000000000000000000000000000000000000000000"
)
