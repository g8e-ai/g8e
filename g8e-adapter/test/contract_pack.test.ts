// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Contract pack tests (Packet 8A). Verifies that every fixture parses through
// the generated validators, that regeneration is deterministic, that the
// stale-output check passes, and that no fixture contains fabricated
// operational numbers (CPU/RAM/throughput) or unsupported verification claims.

import { describe, it, expect } from 'vitest';
import { execSync } from 'node:child_process';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  isObserveBootstrapSnapshot,
  isAgentStatusUpdatedPayload,
  isRunStatusUpdatedPayload,
  isEvalSummary,
  isDownloadArtifact,
} from '../contract-pack/models';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ADAPTER_ROOT = join(__dirname, '..');
const CP_DIR = join(ADAPTER_ROOT, 'contract-pack');
const FIXTURES_DIR = join(CP_DIR, 'fixtures');

function loadFixture(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(FIXTURES_DIR, name), 'utf8'));
}

function loadManifest(): { files: Array<{ path: string; sha256: string }> } {
  return JSON.parse(readFileSync(join(CP_DIR, 'manifest.json'), 'utf8'));
}

describe('contract pack fixtures parse through generated validators', () => {
  it('bootstrap fixture validates as ObserveBootstrapSnapshot', () => {
    const fx = loadFixture('bootstrap.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
  });

  it('empty-evals fixture validates as ObserveBootstrapSnapshot with no evals', () => {
    const fx = loadFixture('empty-evals.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
    expect((fx.snapshot as { latest_evals: unknown[] }).latest_evals).toHaveLength(0);
  });

  it('partial-verification fixture validates and never claims verified', () => {
    const fx = loadFixture('partial-verification.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
    const evals = (fx.snapshot as { latest_evals: Array<{ verification_status: string }> }).latest_evals;
    expect(evals).toHaveLength(1);
    expect(isEvalSummary(evals[0])).toBe(true);
    expect(evals[0].verification_status).toBe('projection_validated');
    expect(evals[0].verification_status).not.toBe('verified');
  });

  it('stale-telemetry fixture validates with stale freshness labels', () => {
    const fx = loadFixture('stale-telemetry.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
    const overview = (fx.snapshot as { overview: { agents_running_freshness: string; tasks_in_queue_freshness: string } }).overview;
    expect(overview.agents_running_freshness).toBe('stale');
    expect(overview.tasks_in_queue_freshness).toBe('stale');
  });

  it('unavailable-metrics fixture omits resource/throughput measurements', () => {
    const fx = loadFixture('unavailable-metrics.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
    const measurements = (fx.snapshot as { measurements: Record<string, unknown> }).measurements;
    // Only schema_version is present; no cpu/ram/vram/disk/total_throughput.
    expect(measurements.cpu).toBeUndefined();
    expect(measurements.ram).toBeUndefined();
    expect(measurements.vram).toBeUndefined();
    expect(measurements.disk).toBeUndefined();
    expect(measurements.total_throughput).toBeUndefined();
  });

  it('download-availability fixture validates a public-safe artifact', () => {
    const fx = loadFixture('download-availability.json');
    expect(isObserveBootstrapSnapshot(fx.snapshot)).toBe(true);
    const downloads = (fx.snapshot as { downloads: Array<Record<string, unknown>> }).downloads;
    expect(downloads).toHaveLength(1);
    expect(isDownloadArtifact(downloads[0])).toBe(true);
    expect(downloads[0].privacy_classification).toBe('public_safe');
  });

  it('live-stream fixture events validate as typed payloads', () => {
    const fx = loadFixture('live-stream.json');
    const events = fx.events as Array<{ type: string; payload: Record<string, unknown> }>;
    expect(events).toHaveLength(2);
    expect(isAgentStatusUpdatedPayload(events[0].payload)).toBe(true);
    expect(isRunStatusUpdatedPayload(events[1].payload)).toBe(true);
  });

  it('reconnect fixture event validates as a typed agent payload', () => {
    const fx = loadFixture('reconnect.json');
    const events = fx.events as Array<{ type: string; payload: Record<string, unknown> }>;
    expect(isAgentStatusUpdatedPayload(events[0].payload)).toBe(true);
  });

  it('replay-gap fixture carries a truncated sentinel, not a typed payload', () => {
    const fx = loadFixture('replay-gap.json');
    const events = fx.events as Array<{ type: string; payload: Record<string, unknown> }>;
    expect(events[0].type).toBe('truncated');
    expect(events[0].payload.since_id).toBe(25);
  });

  it('unauthenticated fixture has no observe data', () => {
    const fx = loadFixture('unauthenticated.json');
    expect(fx.bootstrap_status).toBe(false);
    expect((fx.observe as { available: boolean }).available).toBe(false);
  });

  it('wrong-user-rejection fixture asserts cross-user isolation', () => {
    const fx = loadFixture('wrong-user-rejection.json');
    expect((fx.observe as { cross_user_isolation: boolean }).cross_user_isolation).toBe(true);
  });
});

describe('contract pack determinism and staleness', () => {
  it('stale-output check passes against committed outputs', () => {
    // The generator --check mode regenerates in memory and compares to the
    // committed files. It exits non-zero if any output is stale or missing.
    expect(() => execSync('node generator/gen-contract-pack.mjs --check', { cwd: ADAPTER_ROOT })).not.toThrow();
  });

  it('regeneration is byte-identical across runs', () => {
    // Run --check twice; both must pass and the manifest hashes are stable.
    execSync('node generator/gen-contract-pack.mjs --check', { cwd: ADAPTER_ROOT });
    const manifest1 = loadManifest();
    execSync('node generator/gen-contract-pack.mjs --check', { cwd: ADAPTER_ROOT });
    const manifest2 = loadManifest();
    expect(manifest2.files).toEqual(manifest1.files);
  });

  it('manifest lists every output with a SHA-256', () => {
    const manifest = loadManifest();
    const listed = new Set(manifest.files.map((f) => f.path));
    // Every non-manifest file in the contract pack must appear in the manifest.
    const diskFiles = readdirSync(CP_DIR)
      .filter((f) => f !== 'manifest.json' && !statSync(join(CP_DIR, f)).isDirectory());
    for (const f of diskFiles) expect(listed.has(f)).toBe(true);
    // Fixtures are listed under fixtures/.
    const fixtureFiles = readdirSync(FIXTURES_DIR).map((f) => `fixtures/${f}`);
    for (const f of fixtureFiles) expect(listed.has(f)).toBe(true);
    // Every hash is a 64-char hex string.
    for (const f of manifest.files) expect(f.sha256).toMatch(/^[0-9a-f]{64}$/);
  });
});

describe('contract pack contains no fabricated operational numbers', () => {
  it('no fixture embeds cpu/ram/vram/disk/throughput resource values', () => {
    const files = readdirSync(FIXTURES_DIR);
    for (const name of files) {
      const text = readFileSync(join(FIXTURES_DIR, name), 'utf8');
      // Resource measurements may only appear as absent keys; a present
      // ObservedMeasurement with source_component for host telemetry would be
      // fabrication in this release. The unavailable-metrics fixture omits
      // them entirely. Assert no fixture carries a host-telemetry source.
      expect(text).not.toMatch(/"source_component"\s*:\s*"[^"]*(cpu|ram|vram|disk|host|throughput)/i);
    }
  });

  it('no fixture claims verified eval verification', () => {
    const files = readdirSync(FIXTURES_DIR);
    for (const name of files) {
      const text = readFileSync(join(FIXTURES_DIR, name), 'utf8');
      expect(text).not.toMatch(/"verification_status"\s*:\s*"verified"/);
    }
  });
});
