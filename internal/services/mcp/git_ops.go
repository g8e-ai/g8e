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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GitOpsTool provides git repository operations including status, log, branch info, and remote management.
type GitOpsTool struct{}

// Name returns the tool identifier.
func (t *GitOpsTool) Name() string {
	return "git_ops"
}

// Description returns a human-readable description.
func (t *GitOpsTool) Description() string {
	return "Provides git repository operations including status, log, branch info, and remote management for GitHub/GitLab workflows."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *GitOpsTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Git operation to perform",
				"enum":        []string{"status", "log", "branches", "remotes", "remote_url", "current_branch", "diff"},
			},
			"repo_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to git repository (defaults to current directory)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Limit for log entries (default: 10)",
			},
			"ref": map[string]interface{}{
				"type":        "string",
				"description": "Git reference for diff or log (e.g., HEAD~1, main)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *GitOpsTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Operation string `json:"operation"`
		RepoPath  string `json:"repo_path,omitempty"`
		Limit     int    `json:"limit,omitempty"`
		Ref       string `json:"ref,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Operation == "" {
		req.Operation = "status"
	}

	repoPath := req.RepoPath
	if repoPath == "" {
		repoPath = "."
	}

	if err := validateGitRepoPath(repoPath); err != nil {
		result := map[string]interface{}{
			"operation": req.Operation,
			"repo_path": repoPath,
			"error":     err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	if !isGitRepo(repoPath) {
		result := map[string]interface{}{
			"operation": req.Operation,
			"repo_path": repoPath,
			"error":     "not a git repository",
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	var result map[string]interface{}
	var err error

	switch req.Operation {
	case "status":
		result, err = gitStatus(repoPath)
	case "log":
		limit := req.Limit
		if limit <= 0 {
			limit = 10
		}
		result, err = gitLog(repoPath, limit)
	case "branches":
		result, err = gitBranches(repoPath)
	case "remotes":
		result, err = gitRemotes(repoPath)
	case "remote_url":
		result, err = gitRemoteURL(repoPath)
	case "current_branch":
		result, err = gitCurrentBranch(repoPath)
	case "diff":
		ref := req.Ref
		if ref == "" {
			ref = "HEAD"
		}
		result, err = gitDiff(repoPath, ref)
	default:
		return CallToolResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	if err != nil {
		result = map[string]interface{}{
			"operation": req.Operation,
			"repo_path": repoPath,
			"error":     err.Error(),
		}
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", marshalErr)
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

func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}
	return false
}

func runGitCommand(repoPath string, args ...string) (string, error) {
	// Validate git subcommand is safe
	if len(args) == 0 {
		return "", fmt.Errorf("no git subcommand provided")
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
		return "", fmt.Errorf("git subcommand '%s' is not allowed", subcommand)
	}

	// Validate repo path to prevent path traversal
	if err := validateGitRepoPath(repoPath); err != nil {
		return "", err
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func gitStatus(repoPath string) (map[string]interface{}, error) {
	output, err := runGitCommand(repoPath, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
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

		switch status {
		case "M", "MM":
			modified = append(modified, file)
		case "A":
			added = append(added, file)
		case "D":
			deleted = append(deleted, file)
		case "??":
			untracked = append(untracked, file)
		}
	}

	branch, _ := gitCurrentBranch(repoPath)

	return map[string]interface{}{
		"branch":    branch,
		"modified":  modified,
		"added":     added,
		"deleted":   deleted,
		"untracked": untracked,
		"clean":     len(modified) == 0 && len(added) == 0 && len(deleted) == 0 && len(untracked) == 0,
	}, nil
}

func gitLog(repoPath string, limit int) (map[string]interface{}, error) {
	// Bound the limit to prevent resource exhaustion
	if limit <= 0 {
		limit = 10
	}
	if limit > 1000 {
		limit = 1000
	}

	output, err := runGitCommand(repoPath, "log", "--max-count", strconv.Itoa(limit), "--pretty=format:%H|%an|%ae|%ad|%s", "--date=iso")
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var commits []map[string]interface{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		commits = append(commits, map[string]interface{}{
			"hash":       parts[0],
			"author":     parts[1],
			"email":      parts[2],
			"date":       parts[3],
			"message":    parts[4],
			"short_hash": parts[0][:7],
		})
	}

	return map[string]interface{}{
		"commits": commits,
		"count":   len(commits),
	}, nil
}

func gitBranches(repoPath string) (map[string]interface{}, error) {
	output, err := runGitCommand(repoPath, "branch", "-a")
	if err != nil {
		return nil, fmt.Errorf("git branch failed: %w", err)
	}

	var local []string
	var remote []string
	current, _ := gitCurrentBranch(repoPath)

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

	return map[string]interface{}{
		"current": current,
		"local":   local,
		"remote":  remote,
	}, nil
}

func gitRemotes(repoPath string) (map[string]interface{}, error) {
	output, err := runGitCommand(repoPath, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("git remote failed: %w", err)
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
		typ := strings.TrimSuffix(parts[2], "(")

		if remotes[name] == nil {
			remotes[name] = make(map[string]string)
		}
		remotes[name][typ] = url
	}

	return map[string]interface{}{
		"remotes": remotes,
	}, nil
}

func gitRemoteURL(repoPath string) (map[string]interface{}, error) {
	output, err := runGitCommand(repoPath, "config", "--get", "remote.origin.url")
	if err != nil {
		return nil, fmt.Errorf("git remote url failed: %w", err)
	}

	platform := "unknown"
	if strings.Contains(output, "github.com") {
		platform = "github"
	} else if strings.Contains(output, "gitlab.com") {
		platform = "gitlab"
	} else if strings.Contains(output, "bitbucket.org") {
		platform = "bitbucket"
	}

	return map[string]interface{}{
		"url":      output,
		"platform": platform,
	}, nil
}

func gitCurrentBranch(repoPath string) (map[string]interface{}, error) {
	output, err := runGitCommand(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git current branch failed: %w", err)
	}

	return map[string]interface{}{
		"branch": strings.Trim(output, "'"),
	}, nil
}

func gitDiff(repoPath string, ref string) (map[string]interface{}, error) {
	if err := validateGitRef(ref); err != nil {
		return nil, fmt.Errorf("invalid git reference: %w", err)
	}

	output, err := runGitCommand(repoPath, "diff", ref)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	if output == "" {
		return map[string]interface{}{
			"ref":     ref,
			"diff":    "",
			"changes": false,
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

	return map[string]interface{}{
		"ref":     ref,
		"diff":    output,
		"changes": true,
		"files":   files,
	}, nil
}
