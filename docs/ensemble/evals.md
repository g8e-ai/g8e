# Native Evaluations

## Overview

g8e provides a Go-native evaluation command that proves the core remote execution boundary against the real unified Docker stack. The evaluator runs on the Docker host, submits typed intent to the Gateway, binds execution to one exact remote Operator session, observes the controlled target through a separate networkless Compose observer, and persists canonical content-addressed evidence beneath `.g8e/data/eval/runs/`.

The Phase 1 suite is `core-execution-boundary@1.0.0`. It requires doctrine posture and does not use g8ee, a model provider, model judges, campaigns, scheduling, or synthetic simulators.

## Run

Start and enroll the unified stack with one active remote Operator, then run:

```bash
./g8e eval run core-execution-boundary
```

Pin an exact active remote Operator session when more than one remote Operator is available:

```bash
./g8e eval run core-execution-boundary --operator-session <session-id>
```

The suite performs two attempts. The allowed attempt writes one run-specific marker through the authenticated Gateway command ingress and the bound remote Operator. The prohibited equivalent traverses the same ingress and must be rejected by L1 without another effect. Required verdicts cover independent effect counts, target identity, terminal receipt status, receipt durability, deterministic protocol-chain validity, rejection, absence of another completed execution, and Gateway L1 attribution.

## Verify and inspect

Each run persists `report.json`, `verification.json`, and digest-named evidence files under `.g8e/data/eval/runs/<run-id>/`. Re-run verification without executing another mutation:

```bash
./g8e eval verify <run-id>
./g8e eval show <run-id>
```

Add `--json` to emit canonical protojson. Verification resolves every declared artifact, recomputes content addresses, validates report and attempt bindings, verifies receipt and persistence signatures, validates deterministic stage evidence, reconstructs typed observations, recomputes verdicts and metrics, and rejects missing, substituted, contradictory, misbound, or undeclared evidence.

## Trust boundaries

The Gateway is the Policy Decision Point and owns ingress authentication, envelope construction, L1-L3 decisions, and coordination state. The selected remote Operator is the Policy Execution Point and owns L4-L5 execution and authoritative local evidence. The Gateway receipt query is a verified mirror of Operator-authored evidence, not an independent read of the Operator database.

The independent observer is a short-lived `g8e-eval-observer` Compose process. It has no network, workload identity, runtime volume, credentials, or writeable target mount. It mounts only the shared controlled fixture volume read-only. The observer runs as the root UID with all Linux capabilities dropped so it can read the root-owned, owner-only fixture created by the Operator while remaining unable to mutate the read-only volume or cross a network boundary.
