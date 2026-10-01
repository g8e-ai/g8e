// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// repoRoot is the repository root relative to this package's directory.
const repoRoot = "../../.."

func testAssertions() *compliancev1.ControlAssertionCatalog {
	return &compliancev1.ControlAssertionCatalog{
		Assertions: []*compliancev1.ControlAssertionDefinition{
			{AssertionId: "ASSERT-1", AssertionVersion: "1.0.0"},
			{AssertionId: "ASSERT-2", AssertionVersion: "2.0.0"},
		},
	}
}

func testFrameworks() *compliancev1.FrameworkCatalog {
	return &compliancev1.FrameworkCatalog{
		Frameworks: []*compliancev1.FrameworkDefinition{
			{FrameworkId: "fw-a", Controls: []*compliancev1.FrameworkControlDefinition{{ControlId: "AU-2"}, {ControlId: "AU-6"}}},
			{FrameworkId: "fw-b", Controls: []*compliancev1.FrameworkControlDefinition{{ControlId: "KSI-MLA-07"}}},
		},
	}
}

func writeDoctrineJSON(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doctrine.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestFrameworkControlExists_SearchesEveryFramework(t *testing.T) {
	frameworks := testFrameworks()

	tests := []struct {
		name      string
		controlID string
		want      bool
	}{
		{name: "control in first framework", controlID: "AU-2", want: true},
		{name: "control in later framework", controlID: "KSI-MLA-07", want: true},
		{name: "unknown control", controlID: "ZZ-99", want: false},
		{name: "empty control id", controlID: "", want: false},
		{name: "lookup is case sensitive", controlID: "au-2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, frameworkControlExists(frameworks, tt.controlID))
		})
	}
}

func TestFrameworkControlExists_FalseForCatalogWithoutFrameworks(t *testing.T) {
	assert.False(t, frameworkControlExists(&compliancev1.FrameworkCatalog{}, "AU-2"))
}

func TestValidateDoctrineFile_AcceptsResolvableReferences(t *testing.T) {
	path := writeDoctrineJSON(t, `{"doctrines":[
		{"id":"d1","ksi_ids":["KSI-MLA-07"],"control_ids":["AU-2","AU-6"],
		 "assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"},{"id":"ASSERT-2","version":"2.0.0"}]},
		{"id":"d2","assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"}]}
	]}`)

	require.NoError(t, validateDoctrineFile(path, testAssertions(), testFrameworks()))
}

func TestValidateDoctrineFile_AcceptsEmptyDoctrineList(t *testing.T) {
	path := writeDoctrineJSON(t, `{"doctrines":[]}`)

	require.NoError(t, validateDoctrineFile(path, testAssertions(), testFrameworks()))
}

func TestValidateDoctrineFile_RejectsDoctrinesThatFailReferenceChecks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "doctrine without assertion references",
			body: `{"doctrines":[{"id":"d1","assertion_refs":[]}]}`,
			want: `doctrine "d1" has no assertion references`,
		},
		{
			name: "doctrine with absent assertion_refs key",
			body: `{"doctrines":[{"id":"d1"}]}`,
			want: `doctrine "d1" has no assertion references`,
		},
		{
			name: "doctrine without id",
			body: `{"doctrines":[{"assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"}]}]}`,
			want: `doctrine "" has no assertion references`,
		},
		{
			name: "unknown KSI id",
			body: `{"doctrines":[{"id":"d1","ksi_ids":["KSI-NOPE-01"],"assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"}]}]}`,
			want: "doctrine d1 references unknown framework control KSI-NOPE-01",
		},
		{
			name: "unknown control id",
			body: `{"doctrines":[{"id":"d1","control_ids":["AU-2","ZZ-99"],"assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"}]}]}`,
			want: "doctrine d1 references unknown framework control ZZ-99",
		},
		{
			name: "unknown assertion id",
			body: `{"doctrines":[{"id":"d1","assertion_refs":[{"id":"ASSERT-404","version":"1.0.0"}]}]}`,
			want: "doctrine d1 references unknown assertion",
		},
		{
			name: "known assertion at an unpublished version",
			body: `{"doctrines":[{"id":"d1","assertion_refs":[{"id":"ASSERT-1","version":"9.9.9"}]}]}`,
			want: "doctrine d1 references unknown assertion",
		},
		{
			name: "null assertion reference",
			body: `{"doctrines":[{"id":"d1","assertion_refs":[null]}]}`,
			want: "doctrine d1 references unknown assertion",
		},
		{
			name: "later doctrine fails after earlier one passes",
			body: `{"doctrines":[
				{"id":"good","assertion_refs":[{"id":"ASSERT-1","version":"1.0.0"}]},
				{"id":"bad","assertion_refs":[{"id":"ASSERT-404","version":"1.0.0"}]}
			]}`,
			want: "doctrine bad references unknown assertion",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDoctrineFile(writeDoctrineJSON(t, tt.body), testAssertions(), testFrameworks())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "doctrine references: ")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestValidateDoctrineFile_ReportsMissingFileWithPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")

	err := validateDoctrineFile(path, testAssertions(), testFrameworks())

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "read "+path)
}

func TestValidateDoctrineFile_ReportsMalformedJSONWithPath(t *testing.T) {
	path := writeDoctrineJSON(t, `{"doctrines": [`)

	err := validateDoctrineFile(path, testAssertions(), testFrameworks())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse "+path)
}

func TestValidate_PassesFromRepositoryRootAgainstCommittedFedRAMPDoctrine(t *testing.T) {
	t.Chdir(repoRoot)

	require.NoError(t, validate())
}

func TestValidate_ReportsMissingDoctrineFileWhenRunOutsideRepositoryRoot(t *testing.T) {
	t.Chdir(t.TempDir())

	err := validate()

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), filepath.Join(constants.DemosDirname, constants.DemosOrgFedRAMP, constants.DemosDoctrineDir, constants.DemosFedRAMPDoctrineFile))
}

func TestValidateDoctrineFile_CommittedFedRAMPDoctrineResolvesAgainstCanonicalCatalogs(t *testing.T) {
	assertions, frameworks, _, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	path := filepath.Join(repoRoot, constants.DemosDirname, constants.DemosOrgFedRAMP, constants.DemosDoctrineDir, constants.DemosFedRAMPDoctrineFile)

	require.NoError(t, validateDoctrineFile(path, assertions, frameworks))
}
