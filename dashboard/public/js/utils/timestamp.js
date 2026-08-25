// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Frontend timestamp utilities.
 *
 * Mirrors components/g8ed/utils/timestamp.js for browser use.
 * The server-side file cannot be re-exported here because the relative
 * path resolves above the webroot when loaded as an ES module in the browser.
 *
 * Canonical format: RFC3339Nano, UTC, Z suffix.
 * See protocol/constants/timestamp.json for the platform-wide spec.
 */

import { TimestampFormat } from '../constants/timestamp-constants.js';

export function nowISOString() {
    return new Date().toISOString();
}

export function parseISOString(s) {
    return new Date(s);
}

export function isExpired(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    return t < new Date();
}

export function addSeconds(date, seconds) {
    return new Date(date.getTime() + seconds * 1000);
}

export function addMinutes(date, minutes) {
    return new Date(date.getTime() + minutes * 60 * 1000);
}

export function addHours(date, hours) {
    return new Date(date.getTime() + hours * 60 * 60 * 1000);
}

export function addDays(date, days) {
    return new Date(date.getTime() + days * 24 * 60 * 60 * 1000);
}

const _rtf = new Intl.RelativeTimeFormat(TimestampFormat.RTF_LOCALE, { numeric: TimestampFormat.RTF_NUMERIC });

function _relativeTime(elapsedSeconds) {
    const abs = Math.abs(elapsedSeconds);
    if (abs < 60)       return _rtf.format(-Math.trunc(elapsedSeconds), 'second');
    if (abs < 3600)     return _rtf.format(-Math.trunc(elapsedSeconds / 60), 'minute');
    if (abs < 86400)    return _rtf.format(-Math.trunc(elapsedSeconds / 3600), 'hour');
    if (abs < 2592000)  return _rtf.format(-Math.trunc(elapsedSeconds / 86400), 'day');
    if (abs < 31536000) return _rtf.format(-Math.trunc(elapsedSeconds / 2592000), 'month');
    return _rtf.format(-Math.trunc(elapsedSeconds / 31536000), 'year');
}

export function timeAgo(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    const elapsed = (Date.now() - t.getTime()) / 1000;
    return _relativeTime(elapsed);
}

export function timeUntil(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    const remaining = (t.getTime() - Date.now()) / 1000;
    if (remaining < 0) return TimestampFormat.IN_THE_PAST;
    return _relativeTime(-remaining);
}

const _dtf = new Intl.DateTimeFormat(TimestampFormat.DTF_LOCALE, {
    timeZone: TimestampFormat.DTF_TIMEZONE,
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
    hour12: false,
});

export function formatForDisplay(dt) {
    const t = typeof dt === 'string' ? new Date(dt) : dt;
    return _dtf.format(t).replace(',', '') + TimestampFormat.DISPLAY_SUFFIX;
}
