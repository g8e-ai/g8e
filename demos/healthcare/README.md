# Healthcare Demo — Oregon Prior Authorization System

This demo deploys a realistic Oregon prior authorization (PA) processing environment behind g8e, demonstrating PHI protection, HIPAA enforcement, and real-time doctrine-based threat detection for AI agents operating in a clinical context.

---

## Regulatory Context

The simulated PA system models three distinct Oregon legislative requirements:

| Law / Rule | Effective | What It Requires |
|---|---|---|
| **HB 3134** — PA API Mandate | 2027 | Health plans must expose a FHIR R4-compliant API for PA submissions, replacing fax-based workflows. EHR integration is mandatory. |
| **HB 3134** — Gold Carding | 2027 | Providers whose historic approval rate meets or exceeds a plan-defined threshold (commonly 90%) receive automatic exemption from PA review for qualifying procedure categories. |
| **2026 CCO Medicaid Rule** | 2026 | Coordinated Care Organizations must resolve standard PA requests within 7 calendar days. Plans are expected to alert internal reviewers at day 5 to prevent breach. |
| **DCBS / OHA Annual Reporting** | Ongoing | Plans must submit annual compliance reports to the Oregon Division of Financial Regulation (DCBS) and Oregon Health Authority (OHA) by March 1 (standard PAs) and March 31 (expedited PAs), including denial rates and median decision times. |

---

## What This Demo Shows

g8e sits in front of the entire PA system. Every request — whether it comes from an authorized AI agent, a payer integration, or an adversary — passes through the gateway before reaching any PA service. The doctrine layer inspects each request for PHI exposure, HIPAA violations, and PA-specific attack patterns in real time.

The demo demonstrates:

- **Governed PA operations** via native gateway tools (`run_shell_command` driving the `paop` wrapper) with real doctrine enforcement on every request
- **11 PHI/HIPAA doctrine rules** evaluated on every request
- **Gold card auto-approval** (HB 3134 §6) for providers with historic approval rate ≥ 90%
- **SLA breach tracking** with day-5 alerts and day-7 breach flags for mandatory DCBS/OHA reporting
- **Two-layer PHI defense**: network isolation + doctrine enforcement against exfiltration
- **Metabase compliance dashboards** pre-loaded with DCBS/OHA filing queries

The three core demonstration narratives:

1. **Authorized PA flow** — An AI agent on `net_internal` submits a PA request through the gateway via the native `run_shell_command` tool driving the `paop` wrapper. g8e validates the request against all 11 PHI/HIPAA doctrine rules, the gold carding narrative is carried by the tool arguments, and the compliance dashboard reflects the decision.

2. **SLA enforcement** — The PA-2026-0044 seed record in `init.sql` is already in `SLA_BREACHED` state with `reportable_to_oha: true`, demonstrating the alert and reporting path. PA-2026-0041 is at day 6 with a day-5 alert threshold already triggered.

3. **Bad actor blocked** — A container on `net_untrusted` has no route to `net_internal`. Attempts to reach the gateway directly, exfiltrate PHI, or tamper with FHIR resources are blocked at the network layer and the gateway doctrine engine.

---

## Network Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  net_untrusted  10.20.0.0/24                                │
│    healthcare-bad-actor                                     │
└───────────────────────────┬─────────────────────────────────┘
                            │ no route
┌───────────────────────────▼─────────────────────────────────┐
│  net_perimeter  10.21.0.0/24                                │
│    healthcare-gateway  10.21.0.10   :8081 (HTTP) :8444 (TLS)│
│    healthcare-compliance-ui 10.21.0.20  :3001 (Metabase)   │
└───────────────────────────┬─────────────────────────────────┘
                            │ g8e mTLS enforcement
┌───────────────────────────▼─────────────────────────────────┐
│  net_internal  10.22.0.0/24                                 │
│    healthcare-gateway   10.22.0.10                          │
│    healthcare-operator  10.22.0.20                          │
│    healthcare-agent      (dynamic)                           │
└───────────────────────────┬─────────────────────────────────┘
                            │ operator tunnel only
