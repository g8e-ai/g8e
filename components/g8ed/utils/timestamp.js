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

/**
 * Timestamp utilities for g8ed.
 *
 * Canonical format: RFC3339Nano, UTC, Z suffix.
 * See shared/constants/timestamp.json for the platform-wide spec.
 *
 * JavaScript's Date.toISOString() emits millisecond precision (.mmmZ) which
 * is a valid RFC3339Nano string — all platform consumers accept it.
 *
 * In harmony with:
 *   g8ee: components/g8ee/app/utils/timestamp.py
 *   g8eo: components/g8eo/models/timestamp.go
 */

/**
 * Current UTC timestamp as an RFC3339Nano string (millisecond precision).
 * Equivalent to g8ee now_iso() and g8eo NowTimestamp().
 * @returns {string} e.g. "2026-03-03T19:05:00.123Z"
 */
export function nowISOString() {
    return new Date().toISOString();
}

/**
 * Parse an RFC3339 / ISO 8601 string and return a Date object.
 * Equivalent to g8ee parse_iso() and g8eo ParseTimestamp().
 * @param {string} s
 * @returns {Date}
 */
export function parseISOString(s) {
    return new Date(s);
}

/**
 * Check whether a Date (or RFC3339 string) is in the past.
 * Equivalent to g8ee is_expired() and g8eo (inline comparison).
 * @param {Date|string} dt
 * @returns {boolean}
 */
export function isExpired(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    return t < new Date();
}

/**
 * Add seconds to a Date, returning a new Date. Does not mutate the input.
 * Equivalent to g8ee add_seconds() and g8eo (time.Add).
 * @param {Date} date
 * @param {number} seconds
 * @returns {Date}
 */
export function addSeconds(date, seconds) {
    return new Date(date.getTime() + seconds * 1000);
}

/**
 * Add minutes to a Date, returning a new Date. Does not mutate the input.
 * Equivalent to g8ee add_minutes().
 * @param {Date} date
 * @param {number} minutes
 * @returns {Date}
 */
export function addMinutes(date, minutes) {
    return new Date(date.getTime() + minutes * 60 * 1000);
}

/**
 * Add hours to a Date, returning a new Date. Does not mutate the input.
 * Equivalent to g8ee add_hours().
 * @param {Date} date
 * @param {number} hours
 * @returns {Date}
 */
export function addHours(date, hours) {
    return new Date(date.getTime() + hours * 60 * 60 * 1000);
}

/**
 * Add days to a Date, returning a new Date. Does not mutate the input.
 * Equivalent to g8ee add_days().
 * @param {Date} date
 * @param {number} days
 * @returns {Date}
 */
export function addDays(date, days) {
    return new Date(date.getTime() + days * 24 * 60 * 60 * 1000);
}

const _rtf = new Intl.RelativeTimeFormat('en', { numeric: 'always' });

function _relativeTime(elapsedSeconds) {
    const abs = Math.abs(elapsedSeconds);
    if (abs < 60)   return _rtf.format(-Math.trunc(elapsedSeconds), 'second');
    if (abs < 3600) return _rtf.format(-Math.trunc(elapsedSeconds / 60), 'minute');
    if (abs < 86400) return _rtf.format(-Math.trunc(elapsedSeconds / 3600), 'hour');
    if (abs < 2592000) return _rtf.format(-Math.trunc(elapsedSeconds / 86400), 'day');
    if (abs < 31536000) return _rtf.format(-Math.trunc(elapsedSeconds / 2592000), 'month');
    return _rtf.format(-Math.trunc(elapsedSeconds / 31536000), 'year');
}

/**
 * Human-readable relative time from a past Date or RFC3339 string to now.
 * Equivalent to g8ee time_ago().
 * @param {Date|string} dt
 * @returns {string} e.g. "5 minutes ago"
 */
export function timeAgo(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    const elapsed = (Date.now() - t.getTime()) / 1000;
    return _relativeTime(elapsed);
}

/**
 * Human-readable relative time from now to a future Date or RFC3339 string.
 * Returns 'in the past' when the date has already passed.
 * Equivalent to g8ee time_until().
 * @param {Date|string} dt
 * @returns {string} e.g. "in 5 minutes"
 */
export function timeUntil(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    const remaining = (t.getTime() - Date.now()) / 1000;
    if (remaining < 0) return 'in the past';
    return _relativeTime(-remaining);
}

const _dtf = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'UTC',
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
    hour12: false,
});

/**
 * Format a Date or RFC3339 string for display in UTC.
 * Equivalent to g8ee format_for_display().
 * @param {Date|string} dt
 * @returns {string} e.g. "2026-03-03 19:05:00 UTC"
 */
export function formatForDisplay(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    return _dtf.format(t).replace(',', '') + ' UTC';
}
