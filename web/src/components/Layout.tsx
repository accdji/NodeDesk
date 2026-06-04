import { Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { NavLink, useLocation } from 'react-router-dom';
import { ToastContainer } from './Toast';

const pageTitles: Record<string, string> = {
  '/projects': 'page.projects',
  '/history': 'page.history',
};

export function Topbar() {
  const { t } = useTranslation();
  const location = useLocation();
  const path = '/' + location.pathname.split('/')[1];
  const title = t(pageTitles[path] || 'app.title');
  const isRun = location.pathname.startsWith('/run/');

  return (
    <div className="topbar">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <h2>{isRun ? t('monitor.title') : title}</h2>
      </div>
      <div className="meta">
        <span className="dot" />
        <span>{t('app.running')}</span>
      </div>
    </div>
  );
}

export function Sidebar() {
  const { t } = useTranslation();

  return (
    <div className="sidebar">
      <div className="sidebar-brand">
        <div className="logo">
          <span>⚡</span> Workflow
        </div>
        <div className="ver">{t('app.version')}</div>
      </div>
      <div className="sidebar-nav">
        <NavLink to="/projects"
          className={({ isActive }) =>
            isActive || location.pathname.startsWith('/workflow/') || location.pathname.startsWith('/run/') ? 'active' : ''
          }>
          <span className="icon">📁</span>
          {t('nav.projects')}
        </NavLink>
        <NavLink to="/history" end
          className={({ isActive }) =>
            isActive || location.pathname.startsWith('/history/') ? 'active' : ''
          }>
          <span className="icon">📋</span>
          {t('nav.history')}
        </NavLink>
      </div>
      <div className="sidebar-footer">v1.0.0</div>
    </div>
  );
}

export function Layout() {
  return (
    <>
      <Sidebar />
      <div className="main">
        <Topbar />
        <div className="content">
          <Outlet />
        </div>
      </div>
      <ToastContainer />
    </>
  );
}
