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

package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pathutil"
)

// restoreInfra saves the current Infra state and package-level vars,
// and registers a cleanup to restore them after the test.
func restoreInfra(t *testing.T) {
	t.Helper()
	snapshot := Infra
	gwID := GatewayIDPath
	actJSON := ActuatorPubJSONPath
	actPEM := ActuatorPubPEMPath
	netID := NetworkIdentityPath
	peerCert := PeerCertPath
	peerKey := PeerKeyPath
	peerChain := PeerChainPath
	pkiGwKey := PkiGatewayKeyPath
	t.Cleanup(func() {
		Infra = snapshot
		GatewayIDPath = gwID
		ActuatorPubJSONPath = actJSON
		ActuatorPubPEMPath = actPEM
		NetworkIdentityPath = netID
		PeerCertPath = peerCert
		PeerKeyPath = peerKey
		PeerChainPath = peerChain
		PkiGatewayKeyPath = pkiGwKey
	})
}

func TestInit(t *testing.T) {
	restoreInfra(t)

	if err := Init(); err != nil {
		t.Fatalf("Init() returned unexpected error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() failed: %v", err)
	}

	expectedRuntime := filepath.Join(cwd, constants.RuntimeDirname)
	if Infra.RuntimeDir != expectedRuntime {
		t.Errorf("Infra.RuntimeDir = %q, want %q", Infra.RuntimeDir, expectedRuntime)
	}
}

