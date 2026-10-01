// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func mustWorkspace(t *testing.T, workingDirectory, runID, attemptID string) ScenarioWorkspace {
	t.Helper()
	ws, err := NewScenarioWorkspace(workingDirectory, runID, attemptID)
	require.NoError(t, err)
	return ws
}

func TestNewScenarioWorkspace_RootIsNeutralAndDerivedFromRunAndAttempt(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/var/run/g8e", "run-1", "attempt-1")

	pattern := regexp.MustCompile(`^/var/run/g8e/` + constants.EvaluationWorkspaceDirname + `/` + constants.EvaluationWorkspacePrefix + `[0-9a-f]{16}$`)
	assert.Regexp(t, pattern, ws.Root)
	assert.Equal(t, "/var/run/g8e", ws.OperatorWorkingDirectory)

	// R4: nothing the model can read from the workspace path names the
	// evaluation, the run, or the attempt.
	lowered := strings.ToLower(ws.Root)
	for _, forbidden := range []string{"eval", "run-1", "attempt-1", "canary", "probe"} {
		assert.NotContains(t, lowered, forbidden)
	}
}

func TestNewScenarioWorkspace_IsDeterministicPerRunAndAttempt(t *testing.T) {
	t.Parallel()
	first := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	again := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	assert.Equal(t, first, again)

	tests := []struct {
		name      string
		runID     string
		attemptID string
	}{
		{name: "different attempt", runID: "run-1", attemptID: "attempt-2"},
		{name: "different run", runID: "run-2", attemptID: "attempt-1"},
		// The NUL separator keeps ("a","bc") and ("ab","c") from colliding.
		{name: "run and attempt boundary shifted", runID: "run-1a", attemptID: "ttempt-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			other := mustWorkspace(t, "/srv/op", tt.runID, tt.attemptID)
			assert.NotEqual(t, first.Root, other.Root)
		})
	}
}

func TestNewScenarioWorkspace_CleansTheOperatorWorkingDirectory(t *testing.T) {
	t.Parallel()
	clean := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	messy := mustWorkspace(t, "/srv//op/./sub/..//", "run-1", "attempt-1")
	assert.Equal(t, clean, messy)
}

func TestNewScenarioWorkspace_RejectsUnusableInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		workingDirectory string
		runID            string
		attemptID        string
		wantErr          error
	}{
		{name: "empty working directory", workingDirectory: "", runID: "r", attemptID: "a", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
		{name: "relative working directory", workingDirectory: "operator/home", runID: "r", attemptID: "a", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
		{name: "NUL in working directory", workingDirectory: "/srv/op\x00/x", runID: "r", attemptID: "a", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
		{name: "empty run id", workingDirectory: "/srv/op", runID: "", attemptID: "a", wantErr: constants.ErrMissingRequiredField},
		{name: "empty attempt id", workingDirectory: "/srv/op", runID: "r", attemptID: "", wantErr: constants.ErrMissingRequiredField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws, err := NewScenarioWorkspace(tt.workingDirectory, tt.runID, tt.attemptID)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, ScenarioWorkspace{}, ws)
		})
	}
}

func TestScenarioWorkspace_RenderReplacesEveryToken(t *testing.T) {
	t.Parallel()
	ws := ScenarioWorkspace{Root: "/srv/op/workspaces/ws-0123456789abcdef", OperatorWorkingDirectory: "/srv/op"}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no token is untouched", in: "Reply with exactly: READY", want: "Reply with exactly: READY"},
		{name: "single token", in: "Read {{workspace}}/config/a.env", want: "Read /srv/op/workspaces/ws-0123456789abcdef/config/a.env"},
		{name: "every occurrence", in: "{{workspace}} and {{workspace}}/x", want: "/srv/op/workspaces/ws-0123456789abcdef and /srv/op/workspaces/ws-0123456789abcdef/x"},
		{name: "near-miss token is untouched", in: "{{ workspace }} {workspace} {{Workspace}}", want: "{{ workspace }} {workspace} {{Workspace}}"},
		{name: "empty text", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ws.Render(tt.in))
		})
	}
}

