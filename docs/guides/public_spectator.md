# Public Spectator Operations Guide

## Runtime boundaries

The host-backed public spectator runs one `g8e public mirror run` process in the Docker-host namespace with two distinct loopback listeners over one durable mirror store. The private listener defaults to `127.0.0.1:8081` and serves authenticated publisher ingest, proof ingest, replacement-key registration, and anonymous reads for local diagnostics. The public listener defaults to `127.0.0.1:8082` and mounts only anonymous bootstrap, snapshot, history, SSE, proof catalog, proof manifest, and content-addressed proof download routes. Both addresses must be unique and loopback-only.

The mirror process opens the host `.g8e/` runtime tree through `RuntimeFileService`. Its durable source, key, revocation, batch, proof, catalog, and manifest state resides in the public-mirror state path in that tree. The local publisher uses the private listener and authenticates with the private ingest token. Cloudflared uses the public listener as its plain-HTTP origin. A public browser reaches `https://feed.opendevops.ai` through Cloudflare and never reaches the Gateway, the private mirror listener, a component volume, or an Operator execution boundary.

The mirror is a visibility and publication boundary, not a Policy Decision Point or Policy Execution Point. It does not authorize a governed mutation, and its availability is not execution evidence. The Operator whose L4/L5 boundary produced an underlying governed result retains authoritative local execution evidence.

## Initialize publication state

Run initialization once with a public-safe deployment pseudonym and the private loopback mirror origin:

```bash
./g8e public init --source-id <public-source-pseudonym> --mirror-origin http://127.0.0.1:8081
```

Initialization creates the private signing key, ingest token, and export configuration under the host runtime tree. It refuses to overwrite existing state. Do not print, copy, or place these secrets in Cloudflare configuration, site assets, reports, proof packages, or logs.

## Start the mirror

Start both loopback listeners from one process:

```bash
./g8e public mirror run --listen 127.0.0.1:8081 --public-listen 127.0.0.1:8082
```

The process loads and validates the complete durable mirror state before listening. Corrupt, equivocal, incomplete, oversized, unsafe, or signature-invalid state fails closed. Restart the same command to recover accepted batches, high-water sequence, feed-chain hash, keys, revocations, proofs, catalog, and manifest. SSE subscriber queues and anonymous rate windows are bounded delivery-only memory and are not durable governance or evidence state.

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

A bounded SSE client connects to `/stream`, supplies the source pseudonym and optional `since_id`, and reconciles sequence plus feed-chain state against `/snapshot`. Public clients omit credentials. Freshness is derived from the last accepted batch and transitions honestly through active, delayed, stale, intentionally stopped, safety stopped, and source offline.

## Create the Cloudflare tunnel

Tunnel creation changes Cloudflare tunnel and DNS state and remains a release-owner operation. The tunnel origin is the read-only public listener, never the private listener and never the Gateway:

```bash
./g8e gw tunnel create --name opendevops-feed --hostname feed.opendevops.ai --service http://127.0.0.1:8082
./g8e gw tunnel run
```

The explicit plain-HTTP service prevents Gateway HTTPS origin settings from being inherited. The generated cloudflared ingress terminates at the loopback-only read adapter. Do not configure `feed.opendevops.ai` to target port 8081, Gateway ports 8080 or 8443, the Ensemble, the Dashboard, a container bridge address, or a component-local volume.

## External acceptance

After the owner starts the tunnel, verify HTTPS, CORS, bounded reads, rate limiting, replayable SSE, proof catalog, proof manifest, and content-addressed proof download from an external client. Confirm `/ingest`, `/keys/register`, `/proof-ingest`, Gateway, MCP, A2A, approval, audit, filesystem, eval-launch, and arbitrary paths are absent from the public origin. Scan public responses and proof bytes for credentials, private endpoints, machine paths, SPIFFE identities, provider topology, raw prompts, model outputs, reasoning traces, and restricted canaries; any match blocks publication.

Restart the mirror and tunnel independently. The mirror must recover the same accepted high-water sequence, feed-chain hash, key revocations, catalog, manifest, and proof bytes. Tunnel or mirror unavailability must produce an honest stale or source-offline storefront state and must not trigger inference or mutate accepted evidence.

## Publish and retry

Publish only verifier-clean, disclosure-approved public records and proofs:

```bash
./g8e public publish <public-records.jsonl>
./g8e public push
./g8e public status
```

`public publish` durably appends before transmission. `public push` retries ordered outbox and proof delivery without running inference. A failed transmission remains retryable. Key rotation pre-registers the replacement key through the private authenticated listener, publishes the old-key-signed revocation, switches local signing state, and persists recovery state so restart cannot emit a second revocation.

## Related documentation

- [Public Spectator Architecture and Threat Model](../architecture/public_spectator.md)
- [Unified Docker Stack Guide](unified_stack.md)
- [Generator-Neutral Builder Guide](build_observe_frontend.md)
