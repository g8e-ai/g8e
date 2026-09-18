// Shared copy for how OpenDevOps.ai deploys and presents g8e.
// Used by the landing System overview and the Docs architecture section.

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

export const PLATFORM_LEDE =
  'OpenDevOps.ai is a one-person project running real evaluation campaigns on consumer hardware — open-source SLMs scored through the full g8e agent stack, not isolated API calls. Only signed snapshots leave the host.';

export const PLATFORM_MEASUREMENT_SUMMARY =
  'Each candidate model is scored in every g8e role — Primary, Assistant, and Light — across 25 frozen agent scenarios. The goal is per-role metrics (pass rate, tool selection, throughput, escalation) to identify the strongest open models, then compare those picks against single-LLM baselines.';

export const PLATFORM_PORTFOLIO_NOTE =
  'This explorer is a live portfolio piece — the same pipeline I use for production evals, plus the public mirror UI I built on top. I am available for contract work on governed AI, evaluation infrastructure, and read-only observability surfaces.';

export const PLATFORM_FLOW_STEPS = [
  {
    id: 'workstation',
    label: 'Home PC',
    detail: 'Docker + Ollama',
  },
  {
    id: 'g8e',
    label: 'g8e stack',
    detail: 'g8eg · g8eo · eval',
  },
  {
    id: 'mirror',
    label: 'Public mirror',
    detail: 'SSE mirror',
  },
  {
    id: 'browser',
    label: 'This page',
    detail: 'Portfolio viewer',
  },
] as const;

export const PLATFORM_OVERVIEW_PORTFOLIO_NOTE =
  'Live portfolio piece — looking to join a team, available for contracts on governed AI and eval infrastructure.';

export const G8E_STACK_COMPONENTS = [
  {
    id: 'gateway',
    label: 'g8eg · Gateway',
    detail: 'Policy admission, routing, public mirror, and the Cloudflare tunnel origin on this workstation.',
  },
  {
    id: 'operator',
    label: 'g8eo · Operator',
    detail: 'Host-bound execution boundary — tools, filesystem, and signed evidence on the managed host.',
  },
  {
    id: 'ensemble',
    label: 'g8ee · Ensemble',
    detail: 'Production multi-agent chat path that turns evaluation scenarios into governed inference and tool calls.',
  },
  {
    id: 'eval',
    label: 'g8e eval',
    detail: 'Native campaign orchestration, rubric grading, and signed report bundles for every run you see here.',
  },
] as const;

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
  evals: 'https://github.com/g8e-ai/g8e/blob/main/docs/ensemble/evals.md',
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
  'Primary, Assistant, and Light role stack through the production g8ee chat path',
  'Governed inference dispatch to Ollama — never a direct provider API shortcut',
  'Host-bound tool, filesystem, and process execution through the Data Operator boundary',
  'Rubric pass/fail, tool scorecards, escalation disposition, and timing telemetry when observed',
  'Signed campaign evidence with explicit quality states — live, exploratory, or verified',
] as const;

export const G8E_CAMPAIGN_OPERATORS = [
  {
    role: 'Gateway (g8eg)',
    wire: 'PDP',
    detail: 'Admits envelopes, enforces L1–L3, routes inference and tool work to bound Operator sessions, coordinates provider-boundary observation.',
  },
  {
    role: 'Data Operator',
    wire: 'g8eo',
    detail: 'Governed host boundary for model-originated tools, filesystem, and process actions during scenarios.',
  },
  {
    role: 'Inference Operator',
    wire: 'g8eo',
    detail: 'Sole scored path to the approved Ollama provider — L4/L5 inference PEP on the campaign host.',
  },
  {
    role: 'Observer Operator',
    wire: 'g8eo',
    detail: 'Read-only provider-boundary witness on the GPU host — binds hardware samples to inference attempts without mutation authority.',
  },
] as const;
