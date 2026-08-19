// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
)

// GitOpsTool provides git repository operations including status, log, branch info, and remote management.
type GitOpsTool struct {
	runner gitRunner
	once   sync.Once
}

type gitRunner interface {
	Run(ctx context.Context, repoPath string, args ...string) (string, error)
	IsRepo(path string) bool
}

type realGitRunner struct{}

func (r *realGitRunner) Run(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *realGitRunner) IsRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	return false
}

// gitStatusResult represents the result of a git status operation
type gitStatusResult struct {
	Branch    string   `json:"branch"`
	Modified  []string `json:"modified"`
	Added     []string `json:"added"`
	Deleted   []string `json:"deleted"`
	Untracked []string `json:"untracked"`
	Clean     bool     `json:"clean"`
}

// gitLogResult represents the result of a git log operation
type gitLogResult struct {
	Commits []gitCommit `json:"commits"`
	Count   int         `json:"count"`
}

// gitCommit represents a single git commit
type gitCommit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Author    string `json:"author"`
	Email     string `json:"email"`
	Date      string `json:"date"`
	Message   string `json:"message"`
}

// gitBranchesResult represents the result of a git branches operation
type gitBranchesResult struct {
	Current string   `json:"current"`
	Local   []string `json:"local"`
	Remote  []string `json:"remote"`
}

// gitRemotesResult represents the result of a git remotes operation
type gitRemotesResult struct {
	Remotes map[string]map[string]string `json:"remotes"`
}

// gitRemoteURLResult represents the result of a git remote URL operation
type gitRemoteURLResult struct {
	URL      string `json:"url"`
	Platform string `json:"platform"`
}

// gitCurrentBranchResult represents the result of a git current branch operation
type gitCurrentBranchResult struct {
	Branch string `json:"branch"`
}

// gitDiffResult represents the result of a git diff operation
type gitDiffResult struct {
	Ref     string   `json:"ref"`
	Diff    string   `json:"diff"`
	Changes bool     `json:"changes"`
	Files   []string `json:"files"`
}

// gitErrorResult represents an error result from a git operation
type gitErrorResult struct {
	Operation string `json:"operation"`
	RepoPath  string `json:"repo_path"`
	Error     string `json:"error"`
}

// Name returns the tool identifier.
func (t *GitOpsTool) Name() string {
	return "git_ops"
}

