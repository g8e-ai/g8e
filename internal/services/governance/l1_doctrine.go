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

package governance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// L1Doctrine provides L1 (Technical Bedrock) validation.
// L1 is the foundational hard gate that enforces forbidden patterns,
// blacklist/whitelist rules, and intent validation.
// It implements doctrine validation using protobuf field options.
// It checks the (g8e.common.v1).forbidden_patterns extension on string fields.
// It also performs MITRE-based threat detection on command payloads.
type L1Doctrine struct {
	inputThreatDetectors []ThreatDetector
}

// NewL1Doctrine creates a new protobuf-based doctrine validator.
func NewL1Doctrine() *L1Doctrine {
	d := &L1Doctrine{}
	d.initializeInputThreatDetectors()
	return d
}

// ValidatePayload checks a typed protobuf payload for forbidden pattern violations.
// It performs two types of L1 validation:
// 1. Protobuf field option validation (forbidden_patterns extension)
// 2. MITRE-based threat detection on command and MCP argument payloads
func (v *L1Doctrine) ValidatePayload(msg proto.Message) []string {
	var violations []string

	// Phase 1: Protobuf field option validation (forbidden_patterns extension)
	md := msg.ProtoReflect().Descriptor()
	fields := md.Fields()

	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		opts := fd.Options()
		if opts == nil || !proto.HasExtension(opts, commonv1.E_ForbiddenPatterns) {
			continue
		}
		patternsStr, ok := proto.GetExtension(opts, commonv1.E_ForbiddenPatterns).(string)
		if !ok || patternsStr == "" {
			continue
		}
		val := msg.ProtoReflect().Get(fd)
		if fd.Kind() != protoreflect.StringKind {
			continue
		}
		strVal := val.String()
		for _, p := range strings.Split(patternsStr, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			matched, err := regexp.MatchString(p, strVal)
			if err == nil && matched {
				violations = append(violations, fmt.Sprintf("field %s violates pattern %s", fd.Name(), p))
			}
		}
	}

	// Phase 2: MITRE-based threat detection on specific payload types
	switch payload := msg.(type) {
	case *operatorv1.CommandRequested:
		// Analyze the command string for L1 threat patterns
		signals := v.AnalyzeCommand(payload.Command)
		for _, sig := range signals {
			if sig.BlockRecommended {
				violations = append(violations, fmt.Sprintf("command threat detected: %s (category: %s, MITRE: %s)", sig.Indicator, sig.Category, sig.MitreAttack))
			}
		}

	case *operatorv1.McpCallRequested:
		// Analyze MCP tool arguments for L1 threat patterns
		signals, err := v.AnalyzeMCPArguments(payload.ArgumentsJson)
		if err != nil {
			violations = append(violations, fmt.Sprintf("failed to analyze MCP arguments: %v", err))
		} else {
			for _, sig := range signals {
				if sig.BlockRecommended {
					violations = append(violations, fmt.Sprintf("MCP argument threat detected: %s (category: %s, MITRE: %s, path: %s)", sig.Indicator, sig.Category, sig.MitreAttack, sig.Context))
				}
			}
		}

	case *operatorv1.A2ACallRequested:
		// Analyze A2A skill payload for L1 threat patterns
		signals, err := v.AnalyzeMCPArguments(payload.PayloadJson)
		if err != nil {
			violations = append(violations, fmt.Sprintf("failed to analyze A2A payload: %v", err))
		} else {
			for _, sig := range signals {
				if sig.BlockRecommended {
					violations = append(violations, fmt.Sprintf("A2A payload threat detected: %s (category: %s, MITRE: %s, path: %s)", sig.Indicator, sig.Category, sig.MitreAttack, sig.Context))
				}
			}
		}

	case *operatorv1.FileEditRequested:
		// Analyze file edit content for L1 threat patterns
		// Check the file path for critical system files
		if v.isCriticalSystemFile(payload.FilePath) {
			violations = append(violations, fmt.Sprintf("attempted modification of critical system file: %s", payload.FilePath))
		}
		// Analyze content for threat patterns
		if payload.Content != "" {
			signals := v.AnalyzeCommand(payload.Content)
			for _, sig := range signals {
				if sig.BlockRecommended {
					violations = append(violations, fmt.Sprintf("file content threat detected: %s (category: %s)", sig.Indicator, sig.Category))
				}
			}
		}
	}

	return violations
}

