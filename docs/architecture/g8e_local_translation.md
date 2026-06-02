# g8e.local Internal Translation Layer

## Overview

`g8e.local` is the canonical internal hostname for operator-to-operator communication in the g8e mesh. The gateway translates this alias to installation-specific peer identity and endpoint data, ensuring that users do not manage hostnames, IPs, or DNS records manually. As of v1.0.6, this layer provides the foundational PKI and identity binding for gateway-to-gateway federation.

## Design Goals

1. **Canonical stability**: `g8e.local` remains the stable mesh-facing name across all installations
2. **Hidden complexity**: Real host identity and addressing are resolved internally by the gateway
3. **Frictionless bootstrap**: Users never configure DNS or host-specific addressing
4. **Security**: Translation preserves mTLS identity binding and SPIFFE URI SAN validation

## Translation Layer Components

### 1. Canonical Alias

- **Alias**: `g8e.local`
- **Scope**: Internal mesh communication only
- **Visibility**: Never exposed to end users; used internally for routing and identity resolution

### 2. Gateway Identity Mapping

The gateway maintains a mapping from the canonical alias to installation-specific identity:

```
g8e.local -> spiffe://g8e.local/gateway/<gateway_id>
```

Where `<gateway_id>` is a persistent identifier generated at gateway installation time and stored in the data directory. The identity is defined in `../../protocol/workload_identity.go`.

### 3. Peer Endpoint Resolution

When a gateway needs to communicate with a peer, it utilizes the `PeerConnectionManager` defined in `../../internal/services/gateway/peer_connection.go` to perform resolution:

```
g8e.local -> {
  gateway_id: "gw-abc123-...",
  endpoints: ["10.0.1.5:8440", "192.168.1.100:8440"],
  certificate: <gateway peer leaf cert>,
  last_seen: <timestamp>
}
```

The endpoint set is discovered via:
- Initial federation seed configuration (via `FederationSeedURL`)
- Local network identity detection for standalone deployments as implemented in `../../internal/services/gateway/gateway_service.go`

### 4. Certificate SAN Binding

Gateway peer certificates include the canonical alias in their SPIFFE URI SAN, managed by `PKIAuthority` in `../../internal/services/gateway/gateway_certs.go`:

```
URI SAN: spiffe://g8e.local/gateway/<gateway_id>
```

This ensures:
- Identity is consistent across the mesh
- mTLS validation verifies the canonical namespace
- Certificate revocation operates on the canonical identity rather than host-specific names

## Routing Flow

### Local Operator Resolution

1. Envelope arrives at the local gateway
2. Gateway identifies the target Operator via the internal pub/sub router in `../../internal/services/gateway/gateway_pubsub.go`
3. If the Operator is local, the gateway delivers the envelope via in-process dispatch
4. No alias translation is required for local delivery

### Federation Foundations

The v1.0.6 release provides the PKI and identity foundations for remote resolution:
1. Gateway peer identity is established via `gateway-peer` intermediate CA
2. `PeerConnectionManager` maintains outbound-only connections to a federation seed
3. Envelopes are re-verified by the receiving gateway using the logic in `../../internal/services/gateway/governance_envelope.go`

## Implementation Notes

### Gateway ID Generation

- Generated once at gateway installation by `../../internal/services/gateway/peer_connection.go`
- Persisted in the `gateway-id` file within the gateway data directory
- Format: `gw-<hex>-<hex>-<hex>-<hex>` (16 bytes of entropy)

### Fallback Behavior

- If no federation seed is configured, `g8e.local` utilizes localhost for service discovery
- Standalone gateway behavior is preserved through this fallback
- Federation remains opt-in via seed configuration

### Security Invariants

1. **Identity binding**: All peer connections enforce mTLS with SPIFFE URI SAN validation
2. **Canonical namespace**: Certificates utilize `spiffe://g8e.local/...` regardless of the host environment
3. **No DNS dependency**: Translation is internal to the gateway service; no external DNS is required
4. **Re-verification**: Every gateway re-verifies envelopes on receipt as mandated by the governance pipeline in `../../internal/services/governance/processor.go`

## References

- Federation plan: `../../.local.dev/docs/plans/gateway-federation-option-a-plan.md`
- Gateway PKI: `../../internal/services/gateway/gateway_certs.go`
- Workload identity: `../../protocol/workload_identity.go`
- Port constants: `../../internal/constants/ports.go`
