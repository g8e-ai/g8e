// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// L1Doctrine provides L1 (Technical Bedrock) validation.
// L1 is the foundational hard gate that enforces forbidden patterns,
// blacklist/whitelist rules, and intent validation.
// It implements doctrine validation using protobuf field options.
// It checks the (g8e.common.v1).forbidden_patterns extension on string fields.
// It also performs MITRE-based threat detection on command payloads.
type L1Doctrine struct {
	inputThreatDetectors  []ThreatDetector
	doctrineBundleHash    string
	doctrineBundleVersion string
}

const builtInDoctrineVersion = "g8e-l1-doctrine-v1"

// NewL1Doctrine creates a new protobuf-based doctrine validator.
func NewL1Doctrine() *L1Doctrine {
	d := &L1Doctrine{}
	d.initializeInputThreatDetectors()
	d.setDoctrineBundleIdentity(nil, nil)
	return d
}

func (v *L1Doctrine) DoctrineBundleHash() string {
	return v.doctrineBundleHash
}

func (v *L1Doctrine) DoctrineBundleVersion() string {
	return v.doctrineBundleVersion
}

func appendDoctrineIdentityValue(payload []byte, value string) []byte {
	payload = binary.BigEndian.AppendUint64(payload, uint64(len(value)))
	return append(payload, value...)
}

func (v *L1Doctrine) setDoctrineBundleIdentity(fileMaterials [][]byte, fileVersions []string) {
	payload := appendDoctrineIdentityValue(nil, builtInDoctrineVersion)
	for _, detector := range v.inputThreatDetectors {
		inputDetector, ok := detector.(*InputThreatDetector)
		if !ok {
			payload = appendDoctrineIdentityValue(payload, detector.Name())
			continue
		}
		values := []string{
			inputDetector.name,
			inputDetector.pattern.String(),
			string(inputDetector.category),
			string(inputDetector.severity),
			strconv.FormatFloat(inputDetector.confidence, 'g', -1, 64),
			inputDetector.mitreAttack,
			inputDetector.mitreTactic,
			inputDetector.recommendation,
			strconv.FormatBool(inputDetector.blockRecommended),
			inputDetector.source,
		}
		for _, value := range values {
			payload = appendDoctrineIdentityValue(payload, value)
		}
		for _, values := range [][]string{inputDetector.ksiIDs, inputDetector.controlIDs, inputDetector.overlayIDs} {
			canonicalValues := append([]string(nil), values...)
			sort.Strings(canonicalValues)
			payload = binary.BigEndian.AppendUint64(payload, uint64(len(canonicalValues)))
			for _, value := range canonicalValues {
				payload = appendDoctrineIdentityValue(payload, value)
			}
		}
	}
	for _, material := range fileMaterials {
		payload = binary.BigEndian.AppendUint64(payload, uint64(len(material)))
		payload = append(payload, material...)
	}
	digest := sha256.Sum256(payload)
	v.doctrineBundleHash = hex.EncodeToString(digest[:])
	v.doctrineBundleVersion = builtInDoctrineVersion
	if len(fileVersions) > 0 {
		v.doctrineBundleVersion += "+" + strings.Join(fileVersions, "+")
	}
}

// doctrineFile represents the JSON schema of a doctrine file.
type doctrineFile struct {
	Source      string          `json:"source"`
	Version     string          `json:"version"`
	LastUpdated string          `json:"last_updated,omitempty"`
	License     string          `json:"license,omitempty"`
	Doctrines   []doctrineEntry `json:"doctrines"`
}

// doctrineEntry represents a single doctrine rule in a doctrine JSON file.
type doctrineEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Pattern     string   `json:"pattern"`
	MitreAttack string   `json:"mitre_attack"`
	MitreTactic string   `json:"mitre_tactic"`
	Confidence  float64  `json:"confidence"`
	Enabled     bool     `json:"enabled"`
	KSIIDs      []string `json:"ksi_ids,omitempty"`
	ControlIDs  []string `json:"control_ids,omitempty"`
	OverlayIDs  []string `json:"overlay_ids,omitempty"`
}

