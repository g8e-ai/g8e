// Legacy local development supervisor. Prefer gateway-owned spectator instead:
//   ./g8e gw start -f --public-spectator
// or docker compose up -d g8e-gateway
// which serves mirror 8081/8082 and explorer 5173 without Node/Vite.
//
// This script starts the mirror, optionally seeds fixture data in mock mode,
// optionally replays scripted live events, and runs Vite.
// All child processes use argument arrays (no shell interpolation) and are
// terminated on exit.
//
// Usage:
//   node dev.mjs --mode mock   # mirror + seed + replay + frontend
//   node dev.mjs --mode real   # mirror + frontend; eval launch remains explicit
//   node dev.mjs --mode mock --no-replay  # skip scripted live events
//   node dev.mjs --reset                  # reset disposable feed state first
//
// Environment:
//   G8E_BIN        path to the g8e binary (auto-detected if unset)
//   G8E_PUBLIC_PORT  public mirror port (default: 8082)
//   G8E_PRIVATE_PORT private mirror port (default: 8081)
//   G8E_VITE_PORT  Vite dev server port (default: 5173)

import { spawn, spawnSync } from 'node:child_process';
import { existsSync, rmSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const projectRoot = __dirname;

const PRIVATE_PORT = process.env.G8E_PRIVATE_PORT ?? '8081';
const PUBLIC_PORT = process.env.G8E_PUBLIC_PORT ?? '8082';
const VITE_PORT = process.env.G8E_VITE_PORT ?? '5173';

function parseArgs() {
  const argv = process.argv.slice(2);
  let mode = 'mock';
  let replay = true;
  let reset = false;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--mode' && i + 1 < argv.length) {
      const m = argv[++i];
      if (m !== 'mock' && m !== 'real') throw new Error(`invalid --mode: ${m}`);
      mode = m;
    } else if (argv[i] === '--no-replay') {
      replay = false;
    } else if (argv[i] === '--reset') {
      reset = true;
    }
  }
  return { mode, replay, reset };
}

function g8eBin() {
  const env = process.env.G8E_BIN;
  if (env) return env;
  let dir = projectRoot;
  for (let i = 0; i < 5; i++) {
    const candidate = resolve(dir, 'g8e');
    if (existsSync(candidate)) return candidate;
    dir = resolve(dir, '..');
  }
  throw new Error('g8e binary not found; set G8E_BIN to the full path');
}

function g8eCwd() {
  return resolve(g8eBin(), '..');
}

function checkPort(port) {
  const result = spawnSync('ss', ['-ltn', `sport = :${port}`], { encoding: 'utf8' });
  if (result.status !== 0) return false;
  return result.stdout.includes(`:${port}`);
}

function waitForPort(port, label, timeoutMs) {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + timeoutMs;
    const check = () => {
      if (checkPort(port)) {
        resolve();
        return;
      }
      if (Date.now() > deadline) {
        reject(new Error(`${label} on port ${port} did not start within ${timeoutMs}ms`));
        return;
      }
      setTimeout(check, 200);
    };
    check();
  });
}

function waitForPortFree(port, timeoutMs) {
  return new Promise((resolvePortFree, reject) => {
    const deadline = Date.now() + timeoutMs;
    const check = () => {
      if (!checkPort(port)) {
        resolvePortFree();
        return;
      }
      if (Date.now() > deadline) {
        reject(new Error(`port ${port} still in use after ${timeoutMs}ms`));
        return;
      }
      setTimeout(check, 200);
    };
    check();
  });
}

const children = [];

