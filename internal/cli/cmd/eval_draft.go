// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// evalTempFileWriter abstracts temp file creation so Tier 1 tests do not
// touch the filesystem. The real implementation uses os.CreateTemp.
type evalTempFileWriter interface {
	WriteTempFile(namePattern string, data []byte) (string, error)
}

// realEvalTempFileWriter is the production evalTempFileWriter backed by
// os.CreateTemp.
type realEvalTempFileWriter struct{}

func (realEvalTempFileWriter) WriteTempFile(namePattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", namePattern)
	if err != nil {
		return "", fmt.Errorf("eval: create temp request file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("eval: write temp request file: %w", err)
	}
	return f.Name(), nil
}

// evalDraftDeps carries the injected dependencies for draft commands.
// All fields are interfaces so Tier 1 tests can stub every side effect
// without touching the filesystem or spawning processes.
type evalDraftDeps struct {
	configLoader   func(string) (*config.Config, error)
	stat           evalFileStat
	runner         evalCommandRunner
	tempFileWriter evalTempFileWriter
}

// evalDraftSummary is the typed review summary emitted by draft commands.
// It mirrors the Python DraftReviewSummary model.
type evalDraftSummary struct {
	OperationKind   string         `json:"operation_kind"`
	OperationID     string         `json:"operation_id"`
	Revision        string         `json:"revision"`
	Preset          string         `json:"preset"`
	Suite           string         `json:"suite"`
	ReportRoot      string         `json:"report_root"`
	ContentHash     string         `json:"content_hash"`
	Dimensions      map[string]any `json:"dimensions"`
	Budget          map[string]any `json:"budget"`
	AuthorityHashes map[string]string `json:"authority_hashes"`
	EndpointClass   string         `json:"endpoint_class"`
	Provider        string         `json:"provider"`
}

// evalDraftRequestBase carries the common fields for both diagnostic and
// campaign draft requests. The Go facade marshals this to JSON and passes
// it to the Python draft module.
type evalDraftRequestBase struct {
	Kind            string             `json:"kind"`
	Preset          string             `json:"preset"`
	OperationID     string             `json:"operation_id"`
	Revision        string             `json:"revision"`
	ReportRoot      string             `json:"report_root"`
	GoldSet         evalAuthorityRefJSON `json:"gold_set"`
	EvidenceKey     evalEvidenceKeyRefJSON `json:"evidence_key"`
	ProviderEndpoint evalProviderEndpointRefJSON `json:"provider_endpoint"`
	OutputPath      string             `json:"output_path"`
}

type evalAuthorityRefJSON struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type evalEvidenceKeyRefJSON struct {
	Path  string `json:"path"`
	KeyID string `json:"key_id"`
}

type evalProviderEndpointRefJSON struct {
	Provider      string `json:"provider"`
	EndpointClass string `json:"endpoint_class"`
}

// runEvalDraft invokes the Python draft module with a typed JSON request
// and returns the parsed review summary. The request is written to a temp
// file, passed to the Python module, and cleaned up after invocation.
func runEvalDraft(ctx context.Context, deps evalDraftDeps, requestJSON []byte, stderr io.Writer) (evalDraftSummary, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalDraftSummary{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalDraftSummary{}, err
	}
	resolver := &evalEnvironmentResolver{stat: deps.stat}
	env, err := resolver.Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalDraftSummary{}, err
	}

	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-draft-*.json", requestJSON)
	if err != nil {
		return evalDraftSummary{}, err
	}
	defer os.Remove(tmpPath)

	var stdoutBuf strings.Builder
	args := []string{"-m", constants.EvalDraftModule, tmpPath}
	if err := deps.runner.Run(ctx, env.InterpreterPath, args, &stdoutBuf, stderr); err != nil {
		return evalDraftSummary{}, fmt.Errorf("%w: %w", constants.ErrEvalConfigInvalid, err)
	}

	var summary evalDraftSummary
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdoutBuf.String())), &summary); err != nil {
		return evalDraftSummary{}, fmt.Errorf("%w: parse draft summary: %w", constants.ErrEvalConfigInvalid, err)
	}
	return summary, nil
}

// printEvalDraftSummaryHuman prints a concise human-readable review
// summary to stdout.
func printEvalDraftSummaryHuman(stdout io.Writer, summary evalDraftSummary) {
	fmt.Fprintf(stdout, "Draft created: %s\n", summary.OperationKind)
	fmt.Fprintf(stdout, "  operation_id: %s\n", summary.OperationID)
	fmt.Fprintf(stdout, "  revision:     %s\n", summary.Revision)
	fmt.Fprintf(stdout, "  preset:        %s\n", summary.Preset)
	fmt.Fprintf(stdout, "  suite:         %s\n", summary.Suite)
	fmt.Fprintf(stdout, "  report_root:   %s\n", summary.ReportRoot)
	fmt.Fprintf(stdout, "  content_hash:  %s\n", summary.ContentHash)
	fmt.Fprintf(stdout, "  endpoint:      %s (%s)\n", summary.Provider, summary.EndpointClass)
	if summary.Dimensions != nil {
		fmt.Fprintf(stdout, "  dimensions:\n")
		printEvalDraftMap(stdout, "    ", summary.Dimensions)
	}
	if summary.Budget != nil {
		fmt.Fprintf(stdout, "  budget:\n")
		printEvalDraftMap(stdout, "    ", summary.Budget)
	}
	if summary.AuthorityHashes != nil {
		fmt.Fprintf(stdout, "  authority_hashes:\n")
		for name, hash := range summary.AuthorityHashes {
			fmt.Fprintf(stdout, "    %s: %s\n", name, hash)
		}
	}
}

func printEvalDraftMap(stdout io.Writer, indent string, m map[string]any) {
	for k, v := range m {
		fmt.Fprintf(stdout, "%s%s: %v\n", indent, k, v)
	}
}
