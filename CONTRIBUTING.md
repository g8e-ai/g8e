# Contributing to g8e

Thank you for your interest in contributing to g8e. This guide covers the process for submitting changes.

## About the Codebase

A significant portion of this codebase was written with AI assistance. If you have been around long enough to know what that means in practice, you already know: it is a neverending series of corrections. There is a non-zero chance that what you find here is not just overengineered, oversimplified, or following an absolutely terrible approach — it may be a genuine bug.

PRs that clean up smelly code, fix real bugs, or remove tech debt are explicitly welcome. You do not need a feature gap to contribute — if you see something broken, wrong, or just bad, fixing it is a valued and appreciated contribution.

## Getting Started

1. Fork the repository
2. Clone your fork and set up the development environment:

```bash
# HTTPS
git clone https://github.com/<your-username>/g8e-ai/g8e.git && cd g8e

# SSH
git clone git@github.com:<your-username>/g8e-ai/g8e.git && cd g8e

./g8e platform setup
```

3. Create a feature branch from `main`:

```bash
git checkout -b feature/your-feature-name
```

## Development Environment

g8e runs as a set of Docker containers with source code volume-mounted for hot reload. Only Docker is required on the host — all toolchain operations run inside g8e-pod.

```bash
./g8e platform rebuild      # Rebuild all services + restart
./g8e platform start        # Start without rebuilding
./g8e platform stop         # Stop all containers (data preserved)
./g8e platform wipe         # Wipe data volumes and restart fresh
```

Edit source files directly -- changes are reflected without rebuilding. Rebuild only when modifying `package.json`, `requirements.txt`, `go.mod`, or Dockerfiles.

## Running Tests

```bash
./g8e test           # All components
./g8e test vse       # AI engine (Python/pytest)
./g8e test vsod      # Dashboard (Node/Vitest)
./g8e test vsa       # Operator (Go)
```

All tests must pass before submitting a PR.

## Code Style

- **Python (VSE):** Follow existing patterns. Type hints required. Use Pydantic models for data structures.
- **Node.js (VSOD):** Follow existing Express patterns. Use JSDoc where helpful.
- **Go (VSA/Operator):** Standard `gofmt`. Follow existing package structure.
- **Shell scripts:** `set -euo pipefail`. ShellCheck clean.

## Submitting Changes

1. Ensure all tests pass
2. Write clear, concise commit messages
3. Open a pull request against `main`
4. Fill out the PR template completely
5. Wait for review -- maintainers will respond within a few business days

### Commit Messages

Use clear, descriptive commit messages:

```
component: short description of change

Longer explanation if needed. Reference issues with #123.
```

Prefix with the component name: `vse:`, `vsod:`, `vsa:`, `vsodb:`, `docs:`, `ci:`, `scripts:`.

### PR Guidelines

- Keep PRs focused -- one logical change per PR
- Include tests for new functionality and bug fixes
- Update documentation if behavior changes
- Do not include unrelated formatting or refactoring changes

## Reporting Issues

Join the [Discord](https://discord.gg/g8e) to ask questions, discuss ideas, or get help before opening an issue.

Use [GitHub Issues](https://github.com/g8e-ai/g8e/issues) with the provided templates:

- **Bug reports** -- include reproduction steps, expected vs actual behavior, and environment details
- **Feature requests** -- describe the use case and proposed solution

## Security Vulnerabilities

**Do not open a public issue for security vulnerabilities.** See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.

## Contributor License Agreement

By submitting a pull request or otherwise contributing code, documentation, or other materials to this repository, you agree to the following terms:

1. **Grant of rights** — You grant Lateralus Labs, LLC a perpetual, worldwide, irrevocable, royalty-free, non-exclusive license to use, reproduce, modify, distribute, sublicense, and otherwise exploit your contribution for any purpose, including in commercial products and services.

2. **You own your contribution** — You represent that you are legally entitled to grant the above license. If your employer has rights to intellectual property you create, you represent that you have received permission to contribute on their behalf, or that your employer has waived such rights for your contributions to this project.

3. **No obligation** — Lateralus Labs, LLC is under no obligation to accept, include, or maintain any contribution.

4. **License of your contribution** — Your contribution will be licensed to the public under the [Apache License, Version 2.0](LICENSE), the same license that governs this project.

This CLA does not transfer copyright ownership — you retain copyright in your contribution. It only grants Lateralus Labs, LLC the rights needed to build, maintain, and commercially license the g8e platform.
