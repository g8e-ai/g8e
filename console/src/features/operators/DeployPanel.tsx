// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState } from 'react';
import { CodeLine } from '../../components/ui';
import { Paths } from '../../lib/paths';

// Binary downloads and deploy scripts are served on the Gateway's plain-HTTP
// bootstrap port (default 8080), not the HTTPS console port (default 8443),
// so commands target an http:// origin on the same host.
const HTTP_BOOTSTRAP_PORT = '8080';
const DEFAULT_HTTPS_PORT = '8443';

interface Binary {
  label: string;
  filename: string;
  os: 'linux' | 'darwin' | 'windows';
  fips?: boolean;
}

export const OPERATOR_BINARIES: Binary[] = [
  { label: 'Linux x86_64', filename: 'g8e-linux-amd64', os: 'linux', fips: true },
  { label: 'Linux ARM64', filename: 'g8e-linux-arm64', os: 'linux' },
  { label: 'Linux i386', filename: 'g8e-linux-386', os: 'linux' },
  { label: 'macOS Apple Silicon', filename: 'g8e-darwin-arm64', os: 'darwin' },
  { label: 'macOS Intel', filename: 'g8e-darwin-amd64', os: 'darwin' },
  { label: 'Windows x86_64', filename: 'g8e-windows-amd64.exe', os: 'windows' },
  { label: 'Windows ARM64', filename: 'g8e-windows-arm64.exe', os: 'windows' },
];

export function deployCommands(loc: Pick<Location, 'hostname' | 'port' | 'protocol' | 'host'>) {
  const host = loc.hostname || '127.0.0.1';
  const httpOrigin = loc.protocol === 'http:' ? `http://${loc.host}` : `http://${host}:${HTTP_BOOTSTRAP_PORT}`;
  const portFlag = loc.port && loc.port !== DEFAULT_HTTPS_PORT && loc.port !== HTTP_BOOTSTRAP_PORT ? ` -p ${loc.port}` : '';
  const quickLinux = `curl -fsSL ${httpOrigin}${Paths.deployScriptLinux} | bash`;
  const quickWindows = `iwr ${httpOrigin}${Paths.deployScriptWindows} | iex`;
  const binary = (b: Binary) => {
    const url = `${httpOrigin}${Paths.operatorBinary(b.filename)}`;
    if (b.os === 'windows') {
      return `Invoke-WebRequest -Uri "${url}" -OutFile "g8e.exe"; .\\g8e.exe operator start -e ${host}${portFlag}`;
    }
    return `curl -fsSL ${url} -o g8e && chmod +x g8e && ./g8e operator start -e ${host}${portFlag}`;
  };
  return { httpOrigin, quickLinux, quickWindows, binary };
}

export function DeployPanel() {
  const cmds = deployCommands(window.location);
  const [selected, setSelected] = useState(OPERATOR_BINARIES[0]!.filename);
  const bin = OPERATOR_BINARIES.find((b) => b.filename === selected) ?? OPERATOR_BINARIES[0]!;

  return (
    <div className="card">
      <div className="card-head">
        <h2>Deploy an Operator</h2>
      </div>
      <p className="text-2" style={{ marginTop: 0 }}>
        Run one command on the target host. The Operator connects outbound to this Gateway over mTLS as a Policy Execution
        Point and appears in the inventory once it enrolls.
      </p>
      <div className="field">
        <label>Linux / macOS (auto-detect)</label>
        <CodeLine text={cmds.quickLinux} />
      </div>
      <div className="field">
        <label>Windows PowerShell (auto-detect)</label>
        <CodeLine text={cmds.quickWindows} />
      </div>
      <div className="field" style={{ marginBottom: 0 }}>
        <label htmlFor="binary">Specific platform</label>
        <div className="row" style={{ marginBottom: 8 }}>
          <select id="binary" className="input" style={{ width: 'auto' }} value={selected} onChange={(e) => setSelected(e.target.value)}>
            {OPERATOR_BINARIES.map((b) => (
              <option key={b.filename} value={b.filename}>
                {b.label}
                {b.fips ? ' (FIPS 140-3)' : ''}
              </option>
            ))}
          </select>
          <a className="btn" href={`${cmds.httpOrigin}${Paths.operatorBinary(bin.filename)}`} download={bin.filename}>
            Download {bin.filename}
          </a>
        </div>
        <CodeLine text={cmds.binary(bin)} />
      </div>
    </div>
  );
}
