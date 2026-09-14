import { useEffect } from 'react';
import { HashRouter, NavLink, Route, Routes, useNavigate } from 'react-router-dom';
import { useFeedStatus, useConnection, useStoreState, evalStore } from './state/store';
import { startFeed, stopFeed } from './state/startup';
import { ErrorBanner, FreshnessBadge } from './components/shared';
import { OverviewView } from './views/OverviewView';
import { ModelsView } from './views/ModelsView';
import { ModelDetailView } from './views/ModelDetailView';
import { EvaluationsView } from './views/EvaluationsView';
import { EvaluationDetailView } from './views/EvaluationDetailView';
import { AssignmentDetailView } from './views/AssignmentDetailView';
import { ComparisonView } from './views/ComparisonView';
import { MethodologyView } from './views/MethodologyView';

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
  const navigate = useNavigate();

  useEffect(() => {
    // Start the feed. The startup orchestrator retains only accepted mirror
    // data on transport errors. When runtime.json points at a live mirror,
    // bootstrap/history/snapshot/SSE connect.
    void startFeed();
    return () => stopFeed();
  }, []);

  return (
    <div className={`app-shell connection-${connection}`}>
      <a className="skip-link" href="#main">Skip to content</a>
      <header className="app-header">
        <div className="header-left">
          <button
            type="button"
            className="brand"
            onClick={() => navigate('/')}
            aria-label="OpenDevOps.AI evaluation explorer home"
          >
            OPEN<span>DEVOPS</span>.AI
          </button>
          <span className="header-tag">EVALUATION EXPLORER</span>
        </div>
        <nav className="app-nav" aria-label="Primary navigation">
          <NavItem to="/" label="Overview" />
          <NavItem to="/models" label="Models" />
          <NavItem to="/evaluations" label="Evaluations" />
          <NavItem to="/methodology" label="Methodology" />
        </nav>
        <div className="header-right">
          {feedStatus ? <FreshnessBadge freshness={feedStatus.freshness} /> : null}
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
          <Route path="/compare" element={<ComparisonView />} />
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
    <HashRouter>
      <Shell />
    </HashRouter>
  );
}
