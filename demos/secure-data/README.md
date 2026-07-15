# Secure Data Migration Demo

This demo simulates a **governed SharePoint-to-SharePoint migration** with a two-operator topology, demonstrating chain-of-custody receipts, connector bypass prevention, and cross-tenant leak detection.

## Overview

The secure-data demo demonstrates:

- **Two-Operator topology**: source domain and destination domain each have their own Gateway (PDPoint) and Operator (L5 Actuator)
- **Chain of custody**: both operators sign receipts; neither implicitly trusts the other
- **8 secure data transfer doctrine rules** enforced on every migration operation
- **Connector bypass prevention**: direct rclone/scp/rsync invocation blocked without a GovernanceEnvelope
- **Cross-tenant leak detection**: envelopes targeting destinations not in the signed manifest are rejected
- **Migration manifest enforcement**: bulk transfers require a valid, admin-signed manifest_id

## Network Topology

- **net_src_internal (10.20.0.0/24)**: Source domain: source gateway, source operator, connectors (rclone, SharePoint), source storage
- **net_dst_internal (10.21.0.0/24)**: Destination domain: destination gateway, destination operator, destination storage
- **net_migration (10.22.0.0/24)**: Migration corridor: source operator writes via rclone, destination operator verifies arrival
- **net_untrusted (10.23.0.0/24)**: Bad actor (isolated, no path to source or destination storage)
- **net_mgmt (10.24.0.0/24)**: Observability (reads audit logs from both gateways)

## Port Mappings

| Service | Port |
|---------|------|
| Source Gateway HTTP | 8083 |
| Source Gateway HTTPS | 8446 |
| Source Console | https://localhost:8446/console/ |
| Destination Gateway HTTP | 8084 |
| Destination Gateway HTTPS | 8447 |
| Destination Console | https://localhost:8447/console/ |

## Doctrine Rules

The demo includes 8 secure data transfer doctrine rules covering:

- **Connector Enforcement**: Direct transfer without connector envelope, out-of-band exfiltration
- **Tenant Isolation**: Cross-tenant data leak, transfer to unregistered destination
- **Manifest Integrity**: Bulk transfer without signed manifest
- **Data Protection**: Destructive shell commands in transfer context, unencrypted transit
- **Access Control**: Privilege escalation during transfer, audit logging bypass

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- g8e binary built via `make build` (see below)

### Build the g8e binary

From the repository root:

```bash
make build
```

### Start the secure-data demo

```bash
cd demos/secure-data
docker compose up -d
```

### Check service status

```bash
docker compose ps
```

### View logs

```bash
# Source gateway logs
docker compose logs -f src-gateway

# Destination gateway logs
docker compose logs -f dst-gateway

# Source operator logs
docker compose logs -f src-operator

# Destination operator logs
docker compose logs -f dst-operator

# Observability (both gateways)
docker compose logs -f observability
```

## Demo Scenarios

### Scenario 1: Governed Migration with Chain-of-Custody Receipts

```bash
g8e demos run secure-data 1
```

**Proves**: A SharePoint migration moves data from source to destination only through the governed connector pipeline. Both operators emit signed receipts, forming a cryptographic chain of custody.

### Scenario 2: Connector Bypass Attempt Blocked

```bash
g8e demos run secure-data 2
```

**Proves**: Direct invocation of transfer tools (rclone, scp, robocopy) is blocked by doctrine when not wrapped in a GovernanceEnvelope. The connector_bypass_attempt rule (0.93 confidence) triggers at admission.

### Scenario 3: Cross-Tenant Leak Doctrine Triggered

```bash
g8e demos run secure-data 3
```

**Proves**: Envelopes targeting destinations not in the signed manifest (e.g., rogue-tenant.sharepoint.com) are rejected before execution. The cross_tenant_data_leak rule (0.88 confidence) triggers at admission.

### Run all scenarios

```bash
g8e demos run secure-data
```

## Architecture Notes

### Two-Operator Topology

This is the only demo with two separate gateway/operator pairs:
- **Source domain** (`src-gateway` + `src-operator`): initiates the migration, reads from source storage
- **Destination domain** (`dst-gateway` + `dst-operator`): verifies arrival, writes to destination storage

Both gateways run in default `doctrine` posture (L1 enforcement). The migration corridor (`net_migration`) is the only network shared between domains; source operator writes, destination operator verifies.

### Connectors

Connectors (rclone, SharePoint) hold enrolled mTLS identities issued by `src-gateway`. They submit `GovernanceEnvelope`s; `src-operator` executes the actual transfer tool. Direct tool invocation without an envelope is blocked by doctrine.

### Data Sovereignty

All audit data is committed locally on each domain:
- **Git-backed ledger**: Immutable execution history on each operator container
- **SQLite audit vault**: Queryable audit trail on each gateway and operator
- No data leaves the demo environment unless explicitly transmitted through the governed pipeline

## Stop and Clean Up

```bash
docker compose down
docker compose down -v  # also removes volumes
```

## License

Apache 2.0
