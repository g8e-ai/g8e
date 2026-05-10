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
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// Sentinel is a zero-trust security system that:
//  1. DEFENDS THE HOST - Analyzes AI commands/actions BEFORE execution to block threats
//  2. PROTECTS DATA - Scrubs sensitive data before cloud transmission
//  3. DETECTS THREATS - Identifies malicious activity patterns with MITRE ATT&CK mapping
//
// PRE-EXECUTION ANALYSIS (Defense):
// Sentinel analyzes every command, file edit, and action the AI attempts BEFORE execution.
// This protects the host system from malicious AI, bad actors, bugs, and prompt injection.
// Commands that match threat patterns are flagged/blocked before they can cause damage.
//
// POST-EXECUTION SCRUBBING (Data Sovereignty):
// Sentinel scrubs ONLY actual sensitive data:
//   - Credentials (API keys, tokens, passwords, private keys)
//   - PII (emails, credit cards, SSNs, phone numbers, IBANs)
//   - Connection strings with embedded credentials
//   - Bearer tokens and password config patterns
//
// Sentinel does NOT scrub system/operational data the AI needs for troubleshooting:
//   - IP addresses, hostnames, MAC addresses
//   - File paths (including home directories)
//   - URLs (unless they contain embedded credentials)
//   - UUIDs, AWS ARNs, AWS account IDs
//   - Filenames, hashes, base64 content
type Sentinel struct {
	config               *SentinelConfig
	logger               *slog.Logger
	scrubbers            []Scrubber
	threatDetectors      []ThreatDetector
	inputThreatDetectors []ThreatDetector
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
	ThreatCategoryReverseShell        ThreatCategory = "reverse_shell"
	ThreatCategoryPrivilegeEsc        ThreatCategory = "privilege_escalation"
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
	// Category is the type of threat (reverse_shell, privesc, exfiltration, etc.)
	Category ThreatCategory `json:"category"`

	// Severity indicates how critical this threat is
	Severity ThreatSeverity `json:"severity"`

	// Indicator is the pattern name that triggered detection
	Indicator string `json:"indicator"`

	// Context provides scrubbed context about the detection (safe to transmit)
	Context string `json:"context,omitempty"`

	// Confidence is a 0.0-1.0 score indicating certainty of the threat detection
	Confidence float64 `json:"confidence"`

	// MitreAttack is the MITRE ATT&CK technique ID (e.g., T1059.004)
	MitreAttack string `json:"mitre_attack"`

	// MitreTactic is the MITRE ATT&CK tactic (e.g., Execution, Persistence)
	MitreTactic string `json:"mitre_tactic"`

	// Recommendation is a brief action suggestion
	Recommendation string `json:"recommendation,omitempty"`

	// BlockRecommended indicates if execution should be blocked
	BlockRecommended bool `json:"block_recommended"`
}

// CommandAnalysisResult is the result of analyzing a command BEFORE execution
// This is the pre-execution defense that protects the host from the AI
type CommandAnalysisResult struct {
	// Command is the command that was analyzed (scrubbed for logging)
	Command string `json:"command"`

	// Safe indicates if the command is safe to execute
	Safe bool `json:"safe"`

	// ThreatLevel is the aggregated threat level
	ThreatLevel ThreatLevel `json:"threat_level"`

	// ThreatSignals contains all detected threat indicators
	ThreatSignals []ThreatSignal `json:"threat_signals,omitempty"`

	// BlockReason explains why execution should be blocked (if not safe)
	BlockReason string `json:"block_reason,omitempty"`

	// RequiresApproval indicates if human approval is needed
	RequiresApproval bool `json:"requires_approval"`

	// RiskScore is a 0-100 score indicating overall risk
	RiskScore int `json:"risk_score"`
}

// FileEditAnalysisResult is the result of analyzing a file edit BEFORE it happens
type FileEditAnalysisResult struct {
	// FilePath is the target file (scrubbed)
	FilePath string `json:"file_path"`

	// Operation is the type of operation (create, modify, delete, chmod, etc.)
	Operation string `json:"operation"`

	// Safe indicates if the operation is safe to execute
	Safe bool `json:"safe"`

	// ThreatLevel is the aggregated threat level
	ThreatLevel ThreatLevel `json:"threat_level"`

	// ThreatSignals contains all detected threat indicators
	ThreatSignals []ThreatSignal `json:"threat_signals,omitempty"`

	// BlockReason explains why operation should be blocked (if not safe)
	BlockReason string `json:"block_reason,omitempty"`

	// RequiresApproval indicates if human approval is needed
	RequiresApproval bool `json:"requires_approval"`

	// RiskScore is a 0-100 score indicating overall risk
	RiskScore int `json:"risk_score"`

	// IsCriticalSystemFile indicates if target is a critical system file
	IsCriticalSystemFile bool `json:"is_critical_system_file"`
}

// ThreatDetector defines an interface for threat detection rules
type ThreatDetector interface {
	// Name returns the detector identifier
	Name() string
	// Detect analyzes input and returns any threat signals found
	Detect(input string) []ThreatSignal
}

// RegexThreatDetector detects threats using regex patterns
type RegexThreatDetector struct {
	name           string
	pattern        *regexp.Regexp
	category       ThreatCategory
	severity       ThreatSeverity
	confidence     float64
	mitreAttack    string
	mitreTactic    string
	recommendation string
}

func (r *RegexThreatDetector) Name() string { return r.name }

func (r *RegexThreatDetector) Detect(input string) []ThreatSignal {
	if r.pattern.MatchString(input) {
		return []ThreatSignal{{
			Category:       r.category,
			Severity:       r.severity,
			Indicator:      r.name,
			Confidence:     r.confidence,
			MitreAttack:    r.mitreAttack,
			MitreTactic:    r.mitreTactic,
			Recommendation: r.recommendation,
		}}
	}
	return nil
}

// SentinelConfig holds configuration for the Sentinel data scrubber and threat detector
type SentinelConfig struct {
	// Enabled controls whether Sentinel scrubbing is active
	Enabled bool

	// StrictMode when true, aggressively scrubs anything that looks like data
	StrictMode bool

	// ThreatDetectionEnabled controls whether threat detection is active
	ThreatDetectionEnabled bool

	// MaxOutputLength limits the scrubbed output length (0 = no limit)
	MaxOutputLength int

	// AllowedPatterns are regex patterns that should pass through unscrubbed
	AllowedPatterns []string

	// CustomScrubPatterns are additional patterns to scrub
	CustomScrubPatterns map[string]string
}

// DefaultSentinelConfig returns sensible defaults for production use
func DefaultSentinelConfig() *SentinelConfig {
	return &SentinelConfig{
		Enabled:                true,
		StrictMode:             true,
		ThreatDetectionEnabled: true,
		MaxOutputLength:        4096,
		AllowedPatterns:        []string{},
		CustomScrubPatterns:    map[string]string{},
	}
}

