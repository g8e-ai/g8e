// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { evalStore } from '../src/state/store';
import { AboutView } from '../src/views/AboutView';
import { G8E_CORE_DOCS, G8E_REPO_URL, PLATFORM_CONTACT_CALENDLY, PLATFORM_CONTACT_EMAIL, PLATFORM_CONTACT_LINKEDIN } from '../src/content/platform';

describe('AboutView', () => {
  beforeEach(() => {
    evalStore.setConnection('live');
    evalStore.setStreamConnection('connected');
  });

  it('presents the project purpose, condensed builder background, and engagement links', () => {
    render(
      <MemoryRouter>
        <AboutView />
      </MemoryRouter>,
    );

    const panel = within(screen.getByRole('region', { name: 'About this deployment' }));
    expect(panel.getByRole('heading', { name: 'About' })).toBeInTheDocument();
    expect(panel.getByText(/live window into/i)).toBeInTheDocument();
    expect(panel.getByText(/g8e governs its execution path/i)).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'The solo builder' })).toBeInTheDocument();
    expect(panel.getByText(/Danny Barbour/i)).toBeInTheDocument();
    expect(panel.getByText(/thirty years/i)).toBeInTheDocument();
    expect(panel.getByText(/Danny-as-Code/i)).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'Why I built it' })).toBeInTheDocument();
    expect(panel.getByRole('heading', { name: 'Let’s work together' })).toBeInTheDocument();
    expect(panel.getByText(/contract engagements/i)).toBeInTheDocument();
    expect(panel.getByText(/g8e licensing, consulting, contract engagements, or full-time W-2 roles/i)).toBeInTheDocument();
    expect(panel.getByRole('list', { name: 'Career highlights' })).toBeInTheDocument();
    expect(panel.getByRole('list', { name: 'g8e principles' })).toBeInTheDocument();
    expect(panel.getByRole('link', { name: 'About g8e' })).toHaveAttribute('href', G8E_CORE_DOCS.about);
    expect(panel.getByRole('link', { name: 'position paper' })).toHaveAttribute('href', G8E_CORE_DOCS.position);
    expect(panel.getByRole('link', { name: 'Connect on LinkedIn' })).toHaveAttribute('href', PLATFORM_CONTACT_LINKEDIN);
    expect(panel.getByRole('link', { name: PLATFORM_CONTACT_EMAIL })).toHaveAttribute('href', `mailto:${PLATFORM_CONTACT_EMAIL}`);
    expect(panel.getByRole('link', { name: 'Book a call with Calendly' })).toHaveAttribute('href', PLATFORM_CONTACT_CALENDLY);
    expect(panel.getByRole('link', { name: 'Explore the g8e Codebase on GitHub' })).toHaveAttribute('href', G8E_REPO_URL);
  });
});
