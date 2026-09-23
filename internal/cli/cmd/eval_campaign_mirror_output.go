// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func writeCampaignMirrorRestoreProgress(w io.Writer, progress evaluation.CampaignMirrorReconcileProgress) {
	prefix := fmt.Sprintf("[%d/%d] %s", progress.Index, progress.Total, progress.RunID)
	switch progress.Status {
	case evaluation.CampaignMirrorReconcileChecking:
		_, _ = fmt.Fprintf(w, "Checking verified run %d/%d: %s\n", progress.Index, progress.Total, progress.RunID)
	case evaluation.CampaignMirrorReconcileRestored:
		_, _ = fmt.Fprintf(w, "%s restored (%d record(s))\n", prefix, progress.PublishedRecords)
	case evaluation.CampaignMirrorReconcilePresent:
		_, _ = fmt.Fprintf(w, "%s already present\n", prefix)
	case evaluation.CampaignMirrorReconcileHostAbsent:
		_, _ = fmt.Fprintf(w, "%s skipped (host artifacts absent)\n", prefix)
	case evaluation.CampaignMirrorReconcileFailed:
		_, _ = fmt.Fprintf(w, "%s failed: %v\n", prefix, progress.Err)
	}
}

func writeCampaignMirrorRestoreQueueResult(stdout, stderr io.Writer, result *evaluation.CampaignMirrorReconcileResult) {
	if result == nil {
		return
	}
	restored := len(result.RestoredRunIDs)
	skipped := len(result.SkippedRunIDs)
	hostAbsent := len(result.HostAbsentRunIDs)

	switch {
	case restored > 0:
		_, _ = fmt.Fprintf(stdout, "Restored %d verified dataset(s) to public mirror (%d record(s)).\n", restored, result.PublishedRecords)
		if skipped > 0 {
			_, _ = fmt.Fprintf(stdout, "%d verified dataset(s) were already present in public mirror.\n", skipped)
		}
	case skipped > 0:
		_, _ = fmt.Fprintf(stdout, "Verified campaign mirror restore complete: %d verified dataset(s) already present in public mirror (nothing to do).\n", skipped)
	default:
		_, _ = fmt.Fprintln(stdout, "Verified campaign mirror restore complete: nothing to do.")
	}

	if hostAbsent > 0 {
		writeHostAbsentMirrorNote(stdout, result.HostAbsentRunIDs)
	}

	for runID, reason := range result.FailedRuns {
		_, _ = fmt.Fprintf(stderr, "error: mirror restore failed for %s: %s\n", runID, reason)
	}
}

func writeCampaignMirrorRestoreRunResult(stdout io.Writer, runID string, published int) {
	if published == 0 {
		_, _ = fmt.Fprintf(stdout, "Verified campaign mirror restore complete: run %s already present in public mirror (nothing to do).\n", runID)
		return
	}
	_, _ = fmt.Fprintf(stdout, "Restored run %s to public mirror (%d record(s)).\n", runID, published)
}

func writeCampaignMirrorRestoreInitSummary(stdout io.Writer, result *evaluation.CampaignMirrorReconcileResult) {
	if result == nil {
		return
	}
	restored := len(result.RestoredRunIDs)
	skipped := len(result.SkippedRunIDs)
	hostAbsent := len(result.HostAbsentRunIDs)

	switch {
	case restored > 0:
		_, _ = fmt.Fprintf(stdout, "Restored %d verified dataset(s) to public mirror.\n", restored)
	case skipped > 0:
		_, _ = fmt.Fprintf(stdout, "Verified campaign mirror already contains %d restorable verified dataset(s).\n", skipped)
	}
	if hostAbsent > 0 {
		writeHostAbsentMirrorNote(stdout, result.HostAbsentRunIDs)
	}
}

func writeHostAbsentMirrorNote(w io.Writer, runIDs []string) {
	if len(runIDs) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "Note: %d verified run(s) were skipped — local campaign artifacts under .g8e/data/eval/runs/ are missing (usually after a .g8e wipe).\n", len(runIDs))
	_, _ = fmt.Fprintln(w, "Mirror restore cannot recreate them from the queue. Do not run mirror restore again for these runs.")
	_, _ = fmt.Fprintln(w, "To republish them, run a new init campaign for each model:")
	_, _ = fmt.Fprintln(w, "  ./g8e eval campaign start --queue <variant_id> --prepare-only --publish")
	_, _ = fmt.Fprintln(w, "  ./g8e eval campaign execute --publish --daemon")
	_, _ = fmt.Fprintln(w, "  ./g8e eval campaign verify --require-provider-observation")
	_, _ = fmt.Fprintf(w, "Skipped run IDs: %s\n", formatRunIDList(runIDs))
}

func formatRunIDList(runIDs []string) string {
	if len(runIDs) == 0 {
		return "[]"
	}
	return "[" + strings.Join(runIDs, " ") + "]"
}
