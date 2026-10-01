// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"regexp"
	"slices"
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
		cleanedFirst := leadingWord(trimmed)
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

	// Interrogation protocol check
	if check.Interrogation {
		if ok, detail := evaluateInterrogation(trimmed); !ok {
			return false, detail
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

var (
	interrogationBlock    = regexp.MustCompile(`(?s)^<interrogation>\s*(.*?)\s*</interrogation>$`)
	interrogationQuestion = regexp.MustCompile(`^\d+[.)]\s*(\S.*\?)$`)
)

// binaryOpeners are the words a yes/no question starts with. A question that
// opens with anything else ("which", "what", "how", ...) asks for an answer
// other than yes or no.
var binaryOpeners = []string{
	"is", "are", "was", "were", "do", "does", "did", "can", "could", "has", "have", "had",
	"will", "would", "should", "shall", "may", "might", "must",
}

// evaluateInterrogation checks the Interrogation Protocol: the entire answer is
// one <interrogation> block of exactly three numbered yes/no questions.
func evaluateInterrogation(output string) (bool, string) {
	match := interrogationBlock.FindStringSubmatch(output)
	if match == nil {
		return false, "the answer is not a single <interrogation> block"
	}
	var lines []string
	for _, line := range strings.Split(match[1], "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 3 {
		return false, fmt.Sprintf("the interrogation block has %d questions, want exactly 3", len(lines))
	}
	for _, line := range lines {
		question := interrogationQuestion.FindStringSubmatch(line)
		if question == nil {
			return false, fmt.Sprintf("interrogation line %q is not a numbered question", line)
		}
		words := strings.Fields(strings.ToLower(question[1]))
		if !slices.Contains(binaryOpeners, strings.Trim(words[0], ",")) || slices.Contains(words, "or") {
			return false, fmt.Sprintf("interrogation question %q is not strictly yes/no", question[1])
		}
	}
	return true, ""
}

// leadingWord returns the lowercased first alphanumeric word of text, ignoring
// leading emphasis and quote marks, so "Yes, they contradict" and "**Yes**."
// both lead with "yes" while "Yesterday" does not. A LeadingLabel is therefore
// a single alphanumeric word.
func leadingWord(text string) string {
	cleaned := strings.TrimLeft(strings.TrimSpace(text), "*_`\"' ")
	end := len(cleaned)
	for i, r := range cleaned {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			end = i
			break
		}
	}
	return strings.ToLower(cleaned[:end])
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
