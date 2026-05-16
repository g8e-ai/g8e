# Security Policy

We want g8e to be the safest way to run AI on your machine. If you find a security hole, we'd love to hear about it so we can fix it fast.

## Reporting a Vulnerability

Please don't open a public issue for security bugs. It's better for everyone if we fix it before the world knows about it.

Shoot an email to **security@g8e.ai** with:
- What the problem is.
- How we can reproduce it.
- How bad you think it is.

We'll get back to you within 48 hours to start working on a fix together.

## What's In Scope?

Basically, anything that breaks our trust model:
- Bypassing the 3-Layer Governance (L1/L2/L3).
- Sneaking past mTLS or workload identity.
- Leaking secrets or sensitive audit data.
- Messing with the local audit vaults.

We focus on the Operator (Go), Engine (Python), and Dashboard (Node.js) components. 

*Note: Third-party LLM providers (like OpenAI/Anthropic) and upstream dependencies are out of our direct control, but we still want to know if they're causing issues in g8e.*

## Our Promise

- **Transparency**: We'll keep you updated while we fix the issue.
- **Credit**: We'll give you a shout-out in the release notes (unless you want to stay anonymous).
- **No legal action**: We won't take legal action against you if you're acting in good faith to help us.

## Security Architecture

If you want to dive deep into how we've built our "Zero-Trust" model, check out our [detailed security docs](docs/architecture/security.md). It covers our L1/L2/L3 gates, mTLS setup, and data sovereignty in much more detail.
