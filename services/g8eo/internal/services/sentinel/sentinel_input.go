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

package sentinel

import (
	"regexp"
	"strings"
)

// InputThreatDetector extends ThreatDetector with block recommendation
type InputThreatDetector struct {
	name             string
	pattern          *regexp.Regexp
	category         ThreatCategory
	severity         ThreatSeverity
	confidence       float64
	mitreAttack      string
	mitreTactic      string
	recommendation   string
	blockRecommended bool
}

func (d *InputThreatDetector) Name() string { return d.name }

func (d *InputThreatDetector) Detect(input string) []ThreatSignal {
	if d.pattern.MatchString(input) {
		return []ThreatSignal{{
			Category:         d.category,
			Severity:         d.severity,
			Indicator:        d.name,
			Confidence:       d.confidence,
			MitreAttack:      d.mitreAttack,
			MitreTactic:      d.mitreTactic,
			Recommendation:   d.recommendation,
			BlockRecommended: d.blockRecommended,
		}}
	}
	return nil
}

// CriticalSystemPaths lists paths that should trigger elevated scrutiny
var CriticalSystemPaths = []string{
	"/etc/passwd",
	"/etc/shadow",
	"/etc/group",
	"/etc/gshadow",
	"/etc/sudoers",
	"/etc/sudoers.d/",
	"/etc/ssh/sshd_config",
	"/etc/ssh/ssh_config",
	"/etc/pam.d/",
	"/etc/security/",
	"/etc/ld.so.conf",
	"/etc/ld.so.preload",
	"/etc/hosts",
	"/etc/resolv.conf",
	"/etc/fstab",
	"/etc/crontab",
	"/etc/cron.d/",
	"/etc/cron.daily/",
	"/etc/cron.hourly/",
	"/etc/init.d/",
	"/etc/systemd/system/",
	"/etc/rc.local",
	"/etc/profile",
	"/etc/profile.d/",
	"/etc/bash.bashrc",
	"/etc/environment",
	"/etc/selinux/",
	"/etc/apparmor/",
	"/etc/apparmor.d/",
	"/boot/",
	"/root/.ssh/",
	"/root/.bashrc",
	"/root/.bash_profile",
	"/root/.profile",
}

// CriticalSystemDirs lists directories where any modification is high risk
var CriticalSystemDirs = []string{
	"/bin",
	"/sbin",
	"/usr/bin",
	"/usr/sbin",
	"/usr/local/bin",
	"/usr/local/sbin",
	"/lib",
	"/lib64",
	"/usr/lib",
	"/boot",
	"/proc",
	"/sys",
	"/dev",
}

