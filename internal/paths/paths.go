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
	DbPath:                  constants.RuntimeDirname + "/" + constants.DataDirname + "/" + constants.DbFilename,
	PkiDir:                  constants.RuntimeDirname + "/" + constants.PkiDirname,
	SecretsDir:              constants.RuntimeDirname + "/" + constants.SecretsDirname,
	CaCertPath:              constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileGatewayBundle,
	AppCertDir:              constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirApps,
	DocsDir:                 constants.RuntimeDirname + "/" + constants.DocsDirname,
	ProtocolDir:             constants.RuntimeDirname + "/" + constants.ProtocolDirname,
	ProtocolConstantsDir:    constants.RuntimeDirname + "/" + constants.ProtocolDirname + "/" + constants.ProtocolConstantsDirname,
	ProtocolModelsDir:       constants.RuntimeDirname + "/" + constants.ProtocolDirname + "/" + constants.ProtocolModelsDirname,
	SshConfigPath:           constants.RuntimeDirname + "/" + constants.SshConfigFilename,
	RuntimeDir:              constants.RuntimeDirname,
	DataDir:                 constants.RuntimeDirname + "/" + constants.DataDirname,
	VaultDir:                constants.RuntimeDirname + "/" + constants.VaultDirname,
	TestVaultDir:            constants.RuntimeDirname + "/" + constants.TestVaultDirname,
	LocalStateDBPath:        constants.RuntimeDirname + "/" + constants.LocalStateDBFilename,
	AuditVaultDBPath:        constants.RuntimeDirname + "/" + constants.AuditVaultDBFilename,
	RootCAPath:              constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirRoot + "/" + constants.PkiFileRootCA,
	HubCAPath:               constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirAuthorities + "/" + constants.PkiFileHubCA,
	OperatorCAPath:          constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirAuthorities + "/" + constants.PkiFileOperatorCA,
	GatewayPeerCAPath:       constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirAuthorities + "/" + constants.PkiFileGatewayPeerCA,
	GatewayChainPath:        constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirHub + "/" + constants.PkiFileGatewayChain,
	TrustDomainJSONPath:     constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileTrustDomainJSON,
	ServiceCertPath:         constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirHub + "/" + constants.PkiFileGatewayCert,
	PkiRootDir:              constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirRoot,
	PkiAuthoritiesDir:       constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirAuthorities,
	PkiIssuedHubDir:         constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirHub,
	PkiIssuedGatewayPeerDir: constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirGatewayPeer,
	PkiTrustDir:             constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirTrust,
	PkiRevocationDir:        constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirRevocation,
	ActuatorPubJSONPath:     constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.ActuatorPubJSONFilename,
	ActuatorPubPEMPath:      constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.ActuatorPubPEMFilename,

	OperatorKeyPath:        constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiFileOperatorKey,
	OperatorCertPath:       constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiFileOperatorCert,
	OperatorChainPath:      constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiFileOperatorChain,
	WardenPubPath:          constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiFileWardenPub,
	RootCAKeyPath:          constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirRoot + "/" + constants.PkiFileRootCAKey,
	TrustedSignersDir:      constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirTrustedSigners,
	ClientPkiDir:           constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirClient,
	ClientOperatorKeyPath:  constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirClient + "/" + constants.PkiFileOperatorKey,
	ClientOperatorCertPath: constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirClient + "/" + constants.PkiFileOperatorCert,

	SessionEncKeyPath:   constants.RuntimeDirname + "/" + constants.SecretsDirname + "/" + constants.SecretsFileSessionEncryptionKey,
	BootstrapDigestPath: constants.RuntimeDirname + "/" + constants.SecretsDirname + "/" + constants.SecretsFileBootstrapDigest,

	LogDir:          constants.RuntimeDirname + "/" + constants.LogDirname,
	OperatorLogFile: constants.RuntimeDirname + "/" + constants.LogDirname + "/" + constants.OperatorLogFilename,

	ExecutionVaultDBPath: constants.RuntimeDirname + "/" + constants.DataDirname + "/" + constants.ExecutionVaultDBFilename,
	ReplayStoreDBPath:    constants.RuntimeDirname + "/" + constants.DataDirname + "/" + constants.ReplayStoreDBFilename,
	LedgerDir:            constants.RuntimeDirname + "/" + constants.DataDirname + "/" + constants.LedgerDirname,

	DemosDir:                         constants.DemosDirname,
	DemosHealthcareDir:               constants.DemosDirname + "/" + constants.DemosOrgHealthcare,
	DemosFinanceDir:                  constants.DemosDirname + "/" + constants.DemosOrgFinance,
	DemosGovDir:                      constants.DemosDirname + "/" + constants.DemosOrgGov,
	DemosSecureDataDir:               constants.DemosDirname + "/" + constants.DemosOrgSecureData,
	DemosHealthcareTargetDataDir:     constants.DemosDirname + "/" + constants.DemosOrgHealthcare + "/" + constants.DemosTargetDataDir,
	DemosHealthcareDoctrineDir:       constants.DemosDirname + "/" + constants.DemosOrgHealthcare + "/" + constants.DemosDoctrineDir,
	DemosHealthcarePARequestsPath:    constants.DemosDirname + "/" + constants.DemosOrgHealthcare + "/" + constants.DemosTargetDataDir + "/" + constants.DemosPARequestsFile,
	DemosHealthcareComposePath:       constants.DemosDirname + "/" + constants.DemosOrgHealthcare + "/" + constants.DemosComposeFile,
	DemosHealthcareDoctrineHIPAAPath: constants.DemosDirname + "/" + constants.DemosOrgHealthcare + "/" + constants.DemosDoctrineDir + "/" + constants.DemosHIPAADoctrineFile,
	DemosSecureDataDoctrineDir:       constants.DemosDirname + "/" + constants.DemosOrgSecureData + "/" + constants.DemosDoctrineDir,
	DemosSecureDataDoctrinePath:      constants.DemosDirname + "/" + constants.DemosOrgSecureData + "/" + constants.DemosDoctrineDir + "/" + constants.DemosSecureDataDoctrineFile,
}