func TestInitWithBase(t *testing.T) {
	restoreInfra(t)

	base := "/test/base"
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) returned unexpected error: %v", base, err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RuntimeDir", Infra.RuntimeDir, pathutil.SafeJoin(base, constants.RuntimeDirname)},
		{"DataDir", Infra.DataDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.DataDirname)},
		{"PkiDir", Infra.PkiDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.PkiDirname)},
		{"SecretsDir", Infra.SecretsDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.SecretsDirname)},
		{"ProtocolDir", Infra.ProtocolDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.ProtocolDirname)},
		{"VaultDir", Infra.VaultDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.VaultDirname)},
		{"VaultKeyPath", Infra.VaultKeyPath, pathutil.SafeJoin(Infra.VaultDir, constants.VaultKeyFilename)},
		{"ProtocolConstantsDir", Infra.ProtocolConstantsDir, pathutil.SafeJoin(Infra.ProtocolDir, constants.ProtocolConstantsDirname)},
		{"ProtocolModelsDir", Infra.ProtocolModelsDir, pathutil.SafeJoin(Infra.ProtocolDir, constants.ProtocolModelsDirname)},
		{"DbPath", Infra.DbPath, pathutil.SafeJoin(Infra.DataDir, constants.DbFilename)},
		{"LocalStateDBPath", Infra.LocalStateDBPath, pathutil.SafeJoin(Infra.RuntimeDir, constants.LocalStateDBFilename)},
		{"SuspendedTransactionsDBPath", Infra.SuspendedTransactionsDBPath, pathutil.SafeJoin(Infra.DataDir, constants.SuspendedTxFilename)},
		{"AuditVaultDBPath", Infra.AuditVaultDBPath, pathutil.SafeJoin(Infra.DataDir, constants.AuditVaultDBFilename)},
		{"CaCertPath", Infra.CaCertPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)},
		{"AppCertDir", Infra.AppCertDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirApps)},
		{"DocsDir", Infra.DocsDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.DocsDirname)},
		{"SshConfigPath", Infra.SshConfigPath, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.SshConfigFilename)},
		{"TestVaultDir", Infra.TestVaultDir, pathutil.SafeJoin(base, constants.RuntimeDirname, constants.TestVaultDirname)},
		{"RootCAPath", Infra.RootCAPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRoot, constants.PkiFileRootCA)},
		{"HubCAPath", Infra.HubCAPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileHubCA)},
		{"OperatorCAPath", Infra.OperatorCAPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileOperatorCA)},
		{"GatewayPeerCAPath", Infra.GatewayPeerCAPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities, constants.PkiFileGatewayPeerCA)},
		{"GatewayChainPath", Infra.GatewayChainPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayChain)},
		{"TrustDomainJSONPath", Infra.TrustDomainJSONPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust, constants.PkiFileTrustDomainJSON)},
		{"ServiceCertPath", Infra.ServiceCertPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub, constants.PkiFileGatewayCert)},
		{"PkiRootDir", Infra.PkiRootDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRoot)},
		{"PkiAuthoritiesDir", Infra.PkiAuthoritiesDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirAuthorities)},
		{"PkiIssuedHubDir", Infra.PkiIssuedHubDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirHub)},
		{"PkiIssuedGatewayPeerDir", Infra.PkiIssuedGatewayPeerDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirIssued, constants.PkiSubdirGatewayPeer)},
		{"PkiTrustDir", Infra.PkiTrustDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrust)},
		{"PkiRevocationDir", Infra.PkiRevocationDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirRevocation)},
		{"ActuatorPubJSONPath", Infra.ActuatorPubJSONPath, pathutil.SafeJoin(Infra.PkiDir, constants.ActuatorPubJSONFilename)},
		{"ActuatorPubPEMPath", Infra.ActuatorPubPEMPath, pathutil.SafeJoin(Infra.PkiDir, constants.ActuatorPubPEMFilename)},
		{"OperatorKeyPath", Infra.OperatorKeyPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorKey)},
		{"OperatorCertPath", Infra.OperatorCertPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorCert)},
		{"OperatorChainPath", Infra.OperatorChainPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileOperatorChain)},
		{"WardenPubPath", Infra.WardenPubPath, pathutil.SafeJoin(Infra.PkiDir, constants.PkiFileWardenPub)},
		{"RootCAKeyPath", Infra.RootCAKeyPath, pathutil.SafeJoin(Infra.PkiRootDir, constants.PkiFileRootCAKey)},
		{"TrustedSignersDir", Infra.TrustedSignersDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirTrustedSigners)},
		{"ClientPkiDir", Infra.ClientPkiDir, pathutil.SafeJoin(Infra.PkiDir, constants.PkiSubdirClient)},
		{"ClientOperatorKeyPath", Infra.ClientOperatorKeyPath, pathutil.SafeJoin(Infra.ClientPkiDir, constants.PkiFileOperatorKey)},
		{"ClientOperatorCertPath", Infra.ClientOperatorCertPath, pathutil.SafeJoin(Infra.ClientPkiDir, constants.PkiFileOperatorCert)},
		{"SessionEncKeyPath", Infra.SessionEncKeyPath, pathutil.SafeJoin(Infra.SecretsDir, constants.SecretsFileSessionEncryptionKey)},
		{"BootstrapDigestPath", Infra.BootstrapDigestPath, pathutil.SafeJoin(Infra.SecretsDir, constants.SecretsFileBootstrapDigest)},
		{"LogDir", Infra.LogDir, pathutil.SafeJoin(Infra.RuntimeDir, constants.LogDirname)},
		{"OperatorLogFile", Infra.OperatorLogFile, pathutil.SafeJoin(Infra.LogDir, constants.OperatorLogFilename)},
		{"ExecutionVaultDBPath", Infra.ExecutionVaultDBPath, pathutil.SafeJoin(Infra.DataDir, constants.ExecutionVaultDBFilename)},
		{"ReplayStoreDBPath", Infra.ReplayStoreDBPath, pathutil.SafeJoin(Infra.DataDir, constants.ReplayStoreDBFilename)},
		{"LedgerDir", Infra.LedgerDir, pathutil.SafeJoin(Infra.DataDir, constants.LedgerDirname)},
		{"DemosDir", Infra.DemosDir, pathutil.SafeJoin(base, constants.DemosDirname)},
		{"DemosHealthcareDir", Infra.DemosHealthcareDir, pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgHealthcare)},
		{"DemosFinanceDir", Infra.DemosFinanceDir, pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgFinance)},
		{"DemosGovDir", Infra.DemosGovDir, pathutil.SafeJoin(Infra.DemosDir, constants.DemosOrgGov)},
		{"DemosHealthcareTargetDataDir", Infra.DemosHealthcareTargetDataDir, pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosTargetDataDir)},
		{"DemosHealthcareDoctrineDir", Infra.DemosHealthcareDoctrineDir, pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosDoctrineDir)},
		{"DemosHealthcarePARequestsPath", Infra.DemosHealthcarePARequestsPath, pathutil.SafeJoin(Infra.DemosHealthcareTargetDataDir, constants.DemosPARequestsFile)},
		{"DemosHealthcareComposePath", Infra.DemosHealthcareComposePath, pathutil.SafeJoin(Infra.DemosHealthcareDir, constants.DemosComposeFile)},
		{"DemosHealthcareDoctrineHIPAAPath", Infra.DemosHealthcareDoctrineHIPAAPath, pathutil.SafeJoin(Infra.DemosHealthcareDoctrineDir, constants.DemosHIPAADoctrineFile)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestInitWithBase_PackageVars(t *testing.T) {
	restoreInfra(t)

	base := "/test/base"
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) returned unexpected error: %v", base, err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GatewayIDPath", GatewayIDPath, pathutil.SafeJoin(Infra.DataDir, constants.GatewayIDFilename)},
		{"NetworkIdentityPath", NetworkIdentityPath, pathutil.SafeJoin(Infra.PkiDir, constants.NetworkIdentityFilename)},
		{"PeerCertPath", PeerCertPath, pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerCertFilename)},
		{"PeerKeyPath", PeerKeyPath, pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerKeyFilename)},
		{"PeerChainPath", PeerChainPath, pathutil.SafeJoin(Infra.PkiDir, constants.PeerSubdir, constants.PeerChainFilename)},
		{"PkiGatewayKeyPath", PkiGatewayKeyPath, pathutil.SafeJoin(Infra.PkiIssuedHubDir, constants.PkiFileGatewayKey)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestInitWithBase_AllFieldsPopulated(t *testing.T) {
	restoreInfra(t)

	base := "/test/base"
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) returned unexpected error: %v", base, err)
	}

	// Every field in Infra should be non-empty after InitWithBase.
	// PkiBinariesDir is intentionally excluded — it is declared in the struct
	// but not populated by InitWithBase (it uses the default zero value).
	skipFields := map[string]bool{"PkiBinariesDir": true}

	v := reflect.ValueOf(&Infra).Elem()
	t2 := v.Type()
	var emptyFields []string
	for i := 0; i < t2.NumField(); i++ {
		name := t2.Field(i).Name
		if skipFields[name] {
			continue
		}
		if v.Field(i).String() == "" {
			emptyFields = append(emptyFields, name)
		}
	}

	if len(emptyFields) > 0 {
		t.Errorf("Infra fields empty after InitWithBase: %v", emptyFields)
	}
}