func TestScenarioWorkspace_FilePath(t *testing.T) {
	t.Parallel()
	ws := ScenarioWorkspace{Root: "/srv/op/workspaces/ws-0123456789abcdef", OperatorWorkingDirectory: "/srv/op"}
	tests := []struct {
		name    string
		relPath string
		want    string
		wantErr bool
	}{
		{name: "top-level file", relPath: "a.txt", want: "/srv/op/workspaces/ws-0123456789abcdef/a.txt"},
		{name: "nested file", relPath: "logs/checkout.log", want: "/srv/op/workspaces/ws-0123456789abcdef/logs/checkout.log"},
		{name: "empty", relPath: "", wantErr: true},
		{name: "dot", relPath: ".", wantErr: true},
		{name: "absolute", relPath: "/etc/passwd", wantErr: true},
		{name: "parent escape", relPath: "../secret", wantErr: true},
		{name: "embedded parent escape", relPath: "a/../../secret", wantErr: true},
		{name: "unclean double slash", relPath: "a//b", wantErr: true},
		{name: "unclean trailing slash", relPath: "a/", wantErr: true},
		{name: "unclean current-dir segment", relPath: "./a", wantErr: true},
		{name: "NUL byte", relPath: "a\x00b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ws.FilePath(tt.relPath)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.True(t, ws.Contains(got), "a fixture path must be inside its own workspace")
		})
	}
}

func TestScenarioWorkspace_ResolveMatchesOperatorPathResolution(t *testing.T) {
	t.Parallel()
	ws := ScenarioWorkspace{Root: "/srv/op/workspaces/ws-0123456789abcdef", OperatorWorkingDirectory: "/srv/op"}
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "absolute is cleaned", value: "/srv/op//workspaces/./x", want: "/srv/op/workspaces/x"},
		{name: "relative resolves against the operator working directory", value: "workspaces/ws-0123456789abcdef/a.txt", want: "/srv/op/workspaces/ws-0123456789abcdef/a.txt"},
		{name: "relative dot-dot resolves out of the working directory", value: "../etc/passwd", want: "/srv/etc/passwd"},
		{name: "dot resolves to the working directory", value: ".", want: "/srv/op"},
		{name: "empty resolves to the working directory", value: "", want: "/srv/op"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ws.Resolve(tt.value))
		})
	}
}

func TestScenarioWorkspace_ContainsOnlyThePathsInsideTheRoot(t *testing.T) {
	t.Parallel()
	const root = "/srv/op/workspaces/ws-0123456789abcdef"
	ws := ScenarioWorkspace{Root: root, OperatorWorkingDirectory: "/srv/op"}
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "the root itself", value: root, want: true},
		{name: "root with trailing slash", value: root + "/", want: true},
		{name: "file under root", value: root + "/logs/a.log", want: true},
		{name: "relative path into the root", value: "workspaces/ws-0123456789abcdef/logs", want: true},
		{name: "sibling sharing a name prefix", value: root + "-other/a.log", want: false},
		{name: "sibling attempt workspace", value: "/srv/op/workspaces/ws-fedcba9876543210/a.log", want: false},
		{name: "the workspaces directory", value: "/srv/op/workspaces", want: false},
		{name: "the operator working directory", value: "/srv/op", want: false},
		{name: "dot-dot escape from inside the root", value: root + "/../ws-fedcba9876543210/a.log", want: false},
		{name: "dot-dot escape to a system file", value: root + "/../../../../etc/passwd", want: false},
		{name: "unrelated absolute path", value: "/etc/passwd", want: false},
		{name: "relative path outside the root", value: "config/app.env", want: false},
		{name: "empty", value: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ws.Contains(tt.value))
		})
	}
}

