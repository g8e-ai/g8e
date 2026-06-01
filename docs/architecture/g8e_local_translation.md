# g8e.local Internal Translation Layer

## Overview

`g8e.local` is the canonical internal hostname for operator-to-operator communication in the g8e mesh. The gateway translates this alias to installation-specific peer identity and endpoint data behind the scenes, so users do not need to manage hostnames, IPs, or DNS records manually.

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
g8e.local → spiffe://g8e.local/gateway/<gateway_id>
```

Where `<gateway_id>` is a persistent identifier generated at gateway installation time.

### 3. Peer Endpoint Resolution

When a gateway needs to communicate with a peer, it performs internal resolution:

```
g8e.local → {
  gateway_id: "abc123",
  endpoints: ["10.0.1.5:8443", "192.168.1.100:8443"],
  certificate: <gateway peer leaf cert>,
  last_seen: <timestamp>
}
```

The endpoint set is discovered via:
- Initial seed gateway configuration (if provided)
- Peer exchange during mesh membership synchronization
- Local network identity detection (for single-gateway deployments)

### 4. Certificate SAN Binding

Gateway peer certificates include the canonical alias in their SPIFFE URI SAN:

```
URI SAN: spiffe://g8e.local/gateway/<gateway_id>
```

This ensures:
- Identity is consistent across the mesh
- mTLS validation can verify the canonical namespace
- Certificate revocation operates on the canonical identity, not host-specific names

## Routing Flow

### Local Operator Resolution

1. Envelope arrives at local gateway
2. Gateway checks locality registry for target operator
3. If operator is local, deliver via in-process pub/sub
4. No alias translation needed for local delivery

### Remote Operator Resolution

1. Envelope arrives at local gateway
2. Gateway checks locality registry for target operator
3. If operator is remote, resolve owning gateway via `g8e.local` translation
4. Forward envelope to peer gateway using resolved endpoint and peer certificate
5. Receiving gateway re-verifies envelope on arrival before execution

## Implementation Notes

### Gateway ID Generation

- Generated once at gateway installation
- Persisted in gateway data directory
- Never regenerated unless gateway is reinstalled
- Format: UUID or cryptographically random string

### Translation Cache

- Gateway caches resolved peer endpoints
- Cache entries have TTL based on peer heartbeat
- Stale entries trigger re-resolution via peer exchange

### Fallback Behavior

- If no seed gateway is configured, `g8e.local` resolves to localhost
- This preserves standalone gateway behavior
- Federation is opt-in via seed configuration

### Security Invariants

1. **Identity binding**: All peer connections use mTLS with SPIFFE URI SAN validation
2. **Canonical namespace**: Certificates always use `spiffe://g8e.local/...` regardless of host
3. **No DNS dependency**: Translation is internal; no external DNS required
4. **Re-verification**: Every gateway re-verifies envelopes on receipt, regardless of forwarding path

## Future Extensions

### Multi-Mesh Support

If multi-mesh separation is needed, the canonical alias could be extended:

```
g8e.local → spiffe://g8e.local/mesh/<mesh_id>/gateway/<gateway_id>
```

This would require:
- Mesh ID configuration at install time
- Per-mesh trust domains
- Cross-mesh routing policies

### Service Discovery

The translation layer could be extended to support service discovery:

```
g8e.local → {
  gateway_id: "abc123",
  services: {
    "operator-xyz": {
      endpoints: [...],
      certificate: <operator leaf cert>
    }
  }
}
```

This would enable direct operator-to-operator communication without gateway forwarding.

## References

- Federation plan: `.local.dev/docs/plans/gateway-federation-option-a-plan.md`
- Gateway PKI: `internal/services/gateway/gateway_certs.go`
- Workload identity: `protocol/workload_identity.go`
