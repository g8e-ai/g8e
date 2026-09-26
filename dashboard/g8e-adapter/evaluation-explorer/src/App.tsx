// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect } from 'react';
import { BrowserRouter, Navigate, NavLink, Route, Routes, useSearchParams } from 'react-router-dom';
import { useFeedStatus, useConnection, useStoreState, evalStore } from './state/store';
import { startFeed, stopFeed } from './state/startup';
import { ActiveCampaignBar } from './components/ActiveCampaignBar';
import { ErrorBanner, FreshnessBadge } from './components/shared';
import { AboutView } from './views/AboutView';
import { OverviewView } from './views/OverviewView';
import { ModelsView } from './views/ModelsView';
import { ModelDetailView } from './views/ModelDetailView';
import { EvaluationsView } from './views/EvaluationsView';
import { EvaluationDetailView } from './views/EvaluationDetailView';
import { AssignmentDetailView } from './views/AssignmentDetailView';
import { MethodologyView } from './views/MethodologyView';
import { TasksView } from './views/TasksView';
import { ThemeToggle } from './components/ThemeToggle';
import { G8E_REPO_URL, GITHUB_SPONSORS_URL } from './content/platform';

function CompareRedirect() {
  const [params] = useSearchParams();
  const dest = params.toString() ? `/models?${params.toString()}` : '/models';
  return <Navigate to={dest} replace />;
}

function NavItem({ to, label }: { to: string; label: string }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
    >
      {label}
    </NavLink>
  );
}

function Shell() {
  const feedStatus = useFeedStatus();
  const connection = useConnection();
  const errors = useStoreState((state) => state.errors);

  useEffect(() => {
    // Start the feed. The startup orchestrator retains only accepted mirror
    // data on transport errors. When runtime.json points at a live mirror,
    // bootstrap/history/snapshot/SSE connect.
    void startFeed();
    return () => stopFeed();
  }, []);

  return (
    <div className={`app-shell connection-${connection}`}>
      <a
        className="skip-link"
        href="#main"
        onClick={(event) => {
          event.preventDefault();
          document.getElementById('main')?.scrollIntoView({ behavior: 'smooth' });
        }}
      >
        Skip to content
      </a>
      <header className="app-header">
        <div className="header-primary">
          <nav className="app-nav" aria-label="Primary navigation">
            <NavItem to="/" label="Live" />
            <NavItem to="/evaluations" label="Evals" />
            <NavItem to="/tasks" label="Tasks" />
            <NavItem to="/models" label="Models" />
            <NavItem to="/methodology" label="Docs" />
            <NavItem to="/about" label="About" />
          </nav>
          <div className="header-campaign-slot">
            <ActiveCampaignBar />
          </div>
          <div className="header-utilities">
            <ThemeToggle />
            <a
              className="header-cta header-cta-github"
              href={G8E_REPO_URL}
              target="_blank"
              rel="noopener noreferrer"
            >
              <svg className="header-cta-icon" viewBox="0 0 16 16" aria-hidden="true">
                <path
                  fill="currentColor"
                  d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"
                />
              </svg>
              <span className="header-cta-text">View Source Code</span>
            </a>
            <a
              className="header-cta header-cta-sponsor"
              href={GITHUB_SPONSORS_URL}
              target="_blank"
              rel="noopener noreferrer"
            >
              Sponsor
            </a>
            {feedStatus ? <FreshnessBadge feedStatus={feedStatus} /> : null}
          </div>
        </div>
      </header>
      <ErrorBanner errors={errors} onDismiss={() => evalStore.clearErrors()} />
      <main id="main" className="app-main">
        <Routes>
          <Route path="/" element={<OverviewView />} />
          <Route path="/models" element={<ModelsView />} />
          <Route path="/models/:datasetId?/:variantId" element={<ModelDetailView />} />
          <Route path="/evaluations" element={<EvaluationsView />} />
          <Route path="/evaluations/:datasetId?/:runId" element={<EvaluationDetailView />} />
          <Route path="/evaluations/:datasetId?/:runId/assignments/:assignmentId" element={<AssignmentDetailView />} />
          <Route path="/failures" element={<Navigate to="/evaluations" replace />} />
          <Route path="/compare" element={<CompareRedirect />} />
          <Route path="/about" element={<AboutView />} />
          <Route path="/tasks" element={<TasksView />} />
          <Route path="/tasks/:taskId" element={<TasksView />} />
          <Route path="/methodology" element={<MethodologyView />} />
        </Routes>
      </main>
      <footer className="app-footer">
        <p>
          Anonymous public mirror read-only. No Gateway fallback, no credentials, no provider calls.
          Mirror availability is not verification evidence.
        </p>
      </footer>
    </div>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  );
}