func (s *Sentinel) initializeInputThreatDetectors() {
	if !s.config.ThreatDetectionEnabled {
		return
	}

	s.inputThreatDetectors = []ThreatDetector{
		&InputThreatDetector{
			name:             "destroy_rm_rf_root",
			pattern:          regexp.MustCompile(`(?i)\brm\s+(-[rRf]+\s+)*/*\s*$|\brm\s+(-[rRf]+\s+)*/\s|\brm\s+-[rRf]*\s+/\s`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.99,
			mitreAttack:      "T1485",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted deletion of root filesystem",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_rm_rf_system_dirs",
			pattern:          regexp.MustCompile(`(?i)\brm\s+(-[rRf]+\s+)*/(bin|boot|dev|etc|lib|lib64|opt|proc|root|run|sbin|srv|sys|usr|var)\b`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1485",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted deletion of critical system directory",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_dd_disk",
			pattern:          regexp.MustCompile(`(?i)\bdd\s+.*of=/dev/(sd[a-z]|hd[a-z]|nvme[0-9]n[0-9]|vd[a-z]|xvd[a-z])(\s|$)`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1561.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted raw disk write - will destroy data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_mkfs",
			pattern:          regexp.MustCompile(`(?i)\bmkfs(\.[a-z0-9]+)?\s+/dev/`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1561.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted filesystem format - will destroy all data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_shred_device",
			pattern:          regexp.MustCompile(`(?i)\bshred\s+.*(/dev/sd|/dev/hd|/dev/nvme|/dev/vd|/dev/xvd)`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1485",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted secure wipe of storage device",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_wipefs",
			pattern:          regexp.MustCompile(`(?i)\bwipefs\s+(-a\s+)?/dev/`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1561.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted filesystem signature wipe",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "destroy_fdisk",
			pattern:          regexp.MustCompile(`(?i)\b(fdisk|gdisk|parted|sfdisk)\s+/dev/`),
			category:         ThreatCategoryDataDestruction,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1561.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted partition table modification",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "tamper_passwd_shadow",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/(passwd|shadow|group|gshadow)`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1136.001",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Attempted modification of authentication files",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_sudoers",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/sudoers`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1548.003",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Attempted modification of sudoers",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_pam",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/pam\.d/`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1556.003",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted modification of PAM configuration",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_sshd_config",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee|sed|awk)\s+.*>+\s*/etc/ssh/sshd_config`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1098.004",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Attempted modification of SSH daemon config",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_hosts",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/hosts`),
			category:         ThreatCategoryNetworkManipulation,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1565.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted modification of /etc/hosts",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_resolv_conf",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/resolv\.conf`),
			category:         ThreatCategoryNetworkManipulation,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1565.001",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Attempted modification of DNS configuration",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "tamper_ld_so",
			pattern:          regexp.MustCompile(`(?i)(echo|cat|printf|tee)\s+.*>+\s*/etc/ld\.so\.(conf|preload)`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1574.006",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Attempted modification of dynamic linker config",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "bypass_disable_selinux",
			pattern:          regexp.MustCompile(`(?i)\bsetenforce\s+0\b|sed\s+.*SELINUX=.*disabled`),
			category:         ThreatCategorySecurityBypass,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempted to disable SELinux",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "bypass_disable_apparmor",
			pattern:          regexp.MustCompile(`(?i)\baa-disable\b|\bsystemctl\s+(stop|disable)\s+apparmor`),
			category:         ThreatCategorySecurityBypass,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempted to disable AppArmor",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "bypass_disable_firewall",
			pattern:          regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+(ufw|firewalld|iptables)|\bufw\s+disable\b|\biptables\s+-F\b`),
			category:         ThreatCategorySecurityBypass,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1562.004",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempted to disable firewall",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "bypass_disable_auditd",
			pattern:          regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+auditd|\bauditctl\s+-e\s+0\b`),
			category:         ThreatCategorySecurityBypass,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempted to disable audit daemon",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "malware_curl_bash",
			pattern:          regexp.MustCompile(`(?i)\bcurl\s+.*\|\s*(ba)?sh\b`),
			category:         ThreatCategoryMalwareDeployment,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Piping remote content directly to shell",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "malware_wget_bash",
			pattern:          regexp.MustCompile(`(?i)\bwget\s+.*(-O\s*-|--output-document=-).*\|\s*(ba)?sh\b`),
			category:         ThreatCategoryMalwareDeployment,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Piping remote content directly to shell",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "malware_eval_base64",
			pattern:          regexp.MustCompile(`(?i)\beval\s+.*\$\(.*base64\s+-d`),
			category:         ThreatCategoryMalwareDeployment,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1027",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Executing obfuscated base64 content",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "malware_python_exec_remote",
			pattern:          regexp.MustCompile(`(?i)\bpython[23]?\s+-c\s+['"].*urllib.*exec\s*\(`),
			category:         ThreatCategoryMalwareDeployment,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1059.006",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Python downloading and executing remote code",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "revshell_bash_tcp",
			pattern:          regexp.MustCompile(`(?i)\bbash\s+-i\s+>&\s*/dev/tcp/`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.99,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Bash reverse shell to remote host",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_nc_exec",
			pattern:          regexp.MustCompile(`(?i)\bnc\s+.*-e\s+(/bin/)?(ba)?sh`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Netcat reverse shell",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_ncat_exec",
			pattern:          regexp.MustCompile(`(?i)\bncat\s+.*(-e|--exec)\s+(/bin/)?(ba)?sh`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Ncat reverse shell",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_python",
			pattern:          regexp.MustCompile(`(?i)\bpython[23]?\s+-c\s+['"]import\s+(socket|pty)`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.006",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Python reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_perl",
			pattern:          regexp.MustCompile(`(?i)\bperl\s+-e\s+['"]use\s+Socket`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.006",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Perl reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_ruby",
			pattern:          regexp.MustCompile(`(?i)\bruby\s+-rsocket\s+-e`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.006",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Ruby reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_php",
			pattern:          regexp.MustCompile(`(?i)\bphp\s+-r\s+['"].*fsockopen`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.006",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: PHP reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_socat",
			pattern:          regexp.MustCompile(`(?i)\bsocat\s+.*exec:.*tcp:`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Socat reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_mkfifo",
			pattern:          regexp.MustCompile(`(?i)\bmkfifo\s+.*[;&].*\bnc\b`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Named pipe reverse shell pattern",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "revshell_telnet",
			pattern:          regexp.MustCompile(`(?i)\btelnet\s+.*\|\s*/bin/(ba)?sh\s+\|`),
			category:         ThreatCategoryReverseShell,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1059.004",
			mitreTactic:      "Execution",
			recommendation:   "BLOCK: Telnet reverse shell pattern",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "privesc_suid_binary",
			pattern:          regexp.MustCompile(`(?i)\bchmod\s+[0-7]*4[0-7]{3}\s+|\bchmod\s+u\+s\s+`),
			category:         ThreatCategoryPrivilegeEsc,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1548.001",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Setting SUID bit on binary",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "privesc_sgid_binary",
			pattern:          regexp.MustCompile(`(?i)\bchmod\s+[0-7]*2[0-7]{3}\s+|\bchmod\s+g\+s\s+`),
			category:         ThreatCategoryPrivilegeEsc,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1548.001",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Setting SGID bit on binary",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "privesc_setcap",
			pattern:          regexp.MustCompile(`(?i)\bsetcap\s+.*cap_(setuid|setgid|net_admin|sys_admin|dac_override)`),
			category:         ThreatCategoryPrivilegeEsc,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1548.001",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Setting dangerous capabilities on binary",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "cred_dump_shadow",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+/etc/shadow\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1003.008",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read password hashes",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_dump_aws",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+.*\.aws/(credentials|config)\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1552.001",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read AWS credentials file",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_dump_ssh_private",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+.*\.ssh/(id_rsa|id_ed25519|id_ecdsa|id_dsa)(\s|$)`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1552.004",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read SSH private key",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_copy_shadow",
			pattern:          regexp.MustCompile(`(?i)\bcp\s+/etc/shadow\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1003.008",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to copy password hashes",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_dump_gcp",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+.*\.config/gcloud/.*\.json\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1552.001",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read GCP credentials",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_dump_azure",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+.*\.azure/.*\.json\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1552.001",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read Azure credentials",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_dump_kube",
			pattern:          regexp.MustCompile(`(?i)\bcat\s+.*\.kube/config\b`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1552.001",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Attempted to read Kubernetes credentials",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "persist_crontab_remote",
			pattern:          regexp.MustCompile(`(?i)\bcrontab.*\|\s*(curl|wget)\b`),
			category:         ThreatCategoryPersistence,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1053.003",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Installing cron job from remote source",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "persist_at_job",
			pattern:          regexp.MustCompile(`(?i)\bat\s+.*<<<.*\b(curl|wget|nc|bash)\b`),
			category:         ThreatCategoryPersistence,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1053.002",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Scheduling suspicious at job",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "exfil_dns_tunnel",
			pattern:          regexp.MustCompile(`(?i)\b(dig|nslookup|host)\s+.*\$\(`),
			category:         ThreatCategoryExfiltration,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1048.001",
			mitreTactic:      "Exfiltration",
			recommendation:   "BLOCK: DNS tunneling pattern detected",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "exfil_icmp_tunnel",
			pattern:          regexp.MustCompile(`(?i)\bping\s+.*-p\s+[0-9a-f]{16,}`),
			category:         ThreatCategoryExfiltration,
			severity:         ThreatSeverityHigh,
			confidence:       0.80,
			mitreAttack:      "T1048.003",
			mitreTactic:      "Exfiltration",
			recommendation:   "BLOCK: ICMP tunneling pattern detected",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "evasion_clear_logs",
			pattern:          regexp.MustCompile(`(?i)(\brm\s+(-rf?\s+)?|truncate\s+(-s\s*0\s+)?|shred\s+|>\s*)/var/log/`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1070.002",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to clear system logs",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_clear_history",
			pattern:          regexp.MustCompile(`(?i)\bhistory\s+-c\b|\brm\s+.*\.(bash_history|zsh_history)\b|\bunset\s+HISTFILE\b|\bHISTSIZE=0\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityHigh,
			confidence:       0.90,
			mitreAttack:      "T1070.003",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to clear command history",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_disable_logging",
			pattern:          regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+(rsyslog|syslog-ng|journald)`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable system logging",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "network_arp_spoof",
			pattern:          regexp.MustCompile(`(?i)\b(arpspoof|ettercap|bettercap)\b`),
			category:         ThreatCategoryNetworkManipulation,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1557.002",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: ARP spoofing tool detected",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "network_dns_spoof",
			pattern:          regexp.MustCompile(`(?i)\b(dnsspoof|dnschef)\b`),
			category:         ThreatCategoryNetworkManipulation,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1557.001",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: DNS spoofing tool detected",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "miner_install",
			pattern:          regexp.MustCompile(`(?i)\b(wget|curl)\b.*\b(xmrig|xmr-stak|cpuminer|minerd|cgminer|bfgminer)\b`),
			category:         ThreatCategoryCryptominer,
			severity:         ThreatSeverityCritical,
			confidence:       0.98,
			mitreAttack:      "T1496",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Downloading cryptocurrency miner",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "miner_stratum_connect",
			pattern:          regexp.MustCompile(`(?i)stratum\+tcp://`),
			category:         ThreatCategoryCryptominer,
			severity:         ThreatSeverityCritical,
			confidence:       0.99,
			mitreAttack:      "T1496",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Mining pool connection string",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "miner_pool_domain",
			pattern:          regexp.MustCompile(`(?i)\b(pool\.(minergate|supportxmr|hashvault)|nanopool|f2pool|antpool|ethermine|flypool)\b`),
			category:         ThreatCategoryCryptominer,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1496",
			mitreTactic:      "Impact",
			recommendation:   "BLOCK: Known mining pool domain",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "container_escape_privileged",
			pattern:          regexp.MustCompile(`(?i)\bdocker\s+run\s+.*--privileged`),
			category:         ThreatCategoryResourceHijacking,
			severity:         ThreatSeverityHigh,
			confidence:       0.80,
			mitreAttack:      "T1611",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "Review: Running privileged container",
			blockRecommended: false,
		},
		&InputThreatDetector{
			name:             "container_mount_host",
			pattern:          regexp.MustCompile(`(?i)\bdocker\s+run\s+.*-v\s+/:/`),
			category:         ThreatCategoryResourceHijacking,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1611",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Mounting host root filesystem in container",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "container_mount_docker_sock",
			pattern:          regexp.MustCompile(`(?i)\bdocker\s+run\s+.*-v\s+/var/run/docker\.sock`),
			category:         ThreatCategoryResourceHijacking,
			severity:         ThreatSeverityCritical,
			confidence:       0.90,
			mitreAttack:      "T1611",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Mounting Docker socket in container",
			blockRecommended: true,
		},

		&InputThreatDetector{
			name:             "kernel_module_load",
			pattern:          regexp.MustCompile(`(?i)\b(insmod|modprobe)\s+`),
			category:         ThreatCategorySystemTampering,
			severity:         ThreatSeverityCritical,
			confidence:       0.85,
			mitreAttack:      "T1547.006",
			mitreTactic:      "Persistence",
			recommendation:   "BLOCK: Loading kernel module",
			blockRecommended: true,
		},
	}
}

// AnalyzeCommand analyzes a command before execution and returns threat analysis.
func (s *Sentinel) AnalyzeCommand(command string) *CommandAnalysisResult {
	result := &CommandAnalysisResult{
		Command:     s.ScrubText(command),
		Safe:        true,
		ThreatLevel: ThreatLevelNone,
		RiskScore:   0,
	}

	if !s.config.ThreatDetectionEnabled {
		return result
	}

	var signals []ThreatSignal
	for _, detector := range s.inputThreatDetectors {
		detected := detector.Detect(command)
		signals = append(signals, detected...)
	}

	result.ThreatSignals = signals
	result.ThreatLevel = s.aggregateThreatLevel(signals)
	result.RiskScore = s.calculateRiskScore(signals)

	for _, sig := range signals {
		if sig.BlockRecommended {
			result.Safe = false
			result.BlockReason = sig.Recommendation
			break
		}
	}

	if result.Safe && (result.ThreatLevel == ThreatLevelHigh || result.ThreatLevel == ThreatLevelElevated) {
		result.RequiresApproval = true
	}

	if len(signals) > 0 {
		s.logger.Warn("INPUT threat analysis: threats detected in command",
			"threat_level", result.ThreatLevel,
			"threat_count", len(signals),
			"safe", result.Safe,
			"risk_score", result.RiskScore,
			"command_scrubbed", result.Command)
	}

	return result
}

// AnalyzeFileEdit analyzes a file operation before it happens.
func (s *Sentinel) AnalyzeFileEdit(filePath string, operation string, content string) *FileEditAnalysisResult {
	result := &FileEditAnalysisResult{
		FilePath:             s.ScrubText(filePath),
		Operation:            operation,
		Safe:                 true,
		ThreatLevel:          ThreatLevelNone,
		RiskScore:            0,
		IsCriticalSystemFile: s.isCriticalSystemFile(filePath),
	}

	if !s.config.ThreatDetectionEnabled {
		return result
	}

	var signals []ThreatSignal

	if result.IsCriticalSystemFile {
		signals = append(signals, ThreatSignal{
			Category:         ThreatCategorySystemTampering,
			Severity:         ThreatSeverityHigh,
			Indicator:        "critical_system_file",
			Confidence:       0.90,
			MitreAttack:      "T1565.001",
			MitreTactic:      "Impact",
			Recommendation:   "File is a critical system file - requires approval",
			BlockRecommended: false,
		})
	}

	if content != "" {
		contentSignals := s.analyzeFileContent(content, filePath)
		signals = append(signals, contentSignals...)
	}

	pathSignals := s.analyzeFilePath(filePath, operation)
	signals = append(signals, pathSignals...)

	result.ThreatSignals = signals
	result.ThreatLevel = s.aggregateThreatLevel(signals)
	result.RiskScore = s.calculateRiskScore(signals)

	for _, sig := range signals {
		if sig.BlockRecommended {
			result.Safe = false
			result.BlockReason = sig.Recommendation
			break
		}
	}

	if result.Safe && result.IsCriticalSystemFile {
		result.RequiresApproval = true
	}

	if len(signals) > 0 {
		s.logger.Warn("INPUT threat analysis: threats detected in file operation",
			"threat_level", result.ThreatLevel,
			"threat_count", len(signals),
			"safe", result.Safe,
			"file_path_scrubbed", result.FilePath,
			"operation", operation,
			"is_critical", result.IsCriticalSystemFile)
	}

	return result
}

func (s *Sentinel) isCriticalSystemFile(filePath string) bool {
	normalizedPath := strings.ToLower(filePath)

	for _, criticalPath := range CriticalSystemPaths {
		if strings.HasPrefix(normalizedPath, strings.ToLower(criticalPath)) {
			return true
		}
	}

	for _, criticalDir := range CriticalSystemDirs {
		if strings.HasPrefix(normalizedPath, strings.ToLower(criticalDir)+"/") {
			return true
		}
		if normalizedPath == strings.ToLower(criticalDir) {
			return true
		}
	}

	return false
}

func (s *Sentinel) analyzeFileContent(content string, filePath string) []ThreatSignal {
	var signals []ThreatSignal

	if strings.Contains(content, "#!/bin/") || strings.Contains(content, "#!/usr/bin/env") {
		for _, detector := range s.inputThreatDetectors {
			detected := detector.Detect(content)
			signals = append(signals, detected...)
		}
	}

	if strings.Contains(filePath, "authorized_keys") && strings.Contains(content, "ssh-") {
		signals = append(signals, ThreatSignal{
			Category:         ThreatCategoryPersistence,
			Severity:         ThreatSeverityHigh,
			Indicator:        "ssh_key_injection",
			Confidence:       0.85,
			MitreAttack:      "T1098.004",
			MitreTactic:      "Persistence",
			Recommendation:   "Review: Adding SSH authorized key",
			BlockRecommended: false,
		})
	}

	if strings.Contains(filePath, "cron") {
		if strings.Contains(content, "curl") || strings.Contains(content, "wget") {
			signals = append(signals, ThreatSignal{
				Category:         ThreatCategoryPersistence,
				Severity:         ThreatSeverityHigh,
				Indicator:        "cron_remote_download",
				Confidence:       0.85,
				MitreAttack:      "T1053.003",
				MitreTactic:      "Persistence",
				Recommendation:   "Review: Cron job with remote download",
				BlockRecommended: false,
			})
		}
	}

	if strings.Contains(filePath, "systemd") && strings.HasSuffix(filePath, ".service") {
		signals = append(signals, ThreatSignal{
			Category:         ThreatCategoryPersistence,
			Severity:         ThreatSeverityMedium,
			Indicator:        "systemd_service_creation",
			Confidence:       0.75,
			MitreAttack:      "T1543.002",
			MitreTactic:      "Persistence",
			Recommendation:   "Review: Creating systemd service",
			BlockRecommended: false,
		})
	}

	return signals
}

func (s *Sentinel) analyzeFilePath(filePath string, operation string) []ThreatSignal {
	var signals []ThreatSignal
	normalizedPath := strings.ToLower(filePath)

	if operation != "read" {
		if normalizedPath == "/etc/passwd" || normalizedPath == "/etc/shadow" ||
			normalizedPath == "/etc/group" || normalizedPath == "/etc/gshadow" {
			signals = append(signals, ThreatSignal{
				Category:         ThreatCategorySystemTampering,
				Severity:         ThreatSeverityCritical,
				Indicator:        "auth.file.modification",
				Confidence:       0.98,
				MitreAttack:      "T1136.001",
				MitreTactic:      "Persistence",
				Recommendation:   "BLOCK: Direct modification of authentication file",
				BlockRecommended: true,
			})
		}

		if strings.Contains(normalizedPath, "sudoers") {
			signals = append(signals, ThreatSignal{
				Category:         ThreatCategoryPrivilegeEsc,
				Severity:         ThreatSeverityCritical,
				Indicator:        "sudoers_modification",
				Confidence:       0.98,
				MitreAttack:      "T1548.003",
				MitreTactic:      "Privilege Escalation",
				Recommendation:   "BLOCK: Modification of sudoers file",
				BlockRecommended: true,
			})
		}

		if normalizedPath == "/etc/ld.so.preload" {
			signals = append(signals, ThreatSignal{
				Category:         ThreatCategoryPersistence,
				Severity:         ThreatSeverityCritical,
				Indicator:        "ld_preload_modification",
				Confidence:       0.98,
				MitreAttack:      "T1574.006",
				MitreTactic:      "Persistence",
				Recommendation:   "BLOCK: Modification of ld.so.preload",
				BlockRecommended: true,
			})
		}
	}

	if operation == "delete" && strings.HasPrefix(normalizedPath, "/var/log/") {
		signals = append(signals, ThreatSignal{
			Category:         ThreatCategoryDefenseEvasion,
			Severity:         ThreatSeverityCritical,
			Indicator:        "log_file_deletion",
			Confidence:       0.95,
			MitreAttack:      "T1070.002",
			MitreTactic:      "Defense Evasion",
			Recommendation:   "BLOCK: Deletion of log file",
			BlockRecommended: true,
		})
	}

	return signals
}

func (s *Sentinel) calculateRiskScore(signals []ThreatSignal) int {
	if len(signals) == 0 {
		return 0
	}

	score := 0.0
	for _, sig := range signals {
		var severityScore float64
		switch sig.Severity {
		case ThreatSeverityCritical:
			severityScore = 40
		case ThreatSeverityHigh:
			severityScore = 25
		case ThreatSeverityMedium:
			severityScore = 15
		case ThreatSeverityLow:
			severityScore = 8
		case ThreatSeverityInfo:
			severityScore = 3
		}

		score += severityScore * sig.Confidence
	}

	if score > 100 {
		score = 100
	}

	return int(score)
}
