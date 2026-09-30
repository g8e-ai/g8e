// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Worker 1A: Unified verdict strip and clickable criteria
// Merges grade chips from AssignmentDetailView with scenario criteria lookup.
// Note: Header "lane" summary (formation vs homogeneous) was skipped per plan instructions
// as it cannot be cleanly derived from existing AssignmentResult fields without guessing.

import { useState } from 'react';
import type { AssignmentResult, PublicScenarioCriterion } from '../contract/types';
import { DetailRow } from './shared';
import { assignmentGradeChips } from '../views/derived';

function statusTone(status: string): string {
  if (status === 'pass') return 'ok';
  if (status === 'fail') return 'critical';
  if (status === 'unavailable') return 'info';
  if (status === 'unsupported' || status === 'invalid_evidence') return 'warn';
  return 'neutral';
}

function statusLabel(status: string): string {
  return status.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

export function UnifiedGradeBadges({ assignment }: { assignment: AssignmentResult }) {
  const gradeChips = assignmentGradeChips(assignment);
  const scenarioCriteria = assignment.scenario_summary?.criteria ?? [];

  // Map criterion_id to PublicScenarioCriterion for quick lookup
  const criterionMap = new Map<string, PublicScenarioCriterion>(
    scenarioCriteria.map((c) => [c.criterion_id, c])
  );

  return (
    <div className="unified-grade-badges" aria-label="Grade badges">
      {gradeChips.map((grade) => {
        const criterion = criterionMap.get(grade.criterion_id);
        const tone = statusTone(grade.status);
        const [isExpanded, setIsExpanded] = useState(false);

        return (
          <details
            key={grade.criterion_id}
            className={`grade-badge-details grade-${grade.status}`}
            open={isExpanded}
            onToggle={(e) => setIsExpanded(e.currentTarget.open)}
          >
            <summary className={`grade-badge-summary tone-${tone}`}>
              <span className="badge-label">
                {criterion?.public_label ?? grade.criterion_id.replace(/-/g, ' ')}
              </span>
              <span className="badge-status" aria-label={statusLabel(grade.status)}>
                {statusLabel(grade.status)}
              </span>
            </summary>
            <div className="grade-badge-details-panel">
              {criterion ? (
                <>
                  <DetailRow label="Status">{statusLabel(grade.status)}</DetailRow>
                  <DetailRow label="Grading method">
                    {criterion.grading_method === 'deterministic' ? 'Deterministic check' : 'Semantic judge'}
                  </DetailRow>
                  <DetailRow label="Required">{criterion.required ? 'Yes' : 'No'}</DetailRow>
                  {criterion.public_description ? (
                    <DetailRow label="Description">{criterion.public_description}</DetailRow>
                  ) : null}
                </>
              ) : (
                <DetailRow label="Status">{statusLabel(grade.status)}</DetailRow>
              )}
              {grade.explanation ? (
                <DetailRow label="Explanation">{grade.explanation}</DetailRow>
              ) : null}
            </div>
          </details>
        );
      })}
    </div>
  );
}
