// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Shared copy for how OpenDevOps.ai deploys and presents g8e.
// Used by the About page and the Docs architecture section.

export const G8E_REPO_URL = 'https://github.com/g8e-ai/g8e';

export const GITHUB_SPONSORS_URL = 'https://github.com/sponsors/Badoot';

export const PLATFORM_SITE_URL = 'https://opendevops.ai';

export const PLATFORM_CONTACT_EMAIL = 'danny@lateraluslabs.com';

export const PLATFORM_CONTACT_CALENDLY = 'https://calendly.com/danny-lateraluslabs/quick_discovery';

export const PLATFORM_CONTACT_LINKEDIN = 'https://www.linkedin.com/in/dannybarbour/';

export const SPONSORSHIP_LEDE =
  'OpenDevOps.ai is a fully independent, verifiable LLM benchmarking project. I am completely self-funded and refuse to take venture capital. If this data helps you, please help me keep the servers running and the pipeline unbiased.';

export const SPONSORSHIP_USES = [
  'Keep the public mirror, Cloudflare tunnel, and evaluation pipeline online',
  'Fund GPU time and campaign runs on the home workstation that produces these scores',
  'Preserve editorial independence — no venture capital and no vendor-sponsored benchmarks',
] as const;

export const PLATFORM_SOLO_NOTE = 'Solo operator · home-PC hardware · live pipeline';

export const PLATFORM_OVERVIEW_LEDE =
  'This site is a live deployment of the g8e AI governance suite, not a separate benchmark product. Evaluations run through the governed execution path and publish public-safe results to this Explorer.';

export const ABOUT_SITE_INTRO_BEFORE = 'This Explorer is a live window into ';
export const ABOUT_SITE_INTRO_AFTER =
  ' — the sovereign execution-governance platform I designed, built, and operate end-to-end. Every campaign you see here runs through the same governed path a production agent workload would take, with public-safe telemetry and cryptographic proofs published live to this mirror.';

export const ABOUT_HEADING = 'Who Builds This — And Can Help You Ship Yours?';

export const ABOUT_OPENING_BEFORE = "I'm ";
export const ABOUT_OPENING_AFTER =
  ' — Principal Engineer, U.S. Navy veteran, and founder of Lateralus Labs.';

export const ABOUT_BACKGROUND =
  "For thirty years, I have built and operated mission-critical distributed systems: commissioning shipboard IT from bare steel, running one of the world's largest enterprise backup environments at Nike, scaling petabyte-scale NAS-to-cloud platforms at Igneous and Rubrik, and now shipping sovereign agentic infrastructure that keeps state, credentials, and execution strictly under your control — not the model provider's.";

export const ABOUT_LEADERSHIP =
  "I don't architect and hand off. I lead from the front — design the system, write the code, stand up the platform, run the incidents, mentor the team, and ship.";

export const ABOUT_EXPERIENCE = [
  {
    company: 'At Igneous',
    text:
      "I built the SRE practice from 0→1 and carried it through acquisition into Rubrik's NAS Cloud Direct product.",
  },
  {
    company: 'At Rubrik',
    text:
      'I owned reliability for a multi-petabyte platform and turned manual operations into observable, automated services.',
  },
  {
    company: 'At Lateralus Labs',
    text:
      'I shipped g8e as sole engineer: heterogeneous consensus, zero-trust admission, cryptographic audit state, evaluation infrastructure, edge operators, APIs, SDKs, and this Explorer — work that typically requires entire platform, security, AI, and frontend teams.',
  },
] as const;

export const ABOUT_DELIVER_HEADING = 'What I Own & Deliver';

export const ABOUT_DELIVER_INTRO =
  'I work best with founders and engineering leaders who need someone to own ambiguous, high-stakes problems end to end:';

export const ABOUT_DELIVERABLES = [
  {
    label: 'Agentic Execution & Governance',
    detail:
      'Multi-agent orchestration, agentic execution environments, and fail-closed governance layers.',
  },
  {
    label: 'Platform Engineering & 0→1 SRE',
    detail:
      'Reliability practices, observability pipelines, and infrastructure scaling through acquisition and hypergrowth.',
  },
  {
    label: 'Zero-Trust Security',
    detail:
      'Policy enforcement, identity management, and cryptographic attestation for autonomous workloads.',
  },
  {
    label: 'LLM Evaluation & Red-Teaming',
    detail:
      'Adversarial testing, benchmark telemetry, and CI-integrated regression detection.',
  },
  {
    label: 'Sovereign Infrastructure',
    detail:
      'Petabyte-scale distributed systems, data protection, and air-gapped data plane management.',
  },
] as const;

export const ABOUT_AVAILABILITY_HEADING = 'Availability & Engagement';

