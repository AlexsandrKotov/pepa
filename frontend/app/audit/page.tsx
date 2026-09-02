'use client';

import { useState, useEffect, useCallback, Fragment } from 'react';
import { audit, listUsers, type AuditEntry } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

const ACTION_COLORS: Record<string, { bg: string; text: string; icon: string }> = {
  create: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '+' },
  update: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  delete: { bg: 'bg-red-500/15', text: 'text-red-500', icon: '-' },
  login: { bg: 'bg-purple-500/15', text: 'text-purple-500', icon: '>' },
  startup: { bg: 'bg-cyan-500/15', text: 'text-cyan-500', icon: '>' },
  shutdown: { bg: 'bg-orange-500/10', text: 'text-orange-600', icon: 'x' },
  deploy: { bg: 'bg-indigo-500/15', text: 'text-indigo-500', icon: '>' },
  trigger: { bg: 'bg-amber-500/15', text: 'text-amber-600', icon: '>' },
  install: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '+' },
  uninstall: { bg: 'bg-red-500/15', text: 'text-red-500', icon: '-' },
  enable: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '>' },
  disable: { bg: 'bg-orange-500/10', text: 'text-orange-600', icon: 'x' },
  sync: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  write: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  view: { bg: 'bg-slate-500/10', text: 'text-[var(--text-secondary)]', icon: '>' },
  read: { bg: 'bg-[var(--border-light)]', text: 'text-[var(--text-secondary)]', icon: '>' },
  rotate: { bg: 'bg-yellow-500/15', text: 'text-yellow-600', icon: '~' },
  execute: { bg: 'bg-indigo-500/15', text: 'text-indigo-500', icon: '>' },
  promote: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '>' },
  rollback: { bg: 'bg-orange-500/10', text: 'text-orange-600', icon: '<' },
  cancel: { bg: 'bg-red-500/15', text: 'text-red-500', icon: 'x' },
  restart: { bg: 'bg-amber-500/15', text: 'text-amber-600', icon: '~' },
  stop: { bg: 'bg-red-500/15', text: 'text-red-500', icon: 'x' },
  scale: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  suspend: { bg: 'bg-orange-500/10', text: 'text-orange-600', icon: 'x' },
  resume: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '>' },
  reconcile: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  grant: { bg: 'bg-emerald-500/15', text: 'text-emerald-600', icon: '+' },
  assign: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '+' },
  revoke: { bg: 'bg-red-500/15', text: 'text-red-500', icon: '-' },
  configure: { bg: 'bg-blue-500/15', text: 'text-blue-500', icon: '~' },
  evaluate: { bg: 'bg-purple-500/15', text: 'text-purple-500', icon: '>' },
  // API middleware actions
  api_create: { bg: 'bg-emerald-500/10', text: 'text-emerald-500', icon: '+' },
  api_update: { bg: 'bg-blue-500/10', text: 'text-blue-500', icon: '~' },
  api_delete: { bg: 'bg-red-500/10', text: 'text-red-500', icon: '-' },
  api_patch: { bg: 'bg-blue-500/10', text: 'text-blue-500', icon: '~' },
};

function getActionStyle(action: string) {
  return ACTION_COLORS[action] || { bg: 'bg-[var(--border-light)]', text: 'text-[var(--text-secondary)]', icon: '•' };
}

