'use client';

import { useState, useEffect, useCallback, Fragment } from 'react';
import { audit, listUsers, type AuditEntry, type SSHCommandEntry, type PluginActionEntry } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

type Tab = 'audit' | 'plugin-actions' | 'ssh-commands';

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
  api_create: { bg: 'bg-emerald-500/10', text: 'text-emerald-500', icon: '+' },
  api_update: { bg: 'bg-blue-500/10', text: 'text-blue-500', icon: '~' },
  api_delete: { bg: 'bg-red-500/10', text: 'text-red-500', icon: '-' },
  api_patch: { bg: 'bg-blue-500/10', text: 'text-blue-500', icon: '~' },
  check: { bg: 'bg-blue-500/10', text: 'text-blue-500', icon: '>' },
  block: { bg: 'bg-red-500/10', text: 'text-red-500', icon: 'x' },
  allow: { bg: 'bg-emerald-500/10', text: 'text-emerald-600', icon: '>' },
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

const ALL_ACTIONS = ['view', 'create', 'update', 'delete', 'login', 'startup', 'shutdown', 'deploy', 'trigger', 'install', 'uninstall', 'enable', 'disable', 'sync', 'write', 'execute', 'promote', 'rollback', 'cancel', 'restart', 'scale', 'suspend', 'resume', 'reconcile', 'grant', 'assign', 'revoke', 'configure', 'evaluate', 'rotate', 'check', 'block', 'allow', 'api_create', 'api_update', 'api_delete', 'api_patch'];
const ALL_RESOURCES = ['entity', 'workflow', 'plugin', 'scorecard', 'cluster', 'deployment', 'connection', 'service', 'setting', 'environment', 'docker_host', 'docker_service', 'helm_repository', 'pipeline_source', 'pipeline_run', 'vault', 'team', 'role', 'user', 'credential', 'system', 'discovery', 'marketplace', 'gitops', 'jira', 'k8s_deployment', 'fluxcd_helmrelease', 'workspace', 'blueprint', 'blueprint_group', 'organization', 's3', 'virtualization', 'rbac', 'auth', 'audit', 'observability', 'storage', 'ai', 'deployment_window', 'compliance_policy', 'security_finding', 'secret_rotation', 'batch_operation', 'pre_deploy_gate'];

