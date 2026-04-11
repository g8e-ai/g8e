# g8e v4.1.0 — Execution & Intelligence Refinement

Focused on improving AI interaction reliability, execution tracing, and g8eo listen mode testability. This release standardizes how tool calls and execution IDs are handled across the platform to ensure a consistent audit trail and more reliable model interactions.

## 🚀 Major Features

### AI & Execution
- **Gemini Streaming & Multi-turn** - Fixed function call streaming and state management for complex multi-turn conversations.
- **Tool Call & Declaration Cleanup** - Standardized tool definitions across g8ee for more reliable and predictable model interactions.
- **Execution ID Tracing** - Implemented consistent `execution_id` generation and propagation across all components for a complete audit trail.
- **Strict Payload Typing** - New model definitions for execution results and command payloads to prevent runtime type mismatches.

### Component Improvements
- **g8eo (Virtual Security Agent)** - Enhanced listen mode testability and hardened internal auth token handling.
- **g8ee (Virtual Execution Engine)** - Optimized DB client token loading and synchronized settings definitions.
- **g8ed (Virtual Security Operations Dashboard)** - Improved diagram generation for infrastructure visualization and aligned internal API endpoints.

### CI/CD & DX
- **GitHub Actions Integration** - New automated workflow for pull requests to ensure code quality and test coverage.
- **Local-CI Parity** - Simplified CI environment to match local developer testing workflows exactly.
- **Testing Toolchain** - Standardized `setup-llm.sh` and improved coverage reporting for g8eo.

## 🚀 Quick Start

```bash
git clone https://github.com/g8e-ai/g8e-ai/g8e.git && cd g8e
./g8e platform build

# Then open https://localhost — the setup wizard guides you through the setup
```

## 🛡️ Security & Privacy

g8e v4.1.0 continues our commitment to local-first, human-in-the-loop operations. The new execution tracing ensures that every action taken by the AI is uniquely identifiable and tied back to a specific user session and approval.

- **Execution Tracing**: Every command now carries a unique `execution_id` from inception to completion.
- **Type Safety**: Model-driven boundaries prevent injection or malformed data from reaching the execution plane.
- **Testable Security**: g8eo listen mode improvements allow for more rigorous automated security regression testing.

---

**g8e** - AI-powered, human-driven infrastructure operations. Fully self-hosted. Air-gap capable. Security and privacy by design.

🌐 [Website](https://lateraluslabs.com) | 📖 [Docs](../index.md) | 📄 [License](LICENSE)