func TestInitWithBase_RuntimePathsPrefixedWithBase(t *testing.T) {
	restoreInfra(t)

	base := "/my/project/root"
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) returned unexpected error: %v", base, err)
	}

	// All runtime paths should start with the base directory.
	// Demos paths are relative to baseDir directly (not under runtime dir).
	runtimePaths := []struct {
		name string
		val  string
	}{
		{"RuntimeDir", Infra.RuntimeDir},
		{"DataDir", Infra.DataDir},
		{"PkiDir", Infra.PkiDir},
		{"SecretsDir", Infra.SecretsDir},
		{"ProtocolDir", Infra.ProtocolDir},
		{"VaultDir", Infra.VaultDir},
		{"DocsDir", Infra.DocsDir},
		{"SshConfigPath", Infra.SshConfigPath},
		{"TestVaultDir", Infra.TestVaultDir},
		{"LogDir", Infra.LogDir},
		{"DemosDir", Infra.DemosDir},
	}

	for _, tc := range runtimePaths {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(filepath.ToSlash(tc.val), filepath.ToSlash(base)) {
				t.Errorf("%s = %q, expected prefix %q", tc.name, tc.val, base)
			}
		})
	}
}

func TestInitWithBase_DerivedPathsPrefixedWithParent(t *testing.T) {
	restoreInfra(t)

	base := "/my/project/root"
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) returned unexpected error: %v", base, err)
	}

	// Derived paths should be prefixed with their logical parent directory.
	derivedPaths := []struct {
		name   string
		val    string
		parent string
	}{
		{"DbPath", Infra.DbPath, Infra.DataDir},
		{"VaultKeyPath", Infra.VaultKeyPath, Infra.VaultDir},
		{"ProtocolConstantsDir", Infra.ProtocolConstantsDir, Infra.ProtocolDir},
		{"ProtocolModelsDir", Infra.ProtocolModelsDir, Infra.ProtocolDir},
		{"LocalStateDBPath", Infra.LocalStateDBPath, Infra.RuntimeDir},
		{"SuspendedTransactionsDBPath", Infra.SuspendedTransactionsDBPath, Infra.DataDir},
		{"AuditVaultDBPath", Infra.AuditVaultDBPath, Infra.DataDir},
		{"CaCertPath", Infra.CaCertPath, Infra.PkiDir},
		{"RootCAPath", Infra.RootCAPath, Infra.PkiDir},
		{"HubCAPath", Infra.HubCAPath, Infra.PkiDir},
		{"OperatorCAPath", Infra.OperatorCAPath, Infra.PkiDir},
		{"GatewayPeerCAPath", Infra.GatewayPeerCAPath, Infra.PkiDir},
		{"RootCAKeyPath", Infra.RootCAKeyPath, Infra.PkiRootDir},
		{"ClientOperatorKeyPath", Infra.ClientOperatorKeyPath, Infra.ClientPkiDir},
		{"ClientOperatorCertPath", Infra.ClientOperatorCertPath, Infra.ClientPkiDir},
		{"SessionEncKeyPath", Infra.SessionEncKeyPath, Infra.SecretsDir},
		{"BootstrapDigestPath", Infra.BootstrapDigestPath, Infra.SecretsDir},
		{"OperatorLogFile", Infra.OperatorLogFile, Infra.LogDir},
		{"ExecutionVaultDBPath", Infra.ExecutionVaultDBPath, Infra.DataDir},
		{"ReplayStoreDBPath", Infra.ReplayStoreDBPath, Infra.DataDir},
		{"LedgerDir", Infra.LedgerDir, Infra.DataDir},
		{"GatewayIDPath", GatewayIDPath, Infra.DataDir},
		{"NetworkIdentityPath", NetworkIdentityPath, Infra.PkiDir},
		{"PeerCertPath", PeerCertPath, Infra.PkiDir},
		{"PeerKeyPath", PeerKeyPath, Infra.PkiDir},
		{"PeerChainPath", PeerChainPath, Infra.PkiDir},
		{"PkiGatewayKeyPath", PkiGatewayKeyPath, Infra.PkiIssuedHubDir},
	}

	for _, tc := range derivedPaths {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(tc.val, tc.parent) {
				t.Errorf("%s = %q, expected prefix %q", tc.name, tc.val, tc.parent)
			}
		})
	}
}

