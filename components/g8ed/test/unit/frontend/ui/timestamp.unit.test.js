// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { describe, it, expect } from 'vitest';
import {
    nowISOString,
    parseISOString,
    isExpired,
    addSeconds,
    addMinutes,
    addHours,
    addDays,
    timeAgo,
    timeUntil,
    formatForDisplay,
} from '@g8ed/public/js/utils/timestamp.js';
import { TimestampFormat } from '@g8ed/public/js/constants/timestamp-constants.js';

const RFC3339_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/;

describe('nowISOString', () => {

    it('returns a string', () => {
        expect(typeof nowISOString()).toBe('string');
    });

    it('matches RFC3339Nano pattern', () => {
        const result = nowISOString();
        expect(RFC3339_PATTERN.test(result)).toBe(true);
    });

    it('ends with Z suffix (UTC)', () => {
        expect(nowISOString().endsWith('Z')).toBe(true);
    });

    it('does not contain offset notation', () => {
        const result = nowISOString();
        expect(result).not.toContain('+00:00');
        expect(result).not.toContain('-00:00');
    });

    it('includes millisecond fractional seconds', () => {
        const result = nowISOString();
        expect(result).toContain('.');
        const frac = result.split('.')[1].replace('Z', '');
        expect(frac.length).toBe(3);
    });

    it('is a valid Date', () => {
        const result = nowISOString();
        const d = new Date(result);
        expect(isNaN(d.getTime())).toBe(false);
    });

    it('is within 1 second of current time', () => {
        const before = Date.now();
        const result = nowISOString();
        const after = Date.now();
        const ts = new Date(result).getTime();
        expect(ts).toBeGreaterThanOrEqual(before);
        expect(ts).toBeLessThanOrEqual(after + 1);
    });

    it('produces monotonically non-decreasing values', () => {
        const a = new Date(nowISOString()).getTime();
        const b = new Date(nowISOString()).getTime();
        expect(b).toBeGreaterThanOrEqual(a);
    });

});

describe('parseISOString', () => {

    it('returns a Date', () => {
        const result = parseISOString('2026-03-03T19:05:00.123Z');
        expect(result).toBeInstanceOf(Date);
    });

    it('parses Z-suffix RFC3339Nano string', () => {
        const result = parseISOString('2026-03-03T19:05:00.123456789Z');
        expect(result.getUTCFullYear()).toBe(2026);
        expect(result.getUTCMonth()).toBe(2);
        expect(result.getUTCDate()).toBe(3);
        expect(result.getUTCHours()).toBe(19);
    });

    it('parses millisecond-precision Z string (JS native format)', () => {
        const result = parseISOString('2026-03-03T19:05:00.123Z');
        expect(result.getUTCMilliseconds()).toBe(123);
    });

    it('parses string with no fractional seconds', () => {
        const result = parseISOString('2026-03-03T19:05:00Z');
        expect(result.getUTCSeconds()).toBe(0);
        expect(isNaN(result.getTime())).toBe(false);
    });

    it('parses offset notation and normalises to UTC', () => {
        const withOffset = parseISOString('2026-03-03T21:05:00+02:00');
        const utc = parseISOString('2026-03-03T19:05:00Z');
        expect(withOffset.getTime()).toBe(utc.getTime());
    });

    it('round-trips with nowISOString', () => {
        const s = nowISOString();
        const d = parseISOString(s);
        expect(isNaN(d.getTime())).toBe(false);
        expect(Math.abs(d.getTime() - new Date(s).getTime())).toBe(0);
    });

});

describe('isExpired', () => {

    it('returns true for a past date string', () => {
        const past = new Date(Date.now() - 5000).toISOString();
        expect(isExpired(past)).toBe(true);
    });

    it('returns false for a future date string', () => {
        const future = new Date(Date.now() + 60000).toISOString();
        expect(isExpired(future)).toBe(false);
    });

    it('returns true for a past Date object', () => {
        const past = new Date(Date.now() - 1000);
        expect(isExpired(past)).toBe(true);
    });

    it('returns false for a future Date object', () => {
        const future = new Date(Date.now() + 60000);
        expect(isExpired(future)).toBe(false);
    });

});

describe('addSeconds', () => {

    it('returns a Date', () => {
        expect(addSeconds(new Date(), 10)).toBeInstanceOf(Date);
    });

    it('adds positive seconds', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        expect(addSeconds(base, 90).getTime()).toBe(base.getTime() + 90_000);
    });

    it('subtracts with negative argument', () => {
        const base = new Date('2026-01-01T01:00:00.000Z');
        expect(addSeconds(base, -3600).toISOString()).toBe('2026-01-01T00:00:00.000Z');
    });

    it('does not mutate the input', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        const original = base.getTime();
        addSeconds(base, 999);
        expect(base.getTime()).toBe(original);
    });

});

describe('addMinutes', () => {

    it('returns a Date', () => {
        expect(addMinutes(new Date(), 5)).toBeInstanceOf(Date);
    });

    it('adds minutes correctly', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        expect(addMinutes(base, 30).getTime()).toBe(base.getTime() + 30 * 60 * 1000);
    });

    it('does not mutate the input', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        const original = base.getTime();
        addMinutes(base, 5);
        expect(base.getTime()).toBe(original);
    });

});