┌───────────────────────────▼─────────────────────────────────┐
│  net_secure  10.23.0.0/24                                   │
│    healthcare-operator  10.23.0.20                          │
│    healthcare-reporting-db 10.23.0.70  :5433 (Postgres)     │
│    healthcare-compliance-ui 10.23.0.80                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  net_mgmt  10.24.0.0/24  (out-of-band, no cross-routes)     │
│    healthcare-observability                                  │
└─────────────────────────────────────────────────────────────┘
```

Traffic entering from `net_untrusted` has no route into `net_perimeter`. The gateway is the only service that spans `net_perimeter` and `net_internal`. The operator is the only service that spans `net_internal` and `net_secure`. No agent or external service has a direct path to `net_secure`.

---

## Port Mappings

| Service | Host Port |
|---------|-----------|
| Gateway HTTP | 8081 |
| Gateway HTTPS | 8444 |
| Console | https://localhost:8444/console/ |
| Compliance Dashboard (Metabase) | 3001 |
| Reporting DB (Postgres) | 5433 |

This demo uses offset ports to allow simultaneous operation with other demos:

| Service | Healthcare | Gov | Finance |
|---|---|---|---|
| Gateway HTTP | 8081 | 8080 | 8082 |
| Gateway HTTPS | 8444 | 8443 | 8445 |
| Demo UI / Metabase | 3001 | 3000 | 3002 |
| Postgres | 5433 | — | — |

---

## Services

### g8e Gateway — `healthcare-gateway`

**Compose service:** `gateway`  
**Networks:** `net_perimeter` (10.21.0.10), `net_internal` (10.22.0.10)  
**Ports:** `8081` (HTTP), `8444` (HTTPS)

The security enforcement plane. All inbound requests pass through the gateway's doctrine engine. Loaded with `phi_hipaa_doctrine.json` containing 11 detection rules. Runs in doctrine posture — L1 doctrine rules are enforced on every request. The `paop` wrapper script is mounted at `/usr/local/bin/paop` for governed PA operations via `run_shell_command`. Configuration is applied via command-line flags in `compose.yml` rather than the reference `config/gateway.yml` file.

### g8e Operator — `healthcare-operator`

**Compose service:** `operator`  
**Networks:** `net_internal` (10.22.0.20), `net_secure` (10.23.0.20)

The trust anchor on `net_secure`. Enrolls with the gateway over mTLS, receives an identity certificate, and provides the operator session for governed requests. The operator's certificate file at `/root/.g8e/pki/operator.crt` serves as the health check signal for dependent services. Configuration is applied via command-line flags in `compose.yml` rather than the reference `config/operator.yml` file.

### Reporting DB — `healthcare-reporting-db`

**Compose service:** `reporting-db`  
**Networks:** `net_secure` (10.23.0.70)  
**Image:** `postgres:15`  
**Port:** `5433`

Stores all PA decision records, provider exemption history, and compliance metrics. Metabase connects to this database to generate DCBS/OHA reports. Schema and seed data are initialized from `init.sql` on first startup.

```
Database:  oregon_pa_metrics
User:      compliance_admin
Password:  secure_hipaa_password
Port:      5433 (host) / 5432 (container)
```

### Compliance Dashboard — `healthcare-compliance-ui`

**Compose service:** `compliance-dashboard`  
**Networks:** `net_perimeter` (10.21.0.20), `net_secure` (10.23.0.80)  
**Image:** `metabase/metabase`  
**Port:** `3001`  
**Regulatory mapping:** DCBS / OHA Annual Reporting

Public-facing reporting dashboard connected directly to `healthcare-reporting-db`. Used to generate the two mandatory annual state reports: percent of standard vs. expedited requests denied, and median time elapsed for decisions. Spans `net_perimeter` for public access and `net_secure` to read from `healthcare-reporting-db`.

### Metabase Setup — `healthcare-metabase-setup`

**Compose service:** `metabase-setup`  
**Networks:** `net_perimeter`

One-shot service that configures the Metabase admin account, database connection, and pre-loads the two mandatory DCBS/OHA compliance queries. Exits after setup; never restarts.

### Agent Runtime — `healthcare-agent`

**Compose service:** `agent-runtime`  
**Networks:** `net_internal`

Simulates an authorized payer system or clinical AI agent submitting PA requests through the gateway via native `run_shell_command` tools. Sits on `net_internal` — it must go through the gateway to reach any service on `net_secure`.

### Bad Actor — `healthcare-bad-actor`

**Compose service:** `bad-actor`  
**Networks:** `net_untrusted`

Simulates an unauthorized party attempting to exfiltrate PHI or bypass the PA workflow. Isolated to `net_untrusted` with no route to any other network segment. Attempts to reach the gateway (10.22.0.10) or `healthcare-reporting-db` (10.23.0.70) directly will fail at the network layer before reaching any g8e enforcement point.

### Observability — `healthcare-observability`

**Compose service:** `observability`  
**Networks:** `net_mgmt`

Mounts `gateway_state` and `operator_state` volumes read-only. Tails the gateway's operator audit log at `/data/gateway/logs/operator.log` for out-of-band inspection of the enforcement record. Waits for the log file to appear before starting the tail.

---

## Doctrine Rules

All rules live in `doctrine/phi_hipaa_doctrine.json`. The doctrine is a bind mount — no image rebuild required to change rules.

| Rule ID | Name | Category | Severity | Regulatory Mapping |
|---|---|---|---|---|
| `phi_detection` | PHI Data Detection | data_classification | critical | HIPAA §164.514 |
| `phi_exfil_attempt` | PHI Data Exfiltration Attempt | data_exfiltration | critical | HIPAA §164.312(e) |
| `hipaa_minimum_necessary` | HIPAA Minimum Necessary Violation | access_control | high | HIPAA §164.502(b) |
| `phi_encryption_violation` | PHI Encryption Violation | encryption | critical | HIPAA §164.312(a)(2)(iv) |
| `pa_approval_bypass` | Protected Health Information Approval Bypass | authorization | critical | HB 3134 §4 |
| `hipaa_audit_logging` | HIPAA Audit Logging Bypass | audit | high | HIPAA §164.312(b) |
| `phi_cross_boundary_transfer` | Unauthorized PHI Cross-Boundary Transfer | data_transfer | critical | HIPAA §164.514(e) |
| `de_identification_failure` | PHI De-identification Failure | data_privacy | high | HIPAA §164.514(b) |
| `pa_gold_card_bypass` | PA Gold Card Exemption Bypass | authorization | high | HB 3134 §6 (exemptions) |
| `pa_sla_manipulation` | PA SLA Timestamp Manipulation | data_integrity | high | 2026 CCO Medicaid Rule |
| `fhir_resource_tampering` | Unauthorized FHIR Resource Modification | data_integrity | critical | HB 3134 §4 (FHIR integrity) |

---

## Seed Data

The `target-data/` files are narrative reference data documenting the PA request cases referenced by the demo scenarios. They are not consumed by any running service — the compliance dashboard reads from `init.sql` seed data in Postgres. They are kept as documentation of the PA request states the scenarios prove.

### `target-data/ehr_records.json`

Three patient EHR records (PAT-001 through PAT-003) with names and SSNs redacted. Includes active prescriptions, diagnosis, and two pending PHI disclosure requests (PA-001, PA-002) without patient consent — these are the records the bad actor scenario targets.

### `target-data/pa_requests.json`

Four PA requests in the processing queue, covering every state the worker must handle:

| Request | Provider | Procedure | Days Elapsed | Status | Notes |
|---|---|---|---|---|---|
| PA-2026-0041 | Dr. Sarah Chen (94%) | Echo CPT 93306 | 6 | PENDING_REVIEW | Gold card pending verification; SLA deadline tomorrow |
| PA-2026-0042 | Dr. Marcus Webb (71%) | Spirometry CPT 94010 | 2 | IN_REVIEW | Below gold card threshold; normal review |
| PA-2026-0043 | Dr. Priya Nair (96%) | Mammography CPT 77067 | 0 | AUTO_APPROVED | Gold card threshold met; expedited, zero-day decision |
| PA-2026-0044 | Dr. James O'Brien (58%) | Knee arthroplasty CPT 27447 | 10 | SLA_BREACHED | Alert fired day 5; breached day 7; `reportable_to_oha: true` |

### `init.sql`

PostgreSQL schema and seed data for `healthcare-reporting-db`. Creates the `pa_requests` table and inserts 20 records (12 standard, 8 expedited) including the four documented demo cases. Standard denial rate: 25.00%, expedited denial rate: 37.50%.

---

## Prerequisites

- Docker and Docker Compose installed
- g8e binary built at repository root:

```bash
# From repo root
make build
```

This builds the g8e binary and copies it to `demos/bin/g8e` for host-side CLI use. The demo containers build from the repo-root `Dockerfile` (via `context: ../..` in `compose.yml`), which produces a FIPS 140-3 approved-mode image containing the full g8e binary, `docs/reference/`, and `protocol/constants/`. Docker Compose automatically builds images for the `gateway`, `operator`, and `agent-runtime` services on first `docker compose up`.

---

## Running the Demo

### Using the g8e CLI (recommended)

```bash
# Start the healthcare demo
g8e demos start healthcare