func TestInitWithBase_DifferentBaseDirs(t *testing.T) {
	restoreInfra(t)

	bases := []string{"/a", "/b", "/c", "/some/very/deep/nested/path"}

	for _, base := range bases {
		t.Run(base, func(t *testing.T) {
			if err := InitWithBase(base); err != nil {
				t.Fatalf("InitWithBase(%q) error: %v", base, err)
			}
			expectedRuntime := filepath.Join(base, constants.RuntimeDirname)
			if Infra.RuntimeDir != expectedRuntime {
				t.Errorf("RuntimeDir = %q, want %q", Infra.RuntimeDir, expectedRuntime)
			}
			expectedData := filepath.Join(expectedRuntime, constants.DataDirname)
			if Infra.DataDir != expectedData {
				t.Errorf("DataDir = %q, want %q", Infra.DataDir, expectedData)
			}
		})
	}
}

func TestInitWithBase_RelativeBaseDir(t *testing.T) {
	restoreInfra(t)

	base := "."
	if err := InitWithBase(base); err != nil {
		t.Fatalf("InitWithBase(%q) error: %v", base, err)
	}

	expectedRuntime := filepath.Join(base, constants.RuntimeDirname)
	if Infra.RuntimeDir != expectedRuntime {
		t.Errorf("RuntimeDir = %q, want %q", Infra.RuntimeDir, expectedRuntime)
	}
}

