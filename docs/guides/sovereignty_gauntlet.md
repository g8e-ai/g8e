---
doc_id: sovereignty_gauntlet
title: Sovereignty Gauntlet Evidence Guide
audience: maintainers, evaluators, and publication reviewers
status: retired
last_updated: 2026-10-06
version: v2.3.2
owners:
  - docs/guides/sovereignty_gauntlet.md
related:
  - docs/architecture/evals.md
  - docs/reference/compliance-evidence.md
  - docs/devs/release_process.md
when_to_read: Interpreting historical Sovereignty Gauntlet claims or selecting a maintained evidence workflow.
do_not_use_for:
  - Running removed organization-specific demos
  - Current release acceptance
---

# Sovereignty Gauntlet Evidence Guide

## Status

The original Sovereignty Gauntlet runbook is retired. It depended on the removed DHS and FedRAMP demo stacks, `g8e demos` scenario commands, and `g8e compliance demo-run verify`. Those commands and their implementation owners are not present in the current tree.

Historical release notes and retained evidence remain scoped to the version and run that produced them. Do not replay their commands against v2.3.2, describe the removed synthetic demos as current product capabilities, or convert historical demonstration results into a current release claim.

## Maintained evidence paths

- Evaluation campaigns and their release provenance are documented in [Evaluation Architecture](../architecture/evals.md).
- Signed compliance reports, external trust policies, operational sources, and offline verification are documented in [Compliance Evidence](../reference/compliance-evidence.md).
- Release-specific evidence requirements and separation of duties are documented in [Release Process](../devs/release_process.md).

Any future flagship campaign needs a new maintained runner, preregistered scope, preserved source artifacts, explicit trust policy, reproducible verification, and publication rules grounded in the artifacts actually produced by that runner.
