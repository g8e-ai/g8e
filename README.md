# g8e

A zero-trust execution layer for AI agents and the infrastructure they touch.

g8e helps teams let AI systems propose actions without giving them direct control over real systems. The model can ask. The machine that owns the workload decides.

[![License](https://img.shields.io/badge/license-BSL%201.1-blue.svg)](LICENSE)
[![CI](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml)

This project is a work-in-progress, built by a small solo effort, and is looking for a good home: pilot users, operators, researchers, and contributors who want to push this idea into real deployments.

## What g8e does

g8e sits between an AI client and the systems it might affect. It turns actions into governed, signed, auditable transactions instead of letting an agent speak directly to the host.

The core idea is simple:

- AI systems can express intent
- a gateway screens and authorizes that intent
- a local operator verifies it on the target host
- execution happens under explicit policy and evidence capture

That means:

- no direct host takeover by an LLM
- typed actions instead of freeform shell access
- policy gates before mutation
- signed receipts and audit trails for what actually happened

## Why this exists

Most AI agent stacks collapse four separate jobs into one process:

- reasoning
- authorization
- execution
- audit

That is a dangerous mix.

g8e separates them:

- the model proposes
- the gateway governs
- the operator executes locally
- evidence is recorded at the execution boundary

This is especially useful when you want AI to assist with infrastructure, operations, compliance, tooling, or distributed systems without handing it unrestricted authority.

## The short version

Think of g8e as a control plane for AI-driven operations.

A client or agent submits a typed intent. The gateway validates policy, applies human or quorum-based approvals when needed, and sends the action through an outbound-only connection to a local operator. The operator independently verifies the request, executes only the approved action, and writes its own signed evidence.

This is not a sandbox. It is a governance boundary.

## Try it

You can try the project in a few ways.

### 1. Run the platform locally (recommended)

The default workflow is native on your machine with the repo's Makefile targets:

```bash
git clone --depth 1 https://github.com/g8e-ai/g8e.git
cd g8e
make up
```

This builds the binary and starts the Gateway on localhost.

This Gateway-only track needs Git, Go 1.26.6, Make, and Node.js 22+ for the
embedded explorer build. It has no Python, Ollama, or model SDK requirement.
For setup details, see the [Getting Started guide](docs/guides/getting_started.md#native-host-build).

For interactive setup with the Operator roles and the first-party ensemble, use:

```bash
make ensemble-env
make full-setup
```

The full platform adds four Operators and the Python Ensemble. It requires
Python 3.12+ (provisioned by `uv` as needed); `make ensemble-env` installs only the runtime dependencies needed
by `make full`, while `make dev-python` also installs Ensemble test and lint
dependencies. Both targets bootstrap `uv` when it is not already installed.

For unattended startup, set `G8E_OLLAMA_ENDPOINT` in `.env` and run `make full`. It also reuses `G8E_HOSTNAME`; see [native startup settings](docs/guides/getting_started.md#run-natively-on-localhost).

Then enroll the first owner and approve workloads:

```bash
./g8e auth enroll user -e localhost
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

On a headless machine, use `./g8e auth enroll user -e localhost --headless`. This creates a CLI-only mTLS identity; it does not register a passkey, install OS trust, or enable browser console sign-in.

If you want the Docker Compose stack instead, that's still supported, but it is not the primary path for local development or evaluation.

### 2. Explore the protocol and evidence model

The repo includes a protocol layer and example integrations with Python and Go.

```bash
pip install g8e==2.3.1
```

Go:

```bash
go get github.com/g8e-ai/g8e/v2@v2.3.1
```

### 3. Docker fallback

If you specifically want the containerized stack:

```bash
git clone --depth 1 https://github.com/g8e-ai/g8e.git
cd g8e
cp .env.example .env
make docker-up
```

or

```bash
docker compose up -d --build
```

Then enroll the first owner and approve workloads through the same CLI flow.

## What is in the repo

This repository includes the main platform and supporting pieces:

- `cmd/` — CLI entrypoints
- `internal/` — gateway, operator, storage, governance, and runtime services
- `protocol/` — protobuf contracts and canonical models
- `ensemble/` — Python agentic reasoning service
- `console/` — browser console frontend
- `eval/` — evaluation and compliance harnesses
- `docs/` — architecture, guides, and reference material

## What makes this different

g8e is designed for environments where you want AI assistance without uncontrolled execution.

It is especially relevant for:

- AI/infra integration teams
- security-sensitive deployments
- regulated or compliance-conscious systems
- operators who want auditability
- researchers exploring zero-trust AI execution patterns

It is not trying to be a generic chat app or a toy local agent framework. It is trying to be a real governance boundary around machine actions.

## Project status

This is a serious experimental platform, not a polished enterprise product yet.

The project is:

- actively under development
- built as a single-creator effort with a lot of design depth
- intended for real-world pilot and feedback cycles
- looking for maintainers, adopters, and practical deployment homes

The repo is already fairly rich in architecture and tooling, but it still benefits from:

- production-oriented review
- contributor onboarding
- more real-world deployment feedback
- broader integration testing
- design critique from security and infra practitioners

## Looking for a home

This project is looking for the right environment to land:

- a lab or research team wanting to experiment with governed AI operations
- a platform team exploring real-time control for autonomous tooling
- a security-minded group evaluating LLM execution boundaries
- a contributor or maintainer who wants to help shape the project into something usable beyond a demo

If this resonates, I would love to hear from people who want to:

- pilot it in a real environment
- contribute architecture or implementation changes
- help build documentation and examples
- review security and governance assumptions
- turn this into a maintained open project with a broader community

## How to help

If you want to contribute, the best ways to start are:

- run the project locally and report issues
- run an evaluation or deployment workflow and document what was confusing
- review the docs and architecture for gaps
- help build examples for real use cases
- propose better security or UX patterns
- help shape the roadmap around actual deployments

For local setup, test selection, and pull request guidelines, see
[Contributing](.github/CONTRIBUTING.md).

## Roadmap themes

Near-term priorities are likely to be around:

- easier onboarding and setup
- clearer deployment guides
- stronger operator and gateway ergonomics
- more production-safe defaults
- better example scenarios and community docs
- expanded evaluation and evidence workflows

## License

Business Source License 1.1. Converts to Apache 2.0 on 2030-08-18.

## More information

For deeper detail, start here:

- [docs/architecture/overview.md](docs/architecture/overview.md)
- [docs/guides/getting_started.md](docs/guides/getting_started.md)
- [docs/guides/unified_stack.md](docs/guides/unified_stack.md)
- [docs/architecture/agents.md](docs/architecture/agents.md)
- [docs/architecture/evals.md](docs/architecture/evals.md)
- [ensemble/README.md](ensemble/README.md)

If you are curious, try the stack, poke around the docs, and reach out with a concrete use case.
