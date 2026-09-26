// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// constgen validates protocol/constants/events.json and can emit generated
// constant files. Phase 1 ships validation only; code generation follows W2.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type registryFile struct {
	Events map[string]eventEntry `json:"events"`
}

type eventEntry struct {
	GoConst               string   `json:"_go_const"`
	Value                 string   `json:"value"`
	Kind                  string   `json:"kind"`
	Transport             []string `json:"transport"`
	Producers             []string `json:"producers"`
	Persistence           string   `json:"persistence"`
	Governance            *struct {
		ActionType string `json:"action_type"`
		Payload    string `json:"payload"`
	} `json:"governance"`
	GrammarAllowlistOwner string `json:"grammar_allowlist_owner"`
	Reserved              bool   `json:"reserved"`
}

var (
	wirePattern = regexp.MustCompile(`^g8e\.v1\.[a-z]+(?:\.[a-z0-9_]+)+$`)
	goConstPat  = regexp.MustCompile(`^Event[A-Z][A-Za-z0-9]*$`)

	allowedTerminals = map[string]struct{}{
		"requested": {}, "started": {}, "received": {}, "completed": {}, "failed": {},
		"cancelled": {}, "timeout": {}, "recorded": {}, "created": {}, "updated": {},
		"deleted": {}, "granted": {}, "rejected": {}, "denied": {}, "revoked": {},
		"expired": {}, "acknowledged": {}, "sent": {}, "missed": {}, "bound": {},
		"unbound": {}, "opened": {}, "closed": {}, "established": {}, "checkpointed": {},
		"exported": {}, "published": {}, "rotated": {}, "available": {}, "invoked": {},
		"reached": {}, "detected": {}, "resolved": {}, "appended": {}, "truncated": {},
		"retry": {}, "heartbeat": {},
	}

	allowedKinds = map[string]struct{}{
		"request": {}, "outcome": {}, "fact": {}, "stream": {},
	}

	allowedTransport = map[string]struct{}{
		"governed": {}, "pubsub": {}, "sse": {},
	}

	allowedProducers = map[string]struct{}{
		"gateway": {}, "operator": {}, "ensemble": {},
		"dashboard": {}, "cli": {}, "mcp": {},
	}

	allowedPersistence = map[string]struct{}{
		"operator.audit_log":    {},
		"gateway.audit_log":     {},
		"gateway.sse_store":     {},
		"gateway.operator_docs": {},
		"gateway.docstore":      {},
		"ephemeral":             {},
	}
)

func main() {
	checkOnly := flag.Bool("check", false, "validate registry and verify generated files match committed output")
	write := flag.Bool("write", false, "validate registry and write generated constant files")
	strictGrammar := flag.Bool("strict-grammar", false, "also enforce closed grammar terminals (W14)")
	flag.Parse()
	if *checkOnly && *write {
		fatal("use only one of -check or -write")
	}
	if !*checkOnly && !*write {
		*checkOnly = true
	}

	root, err := findRepoRoot()
	if err != nil {
		fatal("%v", err)
	}

	eventsPath := filepath.Join(root, "protocol/constants/events.json")
	statusPath := filepath.Join(root, "protocol/constants/status.json")

	events, err := loadRegistry(eventsPath)
	if err != nil {
		fatal("%v", err)
	}
	actionTypeMeta, err := loadActionTypeMeta(statusPath)
	if err != nil {
		fatal("%v", err)
	}
	actionTypes := actionTypeValues(actionTypeMeta)

	if err := validateRegistry(events, actionTypes, *strictGrammar); err != nil {
		fatal("%v", err)
	}

	out, err := generateAll(root, events, actionTypeMeta)
	if err != nil {
		fatal("%v", err)
	}

	if *write {
		if err := writeGenerated(root, out); err != nil {
			fatal("%v", err)
		}
		fmt.Println("generated constants from protocol/constants/events.json")
		return
	}

	if err := verifyGenerated(root, out); err != nil {
		fatal("%v", err)
	}
}

func actionTypeValues(actionTypes map[string]actionTypeMeta) map[string]struct{} {
	out := make(map[string]struct{}, len(actionTypes))
	for _, meta := range actionTypes {
		out[meta.Value] = struct{}{}
	}
	return out
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "protocol/constants/events.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find protocol/constants/events.json from %s", dir)
		}
		dir = parent
	}
}