# Check service status
g8e demos status healthcare

# Stop the demo
g8e demos stop healthcare

# Clean the demo (remove containers, volumes, and networks)
g8e demos clean healthcare

# Reset the demo (clean and restart)
g8e demos reset healthcare
```

### Manual Docker Compose

```bash
cd demos/healthcare
docker compose up -d
```

Watch startup progress:

```bash
docker compose ps
```

Expected healthy sequence (takes ~60s on first pull):

1. `reporting-db` → healthy
2. `gateway` → healthy
3. `operator` → healthy (operator cert written)
4. `compliance-dashboard` → running
5. `metabase-setup` → runs once, configures Metabase, exits 0

Check gateway and operator logs to confirm enrollment completed:

```bash
docker compose logs gateway
docker compose logs operator
```

Look for operator enrollment confirmation in the gateway log and the identity certificate issuance in the operator log.

---

## Demo Scenarios

### Scenario 1 — Authorized Agent Submits a FHIR PA Request

```bash
g8e demos run healthcare 1
```

**Proves**: An authorized agent on `net_internal` submits a PA request through the g8e gateway via the native `run_shell_command` tool driving the `paop` wrapper. Every request passes through the doctrine engine (11 PHI/HIPAA rules) before execution.

The scenario runs a 3-step flow:
1. **Gateway health check** — confirms the g8e gateway is live
2. **PA submission** — submits a governed `run_shell_command` request driving `paop submit` through the gateway via the agent runtime
3. **Audit trail inspection** — verifies doctrine enforcement in the observability logs

For manual exploration:

```bash
# Check gateway health
curl -s http://localhost:8081/api/v1/health

