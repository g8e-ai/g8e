# Contributing to g8e

Thank you for your interest in contributing to g8e. This guide covers the process for submitting changes, setting up your environment, and our engineering standards.

## About the Codebase

A significant portion of this codebase was written with AI assistance. If you have been around long enough to know what that means in practice, you already know: it is a never-ending series of corrections. There is a non-zero chance that what you find here is not just overengineered or oversimplified—it may be a genuine bug or hallucinated logic.

PRs that clean up smelly code, fix real bugs, or remove tech debt are explicitly welcome. You do not need a feature gap to contribute. If you see something broken, wrong, or poorly abstracted, fixing it is a highly valued contribution.

## Getting Started

1. Fork the repository
2. Clone your fork and set up the development environment:

```bash
# HTTPS
git clone https://github.com/<your-username>/g8e.git && cd g8e

# SSH
git clone git@github.com:<your-username>/g8e.git && cd g8e

./g8e platform setup
```

3. Create a feature branch from `main`:

```bash
git checkout -b feature/your-feature-name
```

## Development Environment

g8e runs as a set of Docker containers with source code volume-mounted for hot reload. Only Docker is required on the host—all toolchain operations (Python, Go, Node) run inside the `g8ep` runner container.

```bash
./g8e platform start        # Start without rebuilding
./g8e platform rebuild      # Rebuild all services + restart
./g8e platform stop         # Stop all containers (data preserved)
./g8e platform wipe         # Wipe data volumes and restart fresh
```

**Note:** You can edit source files directly on your host machine and changes are reflected inside the containers without rebuilding. Rebuild the platform *only* when modifying `package.json`, `requirements.txt`, `go.mod`, or Dockerfiles.

## Running Tests

All tests execute inside the `g8ep` container to ensure environment parity.

```bash
./g8e test           # Run all test suites
./g8e test g8ee      # AI engine (Python/pytest)
./g8e test g8ed      # Dashboard (Node/Vitest)
./g8e test g8eo      # Operator (Go)
```

All tests must pass before submitting a Pull Request.

## Code Style

- **Python (`g8ee`):** Follow existing patterns. Type hints are mandatory. Use Pydantic models for data structures.
- **Node.js (`g8ed`):** Follow existing Express patterns. Use JSDoc for complex logic.
- **Go (`g8eo` / Operator):** Run `gofmt`. Avoid global state. Follow existing package boundaries.
- **Shell scripts:** Use `set -euo pipefail`. Must be `shellcheck` clean.

## Submitting Changes

1. Ensure all tests pass.
2. Write clear, concise commit messages.
3. Open a pull request against the `main` branch.
4. Fill out the PR template completely.
5. Wait for review. Maintainers will typically respond within a few business days.

### Commit Messages

Use clear, descriptive commit messages prefixing the component name:

```text
g8ee: fix null pointer exception in ReAct loop

Ensure that tool_call_id is correctly propagated when parsing the LLM response.
Fixes #123.
```

Valid prefixes: `g8ee:`, `g8ed:`, `g8eo:`, `g8es:`, `docs:`, `ci:`, `scripts:`.

### PR Guidelines

- Keep PRs strictly focused. One logical change per PR.
- Include tests for new functionality and bug fixes.
- Update documentation if behavior changes.
- Do not include unrelated formatting or refactoring changes (open a separate PR for those).

## Reporting Issues

Join the [Discord](https://discord.gg/g8e) to ask questions, discuss ideas, or get help before opening an issue.

Use [GitHub Issues](https://github.com/g8e-ai/g8e/issues) with the provided templates:

- **Bug reports:** Include reproduction steps, expected vs actual behavior, and environment details.
- **Feature requests:** Describe the use case and proposed architectural solution.

## Security Vulnerabilities

**Do not open a public issue for security vulnerabilities.** See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## Contributor License Agreement

By submitting a pull request or otherwise contributing code, documentation, or other materials to this repository, you agree to the following terms:

1. **Grant of rights** — You grant Lateralus Labs, LLC a perpetual, worldwide, irrevocable, royalty-free, non-exclusive license to use, reproduce, modify, distribute, sublicense, and otherwise exploit your contribution for any purpose, including in commercial products and services.
2. **You own your contribution** — You represent that you are legally entitled to grant the above license. If your employer has rights to intellectual property you create, you represent that you have received permission to contribute on their behalf, or that your employer has waived such rights for your contributions to this project.
3. **No obligation** — Lateralus Labs, LLC is under no obligation to accept, include, or maintain any contribution.
4. **License of your contribution** — Your contribution will be licensed to the public under the [Apache License, Version 2.0](LICENSE), the same license that governs this project.

This CLA does not transfer copyright ownership; you retain copyright in your contribution. It only grants Lateralus Labs, LLC the rights needed to build, maintain, and commercially license the g8e platform.
