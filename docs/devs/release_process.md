# g8e Release Process

Last Updated: 2026-09-24
Version: v2.1.13

The primary purpose of a release is to inventory every change since the last release and ensure that all affected documentation accurately reflects the current state of the code. Version bumps and CHANGELOG entries follow this documentation reconciliation; they do not replace it.

The [Developer Guidelines](devs.md) define the repository-wide engineering rules that every release follows. Their documentation rule points to the [Documentation Guide](docs.md), whose [Documentation Catalog](docs.md#documentation-catalog) enumerates every first-party prose, generated, machine-readable, component, evidence, and historical documentation surface that may require an update. A release preparer reviews that complete catalog against the change inventory instead of relying on a fixed list in this document.

The protocol Go and Python packages and the platform binary share the same version number. There are no independently versioned protocol releases.

> **`make release` handles version syncing, tagging, and pushing.** It does not build binaries, run lint or tests, or create GitHub releases. Pull-request and main-branch CI validate the candidate; tag-triggered GitHub Actions workflows build and publish the release artifacts. Release prep changes are committed and opened as a PR; after merge, pull `main` and run `make release` to tag and push. See [Release Workflow](#release-workflow).

## Release Prerequisites

The release owner needs a checkout of the merged `main` branch, a clean working tree, permission to push both release tags to `origin`, and credentials for the required GitHub Actions and PyPI publication paths. Release preparation also requires the repository's Go, Python, Node, Buf, and `uv` tooling needed by `make proto` and the applicable verification gates. Confirm the candidate version with `cat VERSION`.

Do not run `make release` from a feature branch or before the release-preparation changes are merged. The target creates and pushes tags, which starts external GitHub Actions workflows. If version synchronization makes the working tree dirty, stop, commit the synchronized Python files through the normal PR process, regenerate `ensemble/uv.lock` with `make proto` when required, and retry only after the merge.

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
4. Set `VERSION` to `vX.Y.Z`, add the `CHANGELOG.md` row, sync the Python package files (`protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, and the editable `g8e` package entry in `protocol/python/uv.lock`) to `X.Y.Z` (no `v` prefix), and set the `Version: vX.Y.Z` header in each maintained protocol specification document (`protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`). Run `make proto` to regenerate the downstream `ensemble/uv.lock` and required protocol output before capturing release-candidate build evidence. Preserve these completed steps when resuming preparation with unchanged inputs.
5. Generate the first-party compliance bundle from actual scoped evidence, prepare the assessed signing-trust inputs outside the bundle, verify it, and run `g8e compliance release-evidence <bundle>` to produce the release Markdown and CSV. Complete [Clean Offline Acceptance](#clean-offline-acceptance) when the signed report contract or verifier changed. The agent owns generation; the release owner or delegated engineering assessor supplies the actual signer assessment, not an invented third-party approval.
6. Link the generated evidence and record its scope and verification results in the release notes. Finalize edited-document metadata after reconciliation and final review.
7. Complete the [Verification](#verification) checks and applicable [Large Release Gates](#large-release-gates). Carry forward recorded passing results that still cover the candidate; rerun only checks invalidated by subsequent changes.
8. Stop. The agent does NOT commit, push, open the PR, or run `make release`. Hand the prepared working tree, retained bundle and trust locations, and verification results back to the release owner.

**Release owner (release range, commits, merges, tags, pushes):**
1. Establish the previous-to-current release range, provide the complete change and changed-file inventory to the agent, and identify any release-specific evidence requirements
2. Review the prepared code, documentation reconciliation record, release notes, and verification results, then `git add`, `git commit`, `git push`, and open the PR on GitHub
3. Merge the PR on GitHub after required PR checks pass
4. Wait for CI on `main` to pass (lint, tests, version sync checks)
5. `git checkout main && git pull` locally
6. Run `make release` — this re-syncs the Python package files from `VERSION` (a no-op if the agent already synced them), then creates and pushes the `vX.Y.Z` and `protocol/vX.Y.Z` tags
7. GitHub Actions workflows create the GitHub release, build and sign binaries, and publish the Python package to PyPI

The Python package files (`pyproject.toml`, `__init__.py`, and the editable package entry in `protocol/python/uv.lock`) and the four maintained protocol specification documents (`protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`) are synced to the new version during PR prep by the agent (manually, since the agent cannot run `make release`) so CI's version sync and locked-environment checks pass on the PR. The downstream `ensemble/uv.lock` file is regenerated by `make proto` during PR prep for the same reason. `make release` re-syncs the `protocol/python` files and protocol specification `Version:` headers after merge as a no-op safety net. The agent never runs `make release`, never commits, and never pushes.

## How to Use This Document

Work through the [Standard Checklist Template](#standard-checklist-template) in a version-specific working tracker. The sections below explain each checklist item in detail:

1. **[Change Inventory](#change-inventory)** — Release owner establishes the range and categorizes every change. Everything else depends on this.
2. **[Documentation Reconciliation](#documentation-reconciliation)** — Map changes through the full [Documentation Catalog](docs.md#documentation-catalog); audit every affected document end to end.
3. **[Release Notes](#release-notes)** and **[Version-Bearing Files](#version-bearing-files)** — Draft the permanent record, set `VERSION`, sync Python files, run `make proto`, and update `CHANGELOG.md`.
4. **[Compliance Evidence Generation](#compliance-evidence-generation)** — Capture actual scoped evidence for the candidate, establish first-party signing trust, generate and verify the public bundle, and project the release artifacts.
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

- Repository entry points, contribution and security policy, legal surfaces, and the handwritten root README.
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

Use `## [X.Y.Z] - YYYY-MM-DD` for the release notes header, with the same version and date as the CHANGELOG table row and no `v` prefix in the bracket. Only include the subsections that apply to the release; most releases use 2-4 of these, not all of them.

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

Every release requires a complete, verified, signed public report bundle and its generated Markdown and CSV projections. The agent produces them during release preparation. This is a first-party engineering assessment, not a requirement to obtain an independent audit, certification, or third-party approval.

### Required trust inputs

**External trust means trust supplied separately from, and outside, the report bundle. It does not mean a third-party assessor.** The release owner or delegated engineering assessor records the actual first-party signer assessment. Independent offline verification means reproducing the bundle's evidence and decisions without the original runtime; it does not confer external attestation.

| Input | Requirement | Purpose |
| --- | --- | --- |
| Report-signing identity | Required for generation | Dedicated Ed25519 private key and canonical `ComplianceReportSigningKeyMetadata`; purpose `compliance-report-bundle`, public-key digest, and validity interval |
| Report trust policy (`--trust-policy`) | Required for verification and projection | Canonical `ComplianceReportTrustPolicy` outside the bundle; binds the assessed public key, assessment ID, assessor identity, assessment time, validity, and exact allowed scope |
| Source-evidence trust policy (`--evidence-trust`) | Required when represented source evidence needs assessed signer trust, including commitment and customer or assessor attestation evidence | Canonical `ComplianceEvidenceTrustPolicy` outside the bundle at a distinct path from report trust; binds the actual source signers to the assessed scope and time |

A locally generated signing key is valid input when its identity, purpose, scope, and trust assessment are actually established and recorded. Key generation alone is not a trust assessment. Keep private keys out of the bundle, public artifacts, logs, and repository commits. Supplying a policy outside the bundle never turns first-party evidence into evidence-level L5 external attestation. See [Offline Trust and Replay](../reference/compliance-evidence.md#offline-trust-and-replay-contract) for the verifier contract.

### Release-eligible bundle

A bundle is eligible for release projection when all of these conditions hold:

1. Its protected `AssessmentScope` identifies the target product version and the actual source admissions, runtime owners, acquisition boundaries, verifier versions, applicability, selected population, posture, evidence window, and assessment-as-of time.
2. Required context is present or explicitly declared unavailable with typed reasons. Posture-required authorization policies are present; unavailable context does not replace required authorization or native proof.
3. Every admitted source is reviewed and explicitly classified public, and generation uses `--profile public`.
4. The complete bundle includes its protected source bytes, scope, catalogs, analysis, checksums, signatures, and deterministic renderings. Required trust inputs authenticate the applicable report and source signers.
5. Offline verification returns a valid report with zero verification failures. When the signed report contract or verifier changed, the candidate also passes [Clean Offline Acceptance](#clean-offline-acceptance).
6. The protected product version matches `VERSION`, allowing the conventional `v` prefix, and release claims stay within the demonstrated assessment scope.

**Bundle validity and assertion outcomes are separate.** A valid report can contain `not_satisfied`, `unverifiable`, or `not_applicable` assertions. The standard release gate requires authentic evidence and reproducible decisions, not every assertion passing, a minimum compliance score, or full framework coverage. A release-specific requirement for a particular demonstrated outcome must be stated explicitly in the release inventory and supported by its evidence.

Select actual available evidence: persisted demo or evaluation runs, bounded operational exports, or supported standalone sources such as build/configuration records. A new campaign is not a prerequisite for every release report. Fresh build/configuration evidence supports only that narrower assessment; it does not replace missing runtime or campaign proof. Preserve negative outcomes and selected-population gaps. Missing or invalid explicitly selected source artifacts are generation or verification failures, not permission to fabricate replacements or relabel another run.

### Generate, verify, and project

Complete version synchronization and required generation before capturing candidate build evidence. Then:

1. Capture or retain the selected source bytes and create the canonical protected scope with product version `X.Y.Z`. Populate real contextual evidence or typed unavailable declarations.
2. Prepare the signing identity and assessed trust inputs described above. Use existing scope-eligible inputs or establish new first-party inputs; do not wait for an unrelated external audit.
3. Generate a new immutable report bundle with the explicit public profile.
4. Verify the complete bundle against the separate trust inputs. Perform clean offline acceptance when required.
5. Project the verified bundle into the release directory and compare both output files with its canonical renderings.
6. Retain the complete bundle, public trust inputs, and verification record outside disposable runtime state. Record their locations and digests for handoff; Markdown and CSV alone are not a replayable bundle.

Set `SCOPE`, `REPORT_ID`, `SIGNING_METADATA`, `SIGNING_PRIVATE_KEY`, and `REPORT_TRUST` to the actual scope path, immutable report ID, signing-input paths, and report-trust path. Define the Bash array `SOURCE_ARGS` for the sources admitted by the scope:

| Selected source | `SOURCE_ARGS` example |
| --- | --- |
| Operational export | `SOURCE_ARGS=(--source "$SOURCE_DIR")` |
| Persisted evaluation run | `SOURCE_ARGS=(--eval-run "$RUN_ID")` |
| Persisted demo run | `SOURCE_ARGS=(--demo-run "$RUN_ID")` |
| Build/configuration records | `SOURCE_ARGS=(--build-run-id "$BUILD_RUN_ID" --build-config-attestations "$BUILD_CONFIG")` |

Combine source flags only when the protected scope admits those sources. Consult `./g8e compliance report generate --help` for the remaining standalone source flags. Set `EVIDENCE_TRUST_ARGS=()` when source-evidence trust is not required; otherwise set `EVIDENCE_TRUST_ARGS=(--evidence-trust "$EVIDENCE_TRUST")` using the distinct assessed source-trust policy.

```bash
./g8e compliance report generate \
  --scope "$SCOPE" \
  --report-id "$REPORT_ID" \
  --profile public \
  --signing-metadata "$SIGNING_METADATA" \
  --signing-private-key "$SIGNING_PRIVATE_KEY" \
  "${SOURCE_ARGS[@]}" \
  "${EVIDENCE_TRUST_ARGS[@]}"
```

Set `BUNDLE` to the complete bundle descriptor path printed by generation, not to a Markdown report or analysis file. Require verification to succeed before projection:

```bash
./g8e compliance report verify "$BUNDLE" \
  --trust-policy "$REPORT_TRUST" \
  "${EVIDENCE_TRUST_ARGS[@]}"
```

After any required clean offline acceptance, project the bundle:

```bash
RELEASE_VERSION=$(cat VERSION)
RELEASE_DIR="docs/release_notes/${RELEASE_VERSION%.*}.x"

./g8e compliance release-evidence "$BUNDLE" \
  --trust-policy "$REPORT_TRUST" \
  "${EVIDENCE_TRUST_ARGS[@]}" \
  --out "$RELEASE_DIR"
```

The projection command derives the output version from the protected scope; it does not compare that version with the working tree's `VERSION`. Perform that comparison during preparation. The command verifies the complete bundle before copying its canonical public renderings. It does not collect evidence, execute workloads, regrade assertions, accept a release-version override, or relabel restricted sources as public. Invalid bundles, missing required trust, restricted profiles, and invalid protected versions fail before projection writes.

### Output files and release notes

| Required file | Contents |
| --- | --- |
| `docs/release_notes/vX.Y.x/vX.Y.Z-compliance-evidence.md` | Canonical public Markdown: native assertion outcomes, coverage, content-addressed proof references, conservative external alignment, diagnostic counts, and claim boundaries |
| `docs/release_notes/vX.Y.x/vX.Y.Z-compliance-evidence.csv` | Canonical public CSV: analysis, assertions, framework controls, proof digests, and diagnostic summaries |

Public projections exclude source-local identities, runtime paths, free-form diagnostics, limitations, and source bodies. Preserve their bytes; explain the assessment scope and evidence limits in the release notes rather than editing generated output. These projections do not claim certification, accreditation, authorization, legal compliance, or recurring operating effectiveness.

Add a `### Compliance Evidence` subsection linking both files, identifying the actual assessment scope and first-party trust, and recording the verification result. Use this link format and add the scope-specific facts:

```markdown
### Compliance Evidence

Per-release compliance evidence for vX.Y.Z is in [vX.Y.Z-compliance-evidence.md](vX.Y.Z-compliance-evidence.md) with a machine-readable [CSV](vX.Y.Z-compliance-evidence.csv). These files are canonical projections of the verified public first-party compliance bundle.
```

### Clean Offline Acceptance

This additional acceptance is required when the signed compliance report contract or verifier changes. It is artifact verification, not a requirement to rerun the platform test matrix.

1. Use the identified release-candidate binary in a fresh network-disabled environment. Mount only the complete bundle and required public trust inputs read-only alongside the candidate. Do not mount the source workspace, developer runtime, or signing private key.
2. Run `compliance report verify <bundle> --trust-policy <report-trust>` with `--evidence-trust <source-trust>` only when required. Require exit status zero, `valid: true`, and zero verification failures.
3. In separate copied bundles, mutate protected source content, protected assessment scope, a rendered output, and a report signature. Require each mutation to fail verification with a nonzero exit and the corresponding integrity or replay failure. Leave the accepted original unchanged.
4. Record the candidate digest, bundle descriptor digest, checksum root, trust-policy digests, verifier identity/version, isolation settings, successful verification output, and rejected mutation classes in the release acceptance record. Link or summarize that record in the release notes.

A missing mandatory artifact, an invalid bundle, missing required signer trust, an unsupported release claim, or a failed required offline acceptance blocks handoff. An accurately reported unavailable assertion does not by itself block handoff.

---

## Large Release Gates

Use this section when a release spans multiple subsystems, new user-facing surfaces, or owner-operated acceptance paths. Record which gates apply and why in the release tracker. The mandatory compliance artifacts and required clean offline acceptance cannot be deferred as optional live demonstrations.

| Gate | Requirement | Reference |
| --- | --- | --- |
| Passing automated test matrix | Required for changed platform, Ensemble, Dashboard, adapter, Explorer, or protocol surfaces; retain applicable completed results and run only outstanding or invalidated checks | [Testing Guide](tests.md), component test commands |
| Native eval acceptance | Support any claimed live boundary result with the accepted run; rerun when changed runtime behavior invalidates that claim | [Native Evaluation Acceptance](#native-evaluation-acceptance) |
| Eval campaign acceptance | Support any claimed completed campaign with its exact verify/publish evidence; a new campaign is not required for a code-only infrastructure release | [Ensemble Evaluations](../ensemble/evals.md) |
| Clean offline acceptance | Required when the signed compliance report contract or verifier changed | [Clean Offline Acceptance](#clean-offline-acceptance) |
| Owner-operated browser, builder, cross-platform trust, and credentialed E2E acceptance | Required when the release claims the corresponding result or the release owner explicitly requires it; otherwise record the unperformed acceptance as deferred | Relevant component guide and release notes |

Retain command, candidate/input identity, and result records for completed checks. Do not rerun a passing suite merely because release documentation or a tracker is being finalized. Rerun a check only when relevant code, build inputs, configuration, or environment changed enough to invalidate its result. Required CI checks still apply to the merge and release workflow.

Keep live claims tied to their exact evidence and record unperformed owner-operated acceptance in the release notes. Do not turn an optional demo, full model rollout, or third-party audit into an unstated release prerequisite.

---

## Native Evaluation Acceptance

When the release claims a live native execution-boundary result, retain an accepted Go-native `core-execution-boundary` run covering the relevant implementation. If native evaluation runtime changes invalidate that result, run the suite against a healthy unified stack with one active remote Operator before retaining the claim. Documentation-only changes and isolated unit-test changes do not require another mutation run when accepted evidence still covers the implementation.

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
| 2 | `CHANGELOG.md` | **Manual** (agent, PR prep): add a table row to the minor-version section | `\| X.Y.Z \| YYYY-MM-DD \| ... \|` (no `v` prefix) |
| 3 | `protocol/python/pyproject.toml` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` (no `v` prefix) |
| 4 | `protocol/python/g8e/__init__.py` | **Manual** (agent, PR prep): set to match `VERSION` so CI passes; `make release` re-syncs after merge as a no-op safety net | `__version__ = "X.Y.Z"` (no `v` prefix) |
| 5 | `protocol/python/uv.lock` | **Manual** (agent, PR prep): set the editable `g8e` package entry to match `VERSION` so locked environments remain usable; `make release` re-syncs after merge as a no-op safety net | `version = "X.Y.Z"` under `name = "g8e"` (no `v` prefix) |
| 6 | `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, `protocol/docs/spec.md` | **Manual** (agent, PR prep): set each `Version:` header to match `VERSION`; `make release` re-syncs after merge as a no-op safety net | `Version: vX.Y.Z` |
| 7 | `ensemble/uv.lock` | **`make proto`** (agent, PR prep): regenerated automatically by `make proto` after the `protocol/python` version is bumped. The workspace depends on `g8e` through `../protocol/python`, so a version bump propagates into its lockfile. `make release` does not touch this file; it must be regenerated during PR prep | `version = "X.Y.Z"` under `name = "g8e"` (no `v` prefix) |

> Items 3 through 6 must be synced to `VERSION` during PR prep so CI's version sync check passes on the PR. The agent edits them manually (it cannot run `make release`). Item 7 is regenerated by `make proto` (run after bumping items 3-5) so the downstream `uv sync --locked` checks pass. `make release` re-syncs items 3-6 after merge as a no-op safety net; it does not touch item 7. A mismatch in any of these will fail CI and the Python package `test_protocol_documentation_matches_repository_version` test.

#### CHANGELOG.md Format

The CHANGELOG is a table-based index. Each minor-version series has a section (`## vX.Y.x`) containing a table of releases. Detailed content lives in the per-release notes files, not in the CHANGELOG itself.

If the minor-version section already exists, add a new row at the top of the table. For a new minor-version series, add a new section after the `---` separator.

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
- `Dockerfile`: Builds via `make build-target` for the image platform, which reads `VERSION` from the file at build time (no version build arg)
- `docker-compose.yml`: References build context, not version
- Release workflows under `.github/workflows/`: Triggered by their release tags and derive the release version from the tag. The separate build-and-test workflow includes a version sync check that fails if Python files or maintained protocol specification `Version:` headers don't match `VERSION`.

---

## Documentation Metadata

The [Documentation Guide metadata rule](docs.md#end-to-end-audit-workflow) governs every maintained document. Metadata certifies a completed audit; it is never a mechanical release-wide replacement.

- A document that is edited for any reason is first reviewed end to end and reconciled with related current-state documentation.
- After the audit, generated-output refresh, cross-link review, and focused validation are complete, update the document's existing `Last Updated`, `Version`, or `Document Version` fields while preserving its format.
- Use the exact `vX.Y.Z` value from `VERSION` for `Version:` fields. Preserve a document's established no-`v` format only where that format already exists.
- Do not add metadata to generated files whose source or evidence manifest owns version identity, including generated protobuf or OpenAPI references. Preserve the handwritten root `README.md`'s existing metadata format.
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
- [ ] **Release notes draft** — `docs/release_notes/vX.Y.x/vX.Y.Z.md` reflects the change inventory (see [Release Notes](#release-notes))
- [ ] **`VERSION`** — Set to `vX.Y.Z`
- [ ] **Python package sync** — `pyproject.toml`, `g8e/__init__.py`, `protocol/python/uv.lock` → `X.Y.Z`
- [ ] **Protocol specification version headers** — `protocol/docs/a2a.md`, `constants.md`, `mcp.md`, and `spec.md` → `Version: vX.Y.Z`
- [ ] **Downstream lockfile and generated protocol** — Required `make proto` output, including `ensemble/uv.lock`, is current
- [ ] **`CHANGELOG.md`** — Row under the minor-version section
- [ ] **Scoped evidence and trust** — Actual protected sources, explicit context or unavailable declarations, assessed first-party report trust outside the bundle, and distinct source-evidence trust when required
- [ ] **Signed public bundle** — Complete retained bundle passes offline verification and its protected product version matches `VERSION`
- [ ] **Clean offline acceptance** — When the signed report contract or verifier changed: network-disabled verification and rejected mutation classes recorded (see [Clean Offline Acceptance](#clean-offline-acceptance))
- [ ] **Compliance projections** — Canonical `vX.Y.Z-compliance-evidence.md` and `.csv` generated, compared with the bundle, and linked from release notes with actual scope, trust, and verification results
- [ ] **Document metadata** — Finalized on every edited document only (see [Documentation Metadata](#documentation-metadata))
- [ ] **Verification** — [Verification](#verification) checks complete; relevant passing results retained without redundant suite reruns
- [ ] **Agent handoff** — Prepared tree, audit record, retained bundle and trust locations, and verification results delivered; no commit, push, PR, or `make release` by the agent

#### Large-release gates (when applicable)

- [ ] **Automated test matrix** — Record passing results for the applicable Go and component suites listed in the [Testing Guide](tests.md); reuse completed results covering unchanged inputs
- [ ] **Claim-dependent acceptance** — Support each claimed live result with its exact evidence; record unperformed optional owner-operated acceptance as deferred (see [Large Release Gates](#large-release-gates))

#### Release owner (commit, merge, tag)

- [ ] **PR opened** — `git commit -m "release: vX.Y.Z"`, push, open PR
- [ ] **PR merged** — CI green on `main`
- [ ] **`make release` on `main`** — Tags `vX.Y.Z` and `protocol/vX.Y.Z`
- [ ] **Release workflows succeed** — Binaries, GitHub release, PyPI publish verified

Nine files need manual version edits during PR prep: `VERSION`, `CHANGELOG.md`, `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, `protocol/python/uv.lock`, and the `Version:` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md`. Run `make proto` for `ensemble/uv.lock`. `make release` re-syncs the three `protocol/python` files and the four protocol specification headers after merge as a no-op safety net.

---

## Verification

After preparation, inspect version synchronization, release artifacts, cross-links, and the documentation reconciliation record. Compare the complete changed-file inventory with the audit record and resolve any missing affected surface, owning source, related-document check, final reread, metadata update, or focused validation before handoff. These inspections supplement the audit and retained test results; they do not require another full test run.

The following commands are read-only inspection examples, not an all-success shell script. Searches can legitimately return no matches or identify unchanged historical metadata; classify their output rather than treating every match or nonzero search exit as a release failure.

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

# 4. Verify Python package and protocol specification versions match VERSION.
#    The agent syncs these files manually during PR prep so CI's version sync
#    check passes on the PR.
bash scripts/verify-version-sync.sh
grep -n '^version' protocol/python/pyproject.toml
grep -n '__version__' protocol/python/g8e/__init__.py
grep -A1 '^name = "g8e"' protocol/python/uv.lock
grep -n '^Version: v' protocol/docs/a2a.md protocol/docs/constants.md protocol/docs/mcp.md protocol/docs/spec.md
# All entries should show X.Y.Z or vX.Y.Z matching RELEASE_VERSION / RELEASE_NUM.

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

If step 4 shows a mismatch, sync all Python version entries and protocol specification `Version:` headers to `VERSION` before handoff (`bash scripts/verify-version-sync.sh --sync` applies the Python and protocol-doc updates from `VERSION`). If step 4b shows a mismatch, run `make proto` to regenerate the downstream lockfile. Steps 5 and 6 intentionally show older metadata for untouched documents; compare only the release owner's complete edited-document list, and confirm metadata was updated after each document's audit. Replace the step 7 placeholder identifiers with every renamed or removed identifier from the change inventory and resolve all current-state matches. Classify step 8 matches before changing them so historical and evidence versions remain intact. None of these searches replaces the documented catalog walk, source verification, related-document reconciliation, or final end-to-end read.

Also require the compliance files to exist and match the verified bundle. Set `BUNDLE` to the retained descriptor used in [Compliance Evidence Generation](#compliance-evidence-generation), confirm its protected product version matches `VERSION`, and run:

```bash
: "${BUNDLE:?Set BUNDLE to the verified release bundle descriptor}"
RELEASE_VERSION=$(cat VERSION)
RELEASE_DIR="docs/release_notes/${RELEASE_VERSION%.*}.x"
BUNDLE_DIR=$(dirname "$BUNDLE")

test -s "$RELEASE_DIR/$RELEASE_VERSION-compliance-evidence.md"
test -s "$RELEASE_DIR/$RELEASE_VERSION-compliance-evidence.csv"
cmp "$RELEASE_DIR/$RELEASE_VERSION-compliance-evidence.md" "$BUNDLE_DIR/report.md"
cmp "$RELEASE_DIR/$RELEASE_VERSION-compliance-evidence.csv" "$BUNDLE_DIR/report.csv"
```

Require each artifact check to succeed. Confirm the release-note links resolve and the retained verification record identifies this exact candidate, bundle, and trust. File existence or byte equality alone does not replace signed-bundle verification.

---

## Release Workflow

The `make release` target handles version syncing, tagging, and pushing in a single step. CI handles lint, tests, and version sync verification on PRs. GitHub Actions workflows handle release creation and asset uploads. The current target checks release-note existence but does not generate or verify compliance artifacts, run offline acceptance, audit documentation, or enforce the required branch and CI state. Complete those gates before the release owner invokes it.

### `make release`: Tag and Push

> **Run this on the merged main branch**, not on a feature branch. The tags must point at the merge commit on main. **`make release` is run by the release owner, not by the agent preparing the PR.** The agent's work ends at handoff; the release owner commits, merges, waits for CI on `main` to pass, pulls `main` locally, and then runs `make release`.

1. Syncs `protocol/python/pyproject.toml`, `protocol/python/g8e/__init__.py`, the editable `g8e` package entry in `protocol/python/uv.lock`, and the `Version:` headers in `protocol/docs/a2a.md`, `protocol/docs/constants.md`, `protocol/docs/mcp.md`, and `protocol/docs/spec.md` from `VERSION` (if already in sync, no changes are made). It does not regenerate the downstream `ensemble/uv.lock` — that is regenerated by `make proto` during PR prep.
2. Checks for a clean working tree after synchronization. If synchronization changed a Python file, the target stops with a dirty tree; commit the synchronized files and go through the PR process before retrying.
3. Verifies the release notes file exists at `docs/release_notes/vX.Y.x/vX.Y.Z.md`
4. Verifies tags `vX.Y.Z` and `protocol/vX.Y.Z` don't already exist
5. Creates `vX.Y.Z` and `protocol/vX.Y.Z` tags on the current commit
6. Pushes both tags to origin

The `vX.Y.Z` tag triggers the `release-binary.yml` workflow, which builds all platforms, signs binaries, creates the GitHub release, and uploads assets. The `protocol/vX.Y.Z` tag triggers the `release-python-protocol.yml` workflow, which publishes the Python package to PyPI.

> **Lint and tests are handled by CI** (`.github/workflows/build-and-test.yml`) on pull requests, not by `make release`. The CI workflow includes a version sync check (`scripts/verify-version-sync.sh`) that fails if `pyproject.toml`, `__init__.py`, the editable `g8e` entry in `protocol/python/uv.lock`, or the maintained protocol specification `Version:` headers don't match `VERSION`. The Python package test suite also enforces the protocol-doc headers through `test_protocol_documentation_matches_repository_version`. It also runs `uv sync --locked` in `ensemble`, which fails if the downstream `uv.lock` still pins the old `g8e` version — `make proto` regenerates this during PR prep to prevent this.

### CI Workflows Triggered by Tags

| Tag | Workflow | What It Does |
|-----|----------|-------------|
| `vX.Y.Z` | `.github/workflows/release-binary.yml` | Builds all platforms, signs binaries with cosign, uploads assets to GitHub release, and verifies fresh `go install` works on Ubuntu, macOS, and Windows |
| `protocol/vX.Y.Z` | `.github/workflows/release-python-protocol.yml` | Verifies version sync, builds and publishes Python package to PyPI, verifies fresh PyPI install and imports on Ubuntu, macOS, and Windows |

The `protocol/v*` tag is used only as a trigger for the Python PyPI release workflow. It is NOT used for Go module versioning; the Go module is part of the root module and is versioned by `v*` tags.

---

## Emergency Releases (Hotfixes)

For critical security issues or production bugs:

1. Apply the minimal fix necessary to the appropriate branch
2. Inventory the changes, map them through the [Developer Guidelines](devs.md) and complete [Documentation Catalog](docs.md#documentation-catalog), and perform the same full-document audit, related-document reconciliation, validation, and metadata-finalization gates as a normal release
3. Set `VERSION`, sync the Python package files, run required protocol generation, update `CHANGELOG.md`, and write release notes. Generate and verify the signed public bundle with the target version and evidence window protected in `AssessmentScope`, then project it with `--out`, `--trust-policy`, and conditional `--evidence-trust` as documented in [Compliance Evidence Generation](#compliance-evidence-generation). Complete clean offline acceptance when required.
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