// ValidateIntent is a placeholder for intent-based doctrine validation.
// This will be integrated with Sentinel's intent allowlist in a follow-up.
func (v *L1Doctrine) ValidateIntent(intent constants.CloudIntent) bool {
	// TODO: Integrate with Sentinel's intent validation
	// For now, allow all intents - this is a temporary bridge
	return true
}

// ThreatSeverity represents the severity level of a detected threat
type ThreatSeverity string

const (
	ThreatSeverityCritical ThreatSeverity = "critical"
	ThreatSeverityHigh     ThreatSeverity = "high"
	ThreatSeverityMedium   ThreatSeverity = "medium"
	ThreatSeverityLow      ThreatSeverity = "low"
	ThreatSeverityInfo     ThreatSeverity = "info"
)

// ThreatLevel represents the aggregated threat level for a result
type ThreatLevel string

const (
	ThreatLevelNone     ThreatLevel = "none"
	ThreatLevelLow      ThreatLevel = "low"
	ThreatLevelElevated ThreatLevel = "elevated"
	ThreatLevelHigh     ThreatLevel = "high"
	ThreatLevelCritical ThreatLevel = "critical"
)

// ThreatCategory represents the type of threat detected
type ThreatCategory string

const (
	ThreatCategoryReverseShell ThreatCategory = "reverse_shell"
	ThreatCategoryPrivilegeEsc ThreatCategory = "privilege_escalation"
	// #nosec G101 -- this is a threat category name, not a credential
	ThreatCategoryCredentialAccess    ThreatCategory = "credential_access"
	ThreatCategoryExfiltration        ThreatCategory = "data_exfiltration"
	ThreatCategoryCryptominer         ThreatCategory = "cryptominer"
	ThreatCategoryPersistence         ThreatCategory = "persistence"
	ThreatCategoryLateralMovement     ThreatCategory = "lateral_movement"
	ThreatCategoryDefenseEvasion      ThreatCategory = "defense_evasion"
	ThreatCategoryReconnaissance      ThreatCategory = "reconnaissance"
	ThreatCategoryResourceHijacking   ThreatCategory = "resource_hijacking"
	ThreatCategoryDestructive         ThreatCategory = "destructive"
	ThreatCategorySystemTampering     ThreatCategory = "system_tampering"
	ThreatCategorySecurityBypass      ThreatCategory = "security_bypass"
	ThreatCategoryMalwareDeployment   ThreatCategory = "malware_deployment"
	ThreatCategoryDataDestruction     ThreatCategory = "data_destruction"
	ThreatCategoryNetworkManipulation ThreatCategory = "network_manipulation"
)

// ThreatSignal represents a detected threat indicator
type ThreatSignal struct {
	Category         ThreatCategory `json:"category"`
	Severity         ThreatSeverity `json:"severity"`
	Indicator        string         `json:"indicator"`
	Context          string         `json:"context,omitempty"`
	Confidence       float64        `json:"confidence"`
	MitreAttack      string         `json:"mitre_attack"`
	MitreTactic      string         `json:"mitre_tactic"`
	Recommendation   string         `json:"recommendation,omitempty"`
	BlockRecommended bool           `json:"block_recommended"`
	Source           string         `json:"source,omitempty"`
}