function formatTime(dateStr: string) {
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffH = Math.floor(diffMin / 60);
  if (diffH < 24) return `${diffH}h ago`;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

const ALL_ACTIONS = ['view', 'create', 'update', 'delete', 'login', 'startup', 'shutdown', 'deploy', 'trigger', 'install', 'uninstall', 'enable', 'disable', 'sync', 'write', 'execute', 'promote', 'rollback', 'cancel', 'restart', 'scale', 'suspend', 'resume', 'reconcile', 'grant', 'assign', 'revoke', 'configure', 'evaluate', 'api_create', 'api_update', 'api_delete', 'api_patch'];
const ALL_RESOURCES = ['entity', 'workflow', 'plugin', 'scorecard', 'cluster', 'deployment', 'connection', 'service', 'setting', 'environment', 'docker_host', 'docker_service', 'helm_repository', 'pipeline_source', 'pipeline_run', 'vault', 'team', 'role', 'user', 'credential', 'system', 'discovery', 'marketplace', 'gitops', 'jira', 'k8s_deployment', 'fluxcd_helmrelease', 'workspace', 'blueprint', 'blueprint_group', 'organization', 's3', 'virtualization', 'rbac', 'auth', 'audit', 'observability', 'storage', 'ai'];

// Map API paths to human-readable descriptions
function describePath(method: string, path: string, entityType: string): string {
  // Strip /api/v1/ prefix
  const p = path.replace(/^\/api\/v1\//, '');
  const segments = p.split('/');
  const resource = segments[0] || entityType;
  const id = segments[1];

  const actionWord = method === 'GET' ? 'Viewed' : method === 'POST' ? id ? 'Created' : 'Listed' : method === 'PUT' ? 'Updated' : method === 'PATCH' ? 'Patched' : method === 'DELETE' ? 'Deleted' : method;

  // Friendly resource names
  const friendlyNames: Record<string, string> = {
    'workspaces': 'Workspaces', 'clusters': 'Clusters', 'connections': 'Connections',
    'docker-hosts': 'Docker Hosts', 'docker-services': 'Docker Services',
    'helm-repositories': 'Helm Repositories', 'pipeline-sources': 'Pipeline Sources',
    'pipeline-runs': 'Pipeline Runs', 'vault': 'Vault Secrets', 'teams': 'Teams',
    'roles': 'Roles', 'users': 'Users', 'credentials': 'Credentials',
    'settings': 'Settings', 'environments': 'Environments', 'plugins': 'Plugins',
    'gitops': 'GitOps', 'jira': 'Jira', 'services': 'Services',
    'blueprints': 'Blueprints', 'blueprint-groups': 'Blueprint Groups',
    'scorecards': 'Scorecards', 'workflows': 'Workflows', 'entities': 'Entities',
    'discovery': 'Service Discovery', 'marketplace': 'Marketplace',
    's3-browser': 'S3 Browser', 'virtualization': 'Virtual Machines',
    'observability': 'Observability', 'audit': 'Audit Logs', 'auth': 'Authentication',
    'rbac': 'RBAC', 'ai': 'AI', 'ssh-hosts': 'SSH Hosts',
    'ssh-terminal': 'SSH Terminal', 'storage': 'Storage',
    'catalog': 'Service Catalog', 'organization': 'Organization',
  };

  const friendly = friendlyNames[resource] || resource.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());

  if (id && id !== 'stats' && id !== 'labels' && id !== 'issues' && id !== 'projects' && id !== 'components' && id !== 'sprints' && id !== 'assignees') {
    return `${actionWord} ${friendly} #${id.slice(0, 8)}`;
  }
  return `${actionWord} ${friendly}`;
}

function getStatusColor(code: number): string {
  if (code >= 200 && code < 300) return 'text-emerald-600 bg-emerald-500/10';
  if (code >= 300 && code < 400) return 'text-blue-500 bg-blue-500/10';
  if (code >= 400 && code < 500) return 'text-amber-600 bg-amber-500/10';
  return 'text-red-500 bg-red-500/10';
}

const METHOD_COLORS: Record<string, string> = {
  GET: 'text-sky-500',
  POST: 'text-emerald-500',
  PUT: 'text-amber-500',
  PATCH: 'text-purple-500',
  DELETE: 'text-red-500',
};

export default function AuditPage() {
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('audit', 'read')) {
    return <ForbiddenPage resource="audit" />;
  }

  return <AuditPageContent />;
}

