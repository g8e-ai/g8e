// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

let detectOS;
let detectArch;
let binaryName;
let deriveEnrollContext;
let buildCommands;
let platformLabel;
let initGettingStarted;

beforeEach(async () => {
    vi.resetModules();
    const mod = await import('@g8ed/public/js/components/getting-started.js');
    detectOS = mod.detectOS;
    detectArch = mod.detectArch;
    binaryName = mod.binaryName;
    deriveEnrollContext = mod.deriveEnrollContext;
    buildCommands = mod.buildCommands;
    platformLabel = mod.platformLabel;
    initGettingStarted = mod.initGettingStarted;
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

describe('GettingStarted [UNIT - jsdom]', () => {
    describe('detectOS', () => {
        it('returns windows for Win32 platform', () => {
            expect(detectOS('Win32')).toBe('windows');
        });

        it('returns darwin for MacIntel platform', () => {
            expect(detectOS('MacIntel')).toBe('darwin');
        });

        it('returns darwin for MacPPC platform', () => {
            expect(detectOS('MacPPC')).toBe('darwin');
        });

        it('returns linux for Linux x86_64 platform', () => {
            expect(detectOS('Linux x86_64')).toBe('linux');
        });

        it('returns linux as default for empty platform', () => {
            expect(detectOS('')).toBe('linux');
        });

        it('returns linux as default for undefined platform', () => {
            expect(detectOS(undefined)).toBe('linux');
        });
    });

    describe('detectArch', () => {
        it('returns arm64 for userAgent containing ARM', () => {
            expect(detectArch('Mozilla/5.0 (Linux; ARM)')).toBe('arm64');
        });

        it('returns arm64 for userAgent containing aarch64', () => {
            expect(detectArch('Mozilla/5.0 (Linux; aarch64)')).toBe('arm64');
        });

        it('returns 386 for userAgent containing i686', () => {
            expect(detectArch('Mozilla/5.0 (X11; Linux i686)')).toBe('386');
        });

        it('returns 386 for userAgent containing i386', () => {
            expect(detectArch('Mozilla/5.0 (X11; Linux i386)')).toBe('386');
        });

        it('returns amd64 for userAgent containing Win64 and WOW64', () => {
            expect(detectArch('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36')).toBe('amd64');
        });

        it('returns amd64 for userAgent containing x86_64', () => {
            expect(detectArch('Mozilla/5.0 (X11; Linux x86_64)')).toBe('amd64');
        });

        it('returns amd64 as default for empty userAgent', () => {
            expect(detectArch('')).toBe('amd64');
        });

        it('returns amd64 as default for undefined userAgent', () => {
            expect(detectArch(undefined)).toBe('amd64');
        });
    });

    describe('binaryName', () => {
        it('builds linux amd64 binary name without .exe', () => {
            expect(binaryName('linux', 'amd64')).toBe('g8e-linux-amd64');
        });

        it('builds darwin arm64 binary name without .exe', () => {
            expect(binaryName('darwin', 'arm64')).toBe('g8e-darwin-arm64');
        });

        it('builds windows amd64 binary name with .exe', () => {
            expect(binaryName('windows', 'amd64')).toBe('g8e-windows-amd64.exe');
        });

        it('builds windows 386 binary name with .exe', () => {
            expect(binaryName('windows', '386')).toBe('g8e-windows-386.exe');
        });
    });

    describe('deriveEnrollContext', () => {
        it('derives host, httpsPort, binary, and downloadUrl from a localhost:8443 gateway URL on linux amd64', () => {
            const ctx = deriveEnrollContext('https://localhost:8443', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            expect(ctx).toEqual({
                host: 'localhost',
                httpsPort: '8443',
                binary: 'g8e-linux-amd64',
                downloadUrl: 'http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64',
                os: 'linux',
                arch: 'amd64',
            });
        });

        it('uses default https port 8443 when the gateway URL omits a port', () => {
            const ctx = deriveEnrollContext('https://dev.example.com', 'MacIntel', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15');
            expect(ctx.httpsPort).toBe('8443');
            expect(ctx.host).toBe('dev.example.com');
            expect(ctx.binary).toBe('g8e-darwin-amd64');
        });

        it('derives a windows arm64 binary name and download URL', () => {
            const ctx = deriveEnrollContext('https://10.0.0.5:8443', 'Win32', 'Mozilla/5.0 (Windows NT 10.0; ARM)');
            expect(ctx.os).toBe('windows');
            expect(ctx.arch).toBe('arm64');
            expect(ctx.binary).toBe('g8e-windows-arm64.exe');
            expect(ctx.downloadUrl).toBe('http://10.0.0.5:8080/.well-known/g8e/bin/g8e-windows-arm64.exe');
            expect(ctx.httpsPort).toBe('8443');
        });

        it('preserves a non-default https port from the gateway URL', () => {
            const ctx = deriveEnrollContext('https://gateway.corp:9443', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            expect(ctx.httpsPort).toBe('9443');
            expect(ctx.host).toBe('gateway.corp');
        });

        it('returns null when gatewayUrl is empty string', () => {
            expect(deriveEnrollContext('', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)')).toBeNull();
        });

        it('returns null when gatewayUrl is undefined', () => {
            expect(deriveEnrollContext(undefined, 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)')).toBeNull();
        });

        it('returns null when gatewayUrl is unparseable', () => {
            expect(deriveEnrollContext('not a url', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)')).toBeNull();
        });
    });

    describe('buildCommands', () => {
        it('builds curl download command and ./g8e enroll command on linux', () => {
            const ctx = deriveEnrollContext('https://localhost:8443', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            const cmds = buildCommands(ctx);
            expect(cmds.downloadCmd).toBe('curl -fsSL http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64 -o g8e && chmod +x g8e');
            expect(cmds.enrollCmd).toBe('./g8e auth enroll -e localhost --port 8443');
        });

        it('builds curl download command on darwin', () => {
            const ctx = deriveEnrollContext('https://dev.example.com', 'MacIntel', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15)');
            const cmds = buildCommands(ctx);
            expect(cmds.downloadCmd).toBe('curl -fsSL http://dev.example.com:8080/.well-known/g8e/bin/g8e-darwin-amd64 -o g8e && chmod +x g8e');
            expect(cmds.enrollCmd).toBe('./g8e auth enroll -e dev.example.com --port 8443');
        });

        it('builds iwr download command and .\\g8e.exe enroll command on windows', () => {
            const ctx = deriveEnrollContext('https://10.0.0.5:8443', 'Win32', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)');
            const cmds = buildCommands(ctx);
            expect(cmds.downloadCmd).toBe('iwr -Uri http://10.0.0.5:8080/.well-known/g8e/bin/g8e-windows-amd64.exe -OutFile g8e-windows-amd64.exe');
            expect(cmds.enrollCmd).toBe('.\\g8e.exe auth enroll -e 10.0.0.5 --port 8443');
        });

        it('returns null when context is null', () => {
            expect(buildCommands(null)).toBeNull();
        });
    });

    describe('platformLabel', () => {
        it('returns "Linux x86_64" for linux amd64', () => {
            expect(platformLabel({ os: 'linux', arch: 'amd64' })).toBe('Linux x86_64');
        });

        it('returns "macOS ARM64" for darwin arm64', () => {
            expect(platformLabel({ os: 'darwin', arch: 'arm64' })).toBe('macOS ARM64');
        });

        it('returns "Windows x86_64" for windows amd64', () => {
            expect(platformLabel({ os: 'windows', arch: 'amd64' })).toBe('Windows x86_64');
        });

        it('returns "Windows i386" for windows 386', () => {
            expect(platformLabel({ os: 'windows', arch: '386' })).toBe('Windows i386');
        });
    });

    describe('initGettingStarted', () => {
        function setupDom() {
            document.body.innerHTML = `
                <div class="getting-started" id="getting-started" hidden>
                    <div class="getting-started-steps" id="getting-started-steps">
                        <span id="gs-platform-label">—</span>
                        <span id="gs-binary-url">—</span>
                        <code id="gs-download-cmd">—</code>
                        <code id="gs-enroll-cmd">—</code>
                        <button class="getting-started-copy-btn" data-copy-target="gs-download-cmd">
                            <span class="material-symbols-outlined">content_copy</span>
                        </button>
                        <button class="getting-started-copy-btn" data-copy-target="gs-enroll-cmd">
                            <span class="material-symbols-outlined">content_copy</span>
                        </button>
                    </div>
                    <p class="getting-started-degraded" id="getting-started-degraded" hidden>degraded</p>
                </div>`;
        }

        function setNavigator(platform, userAgent) {
            Object.defineProperty(navigator, 'platform', { value: platform, configurable: true });
            Object.defineProperty(navigator, 'userAgent', { value: userAgent, configurable: true });
        }

        it('populates platform label, binary URL, download and enroll commands for linux amd64', () => {
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            window.G8E_GATEWAY_URL = 'https://localhost:8443';

            initGettingStarted();

            expect(document.getElementById('getting-started').hidden).toBe(false);
            expect(document.getElementById('getting-started-steps').hidden).toBe(false);
            expect(document.getElementById('getting-started-degraded').hidden).toBe(true);
            expect(document.getElementById('gs-platform-label').textContent).toBe('Linux x86_64');
            expect(document.getElementById('gs-binary-url').textContent).toBe('http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64');
            expect(document.getElementById('gs-download-cmd').textContent).toBe('curl -fsSL http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64 -o g8e && chmod +x g8e');
            expect(document.getElementById('gs-enroll-cmd').textContent).toBe('./g8e auth enroll -e localhost --port 8443');
        });

        it('populates windows iwr command and .\\g8e.exe enroll command', () => {
            setupDom();
            setNavigator('Win32', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)');
            window.G8E_GATEWAY_URL = 'https://10.0.0.5:8443';

            initGettingStarted();

            expect(document.getElementById('gs-platform-label').textContent).toBe('Windows x86_64');
            expect(document.getElementById('gs-download-cmd').textContent).toBe('iwr -Uri http://10.0.0.5:8080/.well-known/g8e/bin/g8e-windows-amd64.exe -OutFile g8e-windows-amd64.exe');
            expect(document.getElementById('gs-enroll-cmd').textContent).toBe('.\\g8e.exe auth enroll -e 10.0.0.5 --port 8443');
        });

        it('shows the graceful-degradation message and hides steps when G8E_GATEWAY_URL is unset', () => {
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            delete window.G8E_GATEWAY_URL;

            initGettingStarted();

            expect(document.getElementById('getting-started').hidden).toBe(false);
            expect(document.getElementById('getting-started-steps').hidden).toBe(true);
            expect(document.getElementById('getting-started-degraded').hidden).toBe(false);
            expect(document.getElementById('gs-binary-url').textContent).toBe('—');
        });

        it('shows the graceful-degradation message when G8E_GATEWAY_URL is unparseable', () => {
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            window.G8E_GATEWAY_URL = 'not a url';

            initGettingStarted();

            expect(document.getElementById('getting-started-steps').hidden).toBe(true);
            expect(document.getElementById('getting-started-degraded').hidden).toBe(false);
        });

        it('is a no-op when the #getting-started block is absent', () => {
            document.body.innerHTML = '<div>no getting started here</div>';
            window.G8E_GATEWAY_URL = 'https://localhost:8443';
            expect(() => initGettingStarted()).not.toThrow();
        });

        it('copies the download command to the clipboard when the download copy button is clicked', async () => {
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            window.G8E_GATEWAY_URL = 'https://localhost:8443';
            const writeText = vi.fn().mockResolvedValue(undefined);
            Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });

            initGettingStarted();

            const btn = document.querySelector('.getting-started-copy-btn[data-copy-target="gs-download-cmd"]');
            btn.click();

            await new Promise((resolve) => setTimeout(resolve, 0));
            expect(writeText).toHaveBeenCalledOnce();
            expect(writeText).toHaveBeenCalledWith('curl -fsSL http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64 -o g8e && chmod +x g8e');
        });

        it('copies the enroll command to the clipboard when the enroll copy button is clicked', async () => {
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            window.G8E_GATEWAY_URL = 'https://localhost:8443';
            const writeText = vi.fn().mockResolvedValue(undefined);
            Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });

            initGettingStarted();

            const btn = document.querySelector('.getting-started-copy-btn[data-copy-target="gs-enroll-cmd"]');
            btn.click();

            await new Promise((resolve) => setTimeout(resolve, 0));
            expect(writeText).toHaveBeenCalledWith('./g8e auth enroll -e localhost --port 8443');
        });

        it('adds the copied class and swaps the icon to check after a successful copy', async () => {
            vi.useFakeTimers();
            setupDom();
            setNavigator('Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
            window.G8E_GATEWAY_URL = 'https://localhost:8443';
            const writeText = vi.fn().mockResolvedValue(undefined);
            Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });

            initGettingStarted();

            const btn = document.querySelector('.getting-started-copy-btn[data-copy-target="gs-enroll-cmd"]');
            const icon = btn.querySelector('.material-symbols-outlined');
            btn.click();
            await vi.advanceTimersByTimeAsync(0);

            expect(btn.classList.contains('copied')).toBe(true);
            expect(icon.textContent).toBe('check');

            await vi.advanceTimersByTimeAsync(1500);
            expect(btn.classList.contains('copied')).toBe(false);
            expect(icon.textContent).toBe('content_copy');
        });
    });
});
