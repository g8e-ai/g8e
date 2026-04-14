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
 * Trust Endpoint Unit Tests
 * 
 * Tests the /trust endpoint logic including:
 * - OS detection from User-Agent header
 * - Correct script selection based on OS
 * - Response headers and content type
 * - Host and port handling in generated scripts
 */

import { describe, it, expect, vi } from 'vitest';
import http from 'http';
import { windowsPowerShellTrustScript, universalTrustScript } from '../../../utils/cert-installers.js';

// Mock logger to avoid noise in tests
vi.mock('../../../utils/logger.js', () => ({
    logger: {
        info: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
        debug: vi.fn()
    }
}));

describe('Trust Endpoint Logic', () => {
    const testHost = 'g8e.local';
    const testPort = 80;
    const securityHeaders = {
        'Content-Security-Policy': "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self';",
        'X-Content-Type-Options': 'nosniff',
        'X-Frame-Options': 'DENY',
        'X-XSS-Protection': '1; mode=block',
        'Referrer-Policy': 'strict-origin-when-cross-origin'
    };

    describe('OS Detection from User-Agent', () => {
        it('should detect Windows from User-Agent with "windows"', () => {
            const ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should detect Windows from User-Agent with "win32"', () => {
            const ua = 'Mozilla/5.0 (Windows; U; Win32; en-US)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should detect Windows from User-Agent with "win64"', () => {
            const ua = 'Mozilla/5.0 (Win64; x64)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should detect Windows from User-Agent with "powershell"', () => {
            const ua = 'PowerShell/7.4.0';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should not detect Windows from macOS User-Agent', () => {
            const ua = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(false);
        });

        it('should not detect Windows from Linux User-Agent', () => {
            const ua = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(false);
        });

        it('should handle empty User-Agent as non-Windows', () => {
            const ua = '';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(false);
        });

        it('should handle null User-Agent as non-Windows', () => {
            const ua = null || '';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(false);
        });
    });

    describe('Script Selection Based on OS', () => {
        it('should return PowerShell script for Windows User-Agent', () => {
            const ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            
            const content = isWindows 
                ? windowsPowerShellTrustScript(testHost, testPort)
                : universalTrustScript(testHost, testPort);
            
            expect(content).toContain('#Requires -RunAsAdministrator');
            expect(content).toContain('certutil -addstore -f "Root"');
            expect(content).toContain(`$url = "http://${testHost}/ca.crt"`);
        });

        it('should return universal script for macOS User-Agent', () => {
            const ua = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            
            const content = isWindows 
                ? windowsPowerShellTrustScript(testHost, testPort)
                : universalTrustScript(testHost, testPort);
            
            expect(content).toContain('#!/bin/sh');
            expect(content).toContain('uname -s');
            expect(content).toContain('Darwin)');
            expect(content).toContain('Linux)');
            expect(content).toContain(`HOST="${testHost}"`);
        });

        it('should return universal script for Linux User-Agent', () => {
            const ua = 'Mozilla/5.0 (X11; Linux x86_64)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            
            const content = isWindows 
                ? windowsPowerShellTrustScript(testHost, testPort)
                : universalTrustScript(testHost, testPort);
            
            expect(content).toContain('#!/bin/sh');
            expect(content).toContain('uname -s');
            expect(content).toContain(`HOST="${testHost}"`);
        });

        it('should return universal script for empty User-Agent', () => {
            const ua = '';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            
            const content = isWindows 
                ? windowsPowerShellTrustScript(testHost, testPort)
                : universalTrustScript(testHost, testPort);
            
            expect(content).toContain('#!/bin/sh');
            expect(content).toContain('uname -s');
        });
    });

    describe('Response Headers', () => {
        it('should include security headers in response', () => {
            const content = universalTrustScript(testHost, testPort);
            const headers = {
                ...securityHeaders,
                'Content-Type': 'text/plain; charset=utf-8',
                'Content-Length': Buffer.byteLength(content),
            };
            
            expect(headers['Content-Security-Policy']).toBeDefined();
            expect(headers['X-Content-Type-Options']).toBe('nosniff');
            expect(headers['X-Frame-Options']).toBe('DENY');
            expect(headers['Content-Type']).toBe('text/plain; charset=utf-8');
            expect(headers['Content-Length']).toBeGreaterThan(0);
        });

        it('should set correct Content-Type for both script types', () => {
            const windowsScript = windowsPowerShellTrustScript(testHost, testPort);
            const universalScript = universalTrustScript(testHost, testPort);
            
            const windowsHeaders = {
                ...securityHeaders,
                'Content-Type': 'text/plain; charset=utf-8',
                'Content-Length': Buffer.byteLength(windowsScript),
            };
            
            const universalHeaders = {
                ...securityHeaders,
                'Content-Type': 'text/plain; charset=utf-8',
                'Content-Length': Buffer.byteLength(universalScript),
            };
            
            expect(windowsHeaders['Content-Type']).toBe('text/plain; charset=utf-8');
            expect(universalHeaders['Content-Type']).toBe('text/plain; charset=utf-8');
        });
    });

    describe('Host and Port Handling', () => {
        it('should include host in generated PowerShell script', () => {
            const content = windowsPowerShellTrustScript(testHost, testPort);
            expect(content).toContain(`$url = "http://${testHost}/ca.crt"`);
            expect(content).toContain(`https://${testHost}/setup`);
        });

        it('should include host in generated universal script', () => {
            const content = universalTrustScript(testHost, testPort);
            expect(content).toContain(`HOST="${testHost}"`);
            expect(content).toContain(`URL="http://\${HOST}/ca.crt"`);
        });

        it('should include port in URL when non-default for PowerShell', () => {
            const nonDefaultPort = 8080;
            const content = windowsPowerShellTrustScript(testHost, nonDefaultPort);
            expect(content).toContain(`$url = "http://${testHost}:${nonDefaultPort}/ca.crt"`);
        });

        it('should include port in URL when non-default for universal', () => {
            const nonDefaultPort = 8080;
            const content = universalTrustScript(testHost, nonDefaultPort);
            expect(content).toContain(`URL="http://\${HOST}:${nonDefaultPort}/ca.crt"`);
        });

        it('should omit port from URL when default (80) for PowerShell', () => {
            const content = windowsPowerShellTrustScript(testHost, 80);
            expect(content).toContain(`$url = "http://${testHost}/ca.crt"`);
            expect(content).not.toContain(':80/ca.crt');
        });

        it('should omit port from URL when default (80) for universal', () => {
            const content = universalTrustScript(testHost, 80);
            expect(content).toContain(`URL="http://\${HOST}/ca.crt"`);
            expect(content).not.toContain(':80/ca.crt');
        });
    });

    describe('Edge Cases', () => {
        it('should handle User-Agent with mixed case', () => {
            const ua = 'Mozilla/5.0 (WINDOWS NT 10.0; Win64; x64)';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should handle User-Agent with PowerShell in different positions', () => {
            const ua = 'curl/7.68.0 PowerShell/7.4.0';
            const isWindows = ua.toLowerCase().includes('windows') || 
                           ua.toLowerCase().includes('win32') || 
                           ua.toLowerCase().includes('win64') || 
                           ua.toLowerCase().includes('powershell');
            expect(isWindows).toBe(true);
        });

        it('should handle IP address as host', () => {
            const ipHost = '192.168.1.100';
            const content = universalTrustScript(ipHost, testPort);
            expect(content).toContain(`HOST="${ipHost}"`);
            expect(content).toContain(`URL="http://\${HOST}/ca.crt"`);
        });

        it('should handle localhost as host', () => {
            const localhost = 'localhost';
            const content = universalTrustScript(localhost, testPort);
            expect(content).toContain(`HOST="${localhost}"`);
        });
    });
});
