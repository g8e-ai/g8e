# Python lint configuration audit — 2026-10-06

The previous green ensemble lint result did not establish type safety. The
Pyright configuration explicitly disabled 44 diagnostic categories, including
argument, assignment, return, call, attribute, undefined-name, and optional-value
checks. Ruff independently ignored undefined names (`F821`), entire rule families
(`ASYNC`, `DTZ`, `ARG`, `SIM`, `PTH`), and numerous individual checks. Neither
local nor CI lint commands checked tests.

## Changes

- Use Pyright's `standard` preset without blanket diagnostic overrides. Also
  reject unnecessary type-ignore comments so obsolete suppressions are visible.
- Check both `ensemble/app` and `ensemble/tests` with Ruff and Pyright.
- Add `protocol/python` to Pyright's import paths. This resolves the in-tree
  package for editable installations without disabling missing-import checks.
- Add a root `pyrightconfig.json` extending the ensemble configuration so VS Code
  opened at the monorepo root and the CLI share the same policy.
- Have root Make, ensemble Make, and CI use the same lint recipe. Run both tools
  even when the first fails, and return failure if either fails. The ensemble
  Makefile now also finds the monorepo virtual environment directly.
- Have the optional strict services configuration extend the shared config;
  remove its hardcoded `/opt/venv` path. This strict profile is not the default gate.
- Restore import sorting (`I001`). Ruff formatting does **not** sort imports.
- Replace the global `B008` exception with the specific FastAPI `Depends`,
  `Query`, and `Body` declaration calls. Other function calls in defaults remain
  checked.
- Widen Ollama's `system_instructions` parameter to `str | None`, matching its
  callers and its existing conditional handling. This removes six type errors.

## Remaining exceptions and coverage limits

The four global Ruff exceptions are policies, not a baseline of current failures:

| Rule | Reason |
| --- | --- |
| `E501` | Formatting controls line width; long URLs and prose can remain longer. |
| `PLR0913`, `PLR0917` | Service interfaces may use explicit named parameters without a fixed arity cap. |
| `PLR2004` | Numeric literals do not universally need named constants. |

Tests retain only specific fixture/interface argument exceptions (`ARG001`,
`ARG002`), access to internals (`SLF001`), and class-level pytest data (`RUF012`).
`F401` remains excluded in package initializers and the named compatibility or
generated re-export modules. Those modules expose imports as API; other unused
imports are checked. Re-export exceptions could eventually be replaced with
explicit `__all__` declarations.

Pyright `standard` is not `strict`: unknown-type/annotation-completeness rules and
some optional diagnostics retain the preset defaults. Ruff owns import and unused
code checks. The stricter services profile can be invoked explicitly; it is not a
substitute for the shared gate. Virtual environments and tool caches remain
excluded. This review covers ensemble Python configuration and its editor,
Make, and CI entry points; it does not audit Go or TypeScript rule policies.

The gate does not currently run `ruff format --check`. The separate format target
writes changes, and `make -C ensemble check` still invokes that target. Formatting
verification can be added separately without conflating it with restored type
and correctness checks.

## Findings after the configuration change

**Current status:** the backlog below is fully remediated. `ruff check ensemble/app ensemble/tests` reports no findings and `pyright --project ensemble/pyrightconfig.json` reports 0 errors and 0 warnings. The tables record the original audit and are kept as history.

Pyright analyzed **578 files** and reported **2496 errors and 4 warnings**. Ruff reported **1833 findings** (854 in application code, 979 in tests). These are diagnostic counts, not counts of independent runtime defects; unresolved dynamic API typing can cause multiple downstream findings.

| Pyright diagnostic | Count |
| --- | ---: |
| `reportAttributeAccessIssue` | 1418 |
| `reportCallIssue` | 442 |
| `reportArgumentType` | 388 |
| `reportOptionalMemberAccess` | 91 |
| `reportOperatorIssue` | 41 |
| `reportOptionalSubscript` | 32 |
| `reportUndefinedVariable` | 19 |
| `reportUnnecessaryTypeIgnoreComment` | 12 |
| `reportPossiblyUnboundVariable` | 11 |
| `reportAssignmentType` | 9 |
| `reportGeneralTypeIssues` | 7 |
| `reportIncompatibleVariableOverride` | 7 |
| `reportReturnType` | 6 |
| `reportIndexIssue` | 4 |
| `reportIncompatibleMethodOverride` | 3 |
| `reportUnsupportedDunderAll` | 2 |
| `reportInvalidTypeForm` | 2 |
| `reportOptionalIterable` | 2 |
| `reportInvalidTypeVarUse` | 1 |
| `reportMissingImports` | 1 |
| `reportRedeclaration` | 1 |
| `reportUnusedExpression` | 1 |

Most frequent Ruff findings:

| Ruff rule | Count |
| --- | ---: |
| `PLC0415` | 644 |
| `I001` | 336 |
| `E402` | 243 |
| `PERF401` | 84 |
| `G201` | 56 |
| `ARG001` | 51 |
| `G004` | 30 |
| `PLR0912` | 29 |
| `ARG002` | 27 |
| `PTH123` | 26 |
| `ARG005` | 26 |
| `PLR0915` | 24 |

## Remediation order

1. Fix undefined and possibly unbound variables first. Examples include
   `final_session_id` / `final_op_id` in operator intent execution, the missing
   `FsGrepRequestPayload` import, unresolved annotation names in tribunal models,
   and `fc_cfg` in Gemini generation. Determine the intended values and add
   behavior tests where execution changes; do not silence the diagnostic.
2. Make dynamically built protocol enums and exports visible to the type checker
   through accurate annotations or generated stubs. Many attribute errors involve
   these APIs; fix the shared typing source before chasing downstream messages.
3. Resolve remaining incompatible calls, argument/return types, optional access,
   and stale test fixtures. Check protocol/service interface drift rather than
   assuming every mismatch is a false positive.
4. Address blocking I/O in async paths, timezone checks, broad exception tests,
   and unused code. Imports and mechanical style changes can follow, with review
   for import side effects and circular dependencies.

The backlog was repaired in place. No baseline file, warning downgrade, or new
blanket suppression was added to make existing findings pass. Unused parameters
were removed with their callers and interface declarations; handler signatures
that are fixed by a framework or registry (FastAPI route handlers, native tool
handlers) discard the unused argument with `del`.

## Reproduce and validation

```bash
make ensemble-lint
# Equivalent shared recipe, using the root .venv automatically:
make -C ensemble lint

.venv/bin/pyright --project pyrightconfig.json --outputjson
.venv/bin/pyright --project ensemble/pyrightconfig.json --outputjson
.venv/bin/ruff check ensemble/app ensemble/tests --output-format json

# Optional strict services audit:
.venv/bin/pyright --project ensemble/pyrightconfig.services.json

cd ensemble
../.venv/bin/python -m pytest tests/unit/llm/test_ollama_provider.py
```

The existing Ollama provider unit suite passed: **48 tests**. Configuration JSON
and TOML parse successfully. Root and ensemble Pyright configurations are checked
for diagnostic parity. `make ensemble-lint` runs both analyzers and returns a
nonzero exit status on any finding. The optional strict services profile is
not part of the gate and currently reports 722 `reportUnknown*` errors.

Policy references: [Pyright configuration](https://github.com/microsoft/pyright/blob/main/docs/configuration.md),
[Ruff formatter](https://docs.astral.sh/ruff/formatter/),
[Ruff suppression handling](https://docs.astral.sh/ruff/linter/#error-suppression).
