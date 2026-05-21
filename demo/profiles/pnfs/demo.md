# g8e pNFS Governance Demo

This demo illustrates how the g8e Operator (g8eo) governs runtime data access in a Parallel NFS (pNFS) environment. It demonstrates the "Gateway-first" architecture where all interactions with sensitive data must pass through the Operator's governance layers.

## Architecture

- **Metadata Server**: Manages file layouts and permissions in the pNFS cluster.
- **Data Server**: Stores the actual file data.
- **Client Node**: A simulated edge device that mounts the pNFS export and runs the g8e Operator.
- **g8e Operator**: The mandatory Gateway governing all tool calls and data access on the Client Node.

## Governance Scenario

In this demo, the pNFS mount at `/mnt/pnfs` contains sensitive operational data. We demonstrate:

1. **L1 Hard Gates**: Blocking access to restricted paths (e.g., `/mnt/pnfs/private/*`) via forbidden patterns.
2. **L2 Consensus**: Requiring tribunal verification for metadata mutations.
3. **L3 Authorization**: Human-in-the-loop approval for reading sensitive files.

## Setup

1. **Switch to the pnfs profile**:
   ```bash
   ./g8e demo profile switch pnfs
   ```

2. **Start the demo fleet**:
   ```bash
   ./g8e demo up
   ```

3. **Deploy the Operator**:
   ```bash
   ./g8e demo deploy -d dlk_your_token
   ```

## Demonstration Steps

### 1. Attempt Restricted Read (L1 Block)
Try to read a file that matches a forbidden pattern defined in the Operator's configuration. The Operator will fail-closed immediately.

### 2. Request Sensitive Data (L3 Approval)
Request access to a file in a "Governed" zone. The Operator will pause and request L3 approval from a human operator.

### 3. Metadata Mutation (L2 Consensus)
Attempt to change file permissions on the pNFS mount. This requires L2 consensus from the tribunal to ensure the mutation is authorized.