func TestInitWithBase_Idempotent(t *testing.T) {
	restoreInfra(t)

	base := "/idempotent/test"

	if err := InitWithBase(base); err != nil {
		t.Fatalf("first InitWithBase error: %v", err)
	}
	firstRuntime := Infra.RuntimeDir
	firstData := Infra.DataDir
	firstPki := Infra.PkiDir

	if err := InitWithBase(base); err != nil {
		t.Fatalf("second InitWithBase error: %v", err)
	}

	if Infra.RuntimeDir != firstRuntime {
		t.Errorf("RuntimeDir changed on re-init: %q vs %q", Infra.RuntimeDir, firstRuntime)
	}
	if Infra.DataDir != firstData {
		t.Errorf("DataDir changed on re-init: %q vs %q", Infra.DataDir, firstData)
	}
	if Infra.PkiDir != firstPki {
		t.Errorf("PkiDir changed on re-init: %q vs %q", Infra.PkiDir, firstPki)
	}
}

func TestGetSuspendedTransactionsDBPath(t *testing.T) {
	tests := []struct {
		name    string
		dataDir string
		want    string
	}{
		{
			name:    "simple data dir",
			dataDir: "/data",
			want:    pathutil.SafeJoin("/data", constants.SuspendedTxFilename),
		},
		{
			name:    "nested data dir",
			dataDir: "/var/lib/g8e/data",
			want:    pathutil.SafeJoin("/var/lib/g8e/data", constants.SuspendedTxFilename),
		},
		{
			name:    "relative data dir",
			dataDir: "data",
			want:    pathutil.SafeJoin("data", constants.SuspendedTxFilename),
		},
		{
			name:    "empty data dir",
			dataDir: "",
			want:    pathutil.SafeJoin("", constants.SuspendedTxFilename),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetSuspendedTransactionsDBPath(tc.dataDir)
			if got != tc.want {
				t.Errorf("GetSuspendedTransactionsDBPath(%q) = %q, want %q", tc.dataDir, got, tc.want)
			}
		})
	}
}

func TestGetSuspendedTransactionsDBPath_FilenameSuffix(t *testing.T) {
	got := GetSuspendedTransactionsDBPath("/some/dir")
	if !strings.HasSuffix(got, constants.SuspendedTxFilename) {
		t.Errorf("GetSuspendedTransactionsDBPath result %q should end with %q", got, constants.SuspendedTxFilename)
	}
}

