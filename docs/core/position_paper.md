---
title: Position Paper
parent: Core
---

# Governing Agentic Execution Without Surrendering Data Custody

Last Updated: 2026-09-08
Version: v2.1.7

## Abstract

AI agents combine probabilistic reasoning with access to data, tools, credentials, and persistent state. This combination creates a structural security problem: a model output can become an infrastructure action before an independent authority has established that the action is authentic, permitted, current, and attributable. Empirical studies of tool-integrated agents document successful indirect prompt injection, memory poisoning, and tool misuse across model families, while current guidance identifies data privacy, authorization, and human oversight as unresolved deployment concerns [1–7]. Improving model behavior is necessary, but it does not create an adequate execution boundary.

g8e takes the position that an AI system may propose an action but does not authorize its own execution. The platform separates reasoning from authority, keeps authoritative state and evidence at the data owner's boundary, represents governed mutations as typed and state-bound transactions, and independently verifies each transaction where execution occurs. This paper defines that position, relates it to zero-trust and least-privilege principles, describes the g8e reference architecture, and separates implemented mechanisms from measured evidence and open research questions. The claim is deliberately narrow: operations that traverse the g8e boundary receive the controls and evidence described here; g8e does not govern tools or side channels that bypass that boundary.

## 1. Introduction

The central problem in agentic computing is not that language models sometimes produce incorrect text. It is that systems increasingly convert model output into consequential action. An agent may read an email, retrieve a document, call an API, edit a file, execute a command, or approve a workflow. Once reasoning and execution share the same ambient credentials and mutable context, a prompt-level failure can become a confidentiality, integrity, or availability failure.

This risk is not hypothetical. InjecAgent evaluated 30 tool-integrated agent configurations across 1,054 test cases and reported a 24% attack success rate for a ReAct-prompted GPT-4 agent under its base indirect-prompt-injection setting [1]. AgentDojo introduced 97 realistic tasks and 629 security test cases in stateful environments and found that contemporary agents and defenses did not jointly provide reliable task completion and security [2]. Agent Security Bench evaluated attacks and defenses across 10 scenarios, more than 400 tools, and 13 model backbones; its tested attacks reached a highest average success rate of 84.30% [3]. These results are benchmark-specific and do not estimate universal field failure rates, but they establish that model instructions alone are not a dependable authorization mechanism.

NIST's Generative AI Profile similarly treats prompt injection, data privacy, information security, and human–AI configuration as system-level risks rather than isolated model defects [4]. The 2026 International AI Safety Report concludes that agents heighten reliability risks because autonomous action can reduce the opportunity for human intervention before harm occurs [5]. NIST's 2026 concept paper on software and AI agent identity frames identification, authentication, authorization, auditing, and non-repudiation as prerequisites for reducing agent deployment risk [6].

This paper advances three propositions:

1. **Reasoning is not authority.** A model, agent, or ensemble produces proposals. A separately controlled execution system decides whether a proposal may affect real state.
2. **Governance attaches to the action.** Identity, intent, target, payload, state, freshness, policy evidence, and authorization belong to one verifiable transaction rather than to an open-ended session.
3. **Claims follow evidence.** Signed receipts and independently reproducible verification establish what occurred on the governed path; architecture descriptions and successful demonstrations do not become claims of certification, universal safety, or production suitability.

The g8e architecture operationalizes these propositions. Its design vocabulary includes Gateways, Operators, Doctrine, Consensus, Notary, Warden, and Actuator. These names identify concrete protocol and service roles; they are not claims of artificial personhood or institutional authority.

## 2. Practitioner Origin and Research Position

This position originates in a practitioner method developed over thirty years of operating and protecting remote data systems. In production incidents, trust did not arise because an expert sounded confident. It arose because the expert worked inside the customer's controls, gathered evidence from the affected systems, explained the next action, used authority supplied for that purpose, verified the result, and left a record that others could examine.

