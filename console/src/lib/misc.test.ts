// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { deployCommands } from '../features/operators/DeployPanel';
import { credentialPathId } from '../features/account/AccountView';
import { groupCases } from './cases';
import { parseFragment } from './fragment';
import { InvestigationStatus, type Investigation } from './types';

describe('parseFragment', () => {
  it('reads each deep-link intent', () => {
    expect(parseFragment('#enroll=1&token=abc')).toEqual({ enrollmentToken: 'abc' });
    expect(parseFragment('#recovery=r1')).toEqual({ recoveryToken: 'r1' });
    expect(parseFragment('#approve=tx')).toEqual({ approveTxHash: 'tx' });
    expect(parseFragment('#platform-enrollment=req')).toEqual({ platformEnrollmentId: 'req' });
    expect(parseFragment('#token=abc')).toEqual({});
    expect(parseFragment('')).toEqual({});
  });
});

const inv = (id: string, caseId: string, created: string, updated: string, title = 'T'): Investigation => ({
  id,
  case_id: caseId,
  case_title: title,
  user_id: 'u',
  status: InvestigationStatus.Open,
  created_at: created,
  updated_at: updated,
});

describe('groupCases', () => {
  it('groups investigations by case, newest case first, investigations oldest first', () => {
    const cases = groupCases([
      inv('i2', 'c1', '2026-01-02', '2026-01-05', 'Newer title'),
      inv('i1', 'c1', '2026-01-01', '2026-01-01', 'Old title'),
      inv('i3', 'c2', '2026-01-03', '2026-01-03'),
    ]);
    expect(cases.map((c) => c.id)).toEqual(['c1', 'c2']);
    expect(cases[0]!.investigations.map((i) => i.id)).toEqual(['i1', 'i2']);
    expect(cases[0]!.title).toBe('Newer title');
  });
});

describe('credentialPathId', () => {
  it('converts standard base64 to the unpadded base64url revoke form', () => {
    expect(credentialPathId('+/8A/w==')).toBe('-_8A_w');
  });
});

describe('deployCommands', () => {
  it('targets the plain-HTTP bootstrap port from the HTTPS console', () => {
    const c = deployCommands({ protocol: 'https:', hostname: 'gw.local', port: '8443', host: 'gw.local:8443' });
    expect(c.quickLinux).toBe('curl -fsSL http://gw.local:8080/g8e-deploy.sh | bash');
    expect(c.binary({ label: 'L', filename: 'g8e-linux-amd64', os: 'linux' })).toBe(
      'curl -fsSL http://gw.local:8080/.well-known/g8e/bin/g8e-linux-amd64 -o g8e && chmod +x g8e && ./g8e operator start -e gw.local',
    );
  });

  it('passes a non-default HTTPS port to the Operator', () => {
    const c = deployCommands({ protocol: 'https:', hostname: 'gw', port: '9443', host: 'gw:9443' });
    expect(c.binary({ label: 'W', filename: 'g8e-windows-amd64.exe', os: 'windows' })).toContain('operator start -e gw -p 9443');
  });
});
