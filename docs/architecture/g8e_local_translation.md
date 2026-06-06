# g8e.local Internal Translation Layer

## Overview

`g8e.local` is the canonical internal hostname for operator-to-operator communication in the g8e mesh. The gateway translates this alias to installation-specific peer identity and endpoint data, ensuring that users do not manage hostnames, IPs, or DNS records manually.

## Design Goals

1. **Canonical stability**: `g8e.local` remains the stable mesh-facing name across all installations
2. **Hidden complexity**: Real host identity and addressing are resolved internally by the gateway
3. **Frictionless bootstrap**: Users never configure DNS or host-specific addressing
4. **Security**: Translation preserves mTLS identity binding and SPIFFE URI SAN validation

## Translation Layer Components

### Canonical Alias

- **Alias**: `g8e.local`
- **Scope**: Internal mesh communication only
- **Visibility**: Never exposed to end users; used internally for routing and identity resolution

### Gateway Identity Mapping

The gateway maintains a mapping from the canonical alias to installation-specific identity:

```
g8e.local -> spiffe://g8e.local/gateway/<gateway_id>
```

Where `<gateway_id>` is a persistent identifier generated at gateway installation time.

### Peer Endpoint Resolution

When a gateway needs to communicate with a peer, it utilizes the `PeerConnectionManager` to perform resolution, mapping the canonical alias to specific endpoints, certificates, and metadata.

### Certificate SAN Binding

Gateway peer certificates include the canonical alias in their SPIFFE URI SAN, ensuring identity consistency across the mesh and enabling certificate revocation to operate on canonical identities rather than host-specific names.

## Routing Flow

### Local Operator Resolution

1. Envelope arrives at the local gateway
2. Gateway identifies the target Operator via the internal pub/sub router
3. If the Operator is local, the gateway delivers the envelope via in-process dispatch
4. No alias translation is required for local delivery

### Federation Foundations

The v1.0.6 release provides the PKI and identity foundations for remote resolution:
1. Gateway peer identity is established via `gateway-peer` intermediate CA
2. `PeerConnectionManager` maintains outbound-only connections to a federation seed
3. Envelopes are re-verified by the receiving gateway

## Detailed Information

For detailed information on:

- Complete PKI hierarchy and certificate management
- SPIFFE workload identity formats
- mTLS enforcement and revocation mechanisms
- Port topology and communication patterns
- Gateway ID generation and fallback behavior
- Security invariants and implementation details

See [Network Architecture](./network.md).

## References

- Federation plan: `../../.local.dev/docs/plans/gateway-federation-option-a-plan.md`
- Gateway PKI: `../../internal/services/gateway/gateway_certs.go`
- Workload identity: `../../protocol/workload_identity.go`
