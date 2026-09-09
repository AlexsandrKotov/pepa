'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { gitopsApplications, type GitOpsAppSummary } from '@/lib/api';

function engineBadge(engine: string) {
  const styles: Record<string, string> = {
    fluxcd: 'bg-purple-500/15 text-purple-600',
    argocd: 'bg-orange-500/15 text-orange-600',
  };
  const labels: Record<string, string> = {
    fluxcd: 'FluxCD',
    argocd: 'ArgoCD',
  };
  return (
    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[engine] || 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
      {labels[engine] || engine}
    </span>
  );
}

function healthBadge(health: string) {
  const styles: Record<string, string> = {
    Healthy: 'bg-emerald-500/15 text-emerald-600',
    Ready: 'bg-emerald-500/15 text-emerald-600',
    Degraded: 'bg-red-500/15 text-red-500',
    NotReady: 'bg-red-500/15 text-red-500',
    Progressing: 'bg-blue-500/15 text-blue-500',
    Reconciling: 'bg-blue-500/15 text-blue-500',
    Suspended: 'bg-yellow-500/15 text-yellow-600',
    Missing: 'bg-gray-500/15 text-gray-500',
    Unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  return (
    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[health] || styles.Unknown}`}>
      {health || 'Unknown'}
    </span>
  );
}

function syncBadge(status: string) {
  const styles: Record<string, string> = {
    Synced: 'bg-emerald-500/15 text-emerald-600',
    OutOfSync: 'bg-orange-500/15 text-orange-500',
    Unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  return (
    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[status] || styles.Unknown}`}>
      {status || 'Unknown'}
    </span>
  );
}

export default function GitOpsApplicationsPage() {
  const router = useRouter();
  const [apps, setApps] = useState<GitOpsAppSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [filterEngine, setFilterEngine] = useState('');
  const [filterHealth, setFilterHealth] = useState('');
  const [search, setSearch] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const params: Record<string, string> = {};
      if (filterEngine) params.engine = filterEngine;
      if (filterHealth) params.health = filterHealth;
      const res = await gitopsApplications.list(params);
      setApps(res.applications || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load applications');
      setApps([]);
    }
    setLoading(false);
  }, [filterEngine, filterHealth]);

  useEffect(() => { load(); }, [load]);

  const filteredApps = search
    ? apps.filter(a =>
        a.name.toLowerCase().includes(search.toLowerCase()) ||
        a.namespace.toLowerCase().includes(search.toLowerCase())
      )
    : apps;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Applications</h1>
            <p className="page-subtitle-modern">Unified view of ArgoCD and FluxCD applications</p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => router.push('/gitops')} className="btn btn-secondary text-[12px]">
              Repositories
            </button>
            <button onClick={() => router.push('/gitops/drift')} className="btn btn-secondary text-[12px]">
              Drift Detection
            </button>
          </div>
        </div>

        {/* Filters */}
        <div className="flex items-center gap-3 flex-wrap">
          <input
            type="text"
            placeholder="Search applications..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="input text-[12px] w-64"
          />
          <select
            value={filterEngine}
            onChange={e => setFilterEngine(e.target.value)}
            className="input text-[12px] w-36"
          >
            <option value="">All Engines</option>
            <option value="argocd">ArgoCD</option>
            <option value="fluxcd">FluxCD</option>
          </select>
          <select
            value={filterHealth}
            onChange={e => setFilterHealth(e.target.value)}
            className="input text-[12px] w-36"
          >
            <option value="">All Health</option>
            <option value="Healthy">Healthy</option>
            <option value="Degraded">Degraded</option>
            <option value="Progressing">Progressing</option>
            <option value="Suspended">Suspended</option>
          </select>
          <button onClick={load} className="text-[12px] px-3 py-1.5 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">
            Refresh
          </button>
        </div>

        {/* Error */}
        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-4 text-[12px] text-red-500">
            {error}
          </div>
        )}

        {/* Applications Table */}
        {loading ? (
          <div className="card card-body text-center py-12">
            <p className="text-[13px] text-[var(--text-tertiary)]">Loading applications...</p>
          </div>
        ) : filteredApps.length === 0 ? (
          <div className="card card-body text-center py-16">
            <div className="text-5xl mb-4 opacity-20">🚀</div>
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">
              {apps.length === 0 ? 'No applications found' : 'No applications match the filter'}
            </p>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              {apps.length === 0
                ? 'Configure ArgoCD or FluxCD connections to discover applications'
                : 'Try adjusting your filters'}
            </p>
          </div>
        ) : (
          <div className="card overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Application</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Engine</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Namespace</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Health</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Sync</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider px-4 py-3">Revision</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border-light)]">
                {filteredApps.map((app, idx) => (
                  <tr
                    key={`${app.engine_type}-${app.namespace}-${app.name}-${idx}`}
                    className="hover:bg-[var(--bg)] transition-colors cursor-pointer"
                    onClick={() => {
                      if (app.connection_id) {
                        router.push(`/gitops/applications/${app.connection_id}/${app.namespace}/${app.name}`);
                      }
                    }}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <span className="text-sm">{app.engine_type === 'argocd' ? '🚀' : '🔧'}</span>
                        <span className="text-[13px] font-medium text-[var(--text-primary)]">{app.name}</span>
                        {app.project && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 font-mono">{app.project}</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">{engineBadge(app.engine_type)}</td>
                    <td className="px-4 py-3 text-[12px] font-mono text-[var(--text-secondary)]">{app.namespace}</td>
                    <td className="px-4 py-3">{healthBadge(app.health)}</td>
                    <td className="px-4 py-3">{syncBadge(app.sync_status)}</td>
                    <td className="px-4 py-3 text-[11px] font-mono text-[var(--text-tertiary)]">
                      {app.revision ? app.revision.substring(0, 8) : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="px-4 py-2 border-t border-[var(--border)] bg-[var(--bg)] text-[11px] text-[var(--text-tertiary)]">
              {filteredApps.length} application{filteredApps.length !== 1 ? 's' : ''}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
