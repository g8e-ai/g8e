# OpenDevOps.ai evaluation explorer

## Prerequisites

Run commands from `/home/bob/g8e` unless a section changes directory. Docker Engine and Compose are available, the repository `g8e` binary exists, Node dependencies are installed in `dashboard/g8e-adapter/evaluation-explorer`, the Ensemble eval environment resolves with `uv run --locked`, the CLI has a valid `./g8e auth context`, and the approved Ollama provider is reachable only at `http://192.168.1.2:11434`. Do not install, start, pull, or manage Ollama on this host.

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

## Start the real historical experience

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:real
```

This regenerates and seeds the 1,221-record historical corpus when needed, starts or reuses the mirror, verifies mirror health, and starts Vite without launching a provider evaluation. Use `npm run seed` to regenerate and publish historical records separately and `npm run health` to inspect publisher sequence, mirror sequence, freshness, route isolation, and the last accepted record.

A reviewed contract revision appends without deleting prior history only when every input record carries one exact new schema version and publisher/mirror state is synchronized:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
node scripts/seed.mjs --records scripts/fixtures/projected-records.jsonl --append-revision 1.1.0
```

The append command fails rather than resetting drifted state. Run it once per revision. Ordinary `npm run seed` retains its disposable-development behavior and may request a reset when the same dataset has changed.

## Optional disposable public-feed reset

This reset removes the host `.g8e/public-feed` and `.g8e/public-mirror` state, creates a new local public signing chain, and republishes the selected seed. It does not remove evaluation report roots or Docker component volumes. Obtain explicit owner confirmation immediately before running it, then use one of:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npm run dev:mock -- --reset
npm run dev:real -- --reset
```

Do not use a full Docker or gateway cleanup for a public-feed problem.

## Publish evaluation reports continuously

The supervised report publisher scans `/home/bob/g8e/.local.dev/campaign/public-live` recursively, emits only public-safe projections from committed report records, retains its durable outbox during publisher or mirror outages, and retries every five seconds without rerunning inference. Place each separately authorized finite controller or evaluation report under a fresh child directory in that report root. The publisher does not authorize provider work, generate campaign authority, resume dead evidence, or convert the finite attended controller into an unbounded eval daemon.

```bash
install -Dm600 deploy/systemd/opendevops-eval-publisher.service /home/bob/.config/systemd/user/opendevops-eval-publisher.service
systemctl --user daemon-reload
systemctl --user enable --now opendevops-eval-publisher.service
systemctl --user status opendevops-eval-publisher.service
```

A publication failure leaves the bridge batch in `/home/bob/g8e/.local.dev/campaign/public-live-bridge/outbox`. Recovery publishes that exact batch in order when the local publisher and mirror become available. Disclosure validation failures remain fatal and require correcting the producer or projection; the supervisor does not retry rejected content.

## Run a managed finite campaign

Create one reviewed, non-secret campaign JSON file for each newly authorized operation. Keep the campaign identity, output root, profile, model registry, dataset, endpoint, authentication root, and finite request, token, and USD ceilings in that file. The schema and example are documented in [Evals](../../../docs/ensemble/evals.md#run-managed-campaigns). Never reuse the interrupted `ef7-final-response-20260914-1551` output root or any completed report root.

```bash
cd /home/bob/g8e/ensemble/evals
uv run --locked g8e-evals campaign check <campaign.json>
uv run --locked g8e-evals campaign start <campaign.json> --yes
uv run --locked g8e-evals campaign status <campaign.json>
```

`check` is provider-free. It validates the strict file, bound paths, profile, registry, and finite budgets. `start` is the only provider-backed step and requires explicit acknowledgment. `status` reads the same file, so operators do not repeat report paths or suite names. The continuous publisher discovers committed records under the configured `public-live` child root independently; publication retry never reruns this command.

## Run one fresh live evaluation

Choose paths that do not already exist. Never resume or overwrite a prior report root or bridge state directory.

```bash
export REPORT_ROOT=/home/bob/g8e/.local.dev/campaign/live-opendevops-YYYYMMDD-vN
export BRIDGE_STATE=/home/bob/g8e/.local.dev/campaign/live-opendevops-YYYYMMDD-vN-bridge
```

Start the bridge before the evaluation and leave it running:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
python3 scripts/bridge.py \
  --report-root "$REPORT_ROOT" \
  --state-dir "$BRIDGE_STATE" \
  --g8e-bin /home/bob/g8e/g8e \
  --g8e-cwd /home/bob/g8e
```

