// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { evalStore } from '../src/state/store';
import { AboutView } from '../src/views/AboutView';
import { G8E_REPO_URL, PLATFORM_CONTACT_CALENDLY, PLATFORM_CONTACT_EMAIL } from '../src/content/platform';

describe('AboutView', () => {
  beforeEach(() => {
    evalStore.setConnection('live');
    evalStore.setStreamConnection('connected');
  });

  it('presents a cover-letter style pitch and contact links', () => {
    render(
      <MemoryRouter>
        <AboutView />
      </MemoryRouter>,
    );

    const panel = within(screen.getByRole('region', { name: 'About this deployment' }));
    expect(panel.getByRole('heading', { name: 'About' })).toBeInTheDocument();
    expect(panel.getByText(/live window into/i)).toBeInTheDocument();
    expect(panel.getByText(/cryptographic proofs published live/i)).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'Who Builds This — And Can Help You Ship Yours?' })).toBeInTheDocument();
    expect(panel.getByText(/Danny Barbour/i)).toBeInTheDocument();
    expect(panel.getByText(/thirty years/i)).toBeInTheDocument();
    expect(panel.getByText(/lead from the front/i)).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'What I Own & Deliver' })).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'Availability & Engagement' })).toBeInTheDocument();
    expect(panel.getByText(/contract engagements/i)).toBeInTheDocument();
    expect(panel.getByText(/hands-on leadership roles/i)).toBeInTheDocument();
    expect(panel.getByRole('list', { name: 'Career highlights' })).toBeInTheDocument();
    expect(panel.getByRole('list', { name: 'Areas of practice' })).toBeInTheDocument();
    expect(panel.getByRole('link', { name: PLATFORM_CONTACT_EMAIL })).toHaveAttribute('href', `mailto:${PLATFORM_CONTACT_EMAIL}`);
    expect(panel.getByRole('link', { name: 'Book a call with Calendly' })).toHaveAttribute('href', PLATFORM_CONTACT_CALENDLY);
    expect(panel.getByRole('link', { name: 'Explore the g8e Codebase on GitHub' })).toHaveAttribute('href', G8E_REPO_URL);
  });
});
