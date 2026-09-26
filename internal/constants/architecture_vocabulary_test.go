// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	literalEventTypeCast  = regexp.MustCompile(`constants\.EventType\("g8e\.v1\.`)
	literalActionTypeCast = regexp.MustCompile(`constants\.ActionType\("[^"]+"\)`)
	g8eWireLiteral        = regexp.MustCompile(`"g8e\.v1\.[^"]+"`)
)

var allowedG8eWireLiteralFiles = map[string]bool{
	"senders.go": true, // MessageSender persistence identifiers, not governed events.
}

var bannedVocabularyNeedles = []string{
	"G8eActionType",
	"action_status",
	"thinking_action_type",
	"EXECUTE_STATUS_UPDATE",
	"INFERENCE_PROGRESS",
	"MapActionTypeToEventType",
	"MapEventTypeToResultActionType",
	"map_event_type_to_action_type",
	"action_type_mappings",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found")
		dir = parent
	}
}

func walkProductionGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "vendor", "testdata", "docs", ".local.dev":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") ||
			strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		if allowedG8eWireLiteralFiles[filepath.Base(path)] {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+"internal"+string(filepath.Separator)+"tools"+string(filepath.Separator)+"constgen"+string(filepath.Separator)) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func walkProductionPythonFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "vendor", "testdata", "docs", ".local.dev", ".venv", "venv", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") || strings.HasSuffix(path, "_test.py") {
			return nil
		}
		if strings.HasSuffix(path, string(filepath.Separator)+"constants"+string(filepath.Separator)+"__init__.py") {
			return nil
		}
		if strings.Contains(path, "architecture"+string(filepath.Separator)+"test_event_vocabulary.py") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func walkProductionJSFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "constants", "node_modules", ".local.dev", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".test.js") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func scanFilePatterns(path string, patterns []*regexp.Regexp) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var violations []string
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, pattern := range patterns {
			if pattern.MatchString(line) {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", path, lineNo, strings.TrimSpace(line)))
			}
		}
	}
	return violations, scanner.Err()
}

func scanFileSubstrings(path string, needles []string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var violations []string
	for lineNo, line := range strings.Split(string(data), "\n") {
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", path, lineNo+1, strings.TrimSpace(line)))
			}
		}
	}
	return violations, nil
}

// W12: production Go must not introduce literal event/action casts or wire strings.
func TestArchitecture_NoLiteralEventOrActionTypeCastsInProductionGo(t *testing.T) {
	root := repoRoot(t)
	files, err := walkProductionGoFiles(filepath.Join(root, "internal"))
	require.NoError(t, err)

	var violations []string
	for _, path := range files {
		found, err := scanFilePatterns(path, []*regexp.Regexp{literalEventTypeCast, literalActionTypeCast})
		require.NoError(t, err)
		violations = append(violations, found...)
	}
	assert.Empty(t, violations, "use generated EventType/ActionType constants instead of string literal casts")
}

func TestArchitecture_NoWireEventLiteralsInProductionGo(t *testing.T) {
	root := repoRoot(t)
	files, err := walkProductionGoFiles(filepath.Join(root, "internal"))
	require.NoError(t, err)

	var violations []string
	for _, path := range files {
		found, err := scanFilePatterns(path, []*regexp.Regexp{g8eWireLiteral})
		require.NoError(t, err)
		violations = append(violations, found...)
	}
	assert.Empty(t, violations, "use generated EventType constants instead of g8e.v1 wire literals")
}

func TestArchitecture_NoBannedMapperNamesInProductionGo(t *testing.T) {
	root := repoRoot(t)
	files, err := walkProductionGoFiles(filepath.Join(root, "internal"))
	require.NoError(t, err)

	needles := []string{
		"MapActionTypeToEventType",
		"MapEventTypeToResultActionType",
		"map_event_type_to_action_type",
		"action_type_mappings",
	}
	var violations []string
	for _, path := range files {
		found, err := scanFileSubstrings(path, needles)
		require.NoError(t, err)
		violations = append(violations, found...)
	}
	assert.Empty(t, violations, "deleted event/action mapper names must not reappear")
}

func TestArchitecture_NoBannedVocabularyInProductionCode(t *testing.T) {
	root := repoRoot(t)

	var violations []string

	goFiles, err := walkProductionGoFiles(filepath.Join(root, "internal"))
	require.NoError(t, err)
	for _, path := range goFiles {
		found, err := scanFileSubstrings(path, bannedVocabularyNeedles)
		require.NoError(t, err)
		violations = append(violations, found...)
	}

	pyFiles, err := walkProductionPythonFiles(filepath.Join(root, "ensemble", "app"))
	require.NoError(t, err)
	for _, path := range pyFiles {
		found, err := scanFileSubstrings(path, bannedVocabularyNeedles)
		require.NoError(t, err)
		violations = append(violations, found...)
	}

	jsFiles, err := walkProductionJSFiles(filepath.Join(root, "dashboard", "public", "js"))
	require.NoError(t, err)
	for _, path := range jsFiles {
		found, err := scanFileSubstrings(path, bannedVocabularyNeedles)
		require.NoError(t, err)
		violations = append(violations, found...)
	}

	assert.Empty(t, violations, "banned vocabulary must not appear in production code")
}