In a second terminal, launch the approved five-task diagnostic exactly once:

```bash
cd /home/bob/g8e/ensemble/evals
G8E_APP_TRUST_BUNDLE=/home/bob/g8e/.g8e/pki/trust/g8eg-ca-bundle.pem \
G8E_GATEWAY_TRUST_BUNDLE=/home/bob/g8e/.g8e/pki/trust/g8eg-ca-bundle.pem \
uv run --locked g8e-evals run \
  --suite ifeval_subset --arm ensemble_ungoverned --limit 5 \
  --provider ollama --model gemma4:e4b --primary-endpoint http://192.168.1.2:11434 \
  --assistant-provider ollama --assistant-model gemma4:e2b --assistant-endpoint http://192.168.1.2:11434 \
  --lite-provider ollama --lite-model qwen2.5:0.5b --lite-endpoint http://192.168.1.2:11434 \
  --g8ee-url http://localhost:8000 \
  --auth-project-root /home/bob/g8e \
  --evidence-key-file /home/bob/g8e/.local.dev/campaign/diagnostic-evidence-key.json \
  --output-dir "$REPORT_ROOT"
```

The standalone runner commits its detailed artifacts after the task loop. The browser receives queued and started records immediately, then committed assignment, stage, metric, and terminal records. After the process exits, stop the polling bridge and publish the producer exit disposition from the same report and state paths:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
python3 scripts/bridge.py \
  --report-root "$REPORT_ROOT" \
  --state-dir "$BRIDGE_STATE" \
  --g8e-bin /home/bob/g8e/g8e \
  --g8e-cwd /home/bob/g8e \
  --once --final-status completed
```

Use `--final-status failed` or `--final-status stopped` when that is the actual producer disposition. Never relabel an invalid-evidence or failed report as completed.

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
python3 scripts/test_bridge.py
npm audit --audit-level=moderate
npm run health
```

For an Ensemble production-code change:

```bash
cd /home/bob/g8e/ensemble
uv sync --locked --extra dev --extra test --reinstall-package g8e
uv run --locked --extra dev --extra test make lint
uv run --locked --extra test python -m pytest tests/unit/
```

Rebuild and replace only the affected Ensemble container:

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
install -Dm600 deploy/systemd/opendevops-eval-publisher.service /home/bob/.config/systemd/user/opendevops-eval-publisher.service
systemctl --user daemon-reload
systemctl --user enable --now g8e-public-mirror.service
systemctl --user enable --now opendevops-feed-tunnel.service
systemctl --user enable --now opendevops-eval-publisher.service
systemctl --user status g8e-public-mirror.service opendevops-feed-tunnel.service opendevops-eval-publisher.service
```

User lingering must be enabled for restart-on-boot before login. This host currently reports lingering disabled. Enabling it is an owner/admin action:

```bash
sudo loginctl enable-linger bob
```

Stopping either service preserves publisher and mirror state. Disable the tunnel service first if external route isolation or disclosure acceptance fails; do not reset feed state, rotate keys, or delete the tunnel as an ordinary rollback.

## Focused troubleshooting

- **Ensemble unhealthy:** run `./g8e auth pending-platform-enrollments`, inspect `./g8e docker logs ensemble`, approve the exact pending request when appropriate, and recheck `./g8e docker status`.
- **Mirror unreachable:** run `npm run health`; verify host listeners `8081` and `8082` separately. The browser uses only `8082` and publisher ingest uses only `8081`.
- **Publisher retry:** run `./g8e public push`, then `npm run health`. A retained outbox retries in order without rerunning inference.
- **Malformed projection:** run the projector, bridge, contract, and feed-transport tests. Do not bypass strict validation or publish a known-invalid corpus.
- **No SSE:** verify `/stream` through `npm run health`, reload to reconstruct history, and confirm browser requests do not target `8081`, Gateway ports, report paths, or the provider.
- **Terminal evaluation failure:** retain the report and failed public record, publish the actual standalone exit disposition, reconcile attempt and metric counts, and use a fresh report root only after fixing an offline defect. Never retry under the same run identity.

Stop the foreground supervisor or bridge with Ctrl-C. Stopping these processes preserves mirror state and report roots. Full Docker cleanup, gateway cleanup, volume pruning, and public-feed reset are destructive operations and require immediate explicit confirmation for their exact scope.