// Description returns a human-readable description.
func (t *GitOpsTool) Description() string {
	return "Provides git repository operations including status, log, branch info, and remote management for GitHub/GitLab workflows."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *GitOpsTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"operation": {
				Type:        "string",
				Description: "Git operation to perform",
				Enum:        []string{"status", "log", "branches", "remotes", "remote_url", "current_branch", "diff"},
			},
			"repo_path": {
				Type:        "string",
				Description: "Path to git repository (defaults to current directory)",
			},
			"limit": {
				Type:        "integer",
				Description: "Limit for log entries (default: 10)",
			},
			"ref": {
				Type:        "string",
				Description: "Git reference for diff or log (e.g., HEAD~1, main)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *GitOpsTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req GitOpsRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("git_ops: invalid arguments: %w", err)
	}

	if req.Operation == "" {
		req.Operation = "status"
	}

	repoPath := req.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	t.once.Do(func() {
		if t.runner == nil {
			t.runner = &realGitRunner{}
		}
	})

	if err := validateGitRepoPath(repoPath); err != nil {
		errorResult := gitErrorResult{
			Operation: req.Operation,
			RepoPath:  repoPath,
			Error:     err.Error(),
		}
		resultJSON, marshalErr := json.Marshal(errorResult)
		if marshalErr != nil {
			return CallToolResult{}, fmt.Errorf("git_ops: failed to marshal error result: %w", marshalErr)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	if !t.runner.IsRepo(repoPath) {
		errorResult := gitErrorResult{
			Operation: req.Operation,
			RepoPath:  repoPath,
			Error:     "not a git repository",
		}
		resultJSON, marshalErr := json.Marshal(errorResult)
		if marshalErr != nil {
			return CallToolResult{}, fmt.Errorf("git_ops: failed to marshal error result: %w", marshalErr)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	var result interface{}
	var err error

	switch req.Operation {
	case "status":
		result, err = t.gitStatus(ctx, repoPath)
	case "log":
		limit := req.Limit
		if limit <= 0 {
			limit = 10
		}
		result, err = t.gitLog(ctx, repoPath, limit)
	case "branches":
		result, err = t.gitBranches(ctx, repoPath)
	case "remotes":
		result, err = t.gitRemotes(ctx, repoPath)
	case "remote_url":
		result, err = t.gitRemoteURL(ctx, repoPath)
	case "current_branch":
		result, err = t.gitCurrentBranch(ctx, repoPath)
	case "diff":
		ref := req.Ref
		if ref == "" {
			ref = "HEAD"
		}
		result, err = t.gitDiff(ctx, repoPath, ref)
	default:
		return CallToolResult{}, fmt.Errorf("%w: %s", constants.ErrMCPGitOpsUnsupportedOperation, req.Operation)
	}

	if err != nil {
		errorResult := gitErrorResult{
			Operation: req.Operation,
			RepoPath:  repoPath,
			Error:     err.Error(),
		}
		resultJSON, marshalErr := json.Marshal(errorResult)
		if marshalErr != nil {
			return CallToolResult{}, fmt.Errorf("git_ops: failed to marshal error result: %w", marshalErr)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return CallToolResult{}, fmt.Errorf("git_ops: failed to marshal result: %w", marshalErr)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

func (t *GitOpsTool) runGitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
	// Validate git subcommand is safe
	if len(args) == 0 {
		return "", fmt.Errorf("git_ops: no git subcommand provided")
	}

	// Whitelist of safe git subcommands
	safeSubcommands := map[string]bool{
		"status":    true,
		"log":       true,
		"branch":    true,
		"branches":  true,
		"remote":    true,
		"remotes":   true,
		"config":    true,
		"rev-parse": true,
		"diff":      true,
	}

	subcommand := args[0]
	if !safeSubcommands[subcommand] {
		return "", fmt.Errorf("git_ops: git subcommand '%s' is not allowed", subcommand)
	}

	// Validate repo path to prevent path traversal
	if err := validateGitRepoPath(repoPath); err != nil {
		return "", fmt.Errorf("git_ops: invalid repo path: %w", err)
	}

	output, err := t.runner.Run(ctx, repoPath, args...)
	if err != nil {
		return output, fmt.Errorf("git_ops: git %s failed: %w", subcommand, err)
	}
	return output, nil
}

func (t *GitOpsTool) gitStatus(ctx context.Context, repoPath string) (*gitStatusResult, error) {
	output, err := t.runGitCommand(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git status failed: %w", err)
	}

	lines := strings.Split(output, "\n")
	var modified []string
	var added []string
	var deleted []string
	var untracked []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) < 2 {
			continue
		}
		status := line[:2]
		file := strings.TrimSpace(line[2:])

		if strings.Contains(status, "M") {
			modified = append(modified, file)
		} else if strings.Contains(status, "A") {
			added = append(added, file)
		} else if strings.Contains(status, "D") {
			deleted = append(deleted, file)
		} else if strings.HasPrefix(status, "??") {
			untracked = append(untracked, file)
		}
	}

	branchResult, err := t.gitCurrentBranch(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git_ops: failed to get current branch: %w", err)
	}

	return &gitStatusResult{
		Branch:    branchResult.Branch,
		Modified:  modified,
		Added:     added,
		Deleted:   deleted,
		Untracked: untracked,
		Clean:     len(modified) == 0 && len(added) == 0 && len(deleted) == 0 && len(untracked) == 0,
	}, nil
}

func (t *GitOpsTool) gitLog(ctx context.Context, repoPath string, limit int) (*gitLogResult, error) {
	// Bound the limit to prevent resource exhaustion
	if limit <= 0 {
		limit = 10
	}
	if limit > 1000 {
		limit = 1000
	}

	output, err := t.runGitCommand(ctx, repoPath, "log", "--max-count", strconv.Itoa(limit), "--pretty=format:%H|%an|%ae|%ad|%s", "--date=iso")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git log failed: %w", err)
	}

	var commits []gitCommit
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		commit := gitCommit{
			Hash:    parts[0],
			Author:  parts[1],
			Email:   parts[2],
			Date:    parts[3],
			Message: parts[4],
		}
		if len(parts[0]) >= 7 {
			commit.ShortHash = parts[0][:7]
		} else {
			commit.ShortHash = parts[0]
		}
		commits = append(commits, commit)
	}

	return &gitLogResult{
		Commits: commits,
		Count:   len(commits),
	}, nil
}

func (t *GitOpsTool) gitBranches(ctx context.Context, repoPath string) (*gitBranchesResult, error) {
	output, err := t.runGitCommand(ctx, repoPath, "branch", "-a")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git branch failed: %w", err)
	}

	var local []string
	var remote []string
	currentResult, err := t.gitCurrentBranch(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git_ops: failed to get current branch: %w", err)
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		line = strings.TrimPrefix(line, "* ")

		if strings.HasPrefix(line, "remotes/") {
			remote = append(remote, line)
		} else {
			local = append(local, line)
		}
	}

	return &gitBranchesResult{
		Current: currentResult.Branch,
		Local:   local,
		Remote:  remote,
	}, nil
}

func (t *GitOpsTool) gitRemotes(ctx context.Context, repoPath string) (*gitRemotesResult, error) {
	output, err := t.runGitCommand(ctx, repoPath, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git remote failed: %w", err)
	}

	remotes := make(map[string]map[string]string)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		url := parts[1]
		typ := strings.Trim(parts[2], "()")

		if remotes[name] == nil {
			remotes[name] = make(map[string]string)
		}
		remotes[name][typ] = url
	}

	return &gitRemotesResult{
		Remotes: remotes,
	}, nil
}

