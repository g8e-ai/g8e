// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';

interface NavItemConfig {
  to: string;
  label: string;
  description: string;
  badge?: string;
}

const NAV_ITEMS: NavItemConfig[] = [
  { to: '/', label: 'Live', description: 'Real-time campaign feed & telemetry', badge: 'Live' },
  { to: '/evaluations', label: 'Evals', description: 'Campaign runs & assignment outcomes' },
  { to: '/tasks', label: 'Tasks', description: 'Benchmark scenarios & task suite' },
  { to: '/models', label: 'Models', description: 'Evaluated models & scorecards' },
  { to: '/methodology', label: 'Docs', description: 'Methodology & governance specifications' },
  { to: '/about', label: 'About', description: 'Platform architecture & verification' },
];

export function NavDrawer() {
  const [isOpen, setIsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const location = useLocation();

  // Close on route change
  useEffect(() => {
    setIsOpen(false);
  }, [location.pathname]);

  // Close on outside click or Escape key
  useEffect(() => {
    if (!isOpen) return;

    function handleClickOutside(event: MouseEvent | TouchEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setIsOpen(false);
        buttonRef.current?.focus();
      }
    }

    document.addEventListener('pointerdown', handleClickOutside);
    document.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('pointerdown', handleClickOutside);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen]);

  return (
    <div className="nav-drawer-wrapper" ref={menuRef}>
      <button
        ref={buttonRef}
        type="button"
        className={`nav-hamburger-btn${isOpen ? ' is-active' : ''}`}
        onClick={() => setIsOpen((prev) => !prev)}
        aria-label={isOpen ? 'Close navigation menu' : 'Open navigation menu'}
        aria-expanded={isOpen}
        aria-haspopup="true"
        aria-controls="nav-dropdown-drawer"
        title="Navigation menu"
      >
        <span className="nav-hamburger-icon" aria-hidden="true">
          <span className="hamburger-line line-1" />
          <span className="hamburger-line line-2" />
          <span className="hamburger-line line-3" />
        </span>
      </button>

      {isOpen ? (
        <>
          <div
            className="nav-drawer-backdrop"
            onClick={() => setIsOpen(false)}
            aria-hidden="true"
          />
          <div
            id="nav-dropdown-drawer"
            className="nav-dropdown-drawer"
            role="menu"
            aria-label="Primary navigation drawer"
          >
            <div className="nav-drawer-header">
              <span className="nav-drawer-kicker">Navigation</span>
              <button
                type="button"
                className="nav-drawer-close-btn"
                onClick={() => setIsOpen(false)}
                aria-label="Close navigation drawer"
              >
                ✕
              </button>
            </div>
            <nav className="nav-drawer-items" aria-label="Dropdown navigation">
              {NAV_ITEMS.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === '/'}
                  className={({ isActive }) =>
                    `nav-drawer-item${isActive ? ' active' : ''}`
                  }
                  role="menuitem"
                  onClick={() => setIsOpen(false)}
                >
                  <div className="nav-drawer-item-content">
                    <div className="nav-drawer-item-title-row">
                      <span className="nav-drawer-item-label">{item.label}</span>
                      {item.badge ? (
                        <span className="nav-drawer-item-badge">{item.badge}</span>
                      ) : null}
                    </div>
                    <span className="nav-drawer-item-desc">{item.description}</span>
                  </div>
                  <span className="nav-drawer-item-arrow" aria-hidden="true">
                    →
                  </span>
                </NavLink>
              ))}
            </nav>
          </div>
        </>
      ) : null}
    </div>
  );
}