// Map API paths to human-readable descriptions
function describePath(method: string, path: string, entityType: string): string {
  const p = path.replace(/^\/api\/v1\//, '');
  const segments = p.split('/');
  const resource = segments[0] || entityType;
  const id = segments[1];
  const actionWord = method === 'GET' ? 'Viewed' : method === 'POST' ? id ? 'Created' : 'Listed' : method === 'PUT' ? 'Updated' : method === 'PATCH' ? 'Patched' : method === 'DELETE' ? 'Deleted' : method;
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
    'deployment-windows': 'Deployment Windows', 'compliance-policies': 'Compliance Policies',
    'security-findings': 'Security Findings', 'secret-rotations': 'Secret Rotations',
    'batch-operations': 'Batch Operations', 'pre-deploy-gate': 'Pre-Deploy Gate',
    'deployment-audit': 'Deployment Audit',
  };
  const friendly = friendlyNames[resource] || resource.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
  if (id && id !== 'stats' && id !== 'labels' && id !== 'issues' && id !== 'projects' && id !== 'sprints' && id !== 'assignees') {
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
  GET: 'text-sky-500', POST: 'text-emerald-500', PUT: 'text-amber-500',
  PATCH: 'text-purple-500', DELETE: 'text-red-500',
};

// ── Plugin Action helpers ──────────────────────────────────

const PLUGIN_ACTION_LABELS: Record<string, string> = {
  create_vm: 'Create VM', delete_vm: 'Delete VM', start_vm: 'Start VM', stop_vm: 'Stop VM',
  shutdown_vm: 'Shutdown VM', reboot_vm: 'Reboot VM', suspend_vm: 'Suspend VM',
  create_container: 'Create Container', delete_container: 'Delete Container',
  start_container: 'Start Container', stop_container: 'Stop Container',
  deploy_docker: 'Deploy Docker', create_snapshot: 'Create Snapshot',
  delete_snapshot: 'Delete Snapshot', revert_snapshot: 'Revert Snapshot',
  create_bucket: 'Create Bucket', delete_bucket: 'Delete Bucket',
  upload_object: 'Upload Object', delete_object: 'Delete Object',
};

function getPluginActionStyle(action: string) {
  if (action.startsWith('create_snapshot') || action.startsWith('delete_snapshot') || action.startsWith('revert_snapshot')) return { bg: 'bg-purple-500/15', text: 'text-purple-500' };
  if (action.startsWith('create_bucket') || action.startsWith('upload_object')) return { bg: 'bg-emerald-500/15', text: 'text-emerald-600' };
  if (action.startsWith('delete_bucket') || action.startsWith('delete_object')) return { bg: 'bg-red-500/15', text: 'text-red-500' };
  if (action.startsWith('create') || action.startsWith('deploy')) return { bg: 'bg-emerald-500/15', text: 'text-emerald-600' };
  if (action.startsWith('delete')) return { bg: 'bg-red-500/15', text: 'text-red-500' };
  if (action.startsWith('start') || action.startsWith('resume')) return { bg: 'bg-blue-500/15', text: 'text-blue-500' };
  if (action.startsWith('stop') || action.startsWith('shutdown') || action.startsWith('suspend')) return { bg: 'bg-orange-500/10', text: 'text-orange-600' };
  if (action.startsWith('reboot') || action.startsWith('revert')) return { bg: 'bg-amber-500/15', text: 'text-amber-600' };
  return { bg: 'bg-[var(--border-light)]', text: 'text-[var(--text-secondary)]' };
}

function getPluginBadgeStyle(pluginName: string) {
  switch (pluginName) {
    case 'proxmox': return 'bg-orange-500/15 text-orange-600';
    case 'vmware': return 'bg-blue-500/15 text-blue-500';
    case 's3': return 'bg-cyan-500/15 text-cyan-600';
    default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
  }
}

function formatClock(dateStr: string) {
  return new Date(dateStr).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function formatRelTime(dateStr: string) {
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

// ════════════════════════════════════════════════════════════
// Page component
// ════════════════════════════════════════════════════════════

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

  return <AuditPageContent hasPluginActivityPerm={isAdmin || hasPermission('plugin_activity', 'read')} />;
}

function AuditPageContent({ hasPluginActivityPerm }: { hasPluginActivityPerm: boolean }) {
  const [tab, setTab] = useState<Tab>('audit');

  // ── Audit state ──
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

  // ── Plugin Actions state ──
  const [pluginActionsList, setPluginActionsList] = useState<PluginActionEntry[]>([]);
  const [pluginActionsTotal, setPluginActionsTotal] = useState(0);
  const [pluginActionsPage, setPluginActionsPage] = useState(1);
  const [pluginActionsLoading, setPluginActionsLoading] = useState(false);
  const [pluginFilter, setPluginFilter] = useState('');

  // ── SSH Commands state ──
  const [sshCommands, setSSHCommands] = useState<SSHCommandEntry[]>([]);
  const [sshTotal, setSSHTotal] = useState(0);
  const [sshPage, setSSHPage] = useState(1);
  const [sshLoading, setSSHLoading] = useState(false);

  // ── Shared: user map ──
  useEffect(() => {
    listUsers().then(res => {
      const m = new Map<string, string>();
      for (const u of res.users) m.set(u.id, u.name || u.email);
      setUserMap(m);
    }).catch(() => {});
  }, []);

  const getUserDisplayById = useCallback((userId?: string): string => {
    if (!userId) return 'system';
    if (userMap.has(userId)) return userMap.get(userId)!;
    return userId.slice(0, 8) + '...';
  }, [userMap]);

  // ── Audit data fetching ──
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

  // ── Plugin Actions data fetching ──
  const fetchPluginActions = useCallback(async () => {
    setPluginActionsLoading(true);
    try {
      const params: Record<string, string> = { page: String(pluginActionsPage) };
      if (pluginFilter) params.plugin_name = pluginFilter;
      const data = await audit.pluginActions(params);
      setPluginActionsList(data.items || []);
      setPluginActionsTotal(data.total || 0);
    } catch { /* silently fail */ }
    setPluginActionsLoading(false);
  }, [pluginActionsPage, pluginFilter]);

  // ── SSH Commands data fetching ──
  const fetchSSHCommands = useCallback(async () => {
    setSSHLoading(true);
    try {
      const data = await audit.sshCommands({ page: String(sshPage) });
      setSSHCommands(data.items || []);
      setSSHTotal(data.total || 0);
    } catch { /* silently fail */ }
    setSSHLoading(false);
  }, [sshPage]);

  useEffect(() => { if (tab === 'plugin-actions' && hasPluginActivityPerm) fetchPluginActions(); }, [tab, fetchPluginActions, hasPluginActivityPerm]);
  useEffect(() => { if (tab === 'ssh-commands' && hasPluginActivityPerm) fetchSSHCommands(); }, [tab, fetchSSHCommands, hasPluginActivityPerm]);

  // ── Audit helpers ──
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

  const topActions = Object.entries(stats.by_action).sort((a, b) => b[1] - a[1]).slice(0, 6);
  const topResources = Object.entries(stats.by_resource).sort((a, b) => b[1] - a[1]).slice(0, 4);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* ── Header ── */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Activity Log</h1>
            <p className="page-subtitle-modern">
              {tab === 'audit' && `${total.toLocaleString()} audit entries recorded`}
              {tab === 'plugin-actions' && `${pluginActionsTotal.toLocaleString()} plugin actions recorded`}
              {tab === 'ssh-commands' && `${sshTotal.toLocaleString()} SSH commands recorded`}
            </p>
          </div>
          <div className="flex items-center gap-3">
            {tab === 'audit' && (
              <label className="flex items-center gap-2 text-[12px] text-[var(--text-tertiary)] cursor-pointer">
                <input type="checkbox" checked={autoRefresh} onChange={e => setAutoRefresh(e.target.checked)} className="rounded border-[var(--border)]" />
                Auto-refresh
              </label>
            )}
            <button
              onClick={() => { if (tab === 'audit') fetchData(); else if (tab === 'plugin-actions') fetchPluginActions(); else fetchSSHCommands(); }}
              className="btn btn-secondary text-[12px] px-3 py-1.5"
              disabled={tab === 'audit' ? loading : tab === 'plugin-actions' ? pluginActionsLoading : sshLoading}
            >
              {(tab === 'audit' ? loading : tab === 'plugin-actions' ? pluginActionsLoading : sshLoading) ? '...' : 'Refresh'}
            </button>
          </div>
        </div>

        {/* ── Tabs ── */}
        <div className="page-animate-up page-delay-1 flex items-center gap-1 border-b border-[var(--border-light)]">
          <button onClick={() => setTab('audit')}
            className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${tab === 'audit' ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}>
            Audit Log
            {total > 0 && <span className="ml-2 text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded-full">{total}</span>}
          </button>
          {hasPluginActivityPerm && (
            <>
              <button onClick={() => setTab('plugin-actions')}
                className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${tab === 'plugin-actions' ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}>
                Plugin Actions
                {pluginActionsTotal > 0 && <span className="ml-2 text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded-full">{pluginActionsTotal}</span>}
              </button>
              <button onClick={() => setTab('ssh-commands')}
                className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${tab === 'ssh-commands' ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}>
                SSH Commands
                {sshTotal > 0 && <span className="ml-2 text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded-full">{sshTotal}</span>}
              </button>
            </>
          )}
        </div>

        {/* ═══════════ AUDIT LOG TAB ═══════════ */}
        {tab === 'audit' && (
          <>
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

            {error && <div className="px-4 py-2.5 rounded-xl text-[13px] bg-red-500/10 text-red-500 border border-red-500/20">{error}</div>}

            {/* Audit Table */}
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
                        const description = path && method ? describePath(method, path, entry.entity_type) : `${entry.action} ${entry.entity_type}`;
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
                              <td><span className="text-[12px] text-[var(--text-primary)]" title={entry.user_id || 'system'}>{userDisplay}</span></td>
                              <td><span className="text-mono text-[11px] text-[var(--text-tertiary)]">{entry.ip_address || '—'}</span></td>
                              <td><span className="text-[11px] text-[var(--text-tertiary)]" title={new Date(entry.created_at).toLocaleString()}>{formatTime(entry.created_at)}</span></td>
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
                                              <pre className="bg-[#1a1a2e] border border-[#2a2a4a] rounded-lg p-3 text-[11px] font-mono text-[#c9d1d9] overflow-auto max-h-[200px]">{JSON.stringify(entry.new_values, null, 2)}</pre>
                                            </div>
                                          )}
                                          {entry.old_values && Object.keys(entry.old_values).length > 0 && (
                                            <div>
                                              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Old Values</p>
                                              <pre className="bg-[#1a1a2e] border border-[#2a2a4a] rounded-lg p-3 text-[11px] font-mono text-[#c9d1d9] overflow-auto max-h-[200px]">{JSON.stringify(entry.old_values, null, 2)}</pre>
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
          </>
        )}

        {/* ═══════════ PLUGIN ACTIONS TAB ═══════════ */}
        {tab === 'plugin-actions' && (
          <>
            <div className="flex items-center gap-3">
              <select value={pluginFilter} onChange={e => { setPluginFilter(e.target.value); setPluginActionsPage(1); }}
                className="text-[12px] border border-[var(--border)] rounded-lg px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
                <option value="">All plugins</option>
                <option value="proxmox">Proxmox</option>
                <option value="vmware">VMware</option>
                <option value="s3">S3</option>
              </select>
              {pluginFilter && (
                <button onClick={() => { setPluginFilter(''); setPluginActionsPage(1); }}
                  className="text-[12px] text-[var(--accent)] hover:underline">Clear filter</button>
              )}
            </div>

            <div className="page-animate-up page-delay-2">
              <div className="table-container" style={{ borderRadius: '12px' }}>
                <table>
                  <thead>
                    <tr>
                      <th style={{ width: '80px' }}>Plugin</th>
                      <th style={{ width: '140px' }}>Action</th>
                      <th style={{ width: '80px' }}>Type</th>
                      <th style={{ width: '140px' }}>User</th>
                      <th style={{ width: '70px' }}>Status</th>
                      <th style={{ width: '120px' }}>IP</th>
                      <th style={{ width: '100px' }}>Time</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pluginActionsList.length === 0 && !pluginActionsLoading ? (
                      <tr>
                        <td colSpan={7} className="text-center py-10">
                          <div className="text-3xl mb-2 opacity-20">🔌</div>
                          <p className="text-[13px] text-[var(--text-secondary)] mb-1">No plugin actions recorded</p>
                          <p className="text-[12px] text-[var(--text-tertiary)]">VM and container operations will appear here</p>
                        </td>
                      </tr>
                    ) : pluginActionsList.length === 0 && pluginActionsLoading ? (
                      <tr><td colSpan={7} className="text-center py-10 text-[var(--text-tertiary)] text-[13px]">Loading...</td></tr>
                    ) : (
                      pluginActionsList.map(act => {
                        const style = getPluginActionStyle(act.action);
                        return (
                          <tr key={act.id} className="hover:bg-[var(--bg)]">
                            <td><span className={`text-[11px] font-medium px-2 py-0.5 rounded ${getPluginBadgeStyle(act.plugin_name)}`}>{act.plugin_name}</span></td>
                            <td><span className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium ${style.bg} ${style.text}`}>{PLUGIN_ACTION_LABELS[act.action] || act.action}</span></td>
                            <td><span className="text-[12px] text-[var(--text-secondary)] capitalize">{act.entity_type}</span></td>
                            <td><span className="text-[12px] text-[var(--text-primary)]">{getUserDisplayById(act.user_id)}</span></td>
                            <td><span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium ${act.status === 'success' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>{act.status}</span></td>
                            <td><span className="font-mono text-[11px] text-[var(--text-tertiary)]">{act.ip_address || '\u2014'}</span></td>
                            <td><span className="text-[11px] text-[var(--text-tertiary)]" title={formatRelTime(act.created_at)}>{formatClock(act.created_at)}</span></td>
                          </tr>
                        );
                      })
                    )}
                  </tbody>
                </table>
              </div>
              {pluginActionsTotal > 50 && (
                <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)] mt-4">
                  <span>Page {pluginActionsPage}</span>
                  <div className="flex items-center gap-2">
                    <button onClick={() => setPluginActionsPage(p => Math.max(1, p - 1))} disabled={pluginActionsPage <= 1}
                      className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Prev</button>
                    <button onClick={() => setPluginActionsPage(p => p + 1)} disabled={pluginActionsList.length < 50}
                      className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Next</button>
                  </div>
                </div>
              )}
            </div>
          </>
        )}

        {/* ═══════════ SSH COMMANDS TAB ═══════════ */}
        {tab === 'ssh-commands' && (
          <div className="page-animate-up page-delay-2">
            <div className="table-container" style={{ borderRadius: '12px' }}>
              <table>
                <thead>
                  <tr>
                    <th style={{ width: '140px' }}>User</th>
                    <th style={{ width: '140px' }}>Host</th>
                    <th style={{ width: '80px' }}>SSH User</th>
                    <th>Command</th>
                    <th style={{ width: '100px' }}>Time</th>
                  </tr>
                </thead>
                <tbody>
                  {sshCommands.length === 0 && !sshLoading ? (
                    <tr>
                      <td colSpan={5} className="text-center py-10">
                        <div className="text-3xl mb-2 opacity-20">🖥️</div>
                        <p className="text-[13px] text-[var(--text-secondary)] mb-1">No SSH commands recorded</p>
                        <p className="text-[12px] text-[var(--text-tertiary)]">Commands executed via the SSH terminal will appear here</p>
                      </td>
                    </tr>
                  ) : sshCommands.length === 0 && sshLoading ? (
                    <tr><td colSpan={5} className="text-center py-10 text-[var(--text-tertiary)] text-[13px]">Loading...</td></tr>
                  ) : (
                    sshCommands.map(cmd => (
                      <tr key={cmd.id} className="hover:bg-[var(--bg)]">
                        <td><span className="text-[12px] text-[var(--text-primary)]">{getUserDisplayById(cmd.user_id)}</span></td>
                        <td><span className="text-[12px] text-[var(--text-primary)] font-mono" title={cmd.host_id}>{cmd.host_name}</span></td>
                        <td><span className="text-[12px] text-[var(--text-secondary)] font-mono">{cmd.username}</span></td>
                        <td><code className="text-[12px] text-[var(--text-primary)] bg-[var(--bg)] px-2 py-0.5 rounded font-mono">{cmd.command}</code></td>
                        <td><span className="text-[11px] text-[var(--text-tertiary)]" title={formatRelTime(cmd.created_at)}>{formatClock(cmd.created_at)}</span></td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            {sshTotal > 50 && (
              <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)] mt-4">
                <span>Page {sshPage}</span>
                <div className="flex items-center gap-2">
                  <button onClick={() => setSSHPage(p => Math.max(1, p - 1))} disabled={sshPage <= 1}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Prev</button>
                  <button onClick={() => setSSHPage(p => p + 1)} disabled={sshCommands.length < 50}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Next</button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