func (t *GitOpsTool) gitRemoteURL(ctx context.Context, repoPath string) (*gitRemoteURLResult, error) {
	output, err := t.runGitCommand(ctx, repoPath, "config", "--get", "remote.origin.url")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git remote url failed: %w", err)
	}

	platform := "unknown"
	if strings.Contains(output, "github.com") {
		platform = "github"
	} else if strings.Contains(output, "gitlab.com") {
		platform = "gitlab"
	} else if strings.Contains(output, "bitbucket.org") {
		platform = "bitbucket"
	}

	return &gitRemoteURLResult{
		URL:      output,
		Platform: platform,
	}, nil
}

func (t *GitOpsTool) gitCurrentBranch(ctx context.Context, repoPath string) (*gitCurrentBranchResult, error) {
	output, err := t.runGitCommand(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git_ops: git current branch failed: %w", err)
	}

	return &gitCurrentBranchResult{
		Branch: strings.Trim(output, "'"),
	}, nil
}

func (t *GitOpsTool) gitDiff(ctx context.Context, repoPath string, ref string) (*gitDiffResult, error) {
	if err := validateGitRef(ref); err != nil {
		return nil, fmt.Errorf("git_ops: invalid git reference: %w", err)
	}

	output, err := t.runGitCommand(ctx, repoPath, "diff", ref)
	if err != nil {
		return nil, fmt.Errorf("git_ops: git diff failed: %w", err)
	}

	if output == "" {
		return &gitDiffResult{
			Ref:     ref,
			Diff:    "",
			Changes: false,
		}, nil
	}

	var files []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				files = append(files, parts[3])
			}
		}
	}

	return &gitDiffResult{
		Ref:     ref,
		Diff:    output,
		Changes: true,
		Files:   files,
	}, nil
}
