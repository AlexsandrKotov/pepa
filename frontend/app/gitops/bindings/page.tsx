'use client';

import { useEffect, useState, useMemo, useCallback } from 'react';
import Link from 'next/link';
import { gitopsBindings, environments, connections, GitOpsBinding, DiscoveredApp, Environment, WriteBackResult, Connection } from '@/lib/api';
import BindingWizard from '@/components/BindingWizard';

// ─── Column definitions ──────────────────────────────────────────────────────

const ALL_COLUMNS = [
  { id: 'application', label: 'Application', alwaysVisible: true },
  { id: 'engine', label: 'Engine', alwaysVisible: false },
  { id: 'namespace', label: 'Namespace', alwaysVisible: false },
  { id: 'cluster', label: 'Cluster', alwaysVisible: false },
  { id: 'environment', label: 'Environment', alwaysVisible: false },
  { id: 'strategy', label: 'Strategy', alwaysVisible: false },
  { id: 'actions', label: 'Actions', alwaysVisible: true },
];

const DEFAULT_VISIBLE = new Set(['application', 'engine', 'namespace', 'cluster', 'environment', 'strategy', 'actions']);

const GROUP_BY_OPTIONS = [
  { value: 'cluster', label: 'Cluster' },
  { value: 'namespace', label: 'Namespace' },
  { value: 'engine', label: 'Engine' },
  { value: 'environment', label: 'Environment' },
  { value: 'none', label: 'None' },
];

function loadVisibleColumns(): Set<string> {
  if (typeof window === 'undefined') return new Set(DEFAULT_VISIBLE);
  try {
    const saved = localStorage.getItem('gitops-bindings-columns');
    if (saved) {
      const arr = JSON.parse(saved) as string[];
      return new Set(arr);
    }
  } catch { /* ignore */ }
  return new Set(DEFAULT_VISIBLE);
}

function loadGroupBy(): string {
  if (typeof window === 'undefined') return 'cluster';
  return localStorage.getItem('gitops-bindings-groupby') || 'cluster';
}