// ThreatDetector is the interface for threat detection
type ThreatDetector interface {
	Name() string
	Detect(input string) []ThreatSignal
}

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
	constants.PathEtcPasswd,
	constants.PathEtcShadow,
	constants.PathEtcGroup,
	constants.PathEtcGshadow,
	constants.PathEtcSudoers,
	constants.PathEtcSudoersD,
	constants.PathEtcSshSshdConfig,
	constants.PathEtcSshSshConfig,
	constants.PathEtcPamD,
	constants.PathEtcSecurity,
	constants.PathEtcLdSoConf,
	constants.PathEtcLdSoPreload,
	constants.PathEtcHosts,
	constants.PathEtcResolvConf,
	constants.PathEtcFstab,
	constants.PathEtcCrontab,
	constants.PathEtcCronD,
	constants.PathEtcCronDaily,
	constants.PathEtcCronHourly,
	constants.PathEtcInitD,
	constants.PathEtcSystemdSystem,
	constants.PathEtcRcLocal,
	constants.PathEtcProfile,
	constants.PathEtcProfileD,
	constants.PathEtcBashBashrc,
	constants.PathEtcEnvironment,
	constants.PathEtcSelinux,
	constants.PathEtcApparmor,
	constants.PathEtcApparmorD,
	constants.PathBoot,
	constants.PathRootSsh,
	constants.PathRootBashrc,
	constants.PathRootBashProfile,
	constants.PathRootProfile,
}

// CriticalSystemDirs lists directories where any modification is high risk
var CriticalSystemDirs = []string{
	constants.PathBin,
	constants.PathSbin,
	constants.PathUsrBin,
	constants.PathUsrSbin,
	constants.PathUsrLocalBin,
	constants.PathUsrLocalSbin,
	constants.PathLib,
	constants.PathLib64,
	constants.PathUsrLib,
	constants.PathBoot,
	constants.PathProc,
	constants.PathSys,
	constants.PathDev,
}

// isCriticalSystemFile checks if a path is a critical system file or directory
func (v *L1Doctrine) isCriticalSystemFile(path string) bool {
	// Check exact matches in CriticalSystemPaths
	for _, criticalPath := range CriticalSystemPaths {
		if path == criticalPath {
			return true
		}
		// Check if path is within a critical directory (directories end with /)
		if strings.HasSuffix(criticalPath, "/") && strings.HasPrefix(path, criticalPath) {
			return true
		}
	}

	// Check if path is within a critical directory
	for _, criticalDir := range CriticalSystemDirs {
		if strings.HasPrefix(path, criticalDir) {
			// Ensure it's not just a prefix match (e.g., /bin should match /bin/ls but not /binaries)
			if len(path) > len(criticalDir) && path[len(criticalDir)] == '/' {
				return true
			}
			if path == criticalDir {
				return true
			}
		}
	}

	return false
}