export const ABOUT_AVAILABILITY_LEDE_BEFORE = 'I am available for ';
export const ABOUT_AVAILABILITY_LEDE_MIDDLE = ' and open to joining the right team ';
export const ABOUT_AVAILABILITY_LEDE_AFTER = '.';

export const ABOUT_AVAILABILITY_DETAIL =
  'I am looking for hands-on leadership roles where I can wear multiple hats — architect and implement, mentor engineers, lead by example, and stay close to production. High agency, low supervision. Give me a hard problem and the authority to execute; I will drive it from design through operations.';

export const WORKSTATION_SPECS = [
  { label: 'CPU', value: 'Intel Core i9-13900K' },
  { label: 'Memory', value: '64 GB RAM' },
  { label: 'GPU', value: 'NVIDIA GeForce RTX 4070 Ti SUPER · 16 GB VRAM' },
  { label: 'Runtime', value: 'Docker on Windows · Ollama for local model inference' },
] as const;

export const G8E_ARCHITECTURE_DOCS = {
  overview: 'https://github.com/g8e-ai/g8e/blob/main/docs/architecture/overview.md',
  governance: 'https://github.com/g8e-ai/g8e/blob/main/docs/architecture/governance.md',
  operator: 'https://github.com/g8e-ai/g8e/blob/main/docs/architecture/operator.md',
  evals: 'https://github.com/g8e-ai/g8e/blob/main/docs/architecture/evals.md',
} as const;

export const G8E_DIFFERENTIATORS_LEDE =
  'Most benchmarks score a model API in isolation. g8e scores the full governed execution path — cryptographic proofs, host-bound operators, and the same multi-agent stack a production workload would traverse.';

export const G8E_DIFFERENTIATORS = [
  {
    headline: 'Proof, not promises',
    detail:
      'Every governed mutation is a typed, signed, state-bound GovernanceEnvelope. The Gateway admits it; the Operator independently re-verifies hash, nonce, expiry, and posture proofs before any side effect. Signed receipts and content-addressed evidence land in report.json and verification.json — reproducible offline with g8e eval verify, not trust in this browser.',
  },
  {
    headline: 'Five-layer fail-closed governance',
    detail:
      'L1 Doctrine through L5 Actuator form one pipeline: policy admission, consensus and notary gates when required, local Warden verification, and a single Actuator execution boundary. Required proofs fail closed; optional layers remain auditable evidence.',
  },
  {
    headline: 'Outbound-only, no-install operators',
    detail:
      'Remote Operators and Observers are the same static g8e binary — no package install, no inbound management port, no root required. Each session dials out over mTLS, pulls work from the Gateway, and records authoritative evidence in the directory where it was started.',
  },
  {
    headline: 'Independent witnesses, not self-report',
    detail:
      'Scored campaigns enroll separate remote sessions for data execution, governed inference, and provider-boundary observation. The Observer samples GPU and host telemetry at the inference boundary without prompt access or mutation authority — the executor cannot attest its own hardware usage.',
  },
] as const;

/** The complete package every model candidate is measured against. */
export const G8E_MEASURED_TOGETHER = [
  '25 frozen agent scenarios across nine behavior categories — instruction, tools, routing, security, recovery, and synthesis',
  'Primary, Assistant, and Lite role stack through the production g8ee chat path',
  'Governed inference dispatch to Ollama — never a direct provider API shortcut',
  'Host-bound tool, filesystem, and process execution through the Data Operator boundary',
  'Rubric pass/fail, tool scorecards, escalation disposition, and timing telemetry when observed',
  'Signed campaign evidence with explicit current-standard, run-scoped, incomplete, legacy, live, and failed quality states',
] as const;

export const G8E_CAMPAIGN_OPERATORS = [
  {
    role: 'Gateway (g8eg)',
    wire: 'PDP',
    detail: 'Admits envelopes, enforces L1–L3, routes inference and tool work to bound Operator sessions, coordinates provider-boundary observation and model provenance attestation.',
  },
  {
    role: 'Inference Operator',
    wire: 'g8eo',
    detail: 'Sole scored path to the approved Ollama provider — L4/L5 inference PEP on the campaign host.',
  },
  {
    role: 'Provenance Operator',
    wire: 'g8eo',
    detail: 'Independent storage-side model weight attestor — hashes manifests and blobs at the model storage site and binds digest evidence to inference attempts.',
  },
  {
    role: 'Observer Operator',
    wire: 'g8eo',
    detail: 'Read-only provider-boundary witness on the GPU host — binds hardware samples to inference attempts without mutation authority.',
  },
  {
    role: 'Data Operator',
    wire: 'g8eo',
    detail: 'Governed host boundary for model-originated tools, filesystem, and process actions during scenarios.',
  },
] as const;
