---
title: Public Spectator Operations Guide
parent: Guides
---

# Public Spectator Operations Guide

Last Updated: 2026-09-20
Version: v2.1.10

This guide covers the gateway-owned anonymous public mirror and evaluation explorer. It is separate from the passkey-authenticated owner-local observe frontend connected with `./g8e gw connect <origin>`; see [Generator-Neutral Builder Guide](./build_observe_frontend.md) and [Build a g8e-Compatible Frontend](./build_frontend.md#generator-neutral-observe-frontend).

## Runtime boundaries

The gateway-owned public spectator runs inside the Gateway process when started with `--public-spectator` (Docker Compose default). One in-process `PublicSpectatorRuntime` owns:

- **Private ingest** (`127.0.0.1:8081` by default): authenticated publisher ingest, proof ingest, replacement-key registration
- **Public read/SSE** (`127.0.0.1:8082` by default): anonymous bootstrap, snapshot, history, SSE, proof catalog/manifest, content-addressed proof downloads
- **Evaluation explorer** (`127.0.0.1:5173` by default): embedded acceptance UI

Durable feed, mirror, signing keys, and outbox state live in the gateway volume (`g8e-gateway-data`). Docker stacks do **not** bind-mount host `.g8e/public-feed` or `.g8e/public-mirror`.

Campaign publication from the host CLI uses owner mTLS and `POST /api/v1/public-feed/batches`. Host `g8e public init` is not required for Docker or evaluation campaigns.

Campaign publication emits public-safe assignment records with the enriched `1.1.0` result envelope. Assignment Details can show approved scenario context, typed grades, grouped activity, bounded resource observations, verification metadata, and content bindings. The source run and catalog bindings must match; unavailable capture remains unavailable, and public evidence bindings are references rather than proof of public accessibility. A passing campaign verification report also publishes report-scoped `exploratory_verified` model-summary revisions for eligible variant/role aggregates. The browser reads these stored revisions and does not promote model quality locally.

For an existing run, use verified catch-up after the report is persisted so assignment and model-summary revisions are restored together:

```bash
./g8e eval campaign publish --run-id <run-id>
```

Use `--force` only when the mirror was wiped or is known to be missing records. It resets publication idempotency state and republishes the run; it does not recreate missing inference evidence or make an inapplicable verification report valid. Do not run catch-up concurrently with `execute --publish`.

The mirror is a visibility and publication boundary, not a Policy Decision Point or Policy Execution Point. It does not authorize a governed mutation, and its availability is not execution evidence. The Operator whose L4/L5 boundary produced an underlying governed result retains authoritative local execution evidence.

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
curl -fsS 'http://127.0.0.1:8082/history?source=<public-source-pseudonym>&cursor=0&limit=20'
curl -fsS 'http://127.0.0.1:8082/snapshot?source=<public-source-pseudonym>'
curl -fsS http://127.0.0.1:8082/proof-catalog
curl -fsS http://127.0.0.1:8082/proof-manifest
```

A bounded SSE client connects to `/stream`, supplies the source pseudonym and optional `since_id`, and reconciles sequence plus feed-chain state against `/snapshot`. Public clients omit credentials. The mirror permits concurrent browser streams up to its global connection ceiling, so multiple visitors behind the loopback Cloudflare connector do not collapse into one stream. Anonymous request limits use Cloudflare's connecting IP only when the immediate peer is loopback. Freshness is derived from the last accepted batch and transitions honestly through active, delayed, stale, intentionally stopped, safety stopped, and source offline.

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

## Manual record publish (advanced)

`g8e public publish`, `g8e public push`, and `g8e public status` remain available for pushing pre-built JSONL record files through a **configured remote mirror origin**. They do not start a local mirror process. Evaluation campaigns use gateway-mediated publication instead.

```bash
./g8e public publish <public-records.jsonl>
./g8e public push
./g8e public status
```

`public publish` durably appends before transmission. `public push` retries ordered outbox and proof delivery without running inference. A failed transmission remains retryable.

## Related documentation

- [Public Spectator Architecture and Threat Model](../architecture/public_spectator.md)
- [Docker Gateway Guide](docker_gateway.md#evaluation-publish-and-verify-split-host-and-container)
- [Unified Docker Stack Guide](unified_stack.md)
- [Generator-Neutral Builder Guide](build_observe_frontend.md)