describe('addHours', () => {

    it('returns a Date', () => {
        expect(addHours(new Date(), 1)).toBeInstanceOf(Date);
    });

    it('adds hours correctly', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        expect(addHours(base, 2).getTime()).toBe(base.getTime() + 2 * 60 * 60 * 1000);
    });

    it('does not mutate the input', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        const original = base.getTime();
        addHours(base, 1);
        expect(base.getTime()).toBe(original);
    });

});

describe('addDays', () => {

    it('returns a Date', () => {
        expect(addDays(new Date(), 1)).toBeInstanceOf(Date);
    });

    it('adds days correctly', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        expect(addDays(base, 3).toISOString()).toBe('2026-01-04T00:00:00.000Z');
    });

    it('does not mutate the input', () => {
        const base = new Date('2026-01-01T00:00:00.000Z');
        const original = base.getTime();
        addDays(base, 7);
        expect(base.getTime()).toBe(original);
    });

});

describe('timeAgo', () => {

    it('returns a string', () => {
        expect(typeof timeAgo(new Date())).toBe('string');
    });

    it('accepts an ISO string', () => {
        const past = new Date(Date.now() - 5000).toISOString();
        expect(typeof timeAgo(past)).toBe('string');
    });

    it('returns seconds label for <60s ago', () => {
        const past = new Date(Date.now() - 30_000);
        expect(timeAgo(past)).toMatch(/seconds ago/);
    });

    it('returns minutes label for <60m ago', () => {
        const past = new Date(Date.now() - 5 * 60 * 1000);
        expect(timeAgo(past)).toMatch(/minutes ago/);
    });

    it('returns hours label for <24h ago', () => {
        const past = new Date(Date.now() - 3 * 60 * 60 * 1000);
        expect(timeAgo(past)).toMatch(/hours ago/);
    });

    it('returns days label for <30d ago', () => {
        const past = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000);
        expect(timeAgo(past)).toMatch(/days ago/);
    });

    it('returns months label for <365d ago', () => {
        const past = new Date(Date.now() - 60 * 24 * 60 * 60 * 1000);
        expect(timeAgo(past)).toMatch(/months? ago/);
    });

    it('returns years label for >=365d ago', () => {
        const past = new Date(Date.now() - 400 * 24 * 60 * 60 * 1000);
        expect(timeAgo(past)).toMatch(/years? ago/);
    });

    it('singular second', () => {
        const past = new Date(Date.now() - 1000);
        expect(timeAgo(past)).toBe('1 second ago');
    });

    it('singular minute', () => {
        const past = new Date(Date.now() - 60_000);
        expect(timeAgo(past)).toBe('1 minute ago');
    });

});

describe('timeUntil', () => {

    it('returns a string', () => {
        expect(typeof timeUntil(new Date(Date.now() + 60000))).toBe('string');
    });

    it('accepts an ISO string', () => {
        const future = new Date(Date.now() + 60000).toISOString();
        expect(typeof timeUntil(future)).toBe('string');
    });

    it('returns IN_THE_PAST for past dates', () => {
        const past = new Date(Date.now() - 1000);
        expect(timeUntil(past)).toBe(TimestampFormat.IN_THE_PAST);
    });

    it('returns seconds label for <60s', () => {
        const future = new Date(Date.now() + 30_000);
        expect(timeUntil(future)).toMatch(/in \d+ seconds/);
    });

    it('returns minutes label for <60m', () => {
        const future = new Date(Date.now() + 5 * 60 * 1000);
        expect(timeUntil(future)).toMatch(/in \d+ minutes/);
    });

    it('returns hours label for <24h', () => {
        const future = new Date(Date.now() + 3 * 60 * 60 * 1000);
        expect(timeUntil(future)).toMatch(/in \d+ hours/);
    });

    it('returns days label for longer durations', () => {
        const future = new Date(Date.now() + 2 * 24 * 60 * 60 * 1000);
        expect(timeUntil(future)).toMatch(/in \d+ day/);
    });

});

describe('formatForDisplay', () => {

    it('returns a string', () => {
        expect(typeof formatForDisplay(new Date())).toBe('string');
    });

    it('accepts an ISO string', () => {
        expect(typeof formatForDisplay('2026-03-03T19:05:00.000Z')).toBe('string');
    });

    it('formats to UTC display string', () => {
        expect(formatForDisplay(new Date('2026-03-03T19:05:00.000Z'))).toBe('2026-03-03 19:05:00 UTC');
    });

    it('pads single-digit months and days', () => {
        expect(formatForDisplay(new Date('2026-01-05T08:03:07.000Z'))).toBe('2026-01-05 08:03:07 UTC');
    });

    it('round-trips with parseISOString', () => {
        const s = '2026-06-15T12:30:45.000Z';
        expect(formatForDisplay(parseISOString(s))).toBe('2026-06-15 12:30:45 UTC');
    });

});

describe('canonical spec compliance', () => {

    it('nowISOString output is accepted by parseISOString without information loss', () => {
        const s = nowISOString();
        const d = parseISOString(s);
        expect(d.toISOString()).toBe(s);
    });

    it('nowISOString is lexicographically sortable in chronological order', () => {
        const timestamps = [];
        for (let i = 0; i < 5; i++) {
            timestamps.push(nowISOString());
        }
        const sorted = [...timestamps].sort();
        expect(sorted).toEqual(timestamps);
    });

    it('output conforms to shared/constants/timestamp.json spec (RFC3339Nano, UTC, Z)', () => {
        const s = nowISOString();
        expect(RFC3339_PATTERN.test(s)).toBe(true);
        expect(s.endsWith('Z')).toBe(true);
        expect(s.includes('T')).toBe(true);
    });

});