# View enforcement audit trail
docker compose logs observability --tail 20
```

### Scenario 2 — Gold Card Auto-Approval (HB 3134 §6)

```bash
g8e demos run healthcare 2
```

**Proves**: Providers whose historic approval rate meets or exceeds the plan threshold (90%) are auto-approved without manual review. PA-2026-0043 (Dr. Priya Nair, 96%) is the proof case.

PA-2026-0043 demonstrates the gold card path: Dr. Priya Nair has a 96% historic approval rate, the exemption narrative evaluates against the 90% threshold, and the request resolves to `AUTO_APPROVED` with zero human review time. This is the path that makes HB 3134's efficiency argument — gold-carding eliminates the SLA clock entirely for qualifying providers.

### Scenario 3 — SLA Breach and OHA Reporting (2026 CCO Medicaid Rule)

```bash
g8e demos run healthcare 3
```

**Proves**: The PA worker tracks days-elapsed per request and flags breaches for mandatory DCBS/OHA annual reporting. PA-2026-0044 (Dr. James O'Brien, 10 days) is the proof case.

PA-2026-0044 is already in `SLA_BREACHED` state in the `init.sql` seed data, with `reportable_to_oha: true`. The SLA configuration (7-day timeout, 5-day alert) is documented in the narrative and reflected in the seed data. Navigate to the Metabase dashboard to build the denial rate and median decision time queries against `healthcare-reporting-db`.

### Scenario 4 — Bad Actor PHI Exfiltration Blocked

```bash
g8e demos run healthcare 4
```

**Proves**: Two-layer defense — Layer 1: network isolation (bad-actor on `net_untrusted` has no route to `net_internal`/`net_secure`). Layer 2: doctrine enforcement (`phi_exfil_attempt` at confidence 0.95).

The scenario runs a 2-layer test:
1. **Network isolation** — verifies that the bad-actor container on `net_untrusted` cannot reach the gateway on `net_internal`
2. **Doctrine enforcement** — submits a PHI exfiltration payload through the governed endpoint; the gateway blocks it at confidence ≥ 0.95

For manual exploration:

```bash
# Test network isolation from bad-actor container
docker compose exec -T bad-actor sh -c \
  "wget -qO- -T 5 http://10.22.0.10:8080/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_internal'"

# View doctrine enforcement in audit trail
docker compose logs observability --tail 20
```

### Run all scenarios

```bash
g8e demos run healthcare
```

---

## Viewing Results

### Compliance Dashboard (Metabase) — `http://localhost:3001`

The dashboard is auto-configured on startup by the `metabase-setup` one-shot service. No manual setup wizard is required.

**Login credentials:**
- Username: `admin@g8e.local`
- Password: `Metabase1!`

Pre-loaded compliance queries are available in the **Questions** section:

- **DCBS March 1 Filing - Denial Rates by Request Type** — Percent of standard vs expedited requests denied
- **OHA March 31 Filing - Median Decision Time** — Median time elapsed for decisions and SLA breaches

If the setup service fails to create the queries (e.g., due to timing), you can manually create them using the SQL below:

**Report 1 — Denial rates by request type (March 1 filing)**
```sql
SELECT
  request_type,
  COUNT(*) AS total,
  SUM(CASE WHEN status = 'DENIED' THEN 1 ELSE 0 END) AS denied,
  ROUND(100.0 * SUM(CASE WHEN status = 'DENIED' THEN 1 ELSE 0 END) / COUNT(*), 2) AS denial_pct
FROM pa_requests
GROUP BY request_type;
```

**Report 2 — Median decision time (March 31 expedited filing)**
```sql
SELECT
  PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY days_elapsed) AS median_days,
  MAX(days_elapsed) AS max_days,
  SUM(CASE WHEN status = 'SLA_BREACHED' THEN 1 ELSE 0 END) AS breached_count
FROM pa_requests;
```

### Postgres (direct) — `localhost:5433`

```bash
psql -h localhost -p 5433 -U compliance_admin -d oregon_pa_metrics
```

### Gateway Health — `http://localhost:8081`

```bash
curl http://localhost:8081/api/v1/health
```

### Audit Log Stream

```bash
docker compose logs -f observability
```

This tails the gateway's operator audit log in real time. Each governed request shows the doctrine rules evaluated, confidence scores, and the allow/block decision.

### Per-service logs

```bash
docker compose logs -f gateway
docker compose logs -f operator
```

---

## Architecture Notes

### Gateway Posture: Doctrine

The gateway runs in doctrine posture, meaning:
- L1 Doctrine validation is **enforced** (fail-closed)
- L2 Consensus signatures are **not required**
- L3 Notary proofs are audited but not required

Healthcare scenarios use native gateway tools (`run_shell_command` driving the `paop` wrapper) governed by the doctrine engine, consistent with the fedramp (`cloudop`) and dhs (`dataop`) demos. No downstream MCP server is involved — the governance proof happens at the gateway layer.

### Data Sovereignty

All audit data is committed locally:
- **Git-backed ledger**: Immutable execution history on the operator container
- **SQLite audit vault**: Queryable audit trail on both gateway and operator
- **Postgres reporting DB**: Compliance metrics for DCBS/OHA annual reporting
- No data leaves the demo environment unless explicitly transmitted

---

## File Reference

```
demos/healthcare/
├── compose.yml                          # Full environment definition
├── README.md                            # This file
├── init.sql                             # PostgreSQL schema + seed data for reporting DB
├── paop.sh                              # PA operation wrapper (governed run_shell_command bridge)
├── setup_metabase.py                    # One-shot Metabase configuration (run by metabase-setup service)
├── config/
│   ├── gateway.yml                      # g8e gateway config reference (not used in compose.yml)
│   └── operator.yml                     # g8e operator config reference (not used in compose.yml)
├── doctrine/
│   └── phi_hipaa_doctrine.json          # 11 detection rules (PHI, HIPAA, PA-specific)
└── target-data/
    ├── ehr_records.json                  # 3 patient EHR records + 2 PHI disclosure requests (narrative reference)
    └── pa_requests.json                  # PA queue with 4 requests covering all states (narrative reference)
```

---

## Troubleshooting

### Gateway health check not passing

```bash
docker compose logs gateway
```

If the binary is missing: confirm `demos/bin/g8e` exists (`make build` from repo root).

### Operator enrollment failing

```bash
docker compose logs operator
```

The operator needs the gateway healthy first. Verify:

```bash
docker compose exec operator ping -c 1 g8e.local
```

`g8e.local` resolves to `10.22.0.10` via the `extra_hosts` entry.

### Metabase won't start

Metabase requires `reporting-db` to be healthy before it can initialize. Check:

```bash
docker compose logs reporting-db
docker compose ps reporting-db
```

If the DB is healthy but Metabase is stuck, restart it:

```bash
docker compose restart compliance-dashboard
```

### Compliance queries not auto-loaded

The `metabase-setup` service runs once to pre-load queries. Check its logs:

```bash
docker compose logs metabase-setup
```

If the service exited non-zero, re-run the setup script against the live Metabase:

```bash
docker compose run --rm metabase-setup python /app/setup_metabase.py
```

Or create the queries manually through the Metabase UI using the SQL provided in the Viewing Results section.

### Doctrine not loading

```bash
docker compose exec gateway ls -la /etc/g8e/doctrine/
docker compose exec gateway cat /etc/g8e/doctrine/phi_hipaa_doctrine.json | python3 -m json.tool
```

---

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

---

## License

Business Source License 1.1 (BSL 1.1). Converts to Apache 2.0 on 2030-08-18.
