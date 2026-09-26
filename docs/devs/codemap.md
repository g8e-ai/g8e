---
doc_id: codemap
title: g8e Code Map
audience: maintainers and coding agents
status: current
last_updated: 2026-09-26
version: v2.2.0
owners:
  - cmd/g8e/main.go
  - internal/cli/cmd/main.go
  - internal/cli/cmd/test/test.go
  - internal/cli/cmd/test/public_loop.go
  - internal/cli/cmd/eval/eval.go
  - internal/cli/cmd/eval/public_restore_cmd.go
  - internal/cli/cmd/public/public.go
  - internal/services/gateway/gateway_service.go
  - internal/services/gateway/gateway_http.go
  - internal/services/gateway/gateway_http_router.go
  - internal/services/gateway/gateway_auth.go
  - internal/services/gateway/gateway_db.go
  - internal/services/gateway/embedded/operator.go
  - internal/services/g8eo.go
  - internal/services/pubsub/mode_deps.go
  - internal/services/fs/file_service.go
  - internal/services/mcp/native_tool_registry.go
  - internal/constants/paths.go
  - docker-compose.yml
  - buf.gen.yaml
  - protocol/node/buf.gen.yaml
  - Makefile
related:
  - docs/devs/devs.md
  - docs/devs/tests.md
  - docs/devs/docs.md
  - docs/devs/release_process.md
  - docs/devs/troubleshooting.md
  - docs/architecture/overview.md
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
  - docs/architecture/governance.md
when_to_read: Finding which package owns a runtime mode, CLI group, protocol generator, persistence store, Compose profile, or test entry point.
do_not_use_for:
  - Coding invariants (docs/devs/devs.md)
  - Test selection, fixtures, race, coverage, and CI (docs/devs/tests.md)
  - Documentation audit, catalog, and generation policy (docs/devs/docs.md)
  - Release, native evaluation acceptance, and signed evidence (docs/devs/release_process.md)
  - Deployment and enrollment procedures (docs/guides/getting_started.md)
---

# g8e Code Map

## Purpose

