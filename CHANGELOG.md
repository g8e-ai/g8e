# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] - 2026-04-11

### Added
- **Core Platform:** Open-source release of the `g8e` platform for AI-assisted infrastructure operations.
- **g8ee (AI Engine):** ReAct-based Python orchestration layer with support for Anthropic, OpenAI, and local Ollama models.
- **g8eo (Operator):** ~4MB dependency-free static Go binary for remote host execution. Features zero-inbound ports and outbound-only mTLS.
- **g8es (Data Store):** SQLite-backed persistence layer, KV store, and pub/sub broker running within the Operator framework.
- **g8ed (Dashboard):** Node.js central management console featuring FIDO2 WebAuthn (passkey) authentication and real-time mTLS gateway proxying.
- **Security:** "Tribunal Refinement Pipeline" utilizing stochastic swarm voting to validate AI-proposed terminal commands before human review.
- **Security:** Local execution vaulting to ensure raw stdout/stderr logs are securely encrypted and retained strictly on the target host.
- **DevOps:** Comprehensive `g8e` CLI wrapper for platform setup, testing, operator deployment, and CA certificate management.
