# g8e Observe Frontend Contract Pack

This contract pack is the deterministic, generator-neutral input a builder (Lovable, Notion, or other supported SPA builder) consumes to produce a deployable observability frontend that wraps the audited g8e-adapter package. The pack is generated from canonical protocol JSON and the audited adapter; regenerate it with `npm run gen:contract-pack` from `dashboard/g8e-adapter/`.

## Contents

- `builder-prompt.md` — the prompt to give a builder. Encodes every hard constraint the generated SPA must satisfy.
- `runtime-config.schema.json` — JSON Schema for FrontendRuntimeConfig. The SPA reads its config from a JSON script tag and validates it with the adapter's parseRuntimeConfig.
- `observe.openapi.json` — curated OpenAPI 3.0 for the allowlisted browser operations. mTLS producer endpoints are excluded.
- `event-schemas.json` — the four dashboard event payloads plus sentinel events, with family classifications.
- `models.ts` — standalone TypeScript models and validators derived from protocol JSON. Builders import these for typed presentation code.
- `fixtures/` — typed fixture scenarios for unauthenticated, bootstrap, connected live stream, reconnect, replay gap, empty evals, partial verification, stale telemetry, unavailable metrics, wrong-user rejection, and download availability. Every fixture parses through the generated validators.
- `manifest.json` — schema version and SHA-256 of every output. Detects drift.

## Workflow

1. A builder consumes `builder-prompt.md` plus the models, OpenAPI, event schemas, and fixtures.
2. The builder generates presentation code that imports the audited g8e-adapter for transport, auth, SSE, and state.
3. The generated SPA is deployed at a top-level origin.
4. The owner connects the origin to a local Gateway with `./g8e gw connect <origin>`.
5. The SPA authenticates with a passkey, reads typed observe data, and renders normalized SSE updates.

## Acceptance commands

Run from `dashboard/g8e-adapter/`:

```bash
npm run gen:contract-pack:check   # fail if committed outputs are stale
npm test                          # adapter + contract pack tests
npm run lint                      # type-check adapter and host
npm run build                     # build the audited adapter
npm run build:host                # build the minimal host
```

The connected page must contain no fixture leakage, fabricated values, dead controls, unsupported claims, or mutation surface. Real-browser acceptance (exact-origin CORS, WebAuthn authenticator, SSE credentials, two-user isolation) is an owner-operated gate documented in the release plan.

## Determinism

Re-running the generator against identical inputs produces byte-identical files. JSON outputs use sorted keys and a fixed 2-space indent. The manifest records the SHA-256 of every output; `gen:contract-pack:check` regenerates in memory and compares hashes without rewriting files.
