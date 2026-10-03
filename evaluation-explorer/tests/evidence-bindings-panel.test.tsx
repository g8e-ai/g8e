// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { EvidenceBindingsPanel } from '../src/components/EvidenceBindingsPanel';

describe('EvidenceBindingsPanel', () => {
  it('renders approved bindings as inert text and explains their limits', () => {
    render(<EvidenceBindingsPanel
      bindings={[{ sha256: 'a'.repeat(64), schema_ref: 'eval/v1', kind: 'evaluation_projection' }]}
      verification={{ provenance: 'bound', verifier_state: 'passed' }}
    />);
    expect(screen.getByText('Evaluation projection')).toBeInTheDocument();
    expect(screen.getByText('Bindings identify approved public evidence content. A hash does not make private artifacts accessible or independently verify application telemetry.')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });
});