func TestGetAgentConfigPaths(t *testing.T) {
	home := "/home/testuser"
	got := GetAgentConfigPaths(home)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"DevinConfigDir", got.DevinConfigDir, pathutil.SafeJoin(home, constants.AgentConfigDirDevin)},
		{"DevinConfigPath", got.DevinConfigPath, pathutil.SafeJoin(home, constants.AgentConfigDirDevin, constants.AgentConfigFileMCPDevin)},
		{"GeminiConfigDir", got.GeminiConfigDir, pathutil.SafeJoin(home, constants.AgentConfigDirGemini)},
		{"GeminiConfigPath", got.GeminiConfigPath, pathutil.SafeJoin(home, constants.AgentConfigDirGemini, constants.AgentConfigFileSettings)},
		{"GooseYAMLConfigDir", got.GooseYAMLConfigDir, pathutil.SafeJoin(home, constants.AgentConfigDirGoose)},
		{"GooseYAMLConfigPath", got.GooseYAMLConfigPath, pathutil.SafeJoin(home, constants.AgentConfigDirGoose, constants.AgentConfigFileGooseYAML)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestGetAgentConfigPaths_AllFieldsPopulated(t *testing.T) {
	got := GetAgentConfigPaths("/home/user")

	if got.DevinConfigDir == "" || got.DevinConfigPath == "" {
		t.Error("Devin fields should be populated")
	}
	if got.GeminiConfigDir == "" || got.GeminiConfigPath == "" {
		t.Error("Gemini fields should be populated")
	}
	if got.GooseYAMLConfigDir == "" || got.GooseYAMLConfigPath == "" {
		t.Error("Goose fields should be populated")
	}
}

func TestGetAgentConfigPaths_ConfigPathInsideConfigDir(t *testing.T) {
	home := "/home/user"
	got := GetAgentConfigPaths(home)

	pairs := []struct {
		dir  string
		path string
		name string
	}{
		{got.DevinConfigDir, got.DevinConfigPath, "Devin"},
		{got.GeminiConfigDir, got.GeminiConfigPath, "Gemini"},
		{got.GooseYAMLConfigDir, got.GooseYAMLConfigPath, "Goose"},
	}

	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			if !strings.HasPrefix(p.path, p.dir) {
				t.Errorf("%s config path %q should start with config dir %q", p.name, p.path, p.dir)
			}
		})
	}
}

func TestGetAgentConfigPaths_DifferentHomeDirs(t *testing.T) {
	homes := []string{"/home/alice", "/home/bob", "/root"}

	for _, home := range homes {
		t.Run(home, func(t *testing.T) {
			got := GetAgentConfigPaths(home)
			if !strings.HasPrefix(filepath.ToSlash(got.GeminiConfigDir), filepath.ToSlash(home)) {
				t.Errorf("GeminiConfigDir %q should start with %q", got.GeminiConfigDir, home)
			}
		})
	}
}