export default function GitOpsBindingsPage() {
  const [bindings, setBindings] = useState<GitOpsBinding[]>([]);
  const [envs, setEnvs] = useState<Environment[]>([]);
  const [conns, setConns] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoveredApp[]>([]);
  const [showDiscover, setShowDiscover] = useState(false);
  const [error, setError] = useState('');

  // Filters
  const [filterEnv, setFilterEnv] = useState('');
  const [filterEngine, setFilterEngine] = useState('');
  const [filterCluster, setFilterCluster] = useState('');
  const [filterNamespace, setFilterNamespace] = useState('');
  const [searchQuery, setSearchQuery] = useState('');

  // Group-by & columns
  const [groupBy, setGroupBy] = useState<string>('cluster');
  const [visibleColumns, setVisibleColumns] = useState<Set<string>>(DEFAULT_VISIBLE);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  const [showColumnMenu, setShowColumnMenu] = useState(false);

  // Modals
  const [settingEnv, setSettingEnv] = useState<GitOpsBinding | null>(null);
  const [showWizard, setShowWizard] = useState(false);
  const [writeBackBinding, setWriteBackBinding] = useState<GitOpsBinding | null>(null);
  const [writeBackTag, setWriteBackTag] = useState('');
  const [writeBackName, setWriteBackName] = useState('');
  const [writeBackLoading, setWriteBackLoading] = useState(false);
  const [writeBackResult, setWriteBackResult] = useState<WriteBackResult | null>(null);
  const [writeBackError, setWriteBackError] = useState('');

  // Initialize from localStorage
  useEffect(() => {
    setVisibleColumns(loadVisibleColumns());
    setGroupBy(loadGroupBy());
  }, []);

  const loadBindings = async () => {
    try {
      const res = await gitopsBindings.list();
      setBindings(res.bindings || []);
    } catch (err) {
      setError('Failed to load bindings');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const loadEnvironments = async () => {
    try {
      const res = await environments.list();
      setEnvs(res.environments || []);
    } catch { /* ignore */ }
  };

  const loadConnections = async () => {
    try {
      const res = await connections.list({ per_page: '200' });
      setConns(res.connections || []);
    } catch { /* ignore */ }
  };

  useEffect(() => {
    loadBindings();
    loadEnvironments();
    loadConnections();
  }, []);

  // Connection lookup map
  const connMap = useMemo(() => {
    const m = new Map<string, Connection>();
    conns.forEach(c => m.set(c.id, c));
    return m;
  }, [conns]);

  const connName = useCallback((id: string | null | undefined) => {
    if (!id) return 'Unknown';
    return connMap.get(id)?.name || 'Unknown';
  }, [connMap]);

  // Derived filter options
  const clusterOptions = useMemo(() => {
    const ids = new Set(bindings.map(b => b.argo_connection_id).filter(Boolean));
    return [...ids]
      .map(id => ({ id: id!, name: connName(id) }))
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [bindings, connName]);

  const namespaceOptions = useMemo(() => {
    return [...new Set(bindings.map(b => b.app_namespace))].sort();
  }, [bindings]);

  // Filtered bindings
  const filteredBindings = useMemo(() => {
    return bindings.filter(b => {
      if (filterEnv && b.environment_id !== filterEnv) return false;
      if (filterEngine && b.engine_type !== filterEngine) return false;
      if (filterCluster && b.argo_connection_id !== filterCluster) return false;
      if (filterNamespace && b.app_namespace !== filterNamespace) return false;
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        if (!b.app_name.toLowerCase().includes(q) &&
            !b.app_namespace.toLowerCase().includes(q) &&
            !b.name.toLowerCase().includes(q)) return false;
      }
      return true;
    });
  }, [bindings, filterEnv, filterEngine, filterCluster, filterNamespace, searchQuery]);

  // Stats
  const stats = useMemo(() => ({
    total: bindings.length,
    clusters: new Set(bindings.map(b => b.argo_connection_id).filter(Boolean)).size,
    namespaces: new Set(bindings.map(b => b.app_namespace)).size,
    unbound: bindings.filter(b => !b.environment_id).length,
  }), [bindings]);

  // Group label depends on the active groupBy mode; must be declared before
  // groupedBindings, which is evaluated during the very first render.
  const getGroupLabel = useCallback((key: string) => {
    switch (groupBy) {
      case 'cluster': return connName(key);
      case 'namespace': return key;
      case 'engine': return key === 'argocd' ? 'ArgoCD' : key === 'fluxcd' ? 'FluxCD' : key;
      case 'environment': return key;
      default: return key;
    }
  }, [groupBy, connName]);

  // Grouped bindings
  const groupedBindings = useMemo(() => {
    if (groupBy === 'none') return null;

    const groups = new Map<string, GitOpsBinding[]>();

    filteredBindings.forEach(b => {
      let key: string;
      switch (groupBy) {
        case 'cluster':
          key = b.argo_connection_id || 'unknown';
          break;
        case 'namespace':
          key = b.app_namespace || 'unknown';
          break;
        case 'engine':
          key = b.engine_type || 'unknown';
          break;
        case 'environment':
          key = b.env_name || 'Unassigned';
          break;
        default:
          key = 'all';
      }
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key)!.push(b);
    });

    return [...groups.entries()]
      .map(([key, items]) => ({ key, label: getGroupLabel(key), items }))
      .sort((a, b) => {
        if (a.key === 'unknown' || a.key === 'Unassigned') return 1;
        if (b.key === 'unknown' || b.key === 'Unassigned') return -1;
        return a.label.localeCompare(b.label);
      });
  }, [filteredBindings, groupBy, getGroupLabel]);

  const toggleGroup = (key: string) => {
    setCollapsedGroups(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const handleGroupByChange = (value: string) => {
    setGroupBy(value);
    localStorage.setItem('gitops-bindings-groupby', value);
    setCollapsedGroups(new Set());
  };

  const handleColumnToggle = (colId: string) => {
    setVisibleColumns(prev => {
      const next = new Set(prev);
      if (next.has(colId)) next.delete(colId); else next.add(colId);
      localStorage.setItem('gitops-bindings-columns', JSON.stringify([...next]));
      return next;
    });
  };

  const isColVisible = (id: string) => {
    const col = ALL_COLUMNS.find(c => c.id === id);
    if (col?.alwaysVisible) return true;
    return visibleColumns.has(id);
  };

  // ─── Handlers ────────────────────────────────────────────────────────────────

  const handleDiscover = async () => {
    setDiscovering(true);
    try {
      const res = await gitopsBindings.discover();
      setDiscovered(res.discovered || []);
      setShowDiscover(true);
      await loadBindings();
    } catch (err) {
      setError('Discovery failed');
      console.error(err);
    } finally {
      setDiscovering(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this binding?')) return;
    try {
      await gitopsBindings.delete(id);
      setBindings(bindings.filter(b => b.id !== id));
    } catch (err) {
      setError('Failed to delete binding');
      console.error(err);
    }
  };

  const handleSetEnvironment = async (binding: GitOpsBinding, envId: string) => {
    try {
      if (envId) {
        await gitopsBindings.update(binding.id, {
          environment_id: envId,
          environment: envs.find(e => e.id === envId)?.slug,
        });
      } else {
        await gitopsBindings.update(binding.id, {
          clear_environment: true,
        });
      }
      await loadBindings();
      setSettingEnv(null);
    } catch (err) {
      setError('Failed to set environment');
      console.error(err);
    }
  };

  const handleWriteBack = async (dryRun: boolean) => {
    if (!writeBackBinding || !writeBackTag) return;
    setWriteBackLoading(true);
    setWriteBackError('');
    setWriteBackResult(null);
    try {
      const res = await gitopsBindings.writeBack(writeBackBinding.id, {
        image_tag: writeBackTag,
        image_name: writeBackName || undefined,
        dry_run: dryRun,
      });
      setWriteBackResult(res.result);
    } catch (err) {
      setWriteBackError(err instanceof Error ? err.message : 'Write-back failed');
    } finally {
      setWriteBackLoading(false);
    }
  };

  const openWriteBack = (binding: GitOpsBinding) => {
    setWriteBackBinding(binding);
    setWriteBackTag('');
    setWriteBackName('');
    setWriteBackResult(null);
    setWriteBackError('');
  };

  const getEngineBadge = (engine: string) => {
    const colors: Record<string, string> = {
      argocd: 'bg-blue-500/15 text-blue-600',
      fluxcd: 'bg-purple-500/15 text-purple-600',
    };
    return colors[engine] || 'bg-[var(--border-light)] text-[var(--text-secondary)]';
  };

  const getEngineLabel = (engine: string) => {
    const labels: Record<string, string> = { argocd: 'ArgoCD', fluxcd: 'FluxCD' };
    return labels[engine] || engine;
  };

  // ─── Render helpers ──────────────────────────────────────────────────────────

  const renderBindingRow = (binding: GitOpsBinding) => (
    <tr key={binding.id} className="hover:bg-[var(--border-light)] transition-colors">
      {isColVisible('application') && (
        <td className="!px-3 !py-2.5">
          <Link
            href={`/gitops/applications/${binding.argo_connection_id}/${binding.app_namespace}/${binding.app_name}`}
            className="text-[13px] font-medium text-[var(--accent)] hover:underline"
          >
            {binding.name}
          </Link>
        </td>
      )}
      {isColVisible('engine') && (
        <td className="!px-3 !py-2.5">
          <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${getEngineBadge(binding.engine_type)}`}>
            {getEngineLabel(binding.engine_type)}
          </span>
        </td>
      )}
      {isColVisible('namespace') && (
        <td className="!px-3 !py-2.5 text-[12px] font-mono text-[var(--text-secondary)]">
          {binding.app_namespace}
        </td>
      )}
      {isColVisible('cluster') && (
        <td className="!px-3 !py-2.5 text-[12px] text-[var(--text-secondary)]">
          {connName(binding.argo_connection_id)}
        </td>
      )}
      {isColVisible('environment') && (
        <td className="!px-3 !py-2.5">
          {binding.env_name ? (
            <button
              onClick={() => setSettingEnv(binding)}
              className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium hover:opacity-80 transition-opacity"
              style={{
                backgroundColor: (binding.env_color || '#6B7280') + '20',
                color: binding.env_color || '#6B7280',
              }}
            >
              <span className="w-2 h-2 rounded-full" style={{ backgroundColor: binding.env_color || '#6B7280' }} />
              {binding.env_name}
            </button>
          ) : (
            <button
              onClick={() => setSettingEnv(binding)}
              className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--accent)]"
            >
              Set...
            </button>
          )}
        </td>
      )}
      {isColVisible('strategy') && (
        <td className="!px-3 !py-2.5 text-[11px] text-[var(--text-secondary)]">
          {binding.update_strategy}
        </td>
      )}
      {isColVisible('actions') && (
        <td className="!px-3 !py-2.5">
          <div className="flex items-center justify-end gap-2">
            <button
              onClick={() => openWriteBack(binding)}
              className="text-[11px] px-2 py-1 bg-emerald-500/10 text-emerald-600 rounded hover:bg-emerald-500/15 font-medium"
              title="Write image tag back to Git"
            >
              Write Back
            </button>
            <button
              onClick={() => handleDelete(binding.id)}
              className="text-[11px] px-2 py-1 bg-red-500/5 text-red-400 rounded hover:bg-red-500/10"
            >
              Delete
            </button>
          </div>
        </td>
      )}
    </tr>
  );

  const renderTable = (items: GitOpsBinding[]) => (
    <div className="overflow-x-auto rounded-lg border border-[var(--border)]" style={{ borderRadius: '12px' }}>
      <table>
        <thead>
          <tr>
            {isColVisible('application') && <th className="!px-3">Application</th>}
            {isColVisible('engine') && <th className="!px-3">Engine</th>}
            {isColVisible('namespace') && <th className="!px-3">Namespace</th>}
            {isColVisible('cluster') && <th className="!px-3">Cluster</th>}
            {isColVisible('environment') && <th className="!px-3">Environment</th>}
            {isColVisible('strategy') && <th className="!px-3">Strategy</th>}
            {isColVisible('actions') && <th className="!px-3 text-right">Actions</th>}
          </tr>
        </thead>
        <tbody>
          {items.map(renderBindingRow)}
        </tbody>
      </table>
    </div>
  );

  const renderGroupedTables = () => {
    if (!groupedBindings) return renderTable(filteredBindings);

    return (
      <div className="space-y-4">
        {groupedBindings.map(({ key, label, items }) => {
          const isCollapsed = collapsedGroups.has(key);
          return (
            <div key={key} className="card overflow-hidden">
              {/* Group header */}
              <button
                onClick={() => toggleGroup(key)}
                className="w-full flex items-center justify-between px-4 py-3 bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors border-b border-[var(--border-light)]"
              >
                <div className="flex items-center gap-2">
                  <svg
                    className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${isCollapsed ? '' : 'rotate-90'}`}
                    fill="none" viewBox="0 0 24 24" stroke="currentColor"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                  <span className="text-[13px] font-semibold text-[var(--text-primary)]">{label}</span>
                  {groupBy === 'cluster' && (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${getEngineBadge(items[0]?.engine_type || '')}`}>
                      {getEngineLabel(items[0]?.engine_type || '')}
                    </span>
                  )}
                </div>
                <span className="text-[11px] text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded-full">
                  {items.length} binding{items.length !== 1 ? 's' : ''}
                </span>
              </button>
              {/* Group content */}
              {!isCollapsed && (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="bg-[var(--bg)]">
                        {isColVisible('application') && <th className="!px-3 !py-2 text-[10px]">Application</th>}
                        {isColVisible('engine') && <th className="!px-3 !py-2 text-[10px]">Engine</th>}
                        {isColVisible('namespace') && <th className="!px-3 !py-2 text-[10px]">Namespace</th>}
                        {isColVisible('cluster') && <th className="!px-3 !py-2 text-[10px]">Cluster</th>}
                        {isColVisible('environment') && <th className="!px-3 !py-2 text-[10px]">Environment</th>}
                        {isColVisible('strategy') && <th className="!px-3 !py-2 text-[10px]">Strategy</th>}
                        {isColVisible('actions') && <th className="!px-3 !py-2 text-[10px] text-right">Actions</th>}
                      </tr>
                    </thead>
                    <tbody>
                      {items.map(renderBindingRow)}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          );
        })}
      </div>
    );
  };

  // ─── Loading state ────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="loading-spinner" />
      </div>
    );
  }

  // ─── Main render ─────────────────────────────────────────────────────────────

  const hasActiveFilters = filterEnv || filterEngine || filterCluster || filterNamespace || searchQuery;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">GitOps Bindings</h1>
            <p className="page-subtitle-modern">Map services to GitOps applications (ArgoCD / FluxCD)</p>
          </div>
          <div className="flex gap-2">
            <Link href="/gitops/applications" className="btn btn-secondary text-xs">
              Applications
            </Link>
            <button onClick={() => setShowWizard(true)} className="btn btn-primary text-xs">
              Create Binding
            </button>
            <button
              onClick={handleDiscover}
              disabled={discovering}
              className="btn btn-secondary text-xs"
            >
              {discovering ? 'Discovering...' : 'Auto-Discover'}
            </button>
          </div>
        </div>

        {error && (
          <div className="page-animate-up rounded-xl border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-600">
            {error}
          </div>
        )}

        {/* Stats */}
        <div className="grid grid-cols-4 gap-4 page-animate-up page-delay-1">
          {[
            { label: 'Total Bindings', value: stats.total, color: 'text-[var(--text-primary)]' },
            { label: 'Clusters', value: stats.clusters, color: 'text-blue-600' },
            { label: 'Namespaces', value: stats.namespaces, color: 'text-purple-600' },
            { label: 'Unassigned Env', value: stats.unbound, color: stats.unbound > 0 ? 'text-orange-600' : 'text-[var(--text-tertiary)]' },
          ].map(s => (
            <div key={s.label} className="card card-body py-3 flex items-center gap-3">
              <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
              <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
            </div>
          ))}
        </div>

        {/* Filters + Group-by + Columns */}
        <div className="flex flex-wrap items-center gap-3 page-animate-up page-delay-2">
          <input
            type="text"
            placeholder="Search apps..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="input flex-1 min-w-[180px] max-w-xs"
          />
          <select value={filterCluster} onChange={e => setFilterCluster(e.target.value)} className="input w-36">
            <option value="">All Clusters</option>
            {clusterOptions.map(c => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <select value={filterNamespace} onChange={e => setFilterNamespace(e.target.value)} className="input w-36">
            <option value="">All Namespaces</option>
            {namespaceOptions.map(n => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
          <select value={filterEnv} onChange={e => setFilterEnv(e.target.value)} className="input w-36">
            <option value="">All Environments</option>
            {envs.map(env => (
              <option key={env.id} value={env.id}>{env.name}</option>
            ))}
          </select>
          <select value={filterEngine} onChange={e => setFilterEngine(e.target.value)} className="input w-32">
            <option value="">All Engines</option>
            <option value="argocd">ArgoCD</option>
            <option value="fluxcd">FluxCD</option>
          </select>

          {/* Group by */}
          <div className="flex items-center gap-1.5 ml-auto">
            <label className="text-[11px] text-[var(--text-tertiary)] font-medium whitespace-nowrap">Group by:</label>
            <select
              value={groupBy}
              onChange={e => handleGroupByChange(e.target.value)}
              className="input w-28 !py-1.5 text-[12px]"
            >
              {GROUP_BY_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          {/* Column visibility */}
          <div className="relative">
            <button
              onClick={() => setShowColumnMenu(!showColumnMenu)}
              className="btn btn-secondary !py-1.5 text-[12px] flex items-center gap-1.5"
              title="Toggle columns"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 4v16m6-16v16M4 9h16M4 15h16" />
              </svg>
              Columns
            </button>
            {showColumnMenu && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setShowColumnMenu(false)} />
                <div className="absolute right-0 top-full mt-1 z-50 w-44 card shadow-lg py-2">
                  {ALL_COLUMNS.map(col => (
                    <label
                      key={col.id}
                      className={`flex items-center gap-2 px-3 py-1.5 text-[12px] cursor-pointer ${col.alwaysVisible ? 'opacity-50 cursor-not-allowed' : 'hover:bg-[var(--border-light)]'}`}
                    >
                      <input
                        type="checkbox"
                        checked={col.alwaysVisible || visibleColumns.has(col.id)}
                        disabled={col.alwaysVisible}
                        onChange={() => handleColumnToggle(col.id)}
                        className="rounded border-[var(--border)]"
                      />
                      {col.label}
                    </label>
                  ))}
                </div>
              </>
            )}
          </div>

          {hasActiveFilters && (
            <button
              onClick={() => { setFilterEnv(''); setFilterEngine(''); setFilterCluster(''); setFilterNamespace(''); setSearchQuery(''); }}
              className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
            >
              Clear filters
            </button>
          )}
        </div>

        {/* Discovery Results Modal */}
        {showDiscover && (
          <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50">
            <div className="card w-full max-w-2xl mx-4 max-h-[80vh] flex flex-col">
              <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border)]">
                <h2 className="text-[16px] font-semibold text-[var(--text-primary)]">Discovery Results</h2>
                <button onClick={() => setShowDiscover(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                  <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
              <div className="flex-1 overflow-auto px-6 py-4">
                {discovered.length === 0 ? (
                  <p className="text-[13px] text-[var(--text-tertiary)]">No applications discovered</p>
                ) : (
                  <div className="space-y-2">
                    {discovered.map((app, idx) => (
                      <div key={idx} className="flex items-center justify-between p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg)]">
                        <div>
                          <div className="flex items-center gap-2">
                            <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${getEngineBadge(app.app.engine_type)}`}>
                              {getEngineLabel(app.app.engine_type)}
                            </span>
                            <span className="text-[13px] font-medium text-[var(--text-primary)]">{app.app.name}</span>
                            <span className="text-[12px] text-[var(--text-tertiary)]">{app.app.namespace}</span>
                          </div>
                          <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
                            Connection: {app.connection_name}
                          </div>
                        </div>
                        <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${app.bound ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                          {app.bound ? 'Bound' : 'Unbound'}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div className="px-6 py-4 border-t border-[var(--border)]">
                <button onClick={() => setShowDiscover(false)} className="btn btn-secondary w-full text-xs">Close</button>
              </div>
            </div>
          </div>
        )}

        {/* Set Environment Modal */}
        {settingEnv && (
          <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50">
            <div className="card w-full max-w-sm mx-4">
              <div className="px-6 py-4 border-b border-[var(--border)]">
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                  Set Environment for &ldquo;{settingEnv.app_name}&rdquo;
                </h2>
              </div>
              <div className="px-6 py-4 space-y-1 max-h-[300px] overflow-auto">
                <button
                  onClick={() => handleSetEnvironment(settingEnv, '')}
                  className="w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--border-light)] text-[13px] text-[var(--text-tertiary)]"
                >
                  None
                </button>
                {envs.map(env => (
                  <button
                    key={env.id}
                    onClick={() => handleSetEnvironment(settingEnv, env.id)}
                    className={`w-full text-left px-3 py-2 rounded-lg hover:bg-[var(--border-light)] flex items-center gap-2 ${
                      settingEnv.environment_id === env.id ? 'bg-[var(--accent)]/10' : ''
                    }`}
                  >
                    <span className="w-3 h-3 rounded-full" style={{ backgroundColor: env.color || '#6B7280' }} />
                    <span className="text-[13px] text-[var(--text-primary)]">{env.name}</span>
                    <span className="text-[11px] text-[var(--text-tertiary)] ml-auto">{env.slug}</span>
                  </button>
                ))}
              </div>
              <div className="px-6 py-4 border-t border-[var(--border)]">
                <button onClick={() => setSettingEnv(null)} className="btn btn-secondary w-full text-xs">Cancel</button>
              </div>
            </div>
          </div>
        )}

        {/* Bindings Table(s) */}
        {filteredBindings.length === 0 ? (
          <div className="card card-body text-center py-12">
            <div className="text-4xl mb-3 opacity-30">&#x1F517;</div>
            <h3 className="text-[15px] font-medium text-[var(--text-primary)]">
              {bindings.length === 0 ? 'No bindings' : 'No matching bindings'}
            </h3>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-1">
              {bindings.length === 0
                ? 'Click "Auto-Discover" to find and bind GitOps applications'
                : 'Try adjusting your filters'}
            </p>
          </div>
        ) : (
          <>
            {renderGroupedTables()}
            <div className="text-[11px] text-[var(--text-tertiary)]">
              Showing {filteredBindings.length} of {bindings.length} binding{bindings.length !== 1 ? 's' : ''}
            </div>
          </>
        )}

        {/* Write-Back Modal */}
        {writeBackBinding && (
          <div className="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50">
            <div className="card w-full max-w-lg mx-4">
              <div className="px-6 py-4 border-b border-[var(--border)]">
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Write Image Tag to Git</h2>
                <p className="text-[12px] text-[var(--text-tertiary)] mt-1">
                  Update the image tag for &ldquo;{writeBackBinding.app_name}&rdquo; ({writeBackBinding.update_strategy})
                </p>
              </div>
              <div className="px-6 py-4 space-y-3">
                <div>
                  <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase block mb-1.5">Image Tag *</label>
                  <input
                    type="text"
                    value={writeBackTag}
                    onChange={e => setWriteBackTag(e.target.value)}
                    placeholder="e.g. v1.2.3, sha-abc1234"
                    className="input w-full text-[13px]"
                  />
                </div>
                <div>
                  <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase block mb-1.5">Image Name (optional)</label>
                  <input
                    type="text"
                    value={writeBackName}
                    onChange={e => setWriteBackName(e.target.value)}
                    placeholder="e.g. registry.example.com/myapp"
                    className="input w-full text-[13px]"
                  />
                </div>

                {writeBackError && (
                  <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-600">
                    {writeBackError}
                  </div>
                )}

                {writeBackResult && (
                  <div className="p-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-[12px] text-emerald-600 space-y-1">
                    <div className="font-medium">Write-back successful</div>
                    <div>Commit: <code className="text-[11px]">{writeBackResult.commit_sha?.slice(0, 8) || 'preview'}</code></div>
                    <div>Branch: {writeBackResult.branch}</div>
                    <div>File: {writeBackResult.file_path}</div>
                    {writeBackResult.mr_needed && (
                      <div className="text-orange-600">MR needed — push was rejected, branch created</div>
                    )}
                    {writeBackResult.diff && (
                      <details className="mt-2">
                        <summary className="cursor-pointer text-[11px]">View diff</summary>
                        <pre className="mt-1 text-[10px] overflow-x-auto bg-[var(--bg)] p-2 rounded">{writeBackResult.diff}</pre>
                      </details>
                    )}
                  </div>
                )}
              </div>
              <div className="px-6 py-4 border-t border-[var(--border)] flex items-center gap-2">
                <button
                  onClick={() => handleWriteBack(false)}
                  disabled={!writeBackTag || writeBackLoading}
                  className="flex-1 btn btn-primary text-xs disabled:opacity-50"
                >
                  {writeBackLoading ? 'Writing...' : 'Commit to Git'}
                </button>
                <button
                  onClick={() => handleWriteBack(true)}
                  disabled={!writeBackTag || writeBackLoading}
                  className="btn btn-secondary text-xs"
                >
                  Preview
                </button>
                <button
                  onClick={() => { setWriteBackBinding(null); setWriteBackResult(null); }}
                  className="btn btn-secondary text-xs"
                >
                  Close
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Binding Wizard */}
        <BindingWizard
          open={showWizard}
          onClose={() => setShowWizard(false)}
          onCreated={() => loadBindings()}
        />
      </div>
    </div>
  );
}
