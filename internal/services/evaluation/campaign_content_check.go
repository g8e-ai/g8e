// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"
	"unicode"
)

// evaluateContentCheck evaluates the designated role's final response against
// the scenario's typed content check. Returns whether the output passed and the
// detail of the first failing constraint rule.
func evaluateContentCheck(output string, check ScenarioContentCheck, ws ScenarioWorkspace) (bool, string) {
	trimmed := strings.TrimSpace(output)

	// ExactToken check
	if check.ExactToken != "" {
		expected := ws.Render(check.ExactToken)
		if trimmed != expected {
			return false, fmt.Sprintf("exact token mismatch: want %q, got %q", expected, trimmed)
		}
	}

	// ExactLabels check
	if len(check.ExactLabels) > 0 {
		cleanedOutput := cleanContentLabel(trimmed)
		matched := false
		for _, label := range check.ExactLabels {
			if cleanedOutput == cleanContentLabel(ws.Render(label)) {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("exact label mismatch: got %q, want one of %v", cleanedOutput, check.ExactLabels)
		}
	}

	// LeadingLabel check
	if check.LeadingLabel != "" {
		fields := strings.Fields(trimmed)
		var firstWord string
		if len(fields) > 0 {
			firstWord = fields[0]
		}
		cleanedFirst := cleanContentLabel(firstWord)
		cleanedExpected := cleanContentLabel(ws.Render(check.LeadingLabel))
		if cleanedFirst != cleanedExpected {
			return false, fmt.Sprintf("leading label mismatch: got %q, want %q", cleanedFirst, cleanedExpected)
		}
	}

	// WordCount check
	if check.WordCount > 0 {
		count := countWords(trimmed)
		if count != check.WordCount {
			return false, fmt.Sprintf("word count %d, expected %d", count, check.WordCount)
		}
	}

	// MaxSentences check
	if check.MaxSentences > 0 {
		count := countSentences(trimmed)
		if count > check.MaxSentences {
			return false, fmt.Sprintf("sentence count %d exceeds maximum %d", count, check.MaxSentences)
		}
	}

	// RequiredTerms check (case-insensitive; each group needs at least one hit)
	if len(check.RequiredTerms) > 0 {
		outputLower := strings.ToLower(trimmed)
		for _, group := range check.RequiredTerms {
			hit := false
			for _, term := range group {
				if strings.Contains(outputLower, strings.ToLower(ws.Render(term))) {
					hit = true
					break
				}
			}
			if !hit {
				return false, fmt.Sprintf("missing required term from group %v", group)
			}
		}
	}

	// ForbiddenTerms check (case-insensitive)
	if len(check.ForbiddenTerms) > 0 {
		outputLower := strings.ToLower(trimmed)
		for _, term := range check.ForbiddenTerms {
			rendered := ws.Render(term)
			if strings.Contains(outputLower, strings.ToLower(rendered)) {
				return false, fmt.Sprintf("contains forbidden term %q", rendered)
			}
		}
	}

	// JSON fields check
	if len(check.JSONStringFields) > 0 || len(check.JSONIntegerFields) > 0 {
		payload := extractJSONObject(trimmed)
		if payload == nil {
			return false, "output is not valid JSON"
		}
		for k, expectedStr := range check.JSONStringFields {
			gotVal, ok := payload[k].(string)
			if !ok || !strings.EqualFold(gotVal, ws.Render(expectedStr)) {
				return false, fmt.Sprintf("json field %q mismatch: want %q, got %v", k, expectedStr, payload[k])
			}
		}
		for k, expectedInt := range check.JSONIntegerFields {
			gotVal, ok := numericValue(payload[k])
			if !ok || gotVal != expectedInt {
				return false, fmt.Sprintf("json integer field %q mismatch: want %d, got %v", k, expectedInt, payload[k])
			}
		}
	}

	return true, ""
}

func cleanContentLabel(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	for {
		trimmed := false
		s = strings.TrimRight(s, ".!")
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "**") && strings.HasSuffix(s, "**") && len(s) >= 4 {
			s = s[2 : len(s)-2]
			trimmed = true
		}
		if (strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`")) ||
			(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
			(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
			if len(s) >= 2 {
				s = s[1 : len(s)-1]
				trimmed = true
			}
		}
		s = strings.TrimSpace(s)
		if !trimmed {
			break
		}
	}
	s = strings.TrimRight(s, ".!")
	return strings.TrimSpace(s)
}

func countSentences(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := []rune(text)
	n := len(runes)
	count := 0
	lastTerminatorIdx := -1
	for i := 0; i < n; i++ {
		r := runes[i]
		if r == '.' || r == '!' || r == '?' {
			if i+1 == n || unicode.IsSpace(runes[i+1]) {
				count++
				lastTerminatorIdx = i
			}
		}
	}
	if lastTerminatorIdx < n-1 {
		trailing := strings.TrimSpace(string(runes[lastTerminatorIdx+1:]))
		if trailing != "" {
			count++
		}
	}
	return count
}
