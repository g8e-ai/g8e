# LLM Providers

## Overview

g8ee supports multiple LLM providers through a factory pattern. Each provider implements a common interface for generation, thinking/reasoning, and function calling.

## Supported Providers

- **Gemini** — Native thinking tokens, thought signatures, function calling, code generation
- **Additional providers** — Pluggable via the provider factory

## Provider Factory

Providers are instantiated via a factory that selects the appropriate implementation based on configuration. The factory handles:

- Provider selection and initialization
- API key and credential management
- Fallback and retry logic
- Provider-specific feature detection (e.g., thinking support)

## Common Interface

All providers implement:

- **Text generation** — Standard completion/chat API
- **Function calling** — Structured tool use with typed responses
- **Thinking/reasoning** — Provider-native reasoning where available
- **Streaming** — Streaming responses for real-time output

## Provider-Specific Features

### Gemini

- Native thinking tokens with cryptographic thought signatures
- Code generation with structured output
- Function calling with parallel tool use
- Prompt engineering guidelines for optimal results

## Related

- [Thinking](thinking.md) — How provider reasoning feeds into L2 consensus
- [Prompts](prompts.md) — Prompt architecture and provider-specific handling
- [Constants](constants.md) — Provider configuration constants
