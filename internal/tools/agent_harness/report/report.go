// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

// Package report turns a run into auditable artifacts: a machine-readable JSON
// dump of every exchange, and a human Markdown report that cross-references each
// impersonation against the Operator's real signed receipts.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/scenarios"
)

type Report struct {
	GeneratedAt       time.Time           `json:"generated_at"`
	Gateway           string              `json:"gateway"`
	OperatorSessionID string              `json:"operator_session_id"`
	Results           []scenarios.Result  `json:"results"`
	Receipts          []clientpkg.Receipt `json:"receipts"`
}

// Write emits report.json and report.md at the specified paths.
// The directory containing jsonPath and mdPath will be created if it does not exist.
func Write(jsonPath, mdPath string, rep Report) (string, string, error) {
	if err := os.MkdirAll(filepath.Dir(jsonPath), constants.PermDirStandard); err != nil {
		return "", "", fmt.Errorf("report: write: mkdir: %w", err)
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("report: write: marshal json: %w", err)
	}
	if err := os.WriteFile(jsonPath, b, constants.PermFilePublic); err != nil {
		return "", "", fmt.Errorf("report: write: write json: %w", err)
	}

	if err := os.WriteFile(mdPath, []byte(markdown(rep)), constants.PermFilePublic); err != nil {
		return jsonPath, "", fmt.Errorf("report: write: write markdown: %w", err)
	}
	return jsonPath, mdPath, nil
}

func markdown(rep Report) string {
	var b strings.Builder
	receiptIndex := indexReceipts(rep.Receipts)

	fmt.Fprintf(&b, "# Agent Harness run report\n\n")
	fmt.Fprintf(&b, "- Generated: %s\n", rep.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Gateway: `%s`\n", rep.Gateway)
	fmt.Fprintf(&b, "- Operator session: `%s`\n", orNone(rep.OperatorSessionID))
	fmt.Fprintf(&b, "- Real signed receipts pulled: %d\n\n", len(rep.Receipts))

	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| Scenario | Persona | Posture | Result | Calls | Tx → matched receipt |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|\n")
	for _, r := range rep.Results {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %d | %s |\n",
			r.Name, r.Persona, r.RequiresPosture, mark(r.OK), len(r.Exchanges), txMatch(r.TxHashes, receiptIndex))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Detail\n\n")
	for _, r := range rep.Results {
		fmt.Fprintf(&b, "### %s — %s\n\n", r.Name, r.Title)
		fmt.Fprintf(&b, "**%s** as `%s`, posture `%s`, %dms.\n\n", mark(r.OK), r.Persona, r.RequiresPosture, r.DurationMS)
		if r.Err != "" {
			fmt.Fprintf(&b, "> error: %s\n\n", r.Err)
		}
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
		if len(r.Notes) > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "| # | method | status | ms | url |\n|---|---|---|---|---|\n")
		for i, ex := range r.Exchanges {
			fmt.Fprintf(&b, "| %d | %s | %d | %d | `%s` |\n", i+1, ex.Method, ex.Status, ex.LatencyMS, ex.URL)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "## Real Operator receipts\n\n")
	if len(rep.Receipts) == 0 {
		b.WriteString("_No receipts returned. Confirm the Operator session id and that mutations actually executed._\n")
		return b.String()
	}
	fmt.Fprintf(&b, "| tx_hash | action | status | state before → after |\n|---|---|---|---|\n")
	for _, rc := range rep.Receipts {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s → %s |\n",
			shortHash(rc.TransactionHash), rc.ActionType, rc.Status, shortHash(rc.StateRootBefore), shortHash(rc.StateRootAfter))
	}
	return b.String()
}

func indexReceipts(rs []clientpkg.Receipt) map[string]clientpkg.Receipt {
	m := make(map[string]clientpkg.Receipt, len(rs))
	for _, r := range rs {
		if r.TransactionHash != "" {
			m[r.TransactionHash] = r
		}
	}
	return m
}

func txMatch(hashes []string, idx map[string]clientpkg.Receipt) string {
	if len(hashes) == 0 {
		return "—"
	}
	var parts []string
	for _, h := range hashes {
		if _, ok := idx[h]; ok {
			parts = append(parts, "✓ `"+shortHash(h)+"` matched")
		} else {
			parts = append(parts, "`"+shortHash(h)+"` (no receipt)")
		}
	}
	return strings.Join(parts, "<br>")
}

func mark(ok bool) string {
	if ok {
		return "✅ ok"
	}
	return "❌ fail"
}

func orNone(s string) string {
	if s == "" {
		return "(auto-discover)"
	}
	return s
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}
