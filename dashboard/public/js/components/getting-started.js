// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * GettingStarted - Landing-page guidance for remote workstation enrollment.
 *
 * The gateway serves its HTTPS API surface (:8443) with a certificate signed
 * by the gateway's own root CA. A browser on a remote workstation does not
 * trust that CA, so the browser-direct WebAuthn passkey ceremony fails with a
 * TLS error. The fix is `g8e auth enroll`, which installs the gateway root CA
 * into the OS trust store before opening the browser for the passkey. The
 * gateway also serves the g8e binary for download over plain HTTP (:8080).
 *
 * This module derives the binary download URL and the enroll command from
 * `window.G8E_GATEWAY_URL` (injected by /g8e-config.js), detects the visitor's
 * OS/arch from navigator, populates the static #getting-started block in
 * index.html, and wires copy-to-clipboard buttons. If the gateway URL is
 * unset or unparseable, the section shows a graceful-degradation message.
 *
 * The hostname used in the download URL and enroll command reflects the
 * origin the browser actually navigated to (`window.location.hostname`), not
 * the configured gateway URL's hostname. The dashboard and gateway share the
 * same host (different ports), so a user visiting http://dev.g8e.local:3000
 * gets commands pointing at dev.g8e.local rather than the configured
 * G8E_GATEWAY_URL host (which may default to localhost). The HTTPS port is
 * still taken from the gateway URL so non-default port configurations hold.
 */

/** Fixed HTTP discovery port — the gateway hardcodes 8080 for plain-HTTP binary/CA serving. */
const G8E_HTTP_PORT = '8080';

/** Default HTTPS/mTLS port when the gateway URL omits an explicit port. */
const DEFAULT_HTTPS_PORT = '8443';

/**
 * Detect the visitor OS from navigator.platform.
 * @param {string} platform - navigator.platform value.
 * @returns {'windows'|'darwin'|'linux'} Detected OS, defaulting to linux.
 */
export function detectOS(platform) {
    const p = (platform || '').toLowerCase();
    if (p.startsWith('win')) return 'windows';
    if (p.startsWith('mac')) return 'darwin';
    return 'linux';
}

/**
 * Detect the visitor CPU architecture from navigator.userAgent.
 * @param {string} userAgent - navigator.userAgent value.
 * @returns {'amd64'|'arm64'|'386'} Detected arch, defaulting to amd64 (most common).
 */
export function detectArch(userAgent) {
    const ua = (userAgent || '').toLowerCase();
    if (ua.includes('arm') || ua.includes('aarch64')) return 'arm64';
    if (ua.includes('i686') || ua.includes('i386') || ua.includes('wow32')) return '386';
    return 'amd64';
}

/**
 * Build the g8e binary filename for the given OS/arch.
 * Matches the pattern enforced by handleNodeBinaryDownload:
 * `g8e-(linux|darwin|windows)-(amd64|arm64|386)(.exe)?`.
 * @param {'windows'|'darwin'|'linux'} os
 * @param {'amd64'|'arm64'|'386'} arch
 * @returns {string} Binary filename.
 */
export function binaryName(os, arch) {
    const exe = os === 'windows' ? '.exe' : '';
    return `g8e-${os}-${arch}${exe}`;
}

/**
 * Derive the enrollment context (host, ports, binary, download URL) from the
 * gateway origin. The hostname is overridden by `browserHost` when provided
 * (non-empty), so the commands reflect the origin the browser actually
 * navigated to rather than the configured gateway URL's hostname. The HTTPS
 * port always comes from the gateway URL so non-default port configurations
 * are preserved. Returns null if the URL is unset or unparseable.
 * @param {string} gatewayUrl - window.G8E_GATEWAY_URL value.
 * @param {string} platform - navigator.platform value.
 * @param {string} userAgent - navigator.userAgent value.
 * @param {string} [browserHost] - window.location.hostname; overrides the
 *   gateway URL hostname when non-empty.
 * @returns {{host:string,httpsPort:string,binary:string,downloadUrl:string,os:string,arch:string}|null}
 */
export function deriveEnrollContext(gatewayUrl, platform, userAgent, browserHost) {
    if (!gatewayUrl) return null;
    let url;
    try {
        url = new URL(gatewayUrl);
    } catch {
        return null;
    }
    if (!url.hostname) return null;
    const host = (browserHost && browserHost.trim()) || url.hostname;
    const httpsPort = url.port || DEFAULT_HTTPS_PORT;
    const os = detectOS(platform);
    const arch = detectArch(userAgent);
    const binary = binaryName(os, arch);
    const downloadUrl = `http://${host}:${G8E_HTTP_PORT}/.well-known/g8e/bin/${binary}`;
    return { host, httpsPort, binary, downloadUrl, os, arch };
}

/**
 * Build the copy-pasteable download and enroll commands for the given context.
 * @param {{host:string,httpsPort:string,binary:string,downloadUrl:string,os:string,arch:string}|null} ctx
 * @returns {{downloadCmd:string,enrollCmd:string}|null}
 */