func TestGetSSHConfigPaths(t *testing.T) {
	home := "/home/testuser"
	got := GetSSHConfigPaths(home)

	sshDir := pathutil.SafeJoin(home, constants.SshDirname)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigPath", got.ConfigPath, pathutil.SafeJoin(sshDir, constants.SshConfigBasename)},
		{"KnownHostsPath", got.KnownHostsPath, pathutil.SafeJoin(sshDir, constants.SshKnownHostsBasename)},
		{"IDE25519KeyPath", got.IDE25519KeyPath, pathutil.SafeJoin(sshDir, constants.SshKeyEd25519)},
		{"IDECDSAKeyPath", got.IDECDSAKeyPath, pathutil.SafeJoin(sshDir, constants.SshKeyECDSA)},
		{"IDRSAKeyPath", got.IDRSAKeyPath, pathutil.SafeJoin(sshDir, constants.SshKeyRSA)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestGetSSHConfigPaths_AllFieldsPopulated(t *testing.T) {
	got := GetSSHConfigPaths("/home/user")

	if got.ConfigPath == "" {
		t.Error("ConfigPath should be populated")
	}
	if got.KnownHostsPath == "" {
		t.Error("KnownHostsPath should be populated")
	}
	if got.IDE25519KeyPath == "" {
		t.Error("IDE25519KeyPath should be populated")
	}
	if got.IDECDSAKeyPath == "" {
		t.Error("IDECDSAKeyPath should be populated")
	}
	if got.IDRSAKeyPath == "" {
		t.Error("IDRSAKeyPath should be populated")
	}
}

func TestGetSSHConfigPaths_PathsInsideSSHDir(t *testing.T) {
	home := "/home/user"
	got := GetSSHConfigPaths(home)

	sshDir := pathutil.SafeJoin(home, constants.SshDirname)

	allPaths := []struct {
		name string
		val  string
	}{
		{"ConfigPath", got.ConfigPath},
		{"KnownHostsPath", got.KnownHostsPath},
		{"IDE25519KeyPath", got.IDE25519KeyPath},
		{"IDECDSAKeyPath", got.IDECDSAKeyPath},
		{"IDRSAKeyPath", got.IDRSAKeyPath},
	}

	for _, p := range allPaths {
		t.Run(p.name, func(t *testing.T) {
			if !strings.HasPrefix(p.val, sshDir) {
				t.Errorf("%s = %q, expected prefix %q", p.name, p.val, sshDir)
			}
		})
	}
}

func TestGetSSHConfigPaths_DifferentHomeDirs(t *testing.T) {
	homes := []string{"/home/alice", "/home/bob", "/root"}

	for _, home := range homes {
		t.Run(home, func(t *testing.T) {
			got := GetSSHConfigPaths(home)
			expectedSSHDir := pathutil.SafeJoin(home, constants.SshDirname)
			if !strings.HasPrefix(got.ConfigPath, expectedSSHDir) {
				t.Errorf("ConfigPath %q should start with %q", got.ConfigPath, expectedSSHDir)
			}
		})
	}
}

func TestInfraDefaults_BeforeInit(t *testing.T) {
	restoreInfra(t)

	// Reset Infra to zero values to test defaults.
	Infra = struct {
		DbPath                           string
		PkiDir                           string
		SecretsDir                       string
		CaCertPath                       string
		AppCertDir                       string
		DocsDir                          string
		ProtocolDir                      string
		ProtocolConstantsDir             string
		ProtocolModelsDir                string
		SshConfigPath                    string
		RuntimeDir                       string
		DataDir                          string
		VaultDir                         string
		VaultKeyPath                     string
		TestVaultDir                     string
		LocalStateDBPath                 string
		SuspendedTransactionsDBPath      string
		AuditVaultDBPath                 string
		RootCAPath                       string
		HubCAPath                        string
		OperatorCAPath                   string
		GatewayPeerCAPath                string
		GatewayChainPath                 string
		TrustDomainJSONPath              string
		ServiceCertPath                  string
		PkiRootDir                       string
		PkiAuthoritiesDir                string
		PkiIssuedDir                     string
		PkiIssuedHubDir                  string
		PkiIssuedGatewayPeerDir          string
		PkiTrustDir                      string
		PkiRevocationDir                 string
		PkiBinariesDir                   string
		ActuatorPubJSONPath              string
		ActuatorPubPEMPath               string
		OperatorKeyPath                  string
		OperatorCertPath                 string
		OperatorChainPath                string
		WardenPubPath                    string
		RootCAKeyPath                    string
		TrustedSignersDir                string
		ClientPkiDir                     string
		ClientOperatorKeyPath            string
		ClientOperatorCertPath           string
		SessionEncKeyPath                string
		BootstrapDigestPath              string
		LogDir                           string
		OperatorLogFile                  string
		PidDir                           string
		OperatorPostureFile              string
		OperatorPIDFile                  string
		ExecutionVaultDBPath             string
		ReplayStoreDBPath                string
		LedgerDir                        string
		LedgerFilesDir                   string
		DemosDir                         string
		DemosHealthcareDir               string
		DemosFinanceDir                  string
		DemosGovDir                      string
		DemosHealthcareTargetDataDir     string
		DemosHealthcareDoctrineDir       string
		DemosHealthcarePARequestsPath    string
		DemosHealthcareComposePath       string
		DemosHealthcareDoctrineHIPAAPath string
	}{}

	// After resetting, all fields should be empty.
	if Infra.RuntimeDir != "" {
		t.Errorf("RuntimeDir should be empty before Init, got %q", Infra.RuntimeDir)
	}
	if Infra.DataDir != "" {
		t.Errorf("DataDir should be empty before Init, got %q", Infra.DataDir)
	}

	// After InitWithBase, fields should be populated.
	if err := InitWithBase("/test"); err != nil {
		t.Fatalf("InitWithBase error: %v", err)
	}
	if Infra.RuntimeDir == "" {
		t.Error("RuntimeDir should be populated after InitWithBase")
	}
}