function AuditPageContent() {
  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [actionFilter, setActionFilter] = useState('');
  const [resourceFilter, setResourceFilter] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [expandedIds, setExpandedIds] = useState<string[]>([]);
  const [stats, setStats] = useState<{ by_action: Record<string, number>; by_resource: Record<string, number> }>({ by_action: {}, by_resource: {} });
  const [error, setError] = useState<string | null>(null);
  const [userMap, setUserMap] = useState<Map<string, string>>(new Map());

  // Fetch users list for ID → name resolution
  useEffect(() => {
    listUsers().then(res => {
      const m = new Map<string, string>();
      for (const u of res.users) {
        m.set(u.id, u.name || u.email);
      }
      setUserMap(m);
    }).catch(() => {});
  }, []);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = { per_page: '50', page: String(page) };
      if (actionFilter) params.action = actionFilter;
      if (resourceFilter) params.entity_type = resourceFilter;
      const [data, st] = await Promise.all([
        audit.list(params),
        audit.stats().catch(() => ({ by_action: {}, by_resource: {} })),
      ]);
      setItems(data.items || []);
      setTotal(data.total || 0);
      setTotalPages(data.total_pages || 0);
      setStats({ by_action: st.by_action || {}, by_resource: st.by_resource || {} });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load audit logs');
    }
    setLoading(false);
  }, [page, actionFilter, resourceFilter]);

  useEffect(() => { fetchData(); }, [fetchData]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchData]);

  const topActions = Object.entries(stats.by_action).sort((a, b) => b[1] - a[1]).slice(0, 6);
  const topResources = Object.entries(stats.by_resource).sort((a, b) => b[1] - a[1]).slice(0, 4);

  // Extract metadata from new_values for display
  const getMeta = useCallback((entry: AuditEntry) => {
    if (!entry.new_values || typeof entry.new_values !== 'object') return {};
    return entry.new_values as Record<string, unknown>;
  }, []);

  const getUserDisplay = useCallback((entry: AuditEntry): string => {
    const meta = getMeta(entry);
    const email = meta.user_email as string | undefined;
    if (email) return email;
    if (entry.user_id && userMap.has(entry.user_id)) return userMap.get(entry.user_id)!;
    if (entry.user_id) return entry.user_id.slice(0, 8) + '...';
    return 'system';
  }, [userMap, getMeta]);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Audit Log</h1>
            <p className="page-subtitle-modern">{total.toLocaleString()} entries recorded</p>
          </div>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-[12px] text-[var(--text-tertiary)] cursor-pointer">
              <input type="checkbox" checked={autoRefresh} onChange={e => setAutoRefresh(e.target.checked)} className="rounded border-[var(--border)]" />
              Auto-refresh
            </label>
            <button onClick={fetchData} className="btn btn-secondary text-[12px] px-3 py-1.5" disabled={loading}>
              {loading ? '...' : 'Refresh'}
            </button>
          </div>
        </div>

        {/* Stats Cards */}
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 page-animate-up page-delay-1">
          {topActions.map(([action, count]) => {
            const style = getActionStyle(action);
            return (
              <button key={action} onClick={() => { setActionFilter(action === actionFilter ? '' : action); setPage(1); setExpandedIds([]); }}
                className={`modern-stat-card text-left transition-all ${actionFilter === action ? 'ring-2 ring-[var(--accent)]' : 'hover:border-[var(--border)]'}`}>
                <div className="flex items-center gap-2 mb-1">
                  <span className={`inline-flex items-center justify-center w-5 h-5 rounded text-[10px] font-mono font-bold ${style.bg} ${style.text}`}>{style.icon}</span>
                  <p className="text-[11px] text-[var(--text-tertiary)] capitalize truncate">{action}</p>
                </div>
                <p className="text-[20px] font-bold text-[var(--text-primary)]">{count}</p>
              </button>
            );
          })}
          {topActions.length === 0 && !loading && (
            <div className="col-span-6 card card-body text-center py-10" style={{ borderRadius: '12px' }}>
              <div className="text-4xl mb-3 opacity-30">📝</div>
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">No activity recorded yet</p>
              <p className="text-[12px] text-[var(--text-tertiary)]">Actions like creating services, deployments, and connections will appear here</p>
            </div>
          )}
        </div>

        {/* Filters */}
        <div className="page-animate-up page-delay-1 flex flex-wrap items-center gap-3">
          <select value={actionFilter} onChange={e => { setActionFilter(e.target.value); setPage(1); setExpandedIds([]); }}
            className="text-[12px] border border-[var(--border)] rounded-lg px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
            <option value="">All actions</option>
            {ALL_ACTIONS.map(a => <option key={a} value={a}>{a}</option>)}
          </select>
          <select value={resourceFilter} onChange={e => { setResourceFilter(e.target.value); setPage(1); setExpandedIds([]); }}
            className="text-[12px] border border-[var(--border)] rounded-lg px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
            <option value="">All resources</option>
            {ALL_RESOURCES.map(r => <option key={r} value={r}>{r}</option>)}
          </select>
          {(actionFilter || resourceFilter) && (
            <button onClick={() => { setActionFilter(''); setResourceFilter(''); setPage(1); setExpandedIds([]); }}
              className="text-[12px] text-[var(--accent)] hover:underline">Clear filters</button>
          )}
          {topResources.length > 0 && (
            <div className="flex items-center gap-1 ml-auto">
              {topResources.map(([res, count]) => (
                <button key={res} onClick={() => { setResourceFilter(res === resourceFilter ? '' : res); setPage(1); setExpandedIds([]); }}
                  className={`text-[11px] px-2 py-1 rounded-md transition-colors ${resourceFilter === res ? 'bg-[var(--accent)] text-white' : 'bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--border)]'}`}>
                  {res} <span className="opacity-60">{count}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Error banner */}
        {error && (
          <div className="px-4 py-2.5 rounded-xl text-[13px] bg-red-500/10 text-red-500 border border-red-500/20">
            {error}
          </div>
        )}

        {/* Table */}
        <div className="page-animate-up page-delay-2">
          <div className="table-container" style={{ borderRadius: '12px' }}>
            <table>
              <thead>
                <tr>
                  <th style={{ width: '100px' }}>Action</th>
                  <th>Description</th>
                  <th style={{ width: '60px' }}>Status</th>
                  <th style={{ width: '160px' }}>User</th>
                  <th style={{ width: '120px' }}>IP</th>
                  <th style={{ width: '100px' }}>Time</th>
                </tr>
              </thead>
              <tbody>
                {items.length === 0 && !loading ? (
                  <tr>
                    <td colSpan={6} className="text-center py-10">
                      <div className="text-3xl mb-2 opacity-20">📋</div>
                      <p className="text-[13px] text-[var(--text-secondary)] mb-1">No audit entries</p>
                      <p className="text-[12px] text-[var(--text-tertiary)]">Activity will appear here as you use the platform</p>
                    </td>
                  </tr>
                ) : items.length === 0 && loading ? (
                  <tr><td colSpan={6} className="text-center py-10 text-[var(--text-tertiary)] text-[13px]">Loading...</td></tr>
                ) : (
                  items.map((entry) => {
                    const style = getActionStyle(entry.action);
                    const isExpanded = expandedIds.includes(entry.id);
                    const meta = getMeta(entry);
                    const statusCode = meta.status_code as number | undefined;
                    const path = meta.path as string | undefined;
                    const method = meta.method as string | undefined;
                    const description = path && method
                      ? describePath(method, path, entry.entity_type)
                      : `${entry.action} ${entry.entity_type}`;
                    const userDisplay = getUserDisplay(entry);

                    return (
                      <Fragment key={entry.id}>
                        <tr className={`cursor-pointer hover:bg-[var(--bg)] ${isExpanded ? 'bg-[var(--bg)]' : ''}`}
                          onClick={() => setExpandedIds(prev => prev.includes(entry.id) ? prev.filter(id => id !== entry.id) : [...prev, entry.id])}>
                          <td>
                            <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-medium ${style.bg} ${style.text}`}>
                              <span className="text-[9px] transition-transform duration-200" style={{ display: 'inline-block', transform: isExpanded ? 'rotate(90deg)' : 'rotate(0deg)' }}>{'▸'}</span>
                              <span className="font-mono font-bold">{style.icon}</span>
                              <span className="capitalize">{entry.action}</span>
                            </span>
                          </td>
                          <td>
                            <div>
                              <p className="text-[12px] text-[var(--text-primary)] font-medium truncate max-w-[400px]" title={path || ''}>{description}</p>
                              {path && <p className="text-[10px] text-[var(--text-tertiary)] font-mono truncate max-w-[400px]">{path}</p>}
                            </div>
                          </td>
                          <td>
                            {statusCode ? (
                              <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-mono font-medium ${getStatusColor(statusCode)}`}>{statusCode}</span>
                            ) : <span className="text-[11px] text-[var(--text-tertiary)]">—</span>}
                          </td>
                          <td>
                            <span className="text-[12px] text-[var(--text-primary)]" title={entry.user_id || 'system'}>{userDisplay}</span>
                          </td>
                          <td>
                            <span className="text-mono text-[11px] text-[var(--text-tertiary)]">{entry.ip_address || '—'}</span>
                          </td>
                          <td>
                            <span className="text-[11px] text-[var(--text-tertiary)]" title={new Date(entry.created_at).toLocaleString()}>
                              {formatTime(entry.created_at)}
                            </span>
                          </td>
                        </tr>
                        {isExpanded && (
                          <tr>
                            <td colSpan={6} className="!p-0">
                              <div className="px-5 py-4 bg-[var(--bg)] border-l-2 border-[var(--accent)]">
                                {(() => {
                                  const userEmail = (meta.user_email as string) || '';
                                  return (
                                    <div className="space-y-3">
                                      <div className="flex items-center justify-between">
                                        <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Entry Details</h3>
                                        <button onClick={(e) => { e.stopPropagation(); setExpandedIds(prev => prev.filter(id => id !== entry.id)); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-lg">&times;</button>
                                      </div>
                                      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-[12px]">
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">Action</p>
                                          <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] font-medium ${style.bg} ${style.text}`}>
                                            <span className="font-mono font-bold">{style.icon}</span>
                                            <span className="capitalize">{entry.action}</span>
                                          </span>
                                        </div>
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">Resource</p>
                                          <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium bg-[var(--border-light)] text-[var(--text-secondary)]">{entry.entity_type}</span>
                                        </div>
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">User</p>
                                          <p className="text-[var(--text-primary)] text-[11px]">{userEmail || entry.user_id || 'system'}</p>
                                        </div>
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">Time</p>
                                          <p className="text-[var(--text-primary)] text-[11px]">{new Date(entry.created_at).toLocaleString()}</p>
                                        </div>
                                        <div className="sm:col-span-2">
                                          <p className="text-[var(--text-tertiary)] mb-0.5">HTTP</p>
                                          <p className="font-mono text-[11px]">
                                            <span className={`font-semibold ${METHOD_COLORS[(meta.method as string)] || 'text-[var(--text-secondary)]'}`}>{(meta.method as string) || '—'}</span>
                                            <span className="text-[var(--text-tertiary)] ml-1">{(meta.path as string) || '—'}</span>
                                          </p>
                                        </div>
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">Status</p>
                                          {statusCode ? (
                                            <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-mono font-medium ${getStatusColor(statusCode)}`}>{statusCode}</span>
                                          ) : <span className="text-[11px] text-[var(--text-tertiary)]">—</span>}
                                        </div>
                                        <div>
                                          <p className="text-[var(--text-tertiary)] mb-0.5">IP Address</p>
                                          <p className="font-mono text-[var(--text-secondary)] text-[11px]">{entry.ip_address || '—'}</p>
                                        </div>
                                      </div>
                                      {meta.query ? (
                                        <div>
                                          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Query Parameters</p>
                                          <pre className="bg-[#1a1a2e] border border-[#2a2a4a] rounded-lg p-3 text-[11px] font-mono text-[#c9d1d9] overflow-auto">{JSON.stringify(meta.query, null, 2)}</pre>
                                        </div>
                                      ) : null}
                                      {entry.new_values && Object.keys(entry.new_values).length > 0 && (
                                        <div>
                                          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Full Metadata</p>
                                          <pre className="bg-[#1a1a2e] border border-[#2a2a4a] rounded-lg p-3 text-[11px] font-mono text-[#c9d1d9] overflow-auto max-h-[200px]">
                                            {JSON.stringify(entry.new_values, null, 2)}
                                          </pre>
                                        </div>
                                      )}
                                      {entry.old_values && Object.keys(entry.old_values).length > 0 && (
                                        <div>
                                          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Old Values</p>
                                          <pre className="bg-[#1a1a2e] border border-[#2a2a4a] rounded-lg p-3 text-[11px] font-mono text-[#c9d1d9] overflow-auto max-h-[200px]">
                                            {JSON.stringify(entry.old_values, null, 2)}
                                          </pre>
                                        </div>
                                      )}
                                    </div>
                                  );
                                })()}
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>

        {/* Pagination */}
        {total > 0 && (
          <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)]">
            <span>Showing {(page - 1) * 50 + 1}–{Math.min(page * 50, total)} of {total.toLocaleString()}</span>
            <div className="flex items-center gap-2">
              <button onClick={() => { setPage(p => Math.max(1, p - 1)); setExpandedIds([]); }} disabled={page <= 1}
                className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Prev</button>
              <span>Page {page} / {totalPages}</span>
              <button onClick={() => { setPage(p => Math.min(totalPages, p + 1)); setExpandedIds([]); }} disabled={page >= totalPages}
                className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Next</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
