# OpenDevOps.ai evaluation explorer

## Prerequisites

Run commands from `/home/bob/g8e` unless a section changes directory. Docker Engine and Compose are available, the repository `g8e` binary exists, Node dependencies are installed in `dashboard/g8e-adapter/evaluation-explorer`, the CLI has a valid `./g8e auth context`, and the approved Ollama provider is reachable only at `http://192.168.1.2:11434`. Do not install, start, pull, or manage Ollama on this host. The native evaluation suite does not call Ollama or g8ee; the Ollama variable is supplied only because Compose interpolation requires it.

On the **provider host** (where Ollama runs), enroll two separate witness operator sessions when running full campaigns:

1. **Observer Operator** — `--provider-boundary-observer-enabled` for provider-boundary resource observation.
2. **Provenance Operator** — `--provenance-operator-enabled` with `--model-storage-root` pointing at the Ollama models directory (for example `~/.ollama/models`).

See [Unified Docker Stack Guide](../../../docs/guides/unified_stack.md#provider-boundary-observer-operator-windows-ollama-host) and [Storage-side Provenance Operator](../../../docs/guides/unified_stack.md#storage-side-provenance-operator).

## Start and verify the platform

```bash
cd /home/bob/g8e
./g8e docker start --full --skip-enroll
./g8e auth enroll pending
./g8e docker status
./g8e auth context
curl -fsS http://127.0.0.1:8000/health
curl -fsS http://192.168.1.2:11434/api/version
```

Gateway, Operator, and Ensemble report healthy before a real evaluation. Approve or deny an exact pending workload request only through the documented `auth enroll` flow (`pending`, `approve`, `deny`); do not reset the trust domain to repair an enrollment wait.

## Start the complete mock experience

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:mock
```

Open `http://127.0.0.1:5173`. This command starts or reuses the host mirror on private `127.0.0.1:8081` and public read-only `127.0.0.1:8082`, publishes deterministic fixture history, replays a scripted lifecycle through the real publisher and mirror, and starts Vite. The browser connects only to `127.0.0.1:8082` with credentials omitted.

## Start the real native evaluation experience

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:real
```

Requires the gateway-owned mirror (`docker compose up -d g8e-gateway` or `./g8e gw start --public-spectator`). Use `npm run health` to inspect mirror state, publisher sequence, freshness, route isolation, and the last accepted record.

Run a campaign evaluation with live publication to the public mirror:

```bash
cd /home/bob/g8e
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434 ./g8e eval campaign execute --run-id <run-id> --publish --daemon
```

Or catch up publication for an existing run:

```bash
./g8e eval campaign publish --run-id <run-id>
```

The Go `CampaignPublicationCoordinator` in the evaluation service projects canonical campaign state into public-safe explorer records and publishes them through the real `g8e public` publisher. The explorer's TypeScript `campaign-adapter` decodes those envelopes from the mirror. No separate projector script is involved.

## Optional disposable public-feed reset

This reset removes the host `.g8e/public-feed` and `.g8e/public-mirror` state, creates a new local public signing chain, and republishes the selected seed. It does not remove evaluation report roots or Docker component volumes. Obtain explicit owner confirmation immediately before running it, then use one of:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:mock -- --reset
```

Do not use a full Docker or gateway cleanup for a public-feed problem.

## Publish evaluation reports

Publication is owned by the Go evaluation service. Campaign runs project lifecycle events, assignment results, and aggregate records into the public feed as they execute (`--publish`) or on demand (`g8e eval campaign publish`). The publisher signs batches, writes the durable outbox, and advances the high-water sequence; the mirror ingests signed batches and serves anonymous reads and SSE.

## Expected browser states

The overview first reconstructs bootstrap, complete paginated history, and the sealed snapshot, then connects SSE. A live run appears as queued, running, provisional assignment and metric updates, and finally completed, failed, or stopped without a refresh. Reload reconstructs the same state before SSE resumes. Current-standard verified, run-scoped verified, not fully verified, legacy unverified, in-progress, failed, unavailable, and not-evaluated states remain distinct; the site never averages datasets.

## Verification

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run lint
npm run typecheck
npm test
npm run build
npm run test:e2e
npm audit --audit-level=moderate
npm run health
```

For a platform production-code change:

```bash
cd /home/bob/g8e
make build
./g8e test unit
./g8e test lint
./g8e test coverage
```

Rebuild and replace only the affected platform container when its image changed:

```bash
cd /home/bob/g8e
docker compose build ensemble
docker compose --profile bootstrapped up -d --no-deps ensemble
curl -fsS http://127.0.0.1:8000/health
```

## Production build and packaging

The checked-in local runtime remains loopback-only for development. `npm run build` replaces the emitted runtime with the exact bytes from `runtime.production.json`, disables source maps, scans the complete asset tree for prohibited content, and fails on any unexpected file. Production does **not** use Cloudflare Workers or Pages; the gateway-owned public listener on `8082` serves `dist/` and the anonymous mirror API from one origin.

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=moderate
```

After `npm run build`, restart or recreate `g8e-gateway` so Docker remounts `dist/`. The production runtime names only `https://opendevops.ai`. External browsers reach that origin through the dedicated `opendevops-feed` Cloudflare Tunnel, which terminates at loopback `http://127.0.0.1:8082`.

## Host service supervision

The gateway owns the in-process public mirror (`8081` private ingest, `8082` public read/SSE) and evaluation explorer when started with `--public-spectator` (default in Compose). Do not install a host mirror systemd unit for production.

The checked-in `deploy/systemd/opendevops-feed-tunnel.service` runs only the Cloudflare Tunnel in the Docker-host namespace. It reads generated configuration and credentials from the owner-protected `/home/bob/.cloudflared` directory and forwards `https://opendevops.ai` to `http://127.0.0.1:8082`. Neither the tunnel unit nor the gateway image contains a token, key, or ingest credential in source control.

After the release owner creates the dedicated `opendevops-feed` tunnel and DNS route, install the tunnel unit when no other process owns the same tunnel session:

```bash
install -Dm600 deploy/systemd/opendevops-feed-tunnel.service /home/bob/.config/systemd/user/opendevops-feed-tunnel.service
systemctl --user daemon-reload
systemctl --user enable --now opendevops-feed-tunnel.service
systemctl --user status opendevops-feed-tunnel.service
```

User lingering must be enabled for restart-on-boot before login:

```bash
sudo loginctl enable-linger bob
```

Stopping the tunnel preserves publisher and mirror state. Disable the tunnel service first if external route isolation or disclosure acceptance fails; do not reset feed state, rotate keys, or delete the tunnel as an ordinary rollback.

## Focused troubleshooting

- **Ensemble unhealthy:** run `./g8e auth enroll pending`, inspect `./g8e docker logs ensemble`, approve the exact pending request when appropriate, and recheck `./g8e docker status`.
- **Mirror unreachable:** run `npm run health`; verify host listeners `8081` and `8082` separately. The browser uses only `8082` and publisher ingest uses only `8081`.
- **Publisher retry:** run `./g8e public push`, then `npm run health`. A retained outbox retries in order without rerunning evaluation.
- **Malformed projection:** run the contract and feed-transport tests plus `./g8e test unit --pkg ./internal/services/evaluation`. Do not bypass strict validation or publish a known-invalid corpus.
- **No SSE:** verify `/stream` through `npm run health`, reload to reconstruct history, and confirm browser requests do not target `8081`, Gateway ports, report paths, or the provider.
- **Terminal evaluation failure:** retain the report and failed public record, publish the actual exit disposition, reconcile attempt and metric counts, and use a fresh run directory only after fixing an offline defect. Never retry under the same run identity.

Stop the foreground mirror with Ctrl-C. Stopping this process preserves mirror state and report roots. Full Docker cleanup, gateway cleanup, volume pruning, and public-feed reset are destructive operations and require immediate explicit confirmation for their exact scope.