Package and runtime ownership for the g8e repository. The current tree is the source of truth. Command names and flags come from `./g8e <command> --help`. This file is a map. Coding rules, test essays, documentation audit, and release procedure stay in their owners.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Ownership](#ownership-inv-map), [Mode separation](#mode-separation-inv-mode), [Package placement](#package-placement-inv-pkg), [Protocol generators](#protocol-generators-inv-proto), [Test commands](#test-commands-inv-testmap).

Maps: [CLI packages](#cli-packages), [Runtime modes](#runtime-modes).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Ownership (`INV-MAP`)

| ID | Rule |
| --- | --- |
| INV-MAP-01 | Package and runtime ownership MUST be maintained in this file. Coding invariants MUST stay in [Developer Guidelines](devs.md). Test selection, fixtures, race, coverage, and CI MUST stay in [Testing](tests.md). Documentation audit, catalog, and generation policy MUST stay in [Documentation Guide](docs.md). Release procedure MUST stay in [Release Process](release_process.md). This file MUST NOT paste those essays. |
| INV-MAP-02 | A mapped owner MUST be a repo-relative path that exists in the tree. A claim the tree no longer supports MUST be removed in the same audit. CLI flags MUST be read from `./g8e <command> --help` (INV-ENV-03). |

### Mode separation (`INV-MODE`)

| ID | Rule |
| --- | --- |
| INV-MODE-01 | Gateway HTTP and control-plane ownership MUST stay on `gateway.GatewayModeService` in `internal/services/gateway/`. The in-process Operator substrate MUST stay on `embedded.Service` in `internal/services/gateway/embedded/`. The outbound Operator runtime MUST stay on `services.G8eoService` in `internal/services/g8eo.go`, started from `internal/cli/serve/operator.go`. These three owners MUST NOT be described as one runtime. Construction constraints on `G8eoService` are INV-BOUND-04. |
| INV-MODE-02 | `embedded.Service` MUST own pending registration (`RegisterPending`), claim (`Claim`), deterministic document id `constants.DocIDEmbeddedOperator` (`embedded-operator`), same-user idempotence, and `constants.ErrEmbeddedOperatorClaimed`. HTTP routing for that substrate MUST stay in `internal/services/gateway/operator_controller.go`. Web-session bind MUST stay on `RegistrationService`. |

### Package placement (`INV-PKG`)

| ID | Rule |
| --- | --- |
| INV-PKG-01 | A Cobra group package name MUST match its directory. When that name would collide with an existing import, the package MUST be the directory plus `cmd`: `authcmd`, `operatorcmd`, `vaultcmd`, `testcmd`, `tuicmd`, `compliancecmd`. Where a new group lives is INV-CLI-01. |
| INV-PKG-02 | `g8e test public-loop` MUST stay in `internal/cli/cmd/test/public_loop.go` (`package testcmd`). It MUST NOT move into the `public` spectator group. `public restore` MUST be constructed by `eval.PublicRestoreCmdWithConfig` in `internal/cli/cmd/eval/public_restore_cmd.go` and registered on the `public` command in `internal/cli/cmd/public/public.go`. |
| INV-PKG-03 | `internal/cli/cmd/shared/`, `internal/cli/cmd/gwremote/`, and `internal/cli/cmd/cmdtest/` MUST NOT be registered from `internal/cli/cmd/main.go`. They are not Cobra groups. |

### Protocol generators (`INV-PROTO`)

| ID | Rule |
| --- | --- |
| INV-PROTO-01 | Root `buf.gen.yaml` MUST be described as the Go and Markdown protobuf generator only (`make proto-go`, output beside `protocol/proto/g8e/` and under `protocol/docs/reference/api/`). Python stubs MUST be attributed to `protocol/python/scripts/generate_protos.py` (`make proto-python`, modules under `protocol/python/g8e/`). TypeScript stubs MUST be attributed to `protocol/node/buf.gen.yaml` (`make proto-node`, output under `protocol/node/src/gen/`). `make proto` runs Go, Python, TypeScript, and `proto-lockfiles`. MUST NOT attribute Python or TypeScript generation to the root `buf.gen.yaml`. Hand-editing generated output is INV-GEN-01. |

### Test commands (`INV-TESTMAP`)

| ID | Rule |
| --- | --- |
| INV-TESTMAP-01 | This file maps test commands to tiers and directories. Selection, fixtures, `testutil` path rules, race, coverage, timeouts, and CI MUST be taken from [Testing](tests.md). Platform suites still enter through `./g8e test ...` or the owning Makefile target (INV-TEST-01). |
| INV-TESTMAP-02 | `./g8e test e2e-full` MUST be described from `internal/cli/cmd/test/test.go`: it passes Compose profile name `bootstrapped` (`constants.DockerBootstrappedProfile`), adds `cross-enrollment` (`constants.DockerCrossEnrollProfile`) when `--cross-enrollment` is set, then runs the same `go test` arguments as `./g8e test e2e`. `docker-compose.yml` has no `bootstrapped` profile and no `evaluation` profile. Named `profiles:` keys are `cross-enrollment` and `g8ellama`. Unprofiled services start on `docker compose up -d`. Procedure detail is INV-TEST-12 and INV-TEST-13. `constants.DockerBootstrappedProfile` and `constants.DockerEvaluationProfile` still exist in `internal/constants/paths.go`. |

## Owned surfaces

### Repository

| Path | Owns |
| --- | --- |
| `cmd/g8e/` | Go binary entry. `main` passes build metadata into `serve.VersionInfo` and calls `cmd.ExecuteWithVersionInfo`. |
| `internal/cli/` | CLI commands, API client, authentication, configuration, frontend-origin checks, process management, foreground Gateway and Operator startup, SSE, streams, platform helpers, the TUI, and the onboarding wizard. |
| `internal/services/` | Gateway, outbound Operator, governance, transport, persistence, execution, evaluation, compliance, inference, public disclosure, and supporting services. |
| `internal/adapters/` | Optional external adapters. The current adapter is Anduril Lattice. |
| `internal/constants/` | Go constants for paths, errors, protocol identifiers, permissions, runtime behavior, and Docker profile name constants. |
| `internal/config/`, `internal/models/`, `internal/paths/` | Typed configuration, internal service models, and runtime paths derived from the project root. |
| `internal/governance/` | Envelope hash helpers over the canonical protobuf `GovernanceEnvelope`. L1 through L5 services live under `internal/services/governance/`. |
| `internal/ollama/` | Minimal HTTP client for Ollama model-maintenance endpoints. |
| `internal/infra/cloudflaredns/` | Cloudflare DNS client. |
| `internal/pkg/certutil/`, `internal/pkg/ssh/` | Certificate and SSH helpers. |
| `internal/buildinfo/`, `internal/exitcode/`, `internal/httpclient/`, `internal/jsonschema/`, `internal/marshaler/`, `internal/pathutil/`, `internal/response/`, `internal/security/`, `internal/testutil/`, `internal/timesvc/`, `internal/uuid/`, `internal/certs/` | Shared build metadata, exit handling, HTTP, schema validation, serialization, path utilities, responses, security helpers, test infrastructure, time, UUIDs, and certificates. |
| `protocol/` | Protobuf schemas, generated bindings, JSON registries, model schemas, conformance tests, vectors, examples, and protocol documentation. |
| `test/` | Cross-package integration tests, `test/fixtures/`, and `test/e2e/`. |
| `eval/` | Evaluation campaign inputs, `native-boundary-compose.yml`, and examples. The Go evaluator is `internal/services/evaluation/`, exposed by `internal/cli/cmd/eval/`. |
| `ensemble/` | Python g8ee application. Index: [Ensemble documentation](../ensemble/index.md). |
| `dashboard/` | Node.js g8ed SPA host. Index: [Dashboard documentation](../dashboard/index.md). |
| `demos/` | Healthcare, finance, DHS, and FedRAMP demo environments. Index: [Demo index](../../demos/README.md). |
| `docs/` | Architecture, guides, references, developer docs, and release notes. |
| `scripts/` | Validation, generation, release, and build support scripts. |
| `website/` | Static site generator and Cloudflare Worker packaging. Source overview is the root `README.md`. |
| `third_party/` | Vendored inputs generated into internal adapters. Lattice protobuf provenance is under `third_party/anduril/`. |
| `vendor/` | Third-party Go modules. Not a g8e product owner. |
| `Makefile`, `Dockerfile`, `docker-compose.yml` | Repository build, image, and Compose orchestration. |
| `ROADMAP.md` | Public roadmap for the line. Not a runtime owner. |

### Process entry

The executable starts in `cmd/g8e/main.go`. Command registration and process-level error handling live in `internal/cli/cmd/main.go`. `NewRootCmd` registers groups in this order: `gw`, `auth`, `mcp`, `operator`, `vault`, `test`, `demos`, `docker`, `audit`, `report`, `public`, `swagger`, `tui`, `version`, `compliance`, `eval`.

| Path | Owns |
| --- | --- |
| `internal/cli/serve/gateway.go` | Foreground Gateway startup: paths, `RuntimeFileService`, runtime tree, logging, configuration, `GatewayModeService`, shutdown. Opens `CanonicalDBService` itself only when consensus bootstrap runs, then calls `NewGatewayModeServiceWithDB`. |
| `internal/cli/serve/launch_profile.go` | Versioned `GatewayLaunchProfile` persisted after a successful managed `gw start`. `gw restart` reads it. |
| `internal/cli/serve/operator.go` | Foreground outbound Operator: configuration, credentials, platform enrollment, mTLS transport, `G8eoService`, certificate renewal, shutdown. |
| `internal/cli/serve/platform_enrollment_client.go` | Resumable owner-approved Operator enrollment client. |
| `internal/cli/serve/cert.go` | Operator certificate load, renewal, trust-bundle retrieval, and mTLS client construction. |
| `internal/cli/serve/version.go` | Build version metadata passed from `cmd/g8e`. |
| `internal/services/logging/` | Daemon log files and `slog` configuration. |

### CLI packages

Live group list: `./g8e --help`. Group placement: INV-CLI-01. Name exceptions: INV-PKG-01.

| Group | Package | Path | Alias | Owns |
| --- | --- | --- | --- | --- |
| `gw` | `gw` | `internal/cli/cmd/gw/` | `gateway` | Gateway lifecycle, data, security, and tunnel commands. |
| `auth` | `authcmd` | `internal/cli/cmd/auth/` | | User and platform enrollment, sessions, and approvals. |
| `mcp` | `mcp` | `internal/cli/cmd/mcp/` | | MCP stdio serving and agent integration. |
| `operator` | `operatorcmd` | `internal/cli/cmd/operator/` | `operators` | Operator discovery, startup, deploy, copy, and streams. |
| `vault` | `vaultcmd` | `internal/cli/cmd/vault/` | | Local vault init, unlock, rekey, status, reset, export, and import. |
| `test` | `testcmd` | `internal/cli/cmd/test/` | | Unit, integration, e2e, e2e-full, coverage, lint, chaos, summary, and `public-loop`. |
| `demos` | `demos` | `internal/cli/cmd/demos/` | `demo` | Demo environment and scenario lifecycle. |
| `docker` | `docker` | `internal/cli/cmd/docker/` | | Unified Compose stack lifecycle. |
| `audit` | `audit` | `internal/cli/cmd/audit/` | | Receipt, event, summary, export, and report queries against a running Gateway. |
| `report` | `report` | `internal/cli/cmd/report/` | | Deterministic CSV evidence generation and offline verification. |
| `public` | `public` | `internal/cli/cmd/public/` | | Public spectator feed. `restore` is constructed in the eval package (INV-PKG-02). |
| `swagger` | `swagger` | `internal/cli/cmd/swagger/` | | OpenAPI generation, serving, and validation. |
| `tui` | `tuicmd` | `internal/cli/cmd/tui/` | | Tactical Governance Console. |
| `version` | `version` | `internal/cli/cmd/version/` | | Build metadata. Optional FIPS module status is `./g8e version --help`. |
| `compliance` | `compliancecmd` | `internal/cli/cmd/compliance/` | | `ksi`, `ksi-history`, `overlay`, `demo-run`, `release-evidence`, `evidence`, `evidence-graph`, and `report`. |
| `eval` | `eval` | `internal/cli/cmd/eval/` | `evals` | `boundary`, `campaign`, `models`, `rollout`, `gate`, and `dev`. Also constructs `public restore`. |

Files at `internal/cli/cmd/` root are the root command, its tests, and shared file-service and config-load tests. Factory-error tests live beside the group they cover (`factory_error_<group>_test.go`). The factory rule is INV-FS-08.

| Path | Package | Owns |
| --- | --- | --- |
| `internal/cli/cmd/shared/` | `shared` | Config load, `NewFileSvc` (`fs.NewRuntimeFileService`), source-root lookup, command context, and version-info context. Exists so group packages do not import the root `cmd` package. |
| `internal/cli/cmd/gwremote/` | `gwremote` | Gateway HTTP publication, model provenance, provider observation, and health hook used by `eval`, `public`, and `docker`. Exists so those packages do not import `gw`. |
| `internal/cli/cmd/cmdtest/` | `cmdtest` | Cross-package test helpers. Not a Cobra group. |
| `internal/cli/api/` | | Typed CLI HTTP client. |
| `internal/cli/auth/` | | CLI enrollment, credential staging, key generation, passkey registration, trust bundles, and mTLS clients. |
| `internal/cli/browserorigin/` | | Frontend-origin validation and normalization. |
| `internal/cli/config/` | | CLI configuration resolution and endpoint overrides. |
| `internal/cli/frontendverify/` | | Frontend connection and running-Gateway verification. |
| `internal/cli/operator/` | | Operator discovery and management helpers. |
| `internal/cli/output/` | | Human-readable and JSON command output. |
| `internal/cli/platform/` | | Cross-platform process, browser, and system trust operations. |
| `internal/cli/serve/` | | Foreground Gateway and Operator runtimes. See [Process entry](#process-entry). |
| `internal/cli/sse/` | | CLI SSE client. |
| `internal/cli/stream/` | | Local and SSH streams for Operator management. |
| `internal/cli/tui/` | | Tactical Governance Console implementation. |
| `internal/cli/wizard/` | | Interactive Gateway setup flow. |

### Runtime modes

Mode dependency types live in `internal/services/pubsub/mode_deps.go`. Wiring rules are INV-DEP-01 through INV-DEP-06. Construction order for the Gateway is [Gateway governance construction](devs.md#gateway-governance-construction).

| Mode | Type | Path | Owns |
| --- | --- | --- | --- |
| Gateway HTTP and control plane | `gateway.GatewayModeService` | `internal/services/gateway/gateway_service.go` | Policy decision point: HTTP, PKI, persistence, MCP, pub/sub, and governance wiring. Builder assembles dependencies before start. |
| Embedded Operator substrate | `embedded.Service` | `internal/services/gateway/embedded/operator.go` | In-process operator document: `embedded.New`, `RegisterPending`, `Claim`. Not the outbound runtime. |
| Outbound Operator | `services.G8eoService` | `internal/services/g8eo.go` | Authenticated connection to the Gateway and approved work on the Operator host. Started from `internal/cli/serve/operator.go`. Calls `pubsub.NewOutboundModeDeps`. MUST NOT construct `mcp.GatewayService` (INV-BOUND-04). |

| Deps type | Adds beyond `GovernanceCoreDeps` |
| --- | --- |
| `pubsub.GovernanceCoreDeps` | Shared replay store, state-root provider, transaction audit, L3 notary, signer store, and L1 doctrine. |
| `pubsub.GatewayModeDeps` | Governed document store, consensus policy store, field reader, consensus service, platform enrollment, and posture. |
| `pubsub.OutboundModeDeps` | Nothing. The type embeds only the shared core. |

Outbound groups owned by `G8eoService`: `auth.BootstrapService` and mTLS pub/sub; `ExecutionService` and `FileEditService`; `CanonicalDBService` plus vault, execution vault, suspended transactions, replay, SQL audit, and optional Git ledger; local L1 doctrine, outbound L3 notary, L4, and L5; optional Lattice adapter on the same governed path.

Compose profile assignment is INV-TESTMAP-02. Service names are the Compose keys:

| Compose service | `profiles:` |
| --- | --- |
| `g8e-gateway`, `g8e-operator`, `g8e-inference-operator`, `ensemble`, `dashboard` | None. They start on `docker compose up -d`. |
| `g8e-gateway-secondary` | `cross-enrollment` |
| `g8e-gateway-user`, `g8e-inference` | `g8ellama` |

### Governance flow

Mutations that traverse a g8e ingress use a typed `GovernanceEnvelope` (INV-BOUND-01). Canonical envelope and proof messages live in `protocol/proto/g8e/common/v1/`. Posture and the five layers are specified in [Governance](../architecture/governance.md). Ingress limits are in [AI Agents and the g8e Governance Boundary](../architecture/agents.md).

| Step | Owner |
| --- | --- |
| L1 doctrine | `internal/services/governance/l1_doctrine.go` (`NewL1DoctrineFromDir`). Loader rules are INV-DOCTRINE-*. |
| L2 consensus | `internal/services/consensus/`. Required only for postures that require it (INV-DEP-05). Architecture: [Consensus](../architecture/consensus.md). |
| L3 notary | `internal/services/governance/`. Gateway mode and outbound mode use different notaries. See the mode table. |
| L4 verification and L5 actuation | `internal/services/governance/` with command and file dispatch in `internal/services/pubsub/`. |
| MCP and A2A translation, native tools, suspension | `internal/services/mcp/`. Native registration rules are INV-MCP-*. |
| Sensitive-value tokenization | `internal/services/scrubbing/`. |

### Gateway HTTP boundary

`gateway.HTTPHandler` in `internal/services/gateway/gateway_http.go` is the routing shell. `initHTTPHandler` in `gateway_service.go` builds two listeners:

| Field | Listener | Handler | Role |
| --- | --- | --- | --- |
| `server` | `Gateway.HTTPPort` | `buildHTTPRouter` | Plain HTTP: bootstrap health, state, bootstrap, CA bundle and fingerprint, CLI recovery request/status/complete, platform enrollment request/status/complete, `/.well-known/g8e/bin/`, deploy scripts. Other paths redirect to HTTPS. Path-traversal guard and rate limit wrap the mux. |
| `publicServer` | `Gateway.HTTPSPort` | `HTTPHandler` (`buildPublicRouter`) | Full API. TLS `ClientAuth` is `VerifyClientCertIfGiven`. `AuthService.Middleware` classifies routes. `RouteAuthRegistry.AuthMode` defaults unknown paths to `RouteAuthMTLS`. CORS and the path-traversal guard wrap the handler. |

Controller domains registered on the HTTPS router (`HTTPHandlerDependencies`):

| Domain | Controllers |
| --- | --- |
| Identity and enrollment | PKI, bootstrap, CLI recovery, CLI rotation, CLI refresh, CLI session, enrollment token, user, session, passkey, platform enrollment |
| Data and audit | Data, signer, audit |
| Operators and dispatch | Operator, dispatch, inference dispatch |
| Streams and ingress | SSE, MCP, pub/sub |
| Governance and admin | Governance, health, admin |
| Observation and publication | Observe, observe producer, public feed, eval campaign publication, provider observation, model provenance, ensemble browser proxy |

| Path | Owns |
| --- | --- |
| `internal/services/gateway/console/` | Embedded Console SPA. |
| `internal/services/gateway/scripts/` | Embedded deployment script templates. |
| `internal/services/gateway/docs/` | Generated OpenAPI (`swagger.json` embedded by `docs.go`, plus `swagger.yaml`). Generator: `make swagger-generate`. |
| `internal/services/gateway/explorer/` | Embedded evaluation-explorer assets. |
| `internal/services/gateway/db/schema.sql` | Canonical SQLite schema embedded by `CanonicalDBService`. |

Route and auth inventories stay in the router and `gateway_auth.go`. This map does not copy them.

### Persistence

`.g8e/` file I/O rules are INV-FS-*. This section names owners only.

| Owner | Path | Notes |
| --- | --- | --- |
| `RuntimeFileService` | `internal/services/fs/file_service.go` | Canonical `.g8e/` file service. Startup calls `CreateRuntimeTree` from `internal/cli/serve/gateway.go` and `internal/cli/serve/operator.go`. |
| Path constants | `internal/constants/paths.go` | Reusable system, repository, and runtime path strings, including Docker profile name constants. |
| `CanonicalDBService` | `internal/services/gateway/gateway_db.go` | Primary SQLite connection, schema lifecycle, encrypted vault, secret manager, maintenance, and store lifetimes. Constructor: `OpenCanonicalDBService`. The gateway builder and `G8eoService` both call it. |

`CanonicalDBService` accessors:

| Accessor | Store |
| --- | --- |
| `GetDocStore` | `DocumentStoreService` |
| `GetAppPolicyStore` | `AppPolicyStoreService` |
| `GetSignerStore` | `SignerStoreService` |
| `GetConsensusStore` | `ConsensusStoreService` |
| `GetStateRootSvc` | `StateRootService` |
| `GetReplayStore` | `ReplayStoreService` |
| `GetKVStore` | `KVStoreService` |
| `GetSSEStore` | `SSEEventService` |
| `GetBlobStore` | `BlobStoreService` |
| `GetAuditStore` | `storage.SQLAuditStore` |

`internal/services/storage/` additionally owns the execution vault, suspended transactions, standalone replay protection, Git ledger history, commitments, the token store, and operational snapshots. Storage architecture: [Storage](../architecture/storage.md).

### Internal services

| Path | Owns |
| --- | --- |
| `internal/services/auth/` | Operator bootstrap transport and host fingerprinting. |
| `internal/services/compliance/` | KSI evaluation and history, OSCAL support, catalog validation, evidence import and graph verification, and signed report bundles. |
| `internal/services/consensus/` | Consensus members, policy construction, deliberation, and Ed25519 voting. |
| `internal/services/evaluation/` | Native suite registry, governed command lane, target observer, model campaign controller, grading, evidence storage, and verification. |
| `internal/services/execution/` | Command execution and governed file edits. |
| `internal/services/fs/` | Scoped `.g8e/` runtime file operations. |
| `internal/services/g8ebinaries/` | Deployment-binary catalog, reader, and publisher. CLI wrapper: `internal/tools/g8ebinaries/`. |
| `internal/services/gateway/` | Gateway HTTP and control plane, including the child packages in the HTTP boundary table. |
| `internal/services/gateway/embedded/` | In-process Operator substrate (INV-MODE-02). |
| `internal/services/governance/` | L1, L3, L4, L5, governance interfaces, state-root providers, signer stores, and public-key export. |
| `internal/services/inference/` | Inference dispatch (`inference/dispatch/`), provider backends, and attempt storage. |
| `internal/services/inference/provider_observer/` | Provider-boundary GPU and RAM sampling. |
| `internal/services/inference/model_provenance/` | Storage-side model weight attestation. |
| `internal/services/keystore/` | Encrypted key storage for Gateway secrets and PKI. |
| `internal/services/logging/` | Runtime log file and `slog` configuration. |
| `internal/services/mcp/` | MCP and A2A gateway, native tools, field-path governance, suspension, and downstream clients. |
| `internal/services/network/` | Network identity detection and endpoint construction. |
| `internal/services/operatorcapability/` | Role selection for provider-boundary observer, provenance operator, inference, and governed data operators, plus Ollama command construction. |
| `internal/services/publicdisclosure/` | Validation of public-disclosure records. Gateway publisher and mirror orchestration stay in `internal/services/gateway/`. |
| `internal/services/pubsub/` | Gateway and Operator pub/sub clients, command dispatch, results, heartbeats, and mode dependency types. |
| `internal/services/reporting/` | Deterministic CSV evidence reports and verification. Exposed by `g8e report`. |
| `internal/services/scrubbing/` | Sensitive-value detection, tokenization, and rehydration. |
| `internal/services/sqliteutil/` | Shared SQLite configuration and connection helpers. |
| `internal/services/storage/` | Audit, execution, replay, suspension, commitment, and Git ledger persistence. |
| `internal/services/system/` | Host capability and embedded Git selection. |
| `internal/services/vault/` | Encryption vault lifecycle. |

Test doubles live in `internal/services/storage/storagetest/`, `internal/services/pubsub/pubsubtest/`, `internal/services/governance/governancetest/`, and `internal/services/keystore/keystoretest/`. `internal/tools/chaos` imports `storagetest` for `g8e test chaos`. Gateway and Operator service code does not import these packages.

### Protocol packages

Schemas live under `protocol/proto/g8e/`. Wire requirements: [Protocol Specification](../../protocol/docs/spec.md). Package ownership and generation: [Protocol README](../../protocol/README.md).

| Domain | Owns |
| --- | --- |
| `common/v1` | Governance envelopes, layer metadata, shared enums, and common messages. |
| `compliance/v1` | Compliance evidence, assessment, and report messages. |
| `eval/v1` | Native evaluation runs, attempts, observations, assertions, verdicts, metrics, reports, and deployment identities. |
| `operator/v1` | Operator commands, execution results, telemetry, receipts, and service RPC definitions. |
| `pubsub/v1` | Pub/sub event and message envelopes. |

| Output | Generator | Make target |
| --- | --- | --- |
| Go packages beside `protocol/proto/g8e/` | Root `buf.gen.yaml` (`protoc-gen-go`, `protoc-gen-go-grpc`) | `make proto-go` |
| Markdown under `protocol/docs/reference/api/` | Root `buf.gen.yaml` (`protoc-gen-doc`) | `make proto-go` |
| Python modules under `protocol/python/g8e/` | `protocol/python/scripts/generate_protos.py` | `make proto-python` |
| TypeScript under `protocol/node/src/gen/` | `protocol/node/buf.gen.yaml` | `make proto-node` |
| Ensemble `uv.lock` after the Python package changes | `make proto-lockfiles` | `make proto` runs all four |

| Path | Owns |
| --- | --- |
| `protocol/constants/` | External JSON registries. Go runtime constants in `internal/constants/` remain the implementation source for values the platform executes. When both exist, update both through the owner (INV-TYPE-06). |
| `protocol/models/`, `protocol/schemas/` | JSON model shapes and validation schemas. |
| `protocol/vectors/`, `protocol/test-fixtures/` | Cross-language canonicalization vectors and fixtures. |
| `protocol/conformance/` | Cross-language constants, model, and hash parity tests. |
| `protocol/python/` | Python protocol package. |
| `protocol/node/` | TypeScript protocol package. |
| `protocol/examples/` | Go examples and MCP client configuration templates. |
| `protocol/docs/` | Protocol specifications and generated API references. |
| `internal/tools/constgen/` | Regenerates event and action-type constants from `protocol/constants/events.json` and `protocol/constants/status.json`. `make constants` writes. `make constants-check` verifies. Outputs include `internal/constants/events_gen.go`, `internal/constants/action_types_gen.go`, `dashboard/public/js/constants/events.js`, and `protocol/python/g8e/_data/events.json`. |

### Adapters and native tools

| Path | Owns |
| --- | --- |
| `internal/adapters/lattice/` | Outbound Operator integration with Anduril Lattice over gRPC. Guide: [Lattice Adapter](../../internal/adapters/lattice/README.md). |
| `internal/services/mcp/native_tool_registry.go` | `RegisterNativeTools`. The inventory is this function (INV-MCP-03). Do not copy the tool list into a doc. |

### Compliance and reporting

| Surface | Path | CLI |
| --- | --- | --- |
| CSV evidence | `internal/services/reporting/` | `g8e report` |
| Proof-backed compliance | `internal/services/compliance/` | `g8e compliance` |
| Runtime path constants | `internal/constants/paths.go` | |
| External path reference | `protocol/constants/compliance_paths.json` | |

Evidence scope and signed artifacts are owned by the [Release Process](release_process.md). Control-alignment prose is [Compliance Alignment](../reference/compliance-alignment.md).

### Other product components

| Component | Entry | Also |
| --- | --- | --- |
| Ensemble (g8ee) | `ensemble/app/main.py` | Application code in `ensemble/app/`. Tests in `ensemble/tests/`. Docs: [g8ee index](../ensemble/index.md). |
| Dashboard (g8ed) | `dashboard/server.js` | Resolves workload identity before listen, serves the SPA and `g8e-config.js`, and injects the browser Gateway origin. The browser calls the Gateway directly. App code in `dashboard/public/`. Enrollment in `dashboard/services/infra/`. Tests in `dashboard/test/`. Docs: [g8ed index](../dashboard/index.md). |
| Demos | `demos/` | Containerized services and verification scripts. CLI: `internal/cli/cmd/demos/`. Reference client: `internal/tools/agent_harness/`. |
| Website | `website/` | Renders the root `README.md`. `make website-test`, `make website-build`. Generation policy: [Documentation Guide](docs.md#generated-and-machine-readable-documentation). |

### Test map

Depth is [Testing](tests.md). `e2e-full` and Compose profiles: INV-TESTMAP-02, INV-TEST-12, INV-TEST-13.

| Command | Tier | Where the work lives |
| --- | --- | --- |
| `./g8e test unit` | 1 | Delegates to `make test-unit`. Go tests sit beside packages. `make test-unit` depends on `constants-check`. |
| `./g8e test integration` | 2 | `integration` build tag. Cross-package suites in `test/`. Reusable setup in `test/fixtures/`. |
| `./g8e test e2e` | 3 | `e2e` build tag. Tests in `test/e2e/`. Expects an already running platform. |
| `./g8e test e2e-full` | 3 | Same `go test` arguments as `e2e`, wrapped in `docker compose up -d` and `docker compose down -v`. Profile names: INV-TESTMAP-02. |
| `./g8e test coverage` | | Coverage entry in `internal/cli/cmd/test/test.go`. Report layout: [Testing](tests.md). |
| `./g8e test lint` | | CLI lint entry. Repository lint target is `make lint`. |
| `./g8e test chaos` | | Chaos command in `internal/cli/cmd/test/chaos.go`. Engine: `internal/tools/chaos/`. |
| `./g8e test summary` | | Reads the chaos summary from the test vault. |
| `./g8e test public-loop` | | Provider-free public-feed qualification. Stays in `testcmd` (INV-PKG-02). |

Component test entry points named by the root Makefile: `make test`, `make test-unit`, `make test-integration`, `make test-docker`, `make ensemble-test`, `make test-external`, `make dashboard-test`. `make test-external` is the Ensemble external-provider suite (Tier 4 in [Testing](tests.md)), not a `./g8e test` subcommand.

### Build and validation

Go module line and binary packaging rules are INV-ENV-01 and the owned-surface rows in [Developer Guidelines](devs.md). `Makefile` `MAIN_PKG` is `./cmd/g8e`.

| Target | Owns |
| --- | --- |
| `make build` | Host binary. Writes `bin/g8e-<os>-<arch>` and a repo-root copy. Depends on `embed-explorer`. |
| `make proto` | Go, Python, TypeScript, and Ensemble lockfile refresh (INV-PROTO-01). |
| `make constants`, `make constants-check` | `internal/tools/constgen` write and verify. |
| `make lint` | `lint-no-embedded-newlines`, `vulncheck`, `validate-doctrines`, `validate-cosais`, `swagger-generate`, then `golangci-lint run`. |
| `make test`, `make test-unit`, `make test-integration`, `make test-docker` | Platform tiers. `make test` is unit plus integration. |
| `make python-build` | Python protocol distribution with bundled registries. |
| `make dashboard-test`, `make ensemble-test`, `make test-external`, `make website-test` | Dashboard, Ensemble, external-provider, and website checks. |
| `make build-fips`, `make verify-fips` | Pinned Linux AMD64 FIPS variant. Reference: [FIPS 140-3](../reference/fips140-3.md). |
| `make validate-doctrines` | Doctrine JSON under `protocol/constants/doctrine/`. |
| `make validate-cosais` | `go run ./internal/tools/cosais_validator`. |
| `make swagger-generate` | Gateway OpenAPI from Swagger annotations. |

Other tools under `internal/tools/`: `agent_harness` (typed governance client and demo scenarios), `chaos`, `doctrine_validator`, `g8ebinaries`, `terminalmedia`, and `treehash` (source manifest hash used by `Makefile`).

## Procedures

### Find an owner

1. Start with the table in [Owned surfaces](#owned-surfaces). Open the repo-relative path in the row.
2. If the question is a coding rule, stop and use [Developer Guidelines](devs.md). If it is how to run or fixture a test, stop and use [Testing](tests.md).
3. Confirm a CLI flag with `./g8e <command> --help` before writing it down (INV-MAP-02).

### Confirm the Cobra inventory

1. From the repo root:

```bash
./g8e --help
```

2. Compare the group list with `rootCmd.AddCommand` in `internal/cli/cmd/main.go` and with [CLI packages](#cli-packages).
3. A new group is a package under `internal/cli/cmd/<group>/` (INV-CLI-01). Apply INV-PKG-01 when the directory name collides with an existing import. Do not register `shared`, `gwremote`, or `cmdtest` (INV-PKG-03).

### Confirm a runtime mode

1. Gateway HTTP, embedded substrate, and outbound Operator are three owners (INV-MODE-01).
2. Embedded claim and pending registration stay in `internal/services/gateway/embedded/`. The HTTP shell stays `operator_controller.go` (INV-MODE-02).
3. Outbound startup stays `internal/cli/serve/operator.go` into `G8eoService`. Dependency fields are the mode table. Do not restate INV-DEP or INV-FS here.

### Confirm a generator before editing output

1. Protobuf language outputs use the generator table (INV-PROTO-01). Refresh with:

```bash
make proto
```

2. Event and action-type constants:

```bash
make constants-check
```

Write them with `make constants` when the registry change requires it.

3. Gateway OpenAPI:

```bash
make swagger-generate
```

4. Do not hand-edit generated protobuf, constgen, or OpenAPI output (INV-GEN-01).

### Confirm Compose profiles before describing `e2e-full`

1. Read the live command text:

```bash
./g8e test e2e-full --help
```

2. Read the profile slice in `internal/cli/cmd/test/test.go` and every `profiles:` key in `docker-compose.yml`.
3. Describe the command with INV-TESTMAP-02. The CLI still passes the profile name `bootstrapped`. The Compose file does not define that profile. Health wait and teardown details are INV-TEST-12.

## Anti-patterns

- Pasting a coding, filesystem, dependency-construction, or native-tool essay that already has an id in [Developer Guidelines](devs.md) (INV-MAP-01, INV-MCP-03).
- Copying the `RegisterNativeTools` list into this file or another doc (INV-MCP-03).
- Calling `G8eoService` the embedded substrate, or moving the embedded document owner out of `internal/services/gateway/embedded/` (INV-MODE-01, INV-MODE-02).
- Registering `shared`, `gwremote`, or `cmdtest` as a Cobra group (INV-PKG-03).
- Moving `g8e test public-loop` into `internal/cli/cmd/public/`, or constructing `public restore` anywhere except `eval.PublicRestoreCmdWithConfig` (INV-PKG-02).
- Attributing Python or TypeScript protobuf generation to the root `buf.gen.yaml` (INV-PROTO-01).
- Calling `bootstrapped` or `evaluation` a Compose profile that selects workloads. The CLI passes the name `bootstrapped`; `docker-compose.yml` does not define it (INV-TESTMAP-02, INV-TEST-13).
- A flag dump in place of `./g8e <command> --help` (INV-ENV-03).

## Links out

- [Developer Guidelines](devs.md): coding invariants. CLI group placement is INV-CLI-01. Gateway construction is [Gateway governance construction](devs.md#gateway-governance-construction). `e2e-full` procedure is INV-TEST-12 and INV-TEST-13.
- [Testing](tests.md): tiers, fixtures, selection, race, coverage, and CI. `tests.md` still says `e2e-full` starts a `bootstrapped` Compose profile. That sentence disagrees with tip `docker-compose.yml` (INV-TEST-13). This PR does not rewrite `tests.md`.
- [Documentation Guide](docs.md): audit, catalog, style, generation, and this format.
- [Release Process](release_process.md): versioning, native evaluation acceptance, and signed evidence.
- [Troubleshooting](troubleshooting.md): maintainer diagnosis. Not yet in this format.
- [Architecture Overview](../architecture/overview.md), [Gateway](../architecture/gateway.md), [Operator](../architecture/operator.md), [Governance](../architecture/governance.md), [Consensus](../architecture/consensus.md), [Storage](../architecture/storage.md), [Protocol](../architecture/protocol.md).
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): ingress limits.
- [Protocol README](../../protocol/README.md) and [Protocol Specification](../../protocol/docs/spec.md).
- [Getting Started](../guides/getting_started.md): deployment and enrollment.
- Component indexes: [Ensemble](../ensemble/index.md), [Dashboard](../dashboard/index.md), [Demos](../../demos/README.md).
