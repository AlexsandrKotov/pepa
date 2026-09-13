'use client';

import { useEffect, useState, useMemo, useCallback } from 'react';
import Link from 'next/link';
import { gitopsBindings, environments, connections, GitOpsBinding, DiscoveredApp, Environment, WriteBackResult, Connection } from '@/lib/api';
import BindingWizard from '@/components/BindingWizard';
import { useUrlFilters } from '@/hooks/useUrlFilters';
import FilterBar from '@/components/filters/FilterBar';
import FilterChips, { type ActiveChip } from '@/components/filters/FilterChips';
import FilterMenu from '@/components/filters/FilterMenu';
import QuickFilter from '@/components/filters/QuickFilter';
import SearchInput from '@/components/filters/SearchInput';
import type { FilterGroup, FilterOption } from '@/components/filters/types';
import { fieldLabel, valueLabel, valueTone } from '@/lib/filter-labels';

export default function GitOpsBindingsPage() {
  const [bindings, setBindings] = useState<GitOpsBinding[]>([]);
  const [envs, setEnvs] = useState<Environment[]>([]);
  const [conns, setConns] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoveredApp[]>([]);
  const [showDiscover, setShowDiscover] = useState(false);
  const [error, setError] = useState('');

  // Filters — URL-synced so deep links and Back/Forward just work.
  const filters = useUrlFilters({
    single: ['search', 'engine'],
    multi: ['cluster', 'namespace', 'environment'],
  });

  // Modals
  const [settingEnv, setSettingEnv] = useState<GitOpsBinding | null>(null);
  const [showWizard, setShowWizard] = useState(false);
  const [writeBackBinding, setWriteBackBinding] = useState<GitOpsBinding | null>(null);
  const [writeBackTag, setWriteBackTag] = useState('');
  const [writeBackName, setWriteBackName] = useState('');
  const [writeBackLoading, setWriteBackLoading] = useState(false);
  const [writeBackResult, setWriteBackResult] = useState<WriteBackResult | null>(null);
  const [writeBackError, setWriteBackError] = useState('');

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
    } catch (e) { console.error("Operation failed:", e); }
  };

  const loadConnections = async () => {
    try {
      const res = await connections.list({ per_page: '200' });
      setConns(res.connections || []);
    } catch (e) { console.error("Operation failed:", e); }
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

  // Derived filter options from the loaded data
  const clusterOptions = useMemo(() => {
    const ids = new Set(bindings.map(b => b.argo_connection_id).filter(Boolean));
    return [...ids]
      .map(id => connMap.get(id!))
      .filter(Boolean)
      .sort((a, b) => a!.name.localeCompare(b!.name))
      .map(c => ({ value: c!.id, label: c!.name }));
  }, [bindings, connMap]);

  const namespaceOptions = useMemo(() => {
    return [...new Set(bindings.map(b => b.app_namespace))].filter(Boolean).sort();
  }, [bindings]);

  const envOptions = useMemo<FilterOption[]>(() => {
    return envs.map(env => ({ value: env.id, label: env.name, tone: valueTone(env.slug || '') }));
  }, [envs]);

  const engineOptions = useMemo<FilterOption[]>(() => [
    { value: 'argocd', label: 'ArgoCD', tone: 'accent' as const },
    { value: 'fluxcd', label: 'FluxCD', tone: 'info' as const },
  ], []);

  // Read current filter values from the URL store
  const searchValue = filters.get('search');
  const engineValue = filters.get('engine');
  const clusterValues = filters.getAll('cluster');
  const namespaceValues = filters.getAll('namespace');
  const environmentValues = filters.getAll('environment');

  // Filter groups for the FilterMenu
  const filterGroups = useMemo<FilterGroup[]>(() => [
    { key: 'cluster', label: fieldLabel('cluster'), multi: true, options: clusterOptions },
    { key: 'namespace', label: fieldLabel('namespace'), multi: true, searchable: namespaceOptions.length > 7,
      options: namespaceOptions.map(n => ({ value: n, label: n })) },
    { key: 'environment', label: fieldLabel('environment'), multi: true, options: envOptions },
  ], [clusterOptions, namespaceOptions, envOptions]);

  const menuValues: Record<string, string[]> = {
    cluster: clusterValues,
    namespace: namespaceValues,
    environment: environmentValues,
  };

  const handleMenuSelect = (key: string, value: string) => {
    filters.toggle(key, value);
  };

  // Client-side filtering driven by URL state
  const filteredBindings = useMemo(() => {
    return bindings.filter(b => {
      if (environmentValues.length && (!b.environment_id || !environmentValues.includes(b.environment_id))) return false;
      if (engineValue && b.engine_type !== engineValue) return false;
      if (clusterValues.length && (!b.argo_connection_id || !clusterValues.includes(b.argo_connection_id))) return false;
      if (namespaceValues.length && !namespaceValues.includes(b.app_namespace)) return false;
      if (searchValue) {
        const q = searchValue.toLowerCase();
        if (!b.app_name.toLowerCase().includes(q) &&
            !b.app_namespace.toLowerCase().includes(q) &&
            !b.name.toLowerCase().includes(q)) return false;
      }
      return true;
    });
  }, [bindings, environmentValues, engineValue, clusterValues, namespaceValues, searchValue]);

  // Active chips
  const chips: ActiveChip[] = [];
  if (searchValue) {
    chips.push({ id: 'search', field: fieldLabel('search'), label: `"${searchValue}"`, onRemove: () => filters.set('search', '') });
  }
  if (engineValue) {
    chips.push({ id: 'engine', field: fieldLabel('engine'), label: valueLabel(engineValue), tone: valueTone(engineValue), onRemove: () => filters.set('engine', '') });
  }
  for (const v of clusterValues) {
    const opt = clusterOptions.find(o => o.value === v);
    chips.push({ id: `cluster-${v}`, field: fieldLabel('cluster'), label: opt?.label ?? v, onRemove: () => filters.toggle('cluster', v) });
  }
  for (const v of namespaceValues) {
    chips.push({ id: `namespace-${v}`, field: fieldLabel('namespace'), label: v, onRemove: () => filters.toggle('namespace', v) });
  }
  for (const v of environmentValues) {
    const opt = envOptions.find(o => o.value === v);
    chips.push({ id: `environment-${v}`, field: fieldLabel('environment'), label: opt?.label ?? v, tone: opt?.tone, onRemove: () => filters.toggle('environment', v) });
  }

  const resultSummary = `${filteredBindings.length} of ${bindings.length} binding${bindings.length !== 1 ? 's' : ''}`;

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
      <td className="!px-3 !py-2.5">
        <Link
          href={`/gitops/applications/${binding.argo_connection_id}/${binding.app_namespace}/${binding.app_name}`}
          className="text-[13px] font-medium text-[var(--accent)] hover:underline"
        >
          {binding.name}
        </Link>
      </td>
      <td className="!px-3 !py-2.5">
        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${getEngineBadge(binding.engine_type)}`}>
          {getEngineLabel(binding.engine_type)}
        </span>
      </td>
      <td className="!px-3 !py-2.5 text-[12px] font-mono text-[var(--text-secondary)]">
        {binding.app_namespace}
      </td>
      <td className="!px-3 !py-2.5 text-[12px] text-[var(--text-secondary)]">
        {connName(binding.argo_connection_id)}
      </td>
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
      <td className="!px-3 !py-2.5 text-[11px] text-[var(--text-secondary)]">
        {binding.update_strategy}
      </td>
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
    </tr>
  );

  const renderTable = (items: GitOpsBinding[]) => (
    <div className="overflow-x-auto rounded-lg border border-[var(--border)]" style={{ borderRadius: '12px' }}>
      <table>
        <thead>
          <tr>
            <th className="!px-3">Application</th>
            <th className="!px-3">Engine</th>
            <th className="!px-3">Namespace</th>
            <th className="!px-3">Cluster</th>
            <th className="!px-3">Environment</th>
            <th className="!px-3">Strategy</th>
            <th className="!px-3 text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {items.map(renderBindingRow)}
        </tbody>
      </table>
    </div>
  );

  // ─── Loading state ────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="loading-spinner" />
      </div>
    );
  }

  // ─── Main render ─────────────────────────────────────────────────────────────

  const envFilterCount = environmentValues.length;
  const clusterFilterCount = clusterValues.length;
  const namespaceFilterCount = namespaceValues.length;
  const menuActiveCount = envFilterCount + clusterFilterCount + namespaceFilterCount;
  const menuTriggerLabel = menuActiveCount > 0
    ? `${fieldLabel('environment')}${menuActiveCount > 1 ? ` (+${menuActiveCount - 1})` : ''}`
    : fieldLabel('environment');

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

        {/* Filters */}
        <FilterBar
          className="page-animate-up page-delay-1"
          search={
            <SearchInput
              value={searchValue}
              onCommit={value => filters.set('search', value)}
              placeholder="Search applications..."
              label="Search applications"
            />
          }
          quick={
            <QuickFilter
              field="engine"
              label={fieldLabel('engine')}
              options={engineOptions}
              value={engineValue}
              onChange={value => filters.set('engine', value)}
            />
          }
          menu={
            <FilterMenu
              groups={filterGroups}
              values={menuValues}
              onToggle={handleMenuSelect}
              onClearGroup={key => filters.remove(key)}
              triggerLabel={menuTriggerLabel}
              activeCount={menuActiveCount}
            />
          }
          chips={
            <FilterChips
              chips={chips}
              onClearAll={() => filters.clear()}
              summary={resultSummary}
            />
          }
        />

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
            {renderTable(filteredBindings)}
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
