'use client';

import { useState, useEffect, useCallback } from 'react';
import { gitopsApplications, type GitOpsAppSummary } from '@/lib/api';
import BrandIcon from '@/components/BrandIcon';
import Link from 'next/link';
import ConfirmModal from '@/components/ConfirmModal';

const healthStyles: Record<string, string> = {
  healthy: 'bg-emerald-500/15 text-emerald-600',
  progressing: 'bg-blue-500/15 text-blue-500',
  degraded: 'bg-red-500/15 text-red-500',
  suspended: 'bg-yellow-500/15 text-yellow-600',
  unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
};

const healthLabels: Record<string, string> = {
  healthy: 'Healthy',
  progressing: 'Progressing',
  degraded: 'Degraded',
  suspended: 'Suspended',
  unknown: 'Unknown',
};

const syncStyles: Record<string, string> = {
  synced: 'bg-emerald-500/15 text-emerald-600',
  out_of_sync: 'bg-orange-500/15 text-orange-500',
  unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
};

const syncLabels: Record<string, string> = {
  synced: 'Synced',
  out_of_sync: 'Out of Sync',
  unknown: 'Unknown',
};

function engineBadge(engine: string) {
  const styles: Record<string, string> = {
    fluxcd: 'bg-purple-500/15 text-purple-600',
    argocd: 'bg-orange-500/15 text-orange-600',
  };
  const labels: Record<string, string> = { fluxcd: 'FluxCD', argocd: 'ArgoCD' };
  return (
    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${styles[engine] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
      {labels[engine] || engine}
    </span>
  );
}

export default function ReleasesPage() {
  const [apps, setApps] = useState<GitOpsAppSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [filter, setFilter] = useState('');
  const [healthFilter, setHealthFilter] = useState('');
  const [engineFilter, setEngineFilter] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionConfirm, setActionConfirm] = useState<{ app: GitOpsAppSummary; action: string } | null>(null);
  const [actionResult, setActionResult] = useState<{ ok: boolean; text: string } | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await gitopsApplications.list();
      setApps(res.applications || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load releases');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh for progressing apps
  useEffect(() => {
    const hasProgressing = apps.some(a => a.health === 'progressing' || a.sync_status === 'out_of_sync');
    if (!hasProgressing) return;
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [apps, load]);

  const filtered = apps.filter(a => {
    if (filter) {
      const q = filter.toLowerCase();
      if (!a.name.toLowerCase().includes(q) && !a.namespace.toLowerCase().includes(q) && !(a.project || '').toLowerCase().includes(q)) return false;
    }
    if (healthFilter && a.health !== healthFilter) return false;
    if (engineFilter && a.engine_type !== engineFilter) return false;
    return true;
  });

  const stats = {
    total: apps.length,
    healthy: apps.filter(a => a.health === 'healthy').length,
    degraded: apps.filter(a => a.health === 'degraded').length,
    progressing: apps.filter(a => a.health === 'progressing').length,
    outOfSync: apps.filter(a => a.sync_status === 'out_of_sync').length,
  };

  const handleAction = async (app: GitOpsAppSummary, action: string) => {
    const connId = app.connection_id;
    if (!connId) {
      setActionResult({ ok: false, text: `Cannot ${action}: application connection not found` });
      return;
    }
    setActionLoading(`${app.name}-${action}`);
    setActionResult(null);
    try {
      switch (action) {
        case 'sync':
          await gitopsApplications.sync(connId, app.namespace, app.name);
          setActionResult({ ok: true, text: `Sync triggered for ${app.name}` });
          break;
        case 'refresh':
          await gitopsApplications.refresh(connId, app.namespace, app.name);
          setActionResult({ ok: true, text: `Refresh triggered for ${app.name}` });
          break;
        case 'terminate':
          await gitopsApplications.terminate(connId, app.namespace, app.name);
          setActionResult({ ok: true, text: `Terminate triggered for ${app.name}` });
          break;
      }
      await load();
    } catch {
      setActionResult({ ok: false, text: `Failed to ${action} ${app.name}` });
    }
    setActionLoading(null);
    setActionConfirm(null);
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Releases</h1>
            <p className="page-subtitle-modern">Active GitOps releases across clusters</p>
          </div>
          <button onClick={() => { setLoading(true); load(); }} className="btn btn-secondary text-xs">
            <BrandIcon name="argocd" size={14} /> Refresh
          </button>
        </div>

        {/* Feedback */}
        {actionResult && (
          <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 page-animate-up ${
            actionResult.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
          }`}>
            <p className={`text-sm font-medium ${actionResult.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {actionResult.ok ? '✓ ' : '⚠ '}{actionResult.text}
            </p>
            <button onClick={() => setActionResult(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
          </div>
        )}

        {/* Stats */}
        <div className="grid grid-cols-5 gap-4 page-animate-up page-delay-1">
          {[
            { label: 'Total Releases', value: stats.total, color: 'text-[var(--text-primary)]', icon: 'argocd', iconClass: 'stat-icon-blue' },
            { label: 'Healthy', value: stats.healthy, color: 'text-emerald-600', icon: 'vault', iconClass: 'stat-icon-green' },
            { label: 'Degraded', value: stats.degraded, color: 'text-red-600', icon: 'vault', iconClass: 'stat-icon-red' },
            { label: 'Progressing', value: stats.progressing, color: 'text-blue-600', icon: 'cicd', iconClass: 'stat-icon-amber' },
            { label: 'Out of Sync', value: stats.outOfSync, color: 'text-orange-600', icon: 'gitops', iconClass: 'stat-icon-amber' },
          ].map(s => (
            <div key={s.label} className="modern-stat-card flex items-center gap-3">
              <div className={`w-10 h-10 rounded-xl ${s.iconClass} flex items-center justify-center text-white text-sm`}>
                <BrandIcon name={s.icon} size={20} monochrome />
              </div>
              <div>
                <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
                <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
              </div>
            </div>
          ))}
        </div>

        {/* Filters */}
        <div className="flex gap-3 page-animate-up page-delay-2">
          <input
            type="text"
            placeholder="Search releases..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            className="input flex-1 max-w-xs"
          />
          <select value={healthFilter} onChange={e => setHealthFilter(e.target.value)} className="input w-40">
            <option value="">All health</option>
            <option value="healthy">Healthy</option>
            <option value="progressing">Progressing</option>
            <option value="degraded">Degraded</option>
            <option value="suspended">Suspended</option>
          </select>
          <select value={engineFilter} onChange={e => setEngineFilter(e.target.value)} className="input w-40">
            <option value="">All engines</option>
            <option value="argocd">ArgoCD</option>
            <option value="fluxcd">FluxCD</option>
          </select>
        </div>

        {/* Table */}
        {loading ? (
          <div className="card card-body text-center py-12">
            <p className="text-[13px] text-[var(--text-tertiary)]">Loading releases...</p>
          </div>
        ) : loadError ? (
          <div className="card card-body text-center py-12">
            <div className="text-4xl mb-3 opacity-30 flex items-center justify-center">⚠️</div>
            <p className="text-[13px] text-red-500 mb-1">Failed to load releases</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mb-4">{loadError}</p>
            <button onClick={() => { setLoading(true); load(); }} className="btn btn-primary inline-block">Retry</button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="card card-body text-center py-12">
            <div className="text-4xl mb-3 opacity-30 flex items-center justify-center">📦</div>
            <p className="text-[13px] text-[var(--text-secondary)] mb-1">
              {apps.length === 0 ? 'No releases found' : 'No releases match your filters'}
            </p>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              {apps.length === 0
                ? 'Configure GitOps repositories and deploy applications to see releases here'
                : 'Try adjusting your search or filter criteria'}
            </p>
            {apps.length === 0 && (
              <Link href="/gitops" className="btn btn-primary inline-block mt-4">Go to GitOps</Link>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-[var(--border)]" style={{ borderRadius: '12px' }}>
            <table style={{ minWidth: 1100 }}>
              <colgroup>
                <col style={{ width: 220, minWidth: 180 }} />
                <col style={{ width: 120, minWidth: 100 }} />
                <col style={{ width: 100, minWidth: 90 }} />
                <col style={{ width: 120, minWidth: 100 }} />
                <col style={{ width: 120, minWidth: 100 }} />
                <col style={{ width: 120, minWidth: 100 }} />
                <col style={{ width: 120, minWidth: 100 }} />
                <col style={{ width: 'auto', minWidth: 200 }} />
              </colgroup>
              <thead>
                <tr>
                  <th className="!px-3">Release</th>
                  <th className="!px-3">Namespace</th>
                  <th className="!px-3">Engine</th>
                  <th className="!px-3">Health</th>
                  <th className="!px-3">Sync</th>
                  <th className="!px-3">Revision</th>
                  <th className="!px-3">Environment</th>
                  <th className="!px-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(app => {
                  const actionKey = `${app.name}`;
                  return (
                    <tr key={`${app.connection_id}-${app.namespace}-${app.name}`} className="border-b border-[var(--border-light)] last:border-b-0 hover:bg-[var(--bg)]">
                      <td className="!px-2 !py-2.5">
                        <div className="font-medium text-[var(--text-primary)]">{app.name}</div>
                        {app.project && (
                          <p className="text-[10px] text-[var(--text-tertiary)]">Project: {app.project}</p>
                        )}
                      </td>
                      <td className="!px-2 !py-2.5">
                        <span className="text-[12px] font-mono text-[var(--text-secondary)]">{app.namespace}</span>
                      </td>
                      <td className="!px-2 !py-2.5">
                        {engineBadge(app.engine_type)}
                      </td>
                      <td className="!px-2 !py-2.5">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${healthStyles[app.health] || healthStyles.unknown}`}>
                          {healthLabels[app.health] || app.health}
                        </span>
                      </td>
                      <td className="!px-2 !py-2.5">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${syncStyles[app.sync_status] || syncStyles.unknown}`}>
                          {syncLabels[app.sync_status] || app.sync_status}
                        </span>
                      </td>
                      <td className="!px-2 !py-2.5">
                        <span className="text-[11px] font-mono text-[var(--text-tertiary)] truncate max-w-[100px] block" title={app.revision}>
                          {app.revision ? app.revision.slice(0, 8) : '—'}
                        </span>
                      </td>
                      <td className="!px-2 !py-2.5">
                        {app.environment ? (
                          <span className="text-[11px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] font-medium">
                            {app.environment}
                          </span>
                        ) : (
                          <span className="text-[11px] text-[var(--text-tertiary)]">—</span>
                        )}
                      </td>
                      <td className="!px-2 !py-2.5">
                        <div className="flex gap-1 flex-wrap">
                          <button
                            onClick={() => handleAction(app, 'sync')}
                            disabled={actionLoading === `${actionKey}-sync`}
                            className="text-[11px] px-2 py-1 bg-blue-500/10 text-blue-500 rounded hover:bg-blue-500/15 disabled:opacity-50"
                          >
                            {actionLoading === `${actionKey}-sync` ? '...' : 'Sync'}
                          </button>
                          <button
                            onClick={() => handleAction(app, 'refresh')}
                            disabled={actionLoading === `${actionKey}-refresh`}
                            className="text-[11px] px-2 py-1 bg-emerald-500/10 text-emerald-600 rounded hover:bg-emerald-500/15 disabled:opacity-50"
                          >
                            {actionLoading === `${actionKey}-refresh` ? '...' : 'Refresh'}
                          </button>
                          <button
                            onClick={() => setActionConfirm({ app, action: 'terminate' })}
                            className="text-[11px] px-2 py-1 bg-red-500/5 text-red-400 rounded hover:bg-red-500/10"
                          >
                            Terminate
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Confirm Modal */}
      {actionConfirm && (
        <ConfirmModal
          open
          title={`Terminate ${actionConfirm.app.name}?`}
          description="This will terminate the running application. This action may be irreversible depending on the application configuration."
          confirmLabel="Terminate"
          variant="danger"
          onConfirm={() => handleAction(actionConfirm.app, actionConfirm.action)}
          onCancel={() => setActionConfirm(null)}
        />
      )}
    </div>
  );
}