function spawnChild(label, cmd, args, cwd) {
  const proc = spawn(cmd, args, { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
  children.push({ proc, label });
  proc.stdout?.on('data', (data) => process.stdout.write(`[${label}] ${data}`));
  proc.stderr?.on('data', (data) => process.stderr.write(`[${label}] ${data}`));
  proc.on('exit', (code) => {
    console.log(`[${label}] exited with code ${code}`);
  });
  return proc;
}

function killAll() {
  for (const { proc, label } of children) {
    if (!proc.killed) {
      console.log(`Stopping ${label} (pid ${proc.pid})...`);
      proc.kill('SIGTERM');
    }
  }
  try {
    const bin = g8eBin();
    const cwd = g8eCwd();
    spawnSync(bin, ['eval', 'mirror', 'stop'], { cwd, encoding: 'utf8', stdio: 'inherit' });
  } catch {
    // Best effort during shutdown.
  }
}

process.on('SIGINT', () => {
  console.log('\nShutting down...');
  killAll();
  process.exit(0);
});
process.on('SIGTERM', () => {
  killAll();
  process.exit(0);
});
process.on('exit', () => {
  killAll();
});

async function main() {
  const { mode, replay, reset } = parseArgs();
  const bin = g8eBin();
  const cwd = g8eCwd();

  console.log('=== OpenDevOps.ai Local Dev Supervisor ===');
  console.log(`Mode: ${mode}`);
  console.log(`g8e:  ${bin}`);
  console.log(`cwd:  ${cwd}`);

  if (reset) {
    console.log('\n--- Resetting disposable local feed state ---');
    // Stop any running mirror so it doesn't hold stale state in memory.
    const mirrorRunning = checkPort(PRIVATE_PORT) || checkPort(PUBLIC_PORT);
    if (mirrorRunning) {
      console.log('Stopping existing mirror...');
      spawnSync(bin, ['eval', 'mirror', 'stop'], { cwd, encoding: 'utf8', stdio: 'inherit' });
      await waitForPortFree(PRIVATE_PORT, 10000);
      await waitForPortFree(PUBLIC_PORT, 10000);
    }
    const feedDir = resolve(cwd, '.g8e/public-feed');
    const mirrorDir = resolve(cwd, '.g8e/public-mirror');
    if (existsSync(feedDir)) rmSync(feedDir, { recursive: true, force: true });
    if (existsSync(mirrorDir)) rmSync(mirrorDir, { recursive: true, force: true });
    console.log('Feed state removed.');
    const initResult = spawnSync(bin, ['public', 'init', '--source-id', 'opendevops-local', '--mirror-origin', `http://127.0.0.1:${PRIVATE_PORT}`], { cwd, encoding: 'utf8', stdio: 'inherit' });
    if (initResult.status !== 0) {
      console.error('Feed init failed.');
      process.exit(1);
    }
  }

  const mirrorRunning = checkPort(PRIVATE_PORT) && checkPort(PUBLIC_PORT);
  if (!mirrorRunning) {
    console.log('\nStarting mirror...');
    spawnSync(bin, [
      'eval', 'mirror', 'run', '--daemon',
      '--listen', `127.0.0.1:${PRIVATE_PORT}`,
      '--public-listen', `127.0.0.1:${PUBLIC_PORT}`,
    ], { cwd, encoding: 'utf8', stdio: 'inherit' });
    await waitForPort(PUBLIC_PORT, 'mirror public', 10000);
    console.log(`Mirror is up on ${PRIVATE_PORT} (private) and ${PUBLIC_PORT} (public).`);
  } else {
    console.log('\nMirror already running.');
  }

  if (mode === 'mock') {
    console.log('\n--- Seeding fixture data ---');
    const seedResult = spawnSync('node', [resolve(__dirname, 'scripts/seed.mjs'), '--fixtures'], {
      cwd: projectRoot,
      encoding: 'utf8',
      stdio: 'inherit',
      env: { ...process.env, G8E_BIN: bin, G8E_CWD: cwd },
    });
    if (seedResult.status !== 0) {
      console.error('Fixture seed failed. Continuing with whatever data is available.');
    }
  }

  if (mode === 'mock' && replay) {
    console.log('\n--- Replaying scripted live events ---');
    const replayResult = spawnSync('node', [resolve(__dirname, 'scripts/replay.mjs'), '--interval', '800'], {
      cwd: projectRoot,
      encoding: 'utf8',
      stdio: 'inherit',
      env: { ...process.env, G8E_BIN: bin, G8E_CWD: cwd },
    });
    if (replayResult.status !== 0) {
      console.error('Replay failed. Frontend will still start.');
    }
  }

  console.log('\n--- Mirror health ---');
  spawnSync('node', [resolve(__dirname, 'scripts/mirror-health.mjs')], {
    cwd: projectRoot,
    encoding: 'utf8',
    stdio: 'inherit',
    env: { ...process.env, G8E_BIN: bin, G8E_CWD: cwd },
  });

  console.log('\n--- Starting Vite dev server ---');
  const viteBin = resolve(projectRoot, 'node_modules/.bin/vite');
  if (!existsSync(viteBin)) {
    console.error('Vite not found. Run npm install in the frontend project first.');
    killAll();
    process.exit(1);
  }
  spawnChild('vite', viteBin, [
    '--host', '127.0.0.1',
    '--port', VITE_PORT,
    '--strictPort',
  ], projectRoot);

  console.log(`\n=== Dev environment ready ===`);
  console.log(`Frontend:   http://127.0.0.1:${VITE_PORT}`);
  console.log(`Mirror:     http://127.0.0.1:${PUBLIC_PORT} (public read-only)`);
  console.log(`Private:    http://127.0.0.1:${PRIVATE_PORT} (auth required, not for browser)`);
  console.log(`\nPress Ctrl+C to stop all processes.`);

  await new Promise(() => {});
}

main().catch((err) => {
  console.error('Supervisor failed:', err);
  killAll();
  process.exit(1);
});
