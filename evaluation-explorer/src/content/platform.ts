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

export const ABOUT_SITE_INTRO_BEFORE = 'This Evaluation Explorer is a live window into ';
export const ABOUT_SITE_INTRO_AFTER =
  ' — a zero-trust execution platform between people, AI systems, and the systems they affect. Evaluations are one application of g8e: this Explorer makes their results and available evidence inspectable.';

export const ABOUT_PURPOSE_HEADING = 'Capable help. Control stays with you.';
export const ABOUT_PURPOSE =
  'An AI system can investigate, reason, and propose work. g8e separates that reasoning from the authority to act, so people can get useful help without surrendering control of their systems and data.';

export const ABOUT_PRINCIPLES = [
  {
    label: 'Govern the action',
    detail:
      'Bind identity, intent, payload, and state to a verifiable transaction. Enforce policy, with consensus and exact-action human approval when the active posture requires them.',
  },
  {
    label: 'Verify where execution happens',
    detail:
      'The Gateway admits work; the Operator independently verifies it at the managed system. Execution authority and authoritative receipts stay at the data owner’s boundary.',
  },
  {
    label: 'Show the evidence',
    detail:
      'Signed receipts record execution attempts and outcomes. Claims follow the evidence: g8e governs its execution path, not tools or side channels that bypass it.',
  },
] as const;

export const ABOUT_ORIGIN_HEADING = 'Why I built it';
export const ABOUT_ORIGIN =
  'During production incidents, customers needed someone to take the problem off their plate: gather context, explain the next step, work inside their controls, verify the fix, and leave a clear record. I wanted people to have that kind of help in their pocket.';
export const ABOUT_METHOD =
  'I call that operating method “Danny-as-Code”: investigate carefully, justify the next action, obtain approval when required, execute within the owner’s controls, and follow through with evidence. It is the design philosophy behind g8e.';

export const ABOUT_HEADING = 'The solo builder';
export const ABOUT_OPENING_BEFORE = "I'm ";
export const ABOUT_OPENING_AFTER =
  ' — founder of Lateralus Labs, principal engineer, and U.S. Navy veteran in Portland, Oregon. I designed, built, and operate g8e as a solo builder.';
export const ABOUT_BACKGROUND =
  'Thirty years in data protection, distributed systems, and production operations shape my work in AI security and agentic infrastructure. I take work from architecture through code, deployment, and incident response, while mentoring engineers and working directly with customers.';

export const ABOUT_EXPERIENCE = [
  {
    company: 'Lateralus Labs · 2025–present',
    text: 'Built g8e end to end: execution governance, identity, agent orchestration, evaluations, APIs, and the operator experience.',
  },
  {
    company: 'Igneous → Rubrik · 2019–2025',
    text: 'Built reliability and support practices from the ground up, carried them through acquisition, and owned service reliability for multi-petabyte NAS Cloud Direct.',
  },
  {
    company: 'Quantum · 2015–2019',
    text: 'Led complex NAS and StorNext investigations; built diagnostic labs and Go automation for production support.',
  },
  {
    company: 'Nike Global Backup · 2000–2015',
    text: 'Engineered enterprise data protection, recovery, automation, and chain-of-custody operations across four service providers.',
  },
  {
    company: 'U.S. Navy · 1996–2000',
    text: 'Commissioned shipboard IT aboard USS Bataan and led UNIX, database, and help-desk operations.',
  },
] as const;

export const ABOUT_AVAILABILITY_HEADING = 'Let’s work together';
export const ABOUT_AVAILABILITY =
  'Reach out about g8e licensing, consulting, contract engagements, or full-time W-2 roles.';
export const ABOUT_AVAILABILITY_DETAIL =
  'I can help with AI security and agentic systems, platform engineering and SRE, or distributed data infrastructure — from an ambiguous problem through implementation and production ownership.';

export const G8E_CORE_DOCS = {
  about: `${G8E_REPO_URL}/blob/main/docs/core/about.md`,
  position: `${G8E_REPO_URL}/blob/main/docs/core/position_paper.md`,
} as const;

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
  '26 frozen agent scenarios across nine behavior categories — instruction, tools, routing, security, recovery, and synthesis',
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