func (v *L1Doctrine) initializeInputThreatDetectors() {
	v.inputThreatDetectors = []ThreatDetector{
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
			name:             "evasion_disable_auditd",
			pattern:          regexp.MustCompile(`(?i)\bauditctl\s+-e\s+0\b|\bsystemctl\s+(stop|disable)\s+auditd\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.006",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable audit daemon",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_flush_iptables",
			pattern:          regexp.MustCompile(`(?i)\biptables\s+-F\b|\biptables\s+--flush\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.007",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to flush firewall rules",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_disable_firewalld",
			pattern:          regexp.MustCompile(`(?i)\bsystemctl\s+(stop|disable)\s+firewalld\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.007",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable firewalld",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_disable_ufw",
			pattern:          regexp.MustCompile(`(?i)\bufw\s+disable\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.007",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable UFW firewall",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_disable_apparmor",
			pattern:          regexp.MustCompile(`(?i)\baa-disable\b|\bsystemctl\s+(stop|disable)\s+apparmor\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable AppArmor",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "evasion_disable_selinux_sed",
			pattern:          regexp.MustCompile(`(?i)\bsed\s+-i.*SELINUX=disabled\b`),
			category:         ThreatCategoryDefenseEvasion,
			severity:         ThreatSeverityCritical,
			confidence:       0.95,
			mitreAttack:      "T1562.001",
			mitreTactic:      "Defense Evasion",
			recommendation:   "BLOCK: Attempting to disable SELinux via sed",
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

// AnalyzeCommand analyzes a command string for L1 threat patterns.
// This is the core L1 threat detection function that replaces Sentinel's AnalyzeCommand.
func (v *L1Doctrine) AnalyzeCommand(command string) []ThreatSignal {
	var signals []ThreatSignal
	for _, detector := range v.inputThreatDetectors {
		detected := detector.Detect(command)
		signals = append(signals, detected...)
	}
	return signals
}

// AnalyzeMCPArguments recursively analyzes MCP tool arguments for L1 forbidden patterns.
// This extends L1 validation beyond tool_name to include the arguments_json payload.
func (v *L1Doctrine) AnalyzeMCPArguments(argumentsJSON string) ([]ThreatSignal, error) {
	// Parse the JSON arguments
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid JSON arguments: %w", err)
	}

	// Recursively analyze all string values in the arguments with depth limit
	const maxDepth = 50
	signals := []ThreatSignal{}
	if err := v.analyzeValueRecursive(args, "", &signals, 0, maxDepth); err != nil {
		return nil, err
	}

	return signals, nil
}

// MaxDepthExceededError is returned when JSON recursion exceeds the safety limit
type MaxDepthExceededError struct {
	MaxDepth int
	Path     string
}

func (e *MaxDepthExceededError) Error() string {
	return fmt.Sprintf("JSON recursion depth exceeded maximum limit of %d at path: %s", e.MaxDepth, e.Path)
}

// analyzeValueRecursive recursively traverses a value and detects threats in string fields.
// Returns MaxDepthExceededError if the recursion depth exceeds maxDepth.
func (v *L1Doctrine) analyzeValueRecursive(value interface{}, path string, signals *[]ThreatSignal, currentDepth int, maxDepth int) error {
	if currentDepth > maxDepth {
		return &MaxDepthExceededError{MaxDepth: maxDepth, Path: path}
	}

	switch val := value.(type) {
	case string:
		// Analyze string values for threat patterns
		for _, detector := range v.inputThreatDetectors {
			detected := detector.Detect(val)
			for _, sig := range detected {
				// Add path context to the signal
				sig.Context = path
				*signals = append(*signals, sig)
			}
		}
	case map[string]interface{}:
		// Recursively analyze object fields
		for key, val := range val {
			newPath := path
			if newPath != "" {
				newPath += "."
			}
			newPath += key
			if err := v.analyzeValueRecursive(val, newPath, signals, currentDepth+1, maxDepth); err != nil {
				return err
			}
		}
	case []interface{}:
		// Recursively analyze array elements
		for i, val := range val {
			newPath := path
			if newPath != "" {
				newPath += "["
				newPath += fmt.Sprintf("%d", i)
				newPath += "]"
			}
			if err := v.analyzeValueRecursive(val, newPath, signals, currentDepth+1, maxDepth); err != nil {
				return err
			}
		}
		// Numbers, booleans, and null are ignored
	}
	return nil
}

// aggregateThreatLevel determines the overall threat level from signals
func (v *L1Doctrine) aggregateThreatLevel(signals []ThreatSignal) ThreatLevel {
	if len(signals) == 0 {
		return ThreatLevelNone
	}

	hasCritical := false
	hasHigh := false
	hasMedium := false

	for _, sig := range signals {
		switch sig.Severity {
		case ThreatSeverityCritical:
			hasCritical = true
		case ThreatSeverityHigh:
			hasHigh = true
		case ThreatSeverityMedium:
			hasMedium = true
		}
	}

	if hasCritical {
		return ThreatLevelCritical
	}
	if hasHigh {
		return ThreatLevelHigh
	}
	if hasMedium || len(signals) >= 3 {
		return ThreatLevelElevated
	}
	return ThreatLevelLow
}

// calculateRiskScore computes a 0-100 risk score from threat signals
func (v *L1Doctrine) calculateRiskScore(signals []ThreatSignal) int {
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
