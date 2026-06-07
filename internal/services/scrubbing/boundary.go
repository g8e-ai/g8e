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

package scrubbing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/interfaces"
)

// Config holds configuration for the Sovereign Execution Boundary
type Config struct {
	// Enabled controls whether scrubbing is active
	Enabled bool

	// StrictMode when true, aggressively scrubs anything that looks like data
	StrictMode bool

	// MaxOutputLength limits the scrubbed output length (0 = no limit)
	MaxOutputLength int

	// AllowedPatterns are regex patterns that should pass through unscrubbed
	AllowedPatterns []string

	// CustomScrubPatterns are additional patterns to scrub
	CustomScrubPatterns map[string]string

	// RequirePersistence when true, fails closed if TokenStore is unavailable
	RequirePersistence bool
}

// DefaultConfig returns sensible defaults for production use
func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,
		StrictMode:          true,
		MaxOutputLength:     4096,
		AllowedPatterns:     []string{},
		CustomScrubPatterns: map[string]string{},
		RequirePersistence:  true,
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
	Status constants.SentinelStatus `json:"status"`

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
}

// ScrubbingService handles data scrubbing at the boundary plane:
// - Scrubbing: Removes sensitive data before cloud transmission
// - Rehydration: Restores sensitive data at execution boundary
// - Token Persistence: Maintains {{UEI_N}} mappings across restarts
type ScrubbingService struct {
	config    *Config
	logger    *slog.Logger
	scrubbers []Scrubber

	// Tokenized context state for data scrubbing
	tokenMu       sync.RWMutex
	tokenMap      map[string]string // {{UEI_1}} -> "sensitive value"
	reverseMap    map[string]string // "sensitive value" -> {{UEI_1}}
	tokenSequence int

	// Persistent storage for token maps
	tokenStore interfaces.TokenStore
}

// NewScrubbingService creates a new data scrubbing service
func NewScrubbingService(config *Config, logger *slog.Logger, tokenStore interfaces.TokenStore) *ScrubbingService {
	if config == nil {
		config = DefaultConfig()
	}

	s := &ScrubbingService{
		config:     config,
		logger:     logger,
		tokenMap:   make(map[string]string),
		reverseMap: make(map[string]string),
		tokenStore: tokenStore,
	}

	s.initializeScrubbers()

	// Load persisted tokens if storage is available
	if s.tokenStore != nil && s.tokenStore.IsEnabled() {
		s.loadPersistedTokens()
	}

	return s
}

