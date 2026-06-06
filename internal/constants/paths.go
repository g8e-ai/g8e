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
	"os"
	"path/filepath"
)

// Paths defines canonical G8E filesystem paths.
// All paths are relative to the current working directory by default.
// The binary is fully self-contained and can run from any directory.
var Paths = struct {
	Infra struct {
		DbPath               string
		PkiDir               string
		SecretsDir           string
		CaCertPath           string
		AppCertDir           string
		DocsDir              string
		ProtocolDir          string
		ProtocolConstantsDir string
		ProtocolModelsDir    string
		SshConfigPath        string
		RuntimeDir           string
		DataDir              string
		VaultDir             string
		TestVaultDir         string
		LocalStateDBPath     string
		AuditVaultDBPath     string
	}
}{
	Infra: struct {
		DbPath               string
		PkiDir               string
		SecretsDir           string
		CaCertPath           string
		AppCertDir           string
		DocsDir              string
		ProtocolDir          string
		ProtocolConstantsDir string
		ProtocolModelsDir    string
		SshConfigPath        string
		RuntimeDir           string
		DataDir              string
		VaultDir             string
		TestVaultDir         string
		LocalStateDBPath     string
		AuditVaultDBPath     string
	}{
		DbPath:               ".g8e/data/g8e.db",
		PkiDir:               ".g8e/pki",
		SecretsDir:           ".g8e/secrets",
		CaCertPath:           ".g8e/pki/trust/g8eg-ca-bundle.pem",
		AppCertDir:           ".g8e/pki/issued/apps",
		DocsDir:              ".g8e/docs",
		ProtocolDir:          ".g8e/protocol",
		ProtocolConstantsDir: ".g8e/protocol/constants",
		ProtocolModelsDir:    ".g8e/protocol/models",
		SshConfigPath:        ".g8e/ssh_config",
		RuntimeDir:           ".g8e",
		DataDir:              ".g8e/data",
		VaultDir:             ".g8e/vault",
		TestVaultDir:         ".g8e/test-vault",
		LocalStateDBPath:     ".g8e/local_state.db",
		AuditVaultDBPath:     ".g8e/audit_vault.db",
	},
}

// InitPaths initializes paths relative to the current working directory.
// This should be called once at program startup.
// All paths are resolved relative to cwd, making the binary fully self-contained.
func InitPaths() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	InitPathsWithBase(cwd)
}

// InitPathsWithBase initializes paths relative to the specified base directory.
// This allows tests and specific use cases to override the default cwd behavior.
func InitPathsWithBase(baseDir string) {
	// Resolve all paths relative to baseDir
	Paths.Infra.RuntimeDir = filepath.Join(baseDir, ".g8e")
	Paths.Infra.DataDir = filepath.Join(baseDir, ".g8e/data")
	Paths.Infra.PkiDir = filepath.Join(baseDir, ".g8e/pki")
	Paths.Infra.SecretsDir = filepath.Join(baseDir, ".g8e/secrets")
	Paths.Infra.ProtocolDir = filepath.Join(baseDir, ".g8e/protocol")
	Paths.Infra.VaultDir = filepath.Join(baseDir, ".g8e/vault")

	// Update derived paths
	Paths.Infra.ProtocolConstantsDir = filepath.Join(Paths.Infra.ProtocolDir, "constants")
	Paths.Infra.ProtocolModelsDir = filepath.Join(Paths.Infra.ProtocolDir, "models")
	Paths.Infra.DbPath = filepath.Join(Paths.Infra.DataDir, "g8e.db")
	Paths.Infra.LocalStateDBPath = filepath.Join(Paths.Infra.RuntimeDir, "local_state.db")
	Paths.Infra.AuditVaultDBPath = filepath.Join(Paths.Infra.DataDir, "audit_vault.db")
	Paths.Infra.CaCertPath = filepath.Join(Paths.Infra.PkiDir, "trust/g8eg-ca-bundle.pem")
	Paths.Infra.AppCertDir = filepath.Join(Paths.Infra.PkiDir, "issued/apps")
	Paths.Infra.DocsDir = filepath.Join(baseDir, ".g8e/docs")
	Paths.Infra.SshConfigPath = filepath.Join(baseDir, ".g8e/ssh_config")
	Paths.Infra.TestVaultDir = filepath.Join(baseDir, ".g8e/test-vault")
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

// CA certificate path constants (deprecated - use Paths.Infra.CaCertPath instead)
const (
	CACertDir              = ".g8e/pki/trust"
	CACertBundlePath       = ".g8e/pki/trust/g8eg-ca-bundle.pem"
	CACertLegacyBundlePath = ".g8e/pki/ca-bundle.pem"
)