// Scrubber defines an interface for data scrubbing rules
type Scrubber interface {
	// Name returns the scrubber identifier for logging
	Name() string
	// Scrub processes text and returns scrubbed version
	Scrub(input string) string
}

// RegexScrubber scrubs text matching a regex pattern
type RegexScrubber struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
}

func (r *RegexScrubber) Name() string { return r.name }
func (r *RegexScrubber) Scrub(input string) string {
	return r.pattern.ReplaceAllString(input, r.replacement)
}

// CommandResult represents the raw output from command execution
type CommandResult struct {
	Command    string
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
}

// ScrubbedResult is the sanitized output safe for transmission to cloud AI
type ScrubbedResult struct {
	// Status is the high-level outcome: success, failure, error, timeout
	Status string `json:"status"`

	// ExitCode is preserved as it contains no sensitive data
	ExitCode int `json:"exit_code"`

	// Summary is a scrubbed, generalized description of what happened
	Summary string `json:"summary"`

	// RowCount if the output appears to contain tabular data
	RowCount *int `json:"row_count,omitempty"`

	// ErrorType categorizes any error without exposing details
	ErrorType string `json:"error_type,omitempty"`

	// DurationMs is preserved for performance context
	DurationMs int64 `json:"duration_ms"`

	// OutputLines is the count of output lines (not the content)
	OutputLines int `json:"output_lines"`

	// Warnings are scrubbed warning messages
	Warnings []string `json:"warnings,omitempty"`

	// StructureHints provide schema-level info without data
	StructureHints []string `json:"structure_hints,omitempty"`

	// ThreatSignals contains detected threat indicators (g8e Sentinel)
	ThreatSignals []ThreatSignal `json:"threat_signals,omitempty"`

	// ThreatLevel is the aggregated threat level from all signals
	ThreatLevel ThreatLevel `json:"threat_level,omitempty"`

	// ThreatCount is the total number of threats detected
	ThreatCount int `json:"threat_count,omitempty"`
}

// NewSentinel creates a new Sentinel with default scrubbers and threat detectors
func NewSentinel(config *SentinelConfig, logger *slog.Logger) *Sentinel {
	if config == nil {
		config = DefaultSentinelConfig()
	}

	s := &Sentinel{
		config: config,
		logger: logger,
	}

	s.initializeScrubbers()
	s.initializeThreatDetectors()
	s.initializeInputThreatDetectors()
	return s
}

// initializeScrubbers sets up all the pattern-based scrubbers
// IMPORTANT: Order matters! More specific patterns must come before generic ones.
// The scrubbers are applied sequentially, so a generic pattern matching first
// will prevent the specific pattern from ever seeing the text.
func (s *Sentinel) initializeScrubbers() {
	s.scrubbers = []Scrubber{
		// g8e Operator API Key - g8e_{suffix}_{64 hex chars}
		&RegexScrubber{
			name:        "g8e_api_key",
			pattern:     regexp.MustCompile(`\bg8e_[a-z0-9]+_[0-9a-f]{64}\b`),
			replacement: "[G8E_API_KEY]",
		},
		// JWT (JSON Web Token) - specific base64.base64.base64 format
		&RegexScrubber{
			name:        "jwt",
			pattern:     regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*\b`),
			replacement: "[JWT]",
		},
		// SendGrid API Key - looks like hostname (SG.xxx.xxx) so must come first
		&RegexScrubber{
			name:        "sendgrid_key",
			pattern:     regexp.MustCompile(`\bSG\.[0-9A-Za-z_-]{22}\.[0-9A-Za-z_-]{43}\b`),
			replacement: "[SENDGRID_KEY]",
		},

		// GitHub Token - specific prefix pattern
		&RegexScrubber{
			name:        "github_token",
			pattern:     regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}\b`),
			replacement: "[GITHUB_TOKEN]",
		},
		// GCP API Key - specific AIza prefix
		&RegexScrubber{
			name:        "gcp_api_key",
			pattern:     regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
			replacement: "[GCP_API_KEY]",
		},
		// AWS Access Key ID - specific AKIA/ASIA prefix
		&RegexScrubber{
			name:        "aws_access_key",
			pattern:     regexp.MustCompile(`\b(AKIA|ABIA|ACCA|ASIA)[0-9A-Z]{16}\b`),
			replacement: "[AWS_KEY]",
		},
		// Slack Token - xoxb/xoxp/xoxs/xapp prefixes (enterprise comms)
		&RegexScrubber{
			name:        "slack_token",
			pattern:     regexp.MustCompile(`\b(xoxb|xoxp|xoxs|xapp)-[0-9A-Za-z-]{24,}\b`),
			replacement: "[SLACK_TOKEN]",
		},
		// Okta API Token - 00 prefix with 40 alphanumeric chars (enterprise/gov identity)
		&RegexScrubber{
			name:        "okta_api_token",
			pattern:     regexp.MustCompile(`\b00[A-Za-z0-9_-]{40}\b`),
			replacement: "[OKTA_TOKEN]",
		},
		// Azure AD Client Secret - 3+ chars ~ 34+ chars format (gov/healthcare Microsoft)
		&RegexScrubber{
			name:        "azure_client_secret",
			pattern:     regexp.MustCompile(`\b[A-Za-z0-9]{3,8}~[A-Za-z0-9._-]{34,}\b`),
			replacement: "[AZURE_SECRET]",
		},
		// Twilio Account SID - specific AC prefix with hex
		&RegexScrubber{
			name:        "twilio_sid",
			pattern:     regexp.MustCompile(`\bAC[a-f0-9]{32}\b`),
			replacement: "[TWILIO_SID]",
		},
		// NPM Token - specific npm_ prefix
		&RegexScrubber{
			name:        "npm_token",
			pattern:     regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`),
			replacement: "[NPM_TOKEN]",
		},
		// PyPI Token - specific pypi- prefix with base64
		&RegexScrubber{
			name:        "pypi_token",
			pattern:     regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}\b`),
			replacement: "[PYPI_TOKEN]",
		},
		// Discord Bot Token - specific format with dots
		&RegexScrubber{
			name:        "discord_token",
			pattern:     regexp.MustCompile(`\b[MN][A-Za-z\d]{23,}\.[\w-]{6}\.[\w-]{27}\b`),
			replacement: "[DISCORD_TOKEN]",
		},
		// Private key markers - very specific format
		&RegexScrubber{
			name:        "private_key",
			pattern:     regexp.MustCompile(`-----BEGIN[^-]+PRIVATE KEY-----[\s\S]*?-----END[^-]+PRIVATE KEY-----`),
			replacement: "[PRIVATE_KEY]",
		},

		// AWS Secret Key pattern in config/env
		&RegexScrubber{
			name:        "aws_secret_key",
			pattern:     regexp.MustCompile(`(?i)aws.{0,20}secret.{0,20}['\"]?[0-9a-zA-Z/+=]{40}['\"]?`),
			replacement: "[AWS_SECRET]",
		},
		// Azure Client Secret / Service Principal
		&RegexScrubber{
			name:        "azure_secret",
			pattern:     regexp.MustCompile(`(?i)azure.{0,20}(secret|password|key).{0,20}['"][A-Za-z0-9_\-\.~]{32,}['"]`),
			replacement: "[AZURE_SECRET]",
		},
		// Generic OAuth Client Secret
		&RegexScrubber{
			name:        "oauth_secret",
			pattern:     regexp.MustCompile(`(?i)(client.?secret|oauth.?secret)\s*[=:]\s*['"]?[A-Za-z0-9_\-]{20,}['"]?`),
			replacement: "[OAUTH_SECRET]",
		},
		// Heroku API Key
		&RegexScrubber{
			name:        "heroku_key",
			pattern:     regexp.MustCompile(`(?i)heroku.{0,20}(api.?key|token).{0,20}['"]?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}['"]?`),
			replacement: "[HEROKU_KEY]",
		},

		// URLs with embedded credentials (user:pass@host)
		&RegexScrubber{
			name:        "url_with_creds",
			pattern:     regexp.MustCompile(`https?://[^:]+:[^@]+@[^\s<>"{}|\\^` + "`" + `\[\]]+`),
			replacement: "[URL_WITH_CREDENTIALS]",
		},
		// Connection strings (must come before email for same reason)
		&RegexScrubber{
			name:        "conn_string",
			pattern:     regexp.MustCompile(`(?i)(?:mysql|postgres|mongodb|redis|amqp|jdbc)://[^\s]+`),
			replacement: "[CONN_STRING]",
		},
		// Email addresses - any @domain.tld pattern
		&RegexScrubber{
			name:        "email",
			pattern:     regexp.MustCompile(`[A-Za-z0-9._%+'-]*@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
			replacement: "[EMAIL]",
		},

		// Credit card numbers
		&RegexScrubber{
			name:        "credit_card",
			pattern:     regexp.MustCompile(`\b(?:\d{4}[- ]?){3}\d{4}\b`),
			replacement: "[PII]",
		},
		// SSN
		&RegexScrubber{
			name:        "ssn",
			pattern:     regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			replacement: "[PII]",
		},
		// Phone numbers
		&RegexScrubber{
			name:        "phone",
			pattern:     regexp.MustCompile(`\b(?:\+\d{1,3}[- ]?)?\(?\d{3}\)?[- ]?\d{3}[- ]?\d{4}\b`),
			replacement: "[PHONE]",
		},
		// Password patterns in config (generic catch-all)
		&RegexScrubber{
			name:        "password_config",
			pattern:     regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|token|api_key|apikey)\s*[=:]\s*\S+`),
			replacement: "[CREDENTIAL_REFERENCE]",
		},
		// IBAN (International Bank Account Number) - covers 70+ countries
		&RegexScrubber{
			name:        "iban",
			pattern:     regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{4,30}\b`),
			replacement: "[IBAN]",
		},
		// Generic Bearer Token in headers
		&RegexScrubber{
			name:        "bearer_token",
			pattern:     regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_\-\.]+`),
			replacement: "[BEARER_TOKEN]",
		},
	}

	for name, pattern := range s.config.CustomScrubPatterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			s.logger.Warn("Invalid custom scrub pattern", "name", name, "error", err)
			continue
		}
		s.scrubbers = append(s.scrubbers, &RegexScrubber{
			name:        "custom_" + name,
			pattern:     compiled,
			replacement: "[" + strings.ToUpper(name) + "]",
		})
	}
}

