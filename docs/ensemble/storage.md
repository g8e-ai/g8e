# Storage

## Overview

g8e enforces strict data sovereignty: raw command output and file contents stay on the Operator host, encrypted, and never persist platform-side. Platform state is host-native under `.g8e/`.

## Storage Tiers

- **Platform State** — Host-native under `.g8e/` on the Operator. Contains configuration, session data, and local state.
- **Blob Storage** — Used for large payloads, artifacts, and documents. Accessed via the Blob service client.
- **Document Store** — Gateway-side document store accessible via the HTTPS surface (port 8443).
- **Key-Value Store** — Operator KV service for fast, ephemeral state.

## Data Sovereignty Principles

- Raw command output and file contents never leave the Operator host
- Data is encrypted at rest on the Operator
- Platform-side storage contains only metadata, envelopes, and consensus artifacts
- Operators maintain full control of their data

## Blob Service

The Blob service client provides:

- Upload/download of encrypted blobs
- Pub/Sub notifications for blob events
- Integration with the Gateway document store

## Related

- [PKI & Trust](pki.md) — Encryption keys for data at rest
- [Protocol](protocol.md) — Document store API surface
- [Architecture](architecture.md) — Service client architecture
