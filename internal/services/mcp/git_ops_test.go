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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGitRunner struct {
	runFunc    func(ctx context.Context, repoPath string, args ...string) (string, error)
	isRepoFunc func(path string) bool
}

func (m *mockGitRunner) Run(ctx context.Context, repoPath string, args ...string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, repoPath, args...)
	}
	return "", nil
}

func (m *mockGitRunner) IsRepo(path string) bool {
	if m.isRepoFunc != nil {
		return m.isRepoFunc(path)
	}
	return true
}

func TestGitOpsTool_Metadata(t *testing.T) {
	tool := &GitOpsTool{}
	assert.Equal(t, "git_ops", tool.Name())
	assert.Contains(t, tool.Description(), "git repository operations")
	assert.NotNil(t, tool.InputSchema())
}

func TestGitOpsTool_Execute_NotARepo(t *testing.T) {
	runner := &mockGitRunner{
		isRepoFunc: func(path string) bool { return false },
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "status", "repo_path": "/valid/path"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var errorRes gitErrorResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &errorRes)
	require.NoError(t, err)
	assert.Equal(t, "not a git repository", errorRes.Error)
}

func TestGitOpsTool_Execute_Status(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			subcommand := args[0]
			if subcommand == "status" {
				return "M  file1.go\nA  file2.go\n?? untracked.go", nil
			}
			if subcommand == "rev-parse" {
				return "main", nil
			}
			return "", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "status"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var statusRes gitStatusResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &statusRes)
	require.NoError(t, err)

	assert.Equal(t, "main", statusRes.Branch)
	assert.Contains(t, statusRes.Modified, "file1.go")
	assert.Contains(t, statusRes.Added, "file2.go")
	assert.Contains(t, statusRes.Untracked, "untracked.go")
	assert.False(t, statusRes.Clean)
}

func TestGitOpsTool_Execute_Log(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			return "hash1|author1|email1|date1|msg1\nhash2|author2|email2|date2|msg2", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "log", "limit": 2}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var logRes gitLogResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &logRes)
	require.NoError(t, err)

	assert.Equal(t, 2, logRes.Count)
	assert.Equal(t, "hash1", logRes.Commits[0].Hash)
	assert.Equal(t, "msg2", logRes.Commits[1].Message)
}

func TestGitOpsTool_Execute_Branches(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			subcommand := args[0]
			if subcommand == "branch" {
				return "  master\n* main\n  remotes/origin/main", nil
			}
			if subcommand == "rev-parse" {
				return "main", nil
			}
			return "", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "branches"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var branchRes gitBranchesResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &branchRes)
	require.NoError(t, err)

	assert.Equal(t, "main", branchRes.Current)
	assert.Contains(t, branchRes.Local, "master")
	assert.Contains(t, branchRes.Local, "main")
	assert.Contains(t, branchRes.Remote, "remotes/origin/main")
}

func TestGitOpsTool_Execute_Remotes(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			return "origin\thttps://github.com/org/repo.git (fetch)\norigin\thttps://github.com/org/repo.git (push)", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "remotes"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var remoteRes gitRemotesResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &remoteRes)
	require.NoError(t, err)

	assert.Contains(t, remoteRes.Remotes, "origin")
	assert.Equal(t, "https://github.com/org/repo.git", remoteRes.Remotes["origin"]["fetch"])
	assert.Equal(t, "https://github.com/org/repo.git", remoteRes.Remotes["origin"]["push"])
}

func TestGitOpsTool_Execute_RemoteURL(t *testing.T) {
	tests := []struct {
		url      string
		platform string
	}{
		{"https://github.com/org/repo.git", "github"},
		{"https://gitlab.com/org/repo.git", "gitlab"},
		{"https://bitbucket.org/org/repo.git", "bitbucket"},
		{"https://example.com/org/repo.git", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			runner := &mockGitRunner{
				runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
					return tt.url, nil
				},
			}
			tool := &GitOpsTool{runner: runner}
			ctx := context.Background()

			args := json.RawMessage(`{"operation": "remote_url"}`)
			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)

			var res gitRemoteURLResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &res)
			require.NoError(t, err)

			assert.Equal(t, tt.url, res.URL)
			assert.Equal(t, tt.platform, res.Platform)
		})
	}
}

func TestGitOpsTool_Execute_Diff(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			return "diff --git a/file1.go b/file1.go\nindex 123..456 100644\n--- a/file1.go\n+++ b/file1.go\n@@ -1,1 +1,1 @@\n-old\n+new", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "diff", "ref": "HEAD"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var diffRes gitDiffResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &diffRes)
	require.NoError(t, err)

	assert.Equal(t, "HEAD", diffRes.Ref)
	assert.True(t, diffRes.Changes)
	assert.Contains(t, diffRes.Diff, "diff --git")
	assert.Contains(t, diffRes.Files, "b/file1.go")
}

func TestGitOpsTool_Execute_Diff_NoChanges(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			return "", nil
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "diff"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var diffRes gitDiffResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &diffRes)
	require.NoError(t, err)

	assert.False(t, diffRes.Changes)
	assert.Empty(t, diffRes.Diff)
}

func TestGitOpsTool_Execute_InvalidArgs(t *testing.T) {
	tool := &GitOpsTool{}
	ctx := context.Background()

	args := json.RawMessage(`{invalid json}`)
	_, err := tool.Execute(ctx, args)
	assert.Error(t, err)
}

func TestGitOpsTool_Execute_UnsupportedOperation(t *testing.T) {
	tool := &GitOpsTool{runner: &mockGitRunner{}}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "invalid"}`)
	_, err := tool.Execute(ctx, args)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrMCPGitOpsUnsupportedOperation))
}

func TestGitOpsTool_Execute_GitCommandError(t *testing.T) {
	runner := &mockGitRunner{
		runFunc: func(ctx context.Context, repoPath string, args ...string) (string, error) {
			return "fatal: not a git repository", fmt.Errorf("exit status 128")
		},
	}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "status"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var errorRes gitErrorResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &errorRes)
	require.NoError(t, err)
	assert.Contains(t, errorRes.Error, "git status failed")
}

func TestGitOpsTool_Execute_PathTraversal(t *testing.T) {
	tool := &GitOpsTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "status", "repo_path": "../../../etc/passwd"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var errorRes gitErrorResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &errorRes)
	require.NoError(t, err)
	assert.Contains(t, errorRes.Error, "parent directory references")
}

func TestGitOpsTool_Execute_InvalidRef(t *testing.T) {
	runner := &mockGitRunner{}
	tool := &GitOpsTool{runner: runner}
	ctx := context.Background()

	args := json.RawMessage(`{"operation": "diff", "ref": "HEAD; rm -rf /"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var errorRes gitErrorResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &errorRes)
	require.NoError(t, err)
	assert.Contains(t, errorRes.Error, "invalid git reference")
}
