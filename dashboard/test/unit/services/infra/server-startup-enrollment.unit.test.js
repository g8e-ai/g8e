// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Tier 1 unit tests for the server.js startup enrollment phase.
 *
 * Covers `runStartupEnrollment`, the function extracted from the
 * `isEntryPoint` block that resolves the dashboard's mTLS app identity
 * before `app.listen()`. The three paths exercised:
 *
 * - load-then-proceed: `loadIdentity()` succeeds; `enroll()` is not called.
 * - enroll-then-proceed: `loadIdentity()` throws `ConfigurationError`;
 *   `enroll()` succeeds.
 * - fail-closed: `loadIdentity()` throws a non-`ConfigurationError`, or
 *   `enroll()` throws; `onFatalError` is invoked and the function returns
 *   null.
 *
 * `AppEnrollmentService` is replaced with a stub. No filesystem, no network.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { runStartupEnrollment } from '../../../../server.js';
import { ConfigurationError } from '../../../../services/infra/app-enrollment-service.js';

function _stubIdentity(appId = 'spiffe://g8e.local/app/g8ed') {
    return {
        app_id: appId,
        cert_path: '/tmp/g8ed.crt',
        key_path: '/tmp/g8ed.key',
        ca_cert_path: '/tmp/hub-bundle.pem',
    };
}

function _stubService({ loadResult, enrollResult, loadThrows, enrollThrows } = {}) {
    const svc = {
        loadIdentity: vi.fn(),
        enroll: vi.fn(),
    };
    if (loadThrows) {
        svc.loadIdentity.mockRejectedValueOnce(loadThrows);
    } else {
        svc.loadIdentity.mockResolvedValueOnce(loadResult ?? _stubIdentity());
    }
    if (enrollThrows) {
        svc.enroll.mockRejectedValueOnce(enrollThrows);
    } else {
        svc.enroll.mockResolvedValueOnce(enrollResult ?? _stubIdentity());
    }
    return svc;
}

describe('runStartupEnrollment', () => {
    let _onFatalError;

    beforeEach(() => {
        _onFatalError = vi.fn();
    });

    it('proceeds when loadIdentity succeeds and does not call enroll', async () => {
        const identity = _stubIdentity('spiffe://g8e.local/app/g8ed');
        const svc = _stubService({ loadResult: identity });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(svc.loadIdentity).toHaveBeenCalledTimes(1);
        expect(svc.enroll).not.toHaveBeenCalled();
        expect(_onFatalError).not.toHaveBeenCalled();
        expect(result).toEqual(identity);
    });

    it('falls back to enroll when loadIdentity throws ConfigurationError', async () => {
        const enrolled = _stubIdentity('spiffe://g8e.local/app/g8ed');
        const svc = _stubService({
            loadThrows: new ConfigurationError('app cert not found'),
            enrollResult: enrolled,
        });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(svc.loadIdentity).toHaveBeenCalledTimes(1);
        expect(svc.enroll).toHaveBeenCalledTimes(1);
        expect(_onFatalError).not.toHaveBeenCalled();
        expect(result).toEqual(enrolled);
    });

    it('calls onFatalError and returns null when loadIdentity throws a non-ConfigurationError', async () => {
        const unexpected = new Error('filesystem blew up');
        const svc = _stubService({ loadThrows: unexpected });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(svc.loadIdentity).toHaveBeenCalledTimes(1);
        expect(svc.enroll).not.toHaveBeenCalled();
        expect(_onFatalError).toHaveBeenCalledTimes(1);
        expect(_onFatalError).toHaveBeenCalledWith(unexpected);
        expect(result).toBeNull();
    });

    it('calls onFatalError and returns null when enroll throws after a ConfigurationError load failure', async () => {
        const enrollErr = new ConfigurationError('enrollment rejected by gateway: HTTP 400');
        const svc = _stubService({
            loadThrows: new ConfigurationError('app cert not found'),
            enrollThrows: enrollErr,
        });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(svc.loadIdentity).toHaveBeenCalledTimes(1);
        expect(svc.enroll).toHaveBeenCalledTimes(1);
        expect(_onFatalError).toHaveBeenCalledTimes(1);
        expect(_onFatalError).toHaveBeenCalledWith(enrollErr);
        expect(result).toBeNull();
    });

    it('returns the loaded identity on the load path', async () => {
        const identity = _stubIdentity('spiffe://g8e.local/app/g8ed');
        const svc = _stubService({ loadResult: identity });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(result).toBe(identity);
    });

    it('returns the enrolled identity on the enroll path', async () => {
        const identity = _stubIdentity('spiffe://g8e.local/app/g8ed');
        const svc = _stubService({
            loadThrows: new ConfigurationError('near-expiry'),
            enrollResult: identity,
        });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(result).toBe(identity);
    });

    it('returns null and does not produce an identity when onFatalError is invoked', async () => {
        const svc = _stubService({
            loadThrows: new ConfigurationError('missing'),
            enrollThrows: new ConfigurationError('enrollment failed'),
        });

        const result = await runStartupEnrollment({ enrollmentService: svc, onFatalError: _onFatalError });

        expect(result).toBeNull();
    });
});
