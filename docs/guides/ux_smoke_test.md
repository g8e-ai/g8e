---
doc_id: ux_smoke_test
title: Host-Native Headless UX Smoke Test
audience: developers, evaluators, compliance auditors
status: retired
last_updated: 2026-10-06
version: v2.3.1
owners:
  - docs/guides/ux_smoke_test.md
related:
  - docs/devs/release_process.md
  - docs/devs/tests.md
  - docs/architecture/evals.md
when_to_read: Determining whether the former host-native release smoke workflow is available in the current tree.
do_not_use_for:
  - Release acceptance evidence
  - Assessment authorization or certification
---

# Host-Native Headless UX Smoke Test

## Status

This workflow is retired in the current tree. v2.3.2 removes the `g8e demos` command group and `internal/tools/agent_harness/scenarios/`, including the `ensemble-chat-file-create` and `ensemble-document-update` runners previously documented here. The shared typed harness client remains, but it is not a runnable replacement for those scenarios.

Do not copy the pre-v2.3.2 commands from historical release notes or treat `make ci` as equivalent runtime smoke evidence. A future replacement must provide maintained entry points for the intended ingress paths, exact governed read-back, correlated signed receipts, worker-local commitments and file mutations, and deterministic evidence export before this guide can return to `current` status.

## Current alternatives

- Use [Testing](../devs/tests.md) for the maintained unit, integration, E2E, component, and CI suites.
- Use [Evaluation Architecture](../architecture/evals.md) for maintained evaluation campaigns and verification.
- Use [Release Process](../devs/release_process.md) for the release gate and its explicit unresolved smoke requirement.
- Use `g8e report all` only for evidence produced by an actual governed workload; an empty or unrelated runtime is not release evidence.

## Historical scope

The retired workflow started a localhost Gateway, Ensemble, and dedicated data Operator, then ran a governed file mutation and Gateway-local document merge. It inspected audit records and generated a worker-local report. Historical evidence and release notes retain their original scope, but the removed runner cannot be invoked from this tree.
