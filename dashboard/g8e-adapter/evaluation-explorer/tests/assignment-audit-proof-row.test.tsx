// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AssignmentAuditProofRow, filterNonAuditEvidenceBindings } from '../src/components/AssignmentAuditProofRow';

vi.mock('../src/state/feed', () => ({
  loadRuntimeConfig: vi.fn().mockResolvedValue({ schema_version: '1.0.0', mirror_origin: 'http://127.0.0.1:8082' }),
}));

describe('AssignmentAuditProofRow', () => {
  it('renders downloads when both audit bindings are present', async () => {
    render(<AssignmentAuditProofRow bindings={[
      { kind: 'assignment_audit_slice', sha256: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
      { kind: 'assignment_audit_vault_key', sha256: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
    ]} />);
    expect(await screen.findByRole('link', { name: 'Download SQLite' })).toHaveAttribute(
      'href',
      'http://127.0.0.1:8082/proofs/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    );
    expect(screen.getByRole('link', { name: 'Download vault key' })).toHaveAttribute(
      'href',
      'http://127.0.0.1:8082/proofs/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
    );
    expect(screen.getByRole('button', { name: 'Copy verify command' })).toBeInTheDocument();
  });

  it('is absent when either audit binding is missing', () => {
    const { container } = render(<AssignmentAuditProofRow bindings={[
      { kind: 'assignment_audit_slice', sha256: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
    ]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe('filterNonAuditEvidenceBindings', () => {
  it('removes audit kinds and returns undefined when nothing remains', () => {
    expect(filterNonAuditEvidenceBindings([
      { kind: 'assignment_audit_slice', sha256: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
      { kind: 'assignment_audit_vault_key', sha256: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', schema_ref: 'g8e.eval.v1.PublicAssignmentAuditSlice' },
    ])).toBeUndefined();
  });
});
