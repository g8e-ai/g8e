# g8e v4.0.0 — The Portable AI Ops Platform

The most significant release since launch. g8e is now 100% self-hosted with no external service dependencies. The platform has been entirely rebuilt around the 4MB Operator as the backend data plane. This release introduces the unified CLI, concurrent SSH streaming, and the complete admin console for fleet management.

## 🚀 Major Features

### Platform & Infrastructure
- **4MB Operator as Backend** - The Operator now serves as the backend data plane for the entire platform, handling SQLite persistence, KV caching, and WebSocket pub/sub.
- **Unified CLI (`./g8e`)** - Single entry point for all platform operations. Only Docker is required on the host.
- **g8e-pod Execution Sandbox** - Isolated container for all toolchain operations (builds, tests, security scans).
- **Admin Console** - Complete administrative interface (`/console`) with real-time platform metrics and component health monitoring.
- **Full Documentation in Repo** - All platform documentation now ships inside the repository under `docs/`.

### AI & Execution
- **Tribunal** - 2-of-3 small language model voting for command safety validation.
- **Operator SSH Streaming** - Concurrent, ephemeral deployment via Go-native SSH with zero footprint.
- **Full Context Mode** - Dynamic system prompts that incorporate past conversations and user communication style.
- **LLM Provider Agnostic** - Support for Gemini, Anthropic, OpenAI, and Ollama. Gemini 3.1+ recommended.

### Security & Governance
- **Local-First Audit Architecture (LFAA)** - Audit logs and command history stay locked in local vaults.
- **Sentinel Threat Detection** - Pre and post-execution hooks with 50+ threat detectors mapped to MITRE ATT&CK.
- **Zero-Trust Stealth** - Outbound-only connectivity over port 443 with zero listening ports.
- **Human-in-the-Loop** - Mandatory approval for all state-changing operations.
- **FIDO2 Passkey-only Auth** - Passkey-only authentication for the dashboard.

## 🚀 Quick Start

```bash
git clone https://github.com/g8e-ai/g8e-ai/g8e.git && cd g8e
./g8e platform build

# Then open https://localhost — the setup wizard guides you through the setup
```

## 🛡️ Security & Privacy

g8e is built on the belief that AI should never be fully autonomous. We assume the AI is hostile, assume the environment is hostile, and put human judgment between the AI and your infrastructure.

- **Data Sovereignty**: Raw output is stored in local encrypted vaults, never transmitted to the platform or cloud.
- **Sentinel Scrubbing**: Credentials and PII are scrubbed before the AI can even see them.
- **Zero Standing Privileges**: Operator launches with no access and requests permissions dynamically based on user intent.

---

**g8e** - AI-powered, human-driven infrastructure operations. Fully self-hosted. Air-gap capable. Security and privacy by design.

🌐 [Website](https://lateraluslabs.com) | 📖 [Docs](../index.md) | 📄 [License](LICENSE)
