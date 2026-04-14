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
import { windowsTrustScript, macosTrustScript, linuxTrustScript, g8eDeploy, universalTrustScript, windowsPowerShellTrustScript } from '../../../utils/cert-installers.js';

describe('cert-installers', () => {
    const testHost = 'g8e.local';

    describe('windowsTrustScript', () => {
        it('should generate a script with the correct host', () => {
            const script = windowsTrustScript(testHost);
            expect(script).toContain(`http://${testHost}/ca.crt`);
            expect(script).toContain(`https://${testHost}/setup`);
            expect(script).toContain('certutil -addstore -f "Root"');
            expect(script).toContain('cacls.exe');
        });

        it('should include escalation logic', () => {
            const script = windowsTrustScript(testHost);
            expect(script).toContain('runas');
            expect(script).toContain('getadmin.vbs');
        });

        it('should exit on curl failure', () => {
            const script = windowsTrustScript(testHost);
            const curlIdx = script.indexOf('curl -sSf');
            const errorIdx = script.indexOf('ERROR: Failed to download');
            const certutilIdx = script.indexOf('certutil -addstore');
            expect(curlIdx).toBeLessThan(errorIdx);
            expect(errorIdx).toBeLessThan(certutilIdx);
            expect(script).toContain('exit /B 1');
        });

        it('should exit on certutil failure', () => {
            const script = windowsTrustScript(testHost);
            expect(script).toContain('ERROR: Failed to trust the certificate');
        });

        it('should include port in URL when non-default', () => {
            const script = windowsTrustScript(testHost, 8080);
            expect(script).toContain(`http://${testHost}:8080/ca.crt`);
        });

        it('should omit port from URL when port is 80', () => {
            const script = windowsTrustScript(testHost, 80);
            expect(script).toContain(`http://${testHost}/ca.crt`);
            expect(script).not.toContain(':80/ca.crt');
        });

        it('should use LAN IP when provided', () => {
            const script = windowsTrustScript('10.0.0.2');
            expect(script).toContain('http://10.0.0.2/ca.crt');
            expect(script).toContain('https://10.0.0.2/setup');
        });
    });

    describe('macosTrustScript', () => {
        it('should generate a script with the correct host', () => {
            const script = macosTrustScript(testHost);
            expect(script).toContain(`HOST="${testHost}"`);
            expect(script).toContain('URL="http://${HOST}/ca.crt"');
            expect(script).toContain('https://$HOST/setup');
            expect(script).toContain('security add-trusted-cert');
        });

        it('should include sudo escalation', () => {
            const script = macosTrustScript(testHost);
            expect(script).toContain('exec sudo bash "$0" "$@"');
        });

        it('should include certificate cleanup logic', () => {
            const script = macosTrustScript(testHost);
            expect(script).toContain('security delete-certificate');
            expect(script).toContain('security find-certificate');
        });

        it('should exit on curl failure', () => {
            const script = macosTrustScript(testHost);
            expect(script).toContain('if ! curl -fsSL');
            expect(script).toContain('ERROR: Failed to download CA certificate');
            expect(script).toContain('exit 1');
        });

        it('should include port in URL when non-default', () => {
            const script = macosTrustScript(testHost, 8080);
            expect(script).toContain('URL="http://${HOST}:8080/ca.crt"');
        });
    });

    describe('linuxTrustScript', () => {
        it('should generate a script with the correct host', () => {
            const script = linuxTrustScript(testHost);
            expect(script).toContain(`HOST="${testHost}"`);
            expect(script).toContain('URL="http://${HOST}/ca.crt"');
            expect(script).toContain('https://$HOST/setup');
            expect(script).toContain('update-ca-certificates');
        });

        it('should include sudo escalation', () => {
            const script = linuxTrustScript(testHost);
            expect(script).toContain('exec sudo bash "$0" "$@"');
        });

        it('should include NSS database cleanup', () => {
            const script = linuxTrustScript(testHost);
            expect(script).toContain('certutil -D -d "sql:$db"');
            expect(script).toContain('$HOME/.pki/nssdb');
        });

        it('should exit on curl failure', () => {
            const script = linuxTrustScript(testHost);
            expect(script).toContain('if ! curl -fsSL');
            expect(script).toContain('ERROR: Failed to download CA certificate');
            expect(script).toContain('exit 1');
        });

        it('should include port in URL when non-default', () => {
            const script = linuxTrustScript(testHost, 8080);
            expect(script).toContain('URL="http://${HOST}:8080/ca.crt"');
        });
    });

    describe('universalTrustScript', () => {
        it('should generate a POSIX shell script with correct shebang', () => {
            const script = universalTrustScript(testHost);
            expect(script).toMatch(/^#!\/bin\/sh\n/);
        });

        it('should require root and show usage on non-root', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('id -u');
            expect(script).toContain('ERROR: This script must run as root.');
            expect(script).toContain('curl -fsSL http://${HOST}/trust | sudo sh');
        });

        it('should detect OS via uname -s', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('uname -s');
            expect(script).toContain('Darwin)');
            expect(script).toContain('Linux)');
        });

        it('should include macOS trust logic', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('security add-trusted-cert');
            expect(script).toContain('security delete-certificate');
            expect(script).toContain('/Library/Keychains/System.keychain');
        });

        it('should include Linux trust logic', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('update-ca-certificates');
            expect(script).toContain('/usr/local/share/ca-certificates/g8e-ca.crt');
        });

        it('should include Linux NSS database cleanup', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('certutil -D -d "sql:$db"');
            expect(script).toContain('$HOME/.pki/nssdb');
        });

        it('should download CA cert from the correct URL', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain(`HOST="${testHost}"`);
            expect(script).toContain('URL="http://${HOST}/ca.crt"');
        });

        it('should exit on curl failure', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('if ! curl -fsSL');
            expect(script).toContain('ERROR: Failed to download CA certificate');
        });

        it('should include port in URL when non-default', () => {
            const script = universalTrustScript(testHost, 8080);
            expect(script).toContain('URL="http://${HOST}:8080/ca.crt"');
            expect(script).toContain('curl -fsSL http://${HOST}:8080/trust | sudo sh');
        });

        it('should omit port from URL when port is 80', () => {
            const script = universalTrustScript(testHost, 80);
            expect(script).toContain('URL="http://${HOST}/ca.crt"');
            expect(script).not.toContain(':80/ca.crt');
        });

        it('should exit with error for unsupported OS', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('Unsupported OS');
        });

        it('should clean up temp cert file', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('rm -f "$CERT_FILE"');
        });

        it('should show success message with setup URL', () => {
            const script = universalTrustScript(testHost);
            expect(script).toContain('g8e CA certificate trusted successfully.');
            expect(script).toContain('https://$HOST/setup');
        });
    });

    describe('windowsPowerShellTrustScript', () => {
        it('should require admin privileges', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('#Requires -RunAsAdministrator');
        });

        it('should download CA cert from the correct URL', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain(`$url = "http://${testHost}/ca.crt"`);
        });

        it('should remove existing g8e certificates', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('Cert:\\LocalMachine\\Root');
            expect(script).toContain('*g8e*');
            expect(script).toContain('Remove-Item');
        });

        it('should trust via certutil', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('certutil -addstore -f "Root"');
        });

        it('should handle download failure', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('try {');
            expect(script).toContain('Invoke-WebRequest');
            expect(script).toContain('Failed to download CA certificate');
        });

        it('should handle certutil failure', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('$LASTEXITCODE -ne 0');
            expect(script).toContain('Failed to trust the certificate');
        });

        it('should clean up temp cert file', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('Remove-Item $certFile');
        });

        it('should include port in URL when non-default', () => {
            const script = windowsPowerShellTrustScript(testHost, 8080);
            expect(script).toContain(`$url = "http://${testHost}:8080/ca.crt"`);
        });

        it('should omit port from URL when port is 80', () => {
            const script = windowsPowerShellTrustScript(testHost, 80);
            expect(script).toContain(`$url = "http://${testHost}/ca.crt"`);
            expect(script).not.toContain(':80/ca.crt');
        });

        it('should show success message with setup URL', () => {
            const script = windowsPowerShellTrustScript(testHost);
            expect(script).toContain('g8e CA certificate trusted successfully.');
            expect(script).toContain(`https://${testHost}/setup`);
        });
    });

    describe('g8eDeploy', () => {
        it('should generate a POSIX shell script with correct shebang', () => {
            const script = g8eDeploy(testHost);
            expect(script).toMatch(/^#!\/bin\/sh\n/);
        });

        it('should bake the host into the script', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain(`G8E_HOST="${testHost}"`);
            expect(script).toContain(`G8E_HTTPS_HOST="${testHost}"`);
            expect(script).toContain(`G8E_HTTP_URL="http://${testHost}"`);
        });

        it('should include architecture detection for amd64, arm64, and 386', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('_arch=amd64');
            expect(script).toContain('_arch=arm64');
            expect(script).toContain('_arch=386');
            expect(script).toContain('uname -m');
        });

        it('should include Linux-only check', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('uname -s');
            expect(script).toContain('This script is for Linux');
        });

        it('should support both curl and wget', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('command -v curl');
            expect(script).toContain('command -v wget');
            expect(script).toContain('curl -fsSL');
            expect(script).toContain('wget -q');
        });

        it('should fetch CA over plain HTTP', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('$G8E_HTTP_URL/ca.crt');
            expect(script).toContain(`G8E_HTTP_URL="http://${testHost}"`);
            expect(script).toContain('Fetching platform CA certificate');
        });

        it('should download binary over HTTPS with CA and bearer auth', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('--cacert');
            expect(script).toContain('Authorization: Bearer');
            expect(script).toContain('/operator/download/linux/');
        });

        it('should accept token from positional arg or G8E_TOKEN env var', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('${1:-${G8E_TOKEN:-}}');
        });

        it('should prompt interactively when stdin is a terminal', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('[ -t 0 ]');
            expect(script).toContain('[ -t 1 ]');
            expect(script).toContain('Device link token:');
        });

        it('should error with usage when piped without token', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('Token required');
        });

        it('should clean up temp CA cert file', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('trap _cleanup EXIT INT TERM');
            expect(script).toContain('rm -f');
            expect(script).toContain('.g8e-ca-');
        });

        it('should exec the operator binary with device token and endpoint', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('exec ./g8e.operator');
            expect(script).toContain('--device-token');
            expect(script).toContain(`--endpoint`);
        });

        it('should omit port suffixes when using default ports', () => {
            const script = g8eDeploy(testHost, 443, 80);
            expect(script).toContain(`G8E_HTTPS_HOST="${testHost}"`);
            expect(script).toContain(`G8E_HTTP_URL="http://${testHost}"`);
            expect(script).not.toContain(':443');
            expect(script).not.toContain(':80');
            expect(script).not.toContain('--wss-port');
            expect(script).not.toContain('--http-port');
        });

        it('should include port suffixes when using non-default HTTPS port', () => {
            const script = g8eDeploy(testHost, 8443, 80);
            expect(script).toContain(`G8E_HTTPS_HOST="${testHost}:8443"`);
            expect(script).toContain('--wss-port 8443');
            expect(script).toContain('--http-port 8443');
        });

        it('should include port suffix when using non-default HTTP port', () => {
            const script = g8eDeploy(testHost, 443, 8080);
            expect(script).toContain(`G8E_HTTP_URL="http://${testHost}:8080"`);
        });

        it('should include both port suffixes when both ports are non-default', () => {
            const script = g8eDeploy(testHost, 9443, 9080);
            expect(script).toContain(`G8E_HTTPS_HOST="${testHost}:9443"`);
            expect(script).toContain(`G8E_HTTP_URL="http://${testHost}:9080"`);
            expect(script).toContain('--wss-port 9443');
        });

        it('should work with an IP address host', () => {
            const script = g8eDeploy('10.0.0.2');
            expect(script).toContain('G8E_HOST="10.0.0.2"');
            expect(script).toContain('G8E_HTTP_URL="http://10.0.0.2"');
            expect(script).toContain('G8E_HTTPS_HOST="10.0.0.2"');
            expect(script).toContain('/operator/download/linux/');
        });

        it('should use set -u for unset variable protection', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('set -u');
        });

        it('should use wget --ca-certificate for TLS with wget fallback', () => {
            const script = g8eDeploy(testHost);
            expect(script).toContain('--ca-certificate=');
        });
    });
});