// initializeScrubbers sets up all the pattern-based scrubbers
// IMPORTANT: Order matters! More specific patterns must come before generic ones.
// The scrubbers are applied sequentially, so a generic pattern matching first
// will prevent the specific pattern from ever seeing the text.
func (s *ScrubbingService) initializeScrubbers() {
	s.scrubbers = []Scrubber{
		// g8e Operator API Key - g8e_{suffix}_{64 hex chars}
		&RegexScrubber{
			name:        "internal_api_key",
			pattern:     regexp.MustCompile(`\bg8e_[a-z0-9]+_[0-9a-f]{64}\b`),
			replacement: "[REDACTED_API_KEY]",
		},
		// JWT (JSON Web Token) - specific base64.base64.base64 format
		&RegexScrubber{
			name:        string(constants.AuthProviderJWT),
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		compiled, err := compileRegexWithTimeout(ctx, pattern)
		cancel()
		if err != nil {
			s.logger.Warn("Invalid custom scrub pattern", "name", name, string(constants.ConnectionStateError), err)
			continue
		}
		s.scrubbers = append(s.scrubbers, &RegexScrubber{
			name:        "custom_" + name,
			pattern:     compiled,
			replacement: "[" + strings.ToUpper(name) + "]",
		})
	}
}

// compileRegexWithTimeout compiles a regex pattern with a timeout to prevent
// malicious or malformed patterns from blocking startup indefinitely
func compileRegexWithTimeout(ctx context.Context, pattern string) (*regexp.Regexp, error) {
	type result struct {
		re  *regexp.Regexp
		err error
	}

	resultChan := make(chan result, 1)

	go func() {
		re, err := regexp.Compile(pattern)
		resultChan <- result{re: re, err: err}
	}()

	select {
	case res := <-resultChan:
		return res.re, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("regex compilation timeout: %w", ctx.Err())
	}
}

// ScrubText applies all scrubbers to arbitrary text
func (s *ScrubbingService) ScrubText(input string) string {
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
func (s *ScrubbingService) scrubDataValues(input string) string {
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

// looksLikeKeyValue checks if a line is a key-value pair
func (s *ScrubbingService) looksLikeKeyValue(line string) bool {
	// Patterns like "Key: Value" or "key=value"
	return strings.Contains(line, ": ") ||
		(strings.Contains(line, "=") && !strings.HasPrefix(strings.TrimSpace(line), "#"))
}

// extractKey gets the key portion from a key-value line
func (s *ScrubbingService) extractKey(line string) string {
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

// isLikelySensitiveKey checks if a key name suggests sensitive data.
// It uses word boundaries and exact matches where possible to avoid false positives.
func (s *ScrubbingService) isLikelySensitiveKey(key string) bool {
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

// ScrubCommandResult processes raw command output and returns a safe summary
func (s *ScrubbingService) ScrubCommandResult(result *CommandResult) *ScrubbedResult {
	if !s.config.Enabled {
		// Even when disabled, we still provide structure without raw data
		return &ScrubbedResult{
			Status:      s.determineStatus(result.ExitCode),
			ExitCode:    result.ExitCode,
			Summary:     "Scrubbing disabled - output suppressed for safety",
			DurationMs:  result.DurationMs,
			OutputLines: countLines(result.Stdout),
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

	return scrubbed
}

// ScrubMap scrubs all string values in a map recursively
func (s *ScrubbingService) ScrubMap(data map[string]interface{}) map[string]interface{} {
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
func (s *ScrubbingService) scrubSlice(data []interface{}) []interface{} {
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
func (s *ScrubbingService) scrubKeyName(key string) string {
	// Keys themselves might contain sensitive patterns
	return s.ScrubText(key)
}

// determineStatus maps exit code to a status category
func (s *ScrubbingService) determineStatus(exitCode int) constants.SentinelStatus {
	switch exitCode {
	case 0:
		return constants.SentinelStatusSuccess
	case 1:
		return constants.SentinelStatusFailure
	case 2:
		return constants.SentinelStatusMisuse
	case 126:
		return constants.SentinelStatusNotExecutable
	case 127:
		return constants.SentinelStatusNotFound
	case 128:
		return constants.SentinelStatusInvalidExit
	case 130:
		return constants.SentinelStatusInterrupted
	case 137:
		return constants.SentinelStatusKilled
	case 143:
		return constants.SentinelStatusTerminated
	default:
		if exitCode > 128 {
			return constants.SentinelStatus(fmt.Sprintf("signal_%d", exitCode-128))
		}
		return constants.SentinelStatusError
	}
}

// categorizeError determines the type of error from stderr
func (s *ScrubbingService) categorizeError(stderr string, exitCode int) string {
	if exitCode == 0 {
		return ""
	}

	stderrLower := strings.ToLower(stderr)

	// Check for common error patterns
	switch {
	case strings.Contains(stderrLower, "permission denied"):
		return "permission_denied"
	case strings.Contains(stderrLower, "not found") || strings.Contains(stderrLower, "no such file"):
		return string(constants.SentinelStatusNotFound)
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
func (s *ScrubbingService) extractRowCount(stdout string) *int {
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
func (s *ScrubbingService) extractStructureHints(stdout string) []string {
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
func (s *ScrubbingService) buildSummary(result *CommandResult) string {
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
func (s *ScrubbingService) extractWarnings(stderr string) []string {
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
func (s *ScrubbingService) categorizeWarning(line string) string {
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
	case strings.Contains(lower, string(constants.ToolDisplayCategoryNetwork)):
		return string(constants.ToolDisplayCategoryNetwork)
	case strings.Contains(lower, "certificate") || strings.Contains(lower, "ssl") || strings.Contains(lower, "tls"):
		return "certificate_warning"
	case strings.Contains(lower, "version"):
		return "version_warning"
	default:
		return ""
	}
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

// RehydrateText replaces placeholders like {{UEI_N}} with their original values.
// This is used right before dispatch to restore sensitive data that was hidden from the cloud.
// Falls back to TokenStore if token is not in memory (for persistence across restarts).
func (s *ScrubbingService) RehydrateText(input string) string {
	if input == "" {
		return input
	}

	s.tokenMu.RLock()
	if len(s.tokenMap) == 0 && (s.tokenStore == nil || !s.tokenStore.IsEnabled()) {
		s.tokenMu.RUnlock()
		return input
	}

	result := input
	// First replace from in-memory cache
	for token, value := range s.tokenMap {
		result = strings.ReplaceAll(result, token, value)
	}
	s.tokenMu.RUnlock()

	// If TokenStore is available, check for any remaining tokens not in memory
	if s.tokenStore != nil && s.tokenStore.IsEnabled() {
		// Find all {{UEI_N}} patterns in the result
		tokenPattern := regexp.MustCompile(`\{\{UEI_\d+\}\}`)
		matches := tokenPattern.FindAllString(result, -1)

		// Check which tokens are already in memory
		s.tokenMu.RLock()
		inMemory := make(map[string]bool)
		for _, token := range matches {
			if _, ok := s.tokenMap[token]; ok {
				inMemory[token] = true
			}
		}
		s.tokenMu.RUnlock()

		for _, token := range matches {
			// Skip if already in memory (already replaced above)
			if inMemory[token] {
				continue
			}

			// Try to load from TokenStore
			key := fmt.Sprintf("sentinel_token_%s", token)
			if value, found := s.tokenStore.KVGet(key); found {
				// Add to in-memory cache for future use (requires write lock)
				s.tokenMu.Lock()
				s.tokenMap[token] = value
				s.reverseMap[value] = token
				s.tokenMu.Unlock()
				result = strings.ReplaceAll(result, token, value)
			}
		}
	}

	return result
}

// RehydratePayload recursively rehydrates all string values in a JSON payload.
func (s *ScrubbingService) RehydratePayload(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return payload, nil
	}

	// Try to parse as JSON first
	var data interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		// Not JSON, try text rehydration
		return []byte(s.RehydrateText(string(payload))), nil
	}

	rehydrated := s.rehydrateValueRecursive(data)
	return json.Marshal(rehydrated)
}

func (s *ScrubbingService) rehydrateValueRecursive(val interface{}) interface{} {
	switch v := val.(type) {
	case string:
		return s.RehydrateText(v)
	case map[string]interface{}:
		newMap := make(map[string]interface{}, len(v))
		for k, v2 := range v {
			newMap[k] = s.rehydrateValueRecursive(v2)
		}
		return newMap
	case []interface{}:
		newSlice := make([]interface{}, len(v))
		for i, v2 := range v {
			newSlice[i] = s.rehydrateValueRecursive(v2)
		}
		return newSlice
	default:
		return v
	}
}

// GetTokenForValue registers a sensitive value and returns a unique token for it.
// Fails closed if persistence is required but unavailable.
func (s *ScrubbingService) GetTokenForValue(value string) string {
	if value == "" {
		return ""
	}

	// Fail-closed: if persistence is required but unavailable, reject the operation
	if s.config.RequirePersistence && (s.tokenStore == nil || !s.tokenStore.IsEnabled()) {
		s.logger.Error("Token persistence required but TokenStore unavailable - failing closed to prevent data loss")
		return ""
	}

	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	if token, ok := s.reverseMap[value]; ok {
		return token
	}

	s.tokenSequence++
	token := fmt.Sprintf("{{UEI_%d}}", s.tokenSequence)
	s.tokenMap[token] = value
	s.reverseMap[value] = token

	// Persist to storage if available (24 hour TTL)
	if s.tokenStore != nil && s.tokenStore.IsEnabled() {
		const tokenTTLSeconds = 24 * 60 * 60
		key := fmt.Sprintf("sentinel_token_%s", token)
		if err := s.tokenStore.KVSet(key, value, tokenTTLSeconds); err != nil {
			s.logger.Error("Failed to persist token to local store - failing closed", "token", token, "error", err)
			// Rollback the in-memory token since persistence failed
			delete(s.tokenMap, token)
			delete(s.reverseMap, value)
			s.tokenSequence--
			return ""
		}
	}

	return token
}

// IsEnabled returns whether scrubbing is active
func (s *ScrubbingService) IsEnabled() bool {
	return s.config.Enabled
}

// loadPersistedTokens loads tokens from TokenStore on startup
func (s *ScrubbingService) loadPersistedTokens() {
	if s.tokenStore == nil || !s.tokenStore.IsEnabled() {
		s.logger.Warn("TokenStore not available for token persistence")
		return
	}

	tokens, err := s.tokenStore.KVScanPrefix("sentinel_token_")
	if err != nil {
		s.logger.Error("Failed to load persisted tokens from TokenStore", "error", err)
		return
	}

	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	loadedCount := 0
	maxSequence := 0
	for key, value := range tokens {
		// Extract token from key format: sentinel_token_{{UEI_N}}
		token := strings.TrimPrefix(key, "sentinel_token_")
		if token == key {
			s.logger.Warn("Invalid token key format", "key", key)
			continue
		}

		// Parse sequence number from token: {{UEI_N}}
		var seq int
		_, err := fmt.Sscanf(token, "{{UEI_%d}}", &seq)
		if err != nil {
			s.logger.Warn("Failed to parse token sequence", "token", token, "error", err)
			continue
		}

		if seq > maxSequence {
			maxSequence = seq
		}

		s.tokenMap[token] = value
		s.reverseMap[value] = token
		loadedCount++
	}

	s.tokenSequence = maxSequence
	s.logger.Info("Loaded persisted tokens from TokenStore", "count", loadedCount, "next_sequence", s.tokenSequence+1)
}

// ClearTokens clears all in-memory tokens (useful for testing or security events)
func (s *ScrubbingService) ClearTokens() {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	s.tokenMap = make(map[string]string)
	s.reverseMap = make(map[string]string)
	s.tokenSequence = 0

	s.logger.Info("Cleared all in-memory tokens")
}

// ExtractSafeMetrics extracts only safe numeric metrics from output
func (s *ScrubbingService) ExtractSafeMetrics(stdout string) map[string]int {
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

// ValidateNoLeakage performs a final check that no obvious sensitive data remains
// Note: IPs, UUIDs, hostnames, file paths, ARNs, MAC addresses are intentionally
// preserved (not scrubbed) so they are NOT checked here.
func (s *ScrubbingService) ValidateNoLeakage(text string) (bool, []string) {
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
