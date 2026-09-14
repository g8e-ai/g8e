// Plain-language inline definitions for metric and quality terms. The Term
// component renders a focusable abbreviation with the definition in its
// title; the methodology page carries the same definitions in full so no
// essential information requires hover or a pointing device.

import type { ReactNode } from 'react';

export const TERM_DEFINITIONS = {
  pass_rate:
    'The fraction of eligible assignments that passed. The interval is a bootstrap confidence bound over tasks, not a superiority claim.',
  confidence_interval:
    'A 95% bootstrap interval over per-task means. It expresses sampling uncertainty; overlapping intervals mean the data cannot separate the models.',
  pairwise_agreement:
    'How often two repetitions of the same task produce the same outcome. Higher means more consistent results.',
  all_five_agreement:
    'How often all five repetitions of a task produce the same outcome. Stricter than pairwise agreement.',
  consistently_correct:
    'Tasks where every observed repetition passed.',
  consistently_wrong:
    'Tasks where every observed repetition failed.',
  inconsistent:
    'Tasks where repetitions disagreed — the model passed on some attempts and failed on others.',
  insufficient:
    'Tasks with too few eligible observations to classify. Missing resource observations land here.',
  ttft: 'Time to first token. Not observed at the current observation boundary, so it renders as Unavailable rather than zero.',
  throughput:
    'Output tokens generated per second while the model was responding. Higher means faster generation.',
  denominator:
    'The count of eligible units a rate is computed over — assignments with an observed pass metric, or tasks for bootstrap intervals. Never inflated by repetitions.',
  verification_status:
    'The canonical verifier disposition for a suite or run. Failed means the report did not pass verification — the measured values are still shown, labeled exploratory partial.',
  missingness:
    'Why a metric was not observed or is not applicable. Missing values are disclosed, never rendered as zero.',
  repeatability:
    'How consistently a model produces the same outcome across repetitions of the same task.',
} as const;

export type TermKey = keyof typeof TERM_DEFINITIONS;

export function Term({ term, children }: { term: TermKey; children?: ReactNode }) {
  return (
    <abbr
      className="term"
      title={TERM_DEFINITIONS[term]}
      tabIndex={0}
      style={{ textDecoration: 'underline dotted', textUnderlineOffset: '3px', cursor: 'help' }}
    >
      {children ?? term.replace(/_/g, ' ')}
    </abbr>
  );
}
