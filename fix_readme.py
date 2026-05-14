import re

with open('/home/bob/g8e/README.md', 'r') as f:
    content = f.read()

# 1. Update the g8ee table row
content = re.sub(
    r'\| \*\*AI Engine \(g8ee\)\*\* \| Python \| \*\*Optional.\*\* A reference agentic reasoning engine that orchestrates the Tribunal, Warden, and Auditor workflows\. \|',
    r'| **AI Engine (g8ee)** | Python | **Optional.** A reference agentic reasoning engine that orchestrates the Tribunal and Auditor workflows. |',
    content
)

# 2. Remove g8ed from table
content = re.sub(
    r'\| \*\*Dashboard \(g8ed\)\*\* \| Node \| \*\*Optional.\*\* A reference web application for managing operators, viewing audit logs, and interacting with agents\. \|\n',
    '',
    content
)

# 3. Remove g8ed from mermaid Optional_Apps
content = re.sub(
    r'    subgraph Optional_Apps \[Optional Application Layer\]\n        g8ed\[g8ed<br>Dashboard\]\n        g8ee\[g8ee<br>AI Engine\]\n    end',
    r'    subgraph Optional_Apps [Optional Application Layer]\n        g8ee[g8ee<br>AI Engine]\n    end',
    content
)

# 4. Remove the g8ed section
content = re.sub(
    r'### g8ed \(Dashboard\)\nA stateless React/Node adapter that consumes the g8e protocol to provide a unified UI for fleet management, audit log visualization, and human-in-the-loop \(L3\) authorization\.\n\n',
    '',
    content
)

# 5. Fix Warden Execution text
content = re.sub(
    r'- \*\*Warden\*\* — Defensive coordination: command, error, and file risk classifiers applied before execution\. In the reference Engine, this orchestrates specialized sub-agents\.',
    r'- **Warden** — Defensive execution: The on-host execution boundary inside the Operator that executes transactions, enforces state-root freshness, and emits signed ActionReceipts.',
    content
)

# 6. Update Reference Application Stack description
content = re.sub(
    r'While the g8e substrate is self-contained and protocol-agnostic, the repository includes two optional reference applications that demonstrate the protocol\'s capabilities:',
    r'While the g8e substrate is self-contained and protocol-agnostic, the repository includes an optional reference application that demonstrates the protocol\'s capabilities:',
    content
)

# 7. Update Quick Start App command comments
content = re.sub(
    r'\./g8e platform start --with-apps  # Start Operator plus optional Engine',
    r'./g8e platform start --with-apps  # Start Operator plus optional applications',
    content
)

with open('/home/bob/g8e/README.md', 'w') as f:
    f.write(content)

