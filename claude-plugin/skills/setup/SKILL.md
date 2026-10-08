---
name: setup
description: Walk the user through connecting the g8e plugin to their g8e Gateway. Use when the g8e MCP tools (mcp__plugin_g8e_g8e__*) are missing or failing to connect, or when the user asks how to set up g8e.
---

The user runs every step below in their own terminal, not through a tool. Native shell tools are denied in this session, and enrollment must not run through the governed path it sets up. Show the steps; do not try to run them.

Configured g8e root: `${user_config.g8e_root}`
Configured application identity: `${user_config.app_name}`

Give the user these steps, using the configured values above:

1. **Pick the g8e root.** g8e keeps its runtime state (credentials, trust bundle, audit data) in a `.g8e/` directory under the directory it runs from. Every command below runs from that same directory, and the plugin's `g8e_root` option must point at it.

2. **Start the Gateway** (skip if `g8e gw status` already reports it healthy):

   ```bash
   cd <g8e_root>
   g8e gw start --posture doctrine
   ```

   Posture is fixed for a running Gateway. `doctrine` enforces L1/L4/L5; `consensus`, `ratify`, and `notary` add L2 quorum and/or L3 human approval. `g8e gw start --help` lists the current choices.

3. **Enroll the human CLI identity.** The bridge uses it to receive L3 approval events.

   ```bash
   g8e auth enroll user
   ```

4. **Enroll the application identity Claude acts as**, then approve it as the owner in the g8e Console or with the command the enrollment prints:

   ```bash
   g8e auth enroll app <app_name>
   g8e auth enroll approve <request-id> --yes
   ```

5. **Set the plugin options** if they are wrong: `/config` in Claude Code, plugin `g8e`, set `g8e_root` and, if `g8e` is not on `PATH`, `g8e_binary`.

6. **Reconnect.** Run `/mcp` in Claude Code and reconnect the `plugin:g8e:g8e` server, or restart Claude Code. The server's log in `/mcp` shows the bridge's error if it still cannot connect.

If the user already launches Claude with `g8e mcp agent run claude`, that launcher performs steps 2 to 4 itself. Tell them to use the launcher or the plugin, not both in the same session: the plugin denies the launcher's `mcp__g8e__*` tools.
