---
title: Public Spectator Operations Guide
parent: Guides
---

# Public Spectator Operations Guide

Last Updated: 2026-09-24
Version: v2.1.13

This guide covers the gateway-owned anonymous public mirror and evaluation explorer. It is separate from the passkey-authenticated owner-local observe frontend connected with `./g8e gw connect <origin>`; see [Generator-Neutral Builder Guide](./build_observe_frontend.md) and [Build a g8e-Compatible Frontend](./build_frontend.md#generator-neutral-observe-frontend).

## Runtime boundaries

The gateway-owned public spectator runs inside the Gateway process when started with `--public-spectator` (Docker Compose default). One in-process `PublicSpectatorRuntime` owns:

- **Private ingest** (container port `8081`, host-published as `127.0.0.1:8081`): authenticated publisher ingest, proof ingest, replacement-key registration
- **Public read/SSE** (container port `8082`, host-published as `127.0.0.1:8082`): anonymous bootstrap, snapshot, history, SSE, proof catalog/manifest, content-addressed proof downloads
- **Evaluation explorer** (container port `5173`, host-published as `127.0.0.1:5173`): embedded acceptance UI

Durable feed, mirror, signing keys, and outbox state live in the gateway volume (`g8e-gateway-data`). Docker stacks do **not** bind-mount host `.g8e/public-feed` or `.g8e/public-mirror`.

The unified Compose deployment binds host ports 8081, 8082, and 5173 to loopback only. The Gateway listens on the container's `g8e-net` interfaces, and the fixed Docker bridge gateway address `172.28.0.1/32` is the only trusted proxy CIDR for `CF-Connecting-IP`. The Gateway container has a 16,384 soft and hard `nofile` limit and uses `unless-stopped` recovery; restart does not delete mirror state or publisher outbox state.

Campaign publication from the host CLI uses owner mTLS and `POST /api/v1/public-feed/batches`. Host `g8e public init` is not required for Docker or evaluation campaigns.

Campaign publication emits public-safe assignment records with the enriched `1.1.0` result envelope and Evaluation Explorer summaries with view schema `1.5.0`. Assignment Details use a compact layout with approved scenario context, typed grades, grouped activity when present, bounded resource observations rendered only when the public record contains a value, verification metadata mapped from campaign `unverified` to `not_run`, and content bindings. The Tasks view lists the frozen north-star-25 scenario catalog with disclosure-safe prompts, pass criteria, and source links. Empty sections and values absent from the public record are omitted. Evaluation summaries carry typed pass-rate, latency-p50, and output-throughput-p50 metrics with observed, eligible, and unavailable contributor counts; model evaluations omit system-only routing and correlation metrics. New summaries use `not_run` until a population-bound verification report is published, and later aggregate revisions preserve the bound `passed` or `failed` state and verification metadata. The source run and catalog bindings must match; unavailable capture remains unavailable, and public evidence bindings are references rather than proof of public accessibility. A passing campaign verification report also publishes report-scoped `exploratory_verified` model-summary revisions for eligible variant/role aggregates. The browser reads these stored revisions and does not promote model quality locally.

For an existing run, use verified catch-up after the report is persisted so assignment and model-summary revisions are restored together:

```bash
./g8e eval campaign publish --run-id <run-id>
```

The publication coordinator probes the gateway-owned dataset during catch-up. If the dataset is absent while host publication idempotency still lists records, it clears that stale state and republishes the canonical run without requiring manual JSON edits. Use `--force` only when the mirror was wiped or is known to be missing records and the normal drift-aware path is not sufficient; it resets publication idempotency state and republishes the run. Neither path recreates missing inference evidence or makes an inapplicable verification report valid. Do not run catch-up concurrently with `execute --publish`.

The mirror is a visibility and publication boundary, not a Policy Decision Point or Policy Execution Point. It does not authorize a governed mutation, and its availability is not execution evidence. The Operator whose L4/L5 boundary produced an underlying governed result retains authoritative local execution evidence.

## Start a fresh source chain

Use an explicit source transition when the owner archives the current public datasets and starts a new independent publication generation. Stop the Gateway publisher before changing its source state. The transition preserves the mirror store and archives the current publisher configuration, key, token, outbox, snapshot, key-rotation record, and local proof package under `.g8e/public-feed-archive/generations/<old-source-id>/`. Each source identity has one immutable local generation, so repeated transitions preserve every prior publisher generation and reject source-ID reuse. The command creates a fresh source identity with a new signing key and ingest token; the first new batch starts at sequence one with the zero predecessor hash and never merges the old and new chains. Existing single-generation archives are migrated into the generation layout without changing their contents.

For the unified Compose deployment, run the transition against the Gateway volume rather than the Docker-host runtime:

```bash
docker compose stop g8e-gateway
docker compose run --rm --no-deps g8e-gateway public source transition --source-id <new-public-source-pseudonym> --yes
docker compose up -d g8e-gateway
```

After restart, the Gateway registers the new source as active. Anonymous bootstrap, snapshot, history, and SSE requests that omit `source` use the new source. The archived source remains available through an explicit `?source=<old-public-source-pseudonym>` query and retains its original signed chain. Do not resume old campaign run IDs or relabel their datasets as part of the transition.

## Verify listener separation

The private listener accepts authenticated publisher operations. Requests without the ingest token fail authentication. The public listener does not register mutation routes, so the three mutation paths return HTTP 404 even if a caller supplies a token:

```bash
curl -o /dev/null -sS -w '%{http_code}\n' -X POST http://127.0.0.1:8082/ingest
curl -o /dev/null -sS -w '%{http_code}\n' -X POST http://127.0.0.1:8082/keys/register
curl -o /dev/null -sS -w '%{http_code}\n' -X POST http://127.0.0.1:8082/proof-ingest
```

Verify the anonymous allowlist on the public listener:

```bash
curl -fsS http://127.0.0.1:8082/bootstrap
curl -fsS 'http://127.0.0.1:8082/history?source=<public-source-pseudonym>&cursor=0&limit=20&kind=catalog_snapshot'
curl -fsS 'http://127.0.0.1:8082/snapshot?source=<public-source-pseudonym>'
curl -fsS http://127.0.0.1:8082/proof-catalog
curl -fsS http://127.0.0.1:8082/proof-manifest
```

The history endpoint applies its optional `kind` filter before cursor pagination, so catalog reconciliation reads only `catalog_snapshot` records instead of replaying every assignment projection. A bounded SSE client connects to `/stream`, supplies the source pseudonym and optional `since_id`, and reconciles sequence plus feed-chain state against `/snapshot`. Public clients omit credentials. The mirror permits concurrent browser streams up to its global connection ceiling, so multiple visitors behind the Cloudflare connector do not collapse into one stream. Anonymous request limits use `CF-Connecting-IP` only when the immediate socket peer belongs to the explicitly configured trusted-proxy CIDR set and the header contains exactly one valid unicast address. Forwarding headers from every other peer are ignored. Freshness is derived from the last accepted batch and transitions honestly through active, delayed, stale, intentionally stopped, safety stopped, and source offline.

## Create the Cloudflare tunnel

Tunnel creation changes Cloudflare tunnel and DNS state and remains a release-owner operation. The tunnel origin is the read-only public listener, never the private listener and never the Gateway console:

```bash
./g8e gw tunnel create --name opendevops-feed --hostname opendevops.ai --service http://127.0.0.1:8082
./g8e gw tunnel run
```

The explicit plain-HTTP service prevents Gateway HTTPS origin settings from being inherited. The generated cloudflared ingress terminates at the loopback-only read adapter. Start the gateway with `--public-base-url https://opendevops.ai` so the embedded explorer runtime points at the public origin. Do not configure `opendevops.ai` to target port 8081, Gateway ports 8080 or 8443, the Ensemble, the Dashboard, a component bridge address, or a component-local volume.

## External acceptance

After the owner starts the tunnel, verify HTTPS, CORS, bounded reads, rate limiting, replayable SSE, proof catalog, proof manifest, and content-addressed proof download from an external client. Confirm `/ingest`, `/keys/register`, `/proof-ingest`, Gateway, MCP, A2A, approval, audit, filesystem, eval-launch, and arbitrary paths are absent from the public origin. Scan public responses and proof bytes for credentials, private endpoints, machine paths, SPIFFE identities, provider topology, raw prompts, model outputs, reasoning traces, and restricted canaries; any match blocks publication.

Restart the gateway and tunnel independently. The mirror must recover the same accepted high-water sequence, feed-chain hash, key revocations, catalog, manifest, and proof bytes. Tunnel or mirror unavailability must produce an honest stale or source-offline storefront state and must not trigger inference or mutate accepted evidence.

The repository capacity harness requires an explicit target and does not default to a live deployment. Use loopback for local acceptance:

```bash
go run ./test/capacity --target http://127.0.0.1:8082 --synthetic-client-ips --mode cold --clients 300 --hold 5m --docker-container g8e-gateway
go run ./test/capacity --target http://127.0.0.1:8082 --synthetic-client-ips --mode stream --clients 300 --hold 5m --docker-container g8e-gateway
```

A public origin additionally requires `--allow-public`, and running it remains an owner-approved operation:

```bash
go run ./test/capacity --target https://opendevops.ai --allow-public --mode stream --clients 300 --hold 5m
```

For 300 distinct external cold visitors, run coordinated shards from separate generator hosts (distinct egress / Cloudflare client identity per host). Each host runs one shard:

```bash
go run ./test/capacity --target https://opendevops.ai --allow-public --mode cold \
  --clients 100 --shard-index 0 --shard-count 3
# repeat on other hosts with --shard-index 1 and --shard-index 2
```

The harness emits JSON status, outcome, latency, stream-survival, shard metadata, and optional Docker resource samples, and exits nonzero unless every requested lifecycle completes and every stream survives. `--synthetic-client-ips` is accepted only with a loopback target and exercises distinct forwarded identities through the configured Docker trusted peer. One generator host remains one Cloudflare client identity and correctly shares one anonymous request window. Callers never spoof `CF-Connecting-IP` against a public target to manufacture identity cardinality.

## Manual record publish (advanced)

`g8e public publish`, `g8e public push`, `g8e public status`, `g8e public repair-outbox`, and `g8e public restore` cover host publisher state and gateway-owned mirror reconciliation. Publication and retry use a **configured remote mirror origin**; repair compacts the local host outbox and snapshot; restore republishes verified campaign runs to the gateway-owned public mirror after a volume wipe. These commands do not start a local mirror process. Evaluation campaigns use gateway-mediated publication instead.

```bash
./g8e public publish <public-records.jsonl>
./g8e public push
./g8e public status
./g8e public restore --queue
```

`public publish` durably appends before transmission. `public push` retries ordered outbox and proof delivery without running inference. `public repair-outbox` compacts a host publisher outbox against its local snapshot; when the Gateway owns the public mirror, it reports that the Gateway manages the outbox instead of requiring host export configuration. A failed transmission remains retryable.

## Related documentation

- [Public Spectator Architecture and Threat Model](../architecture/public_spectator.md)
- [Docker Gateway Guide](docker_gateway.md#evaluation-publish-and-verify-split-host-and-container)
- [Unified Docker Stack Guide](unified_stack.md)
- [Generator-Neutral Builder Guide](build_observe_frontend.md)
