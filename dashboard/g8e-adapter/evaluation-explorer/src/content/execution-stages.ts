// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// The assignment-to-projection pipeline. Shared by the Docs page (full
// pipeline) and the Overview "What am I looking at?" guide (condensed).

export const EXECUTION_STAGES = [
  {
    label: 'Freeze',
    system: 'Campaign controller',
    detail: 'Binds the scenario catalog, model registry, role, repetitions, and exact Operator sessions before work starts.',
    output: 'Campaign + assignment identity',
  },
  {
    label: 'Admit',
    system: 'Gateway · PDP',
    detail: 'Authenticates ingress, constructs or verifies the GovernanceEnvelope, and applies posture-required L1–L3 policy.',
    output: 'State-bound envelope',
  },
  {
    label: 'Execute',
    system: 'Operator · PEP',
    detail: 'The exact bound Operator independently re-runs L1–L4, then performs L5 against its own runtime boundary.',
    output: 'Signed local receipt',
  },
  {
    label: 'Witness',
    system: 'Observer + Provenance',
    detail: 'Separate sessions bind GPU/RAM samples and model-weight hashes to the provider attempt without executor self-report.',
    output: 'Observation windows',
  },
  {
    label: 'Verify',
    system: 'Offline verifier',
    detail: 'Recomputes digests, signatures, bindings, populations, verdicts, and metrics without executing another mutation.',
    output: 'report.json + verification.json',
  },
  {
    label: 'Project',
    system: 'Public mirror',
    detail: 'Emits an allowlisted, public-safe projection. Private prompts, outputs, identities, paths, and receipt bodies stay owner-local.',
    output: 'Bootstrap + history + SSE',
  },
] as const;
