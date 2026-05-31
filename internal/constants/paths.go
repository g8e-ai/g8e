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
		TestVaultDir         string
		LocalStateDBPath     string
		AuditVaultDBPath     string
	}{
		DbPath:               ".g8e/data/g8e.db",
		PkiDir:               ".g8e/pki",
		SecretsDir:           ".g8e/secrets",
		CaCertPath:           ".g8e/pki/trust/g8e-gw-ca-bundle.pem",
		AppCertDir:           ".g8e/pki/issued/apps",
		DocsDir:              ".g8e/docs",
		ProtocolDir:          ".g8e/protocol",
		ProtocolConstantsDir: ".g8e/protocol/constants",
		ProtocolModelsDir:    ".g8e/protocol/models",
		SshConfigPath:        ".g8e/ssh_config",
		RuntimeDir:           ".g8e",
		DataDir:              ".g8e/data",
		TestVaultDir:         ".g8e/test-vault",
		LocalStateDBPath:     ".g8e/local_state.db",
		AuditVaultDBPath:     ".g8e/audit_vault.db",
	},
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
	PathUsrLocalShareCaCertificates                           = "/usr/local/share/ca-certificates"
	PathEtcSslCerts                                           = "/etc/ssl/certs"
	PathUsrShareCaCertificates                                = "/usr/share/ca-certificates"
)

// CA certificate path constants
const (
	CACertDir              = ".g8e/pki/trust"
	CACertBundlePath       = ".g8e/pki/trust/g8e-gw-ca-bundle.pem"
	CACertLegacyBundlePath = ".g8e/pki/ca-bundle.pem"
	GatewayCACertPath      = "/usr/local/share/ca-certificates/g8e-gateway-ca.crt"
	GatewayOldCAPaths      = "/usr/local/share/ca-certificates/g8e*.crt /etc/ssl/certs/g8e*.pem /usr/share/ca-certificates/g8e*.crt"
)

func init() {
	resolvePaths()
}

// resolvePaths dynamically resolves filesystem paths from environment variables.
// Priority:
// 1. Explicit environment variable (e.g., G8E_PROTOCOL_DIR)
// 2. Relative to G8E_PROJECT_ROOT if set
// 3. Fallback to hardcoded default (container path)
func resolvePaths() {
	projectRoot := os.Getenv("G8E_PROJECT_ROOT")
	if projectRoot == "" {
		projectRoot = resolveProjectRoot()
	}

	// Resolve ProtocolDir
	if protocolDir := os.Getenv("G8E_PROTOCOL_DIR"); protocolDir != "" {
		if filepath.IsAbs(protocolDir) {
			Paths.Infra.ProtocolDir = protocolDir
		} else {
			Paths.Infra.ProtocolDir = filepath.Join(projectRoot, protocolDir)
		}
	} else {
		// Default to protocol/ relative to project root for host-native execution
		Paths.Infra.ProtocolDir = filepath.Join(projectRoot, "protocol")
	}

	// Update derived paths
	Paths.Infra.ProtocolConstantsDir = filepath.Join(Paths.Infra.ProtocolDir, "constants")
	Paths.Infra.ProtocolModelsDir = filepath.Join(Paths.Infra.ProtocolDir, "models")
}

// resolveProjectRoot returns the project root directory.
// This mirrors the logic in internal/services/system/path.go
// but is duplicated here to avoid circular dependencies during init.
func resolveProjectRoot() string {
	if root := os.Getenv("G8E_PROJECT_ROOT"); root != "" {
		abs, err := filepath.Abs(root)
		if err == nil {
			return abs
		}
		return root
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "." // Fallback to current working directory
	}

	// Try to find the root by looking for protocol or .git
	current := cwd
	for {
		_, protocolErr := os.Stat(filepath.Join(current, "protocol"))
		_, gitErr := os.Stat(filepath.Join(current, ".git"))

		if protocolErr == nil || gitErr == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return cwd
}