The [About g8e](./about.md) page describes this method as “Danny as Code”: gather broad context, reduce uncertainty with focused questions, converge on a justified next step, present that step to the person with the most at stake, bind any required approval to the exact action, execute through a constrained local boundary, and preserve evidence through completion. The phrase supplies design history, not empirical validation. The research position is that this operating method can be expressed as a machine-verifiable separation among proposal, authorization, execution, and evidence.

The human motivation remains important. Good automation reduces operational burden without asking a person to surrender control. In this model, an AI system can perform substantial investigative and planning work, but confidence, fluency, or apparent expertise never substitutes for authorization. Trust is constructed from bounded authority and inspectable evidence.

## 3. The Coupling Problem in Agent Architectures

### 3.1 Reasoning, context, and authority

A useful agent needs context. A useful tool needs authority. A conventional implementation often gives one agent process both: it reads application data into model context and carries credentials broad enough to execute the resulting tool calls. Persistent memory may then retain selected observations and outputs. This design couples four distinct concerns:

- **Epistemic state:** what the model has observed or inferred.
- **Authoritative state:** what the managed system currently contains.
- **Execution authority:** what the process is permitted to change.
- **Evidence:** what can later establish which action was requested, admitted, attempted, and completed.

The coupling is dangerous because model context is both data and instruction surface. Indirect prompt injection exploits this ambiguity by placing adversarial instructions in content an agent is expected to read [1,2]. Memory poisoning extends the effect across later interactions [3]. Tool misuse turns a valid tool and valid credential toward an invalid purpose. A more capable model can improve task performance while leaving this architectural ambiguity intact.

### 3.2 Custody as an architectural variable

Cloud inference does not inherently require cloud custody of all authoritative state. A deployment can instead keep raw values, keys, execution state, and authoritative evidence beside the systems that own them while sending a minimized projection to a remote reasoning service. The remote model returns a proposal; the local execution boundary verifies and applies it.

This separation does not imply zero disclosure. A model can only reason from information it receives, and some tasks require semantically meaningful data. Scrubbing and reversible placeholders reduce selected disclosures but cannot prove that every prompt is free of sensitive information. The defensible claim is therefore path- and deployment-specific: g8e provides bounded scrubbing and local rehydration mechanisms for governed paths, while the deployment owner remains responsible for model inputs, enabled tools, local account privileges, external credentials, and bypass channels. The [AI agent boundary](../architecture/agents.md) and [encryption architecture](../architecture/encryption.md) define these limits.

### 3.3 Session authority and action authority

Session-based authorization answers whether a principal may use a service over a period of time. Agentic execution requires a narrower question: may this principal perform this exact mutation, against this target and state, before this expiry, with these policy proofs? A broad bearer credential does not answer that question.

NIST SP 800-207 defines zero trust around least-privilege, per-request access decisions in a network assumed to be compromised [7]. The classic principle of least privilege likewise limits a subject to the privileges necessary for a task [8]. g8e applies these principles to agentic actions: identity establishes who or what proposed work, but authority is derived for one verified transaction rather than inferred from the proposer's identity or network location.

## 4. Design Requirements

The preceding problem statement yields six requirements for governed agentic execution.

### 4.1 Treat every proposer as untrusted

Humans, models, ensembles, applications, MCP clients, and A2A clients may request work. None receives execution authority merely because it generated a plausible plan. This rule removes model alignment from the trusted computing base for authorization.

### 4.2 Bind policy to canonical intent

The governed object must encode the action type, target resource, typed payload, requesting identities, state binding, nonce, expiry, and required governance evidence. Canonical serialization and hashing must make changes detectable so that signatures over one transaction cannot authorize another.

### 4.3 Verify at the execution site

A remote admission decision cannot force a local mutation. The component beside the managed system must independently verify freshness, replay state, canonical intent, policy, state, and posture-required approvals before dispatch.

### 4.4 Minimize standing authority

Execution handlers receive authority scoped to the admitted transaction. The model does not carry host credentials, and admission does not create a reusable authorization session. Operating-system privileges and external credentials remain deployment controls and must be constrained independently.

