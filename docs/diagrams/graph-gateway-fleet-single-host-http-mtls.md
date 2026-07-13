# Graph: Gateway to Single-Host Fleet (mTLS Streamable HTTP)

First appeared in commit `8e12e57f`. Adds "mTLS, Streamable HTTP" annotation on the MCP client edge and lists Windsurf as a supported client.

```mermaid
graph TD
    subgraph Clients ["Any AI client, agent-agnostic"]
        C1["MCP client<br/>(Claude / Cursor / Windsurf)"]
        C2["Agentic ensemble<br/>(A2A / tool calls)"]
    end

    GW["Governance Gateway · g8eg<br/>(Policy Decision Point)<br/>admits signed envelopes · owns PKI"]

    subgraph Fleet ["Sovereign hosts, platform-agnostic"]
        O1["Governed Operator · g8eo<br/>(Policy Execution Point)<br/>governs + executes locally"]
        D1[("Raw data + audit<br/>stay on host")]
        O1 --- D1
    end

    C1 -. "mTLS<br/>Streamable HTTP" .-> GW
    C2 --> GW
    O1 -. "outbound-only mTLS" .-> GW
```