// NewL1DoctrineFromDir creates a doctrine validator that combines the hardcoded
// MITRE threat detectors with file-loaded doctrine patterns from doctrineDir.
// If doctrineDir is empty, it falls back to NewL1Doctrine() (backward compatible).
func NewL1DoctrineFromDir(doctrineDir string) (*L1Doctrine, error) {
	d := NewL1Doctrine()

	if doctrineDir == "" {
		return d, nil
	}

	entries, err := os.ReadDir(doctrineDir)
	if err != nil {
		return nil, fmt.Errorf("governance: read doctrine dir %s: %w", doctrineDir, err)
	}

	var loadedCount int
	var fileMaterials [][]byte
	var fileVersions []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(doctrineDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("governance: read doctrine file %s: %w", entry.Name(), err)
		}

		var df doctrineFile
		if err := json.Unmarshal(data, &df); err != nil {
			return nil, fmt.Errorf("governance: parse doctrine file %s: %w", entry.Name(), err)
		}
		fileMaterials = append(fileMaterials, []byte(entry.Name()), data)
		fileVersions = append(fileVersions, df.Source+"@"+df.Version)

		for _, de := range df.Doctrines {
			if !de.Enabled {
				continue
			}

			pattern, err := regexp.Compile(de.Pattern)
			if err != nil {
				return nil, fmt.Errorf("governance: compile pattern for %s in %s: %w", de.ID, entry.Name(), err)
			}

			category := parseThreatCategory(de.Category)
			if category == ThreatCategoryCustom {
				slog.Warn("governance: unknown doctrine category mapped to custom",
					"source", df.Source, "id", de.ID, "category", de.Category)
			}

			severity := parseThreatSeverity(de.Severity)
			blockRecommended := severity == ThreatSeverityCritical || severity == ThreatSeverityHigh

			detector := &InputThreatDetector{
				name:             de.ID,
				pattern:          pattern,
				category:         category,
				severity:         severity,
				confidence:       de.Confidence,
				mitreAttack:      de.MitreAttack,
				mitreTactic:      de.MitreTactic,
				blockRecommended: blockRecommended,
				source:           df.Source,
				ksiIDs:           de.KSIIDs,
				controlIDs:       de.ControlIDs,
				overlayIDs:       de.OverlayIDs,
			}
			d.inputThreatDetectors = append(d.inputThreatDetectors, detector)
			loadedCount++
		}
	}

	d.setDoctrineBundleIdentity(fileMaterials, fileVersions)
	slog.Info("governance: doctrine files loaded",
		"dir", doctrineDir, "detectors_loaded", loadedCount)

	return d, nil
}

// parseThreatCategory maps a doctrine JSON category string to a ThreatCategory.
// Unknown categories map to ThreatCategoryCustom.
func parseThreatCategory(s string) ThreatCategory {
	switch s {
	case "reverse_shell":
		return ThreatCategoryReverseShell
	case "privilege_escalation":
		return ThreatCategoryPrivilegeEsc
	case "credential_access":
		return ThreatCategoryCredentialAccess
	case "data_exfiltration":
		return ThreatCategoryExfiltration
	case "cryptominer":
		return ThreatCategoryCryptominer
	case "persistence":
		return ThreatCategoryPersistence
	case "lateral_movement":
		return ThreatCategoryLateralMovement
	case "defense_evasion":
		return ThreatCategoryDefenseEvasion
	case "reconnaissance":
		return ThreatCategoryReconnaissance
	case "resource_hijacking":
		return ThreatCategoryResourceHijacking
	case "destructive":
		return ThreatCategoryDestructive
	case "system_tampering":
		return ThreatCategorySystemTampering
	case "security_bypass":
		return ThreatCategorySecurityBypass
	case "malware_deployment":
		return ThreatCategoryMalwareDeployment
	case "data_destruction":
		return ThreatCategoryDataDestruction
	case "network_manipulation":
		return ThreatCategoryNetworkManipulation
	default:
		return ThreatCategoryCustom
	}
}