### 4.5 Preserve evidence before and after dispatch

A system that records only successful outcomes cannot distinguish “never executed” from “executed but failed to report.” The execution boundary must preserve signed pre-execution and final evidence, with durable-persistence evidence where supported, and must refuse dispatch when required pre-execution evidence cannot be persisted.

### 4.6 Make the claim boundary explicit

A receipt proves activity on the governed route. It does not prove the absence of direct shell access, client-native tools, another MCP server, compromised operating-system privileges, or any other side channel. Security evaluation must test the complete deployed topology rather than infer safety from the presence of a governance component.

## 5. The g8e System Model

### 5.1 Roles and trust boundaries

g8e implements a Policy Decision Point and Policy Execution Point pattern consistent with zero-trust architecture [7]:

- The **Governance Gateway (g8eg)** authenticates callers, constructs and admits canonical envelopes, applies L1 Doctrine, coordinates posture-required L2 Consensus and L3 Notary workflows, and routes work. It cannot bypass downstream verification.
- The **Governed Operator (g8eo)** runs beside managed systems, initiates an outbound mTLS connection, pulls work, performs L4 verification, dispatches accepted transactions through L5, and retains authoritative local evidence.
- A **reasoning application**, including the optional g8ee ensemble or a third-party client, gathers context and proposes actions. It remains outside the trusted execution boundary.
- A **human authorizer** supplies transaction-bound approval when the active governance posture requires it. Approval proves authorization of the bound transaction under the configured ceremony; it does not prove that the person understood every consequence.

The Gateway and Operator roles are implemented in the same statically linked Go binary but run in separate modes. Their separation is logical and operational: the Gateway admits and routes; the outbound Operator re-verifies and executes. The Gateway process also contains an in-process Operator substrate for work executed on the Gateway host, and that path still traverses L4 and L5. A remote Operator exposes no inbound management port to the Gateway and can remain behind NAT or restrictive firewall policy. The [platform overview](../architecture/overview.md) defines the complete topology.

### 5.2 GovernanceEnvelope

The canonical unit of governed work is the protobuf `GovernanceEnvelope`. It carries typed intent, requesting identities, target, payload, nonce, expiry, state root, transaction hash, active posture, and available governance proofs. Its canonical transaction hash binds the action type, target, payload, state root, nonce, expiry, structured intent, requesting user, and acting application. Posture remains explicit policy metadata outside that hash, while posture-required proofs are verified against the transaction at L4.

This action-centric representation is the main architectural move. Policy evaluation, consensus votes, human approval, local verification, execution capability, receipts, and commitments refer to one transaction. They do not refer to an agent's conversational confidence or to an indefinitely reusable authorization grant. The [protocol specification](../../protocol/docs/spec.md) owns the wire contract and canonicalization rules.

### 5.3 Five-layer verification and execution

Every governed operation reaches the L4 Warden and L5 Actuator boundary. The active posture determines whether L2 and L3 are required gates or recorded, non-gating evidence:

1. **L1 Doctrine** decodes typed payloads and applies field constraints, forbidden-pattern rules, and threat-oriented checks. L1 applies in every posture.
2. **L2 Consensus** verifies Ed25519 votes from enrolled members against the configured policy and quorum when the posture requires multi-agent authorization.
3. **L3 Notary** verifies transaction-bound human authorization for mutations when the posture requires it. Gateway workflows use WebAuthn; outbound Operator workflows use signed approval proofs.
4. **L4 Warden** reserves the nonce, checks expiry and replay state, recomputes the transaction hash, validates state and payload, reruns Doctrine, and verifies posture-required L2 and L3 evidence.
5. **L5 Actuator** persists signed pre-execution evidence, appends a commitment when the SQL commitment ledger is available, rehydrates explicitly registered protected values at the execution site, mints a transaction-bound capability, dispatches the handler, dissolves the capability, and persists the signed final outcome.

