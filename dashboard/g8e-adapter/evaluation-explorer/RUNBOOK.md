# OpenDevOps.ai evaluation explorer

## Prerequisites

Run commands from `/home/bob/g8e` unless a section changes directory. Docker Engine and Compose are available, the repository `g8e` binary exists, Node dependencies are installed in `dashboard/g8e-adapter/evaluation-explorer`, the CLI has a valid `./g8e auth context`, and the approved Ollama provider is reachable only at `http://192.168.1.2:11434`. Do not install, start, pull, or manage Ollama on this host. The native evaluation suite does not call Ollama or g8ee; the Ollama variable is supplied only because Compose interpolation requires it.

## Start and verify the platform

```bash
cd /home/bob/g8e
./g8e docker start --full --skip-enroll
./g8e auth pending-platform-enrollments
./g8e docker status
./g8e auth context
curl -fsS http://127.0.0.1:8000/health
curl -fsS http://192.168.1.2:11434/api/version
```

Gateway, Operator, and Ensemble report healthy before a real evaluation. Approve an exact pending workload request only through the documented enrollment flow; do not reset the trust domain to repair an enrollment wait.

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

This starts or reuses the mirror, verifies mirror health, and starts Vite without launching a native evaluation. Use `npm run health` to inspect publisher sequence, mirror sequence, freshness, route isolation, and the last accepted record.

Run three fresh native evaluations and project them into the public feed:

```bash
cd /home/bob/g8e
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434 ./g8e eval run core-execution-boundary --json
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434 ./g8e eval run core-execution-boundary --json
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434 ./g8e eval run core-execution-boundary --json
```

Verify and inspect each run independently:

```bash
./g8e eval verify <run-id> --json
./g8e eval show <run-id> --json
```

Project the persisted native runs into public-safe records and publish them through the real publisher and mirror:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
python3 scripts/project.py \
  --run-dir /home/bob/g8e/.g8e/data/eval/runs/<run-id-1> \
  --run-dir /home/bob/g8e/.g8e/data/eval/runs/<run-id-2> \
  --run-dir /home/bob/g8e/.g8e/data/eval/runs/<run-id-3> \
  --out .local.dev/native-eval-records.jsonl
./g8e public publish --records .local.dev/native-eval-records.jsonl
```

The projector reads only canonical `report.json` and `verification.json` from each run directory, verifies the report/run/verification bindings and content-addressed verification reference, reconciles attempts, assertions, verdicts, metrics, counts, posture, lane, and verification status, and emits deterministic mirror-ready JSONL. One invocation emits one shared `native-core-execution-boundary` live-run catalog plus one evaluation summary per supplied run. Each summary carries nested typed native scenario, invariant verdict, metric, and verification detail and emits no model, provider, assignment, campaign, principal, Operator, session, endpoint, target path, receipt, audit, envelope, or evidence body.

## Optional disposable public-feed reset

This reset removes the host `.g8e/public-feed` and `.g8e/public-mirror` state, creates a new local public signing chain, and republishes the selected seed. It does not remove evaluation report roots or Docker component volumes. Obtain explicit owner confirmation immediately before running it, then use one of:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:mock -- --reset
```

Do not use a full Docker or gateway cleanup for a public-feed problem.

## Publish native evaluation reports

The native evaluation projector reads persisted canonical `report.json` and `verification.json` from `.g8e/data/eval/runs/<run-id>/` and emits only public-safe projections. Publication uses the real `g8e public publish` command, which signs batches, writes the durable outbox, and advances the publisher high-water sequence. The mirror ingests signed batches and serves anonymous reads and SSE.

See the [Start the real native evaluation experience](#start-the-real-native-evaluation-experience) section for the exact projection and publication commands.

## Expected browser states

The overview first reconstructs bootstrap, complete paginated history, and the sealed snapshot, then connects SSE. A live run appears as queued, running, provisional assignment and metric updates, and finally completed, failed, or stopped without a refresh. Reload reconstructs the same state before SSE resumes. Exploratory, verified-public, live-in-progress, failed, unavailable, and not-evaluated states remain distinct; the site never averages datasets.

## Verification

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run lint
npm run typecheck
npm test
npm run build
npm run test:e2e
python3 scripts/test_projector.py
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

The checked-in local runtime remains loopback-only for development. `npm run build` replaces the emitted runtime with the exact bytes from `runtime.production.json`, disables source maps, scans the complete asset tree for prohibited content, and fails on any unexpected file. The Cloudflare Worker configuration in this directory packages only `dist/`.

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm ci
npm run lint
npm run typecheck
npm test
npm run build
npm audit --audit-level=moderate
wrangler deploy --dry-run
```

The production runtime names only `https://feed.opendevops.ai`. A real `wrangler deploy` remains an explicit release-owner action and runs only after external feed acceptance and rollback capture.

## Host service supervision

The checked-in user service units run the mirror and dedicated Cloudflare Tunnel in the Docker-host namespace. The mirror continues to use host `.g8e/` durable state and loopback listeners. The tunnel reads its generated configuration and credentials from the owner-protected `/home/bob/.cloudflared` directory; neither unit contains a token, key, or ingest credential.

After the release owner creates the dedicated `opendevops-feed` tunnel and DNS route, install the units in a controlled window when no foreground mirror or tunnel owns the same ports or tunnel session:

```bash
install -Dm600 deploy/systemd/g8e-public-mirror.service /home/bob/.config/systemd/user/g8e-public-mirror.service
install -Dm600 deploy/systemd/opendevops-feed-tunnel.service /home/bob/.config/systemd/user/opendevops-feed-tunnel.service
systemctl --user daemon-reload
systemctl --user enable --now g8e-public-mirror.service
systemctl --user enable --now opendevops-feed-tunnel.service
systemctl --user status g8e-public-mirror.service opendevops-feed-tunnel.service
```

User lingering must be enabled for restart-on-boot before login. This host currently reports lingering disabled. Enabling it is an owner/admin action:

```bash
sudo loginctl enable-linger bob
```

Stopping either service preserves publisher and mirror state. Disable the tunnel service first if external route isolation or disclosure acceptance fails; do not reset feed state, rotate keys, or delete the tunnel as an ordinary rollback.

## Focused troubleshooting

- **Ensemble unhealthy:** run `./g8e auth pending-platform-enrollments`, inspect `./g8e docker logs ensemble`, approve the exact pending request when appropriate, and recheck `./g8e docker status`.
- **Mirror unreachable:** run `npm run health`; verify host listeners `8081` and `8082` separately. The browser uses only `8082` and publisher ingest uses only `8081`.
- **Publisher retry:** run `./g8e public push`, then `npm run health`. A retained outbox retries in order without rerunning evaluation.
- **Malformed projection:** run the projector, contract, and feed-transport tests. Do not bypass strict validation or publish a known-invalid corpus.
- **No SSE:** verify `/stream` through `npm run health`, reload to reconstruct history, and confirm browser requests do not target `8081`, Gateway ports, report paths, or the provider.
- **Terminal evaluation failure:** retain the report and failed public record, publish the actual exit disposition, reconcile attempt and metric counts, and use a fresh run directory only after fixing an offline defect. Never retry under the same run identity.

Stop the foreground mirror with Ctrl-C. Stopping this process preserves mirror state and report roots. Full Docker cleanup, gateway cleanup, volume pruning, and public-feed reset are destructive operations and require immediate explicit confirmation for their exact scope.
