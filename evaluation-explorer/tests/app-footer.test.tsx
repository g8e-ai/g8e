// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AppFooter } from '../src/components/AppFooter';

describe('AppFooter', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('links mirror transport endpoints and site pages in one row', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ schema_version: '1.0.0', mirror_origin: 'https://mirror.example' }),
    }));
    render(<MemoryRouter><AppFooter /></MemoryRouter>);
    expect(await screen.findByRole('link', { name: /Live updates/ })).toHaveAttribute('href', 'https://mirror.example/stream');
    expect(screen.getByRole('link', { name: /History/ })).toHaveAttribute('href', 'https://mirror.example/history?cursor=0&limit=500');
    expect(screen.getByRole('link', { name: /Bootstrap/ })).toHaveAttribute('href', 'https://mirror.example/bootstrap');
    expect(screen.getByRole('link', { name: /Schema/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Methodology' })).toHaveAttribute('href', '/methodology');
    expect(screen.queryByRole('link', { name: /Proof manifest/ })).not.toBeInTheDocument();
  });
});
