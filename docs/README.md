---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

Last Updated: 2026-05-16
Version: v0.2.5

The documentation is organized by service and role, separating the mandatory protocol substrate from the optional application-layer components.

## 1. Protocol Substrate
The core contract and governance model that all components must follow.

- [**Protocol Substrate**](protocol/README.md) — The wire contract, transaction flow, and governance bedrock.
- [**Governance Hierarchy**](protocol/governance.md) — L1/L2/L3 validation model.
- [**Security Principles**](protocol/security.md) — Trustless execution and host sovereignty.

## 2. g8eo — Reference Operator
The primary substrate implementation in Go.

- [**g8eo Overview**](g8eo/README.md) — Lifecycle, modes, and capabilities.
- [**Operator Architecture**](g8eo/architecture.md) — Hub vs Satellite modes and verification pipeline.
- [**Storage & LFAA**](g8eo/storage.md) — Local-First Audit Architecture and Multi-Ledger (git).

## 3. Reference Applications
Optional components demonstrating the protocol in action.

- [**g8ee Engine**](g8ee/README.md) — Reference AI reasoning and Tribunal orchestration.

## 4. Developer Resources
Guides for setting up, testing, and contributing to the platform.

- [**Developer Guide**](developer/README.md) — Environment setup and development workflows.
- [**Testing Guide**](developer/testing.md) — CI/CD and validation standards.
- [**CLI Reference**](general/cli_help.md) — Help for the `./g8e` management tool.

## 5. General Reference
Broad architectural context and platform history.

- [**About g8e**](general/about.md) — Origins and governance philosophy.
- [**Position Paper**](general/position_paper.md) — Thesis on AI-powered, human-driven infrastructure.
- [**Glossary**](general/glossary.md) — Canonical platform terminology.