The ordering reduces unnecessary human interruption: deterministic and machine-checkable gates run before a required human ceremony. It also separates verification from side effects. L4 produces a verified transaction; L5 executes only that typed result. The [governance architecture](../architecture/governance.md) is the canonical source for posture semantics and receipt flow.

### 5.4 Local state, minimized disclosure, and rehydration

For outbound Operator execution, authoritative host state and local audit evidence remain at the managed boundary. Governed read and tool outputs pass through bounded scrubbing paths before return. Explicitly registered sensitive values can be replaced with reversible placeholders and rehydrated at L5, where the target data and keys already reside.

This mechanism supports a “commitments and projections, not custody by default” deployment pattern. It does not guarantee that every application prompt is tokenized, that every sensitive value is detected, or that a cloud model receives no meaningful information. Those are measurable properties of a specific integration and workload. g8e's compliance evidence model accordingly defines separate assertions for sensitive-data detection, model-boundary leakage, and local rehydration rather than collapsing them into a single “zero leakage” claim.

### 5.5 Transaction-bound human authorization

L3 binds human authorization to the transaction hash instead of treating login as approval for all later actions. In Gateway workflows, the WebAuthn assertion signs authenticator data and a hash of client data containing the server-supplied challenge [12]. g8e uses the transaction identity in that challenge and verifies the returned assertion against the pending transaction. Outbound Operator workflows verify a signed approval proof associated with the suspended transaction.

This construction provides cryptographic transaction binding within the g8e protocol. Plain WebAuthn does not, by itself, guarantee that an authenticator displayed the complete transaction or that the person understood it. Interface design, approval text, origin security, and organizational procedure remain part of the human authorization system. The [authentication architecture](../architecture/auth.md) defines the implemented ceremonies.

### 5.6 Receipts, commitments, and accountable memory

L5 signs and persists an `EXECUTING` receipt before dispatch and a final receipt after the handler returns. The final record binds the result and governance evidence; a persistence attestation can bind the receipt signature to durable storage. When configured, the commitment ledger adds hash-chained attestations, and governed file operations retain file evidence.

These records serve two related purposes. Operationally, they support diagnosis, audit, and result verification. Epistemically, they provide a more defensible memory substrate than unverified conversational summaries: future reasoning can distinguish proposed, rejected, executing, completed, and failed work by reference to signed records. In that limited sense, memory is sovereign—it remains controlled and verifiable at the boundary that owns the affected state. This phrase describes custody and provenance, not infallibility. A valid receipt can faithfully record a bad but authorized outcome, and a ledger cannot observe actions taken outside its path.

## 6. Evidence and Claim Discipline

### 6.1 External evidence motivating the architecture

The literature supports the need for an execution boundary, but it does not validate g8e itself.

| Source | Reported result | Relevance | Limitation |
| --- | --- | --- | --- |
| InjecAgent [1] | 1,054 cases, 30 agent configurations, and 24% attack success for ReAct-prompted GPT-4 in the reported base setting | Untrusted tool content can redirect an agent toward direct harm or data exfiltration | Benchmark result for specified agents, attacks, and tools; not a universal incident rate |
| AgentDojo [2] | 97 realistic tasks and 629 security test cases in stateful tool environments | Utility and security must be evaluated together against environment state | The benchmark does not test g8e's protocol or deployed boundary |
| Agent Security Bench [3] | 10 scenarios, more than 400 tools, 13 backbones, and a highest average attack success rate of 84.30% among tested attacks | Vulnerabilities span prompts, tools, planning, and memory; no single prompt filter covers the system | The maximum is benchmark- and attack-specific, not an average across deployed agents |
| NIST AI 600-1 [4] | Identifies prompt injection, data privacy, information security, and human–AI configuration as generative-AI risk categories | Risk management applies across the system lifecycle | Guidance, not an empirical product evaluation |
| International AI Safety Report 2026 [5] | Synthesizes evidence that agent autonomy can make intervention before harm more difficult | Consequential actions require controls outside model behavior | Broad scientific synthesis; it does not prescribe or assess this architecture |

### 6.2 Published g8e evidence