export function buildCommands(ctx) {
    if (!ctx) return null;
    const isWindows = ctx.os === 'windows';
    const downloadCmd = isWindows
        ? `iwr -Uri ${ctx.downloadUrl} -OutFile g8e.exe`
        : `curl -fsSL ${ctx.downloadUrl} -o g8e && chmod +x g8e`;
    const enrollCmd = isWindows
        ? `.\\g8e.exe auth enroll user -e ${ctx.host} --port ${ctx.httpsPort}`
        : `./g8e auth enroll user -e ${ctx.host} --port ${ctx.httpsPort}`;
    return { downloadCmd, enrollCmd };
}

/**
 * Human-readable platform label for the "Detected platform" line.
 * @param {{os:string,arch:string}} ctx
 * @returns {string}
 */
export function platformLabel(ctx) {
    const osNames = { windows: 'Windows', darwin: 'macOS', linux: 'Linux' };
    const archNames = { amd64: 'x86_64', arm64: 'ARM64', '386': 'i386' };
    return `${osNames[ctx.os] || ctx.os} ${archNames[ctx.arch] || ctx.arch}`;
}

const COPIED_RESET_MS = 1500;

/**
 * Copy text to the clipboard, preferring the async Clipboard API
 * (navigator.clipboard.writeText) and falling back to the legacy
 * document.execCommand('copy') path with a temporary textarea.
 *
 * The fallback is required because the dashboard is served over plain HTTP
 * on non-localhost hosts (e.g. http://dev.g8e.local:3000), which browsers
 * classify as a non-secure context. In non-secure contexts navigator.clipboard
 * is undefined, so without the fallback the copy buttons silently no-op.
 * @param {string} text - The text to copy.
 * @returns {Promise<boolean>} true if the copy succeeded, false otherwise.
 */
export async function copyToClipboard(text) {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
        try {
            await navigator.clipboard.writeText(text);
            return true;
        } catch {
            // Permission denied or other clipboard error — fall through to
            // the execCommand fallback rather than giving up silently.
        }
    }
    if (typeof document.execCommand === 'function') {
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.setAttribute('readonly', '');
        textarea.style.position = 'fixed';
        textarea.style.top = '0';
        textarea.style.left = '0';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        let ok = false;
        try {
            ok = document.execCommand('copy');
        } catch {
            ok = false;
        }
        document.body.removeChild(textarea);
        return ok;
    }
    return false;
}

/**
 * Wire a copy button to its target <code> element. Uses copyToClipboard, which
 * prefers navigator.clipboard and falls back to document.execCommand for
 * non-secure (plain-HTTP, non-localhost) contexts where the async Clipboard
 * API is unavailable.
 * @param {HTMLButtonElement} btn - Copy button with data-copy-target attribute.
 * @param {() => string} getText - Returns the text to copy (re-read each click).
 */
function wireCopyButton(btn, getText) {
    btn.addEventListener('click', async () => {
        const text = getText();
        if (!text) return;
        const ok = await copyToClipboard(text);
        if (!ok) return;
        btn.classList.add('copied');
        const icon = btn.querySelector('.material-symbols-outlined');
        const original = icon ? icon.textContent : null;
        if (icon) icon.textContent = 'check';
        setTimeout(() => {
            btn.classList.remove('copied');
            if (icon && original !== null) icon.textContent = original;
        }, COPIED_RESET_MS);
    });
}

/**
 * Populate the #getting-started block from window.G8E_GATEWAY_URL and wire
 * copy buttons. Shows the graceful-degradation message if the gateway URL is
 * unset or unparseable. Safe to call when the block is absent (no-op).
 */
export function initGettingStarted() {
    const root = document.getElementById('getting-started');
    if (!root) return;

    const gatewayUrl = typeof window !== 'undefined' ? window.G8E_GATEWAY_URL : null;
    const platform = typeof navigator !== 'undefined' ? navigator.platform : '';
    const userAgent = typeof navigator !== 'undefined' ? navigator.userAgent : '';
    const browserHost = typeof window !== 'undefined' && window.location ? window.location.hostname : '';

    const ctx = deriveEnrollContext(gatewayUrl, platform, userAgent, browserHost);
    const steps = document.getElementById('getting-started-steps');
    const degraded = document.getElementById('getting-started-degraded');

    if (!ctx) {
        if (steps) steps.hidden = true;
        if (degraded) degraded.hidden = false;
        root.hidden = false;
        return;
    }

    const cmds = buildCommands(ctx);

    const platformLabelEl = document.getElementById('gs-platform-label');
    if (platformLabelEl) platformLabelEl.textContent = platformLabel(ctx);

    const binaryUrlEl = document.getElementById('gs-binary-url');
    if (binaryUrlEl) binaryUrlEl.textContent = ctx.downloadUrl;

    const downloadCmdEl = document.getElementById('gs-download-cmd');
    if (downloadCmdEl) downloadCmdEl.textContent = cmds.downloadCmd;

    const enrollCmdEl = document.getElementById('gs-enroll-cmd');
    if (enrollCmdEl) enrollCmdEl.textContent = cmds.enrollCmd;

    if (steps) steps.hidden = false;
    if (degraded) degraded.hidden = true;
    root.hidden = false;

    const copyButtons = root.querySelectorAll('.getting-started-copy-btn');
    copyButtons.forEach((btn) => {
        const targetId = btn.getAttribute('data-copy-target');
        if (!targetId) return;
        const target = document.getElementById(targetId);
        if (!target) return;
        wireCopyButton(btn, () => target.textContent || '');
    });
}

if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', initGettingStarted);
}
