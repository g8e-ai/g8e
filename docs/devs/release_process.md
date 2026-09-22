# g8e Release Process

Last Updated: 2026-09-21

The primary purpose of a release is to inventory every change since the last release and ensure that all affected documentation accurately reflects the current state of the code. Version bumps and CHANGELOG entries follow this documentation reconciliation; they do not replace it.

The [Developer Guidelines](devs.md) define the repository-wide engineering rules that every release follows. Their documentation rule points to the [Documentation Guide](docs.md), whose [Documentation Catalog](docs.md#documentation-catalog) enumerates every first-party prose, generated, machine-readable, component, evidence, and historical documentation surface that may require an update. A release preparer reviews that complete catalog against the change inventory instead of relying on a fixed list in this document.

The protocol Go and Python packages and the platform binary share the same version number. There are no independently versioned protocol releases.

> **`make release` handles version syncing, tagging, and pushing.** It does NOT build binaries, run lint or tests, or create GitHub releases; CI and GitHub Actions workflows handle those. Release prep changes are committed and opened as a PR; after merge, pull main and run `make release` to tag and push. See [Release Workflow](#release-workflow).

## Release Checklist

Each release uses the [Standard Checklist Template](#standard-checklist-template) below. Check items off in a **version-specific working tracker** (for example `.local.dev/docs/plans/in-progress/vX.Y.Z-readiness.md`), not in this file. This document stays release-agnostic; when a release ships, archive or delete that tracker — do not replace prose here with the next version's checked boxes.

Large releases also require the conditional gates in [Large Release Gates](#large-release-gates). Complete every standard item plus every large-release item that applies to the change inventory.

---

## Separation of Duties

Release work is split between the **agent** (PR prep) and the **release owner** (merge, CI gate, tag/push). The agent never runs `make release`, never commits, and never pushes.

**Agent (PR prep, on a feature branch):**
1. Receive the release owner's complete change inventory and inspect the current working tree as the source of truth for current behavior
2. Map every change to the full documentation catalog, then audit each affected document end to end against its owning current code, configuration, schema, generator, test, or scope-bound evidence
3. Update every inaccurate or incomplete affected document and all related current-state cross-links, record the audit, and write `docs/release_notes/vX.Y.x/vX.Y.Z.md`; defer document metadata until the release version is set
4. Generate the compliance evidence artifacts as part of agent prep: establish or use the release assessment binding, run `g8e compliance release-evidence` with the release version, output directory, assessment binding, and evidence-window flags, and retain the generated Markdown and CSV in `docs/release_notes/vX.Y.x/` (see [Compliance Evidence Generation](#compliance-evidence-generation)). The agent owns this generation step; do not defer it to the release owner or leave placeholder release notes.
5. Set `VERSION` to `vX.Y.Z`, finalize metadata for every edited document, add the `CHANGELOG.md` row, and sync the Python package files (`protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and the editable `g8e` package entry in `protocol/python/uv.lock`) to `X.Y.Z` (no `v` prefix). Then run `make proto` to regenerate the downstream `ensemble/uv.lock` file, which depends on `g8e` through the in-tree protocol package, so CI's version sync and locked-environment checks pass on the PR
6. Run the read-only [Verification](#verification) checks (all steps should pass, including step 4)
7. Stop. The agent does NOT commit, push, open the PR, or run `make release`. Hand the prepared working tree back to the release owner.

**Release owner (release range, commits, merges, tags, pushes):**
1. Establish the previous-to-current release range, provide the complete change and changed-file inventory to the agent, and identify any release-specific evidence requirements
2. Review the prepared code, documentation reconciliation record, release notes, and verification results, then `git add`, `git commit`, `git push`, and open the PR on GitHub
3. Merge the PR on GitHub
4. Wait for CI on `main` to pass (lint, tests, version sync checks)
5. `git checkout main && git pull` locally
6. Run `make release` — this re-syncs the Python package files from `VERSION` (a no-op if the agent already synced them), then creates and pushes the `vX.Y.Z` and `protocol/vX.Y.Z` tags
7. GitHub Actions workflows create the GitHub release, build and sign binaries, and publish the Python package to PyPI

The Python package files (`pyproject.toml`, `__init__.py`, and the editable package entry in `protocol/python/uv.lock`) are synced to the new version during PR prep by the agent (manually, since the agent cannot run `make release`) so CI's version sync and locked-environment checks pass on the PR. The downstream `ensemble/uv.lock` file is regenerated by `make proto` during PR prep for the same reason. `make release` re-syncs the `protocol/python` files after merge as a no-op safety net. The agent never runs `make release`, never commits, and never pushes.

## How to Use This Document

Work through the [Standard Checklist Template](#standard-checklist-template) in a version-specific working tracker. The sections below explain each checklist item in detail:

1. **[Change Inventory](#change-inventory)** — Release owner establishes the range and categorizes every change. Everything else depends on this.
2. **[Documentation Reconciliation](#documentation-reconciliation)** — Map changes through the full [Documentation Catalog](docs.md#documentation-catalog); audit every affected document end to end.
3. **[Release Notes](#release-notes)** and **[Compliance Evidence Generation](#compliance-evidence-generation)** — Permanent record and per-release compliance artifacts.
4. **[Version-Bearing Files](#version-bearing-files)** — Set `VERSION`, sync Python files, run `make proto`, update `CHANGELOG.md`.
5. **[Verification](#verification)** — Read-only checks that supplement the audit record.
6. **[Separation of Duties](#separation-of-duties)** — Agent stops at handoff; release owner commits, merges, and runs `make release`.
7. **[Release Workflow](#release-workflow)** — Tag push and GitHub Actions asset publication.

For large releases, also review [Large Release Gates](#large-release-gates).

---

## Change Inventory

**This is the first and most important step.** You cannot write accurate release notes or update documentation without a complete inventory of what changed.

### Determine the Release Range

The release owner finds the previous release tag, diffs the range to `HEAD`, and gives the agent the complete changed-file and change inventory. Repository history establishes release scope; the current working tree remains the source of truth for documented behavior:

```bash
# Find the previous release tag
git tag -l 'v*' --sort=-version:refname | head -5

# If the tag exists locally:
git log --oneline v1.6.2..HEAD

# If the tag isn't fetched yet, use the commit SHA from the CHANGELOG or git log:
git log --oneline <prev-release-commit>..HEAD
```

### Categorize Every Change

The release owner reviews each commit in the release range and categorizes it. The resulting inventory must identify every changed file and affected public or internal contract before documentation reconciliation begins:

```bash
# List all files changed
git diff --name-only <prev-tag>..HEAD

# Review each commit's diff in detail
git log --format='%H %s%n%b---' <prev-tag>..HEAD

# Get diff stats for a quick overview
git diff --stat <prev-tag>..HEAD
```

Categorize changes into:

- **Added**: New files, new functions, new features, new config options, new endpoints
- **Changed**: Refactored code, renamed files, changed signatures, changed behavior
- **Removed**: Deleted files, removed functions, removed endpoints, removed config
- **Fixed**: Bug fixes, corrected behavior
- **Security**: Security-related changes
- **Breaking**: Anything that requires user action or breaks compatibility

For each change, note:

1. **What changed** (file path, function/type name, old → new)
2. **Whether it's user-visible** (API surface, CLI flags, config keys, behavior) or internal-only
3. **Which docs reference the changed thing**: grep the docs for identifiers that were renamed, removed, or changed

### Find Stale Doc References

For every renamed, removed, or changed identifier, search the docs:

```bash
# Search docs for identifiers that were removed or renamed
grep -rnE 'OldName|old_command|OLD_CONSTANT|removedEndpoint' docs/ protocol/docs/ --include='*.md'

# Find explicit g8e version pins in maintained first-party documentation. Classify each
# match as current-state installation guidance, generated content, or historical evidence.
grep -rnE "g8e==[0-9]+\.[0-9]+\.[0-9]+|g8e-ai/g8e/v2@v[0-9]+\.[0-9]+\.[0-9]+" docs/ protocol/ dashboard/ ensemble/ demos/ internal/adapters/ README.md .github/ --include='*.md' --include='*.tmpl' \
  | grep -v release_notes | grep -v CHANGELOG

# Check which docs were modified in the release range
git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/
```

Any current-state document that references removed or renamed behavior is stale and must be updated. Any affected public behavior without an owning document is missing and must be added to the appropriate existing documentation surface. The version-pin search is a candidate inventory, not an edit list: classify each match against the document's purpose and the release change, and update a current-state installation callout only when the complete-document audit confirms that it is intended to name the current release. Historical release notes, evidence-bearing documents, and unrelated accurate guides retain their declared version and metadata.

---

## Documentation Reconciliation

This is the core work of a release. The change inventory from the previous section tells you what changed; now make the docs match.

### When to Update a Doc

**Update a doc when one of these conditions is true:**

1. **Inaccuracy**: The doc describes something that is no longer true (a renamed command, a removed endpoint, a changed default, a corrected behavior). Fix the inaccurate prose so it matches the code.
2. **Missing feature**: The release adds a user-visible feature, command, endpoint, or config option that has no documentation at all. Add the missing documentation.
3. **Stale reference**: The doc references something that was removed or renamed in this release (e.g., a deprecated alias, a deleted route, a renamed constant, a deleted file). Remove or update the reference.
4. **Stale current-version callout**: A maintained current-state document contains a `go get ...@vX.Y.Z`, `pip install g8e==X.Y.Z`, `pip download g8e==X.Y.Z`, or other install command that is intended to name the current release but still names the prior release. Audit the complete document, correct the callout, reconcile related documents, validate the result, and update metadata last. Historical and evidence-bearing documents retain the version in their declared scope.

**Do NOT update a doc when:**

- The doc is already accurate; even if the underlying code was refactored internally, if the user-facing behavior and interface are unchanged, the doc is fine as-is.
- The doc wasn't touched by any code change in this release and contains no stale version callouts; leave its version header and content alone.
- You're tempted to "improve" prose that isn't wrong; cosmetic rewrites are not part of the release process.

> **Scope narrowly, audit completely.** The change inventory determines which documents are affected. Once a document is in scope or receives any edit, review it from beginning to end against the current implementation and correct every factual or structural defect found. Do not use cosmetic churn as a substitute for completeness.

### What to Review

Start with the [Developer Guidelines](devs.md), then walk every category in the [Documentation Catalog](docs.md#documentation-catalog). The catalog is the authoritative inventory; this release process does not maintain a second list that can drift. For each release change, consider all of these ownership classes:

- Repository entry points, contribution and security policy, legal surfaces, and the generated root README.
- Platform concept, architecture, guide, reference, developer, diagram, demo, adapter, Dashboard, Ensemble, and protocol documentation.
- Component READMEs, indexes, contribution guides, changelogs, examples, and package documentation.
- Protobuf comments and generated API references, Swagger annotations and generated OpenAPI, JSON registries and schemas, compliance catalogs, native evaluation evidence, signed compliance evidence, and website output.
- Current release notes, compliance evidence, and other scope-bound release artifacts. Historical release notes remain immutable unless a clearly identified correction is required.

Map changes by impact, not only by matching file names. A changed route can affect authentication architecture, protocol documentation, a task guide, Swagger, examples, tests, and the README. A changed component capability can affect its component index, platform overview, deployment guide, and cross-links even when none contains the changed Go or Python symbol.

### How to Reconcile

For each document that falls in scope or receives any edit, follow the complete [End-to-End Audit Workflow](docs.md#end-to-end-audit-workflow). The triggering stale sentence is only the starting point; the review covers the document's title, metadata, prose, tables, examples, diagrams, links, generated sections, behavioral claims, commands, paths, signatures, security boundaries, evidence limits, and related-document summaries.

1. Read the entire document before editing and classify its purpose.
2. Verify every claim against the owning current code, configuration, schema, registry, generator, test, or scope-bound evidence. Existing prose is never evidence of current behavior.
3. Update every inaccurate, incomplete, duplicated, or stale part of the document, not only the change that first brought it into scope.
4. Search related current-state documents for the same concept. Update affected summaries and cross-links, choose one canonical explanation, and remove contradictions.
5. Run the generator or focused validation owned by the changed surface, then read the final handwritten and generated output end to end.
6. Update `Last Updated` and `Version` metadata last, after the audit and related-document reconciliation are complete, using the exact value in `VERSION` and preserving the document's existing metadata format.
7. Record the document, owning sources inspected, related documents checked, and validations run for release-owner review.

A document that was evaluated for scope but did not require an edit keeps its existing metadata. A document that was edited for any reason receives the complete audit; no wording-only, link-only, metadata-only, or generated-section exception exists.

---

## Release Notes

Create a new release notes file for every release. This is where the change inventory becomes a permanent record.

| # | File | Action |
|---|------|--------|
| 1 | `docs/release_notes/vX.Y.x/vX.Y.Z.md` | Create new file in the minor-version subdirectory (e.g., `v1.3.x/v1.3.1.md`) |

The release notes file should mirror the CHANGELOG entry but can be more detailed. Include the version header and date, an Overview, all applicable Keep-a-Changelog sections, and any additional context, examples, or links to relevant documentation. Past release notes are immutable; never edit historical entries.

#### Release Notes Template

Use the same `## [X.Y.Z] - YYYY-MM-DD` header as the CHANGELOG (no `v` prefix in the bracket). Only include the subsections that apply to the release; most releases use 2-4 of these, not all of them.

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Overview

[Brief summary of the release — 2-5 sentences highlighting major changes, themes, and motivation]

### Breaking Changes

* **Change title** — Description of breaking change and migration path

### Added

* **Feature title** — Description of new feature

### Changed

* **Change title** — Description of modification

### Deprecated

* **Item title** — What is deprecated and the recommended replacement

### Removed

* **Item title** — What was removed and why

### Fixed

* **Bug title** — Description of fix

### Security

* **Security title** — Description of security improvement

### Deferred

* **Item title** — What was considered but deferred, and why

### Tests

* Description of significant test coverage additions or refactors
```

**Conventions observed across existing release notes:**

- **Bullet format**: `* **Item title** — Description` using an em-dash (`—`) separator between the bold title and description. Sub-bullets use `- ` for nested detail.
- **Section selection**: Only include sections that have content. Common combinations: small fix releases use just `Overview` + `Fixed`; feature releases use `Added` + `Changed` + `Fixed`; cleanup releases use `Removed` + `Changed` + `Fixed`.
- **Optional sections**: `Deferred`, `Tests`, `Documentation`, `Dependencies`, and `Migration Notes for External Consumers` are not part of the Keep-a-Changelog standard but have been used in past release notes when those categories carry significant content. Use them when appropriate.
- **Detail level**: Release notes can be more verbose than the CHANGELOG entry; include file paths, function names, and links to relevant docs where helpful.
- **Trailing separator**: Some release notes end with a `---` horizontal rule. This is optional.

---

## Compliance Evidence Generation

Every release generates a per-release compliance evidence artifact that captures demonstrated technical control operation at the release boundary. The artifact is produced by `g8e compliance release-evidence` and lives alongside the release notes in `docs/release_notes/vX.Y.x/`.

### What it captures

The command runs three currently-available compliance evidence sources and renders them into a single markdown report and a CSV:

1. **KSI evaluation** — evaluates class C against the live runtime stores (audit store, git ledger, commitment ledger) under the caller-declared scope, run, assertion-assessment set, and evidence window, then records the per-KSI status, method count, and last-validated timestamp. When the runtime stores are unavailable, the report records the gap honestly rather than inventing a passing result.
2. **KSI history snapshot inventory** — reads previously persisted KSI evaluation snapshots from `.g8e/data/compliance/ksi-history/` and records the snapshot count and time range. Release-evidence aggregation is read-only and does not persist its current KSI evaluation as a history snapshot.
3. **Demo-run verification** — runs `g8e compliance demo-run verify <run-id>` for each persisted demo evidence run (or for explicit run IDs passed via `--demo-run`) and records the verification result, failure count, verifier ID and version, reproduced checksum root, and verified-at timestamp.

`g8e compliance report generate` produces signed bundles with canonical cross-framework analysis, OSCAL and other deterministic renderers, protected source inventories, and independent offline replay. It requires one protected `AssessmentScope` supplied with `--scope`; that scope binds source admissions, runtime ownership and acquisition boundaries, verifier versions, applicability, selected population, posture, evidence window, and assessment-as-of time. Required contextual evidence is either protected in the scope or represented by an explicit typed unavailable declaration with a reason. An unavailable declaration limits the release claim and never replaces a posture-required policy or native proof. `g8e compliance report verify` authenticates report signatures through external report trust and separately authenticates represented commitment and customer or assessor attestation signers through external evidence trust. When a release plan requires clean offline acceptance, the acceptance record identifies the exact candidate, bundle checksum root, external trust digests, isolated network-disabled environment, command status, canonical verification-report digest, and rejected mutation classes. The legacy `compliance release-evidence` artifact remains a separate KSI, KSI-history, and demo summary until the canonical verify-and-project release path replaces it; it does not substitute for a signed report bundle and does not claim certification, accreditation, authorization, legal compliance, or recurring operating effectiveness. See the [Proof-Backed Compliance Evidence](../reference/compliance-evidence.md) document for the current bundle contract and remaining limits.

### Output files

Two files are written into the output directory (typically `docs/release_notes/vX.Y.x/`):

| File | Format | Purpose |
|------|--------|---------|
| `vX.Y.Z-compliance-evidence.md` | Markdown | Readable report with summary table, KSI evaluation table, KSI history summary, demo-run verification table, and claim boundaries |
| `vX.Y.Z-compliance-evidence.csv` | CSV | One row per evidence item (KSI results and demo-run verifications) with columns: `evidence_type`, `identifier`, `status`, `valid`, `method_count`, `last_validated`, `failure_count`, `verifier_id`, `verifier_version`, `checksum_root`, `evaluated_at` |

### Command

```bash
export RELEASE_SCOPE_ID='<assessment-scope-id>'
export RELEASE_RUN_ID='<assessment-run-id>'
export ASSERTION_ASSESSMENT_ID='<consumed-assertion-assessment-id>'
export EVIDENCE_WINDOW_START_UNIX_MS='<inclusive-start-unix-ms>'
export EVIDENCE_WINDOW_END_UNIX_MS='<inclusive-end-unix-ms>'

g8e compliance release-evidence \
  --version vX.Y.Z \
  --out docs/release_notes/vX.Y.x/ \
  --class C \
  --scope-id "${RELEASE_SCOPE_ID}" \
  --run-id "${RELEASE_RUN_ID}" \
  --assertion-assessment-id "${ASSERTION_ASSESSMENT_ID}" \
  --evidence-window-start-unix-ms "${EVIDENCE_WINDOW_START_UNIX_MS}" \
  --evidence-window-end-unix-ms "${EVIDENCE_WINDOW_END_UNIX_MS}"
```

The scope ID, run ID, assertion-assessment IDs, and inclusive evidence window are declared by the release assessment and must identify the actual assessment context. Repeat the assertion-assessment, attempt, scenario, and action flags for every identifier admitted into that context. Evidence references carrying an identifier outside these allowlists or a timestamp outside the declared window fail closed.

Flags:

- `--version` (required): Release version with `v` prefix (e.g. `v2.1.5`).
- `--out` (required): Output directory for the markdown report and CSV. Typically `docs/release_notes/vX.Y.x/`.
- `--scope-id` (required): Assessment scope ID bound to every KSI result.
- `--run-id` (required): Assessment run ID bound to every KSI result.
- `--assertion-assessment-id` (required, repeatable): Assertion-assessment ID consumed by the KSI evaluation. At least one is required.
- `--evidence-window-start-unix-ms` (required): Inclusive start of the declared evidence collection interval in Unix milliseconds.
- `--evidence-window-end-unix-ms` (required): Inclusive end of the declared evidence collection interval in Unix milliseconds.
- `--attempt-id`: Attempt ID admitted into the assertion-assessment scope. Repeatable.
- `--scenario-id`: Scenario ID admitted into the assertion-assessment scope. Repeatable.
- `--action-id`: Action or transaction ID admitted into the assertion-assessment scope. Repeatable.
- `--class`: FedRAMP 20x certification class (A, B, C, D). Defaults to `C`.
- `--catalog`: Path to KSI catalog JSON. Defaults to `docs/reference/ksi-catalog.json`.
- `--demo-run`: Demo run ID to verify. Repeatable. When omitted, all persisted runs under `.g8e/data/compliance/demo-evidence/` are verified.
- `--project-root`: Project root for demo provenance verification. Defaults to cwd.
- `--fail-closed`: Exit nonzero if any KSI is not satisfied, KSI evaluation is unavailable, or any demo run is invalid. Use this in CI gates; do not use it during release prep when the runtime stores may not have live governance data.

### When to run it

The **agent runs compliance evidence generation during PR prep**, after writing the release notes and before finalizing version-bearing files (see the [Standard Checklist Template](#standard-checklist-template) agent-prep order). The command needs the release version string, which is determined during the change inventory, and writes into the release notes directory, which is created with the release notes. The agent establishes or consumes the release assessment scope, run, assertion-assessment set, and inclusive evidence window for that release; the release owner reviews the generated artifacts but does not generate them as a substitute for agent prep.

The command reads runtime evidence from the `.g8e/` tree of the deployment it runs against. For release prep, run it against a deployment with current demo evidence persisted (e.g. after `./g8e demos scenarios run` has produced evidence-grade demo runs). If no demo evidence is persisted, the report records "No demo runs persisted" — this is an honest gap, not a failure.

### Release notes cross-reference

Add a `### Compliance Evidence` subsection to the release notes file pointing to the generated artifact:

```markdown
### Compliance Evidence

Per-release compliance evidence for vX.Y.Z is in [vX.Y.Z-compliance-evidence.md](vX.Y.Z-compliance-evidence.md) with a machine-readable [CSV](vX.Y.Z-compliance-evidence.csv). The report captures KSI evaluation, KSI history, and demo-run verification at the release boundary.
```

---

## Large Release Gates

Use this section when a release spans multiple subsystems, new user-facing surfaces, or owner-operated acceptance paths. **Not every item applies to every release** — the change inventory determines which gates are in scope. Automated test matrix and code gates are release blockers; owner-operated, live-stack, and optional eval-demo gates are not unless the release notes claim they were demonstrated.

| Gate | When required | Reference |
|------|---------------|-----------|
| Full automated test matrix | Any platform, ensemble, dashboard, or protocol surface changed | `./g8e test *`, ensemble `make test`/`make lint`, adapter npm scripts |
| Native eval acceptance | Native evaluation runtime behavior changed **and** release notes claim a live boundary run | [Native Evaluation Acceptance](#native-evaluation-acceptance) |
| Eval campaign live demo | Release notes or compliance prose claim a completed live campaign (verify + publish) | Release notes, `docs/ensemble/evals.md` — **not required** for code-only eval infrastructure releases |
| Owner-operated browser gates | Browser-scoped observe API, WebAuthn, CORS, SSE, or adapter contract pack changed | Release notes owner-operated section — defer with honest gaps when not run |
| Builder acceptance | Contract pack or generator-neutral frontend path changed | `dashboard/g8e-adapter/contract-pack/` — defer when not run |
| Cross-platform trust | `gw connect`, trust installation, or certificate flows changed | Release notes security section — defer when not run |
| Clean offline acceptance | Signed compliance report contract or verifier changed | [Standard Checklist Template](#standard-checklist-template), release notes |
| E2E with owner credentials | CLI or gateway flows requiring enrolled identity changed | `./g8e test e2e` — defer when not run |

Record owner-operated and human gates in the release notes (`### Deferred` or a dedicated acceptance subsection) when they cannot be satisfied before tag. Do not claim passing results that were not demonstrated. **Do not block ship on optional live eval demos** unless release notes explicitly assert they completed.

---

## Native Evaluation Acceptance

A release that changes native evaluation runtime behavior runs the Go-native `core-execution-boundary` suite against a healthy unified stack with one active remote Operator. Documentation-only changes and isolated unit-test changes do not require another mutation run when an accepted runtime result already covers the unchanged implementation.

```bash
./g8e eval boundary run
./g8e eval boundary verify <run-id>
./g8e eval boundary show <run-id>
```

The run command must report 10/10 required invariants, a passing summary, valid verification, and one exact Operator and session. The separate verification invocation must report `Valid: true` with zero failures, and `show` must return the same suite, Operator, session, status, and summary. Retain the run ID and command outputs in the release acceptance record. The persisted authority is the canonical `report.json`, `verification.json`, and digest-named evidence under `.g8e/data/eval/runs/<run-id>/`.

The suite proves one allowed governed mutation and one doctrine-prohibited equivalent against a controlled target. It verifies exact target and session binding, exactly one allowed effect, no prohibited additional effect, receipt and persistence signatures, the deterministic protocol chain, evidence bindings, and Gateway L1 attribution. It does not measure model quality, use g8ee or a provider, establish certification, or demonstrate recurring operating effectiveness. See [Evaluations](../architecture/evals.md) for the complete boundary.

---

## Version-Bearing Files

After the change inventory and documentation reconciliation are complete, bump the version files. `VERSION` is the single source of truth; `make release` synchronizes the three protocol Python version files listed below, while PR preparation updates the other version-bearing and generated files.

### Core Version Files

| # | File | How It's Updated | Format |
|---|------|-----------------|--------|
| 1 | `VERSION` | **Manual** (agent, PR prep): set to new version | `vX.Y.Z\n` (with trailing newline, no trailing spaces) |
| 2 | `CHANGELOG.md` | **Manual** (agent, PR prep): add a table row to the major-version section | `\| X.Y.Z \| YYYY-MM-DD \| ... \|` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `__version__ = "X.Y.Z"` (no `v` prefix) |
| 5 | `protocol/python/uv.lock` | **Manual** (agent, PR prep): set the editable `g8e` package entry to match `VERSION` so locked environments remain usable; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` under `name = "g8e"` (no `v` prefix) |
| 6 | `ensemble/uv.lock` | **`make proto`** (agent, PR prep): regenerated automatically by `make proto` after the `protocol/python` version is bumped. The workspace depends on `g8e` through `../protocol/python`, so a version bump propagates into its lockfile. `make release` does not touch this file; it must be regenerated during PR prep | `version = "X.Y.Z"` under `name = "g8e"` (no `v` prefix) |

> Items 3 through 5 must be synced to `VERSION` during PR prep so CI's version sync check passes on the PR. The agent edits them manually (it cannot run `make release`). Item 6 is regenerated by `make proto` (run after bumping items 3-5) so the downstream `uv sync --locked` checks pass. `make release` re-syncs items 3-5 after merge as a no-op safety net; it does not touch item 6. A mismatch in any of these will fail CI.

#### CHANGELOG.md Format

The CHANGELOG is a table-based index. Each major version has a section (`## vX.Y.x`) containing a table of releases. Detailed content lives in the per-release notes files, not in the CHANGELOG itself.

If a major-version section already exists, add a new row at the top of the table. If this is a new major version, add a new section after the `---` separator.

```markdown
## vX.Y.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| X.Y.Z | YYYY-MM-DD | Brief summary of the release (1-3 sentences). | [vX.Y.Z](docs/release_notes/vX.Y.x/vX.Y.Z.md) |
```

The Description column should concisely summarize the release. The Notes column links to the full release notes file. Use the `v` prefix in the link text but not in the Version column.

### Versioning Rules

Follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- **MAJOR** (X.0.0): Breaking changes that require user action
- **MINOR** (x.Y.0): New features, backward-compatible changes
- **PATCH** (x.y.Z): Bug fixes, backward-compatible changes

Check the current version before starting:

```bash
cat VERSION
```

Throughout this document, **`vX.Y.Z`** refers to the new release version (e.g., `v1.3.1`), and **`YYYY-MM-DD`** refers to the release date. Note the two version string conventions in use:

- **With `v` prefix** (`vX.Y.Z`): `VERSION`, all doc `Version:` headers, git tags
- **Without `v` prefix** (`X.Y.Z`): `CHANGELOG.md`, Python package version, `**Document Version:**` headers

### Go Protocol Module

The Go protocol code is part of the root module `github.com/g8e-ai/g8e/v2`. There is no separate `protocol/go.mod` to update. The Go module version is derived from git tags (`vX.Y.Z`), created by `make release`. External consumers use `go get github.com/g8e-ai/g8e/v2@vX.Y.Z`.

### Build & CI Files (No Version Update Required)

The following files read the version dynamically from `VERSION` at build time and do **not** require manual updates:

- `Makefile`: Reads `VERSION` via `$(shell cat VERSION)`; `make release` syncs Python files, tags, and pushes (GitHub Actions workflows create the release)
- `Dockerfile`: Builds via `make build-all`, which reads `VERSION` from the file at build time (no version build arg)
- `docker-compose.yml`: References build context, not version
- `.github/workflows/*.yml`: Triggered by git tags, no hardcoded version. CI includes a version sync check that fails if Python files don't match `VERSION`.

---

## Documentation Metadata

The [Documentation Guide metadata rule](docs.md#end-to-end-audit-workflow) governs every maintained document. Metadata certifies a completed audit; it is never a mechanical release-wide replacement.

- A document that is edited for any reason is first reviewed end to end and reconciled with related current-state documentation.
- After the audit, generated-output refresh, cross-link review, and focused validation are complete, update the document's existing `Last Updated`, `Version`, or `Document Version` fields while preserving its format.
- Use the exact `vX.Y.Z` value from `VERSION` for `Version:` fields. Preserve a document's established no-`v` format only where that format already exists.
- Do not add metadata to generated files whose source or evidence manifest owns version identity, including the root `README.md` and generated protobuf or OpenAPI references.
- Do not blanket-bump untouched documents. A document evaluated for impact but not edited keeps its existing metadata.
- Historical release notes and evidence artifacts retain the release, run, cutoff, and schema versions they describe.

Do not maintain a hard-coded list of versioned documents here. The [Documentation Catalog](docs.md#documentation-catalog) is the complete navigation inventory, and each affected document's existing header determines whether it carries metadata. The release reconciliation record identifies every edited document and confirms that its metadata was updated last.

---

## Standard Checklist Template

Copy this template into a version-specific working tracker when starting a release (for example `.local.dev/docs/plans/in-progress/vX.Y.Z-readiness.md`). Replace `vX.Y.Z`, `X.Y.Z`, and the release range placeholder. **Do not paste checked progress back into this file.**

### Checklist for vX.Y.Z

**Release range:** `<prev-tag>..HEAD`

#### Agent prep (feature branch — do not commit, push, or tag)

- [ ] **Change inventory** — Complete inventory for the release range (see [Change Inventory](#change-inventory))
- [ ] **Documentation reconciliation** — Catalog walk, end-to-end audits, cross-links, validation, reconciliation record (see [Documentation Reconciliation](#documentation-reconciliation))
- [ ] **Release notes** — `docs/release_notes/vX.Y.x/vX.Y.Z.md` (see [Release Notes](#release-notes))
- [ ] **Compliance evidence** — `vX.Y.Z-compliance-evidence.md` + `.csv`; release notes cross-reference (see [Compliance Evidence Generation](#compliance-evidence-generation))
- [ ] **Clean offline acceptance** — When required: network-disabled report verification with rejected mutation classes recorded
- [ ] **`VERSION`** — Set to `vX.Y.Z`
- [ ] **Python package sync** — `pyproject.toml`, `g8e/__init__.py`, `protocol/python/uv.lock` → `X.Y.Z`
- [ ] **Downstream lockfile** — `make proto` → `ensemble/uv.lock`
- [ ] **`CHANGELOG.md`** — Row under the major-version section
- [ ] **Document metadata** — Finalized on every edited document only (see [Documentation Metadata](#documentation-metadata))
- [ ] **Verification** — [Verification](#verification) checks pass
- [ ] **Agent handoff** — No commit, push, PR, or `make release` by the agent

#### Large-release gates (when applicable)

- [ ] **Automated test matrix** — `./g8e test unit`, `./g8e test integration`, `./g8e test lint`, `./g8e test coverage`, ensemble `make test` / `make lint`, adapter `npm test` / `npm run lint` / contract-pack check
- [ ] Complete every other applicable gate from [Large Release Gates](#large-release-gates), or record an honest deferral in release notes

#### Release owner (commit, merge, tag)

- [ ] **PR opened** — `git commit -m "release: vX.Y.Z"`, push, open PR
- [ ] **PR merged** — CI green on `main`
- [ ] **`make release` on `main`** — Tags `vX.Y.Z` and `protocol/vX.Y.Z`
- [ ] **Release workflows succeed** — Binaries, GitHub release, PyPI publish verified

Five files need manual version edits during PR prep: `VERSION`, `CHANGELOG.md`, `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and `protocol/python/uv.lock`. Run `make proto` for `ensemble/uv.lock`. `make release` re-syncs the three `protocol/python` files after merge as a no-op safety net.

---

## Verification

After making all updates, run these checks to catch missed generated output and version synchronization. They supplement, but cannot prove, the required full-document audits. Before running them, the release owner compares the complete changed-file inventory with the documentation reconciliation record and rejects the release if any affected catalog surface, owning source, related-document check, final reread, metadata update, or focused validation is missing. These commands are read-only.

```bash
RELEASE_VERSION=$(cat VERSION)        # e.g. v1.3.1
RELEASE_NUM=${RELEASE_VERSION#v}      # e.g. 1.3.1
RELEASE_DATE="YYYY-MM-DD"             # set to the release date

# 1. Verify VERSION file
cat VERSION

# 2. Verify CHANGELOG has the new version section
head -n 20 CHANGELOG.md

# 3. Verify release notes file exists
ls "docs/release_notes/${RELEASE_VERSION%.*}.x/${RELEASE_VERSION}.md"

# 4. Verify Python package version matches VERSION. The agent syncs these files
#    manually during PR prep so CI's
#    version sync check passes on the PR. All three must show X.Y.Z matching RELEASE_NUM.
grep -n '^version' protocol/python/pyproject.toml
grep -n '__version__' protocol/python/g8e/__init__.py
grep -A1 '^name = "g8e"' protocol/python/uv.lock
# All three should show X.Y.Z matching RELEASE_NUM.

# 4b. Verify downstream uv.lock is in sync. `make proto` regenerates this
#     during PR prep. It must show X.Y.Z matching RELEASE_NUM under the
#     g8e package entry. If it is stale, `uv sync --locked` fails in CI.
grep -A1 '^name = "g8e"' ensemble/uv.lock
# Should show X.Y.Z matching RELEASE_NUM.

# 5. Find any doc version header (plain, bold, or "Document Version") NOT on the new
#    version; should return nothing for docs modified in this release. Docs NOT modified
#    are expected to still show the old version (that's the point). To check only modified
#    docs, pipe the git diff list:
#      git diff --name-only <prev-tag>..HEAD -- docs/ protocol/docs/ | xargs grep -niE '^(\*\*)?(document )?version:'
#    and verify each shows the new version. Unmodified docs are expected to lag.
grep -rniE '^(\*\*)?(document )?version:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -viE "v?${RELEASE_NUM}([^0-9]|$)"

# 6. Find any "Last Updated" header not on the release date; should return nothing,
#    or only intentional entries (e.g., docs/devs/ docs on their own cadence, or docs
#    not modified in this release which are expected to keep their old date).
grep -rniE '^(\*\*)?last updated:' docs/ protocol/docs/ --include='*.md' --exclude-dir=release_notes \
  | grep -v "$RELEASE_DATE"

# 7. Verify no stale references remain; grep for identifiers that were removed or
#    renamed in this release and confirm no docs still reference them.
grep -rnE 'OldName|old_command|OLD_CONSTANT' docs/ protocol/docs/ --include='*.md'

# 8. Find explicit g8e version pins in maintained first-party documentation that
#    do not match the release. Review every match by document classification:
#    current-state installation guidance uses the intended current version, while
#    historical release notes and scope-bound evidence retain their own version.
#    Third-party tool pins are excluded by this g8e-specific pattern.
grep -rnE "g8e==[0-9]+\.[0-9]+\.[0-9]+|g8e-ai/g8e/v2@v[0-9]+\.[0-9]+\.[0-9]+" docs/ protocol/ dashboard/ ensemble/ demos/ internal/adapters/ README.md .github/ --include='*.md' --include='*.tmpl' \
  | grep -v release_notes | grep -v CHANGELOG \
  | grep -viE "v?${RELEASE_NUM}([^0-9]|$)"
```

If step 4 shows a mismatch, sync all three Python version entries to `VERSION` before handoff. If step 4b shows a mismatch, run `make proto` to regenerate the downstream lockfile. Steps 5 and 6 intentionally show older metadata for untouched documents; compare only the release owner's complete edited-document list, and confirm metadata was updated after each document's audit. Replace the step 7 placeholder identifiers with every renamed or removed identifier from the change inventory and resolve all current-state matches. Classify step 8 matches before changing them so historical and evidence versions remain intact. None of these searches replaces the documented catalog walk, source verification, related-document reconciliation, or final end-to-end read.

---

## Release Workflow

The `make release` target handles version syncing, tagging, and pushing in a single step. CI handles lint, tests, and version sync verification on PRs. GitHub Actions workflows handle release creation and asset uploads.

### `make release`: Tag and Push

> **Run this on the merged main branch**, not on a feature branch. The tags must point at the merge commit on main. **`make release` is run by the release owner, not by the agent preparing the PR.** The agent's work ends at handoff; the release owner commits, merges, waits for CI on `main` to pass, pulls `main` locally, and then runs `make release`.

1. Syncs `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and the editable `g8e` package entry in `protocol/python/uv.lock` from `VERSION` (if already in sync, no changes are made). It does NOT regenerate the downstream `ensemble/uv.lock` — that is regenerated by `make proto` during PR prep.
2. Verifies working tree is clean (fails if Python files were out of sync; commit synced files and go through the PR process first)
3. Verifies release notes file exists at `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Verifies tags `vX.Y.Z` and `protocol/vX.Y.Z` don't already exist
5. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
6. Pushes both tags to origin

The `vX.Y.Z` tag triggers the `release-binary.yml` workflow, which builds all platforms, signs binaries, creates the GitHub release, and uploads assets. The `protocol/vX.Y.Z` tag triggers the `release-python-protocol.yml` workflow, which publishes the Python package to PyPI.

> **Lint and tests are handled by CI** (`.github/workflows/build-and-test.yml`) on pull requests, not by `make release`. The CI workflow includes a version sync check that fails if `pyproject.toml`, `__init__.py`, or the editable `g8e` entry in `protocol/python/uv.lock` doesn't match `VERSION`. It also runs `uv sync --locked` in `ensemble`, which fails if the downstream `uv.lock` still pins the old `g8e` version — `make proto` regenerates this during PR prep to prevent this.

### CI Workflows Triggered by Tags

| Tag | Workflow | What It Does |
|-----|----------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds all platforms, signs binaries with cosign, uploads assets to GitHub release, and verifies fresh `go install` works on Ubuntu, macOS, and Windows |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Builds and publishes Python package to PyPI, verifies fresh PyPI install and imports on Ubuntu, macOS, and Windows |

The `protocol/v*` tag is used only as a trigger for the Python PyPI release workflow. It is NOT used for Go module versioning; the Go module is part of the root module and is versioned by `v*` tags.

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Inventory the changes, map them through the [Developer Guidelines](devs.md) and complete [Documentation Catalog](docs.md#documentation-catalog), and perform the same full-document audit, related-document reconciliation, validation, and metadata-finalization gates as a normal release
3. Set `VERSION`, sync the Python package files to match, run `make proto` to regenerate the downstream `uv.lock` files, update `CHANGELOG.md`, write release notes, and generate compliance evidence with the release version, output directory, assessment binding, and evidence-window flags documented in [Compliance Evidence Generation](#compliance-evidence-generation)
4. Hand off to the release owner, who commits, pushes, opens and merges the PR; after CI on `main` passes, pulls main locally, and runs `make release` to re-sync the Python files (no-op), tag, and push; GitHub Actions workflows create the release and upload assets

---

## Out of Scope: Manual Git Steps

The only git operations not automated by `make release` are:

- Staging and committing the release-prep changes (`git add`, `git commit`) — release owner
- Pushing the branch and opening a PR (`git push`, GitHub PR) — release owner
- Merging the PR on GitHub — release owner
- Pulling main after merge (`git checkout main && git pull`) — release owner

`make release` handles tag creation and tag pushing automatically; the release owner runs it on the merged main branch after CI passes. GitHub Actions workflows handle release creation and asset uploads. The agent does not run `git commit`, `git push`, `make release`, or any other mutating git/make command — it prepares the working tree and hands it back.

---

## References

- [Developer Guidelines](devs.md)
- [Documentation Guide and Catalog](docs.md)
- [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
- [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
- [GitHub Releases Documentation](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [Go Module Versioning](https://go.dev/doc/modules/versioning)
- [PyPI Packaging](https://packaging.python.org/en/latest/tutorials/packaging-projects/)