// Mutable path vars that are derived from the base directory at init time.
// These complement Infra for paths accessed as bare variables.
var (
	GatewayIDPath       = constants.RuntimeDirname + "/" + constants.DataDirname + "/" + constants.GatewayIDFilename
	ActuatorPubJSONPath = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.ActuatorPubJSONFilename
	ActuatorPubPEMPath  = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.ActuatorPubPEMFilename
	NetworkIdentityPath = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.NetworkIdentityFilename
	PeerCertPath        = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PeerSubdir + "/" + constants.PeerCertFilename
	PeerKeyPath         = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PeerSubdir + "/" + constants.PeerKeyFilename
	PeerChainPath       = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PeerSubdir + "/" + constants.PeerChainFilename
	PkiGatewayKeyPath   = constants.RuntimeDirname + "/" + constants.PkiDirname + "/" + constants.PkiSubdirIssued + "/" + constants.PkiSubdirHub + "/" + constants.PkiFileGatewayKey
	SwaggerFilePath     = constants.DocsDirname + "/" + constants.SwaggerFilename
	OperatorLogPath     = constants.OperatorLogFilename
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

	Infra.RuntimeDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname)
	Infra.DataDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.DataDirname)
	Infra.PkiDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.PkiDirname)
	Infra.SecretsDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.SecretsDirname)
	Infra.ProtocolDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.ProtocolDirname)
	Infra.VaultDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.VaultDirname)
	Infra.VaultKeyPath = pathutil.SafeJoin(Infra.VaultDir, constants.VaultKeyFilename)

	Infra.ProtocolConstantsDir = pathutil.SafeJoin(Infra.ProtocolDir, constants.ProtocolConstantsDirname)
	Infra.ProtocolModelsDir = pathutil.SafeJoin(Infra.ProtocolDir, constants.ProtocolModelsDirname)
	Infra.DbPath = pathutil.SafeJoin(Infra.DataDir, constants.DbFilename)
	Infra.LocalStateDBPath = pathutil.SafeJoin(Infra.RuntimeDir, constants.LocalStateDBFilename)
	Infra.SuspendedTransactionsDBPath = pathutil.SafeJoin(Infra.DataDir, constants.SuspendedTxFilename)
	Infra.AuditVaultDBPath = pathutil.SafeJoin(Infra.DataDir, constants.AuditVaultDBFilename)
	Infra.CaCertPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
	Infra.AppCertDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirApps)
	Infra.DocsDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.DocsDirname)
	Infra.SshConfigPath = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.SshConfigFilename)
	Infra.TestVaultDir = pathutil.SafeJoin(baseDir, constants.RuntimeDirname, constants.TestVaultDirname)
	Infra.RootCAPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)
	Infra.HubCAPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileHubCA)
	Infra.OperatorCAPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA)
	Infra.GatewayPeerCAPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileGatewayPeerCA)
	Infra.GatewayChainPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)
	Infra.TrustDomainJSONPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust, constants.PkiFileTrustDomainJSON)
	Infra.ServiceCertPath = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)
	Infra.PkiRootDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRoot)
	Infra.PkiAuthoritiesDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities)
	Infra.PkiIssuedHubDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub)
	Infra.PkiIssuedGatewayPeerDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirGatewayPeer)
	Infra.PkiTrustDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust)
	Infra.PkiRevocationDir = pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRevocation)
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
	Infra.OperatorLogFile = pathutil.SafeJoin(Infra.LogDir, constants.OperatorLogFilename)

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
	sshDir := pathutil.SafeJoin(homeDir, constants.SshDirname)
	return SSHConfigPaths{
		ConfigPath:      pathutil.SafeJoin(sshDir, constants.SshConfigBasename),
		KnownHostsPath:  pathutil.SafeJoin(sshDir, constants.SshKnownHostsBasename),
		IDE25519KeyPath: pathutil.SafeJoin(sshDir, constants.SshKeyEd25519),
		IDECDSAKeyPath:  pathutil.SafeJoin(sshDir, constants.SshKeyECDSA),
		IDRSAKeyPath:    pathutil.SafeJoin(sshDir, constants.SshKeyRSA),
	}
}
