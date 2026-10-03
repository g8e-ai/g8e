// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { NavDrawer } from '../src/components/NavDrawer';

describe('NavDrawer', () => {
  it('renders hamburger button and toggles dropdown drawer', () => {
    render(
      <MemoryRouter>
        <NavDrawer />
      </MemoryRouter>,
    );

    const toggleBtn = screen.getByRole('button', { name: /Open navigation menu/i });
    expect(toggleBtn).toBeInTheDocument();
    expect(toggleBtn).toHaveAttribute('aria-expanded', 'false');

    // Initially drawer is not visible
    expect(screen.queryByRole('menu', { name: /Primary navigation drawer/i })).not.toBeInTheDocument();

    // Click to open
    fireEvent.click(toggleBtn);
    expect(toggleBtn).toHaveAttribute('aria-expanded', 'true');

    const drawer = screen.getByRole('menu', { name: /Primary navigation drawer/i });
    expect(drawer).toBeInTheDocument();

    // Verify all 6 navigation links are present
    expect(screen.getByRole('menuitem', { name: /Live/i })).toHaveAttribute('href', '/');
    expect(screen.getByRole('menuitem', { name: /Evals/i })).toHaveAttribute('href', '/evaluations');
    expect(screen.getByRole('menuitem', { name: /Tasks/i })).toHaveAttribute('href', '/tasks');
    expect(screen.getByRole('menuitem', { name: /Models/i })).toHaveAttribute('href', '/models');
    expect(screen.getByRole('menuitem', { name: /Docs/i })).toHaveAttribute('href', '/methodology');
    expect(screen.getByRole('menuitem', { name: /About/i })).toHaveAttribute('href', '/about');

    // Click link closes drawer
    fireEvent.click(screen.getByRole('menuitem', { name: /Tasks/i }));
    expect(screen.queryByRole('menu', { name: /Primary navigation drawer/i })).not.toBeInTheDocument();
  });

  it('closes on Escape key press', () => {
    render(
      <MemoryRouter>
        <NavDrawer />
      </MemoryRouter>,
    );

    const toggleBtn = screen.getByRole('button', { name: /Open navigation menu/i });
    fireEvent.click(toggleBtn);
    expect(screen.getByRole('menu', { name: /Primary navigation drawer/i })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('menu', { name: /Primary navigation drawer/i })).not.toBeInTheDocument();
  });

  it('closes on close button click', () => {
    render(
      <MemoryRouter>
        <NavDrawer />
      </MemoryRouter>,
    );

    const toggleBtn = screen.getByRole('button', { name: /Open navigation menu/i });
    fireEvent.click(toggleBtn);
    expect(screen.getByRole('menu', { name: /Primary navigation drawer/i })).toBeInTheDocument();

    const closeBtn = screen.getByRole('button', { name: /Close navigation drawer/i });
    fireEvent.click(closeBtn);
    expect(screen.queryByRole('menu', { name: /Primary navigation drawer/i })).not.toBeInTheDocument();
  });
});
