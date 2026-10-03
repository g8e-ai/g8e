// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AssignmentAuditProofRow, filterNonAuditEvidenceBindings } from '../src/components/AssignmentAuditProofRow';
import type { ProofCatalogEntry } from '../src/contract/types';
import { evalStore } from '../src/state/store';

vi.mock('../src/state/feed', () => ({
  loadRuntimeConfig: vi.fn().mockResolvedValue({ schema_version: '1.0.0', mirror_origin: 'http://127.0.0.1:8082' }),
}));

const sliceHash = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
const keyHash = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb';

function catalogEntry(sha256: string, filename: string, byteSize = 4096): ProofCatalogEntry {
  return {
    artifact_id: sha256,
    filename,
    media_type: sha256 === sliceHash ? 'application/vnd.sqlite3' : 'text/plain',
    byte_size: byteSize,
    sha256,
    classification: 'public_safe',
    campaign_id: 'campaign-1',
    generated_at: '2026-09-20T00:00:00Z',
    verification_command: 'g8e public verify-assignment --db assignment.db --vault-key assignment.vault.key',
    immutable_url: `/proofs/${sha256}`,
  };
}

const auditBindings = [
  { kind: 'assignment_audit_slice' as const, sha256: sliceHash, schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
  { kind: 'assignment_audit_vault_key' as const, sha256: keyHash, schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
];

describe('AssignmentAuditProofRow', () => {
  beforeEach(() => {
    evalStore.loadFixtures([], []);
    evalStore.loadProofCatalog([
      catalogEntry(sliceHash, 'assignment-1.db', 8192),
      catalogEntry(keyHash, 'assignment-1.vault.key', 65),
    ]);
  });

  it('renders downloads when bindings and catalog entries are present', async () => {
    render(<AssignmentAuditProofRow bindings={auditBindings} />);
    expect(await screen.findByRole('link', { name: 'Download SQLite' })).toHaveAttribute(
      'href',
      `http://127.0.0.1:8082/proofs/${sliceHash}`,
    );
    expect(screen.getByRole('link', { name: 'Download vault key' })).toHaveAttribute(
      'href',
      `http://127.0.0.1:8082/proofs/${keyHash}`,
    );
    expect(screen.getByText('8.0 KiB')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy verify command' })).toBeInTheDocument();
  });

  it('is absent when either audit binding is missing', () => {
    const { container } = render(<AssignmentAuditProofRow bindings={[auditBindings[0]!]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('is absent when catalog entries are missing even with bindings', () => {
    evalStore.loadProofCatalog([]);
    const { container } = render(<AssignmentAuditProofRow bindings={auditBindings} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe('filterNonAuditEvidenceBindings', () => {
  it('removes audit kinds and returns undefined when nothing remains', () => {
    expect(filterNonAuditEvidenceBindings(auditBindings)).toBeUndefined();
  });
});