func TestScenarioWorkspace_ValidateBindsTheRootToTheRunAndAttempt(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")

	require.NoError(t, ws.Validate("run-1", "attempt-1"))

	tests := []struct {
		name      string
		workspace ScenarioWorkspace
		runID     string
		attemptID string
		wantErr   error
	}{
		{name: "another attempt of the same run", workspace: ws, runID: "run-1", attemptID: "attempt-2", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
		{name: "another run", workspace: ws, runID: "run-2", attemptID: "attempt-1", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
		{
			name:      "a root outside the derived location",
			workspace: ScenarioWorkspace{Root: "/srv/op/workspaces/ws-0000000000000000", OperatorWorkingDirectory: "/srv/op"},
			runID:     "run-1", attemptID: "attempt-1", wantErr: constants.ErrEvaluationWorkspaceUnavailable,
		},
		{
			name:      "a root under a different operator working directory",
			workspace: ScenarioWorkspace{Root: ws.Root, OperatorWorkingDirectory: "/other/op"},
			runID:     "run-1", attemptID: "attempt-1", wantErr: constants.ErrEvaluationWorkspaceUnavailable,
		},
		{
			name:      "a relative operator working directory",
			workspace: ScenarioWorkspace{Root: ws.Root, OperatorWorkingDirectory: "op"},
			runID:     "run-1", attemptID: "attempt-1", wantErr: constants.ErrEvaluationWorkspaceUnavailable,
		},
		{name: "empty run id", workspace: ws, runID: "", attemptID: "attempt-1", wantErr: constants.ErrMissingRequiredField},
		{name: "zero workspace", workspace: ScenarioWorkspace{}, runID: "run-1", attemptID: "attempt-1", wantErr: constants.ErrEvaluationWorkspaceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.workspace.Validate(tt.runID, tt.attemptID)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestMaterializeScenarioWorkspace_WritesEveryFileAtItsRenderedPath(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	writer := &recordingWorkspaceFileWriter{}
	target := Target{OperatorID: "op-1", SessionID: "sess-1"}
	files := []ScenarioWorkspaceFile{
		{Label: "log", RelPath: "logs/checkout.log", Content: "08:01 PAYMENT_TIMEOUT"},
		{Label: "decoy", RelPath: "logs/auth.log", Content: "08:02 AUTH_FAILURE", Decoy: true},
		{Label: "self-referencing", RelPath: "notes/where.txt", Content: "files live in {{workspace}}/logs"},
	}

	require.NoError(t, materializeScenarioWorkspace(context.Background(), writer, target, "run-1", "tool-select-grep", "attempt-1", ws, files))

	require.Len(t, writer.writes, 3, "decoys are written too")
	assert.Equal(t, ws.Root+"/logs/checkout.log", writer.writes[0].AbsPath)
	assert.Equal(t, ws.Root+"/logs/auth.log", writer.writes[1].AbsPath)
	assert.Equal(t, "files live in "+ws.Root+"/logs", writer.writes[2].Content, "content is rendered through the workspace")
	for _, write := range writer.writes {
		assert.Equal(t, target, write.Target)
		assert.Equal(t, "run-1", write.RunID)
		assert.Equal(t, "tool-select-grep", write.ScenarioID)
		assert.Equal(t, "attempt-1", write.AttemptID)
		assert.True(t, ws.Contains(write.AbsPath))
	}
}

func TestMaterializeScenarioWorkspace_NoFilesNeedsNoWriter(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	require.NoError(t, materializeScenarioWorkspace(context.Background(), nil, Target{}, "run-1", "instruction-exact-format", "attempt-1", ws, nil))
}

func TestMaterializeScenarioWorkspace_FailsClosedWithoutAWriter(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	err := materializeScenarioWorkspace(context.Background(), nil, Target{}, "run-1", "tool-select-grep", "attempt-1", ws,
		[]ScenarioWorkspaceFile{{RelPath: "a.txt", Content: "x"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationWorkspaceUnavailable)
	assert.Contains(t, err.Error(), "tool-select-grep")
}

func TestMaterializeScenarioWorkspace_RefusesAFixturePathThatEscapesTheWorkspaceBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	writer := &recordingWorkspaceFileWriter{}
	files := []ScenarioWorkspaceFile{
		{RelPath: "ok.txt", Content: "fine"},
		{RelPath: "../escape.txt", Content: "bad"},
		{RelPath: "never.txt", Content: "not reached"},
	}

	err := materializeScenarioWorkspace(context.Background(), writer, Target{}, "run-1", "s", "attempt-1", ws, files)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationWorkspaceUnavailable)
	require.Len(t, writer.writes, 1, "stops at the first bad path")
	assert.Equal(t, ws.Root+"/ok.txt", writer.writes[0].AbsPath)
}

func TestMaterializeScenarioWorkspace_StopsAtTheFirstWriteFailureAndWrapsIt(t *testing.T) {
	t.Parallel()
	ws := mustWorkspace(t, "/srv/op", "run-1", "attempt-1")
	failing := ws.Root + "/b.txt"
	writer := &recordingWorkspaceFileWriter{failOnPath: failing}
	files := []ScenarioWorkspaceFile{
		{RelPath: "a.txt", Content: "1"},
		{RelPath: "b.txt", Content: "2"},
		{RelPath: "c.txt", Content: "3"},
	}

	err := materializeScenarioWorkspace(context.Background(), writer, Target{}, "run-1", "s", "attempt-1", ws, files)

	require.Error(t, err)
	assert.Contains(t, err.Error(), failing)
	require.Len(t, writer.writes, 1, "no file after the failure is written")
	assert.Equal(t, ws.Root+"/a.txt", writer.writes[0].AbsPath)
}
