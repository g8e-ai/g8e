---
doc_id: release_process
title: Release Process
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.1
owners:
  - VERSION
  - Makefile
  - CHANGELOG.md
  - protocol/python/pyproject.toml
  - protocol/python/g8e/__init__.py
  - protocol/python/uv.lock
  - protocol/docs/
  - internal/services/gateway/explorer/static/
  - internal/services/gateway/console/static/
  - internal/cli/cmd/compliance/
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/docs.md
  - docs/devs/troubleshooting.md
  - docs/guides/ux_smoke_test.md
when_to_read: Preparing, auditing, generating compliance evidence for, and executing a platform release.
do_not_use_for:
  - Coding invariants and style guidelines (docs/devs/devs.md)
  - Documentation audit workflow and documentation catalog (docs/devs/docs.md)
  - Test tiers and test execution (docs/devs/tests.md)
  - Runtime and package ownership maps (docs/devs/codemap.md)
---

# Release Process

## Purpose

Defines the release preparation, change inventory, documentation reconciliation, version synchronization, compliance evidence generation, and release workflow for the g8e platform and protocol packages.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Separation of duties](#separation-of-duties-inv-rel-duty), [Version synchronization](#version-synchronization-inv-rel-ver), [Release smoke evidence](#release-smoke-evidence-inv-rel-smoke), [Compliance evidence](#compliance-evidence-inv-rel-comp).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Separation of duties (`INV-REL-DUTY`)

| ID | Rule |
| --- | --- |
| INV-REL-DUTY-01 | The AI coding agent MUST NOT run `git commit`, `git push`, `gh pr create`, or `make release`. The agent prepares the working tree, audits docs, generates compliance evidence, and hands back the prepared tree to the release owner. |
| INV-REL-DUTY-02 | The release owner MUST review prepared changes, commit, open and merge the PR, pull the merged `main` branch locally, and execute `make release` to tag and push. |

### Version synchronization (`INV-REL-VER`)

| ID | Rule |
| --- | --- |
| INV-REL-VER-01 | `VERSION` is the single source of truth for the platform and protocol version (`vX.Y.Z\n`). |
| INV-REL-VER-02 | PR preparation MUST synchronize `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, the editable package entry in `protocol/python/uv.lock`, and the `Version: vX.Y.Z` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`. |
| INV-REL-VER-03 | `make proto-generate` MUST be run during release PR prep to regenerate downstream `ensemble/uv.lock` and protocol bindings so locked CI checks pass. |
| INV-REL-VER-04 | `CHANGELOG.md` MUST include a row under the minor-version section (`## vX.Y.x`) linking to the new release notes file `docs/release_notes/vX.Y.x/vX.Y.Z.md`. The first release of a minor version creates both the `## vX.Y.x` section, above the previous minor's, and the `docs/release_notes/vX.Y.x/` directory; `make release` derives the notes path from `VERSION` and aborts when the file is missing. |
| INV-REL-VER-05 | Before tagging, every verified evaluation campaign MUST state the release it measured: campaigns frozen by v2.3.0 or later record it in their digest; older campaigns MUST carry an asserted tag (`g8e eval campaigns tag --release vX.Y.Z <campaign>...`). `g8e eval campaigns list` shows any campaign whose release is `unknown`. |
| INV-REL-VER-06 | The Evaluation Explorer bundle compiles the current release from `VERSION` (`CURRENT_PLATFORM_RELEASE`). After changing `VERSION`, PR preparation MUST rebuild the bundle and refresh the Gateway embed (`cd evaluation-explorer && npm run build`, then `make explorer-embed`) and commit `internal/services/gateway/explorer/static/`. A stale embed defaults the Explorer's release filter to the previous release. |
| INV-REL-VER-07 | The console embed (`internal/services/gateway/console/static/`) carries no release string, but `make console-embed-check` MUST pass before the PR so the committed embed matches `console/` at the release commit. |

### Compliance evidence (`INV-REL-COMP`)

| ID | Rule |
| --- | --- |
| INV-REL-COMP-01 | Every release MUST include a signed, verified public compliance report bundle and its canonical projections (`vX.Y.Z-compliance-evidence.md` and `vX.Y.Z-compliance-evidence.csv`) in `docs/release_notes/vX.Y.x/`. |
| INV-REL-COMP-02 | Report trust policies and evidence trust policies MUST be external to the report bundle. Supplying external trust does not replace first-party engineering assessment. |
| INV-REL-COMP-03 | Compliance projections MUST match the canonical renderings in the bundle (`report.md` and `report.csv`). |

### Release smoke evidence (`INV-REL-SMOKE`)

| ID | Rule |
| --- | --- |
| INV-REL-SMOKE-01 | Release PR preparation MUST run the host-native core-platform smoke workflow in [Host-Native Headless UX Smoke Test](../guides/ux_smoke_test.md) against binaries built from the prepared working tree. |
| INV-REL-SMOKE-02 | The smoke run MUST pass both `ensemble-chat-file-create` and `ensemble-document-update`. The file scenario MUST return a completed signed `FILE_EDIT` receipt and exact governed read-back; the document scenario MUST return completed signed create and merge receipts and prove preserved fields by read-back. |
| INV-REL-SMOKE-03 | The data worker MUST run with a dedicated `--working-dir`, and `report all` MUST read that worker's populated `.g8e` stores. A report against Gateway state, an empty runtime, or stores combined from different runtime roots is not release evidence. |
| INV-REL-SMOKE-04 | Release smoke acceptance requires non-empty receipts, commitments, and file mutations; `commitment_chain`, `git_merkle_root`, `file_mutation_linkage`, and `receipt_commitment_crosslink` MUST be `PASS`; and the verification summary MUST contain zero `FAIL` and zero `SKIPPED` rows. |
| INV-REL-SMOKE-05 | Smoke artifacts are engineering acceptance evidence, not the signed public compliance bundle required by `INV-REL-COMP-01`. A passing smoke run does not replace release compliance preparation or expand the claims of the tested `doctrine` posture. |
| INV-REL-SMOKE-06 | The v2.3.2 tree removes the scenario runner that implemented `INV-REL-SMOKE-02`. `make ci` is not a substitute. Release preparation MUST restore a maintained equivalent runner or record the smoke gate as unresolved; it MUST NOT invoke historical `g8e demos` commands or claim smoke acceptance from them. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Repository version | `VERSION` | `cat VERSION` |
| Release index | `CHANGELOG.md` | Table row under `## vX.Y.x` |
| Python protocol version | `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock` | Version match with `VERSION` |
| Protocol spec headers | `protocol/docs/a2a.md`, `constants.md`, `mcp.md`, `spec.md` | `Version: vX.Y.Z` header |
| Downstream lockfile | `ensemble/uv.lock` | Matches `g8e` version |
| Explorer embed | `internal/services/gateway/explorer/static/` | `grep -l "$(cat VERSION)" internal/services/gateway/explorer/static/assets/*.js` lists the bundle |
| Console embed | `internal/services/gateway/console/static/` | `make console-embed-check` |
| Core-platform smoke runbook | `docs/guides/ux_smoke_test.md` | Currently retired; replacement required by INV-REL-SMOKE-06 |
| Smoke evidence artifacts | `reports/localhost-smoke/` | Audit export plus worker-local manifest, verification summary, receipts, commitments, and file mutations |
| Compliance projection | `docs/release_notes/vX.Y.x/vX.Y.Z-compliance-evidence.md`, `.csv` | `g8e compliance release-evidence` |
| Tag and push target | `Makefile` (`make release`) | Release owner only on merged `main` |

## Procedures

### Agent PR Preparation Sequence

For campaign evidence, check the recorded release in the protected `campaign-spec.json`. Compliance capture preserves that digest-bound release and source revision and excludes operator `release-tag.json` assertions. Campaign exports report the release and its recorded/asserted/unknown basis in `run_summary.json` and SQLite metadata; the exporting build does not fill in an unknown historical release. See [Evaluation Architecture](../architecture/evals.md) for the export contract.

Public campaign records and the Explorer also distinguish recorded, asserted, and unknown release provenance. The Explorer's current-release default comes from `VERSION` at bundle build time, so rebuild and embed it after changing `VERSION`. Tag an older campaign only from evidence of its producing release; otherwise re-run it. To refresh an already published run after adding or correcting an asserted tag, use `g8e public restore --run-id <run> --force`: this clears host publication idempotency and appends current projections without modifying the campaign digest or its stored scores.

1. **Determine change inventory:** Diff `v<prev-tag>..HEAD` to identify all added, changed, removed, fixed, and security-sensitive features.
2. **Reconcile documentation:** Walk the documentation catalog in `docs/devs/docs.md`, audit affected documents end-to-end against production code, and update stale cross-links.
3. **Draft release notes:** Create `docs/release_notes/vX.Y.x/vX.Y.Z.md` following the template with Overview, Added, Changed, Removed (when anything is removed), Fixed, Tests, Documentation, Compliance Evidence, and Deferred sections.
4. **Set version:** Update `VERSION` to `vX.Y.Z\n`.
5. **Sync Python package & protocol specs:** Update `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock`, and the `Version: vX.Y.Z` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`.
6. **Regenerate downstream lockfiles:** Run `make proto-generate` to update `ensemble/uv.lock` and protobuf bindings.
7. **Update CHANGELOG:** Add the release table row under `## vX.Y.x` in `CHANGELOG.md`. For the first release of a new minor version, add the `## vX.Y.x` section with its table header above the previous minor and create `docs/release_notes/vX.Y.x/`.
8. **Refresh embedded frontends:** Run `cd evaluation-explorer && npm run build`, then `make explorer-embed` (INV-REL-VER-06), and run `make console-embed-check` (INV-REL-VER-07). `make build` runs `make explorer-embed` and `make console-embed`, but only copies a `dist/` that already exists.
9. **Resolve the core-platform smoke gate:** The former [Host-Native Headless UX Smoke Test](../guides/ux_smoke_test.md) is retired because v2.3.2 removes its `g8e demos` scenario runner. Restore a maintained equivalent that satisfies `INV-REL-SMOKE-02` through `INV-REL-SMOKE-04`, or record the gate as unresolved and do not claim release readiness. `make ci`, historical scenario output, an empty `report all`, or manually assembled stores do not satisfy this gate.
10. **Generate and verify compliance evidence:** From the repository root, run `./g8e compliance release-prepare` (add `--new-key` on the first run to create the report-signing key under `.g8e/secrets/`; later runs reuse it; `--signing-metadata` and `--signing-private-key` override the location together). No other flags or manual steps are needed. The command copies the running Gateway's `g8e.db` (and its `-wal` and `-shm` files when present) out of the `g8e-gateway` container into `release-evidence/vX.Y.Z/gateway-snapshot/` (`--db <path>` assesses a local copy instead), derives the protected scope with `ProductVersion = "X.Y.Z"` from `VERSION`, git, and the Gateway image digest, exports the assessment window, builds the external report and evidence trust policies, generates a public bundle, verifies it offline, and projects the verified `report.md` and `report.csv` into `docs/release_notes/vX.Y.x/`. The recorded source revision is the git `HEAD` of `--repo-root`, so the running Gateway MUST be built from that `HEAD` (`make build`, then `./g8e docker rebuild`; confirm with `g8e version` inside the Gateway container) and the assessment window MUST contain governed activity from that build; the automatic window fails when it holds no receipts. Uncommitted release changes are never part of the recorded revision. The command stops before projection when verification fails. Run `./g8e compliance report generate`, `report verify`, and `release-evidence` individually only for a scope authored by hand. Record the bundle path, verification result, and checksum root that the command prints in the release notes.
11. **Finalize document metadata:** Update `last_updated` and `version` on every audited document.
12. **Verification checks:** Run `./g8e test lint`, `./g8e test unit`, `make console-lint console-test console-embed-check`, `make constants-check`, `make swagger-generate` (expect no diff), and focused package integration tests. Record in the release notes what ran and what did not.

### Release Owner Tagging and Publication (Post-Merge)

```bash
# On merged main branch with clean tree:
git checkout main && git pull
make release
```

`make release` rewrites any out-of-sync Python package version or protocol spec header to match `VERSION`, then aborts if that leaves the working tree dirty. It also aborts when the release notes file is missing or either tag already exists. Otherwise it creates tags `vX.Y.Z` and `protocol/vX.Y.Z` and pushes them to origin. GitHub Actions workflows build binaries, sign assets, create the GitHub release, and publish to PyPI.

## Anti-patterns

- Agent running `git commit`, `git push`, or `make release` (INV-REL-DUTY-01).
- Running `make release` from a feature branch or with unmerged changes (INV-REL-DUTY-02).
- Updating document metadata without auditing the entire document against code (INV-DOC-FMT-02 in [Documentation Guide](docs.md)).
- Recording a source revision for a Gateway that was built from a different commit (INV-REL-COMP-01).
- Hand-editing compliance projections instead of generating through `g8e compliance release-evidence` (INV-REL-COMP-01, INV-REL-COMP-03).
- Missing `make proto-generate` to update `ensemble/uv.lock` after bumping Python version (INV-REL-VER-03).
- Bumping `VERSION` without rebuilding the Explorer embed, which leaves its release filter on the previous release (INV-REL-VER-06).
- Adding a release row to an `## vX.Y.x` section that does not exist yet instead of creating the new minor section (INV-REL-VER-04).
- Invoking removed historical `g8e demos` scenario commands or treating `make ci` as release smoke acceptance (INV-REL-SMOKE-06).
- Treating scenario exit status alone as smoke acceptance without checking signed receipts, governed read-back, populated worker-local stores, and every verification row (INV-REL-SMOKE-02, INV-REL-SMOKE-04).
- Generating a release smoke report from the repository or Gateway `.g8e` tree when the mutation executed in a dedicated data worker runtime (INV-REL-SMOKE-03).
- Substituting smoke CSVs for the signed and offline-verified public compliance bundle (INV-REL-SMOKE-05, INV-REL-COMP-01).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting standards.
- [Testing Guide](tests.md): test execution tiers and verification commands.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Host-Native Headless UX Smoke Test](../guides/ux_smoke_test.md): detailed service setup, live scenarios, audit queries, and worker-local evidence validation.