// initializeThreatDetectors sets up all threat detection patterns
// These detectors run alongside scrubbing to identify malicious activity
// Each detector is mapped to MITRE ATT&CK framework for SOC integration
func (s *Sentinel) initializeThreatDetectors() {
	if !s.config.ThreatDetectionEnabled {
		return
	}

	s.threatDetectors = []ThreatDetector{
		&RegexThreatDetector{
			name:           "reverse_shell_nc",
			pattern:        regexp.MustCompile(`(?i)nc\s+.*-e\s+(/bin/)?(sh|bash|zsh)`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.95,
			mitreAttack:    "T1059.004",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_bash_tcp",
			pattern:        regexp.MustCompile(`(?i)bash\s+-i\s+>&\s*/dev/tcp/`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.98,
			mitreAttack:    "T1059.004",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_python",
			pattern:        regexp.MustCompile(`(?i)python[23]?\s+-c\s+['"]import\s+socket`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.90,
			mitreAttack:    "T1059.006",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_perl",
			pattern:        regexp.MustCompile(`(?i)perl\s+-e\s+['"]use\s+Socket`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.90,
			mitreAttack:    "T1059.006",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_ruby",
			pattern:        regexp.MustCompile(`(?i)ruby\s+-rsocket\s+-e`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.90,
			mitreAttack:    "T1059.006",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_php",
			pattern:        regexp.MustCompile(`(?i)php\s+-r\s+['"].*fsockopen`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.90,
			mitreAttack:    "T1059.006",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},
		&RegexThreatDetector{
			name:           "reverse_shell_mkfifo",
			pattern:        regexp.MustCompile(`(?i)mkfifo\s+.*&&.*nc\s+`),
			category:       ThreatCategoryReverseShell,
			severity:       ThreatSeverityCritical,
			confidence:     0.95,
			mitreAttack:    "T1059.004",
			mitreTactic:    "Execution",
			recommendation: "Immediately terminate process and investigate source",
		},

		&RegexThreatDetector{
			name:           "privesc_sudo_nopasswd",
			pattern:        regexp.MustCompile(`(?i)echo\s+.*NOPASSWD.*>>\s*/etc/sudoers`),
			category:       ThreatCategoryPrivilegeEsc,
			severity:       ThreatSeverityCritical,
			confidence:     0.98,
			mitreAttack:    "T1548.003",
			mitreTactic:    "Privilege Escalation",
			recommendation: "Review sudoers file and remove unauthorized entries",
		},
		&RegexThreatDetector{
			name:           "privesc_suid_chmod",
			pattern:        regexp.MustCompile(`(?i)chmod\s+(u\+s|4[0-7]{3}|[0-7]*4[0-7]*)\s+.*(/bin/|/usr/bin/)`),
			category:       ThreatCategoryPrivilegeEsc,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1548.001",
			mitreTactic:    "Privilege Escalation",
			recommendation: "Audit SUID binaries and remove unauthorized permissions",
		},
		&RegexThreatDetector{
			name:           "privesc_capability_set",
			pattern:        regexp.MustCompile(`(?i)setcap\s+.*cap_setuid`),
			category:       ThreatCategoryPrivilegeEsc,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1548.001",
			mitreTactic:    "Privilege Escalation",
			recommendation: "Review file capabilities and remove unauthorized",
		},
		&RegexThreatDetector{
			name:           "privesc_passwd_edit",
			pattern:        regexp.MustCompile(`(?i)(vi|vim|nano|echo)\s+.*(/etc/passwd|/etc/shadow)`),
			category:       ThreatCategoryPrivilegeEsc,
			severity:       ThreatSeverityCritical,
			confidence:     0.85,
			mitreAttack:    "T1136.001",
			mitreTactic:    "Privilege Escalation",
			recommendation: "Verify integrity of passwd/shadow files",
		},
		&RegexThreatDetector{
			name:           "privesc_ld_preload",
			pattern:        regexp.MustCompile(`(?i)LD_PRELOAD\s*=`),
			category:       ThreatCategoryPrivilegeEsc,
			severity:       ThreatSeverityHigh,
			confidence:     0.80,
			mitreAttack:    "T1574.006",
			mitreTactic:    "Privilege Escalation",
			recommendation: "Investigate LD_PRELOAD usage and loaded libraries",
		},

		&RegexThreatDetector{
			name:           "cred_passwd_dump",
			pattern:        regexp.MustCompile(`(?i)cat\s+/etc/(passwd|shadow|master\.passwd)`),
			category:       ThreatCategoryCredentialAccess,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1003.008",
			mitreTactic:    "Credential Access",
			recommendation: "Investigate credential harvesting attempt",
		},
		&RegexThreatDetector{
			name:           "cred_ssh_key_access",
			pattern:        regexp.MustCompile(`(?i)cat\s+.*\.ssh/(id_rsa|id_ed25519|authorized_keys)`),
			category:       ThreatCategoryCredentialAccess,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1552.004",
			mitreTactic:    "Credential Access",
			recommendation: "Review SSH key access and rotate compromised keys",
		},
		&RegexThreatDetector{
			name:           "cred_aws_credentials",
			pattern:        regexp.MustCompile(`(?i)cat\s+.*\.aws/(credentials|config)`),
			category:       ThreatCategoryCredentialAccess,
			severity:       ThreatSeverityCritical,
			confidence:     0.95,
			mitreAttack:    "T1552.001",
			mitreTactic:    "Credential Access",
			recommendation: "Rotate AWS credentials immediately",
		},
		&RegexThreatDetector{
			name:           "cred_browser_passwords",
			pattern:        regexp.MustCompile(`(?i)(Login\s+Data|logins\.json|cookies\.sqlite)`),
			category:       ThreatCategoryCredentialAccess,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1555.003",
			mitreTactic:    "Credential Access",
			recommendation: "Investigate browser credential theft",
		},
		&RegexThreatDetector{
			name:           "cred_mimikatz_pattern",
			pattern:        regexp.MustCompile(`(?i)(sekurlsa|lsadump|kerberos::)`),
			category:       ThreatCategoryCredentialAccess,
			severity:       ThreatSeverityCritical,
			confidence:     0.98,
			mitreAttack:    "T1003.001",
			mitreTactic:    "Credential Access",
			recommendation: "Isolate system and investigate credential compromise",
		},

		&RegexThreatDetector{
			name:           "exfil_curl_post",
			pattern:        regexp.MustCompile(`(?i)curl\s+.*(-d|--data|--data-binary)\s+@`),
			category:       ThreatCategoryExfiltration,
			severity:       ThreatSeverityMedium,
			confidence:     0.70,
			mitreAttack:    "T1048.003",
			mitreTactic:    "Exfiltration",
			recommendation: "Review outbound data transfers",
		},
		&RegexThreatDetector{
			name:           "exfil_base64_pipe",
			pattern:        regexp.MustCompile(`(?i)base64\s+.*\|\s*(curl|wget|nc)`),
			category:       ThreatCategoryExfiltration,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1048.003",
			mitreTactic:    "Exfiltration",
			recommendation: "Block outbound transfer and investigate data",
		},
		&RegexThreatDetector{
			name:           "exfil_dns_tunnel",
			pattern:        regexp.MustCompile(`(?i)(dig|nslookup)\s+.*\$\(`),
			category:       ThreatCategoryExfiltration,
			severity:       ThreatSeverityHigh,
			confidence:     0.80,
			mitreAttack:    "T1048.001",
			mitreTactic:    "Exfiltration",
			recommendation: "Investigate DNS tunneling activity",
		},
		&RegexThreatDetector{
			name:           "exfil_tar_stream",
			pattern:        regexp.MustCompile(`(?i)tar\s+.*\|\s*(curl|wget|nc|ssh)`),
			category:       ThreatCategoryExfiltration,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1048.003",
			mitreTactic:    "Exfiltration",
			recommendation: "Block transfer and review archived data",
		},

		&RegexThreatDetector{
			name:           "miner_xmrig",
			pattern:        regexp.MustCompile(`(?i)(xmrig|xmr-stak|minerd|cpuminer)`),
			category:       ThreatCategoryCryptominer,
			severity:       ThreatSeverityHigh,
			confidence:     0.95,
			mitreAttack:    "T1496",
			mitreTactic:    "Impact",
			recommendation: "Terminate mining process and remove malware",
		},
		&RegexThreatDetector{
			name:           "miner_stratum",
			pattern:        regexp.MustCompile(`(?i)stratum\+tcp://`),
			category:       ThreatCategoryCryptominer,
			severity:       ThreatSeverityHigh,
			confidence:     0.98,
			mitreAttack:    "T1496",
			mitreTactic:    "Impact",
			recommendation: "Block mining pool connections",
		},
		&RegexThreatDetector{
			name:           "miner_pool_domains",
			pattern:        regexp.MustCompile(`(?i)(pool\.(minergate|supportxmr|hashvault)|moneropool|nanopool)`),
			category:       ThreatCategoryCryptominer,
			severity:       ThreatSeverityHigh,
			confidence:     0.95,
			mitreAttack:    "T1496",
			mitreTactic:    "Impact",
			recommendation: "Block mining pool domains",
		},

		&RegexThreatDetector{
			name:           "persist_cron_download",
			pattern:        regexp.MustCompile(`(?i)crontab.*\|\s*(curl|wget)\s+`),
			category:       ThreatCategoryPersistence,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1053.003",
			mitreTactic:    "Persistence",
			recommendation: "Review crontab entries for unauthorized jobs",
		},
		&RegexThreatDetector{
			name:           "persist_systemd_create",
			pattern:        regexp.MustCompile(`(?i)(echo|cat)\s+.*>\s*/etc/systemd/system/.*\.service`),
			category:       ThreatCategoryPersistence,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1543.002",
			mitreTactic:    "Persistence",
			recommendation: "Review systemd services for unauthorized entries",
		},
		&RegexThreatDetector{
			name:           "persist_rc_local",
			pattern:        regexp.MustCompile(`(?i)(echo|cat)\s+.*>>\s*/etc/rc\.local`),
			category:       ThreatCategoryPersistence,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1037.004",
			mitreTactic:    "Persistence",
			recommendation: "Review rc.local for unauthorized entries",
		},
		&RegexThreatDetector{
			name:           "persist_bashrc_inject",
			pattern:        regexp.MustCompile(`(?i)(echo|cat)\s+.*>>\s*~?/.*\.(bashrc|bash_profile|zshrc|profile)`),
			category:       ThreatCategoryPersistence,
			severity:       ThreatSeverityMedium,
			confidence:     0.80,
			mitreAttack:    "T1546.004",
			mitreTactic:    "Persistence",
			recommendation: "Review shell profile files for malicious entries",
		},
		&RegexThreatDetector{
			name:           "persist_ssh_keys",
			pattern:        regexp.MustCompile(`(?i)(echo|cat)\s+.*>>\s*.*\.ssh/authorized_keys`),
			category:       ThreatCategoryPersistence,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1098.004",
			mitreTactic:    "Persistence",
			recommendation: "Review authorized_keys for unauthorized keys",
		},

		&RegexThreatDetector{
			name:           "evasion_history_clear",
			pattern:        regexp.MustCompile(`(?i)(history\s+-c|rm\s+.*\.(bash_history|zsh_history)|unset\s+HISTFILE)`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityMedium,
			confidence:     0.85,
			mitreAttack:    "T1070.003",
			mitreTactic:    "Defense Evasion",
			recommendation: "Investigate why command history was cleared",
		},
		&RegexThreatDetector{
			name:           "evasion_log_tampering",
			pattern:        regexp.MustCompile(`(?i)(rm|truncate|shred)\s+.*/var/log/`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1070.002",
			mitreTactic:    "Defense Evasion",
			recommendation: "Restore logs from backup and investigate tampering",
		},
		&RegexThreatDetector{
			name:           "evasion_timestomp",
			pattern:        regexp.MustCompile(`(?i)touch\s+-[amdrt]`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityMedium,
			confidence:     0.70,
			mitreAttack:    "T1070.006",
			mitreTactic:    "Defense Evasion",
			recommendation: "Investigate file timestamp manipulation",
		},

		&RegexThreatDetector{
			name:           "recon_port_scan",
			pattern:        regexp.MustCompile(`(?i)(nmap|masscan|zmap)\s+`),
			category:       ThreatCategoryReconnaissance,
			severity:       ThreatSeverityMedium,
			confidence:     0.80,
			mitreAttack:    "T1046",
			mitreTactic:    "Discovery",
			recommendation: "Verify port scanning is authorized",
		},
		&RegexThreatDetector{
			name:           "recon_internal_network",
			pattern:        regexp.MustCompile(`(?i)(arp\s+-a|ip\s+neigh|netstat\s+-rn)`),
			category:       ThreatCategoryReconnaissance,
			severity:       ThreatSeverityLow,
			confidence:     0.60,
			mitreAttack:    "T1016",
			mitreTactic:    "Discovery",
			recommendation: "Monitor for follow-up lateral movement",
		},

		&RegexThreatDetector{
			name:           "lotl_curl_bash",
			pattern:        regexp.MustCompile(`(?i)curl\s+.*\|\s*(ba)?sh`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1059.004",
			mitreTactic:    "Execution",
			recommendation: "Block piped execution and review URL",
		},
		&RegexThreatDetector{
			name:           "lotl_wget_execute",
			pattern:        regexp.MustCompile(`(?i)wget\s+.*(-O\s*-|--output-document=-).*\|\s*(ba)?sh`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1059.004",
			mitreTactic:    "Execution",
			recommendation: "Block piped execution and review URL",
		},
		&RegexThreatDetector{
			name:           "lotl_eval_base64",
			pattern:        regexp.MustCompile(`(?i)eval\s+.*\$\(.*base64\s+-d`),
			category:       ThreatCategoryDefenseEvasion,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1027",
			mitreTactic:    "Defense Evasion",
			recommendation: "Decode and analyze obfuscated command",
		},

		&RegexThreatDetector{
			name:           "lateral_ssh_remote",
			pattern:        regexp.MustCompile(`(?i)ssh\s+.*[A-Za-z0-9_.-]+@[A-Za-z0-9._-]+`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityMedium,
			confidence:     0.60,
			mitreAttack:    "T1021.004",
			mitreTactic:    "Lateral Movement",
			recommendation: "Verify SSH connection is authorized",
		},
		&RegexThreatDetector{
			name:           "lateral_rdp_connection",
			pattern:        regexp.MustCompile(`(?i)(xfreerdp|rdesktop|mstsc)\s+`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityMedium,
			confidence:     0.70,
			mitreAttack:    "T1021.001",
			mitreTactic:    "Lateral Movement",
			recommendation: "Verify RDP connection is authorized",
		},
		&RegexThreatDetector{
			name:           "lateral_smb_mount",
			pattern:        regexp.MustCompile(`(?i)mount\s+.*-t\s+(cifs|smb)`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityMedium,
			confidence:     0.65,
			mitreAttack:    "T1021.002",
			mitreTactic:    "Lateral Movement",
			recommendation: "Verify SMB share mount is authorized",
		},
		&RegexThreatDetector{
			name:           "lateral_psexec",
			pattern:        regexp.MustCompile(`(?i)(psexec|winexe|smbexec)\s+`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityHigh,
			confidence:     0.90,
			mitreAttack:    "T1021.002",
			mitreTactic:    "Lateral Movement",
			recommendation: "Investigate remote execution tool usage",
		},
		&RegexThreatDetector{
			name:           "lateral_winrm",
			pattern:        regexp.MustCompile(`(?i)(winrm|evil-winrm|Enter-PSSession)`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityHigh,
			confidence:     0.85,
			mitreAttack:    "T1021.006",
			mitreTactic:    "Lateral Movement",
			recommendation: "Investigate WinRM remote management usage",
		},
		&RegexThreatDetector{
			name:           "lateral_pass_the_hash",
			pattern:        regexp.MustCompile(`(?i)(pth-|pass.the.hash|wmiexec|atexec|dcomexec)`),
			category:       ThreatCategoryLateralMovement,
			severity:       ThreatSeverityCritical,
			confidence:     0.95,
			mitreAttack:    "T1550.002",
			mitreTactic:    "Lateral Movement",
			recommendation: "Isolate system immediately - credential abuse detected",
		},

		&RegexThreatDetector{
			name:           "hijack_container_spawn",
			pattern:        regexp.MustCompile(`(?i)docker\s+run\s+.*--privileged`),
			category:       ThreatCategoryResourceHijacking,
			severity:       ThreatSeverityHigh,
			confidence:     0.75,
			mitreAttack:    "T1610",
			mitreTactic:    "Execution",
			recommendation: "Review privileged container deployment",
		},
		&RegexThreatDetector{
			name:           "hijack_kubectl_exec",
			pattern:        regexp.MustCompile(`(?i)kubectl\s+exec\s+.*--\s*(ba)?sh`),
			category:       ThreatCategoryResourceHijacking,
			severity:       ThreatSeverityMedium,
			confidence:     0.65,
			mitreAttack:    "T1609",
			mitreTactic:    "Execution",
			recommendation: "Verify kubectl exec is authorized",
		},
		&RegexThreatDetector{
			name:           "hijack_stress_test",
			pattern:        regexp.MustCompile(`(?i)(stress|stress-ng|cpuburn)\s+`),
			category:       ThreatCategoryResourceHijacking,
			severity:       ThreatSeverityMedium,
			confidence:     0.70,
			mitreAttack:    "T1496",
			mitreTactic:    "Impact",
			recommendation: "Investigate CPU stress testing - may indicate abuse",
		},
	}
}

// detectThreats runs all threat detectors on the input and returns signals
func (s *Sentinel) detectThreats(input string) []ThreatSignal {
	if !s.config.ThreatDetectionEnabled {
		return nil
	}

	var signals []ThreatSignal
	for _, detector := range s.threatDetectors {
		detected := detector.Detect(input)
		signals = append(signals, detected...)
	}
	return signals
}

// aggregateThreatLevel determines the overall threat level from signals
func (s *Sentinel) aggregateThreatLevel(signals []ThreatSignal) ThreatLevel {
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

// ScrubCommandResult processes raw command output and returns a safe summary
// It performs dual-purpose scanning: data protection AND threat detection
func (s *Sentinel) ScrubCommandResult(result *CommandResult) *ScrubbedResult {
	if !s.config.Enabled {
		// Even when disabled, we still provide structure without raw data
		return &ScrubbedResult{
			Status:      s.determineStatus(result.ExitCode),
			ExitCode:    result.ExitCode,
			Summary:     "Scrubbing disabled - output suppressed for safety",
			DurationMs:  result.DurationMs,
			OutputLines: countLines(result.Stdout),
			ThreatLevel: ThreatLevelNone,
		}
	}

	scrubbed := &ScrubbedResult{
		Status:      s.determineStatus(result.ExitCode),
		ExitCode:    result.ExitCode,
		DurationMs:  result.DurationMs,
		OutputLines: countLines(result.Stdout),
	}

	// Extract structural information before scrubbing
	scrubbed.RowCount = s.extractRowCount(result.Stdout)
	scrubbed.StructureHints = s.extractStructureHints(result.Stdout)
	scrubbed.ErrorType = s.categorizeError(result.Stderr, result.ExitCode)

	// Build summary from scrubbed content
	scrubbed.Summary = s.buildSummary(result)

	// Extract and scrub warnings
	scrubbed.Warnings = s.extractWarnings(result.Stderr)

	// Threat detection: scan command, stdout, and stderr for malicious patterns
	combinedInput := result.Command + "\n" + result.Stdout + "\n" + result.Stderr
	scrubbed.ThreatSignals = s.detectThreats(combinedInput)
	scrubbed.ThreatCount = len(scrubbed.ThreatSignals)
	scrubbed.ThreatLevel = s.aggregateThreatLevel(scrubbed.ThreatSignals)

	// Log threat detections for audit trail
	if scrubbed.ThreatCount > 0 {
		s.logger.Warn("Threat signals detected",
			"threat_level", scrubbed.ThreatLevel,
			"threat_count", scrubbed.ThreatCount,
			"categories", s.extractThreatCategories(scrubbed.ThreatSignals))
	}

	return scrubbed
}

// extractThreatCategories returns unique threat categories from signals
func (s *Sentinel) extractThreatCategories(signals []ThreatSignal) []string {
	seen := make(map[ThreatCategory]bool)
	var categories []string
	for _, sig := range signals {
		if !seen[sig.Category] {
			seen[sig.Category] = true
			categories = append(categories, string(sig.Category))
		}
	}
	return categories
}

// ScrubText applies all scrubbers to arbitrary text
func (s *Sentinel) ScrubText(input string) string {
	if !s.config.Enabled {
		return "[OUTPUT_SUPPRESSED]"
	}

	result := input
	for _, scrubber := range s.scrubbers {
		result = scrubber.Scrub(result)
	}

	// In strict mode, also scrub anything that looks like data values
	if s.config.StrictMode {
		result = s.scrubDataValues(result)
	}

	// Truncate if needed
	if s.config.MaxOutputLength > 0 && len(result) > s.config.MaxOutputLength {
		result = result[:s.config.MaxOutputLength] + "... [TRUNCATED]"
	}

	return result
}

// scrubDataValues handles additional scrubbing for sensitive key-value pairs
// Pattern scrubbers already handle PII, credentials, IPs, etc. in the data values.
// This function only redacts values for keys that are inherently sensitive (passwords, secrets, etc.)
func (s *Sentinel) scrubDataValues(input string) string {
	lines := strings.Split(input, "\n")
	var result []string

	for _, line := range lines {
		// Preserve empty lines for output formatting
		if strings.TrimSpace(line) == "" {
			result = append(result, line)
			continue
		}

		// Check if line looks like key-value data with a sensitive key
		// Only redact the VALUE if the KEY itself indicates sensitive data
		if s.looksLikeKeyValue(line) {
			key := s.extractKey(line)
			if s.isLikelySensitiveKey(key) {
				result = append(result, fmt.Sprintf("%s: [VALUE]", key))
				continue
			}
		}

		// Keep the line - pattern scrubbers already handled sensitive values
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// isScrubberPlaceholder checks if a string is one of our scrubber placeholders
// like [PATH], [HOST], [EMAIL], [VALUE], etc.
func (s *Sentinel) isScrubberPlaceholder(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Placeholders are in format [UPPERCASE_WITH_UNDERSCORES]
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return false
	}
	// Extract content between brackets
	content := trimmed[1 : len(trimmed)-1]
	// Placeholders are all uppercase with optional underscores
	for _, r := range content {
		if !((r >= 'A' && r <= 'Z') || r == '_') {
			return false
		}
	}
	return len(content) > 0
}

// looksLikeKeyValue checks if a line is a key-value pair
func (s *Sentinel) looksLikeKeyValue(line string) bool {
	// Patterns like "Key: Value" or "key=value"
	return strings.Contains(line, ": ") ||
		(strings.Contains(line, "=") && !strings.HasPrefix(strings.TrimSpace(line), "#"))
}

// extractKey gets the key portion from a key-value line
func (s *Sentinel) extractKey(line string) string {
	if idx := strings.Index(line, ": "); idx > 0 {
		key := strings.TrimSpace(line[:idx])
		// Scrub the key itself if it contains sensitive patterns
		return s.ScrubText(key)
	}
	if idx := strings.Index(line, "="); idx > 0 {
		key := strings.TrimSpace(line[:idx])
		return s.ScrubText(key)
	}
	return "[KEY]"
}

// determineStatus maps exit code to a status category
func (s *Sentinel) determineStatus(exitCode int) string {
	switch exitCode {
	case 0:
		return "success"
	case 1:
		return "failure"
	case 2:
		return "misuse"
	case 126:
		return "not_executable"
	case 127:
		return "not_found"
	case 128:
		return "invalid_exit"
	case 130:
		return "interrupted"
	case 137:
		return "killed"
	case 143:
		return constants.Status.OperatorStatus.Terminated
	default:
		if exitCode > 128 {
			return fmt.Sprintf("signal_%d", exitCode-128)
		}
		return "error"
	}
}

// categorizeError determines the type of error from stderr
func (s *Sentinel) categorizeError(stderr string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	stderrLower := strings.ToLower(stderr)

	// Check for common error patterns
	switch {
	case strings.Contains(stderrLower, "permission denied"):
		return "permission_denied"
	case strings.Contains(stderrLower, "not found") || strings.Contains(stderrLower, "no such file"):
		return "not_found"
	case strings.Contains(stderrLower, "timeout") || strings.Contains(stderrLower, "timed out"):
		return "timeout"
	case strings.Contains(stderrLower, "connection refused"):
		return "connection_refused"
	case strings.Contains(stderrLower, "connection reset"):
		return "connection_reset"
	case strings.Contains(stderrLower, "out of memory") || strings.Contains(stderrLower, "oom"):
		return "out_of_memory"
	case strings.Contains(stderrLower, "disk full") || strings.Contains(stderrLower, "no space"):
		return "disk_full"
	case strings.Contains(stderrLower, "authentication") || strings.Contains(stderrLower, "unauthorized"):
		return "authentication_failed"
	case strings.Contains(stderrLower, "syntax error"):
		return "syntax_error"
	case strings.Contains(stderrLower, "invalid"):
		return "invalid_input"
	case strings.Contains(stderrLower, "already exists"):
		return "already_exists"
	case strings.Contains(stderrLower, "locked") || strings.Contains(stderrLower, "busy"):
		return "resource_busy"
	case strings.Contains(stderrLower, "quota"):
		return "quota_exceeded"
	default:
		return "unknown_error"
	}
}

// extractRowCount tries to determine how many data rows are in the output
func (s *Sentinel) extractRowCount(stdout string) *int {
	lines := strings.Split(stdout, "\n")

	// Filter out empty lines and obvious headers/footers
	dataLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip common header/footer patterns
		if strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "--") ||
			strings.HasPrefix(trimmed, "==") ||
			strings.HasPrefix(trimmed, "+-") {
			continue
		}
		dataLines++
	}

	if dataLines > 0 {
		return &dataLines
	}
	return nil
}

// extractStructureHints provides schema-level information without data
func (s *Sentinel) extractStructureHints(stdout string) []string {
	var hints []string
	lines := strings.Split(stdout, "\n")

	if len(lines) > 0 {
		hints = append(hints, fmt.Sprintf("output_lines: %d", len(lines)))
	}

	// Check for JSON structure
	trimmed := strings.TrimSpace(stdout)
	if strings.HasPrefix(trimmed, "{") {
		hints = append(hints, "format: json_object")
	} else if strings.HasPrefix(trimmed, "[") {
		hints = append(hints, "format: json_array")
	}

	// Check for tabular structure
	if len(lines) > 1 {
		firstLine := lines[0]
		if strings.Contains(firstLine, "|") {
			colCount := strings.Count(firstLine, "|") - 1
			if colCount > 0 {
				hints = append(hints, fmt.Sprintf("columns: %d", colCount))
			}
		} else if strings.Contains(firstLine, "\t") {
			colCount := strings.Count(firstLine, "\t") + 1
			hints = append(hints, fmt.Sprintf("columns: %d", colCount))
		}
	}

	// Estimate data size category
	size := len(stdout)
	switch {
	case size < 100:
		hints = append(hints, "size: minimal")
	case size < 1000:
		hints = append(hints, "size: small")
	case size < 10000:
		hints = append(hints, "size: medium")
	case size < 100000:
		hints = append(hints, "size: large")
	default:
		hints = append(hints, "size: very_large")
	}

	return hints
}

// buildSummary creates a safe summary of the command result
func (s *Sentinel) buildSummary(result *CommandResult) string {
	var parts []string

	// Command executed (scrubbed)
	if result.Command != "" {
		parts = append(parts, fmt.Sprintf("Executed: %s", s.ScrubText(result.Command)))
	}

	// Exit status
	status := s.determineStatus(result.ExitCode)
	parts = append(parts, fmt.Sprintf("Status: %s (exit %d)", status, result.ExitCode))

	// Output presence
	if len(result.Stdout) > 0 {
		lines := countLines(result.Stdout)
		parts = append(parts, fmt.Sprintf("Output: %d lines", lines))
	} else {
		parts = append(parts, "Output: none")
	}

	// Error presence
	if len(result.Stderr) > 0 {
		errType := s.categorizeError(result.Stderr, result.ExitCode)
		if errType != "" {
			parts = append(parts, fmt.Sprintf("Error type: %s", errType))
		}
	}

	// Duration
	parts = append(parts, fmt.Sprintf("Duration: %dms", result.DurationMs))

	return strings.Join(parts, " | ")
}

// extractWarnings pulls warning messages from stderr and scrubs them
func (s *Sentinel) extractWarnings(stderr string) []string {
	if stderr == "" {
		return nil
	}

	var warnings []string
	lines := strings.Split(stderr, "\n")

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
			// Scrub the warning text
			scrubbed := s.ScrubText(line)
			// Extract just the warning category if possible
			category := s.categorizeWarning(line)
			if category != "" {
				warnings = append(warnings, category)
			} else {
				warnings = append(warnings, scrubbed)
			}
		}
	}

	return warnings
}

// categorizeWarning attempts to categorize a warning without exposing details
func (s *Sentinel) categorizeWarning(line string) string {
	lower := strings.ToLower(line)

	switch {
	case strings.Contains(lower, "deprecated"):
		return "deprecation_warning"
	case strings.Contains(lower, "insecure"):
		return "security_warning"
	case strings.Contains(lower, "performance"):
		return "performance_warning"
	case strings.Contains(lower, "memory"):
		return "memory_warning"
	case strings.Contains(lower, "disk"):
		return "disk_warning"
	case strings.Contains(lower, "network"):
		return "network_warning"
	case strings.Contains(lower, "certificate") || strings.Contains(lower, "ssl") || strings.Contains(lower, "tls"):
		return "certificate_warning"
	case strings.Contains(lower, "version"):
		return "version_warning"
	default:
		return ""
	}
}

// IsEnabled returns whether Sentinel scrubbing is active
func (s *Sentinel) IsEnabled() bool {
	return s.config.Enabled
}

// countLines counts non-empty lines in text
func countLines(text string) int {
	if text == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// ScrubMap scrubs all string values in a map recursively
func (s *Sentinel) ScrubMap(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range data {
		scrubbedKey := s.scrubKeyName(key)
		switch v := value.(type) {
		case string:
			result[scrubbedKey] = s.ScrubText(v)
		case map[string]interface{}:
			result[scrubbedKey] = s.ScrubMap(v)
		case []interface{}:
			result[scrubbedKey] = s.scrubSlice(v)
		case int, int64, float64, bool:
			// Numeric and boolean values are generally safe
			// but in strict mode, we might want to obscure them
			if s.config.StrictMode && s.isLikelySensitiveKey(key) {
				result[scrubbedKey] = "[VALUE]"
			} else {
				result[scrubbedKey] = v
			}
		default:
			result[scrubbedKey] = "[UNKNOWN_TYPE]"
		}
	}
	return result
}

// scrubSlice scrubs all elements in a slice
func (s *Sentinel) scrubSlice(data []interface{}) []interface{} {
	result := make([]interface{}, len(data))
	for i, item := range data {
		switch v := item.(type) {
		case string:
			result[i] = s.ScrubText(v)
		case map[string]interface{}:
			result[i] = s.ScrubMap(v)
		case []interface{}:
			result[i] = s.scrubSlice(v)
		default:
			result[i] = v
		}
	}
	return result
}

// scrubKeyName sanitizes key names that might contain sensitive info
func (s *Sentinel) scrubKeyName(key string) string {
	// Keys themselves might contain sensitive patterns
	return s.ScrubText(key)
}

// isLikelySensitiveKey checks if a key name suggests sensitive data.
// It uses word boundaries and exact matches where possible to avoid false positives.
func (s *Sentinel) isLikelySensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))

	// Exact matches for highest precision
	exactMatches := map[string]bool{
		"pwd":             true,
		"token":           true,
		"key":             true,
		"ssn":             true,
		"iban":            true,
		"api_key":         true,
		"apikey":          true,
		"secret":          true,
		"password":        true,
		"passwd":          true,
		"private":         true,
		"credit":          true,
		"account":         true,
		"account_number":  true,
		"balance":         true,
		"account_balance": true,
		"admin_pwd":       true,
		"card":            true,
		"card_number":     true,
	}
	if exactMatches[lower] {
		return true
	}

	// Pattern matches with context
	sensitivePatterns := []string{
		"password", "passwd", "secret", "token", "credential",
		"auth", "api_key", "apikey", "access_token", "private_key",
		"credit_card", "ssn", "income", "salary", "private", "credit",
		"account", "balance", "pwd", "card",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// ExtractSafeMetrics extracts only safe numeric metrics from output
func (s *Sentinel) ExtractSafeMetrics(stdout string) map[string]int {
	metrics := make(map[string]int)

	// Look for common metric patterns
	patterns := map[string]*regexp.Regexp{
		"row_count":     regexp.MustCompile(`(?i)(\d+)\s*rows?`),
		"record_count":  regexp.MustCompile(`(?i)(\d+)\s*records?`),
		"match_count":   regexp.MustCompile(`(?i)(\d+)\s*match(?:es)?`),
		"file_count":    regexp.MustCompile(`(?i)(\d+)\s*files?`),
		"error_count":   regexp.MustCompile(`(?i)(\d+)\s*errors?`),
		"warning_count": regexp.MustCompile(`(?i)(\d+)\s*warnings?`),
		"success_count": regexp.MustCompile(`(?i)(\d+)\s*success(?:ful)?`),
		"failed_count":  regexp.MustCompile(`(?i)(\d+)\s*failed`),
	}

	for name, pattern := range patterns {
		matches := pattern.FindStringSubmatch(stdout)
		if len(matches) >= 2 {
			if val, err := strconv.Atoi(matches[1]); err == nil {
				metrics[name] = val
			}
		}
	}

	return metrics
}

// ScrubForCloudAI is the main entry point for preparing data for AI Agent Services
// It applies maximum scrubbing to ensure no sensitive data leaks
func (s *Sentinel) ScrubForCloudAI(result *CommandResult) *ScrubbedResult {
	s.logger.Info("Scrubbing command result for cloud AI transmission",
		"command_length", len(result.Command),
		"stdout_length", len(result.Stdout),
		"stderr_length", len(result.Stderr),
		"exit_code", result.ExitCode)

	scrubbed := s.ScrubCommandResult(result)

	s.logger.Info("Scrubbing complete",
		"status", scrubbed.Status,
		"summary_length", len(scrubbed.Summary),
		"row_count", scrubbed.RowCount,
		"error_type", scrubbed.ErrorType)

	return scrubbed
}

// ValidateNoLeakage performs a final check that no obvious sensitive data remains
// Note: IPs, UUIDs, hostnames, file paths, ARNs, MAC addresses are intentionally
// preserved (not scrubbed) so they are NOT checked here.
func (s *Sentinel) ValidateNoLeakage(text string) (bool, []string) {
	var violations []string

	// Check for sensitive patterns that should have been scrubbed
	// System/operational data (IPs, UUIDs, hostnames, paths) are preserved intentionally
	checks := map[string]*regexp.Regexp{
		"email":       regexp.MustCompile(`[A-Za-z0-9._%+'-]*@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		"private_key": regexp.MustCompile(`-----BEGIN`),
	}

	for name, pattern := range checks {
		if pattern.MatchString(text) {
			// Check if it's one of our placeholders
			if !strings.Contains(text, "["+strings.ToUpper(name)+"]") {
				violations = append(violations, name)
			}
		}
	}

	return len(violations) == 0, violations
}

// SanitizeForDisplay creates a version suitable for showing to users
// This is less aggressive than ScrubForCloudAI
func (s *Sentinel) SanitizeForDisplay(text string) string {
	// Apply pattern scrubbing but keep structure
	result := text
	for _, scrubber := range s.scrubbers {
		result = scrubber.Scrub(result)
	}
	return result
}