// parseThreatSeverity maps a doctrine JSON severity string to a ThreatSeverity.
// Unknown severity strings default to ThreatSeverityMedium.
func parseThreatSeverity(s string) ThreatSeverity {
	switch ThreatSeverity(s) {
	case ThreatSeverityCritical:
		return ThreatSeverityCritical
	case ThreatSeverityHigh:
		return ThreatSeverityHigh
	case ThreatSeverityMedium:
		return ThreatSeverityMedium
	case ThreatSeverityLow:
		return ThreatSeverityLow
	case ThreatSeverityInfo:
		return ThreatSeverityInfo
	default:
		return ThreatSeverityMedium
	}
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
			if err != nil {
				violations = append(violations, fmt.Sprintf("field %s has invalid forbidden pattern %s: %v", fd.Name(), p, err))
				continue
			}
			if matched {
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
	ThreatCategoryCustom              ThreatCategory = "custom"
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
	KSIIDs           []string       `json:"ksi_ids,omitempty"`
	ControlIDs       []string       `json:"control_ids,omitempty"`
	OverlayIDs       []string       `json:"overlay_ids,omitempty"`
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
	source           string
	ksiIDs           []string
	controlIDs       []string
	overlayIDs       []string
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
			Source:           d.source,
			KSIIDs:           d.ksiIDs,
			ControlIDs:       d.controlIDs,
			OverlayIDs:       d.overlayIDs,
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
		&InputThreatDetector{
			name:             "cred_leak_password_assignment",
			pattern:          regexp.MustCompile(`(?i)\bpassword\s*[=:]\s*\S+`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1552",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Credential leak — password value in field data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_leak_api_key_assignment",
			pattern:          regexp.MustCompile(`(?i)\bapi[_-]?key\s*[=:]\s*\S+`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1552",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Credential leak — API key value in field data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_leak_secret_assignment",
			pattern:          regexp.MustCompile(`(?i)\bsecret\s*[=:]\s*\S+`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityHigh,
			confidence:       0.80,
			mitreAttack:      "T1552",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Credential leak — secret value in field data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_leak_token_assignment",
			pattern:          regexp.MustCompile(`(?i)\b(access_token|refresh_token|auth_token|bearer_token|session_token|api_token)\s*[=:]\s*\S+`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityHigh,
			confidence:       0.85,
			mitreAttack:      "T1552",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Credential leak — authentication token value in field data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "cred_leak_private_key_block",
			pattern:          regexp.MustCompile(`(?i)-----BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`),
			category:         ThreatCategoryCredentialAccess,
			severity:         ThreatSeverityCritical,
			confidence:       0.99,
			mitreAttack:      "T1552.004",
			mitreTactic:      "Credential Access",
			recommendation:   "BLOCK: Credential leak — private key material in field data",
			blockRecommended: true,
		},
		&InputThreatDetector{
			name:             "privesc_su_command",
			pattern:          regexp.MustCompile(`(?i)\bsu\s+(-\w|--login|--command)\b`),
			category:         ThreatCategoryPrivilegeEsc,
			severity:         ThreatSeverityHigh,
			confidence:       0.80,
			mitreAttack:      "T1548.003",
			mitreTactic:      "Privilege Escalation",
			recommendation:   "BLOCK: Privilege escalation via su command",
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
	// Parse the JSON arguments using json.RawMessage to avoid untyped maps
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(argumentsJSON), &raw); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrMCPUnmarshalArguments, err)
	}

	// Recursively analyze all string values in the arguments with depth limit
	const maxDepth = 50
	signals := []ThreatSignal{}
	if err := v.analyzeJSONRecursive(raw, "", &signals, 0, maxDepth); err != nil {
		return nil, err
	}

	return signals, nil
}

// analyzeJSONRecursive recursively traverses JSON raw message and detects threats in string fields.
// Returns ErrGovernanceJSONDepthExceeded if the recursion depth exceeds maxDepth.
func (v *L1Doctrine) analyzeJSONRecursive(raw json.RawMessage, path string, signals *[]ThreatSignal, currentDepth int, maxDepth int) error {
	if currentDepth > maxDepth {
		return fmt.Errorf("%w: maxDepth=%d, path=%s", constants.ErrGovernanceJSONDepthExceeded, maxDepth, path)
	}

	// Try to parse as string first
	var strVal string
	if err := json.Unmarshal(raw, &strVal); err == nil {
		// Analyze string values for threat patterns
		for _, detector := range v.inputThreatDetectors {
			detected := detector.Detect(strVal)
			for _, sig := range detected {
				// Add path context to the signal
				sig.Context = path
				*signals = append(*signals, sig)
			}
		}
		return nil
	}

	// Try to parse as object
	var objVal map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objVal); err == nil {
		// Recursively analyze object fields
		for key, val := range objVal {
			newPath := path
			if newPath != "" {
				newPath += "."
			}
			newPath += key
			if err := v.analyzeJSONRecursive(val, newPath, signals, currentDepth+1, maxDepth); err != nil {
				return err
			}
		}
		return nil
	}

	// Try to parse as array
	var arrVal []json.RawMessage
	if err := json.Unmarshal(raw, &arrVal); err == nil {
		// Recursively analyze array elements
		for i, val := range arrVal {
			newPath := path
			if newPath != "" {
				newPath += "["
				newPath += fmt.Sprintf("%d", i)
				newPath += "]"
			}
			if err := v.analyzeJSONRecursive(val, newPath, signals, currentDepth+1, maxDepth); err != nil {
				return err
			}
		}
		return nil
	}

	// Numbers, booleans, and null are ignored - they don't contain string threats
	return nil
}
