'use client';

import { usePathname } from 'next/navigation';
import { useState, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import Link from 'next/link';
import { audit, logout as doLogout, removeToken, getStoredUser, setStoredUser, getMe, setToken, workspaces, getBase, type AuditEntry, type Workspace } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';

const pageNames: Record<string, string> = {
  '/': 'Dashboard',
  '/deploy': 'Quick Deploy',
  '/connections': 'Connections',
  '/deployments': 'Deployments',
  '/settings': 'Settings',
  '/clusters': 'Kubernetes Clusters',
  '/jira': 'Jira Integration',
  '/workflows': 'Workflows',
  '/scorecards': 'Scorecards',
  '/plugins': 'Plugins',
  '/audit': 'Audit Log',
  '/entities': 'Entities',
  '/roles': 'Roles & Permissions',
  '/ai': 'AI Assistant',
  '/marketplace': 'Marketplace',
  '/workflows/designer': 'Workflow Designer',
  '/setup': 'Setup Wizard',
  '/services': 'Services',
  '/security': 'Security',
  '/policies': 'Policies',
  '/analytics': 'Analytics',
  '/observability': 'Observability',
  '/optimization': 'Optimization',
  '/compliance': 'Compliance',
  '/integrations': 'Integrations',
  '/environments': 'Environments',
  '/automation': 'Automation',
  '/import': 'Import',
  '/vault': 'Vault',
  '/graph': 'Knowledge Graph',
  '/docs': 'Documentation',
};

interface Notification {
  id: string;
  title: string;
  description: string;
  time: string;
  read: boolean;
  type: 'info' | 'success' | 'warning' | 'error';
  icon: string; // SVG path for the icon
}

// Human-readable action labels
const ACTION_LABELS: Record<string, string> = {
  login: 'Signed in',
  logout: 'Signed out',
  reset_password: 'Password reset',
  create: 'Created',
  update: 'Updated',
  delete: 'Deleted',
  deploy: 'Deployed',
  startup: 'System started',
  shutdown: 'System stopped',
  trigger: 'Triggered',
  install: 'Installed',
  uninstall: 'Uninstalled',
  enable: 'Enabled',
  disable: 'Disabled',
  sync: 'Synced',
  write: 'Updated',
  execute: 'Executed',
  promote: 'Promoted',
  rollback: 'Rolled back',
  cancel: 'Cancelled',
  restart: 'Restarted',
  scale: 'Scaled',
  suspend: 'Suspended',
  resume: 'Resumed',
  reconcile: 'Reconciled',
  grant: 'Granted',
  assign: 'Assigned',
  revoke: 'Revoked',
  configure: 'Configured',
  evaluate: 'Evaluated',
  api_create: 'Created via API',
  api_update: 'Updated via API',
  api_delete: 'Deleted via API',
  api_patch: 'Patched via API',
};

// Human-readable entity type labels
const ENTITY_LABELS: Record<string, string> = {
  user: 'User',
  role: 'Role',
  team: 'Team',
  service: 'Service',
  deployment: 'Deployment',
  cluster: 'Cluster',
  connection: 'Connection',
  entity: 'Entity',
  workflow: 'Workflow',
  plugin: 'Plugin',
  scorecard: 'Scorecard',
  setting: 'Setting',
  environment: 'Environment',
  docker_host: 'Docker Host',
  docker_service: 'Docker Service',
  helm_repository: 'Helm Repository',
  pipeline_source: 'Pipeline Source',
  pipeline_run: 'Pipeline Run',
  vault: 'Vault',
  credential: 'Credential',
  system: 'System',
  discovery: 'Discovery',
  marketplace: 'Marketplace',
  gitops: 'GitOps',
  jira: 'Jira',
  k8s_deployment: 'K8s Deployment',
  fluxcd_helmrelease: 'Helm Release',
};

// Specific SVG icon paths per action
const ACTION_ICONS: Record<string, string> = {
  login: 'M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3 0l3-3m0 0l-3-3m3 3H9',
  logout: 'M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3 0l3-3m0 0l-3-3m3 3H9',
  reset_password: 'M15.75 5.25a3 3 0 013 3m3-3a3 3 0 00-3-3m3 3h-3m-2.25 0a3 3 0 10-6 0m-2.25 0H6.75a3 3 0 00-3 3v.75a3 3 0 003 3h.75',
  deploy: 'M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699 2.7c-.103.021-.207.041-.311.06a15.09 15.09 0 01-2.448-2.448 14.9 14.9 0 01.06-.312m-2.24 2.39a4.493 4.493 0 00-1.757 4.306 4.493 4.493 0 004.306-1.758M16.5 9a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0z',
  create: 'M12 4.5v15m7.5-7.5h-15',
  delete: 'M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0',
  update: 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.037 9.348H9.03m-4.993 0l3.18-3.182a8.25 8.25 0 0113.803 3.7M20.015 4.356v4.992',
  install: 'M9 8.25H7.5a2.25 2.25 0 00-2.25 2.25v9a2.25 2.25 0 002.25 2.25h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25H15m0-3l-3-3m0 0l-3 3m3-3V15',
  enable: 'M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  disable: 'M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636',
  sync: 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.037 9.348H9.03m-4.993 0l3.18-3.182a8.25 8.25 0 0113.803 3.7M20.015 4.356v4.992',
  grant: 'M15.75 5.25a3 3 0 013 3m3-3a3 3 0 00-3-3m3 3h-3m-2.25 0a3 3 0 10-6 0m-2.25 0H6.75a3 3 0 00-3 3v.75a3 3 0 003 3h.75',
  assign: 'M19 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zM4 19.235v-.11a6.375 6.375 0 0112.75 0v.109A12.318 12.318 0 0110.374 21c-2.331 0-4.512-.645-6.374-1.766z',
  revoke: 'M15.75 5.25a3 3 0 013 3m3-3a3 3 0 00-3-3m3 3h-3m-2.25 0a3 3 0 10-6 0M4.5 19.5h12a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0016.5 4.5h-12a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z',
  rollback: 'M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3',
  restart: 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.037 9.348H9.03m-4.993 0l3.18-3.182a8.25 8.25 0 0113.803 3.7M20.015 4.356v4.992',
  scale: 'M3 7.5L7.5 3m0 0L12 7.5M7.5 3v13.5m13.5-4.5L16.5 16.5m0 0L12 12m4.5 4.5V3',
  configure: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
};

// Fallback icon paths by notification type
const FALLBACK_ICONS: Record<string, string> = {
  info: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  success: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
  warning: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z',
  error: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z',
};

const typeColors: Record<string, string> = {
  info: 'bg-blue-500/10 text-blue-400',
  success: 'bg-emerald-500/10 text-emerald-400',
  warning: 'bg-amber-500/10 text-amber-400',
  error: 'bg-red-500/10 text-red-400',
};

export default function TopBar() {
  const pathname = usePathname();
  const { isAdmin, hasPermission } = usePermission();
  const [apiStatus, setApiStatus] = useState<'checking' | 'online' | 'offline'>('checking');
  const [profileOpen, setProfileOpen] = useState(false);
  const [notifOpen, setNotifOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [storedUser, setStoredUser] = useState<ReturnType<typeof getStoredUser>>(null);
  const [theme, setTheme] = useState<'light' | 'dark' | 'system'>(() => {
    if (typeof window === 'undefined') return 'light';
    return (localStorage.getItem('pepa-theme') as 'light' | 'dark' | 'system') || 'light';
  });
  const [glassEnabled, setGlassEnabled] = useState(() => {
    if (typeof window === 'undefined') return true;
    const saved = localStorage.getItem('pepa-liquid-glass');
    return saved !== 'off';
  });

  // Workspace switcher
  const [wsList, setWsList] = useState<Workspace[]>([]);
  const [wsCurrent, setWsCurrent] = useState<string>('');
  const [wsOpen, setWsOpen] = useState(false);
  const [wsSwitching, setWsSwitching] = useState(false);
  const wsRef = useRef<HTMLDivElement>(null);
  const wsBtnRef = useRef<HTMLButtonElement>(null);
  const wsPortalRef = useRef<HTMLDivElement>(null);
  const [wsPos, setWsPos] = useState({ top: 0, left: 0 });
  const profileRef = useRef<HTMLDivElement>(null);
  const notifRef = useRef<HTMLDivElement>(null);
  const profileBtnRef = useRef<HTMLButtonElement>(null);
  const notifBtnRef = useRef<HTMLButtonElement>(null);
  const profilePortalRef = useRef<HTMLDivElement>(null);
  const notifPortalRef = useRef<HTMLDivElement>(null);
  const [profilePos, setProfilePos] = useState({ top: 0, right: 0 });
  const [notifPos, setNotifPos] = useState({ top: 0, right: 0 });

  // Fetch workspaces for switcher
  const fetchWorkspaces = useCallback(() => {
    workspaces.list().then(res => {
      setWsList(res.workspaces || []);
      setWsCurrent(res.current_workspace || '');
    }).catch(() => {});
  }, []);

  useEffect(() => {
    fetchWorkspaces();
    // Refresh when workspaces change (created/deleted from settings page)
    const handler = () => fetchWorkspaces();
    window.addEventListener('pepa:workspaces-changed', handler);
    // Also refresh on navigation (page focus)
    window.addEventListener('focus', fetchWorkspaces);
    return () => {
      window.removeEventListener('pepa:workspaces-changed', handler);
      window.removeEventListener('focus', fetchWorkspaces);
    };
  }, [fetchWorkspaces]);

  // Notifications — fetched from audit log
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [notifLoading, setNotifLoading] = useState(false);
  const unreadCount = notifications.filter(n => !n.read).length;

  // Map audit entries to notifications
  const mapAuditToNotification = useCallback((entry: AuditEntry): Notification => {
    const action = entry.action?.toLowerCase() || '';
    const entityType = entry.entity_type?.toLowerCase() || '';

    // Determine notification type from action
    let type: Notification['type'] = 'info';
    if (action.includes('delete') || action.includes('remove') || action.includes('revoke')) {
      type = 'warning';
    } else if (action.includes('create') || action.includes('deploy') || action.includes('install') || action.includes('enable') || action.includes('grant') || action.includes('assign') || action.includes('login')) {
      type = 'success';
    } else if (action.includes('fail') || action.includes('error') || action.includes('violation')) {
      type = 'error';
    }

    // Human-readable title
    const actionLabel = ACTION_LABELS[action] || entry.action || 'Activity';
    const entityLabel = ENTITY_LABELS[entityType] || entry.entity_type || '';
    const title = entityLabel ? `${actionLabel} ${entityLabel.toLowerCase()}` : actionLabel;

    // Description: show entity name from new_values if available, otherwise entity_id snippet
    let description = '';
    const vals = entry.new_values;
    if (vals && typeof vals === 'object') {
      const name = (vals.name || vals.email || vals.title || vals.username) as string;
      if (name) description = name;
    }
    if (!description && entry.entity_id) {
      const id = entry.entity_id;
      description = id.length > 12 ? `${id.slice(0, 8)}...` : id;
    }
    if (!description) {
      description = entityLabel || 'System event';
    }

    // Pick the best icon
    const icon = ACTION_ICONS[action] || FALLBACK_ICONS[type];

    // Format relative time
    const created = new Date(entry.created_at);
    const now = new Date();
    const diffSec = Math.floor((now.getTime() - created.getTime()) / 1000);
    let time = 'just now';
    if (diffSec >= 86400) {
      const days = Math.floor(diffSec / 86400);
      time = days === 1 ? 'yesterday' : `${days}d ago`;
    } else if (diffSec >= 3600) {
      const hours = Math.floor(diffSec / 3600);
      time = `${hours}h ago`;
    } else if (diffSec >= 60) {
      time = `${Math.floor(diffSec / 60)}m ago`;
    }

    return { id: entry.id, title, description, time, read: false, type, icon };
  }, []);

  // Load user: start from localStorage, then fetch fresh data from server
  useEffect(() => {
    let cancelled = false;
    const loadUser = () => {
      setStoredUser(getStoredUser());
      getMe().then(({ user, roles, permissions }) => {
        if (cancelled) return;
        const fresh = { id: user.id, email: user.email, name: user.name, roles, permissions: permissions || [] };
        setStoredUser(fresh);
      }).catch((err) => {
        if (cancelled) return;
        // Only clear auth on 401 (session expired/invalid).
        // Network errors or transient failures should NOT log the user out.
        if (err?.message === 'Not authenticated') {
          removeToken(); // clears both token and stored user from localStorage
        }
      });
    };
    loadUser();
    // Re-fetch user when auth state changes (e.g. after login on the /login page)
    window.addEventListener('pepa:auth-changed', loadUser);
    return () => { cancelled = true; window.removeEventListener('pepa:auth-changed', loadUser); };
  }, []);

  // Fetch recent audit entries as notifications — deferred until dropdown opens
  const [notifFetched, setNotifFetched] = useState(false);
  const fetchNotifications = useCallback(() => {
    if (notifFetched) return;
    setNotifLoading(true);
    audit.list({ per_page: '10', page: '1' })
      .then(res => {
        const items = (res.items || []).slice(0, 5);
        setNotifications(items.map(mapAuditToNotification));
      })
      .catch(() => {
        // Silently fail — notifications are non-critical
      })
      .finally(() => { setNotifLoading(false); setNotifFetched(true); });
  }, [mapAuditToNotification, notifFetched]);

  // API health check — deferred to avoid blocking initial paint
  useEffect(() => {
    const timer = setTimeout(() => {
      fetch(`${getBase()}/healthz`)
        .then(r => r.ok ? r.json().then(d => setApiStatus(d.status === 'ok' ? 'online' : 'offline')) : setApiStatus('offline'))
        .catch(() => setApiStatus('offline'));
    }, 2000);
    return () => clearTimeout(timer);
  }, []);

  // Close dropdowns on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      const clickedInsideProfile = profileRef.current?.contains(target) || profilePortalRef.current?.contains(target);
      const clickedInsideNotif = notifRef.current?.contains(target) || notifPortalRef.current?.contains(target);
      const clickedInsideWs = wsRef.current?.contains(target) || wsPortalRef.current?.contains(target);
      if (!clickedInsideProfile) setProfileOpen(false);
      if (!clickedInsideNotif) setNotifOpen(false);
      if (!clickedInsideWs) setWsOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  // Apply theme
  useEffect(() => {
    const root = document.documentElement;
    const resolvedTheme = theme === 'system'
      ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : theme;
    root.setAttribute('data-theme', resolvedTheme);
    root.style.colorScheme = resolvedTheme;
    root.style.background = resolvedTheme === 'dark' ? '#1a1d24' : '';
    localStorage.setItem('pepa-theme', theme);
  }, [theme]);

  // Apply liquid glass state
  useEffect(() => {
    document.documentElement.setAttribute('data-glass', glassEnabled ? 'on' : 'off');
    localStorage.setItem('pepa-liquid-glass', glassEnabled ? 'on' : 'off');
  }, [glassEnabled]);

  // Listen for system theme changes
  useEffect(() => {
    if (theme !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => {
      const d = mq.matches ? 'dark' : 'light';
      document.documentElement.setAttribute('data-theme', d);
      document.documentElement.style.colorScheme = d;
      document.documentElement.style.background = d === 'dark' ? '#1a1d24' : '';
    };
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }, [theme]);

  const markAllRead = () => {
    setNotifications(prev => prev.map(n => ({ ...n, read: true })));
  };

  const dismissNotification = (id: string) => {
    setNotifications(prev => prev.filter(n => n.id !== id));
  };

  // Build page title from path
  const segments = pathname.split('/').filter(Boolean);
  const pagePath = '/' + (segments[0] || '');
  const pageTitle = pageNames[pathname] || pageNames[pagePath] || (segments.length > 1 ? segments[segments.length - 1] : 'Dashboard');

  const handleSwitchWorkspace = async (id: string) => {
    if (id === wsCurrent) { setWsOpen(false); return; }
    setWsSwitching(true);
    try {
      const res = await workspaces.switch(id);
      setToken(res.token);
      setWsCurrent(id);
      setWsOpen(false);
      window.location.reload();
    } catch {
      setWsSwitching(false);
    }
  };

  const currentWsName = wsList.find(w => w.id === wsCurrent)?.name || 'Workspace';

  return (
    <header className={`h-[52px] border-b flex items-center justify-between px-4 md:px-6 shrink-0 relative z-[100] ${glassEnabled ? 'glass-surface border-[var(--border)]' : 'bg-[var(--surface)] border-[var(--border)]'}`}>
      {/* Left: Mobile hamburger + Page title */}
      <div className="flex items-center gap-3">
        <button
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          className="md:hidden p-1.5 -ml-1.5 text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
          aria-label="Toggle navigation"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d={mobileMenuOpen ? 'M6 18L18 6M6 6l12 12' : 'M4 6h16M4 12h16M4 18h16'} />
          </svg>
        </button>
        <h1 className="text-[15px] font-semibold text-[var(--text-primary)] truncate">{pageTitle}</h1>

        {/* Workspace Switcher */}
        {wsList.length > 0 && (
          <div ref={wsRef} className="relative hidden sm:block">
            <button
              ref={wsBtnRef}
              onClick={() => {
                if (wsBtnRef.current) {
                  const rect = wsBtnRef.current.getBoundingClientRect();
                  setWsPos({ top: rect.bottom + 8, left: rect.left });
                }
                setWsOpen(!wsOpen);
                setProfileOpen(false);
                setNotifOpen(false);
              }}
              className="flex items-center gap-1.5 px-2.5 py-1 text-[12px] font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
              title="Switch workspace"
            >
              <svg className="w-3.5 h-3.5 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 21h16.5M4.5 3h15M5.25 3v18m13.5-18v18M9 6.75h1.5m-1.5 3h1.5m-1.5 3h1.5m3-6H15m-1.5 3H15m-1.5 3H15M9 21v-3.375c0-.621.504-1.125 1.125-1.125h3.75c.621 0 1.125.504 1.125 1.125V21" />
              </svg>
              <span className="max-w-[100px] truncate">{currentWsName}</span>
              <svg className="w-3 h-3 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
              </svg>
            </button>

            {wsOpen && typeof document !== 'undefined' && createPortal(
              <div ref={wsPortalRef} className={`fixed w-[240px] rounded-xl shadow-xl border overflow-hidden z-[9999] ${glassEnabled ? 'glass-dropdown' : 'bg-[var(--surface)] border-[var(--border)]'}`}
                style={{ top: wsPos.top, left: wsPos.left }}
              >
                <div className="px-3 py-2 border-b border-[var(--border-light)]">
                  <p className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider">Workspaces</p>
                </div>
                <div className="py-1 max-h-[240px] overflow-y-auto">
                  {wsList.map(ws => (
                    <button
                      key={ws.id}
                      onClick={() => handleSwitchWorkspace(ws.id)}
                      disabled={wsSwitching}
                      className={`flex items-center gap-2.5 w-full px-3 py-2 text-[13px] transition-colors ${
                        ws.id === wsCurrent
                          ? 'bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                          : 'text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)]'
                      }`}
                    >
                      <div className={`w-6 h-6 rounded flex items-center justify-center text-[10px] font-semibold shrink-0 ${
                        ws.id === wsCurrent
                          ? 'bg-[var(--accent)] text-white'
                          : 'bg-[var(--bg)] text-[var(--text-tertiary)]'
                      }`}>
                        {ws.name.charAt(0).toUpperCase()}
                      </div>
                      <span className="truncate">{ws.name}</span>
                      {ws.id === wsCurrent && (
                        <svg className="w-3.5 h-3.5 ml-auto shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                        </svg>
                      )}
                    </button>
                  ))}
                </div>
                <div className="border-t border-[var(--border-light)] px-3 py-1.5">
                  <Link href="/settings/workspaces" onClick={() => setWsOpen(false)} className="text-[11px] text-[var(--accent)] hover:underline flex items-center gap-1 py-1">
                    Manage workspaces
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
                    </svg>
                  </Link>
                </div>
              </div>,
              document.body
            )}
          </div>
        )}
      </div>

      {/* Right side */}
      <div className="flex items-center gap-2 md:gap-5">
        {/* Search — opens Command Palette */}
        <button
          onClick={() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))}
          className="p-1.5 text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
          aria-label="Search (Cmd+K)"
          title="Search (⌘K)"
        >
          <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </button>

        {/* API Status — hidden on mobile */}
        <div className="hidden sm:flex items-center gap-2">
          <div className="relative">
            <span className={`block w-[7px] h-[7px] rounded-full ${
              apiStatus === 'online' ? 'bg-emerald-500' :
              apiStatus === 'offline' ? 'bg-red-400' :
              'bg-gray-300'
            }`} />
            {apiStatus === 'online' && (
              <span className="absolute inset-0 w-[7px] h-[7px] rounded-full bg-emerald-500 animate-ping opacity-30" />
            )}
          </div>
          <span className="text-[11px] text-[var(--text-tertiary)] font-medium">
            {apiStatus === 'checking' ? 'Connecting' : apiStatus === 'online' ? 'API Online' : 'API Offline'}
          </span>
        </div>

        {/* Separator — hidden on mobile */}
        <div className="hidden sm:block w-px h-4 bg-[var(--border)]" />

        {/* Notifications */}
        <div ref={notifRef} className="relative">
          <button
            ref={notifBtnRef}
            onClick={() => {
              if (notifBtnRef.current) {
                const rect = notifBtnRef.current.getBoundingClientRect();
                setNotifPos({ top: rect.bottom + 8, right: window.innerWidth - rect.right });
              }
              setNotifOpen(!notifOpen); setProfileOpen(false);
              if (!notifOpen) fetchNotifications();
            }}
            className="relative p-1.5 text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
            aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ''}`}
          >
            <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
            </svg>
            {unreadCount > 0 && (
              <span className="absolute -top-0.5 -right-0.5 w-4 h-4 bg-red-500 text-white text-[9px] font-bold rounded-full flex items-center justify-center">
                {unreadCount}
              </span>
            )}
          </button>

          {notifOpen && typeof document !== 'undefined' && createPortal(
            <div ref={notifPortalRef} className={`fixed w-[360px] rounded-xl shadow-xl border overflow-hidden z-[9999] ${glassEnabled ? 'glass-dropdown' : 'bg-[var(--surface)] border-[var(--border)]'}`}
              style={{ top: notifPos.top, right: notifPos.right }}
            >
              <div className="flex items-center justify-between px-4 py-2.5 border-b border-[var(--border-light)]">
                <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Notifications</h3>
                {unreadCount > 0 && (
                  <button onClick={markAllRead} className="text-[11px] text-[var(--accent)] hover:underline">
                    Mark all read
                  </button>
                )}
              </div>
              <div className="max-h-[320px] overflow-y-auto">
                {notifLoading ? (
                  <div className="px-4 py-8 text-center">
                    <div className="w-5 h-5 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin mx-auto mb-2" />
                    <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
                  </div>
                ) : notifications.length === 0 ? (
                  <div className="px-4 py-8 text-center">
                    <svg className="w-8 h-8 mx-auto mb-2 text-[var(--text-tertiary)] opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
                    </svg>
                    <p className="text-[13px] text-[var(--text-tertiary)]">No notifications yet</p>
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Recent activity will appear here</p>
                  </div>
                ) : (
                  notifications.map((n, idx) => (
                    <div key={n.id} className={`group flex items-start gap-3 px-4 py-3 hover:bg-[var(--bg)] transition-colors cursor-default ${!n.read ? 'bg-[var(--accent-subtle)]/40' : ''} ${idx !== notifications.length - 1 ? 'border-b border-[var(--border-light)]/50' : ''}`}>
                      <div className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 mt-0.5 ${typeColors[n.type]}`}>
                        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                          <path strokeLinecap="round" strokeLinejoin="round" d={n.icon} />
                        </svg>
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-[13px] font-medium text-[var(--text-primary)] leading-tight">{n.title}</p>
                        <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5 truncate">{n.description}</p>
                      </div>
                      <span className="text-[10px] text-[var(--text-tertiary)] shrink-0 mt-1 tabular-nums">{n.time}</span>
                      <button onClick={() => dismissNotification(n.id)} className="text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0 opacity-0 group-hover:opacity-100 transition-opacity -mr-1">
                        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      </button>
                    </div>
                  ))
                )}
              </div>
              {notifications.length > 0 && (
                <div className="border-t border-[var(--border-light)] px-4 py-2">
                  <Link href="/audit" onClick={() => setNotifOpen(false)} className="text-[11px] text-[var(--accent)] hover:underline flex items-center gap-1">
                    View all activity
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                    </svg>
                  </Link>
                </div>
              )}
            </div>,
            document.body
          )}
        </div>

        {/* Liquid Glass toggle */}
        <button
          onClick={() => setGlassEnabled(g => !g)}
          className={`p-1.5 rounded-lg transition-colors ${glassEnabled ? 'text-[var(--accent)] bg-[var(--accent-subtle)]' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)]'}`}
          title={`Liquid Glass: ${glassEnabled ? 'On' : 'Off'}`}
          aria-label={`Liquid Glass: ${glassEnabled ? 'On' : 'Off'}`}
        >
          {glassEnabled ? (
            <svg className="w-[18px] h-[18px]" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 2.69l5.66 5.66a8 8 0 11-11.31 0L12 2.69z" />
            </svg>
          ) : (
            <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 2.69l5.66 5.66a8 8 0 11-11.31 0L12 2.69z" />
            </svg>
          )}
        </button>

        {/* Theme toggle */}
        <button
          onClick={() => setTheme(t => t === 'light' ? 'dark' : t === 'dark' ? 'system' : 'light')}
          className="p-1.5 text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
          title={`Theme: ${theme}`}
          aria-label={`Theme: ${theme}`}
        >
          {theme === 'light' && (
            <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
            </svg>
          )}
          {theme === 'dark' && (
            <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z" />
            </svg>
          )}
          {theme === 'system' && (
            <svg className="w-[18px] h-[18px]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 17.25v1.007a3 3 0 01-.879 2.122L7.5 21h9l-.621-.621A3 3 0 0115 18.257V17.25m6-12V15a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 15V5.25m18 0A2.25 2.25 0 0018.75 3H5.25A2.25 2.25 0 003 5.25m18 0V12a2.25 2.25 0 01-2.25 2.25H5.25A2.25 2.25 0 013 12V5.25" />
            </svg>
          )}
        </button>

        {/* Separator */}
        <div className="w-px h-4 bg-[var(--border)]" />

        {/* User profile dropdown */}
        <div ref={profileRef} className="relative">
          <button
            ref={profileBtnRef}
            onClick={() => {
              if (profileBtnRef.current) {
                const rect = profileBtnRef.current.getBoundingClientRect();
                setProfilePos({ top: rect.bottom + 8, right: window.innerWidth - rect.right });
              }
              setProfileOpen(!profileOpen); setNotifOpen(false);
            }}
            className="flex items-center gap-2 hover:bg-[var(--border-light)] rounded-lg px-1.5 py-1 transition-colors"
          >
            <div className="w-[28px] h-[28px] rounded-full bg-gradient-to-br from-[#6366f1] to-[#8b5cf6] flex items-center justify-center shadow-sm">
              <span className="text-[11px] font-semibold text-white">{(storedUser?.name?.[0] || 'A').toUpperCase()}</span>
            </div>
            <span className="hidden md:block text-[12px] font-medium text-[var(--text-secondary)]">{storedUser?.name || ''}</span>
          </button>

          {profileOpen && typeof document !== 'undefined' && createPortal(
            <div ref={profilePortalRef} className={`fixed w-56 rounded-xl shadow-xl border overflow-hidden z-[9999] ${glassEnabled ? 'glass-dropdown' : 'bg-[var(--surface)] border-[var(--border)]'}`}
              style={{ top: profilePos.top, right: profilePos.right }}
            >
              <div className="px-4 py-3 border-b border-[var(--border-light)]">
                <p className="text-[13px] font-medium text-[var(--text-primary)]">{storedUser?.name || 'User'}</p>
                <p className="text-[11px] text-[var(--text-tertiary)]">{storedUser?.email || ''}</p>
              </div>
              <div className="py-1">
                {isAdmin && (
                <Link href="/settings" onClick={() => setProfileOpen(false)} className="flex items-center gap-2.5 px-4 py-2 text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)] transition-colors">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  </svg>
                  Settings
                </Link>
                )}
                <Link href="/roles" onClick={() => setProfileOpen(false)} className="flex items-center gap-2.5 px-4 py-2 text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)] transition-colors">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
                  </svg>
                  Roles & Permissions
                </Link>
                <div className="divider mx-4 my-1" />
                <Link href="/docs" onClick={() => setProfileOpen(false)} className="flex items-center gap-2.5 px-4 py-2 text-[13px] text-[var(--text-secondary)] hover:bg-[var(--bg)] hover:text-[var(--text-primary)] transition-colors">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
                  </svg>
                  Documentation
                </Link>
                <div className="divider mx-4 my-1" />
                <button
                  onClick={async () => {
                    setProfileOpen(false);
                    await doLogout();
                    window.location.href = '/login';
                  }}
                  className="flex items-center gap-2.5 px-4 py-2 text-[13px] text-red-400 hover:bg-red-500/10 hover:text-red-300 transition-colors w-full text-left"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3 0l3-3m0 0l-3-3m3 3H9" />
                  </svg>
                  Sign out
                </button>
              </div>
            </div>,
            document.body
          )}
        </div>
      </div>

      {/* Mobile menu overlay */}
      {mobileMenuOpen && (
        <div className="fixed inset-0 z-[9990] md:hidden" onClick={() => setMobileMenuOpen(false)}>
          <div className="absolute inset-0 bg-black/30" />
          <div className="absolute left-0 top-0 bottom-0 w-[240px] bg-[var(--surface)] shadow-xl" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 h-[52px] border-b border-[var(--border)]">
              <span className="font-semibold text-[14px] text-[var(--text-primary)]">Navigation</span>
              <button onClick={() => setMobileMenuOpen(false)} className="p-1 text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <nav className="p-3 space-y-1 overflow-y-auto max-h-[calc(100vh-52px)]">
              {[
                { href: '/', label: 'Dashboard', icon: '🏠' },
                { href: '/services/new', label: 'Deploy', icon: '🚀' },
                { href: '/services', label: 'Services', icon: '📦' },
                { href: '/connections', label: 'Connections', icon: '🔗', adminOnly: true, permission: 'connections' },
                { href: '/deployments', label: 'Deployments', icon: '🚀' },
                { href: '/clusters', label: 'Clusters', icon: '☸️', adminOnly: true, permission: 'clusters' },
                { href: '/workflows', label: 'Workflows', icon: '⚙️' },
                { href: '/scorecards', label: 'Scorecards', icon: '📋' },
                { href: '/security', label: 'Security', icon: '🛡️' },
                { href: '/analytics', label: 'Analytics', icon: '📊' },
                { href: '/ai', label: 'AI Assistant', icon: '🤖' },
                { href: '/audit', label: 'Audit Log', icon: '📝', adminOnly: true, permission: 'audit' },
                { href: '/roles', label: 'Roles', icon: '👥', adminOnly: true, permission: 'roles' },
                { href: '/settings', label: 'Settings', icon: '⚙️', adminOnly: true, permission: 'settings' },
              ].filter(item => !item.adminOnly || isAdmin || (item.permission && hasPermission(item.permission, 'read'))).map(item => (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={() => setMobileMenuOpen(false)}
                  className={`flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-[13px] transition-colors ${
                    pathname === item.href ? 'bg-[var(--border-light)] text-[var(--text-primary)] font-medium' : 'text-[var(--text-secondary)] hover:bg-[var(--bg)]'
                  }`}
                >
                  <span className="text-[14px]">{item.icon}</span>
                  {item.label}
                </Link>
              ))}
            </nav>
          </div>
        </div>
      )}
    </header>
  );
}