func loadRegistry(path string) (registryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return registryFile{}, err
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		return registryFile{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return reg, nil
}

func validateRegistry(reg registryFile, actionTypes map[string]struct{}, strictGrammar bool) error {
	if len(reg.Events) == 0 {
		return fmt.Errorf("events registry is empty")
	}

	var errs []string
	seenValue := map[string]string{}
	seenGoConst := map[string]string{}

	keys := make([]string, 0, len(reg.Events))
	for k := range reg.Events {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := reg.Events[key]
		if entry.GoConst == "" {
			errs = append(errs, fmt.Sprintf("%s: missing _go_const", key))
		} else if !goConstPat.MatchString(entry.GoConst) {
			errs = append(errs, fmt.Sprintf("%s: invalid _go_const %q", key, entry.GoConst))
		} else if owner, exists := seenGoConst[entry.GoConst]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate _go_const %q (also %s)", key, entry.GoConst, owner))
		} else {
			seenGoConst[entry.GoConst] = key
		}

		if entry.Value == "" {
			errs = append(errs, fmt.Sprintf("%s: missing value", key))
			continue
		}
		if !wirePattern.MatchString(entry.Value) {
			errs = append(errs, fmt.Sprintf("%s: invalid wire value %q", key, entry.Value))
		}
		if owner, exists := seenValue[entry.Value]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate wire value %q (also %s)", key, entry.Value, owner))
		} else {
			seenValue[entry.Value] = key
		}

		if entry.Kind != "" {
			if _, ok := allowedKinds[entry.Kind]; !ok {
				errs = append(errs, fmt.Sprintf("%s: unknown kind %q", key, entry.Kind))
			}
		}

		for _, t := range entry.Transport {
			if _, ok := allowedTransport[t]; !ok {
				errs = append(errs, fmt.Sprintf("%s: unknown transport %q", key, t))
			}
		}

		if entry.Governance != nil {
			if entry.Kind != "request" {
				errs = append(errs, fmt.Sprintf("%s: governance requires kind=request", key))
			}
			if !strings.HasSuffix(entry.Value, ".requested") {
				errs = append(errs, fmt.Sprintf("%s: governance requires .requested wire value", key))
			}
			if entry.Governance.ActionType == "" || entry.Governance.Payload == "" {
				errs = append(errs, fmt.Sprintf("%s: governance requires action_type and payload", key))
			} else if _, ok := actionTypes[entry.Governance.ActionType]; !ok {
				errs = append(errs, fmt.Sprintf("%s: unknown governance.action_type %q", key, entry.Governance.ActionType))
			}
			hasGoverned := false
			for _, t := range entry.Transport {
				if t == "governed" {
					hasGoverned = true
					break
				}
			}
			if !hasGoverned {
				errs = append(errs, fmt.Sprintf("%s: governed request missing transport governed", key))
			}
		}

		if strictGrammar && entry.GrammarAllowlistOwner == "" {
			if err := checkGrammar(entry.Value); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", key, err))
			}
		}

		if entry.Kind == "stream" && entry.Transport != nil {
			for _, t := range entry.Transport {
				if t != "sse" {
					errs = append(errs, fmt.Sprintf("%s: stream events must use sse transport only", key))
				}
			}
		}

		if len(entry.Producers) == 0 {
			errs = append(errs, fmt.Sprintf("%s: missing producers", key))
		}
		for _, producer := range entry.Producers {
			if _, ok := allowedProducers[producer]; !ok {
				errs = append(errs, fmt.Sprintf("%s: unknown producer %q", key, producer))
			}
		}

		if entry.Persistence == "" {
			errs = append(errs, fmt.Sprintf("%s: missing persistence", key))
		} else if _, ok := allowedPersistence[entry.Persistence]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown persistence %q", key, entry.Persistence))
		}

		if entry.Kind == "stream" && entry.Persistence != "ephemeral" {
			errs = append(errs, fmt.Sprintf("%s: stream events must use ephemeral persistence", key))
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("event registry validation failed (%d issues):\n  %s", len(errs), strings.Join(errs, "\n  "))
	}
	return nil
}

func checkGrammar(value string) error {
	parts := strings.Split(strings.TrimPrefix(value, "g8e.v1."), ".")
	if len(parts) < 3 {
		return fmt.Errorf("grammar: expected g8e.v1.<domain>.<entity>.<terminal>")
	}
	terminal := parts[len(parts)-1]
	if _, ok := allowedTerminals[terminal]; ok {
		return nil
	}
	// Multi-word terminals such as progress.updated are allowlisted separately.
	knownViolations := []string{
		"g8e.v1.ai.llm.chat.filter.event",
		"g8e.v1.platform.notification",
		"g8e.v1.platform.auth.info",
		"g8e.v1.operator.command.execution",
		"g8e.v1.operator.command.result",
		"g8e.v1.ai.llm.chat.stop.show",
		"g8e.v1.ai.llm.chat.stop.hide",
	}
	for _, v := range knownViolations {
		if value == v {
			return nil
		}
	}
	if strings.Contains(value, ".status.updated.") {
		return nil
	}
	if strings.Contains(value, ".stream.") || strings.Contains(value, ".chunk.") ||
		strings.Contains(value, ".delta.") || strings.Contains(value, ".keepalive.") ||
		strings.Contains(value, ".thinking.") {
		return nil
	}
	if strings.HasPrefix(value, "g8e.v1.source.") {
		return nil
	}
	return fmt.Errorf("grammar: terminal %q not in closed list for %q", terminal, value)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "constgen: "+format+"\n", args...)
	os.Exit(1)
}