The repository publishes bounded evidence rather than treating implementation claims as measured outcomes:

- A clean v2.1.7 acceptance run used a fresh, network-disabled, read-only Linux container with all capabilities dropped and `no-new-privileges`. The candidate reproduced a signed compliance bundle and passed 10 verification checks. Four separate mutations—to protected ledger source, protected build/configuration source, rendered Markdown, and the manifest signature—each failed closed. This demonstrates one candidate and one point-in-time assessment scope; it is not certification or recurring operating effectiveness ([acceptance record](../release_notes/v2.1.x/v2.1.7-offline-acceptance.md)).
- The current public eval snapshot contains two complete five-task runs with all 10 terminal attempts retained. Their deterministic results are 4/5 and 3/5, or 7/10 in aggregate, on a curated instruction-following diagnostic using a declared local model cohort. The tasks produced no receipts, so the snapshot supports no mutation, governance, persistence, state, or compliance claim ([README evidence](../../README.md#current-public-eval-snapshot)).
- The compliance evidence model defines 13 typed control assertions and 14 evidence-grade scenarios. Its current FedRAMP 20x and NIST SP 800-53 catalog classifies 131 controls: 34 mapped and 97 unsupported. Catalog coverage is not customer compliance, authorization, or external attestation ([compliance evidence](../reference/compliance-evidence.md)).

This evidence is meaningful partly because it contains non-passing and unsupported results. A research program that reports only positive demonstrations cannot reveal the boundary of its claims. g8e's evidence model distinguishes documented, implemented, deterministically evaluated, demonstrated, continuously evidenced, and externally attested levels; the current pipeline does not claim to produce the last two.

### 6.3 What the evidence does not establish

The published record does not establish a universal sensitive-data leakage rate, resistance to every prompt injection, broad model quality, production availability, complete regulatory compliance, operating effectiveness across an assessment period, or independent external validation. It also does not establish that an Operator host is secure when its operating-system account or external credentials are overprivileged.

These exclusions are not rhetorical caveats. They define falsifiable work. A deployment claim about leakage requires provider-boundary observations and canary-based measurement. A claim about unauthorized mutation requires adversarial attempts, verified receipts, and independent terminal-state observation. A claim about durable accountability requires signature, chain, and persistence verification under fault injection. The [proof-backed compliance model](../reference/compliance-evidence.md) defines the artifact and evidence-level distinctions used for such evaluations.

## 7. Reasoning Topology and the Economics of Governance

Governance need not mean that every check invokes a frontier model. Doctrine evaluation, canonical hashing, signature verification, nonce reservation, state comparison, capability scoping, and receipt verification are deterministic. L2 is model-assisted only when the selected posture requires consensus. L3 is a human authorization ceremony, not an inference call. The expensive general-purpose reasoning role can therefore remain separate from the enforcement path.

Belcak et al. argue that small language models are often more suitable for repetitive, specialized agent subtasks and that heterogeneous systems should reserve larger models for work requiring general-purpose capabilities [9]. This is a position paper rather than proof of g8e's economics, but it supports evaluating each reasoning role independently rather than assigning one frontier model to every stage.

The consensus literature also cautions against assuming that more agents automatically produce better decisions. Bertalanič and Fortuna report that homogeneous teams of ten 7–8B models engaged in unguided three-round debate consumed 2.1–3.4 times more tokens than isolated self-correction for equal or lower accuracy, with modal adoption reaching 85.5% in their experiments [10]. By contrast, Lin et al. report gains from structured critic, defender, and judge roles on a human-annotated safety-evaluation benchmark [11]. These findings are not contradictory: they suggest that task structure, role separation, model diversity, quorum design, and communication protocol are experimental variables, not decorative complexity.

Accordingly, g8e makes no general claim that consensus is cheaper or more accurate than a single model. Its protocol gives consensus votes authority only when they come from enrolled members and satisfy a configured policy and quorum. Whether a specific member set improves decision quality, latency, or cost must be measured for that doctrine and workload. The current public 7/10 instruction-following snapshot does not evaluate L2 consensus and cannot support such a conclusion.

## 8. Scope, Limitations, and Research Agenda

### 8.1 Governed-path scope

g8e governs operations that enter an implemented Gateway or Operator path and reach the verification and execution boundary. It does not sandbox an AI client. Client-native shell access, direct filesystem access, ungoverned APIs, other MCP servers, and compromised host accounts remain outside the receipt boundary. A secure deployment removes or separately constrains bypasses rather than assuming that the governed path is the only path.

### 8.2 Data minimization limits

Pattern detection and registered reversible placeholders reduce disclosure but can produce false negatives and false positives. Semantic secrets, transformed identifiers, images, embeddings, and domain-specific values require workload-specific detection and evaluation. Model-boundary telemetry must itself avoid becoming a new sensitive-data store.

### 8.3 Human authorization limits

A transaction-bound signature proves that a configured credential approved a bound challenge under a verified ceremony. It does not prove comprehension, voluntariness, or correct risk assessment. Approval interfaces must present intelligible action details, avoid habituation, and reserve interruption for decisions where human judgment changes the outcome.

### 8.4 Endpoint and key compromise

Local custody concentrates responsibility at the Operator boundary. If the host, signing keys, vault unlock path, operating-system account, or external service credentials are compromised, protocol-level verification cannot restore endpoint integrity. Hardware-backed keys, platform attestation, credential minimization, host hardening, and independent monitoring remain complementary controls.

### 8.5 Evaluation priorities

The next evidence-bearing evaluations follow from the architecture's strongest claims:

1. Measure sensitive-data detection precision and recall, model-boundary raw-secret rate, and exact local rehydration across realistic modalities and domains.
2. Exercise unauthorized-mutation attempts through every supported ingress path and verify both canonical receipts and independently observed terminal state.
3. Fault-inject receipt signing, pre-execution persistence, final persistence, commitment append, network interruption, replay storage, and state-root changes.
4. Compare single-model, homogeneous-consensus, and heterogeneous-consensus configurations under fixed tasks, policies, model versions, cost accounting, and latency budgets.
5. Reproduce evidence on independently administered infrastructure with separate trust roots and publish negative results alongside successful runs.
6. Evaluate approval comprehension and operator workload rather than treating ceremony completion as a proxy for meaningful human control.

## 9. Conclusion

Agentic systems require a boundary between producing an answer and exercising authority. Prompt instructions, model alignment, and application-level confirmation remain useful, but empirical attack results and current risk guidance show that they do not substitute for independent authorization, least privilege, local verification, and durable evidence.

g8e's position is that the data owner retains custody of authoritative state, keys, execution, and evidence while AI systems remain replaceable reasoning components. A proposal becomes executable only as a canonical, state-bound transaction that clears the controls required by the active posture and is independently verified where the side effect occurs. Human approval, when required, binds to that transaction. Execution receives a narrow capability. Signed evidence records the attempt and outcome.

The architecture is not a claim that AI becomes trustworthy. It is a method for reducing how much trust consequential execution places in AI—or in any other proposer. The practitioner idea behind “Danny as Code” survives in a precise form: gather evidence, justify one next action, work inside the owner's controls, prove what happened, and carry the work through without taking custody away from the people who bear the consequences.

## References

1. Qiusi Zhan, Zhixiang Liang, Zifan Ying, and Daniel Kang. “[InjecAgent: Benchmarking Indirect Prompt Injections in Tool-Integrated Large Language Model Agents](https://doi.org/10.18653/v1/2024.findings-acl.624).” *Findings of the Association for Computational Linguistics: ACL 2024*, 2024.
2. Edoardo Debenedetti, Jie Zhang, Mislav Balunović, Luca Beurer-Kellner, Marc Fischer, and Florian Tramèr. “[AgentDojo: A Dynamic Environment to Evaluate Prompt Injection Attacks and Defenses for LLM Agents](https://doi.org/10.52202/079017-2636).” *Advances in Neural Information Processing Systems 37*, Datasets and Benchmarks Track, 2024.
3. Hanrong Zhang, Jingyuan Huang, Kai Mei, Yifei Yao, Zhenting Wang, Chenlu Zhan, Hongwei Wang, and Yongfeng Zhang. “[Agent Security Bench (ASB): Formalizing and Benchmarking Attacks and Defenses in LLM-based Agents](https://openreview.net/forum?id=V4y0CpX4hK).” *International Conference on Learning Representations*, 2025.
4. Chloe Autio, Reva Schwartz, Jesse Dunietz, Shomik Jain, Martin Stanley, Elham Tabassi, Patrick Hall, and Kamie Roberts. “[Artificial Intelligence Risk Management Framework: Generative Artificial Intelligence Profile](https://doi.org/10.6028/NIST.AI.600-1).” NIST AI 600-1, 2024.
5. Yoshua Bengio et al. “[International AI Safety Report 2026](https://internationalaisafetyreport.org/publication/international-ai-safety-report-2026).” DSIT 2026/001, 2026.
6. National Cybersecurity Center of Excellence. “[Accelerating the Adoption of Software and Artificial Intelligence Agent Identity and Authorization](https://csrc.nist.gov/pubs/other/2026/02/05/accelerating-the-adoption-of-software-and-ai-agent/ipd).” NIST concept paper, 2026.
7. Scott Rose, Oliver Borchert, Stu Mitchell, and Sean Connelly. “[Zero Trust Architecture](https://doi.org/10.6028/NIST.SP.800-207).” NIST Special Publication 800-207, 2020.
8. Jerome H. Saltzer and Michael D. Schroeder. “[The Protection of Information in Computer Systems](https://doi.org/10.1109/PROC.1975.9939).” *Proceedings of the IEEE* 63, no. 9, 1975.
9. Peter Belcak, Greg Heinrich, Shizhe Diao, Yonggan Fu, Xin Dong, Saurav Muralidharan, Yingyan Celine Lin, and Pavlo Molchanov. “[Small Language Models are the Future of Agentic AI](https://doi.org/10.48550/arXiv.2506.02153).” arXiv, 2025.
10. Blaž Bertalanič and Carolina Fortuna. “[The Cost of Consensus: Isolated Self-Correction Prevails Over Unguided Homogeneous Multi-Agent Debate](https://doi.org/10.48550/arXiv.2605.00914).” arXiv, 2026.
11. Dachuan Lin, Guobin Shen, Zihao Yang, Tianrong Liu, Dongcheng Zhao, and Yi Zeng. “[Efficient LLM Safety Evaluation through Multi-Agent Debate](https://doi.org/10.48550/arXiv.2511.06396).” arXiv, 2025, revised 2026.
12. World Wide Web Consortium. “[Web Authentication: An API for Accessing Public Key Credentials, Level 3](https://www.w3.org/TR/webauthn-3/).” W3C Recommendation, 2026.

## Related Documentation

- [About g8e](./about.md): Practitioner origin, platform scope, and current reference implementation.
- [Platform Overview](../architecture/overview.md): Components, trust boundaries, and end-to-end transaction flow.
- [Governance](../architecture/governance.md): Five-layer verification, posture semantics, and receipt flow.
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): Governed ingress paths, guarantees, and bypass limits.
- [Authentication and Authorization](../architecture/auth.md): mTLS, workload identity, WebAuthn, and outbound approval proofs.
- [Encryption Architecture](../architecture/encryption.md): Vault, keystore, scrubbing, rehydration, and key-custody limits.
- [Storage Architecture](../architecture/storage.md): Audit, commitment, execution-vault, and file-evidence ownership.
- [Consensus Architecture](../architecture/consensus.md): Enrollment, deliberation, signatures, policy, and quorum.
- [Proof-Backed Compliance Evidence](../reference/compliance-evidence.md): Evidence levels, typed assertions, verification, and claim boundaries.
- [Protocol Specification](../../protocol/docs/spec.md): Canonical messages, hashes, proofs, and wire rules.
