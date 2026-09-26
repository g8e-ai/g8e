# g8e Roadmap

Last Updated: 2026-09-26
Version: v2.1.13

This page is the public sequence of repository work. It states what the current line is for, what follows it, and what this roadmap will not do. `VERSION` stays `v2.1.13` until v2.2.0 release preparation.

## Now — v2.2.0

v2.2.0 is in progress on `draft/v2.2.0`. The theme is a boundaries release after monorepo reunification. g8ee and g8ed have been in-tree since v2.0.0.

- Protocol under [`protocol/`](protocol/README.md) is the schema and source of truth. Go and protobuf live there. Python and Node are consumers.
- Keep three code boundaries distinct: the Gateway HTTP and control plane, the embedded Operator substrate, and the outbound Operator runtime (`G8eoService`). Package extraction for that split is still open.
- `./g8e` stays command-and-control: grouped commands, not a grab-bag. The [Code Map](docs/devs/codemap.md) lists the current groups. Splitting `internal/cli/cmd` into those groups is still open.
- Diligence hygiene already merged to `draft/v2.2.0`: toolchain pins, typed contracts, test hygiene, and ensemble utils ownership ([#314](https://github.com/g8e-ai/g8e/pull/314), [#315](https://github.com/g8e-ai/g8e/pull/315), [#316](https://github.com/g8e-ai/g8e/pull/316)).
- Merge to `main` only after the full [Release Process](docs/devs/release_process.md) checklist: change inventory, documentation reconciliation, release notes, version sync, compliance evidence, and the large-release gates.

## Next — v2.3

- Unified protobuf codegen on one buf-driven path. Retire the dual Python generator.
- Extract Makefile target groups. `make` remains the single entrypoint.
- Optional ownership split for dashboard `public/js/utils`. Keep the vanilla UI and Evaluation Explorer as separate surfaces.

## Later

- Optional JavaScript workspace across g8ed, the Evaluation Explorer, `protocol/node`, and `website`.
- Further package-size cuts only if navigation still hurts.
- Product features only after the tree stays navigable.

## Non-goals

- Do not collapse the g8ed, g8ee, and platform version numbers into one version.
- Do not treat OSCAL or jsonschema dynamic maps, or `json.RawMessage` envelopes, as cleanup targets.
