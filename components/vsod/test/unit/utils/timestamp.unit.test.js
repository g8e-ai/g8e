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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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
    formatForDisplay
} from '../../../utils/timestamp.js';

describe('timestamp utils', () => {
    const mockNow = new Date('2026-03-30T12:00:00Z');

    beforeEach(() => {
        vi.useFakeTimers();
        vi.setSystemTime(mockNow);
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    it('nowISOString should return current time in ISO format', () => {
        expect(nowISOString()).toBe(mockNow.toISOString());
    });

    it('parseISOString should return a Date object', () => {
        const iso = '2026-03-30T15:00:00.000Z';
        const date = parseISOString(iso);
        expect(date).toBeInstanceOf(Date);
        expect(date.toISOString()).toBe(iso);
    });

    describe('isExpired', () => {
        it('should return true for past dates', () => {
            const past = new Date(mockNow.getTime() - 1000);
            expect(isExpired(past)).toBe(true);
            expect(isExpired(past.toISOString())).toBe(true);
        });

        it('should return false for future dates', () => {
            const future = new Date(mockNow.getTime() + 1000);
            expect(isExpired(future)).toBe(false);
            expect(isExpired(future.toISOString())).toBe(false);
        });
    });

    describe('date arithmetic', () => {
        it('addSeconds should add seconds correctly', () => {
            const result = addSeconds(mockNow, 30);
            expect(result.getTime()).toBe(mockNow.getTime() + 30000);
        });

        it('addMinutes should add minutes correctly', () => {
            const result = addMinutes(mockNow, 5);
            expect(result.getTime()).toBe(mockNow.getTime() + 5 * 60 * 1000);
        });

        it('addHours should add hours correctly', () => {
            const result = addHours(mockNow, 2);
            expect(result.getTime()).toBe(mockNow.getTime() + 2 * 60 * 60 * 1000);
        });

        it('addDays should add days correctly', () => {
            const result = addDays(mockNow, 1);
            expect(result.getTime()).toBe(mockNow.getTime() + 24 * 60 * 60 * 1000);
        });
    });

    describe('relative time', () => {
        it('timeAgo should return human readable past time', () => {
            const fiveMinsAgo = new Date(mockNow.getTime() - 5 * 60 * 1000);
            expect(timeAgo(fiveMinsAgo)).toBe('5 minutes ago');
            
            const oneHourAgo = new Date(mockNow.getTime() - 60 * 60 * 1000);
            expect(timeAgo(oneHourAgo)).toBe('1 hour ago');
        });

        it('timeUntil should return human readable future time', () => {
            const inFiveMins = new Date(mockNow.getTime() + 5 * 60 * 1000);
            expect(timeUntil(inFiveMins)).toBe('in 5 minutes');
            
            const inPast = new Date(mockNow.getTime() - 1000);
            expect(timeUntil(inPast)).toBe('in the past');
        });
    });

    it('formatForDisplay should format correctly in UTC', () => {
        const date = new Date('2026-03-30T15:04:05Z');
        // Intl.DateTimeFormat 'en-CA' with UTC usually results in YYYY-MM-DD HH:MM:SS
        // Note: replace(',', '') in the utility handles variations
        const formatted = formatForDisplay(date);
        expect(formatted).toContain('2026-03-30');
        expect(formatted).toContain('15:04:05');
        expect(formatted).toContain('UTC');
    });
});
