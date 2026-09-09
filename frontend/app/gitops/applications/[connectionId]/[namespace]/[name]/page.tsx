'use client';

import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { gitopsApplications, type GitOpsAppDetail, type GitOpsHistoryEntry, type GitOpsResourceNode } from '@/lib/api';

function healthBadge(health: string) {
  const styles: Record<string, string> = {
    Healthy: 'bg-emerald-500/15 text-emerald-600', Ready: 'bg-emerald-500/15 text-emerald-600',
    Degraded: 'bg-red-500/15 text-red-500', NotReady: 'bg-red-500/15 text-red-500',
    Progressing: 'bg-blue-500/15 text-blue-500', Reconciling: 'bg-blue-500/15 text-blue-500',
    Suspended: 'bg-yellow-500/15 text-yellow-600', Unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  return <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[health] || styles.Unknown}`}>{health || 'Unknown'}</span>;
}

function syncBadge(status: string) {
  const styles: Record<string, string> = { Synced: 'bg-emerald-500/15 text-emerald-600', OutOfSync: 'bg-orange-500/15 text-orange-500', Unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]' };
  return <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[status] || styles.Unknown}`}>{status || 'Unknown'}</span>;
}

type Tab = 'overview' | 'tree' | 'history' | 'events';

export default function ApplicationDetailPage() {
  const params = useParams();
  const router = useRouter();
  const connectionId = params.connectionId as string;
  const namespace = params.namespace as string;
  const name = params.name as string;

  const [detail, setDetail] = useState<GitOpsAppDetail | null>(null);
  const [history, setHistory] = useState<GitOpsHistoryEntry[]>([]);
  const [tree, setTree] = useState<GitOpsResourceNode[]>([]);
  const [events, setEvents] = useState<Record<string, unknown>[]>([]);
  const [eventsError, setEventsError] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [tab, setTab] = useState<Tab>('overview');
  const [actionLoading, setActionLoading] = useState('');
  const [actionResult, setActionResult] = useState<{ ok: boolean; msg: string } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const d = await gitopsApplications.get(connectionId, namespace, name);
      setDetail(d);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load');
    }
    setLoading(false);
  }, [connectionId, namespace, name]);

  useEffect(() => { load(); }, [load]);

  const loadTab = useCallback(async (t: Tab) => {
    try {
      if (t === 'history') {
        const res = await gitopsApplications.history(connectionId, namespace, name);
        setHistory(res.history || []);
      } else if (t === 'tree') {
        const res = await gitopsApplications.tree(connectionId, namespace, name);
        setTree((res as unknown as { nodes?: GitOpsResourceNode[] })?.nodes || []);
      } else if (t === 'events') {
        setEventsError('');
        const res = await gitopsApplications.events(connectionId, namespace, name);
        setEvents((res.events || []) as Record<string, unknown>[]);
      }
    } catch (err) {
      if (t === 'events') {
        setEventsError(err instanceof Error ? err.message : 'Failed to load events');
      }
    }
  }, [connectionId, namespace, name]);

  useEffect(() => { loadTab(tab); }, [tab, loadTab]);

  const doAction = async (label: string, fn: () => Promise<unknown>) => {
    setActionLoading(label);
    setActionResult(null);
    try {
      await fn();
      setActionResult({ ok: true, msg: `${label} completed` });
      load();
    } catch (err) {
      setActionResult({ ok: false, msg: err instanceof Error ? err.message : `${label} failed` });
    }
    setActionLoading('');
  };

  const caps = detail?.capabilities;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div className="flex items-center gap-3">
            <button onClick={() => router.push('/gitops/applications')} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-lg">&larr;</button>
            <div>
              <h1 className="page-title-modern">{name}</h1>
              <p className="page-subtitle-modern">{namespace} &middot; {detail?.engine_type || '...'}</p>
            </div>
            {detail && healthBadge(detail.health)}
            {detail && syncBadge(detail.sync_status)}
          </div>
          {/* Action Buttons */}
          {detail && (
            <div className="flex items-center gap-2">
              {caps?.refresh && (
                <button disabled={!!actionLoading} onClick={() => doAction('Refresh', () => gitopsApplications.refresh(connectionId, namespace, name))}
                  className="btn btn-secondary text-[11px] disabled:opacity-50">
                  {actionLoading === 'Refresh' ? '...' : 'Refresh'}
                </button>
              )}
              <button disabled={!!actionLoading} onClick={() => doAction('Sync', () => gitopsApplications.sync(connectionId, namespace, name))}
                className="btn btn-secondary text-[11px] disabled:opacity-50">
                {actionLoading === 'Sync' ? '...' : 'Sync'}
              </button>
              {caps?.auto_sync && (
                <button disabled={!!actionLoading} onClick={() => doAction('Toggle Auto-Sync', () => gitopsApplications.setAutoSync(connectionId, namespace, name, true))}
                  className="btn btn-secondary text-[11px] disabled:opacity-50">
                  {actionLoading === 'Toggle Auto-Sync' ? '...' : 'Auto-Sync'}
                </button>
              )}
              <button disabled={!!actionLoading} onClick={() => doAction('Terminate', () => gitopsApplications.terminate(connectionId, namespace, name))}
                className="text-[11px] px-3 py-1.5 text-red-500 border border-red-500/30 hover:bg-red-500/10 rounded-lg transition-colors disabled:opacity-50">
                {actionLoading === 'Terminate' ? '...' : 'Terminate'}
              </button>
            </div>
          )}
        </div>

        {/* Action Result */}
        {actionResult && (
          <div className={`rounded-lg p-3 text-[12px] ${actionResult.ok ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-600' : 'bg-red-500/10 border border-red-500/20 text-red-500'}`}>
            {actionResult.msg}
            <button onClick={() => setActionResult(null)} className="ml-3 text-[10px] underline">Dismiss</button>
          </div>
        )}

        {error && <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-4 text-[12px] text-red-500">{error}</div>}

        {/* Tabs */}
        <div className="flex gap-1 border-b border-[var(--border)]">
          {(['overview', 'tree', 'history', 'events'] as Tab[]).map(t => (
            <button key={t} onClick={() => setTab(t)}
              className={`px-4 py-2 text-[12px] font-medium rounded-t-lg transition-colors ${tab === t ? 'bg-[var(--bg)] text-[var(--accent)] border-b-2 border-[var(--accent)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}>
              {t.charAt(0).toUpperCase() + t.slice(1)}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        {loading ? (
          <div className="card card-body text-center py-12"><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div>
        ) : tab === 'overview' && detail ? (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div className="card p-5 space-y-3">
              <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Application Info</h3>
              <InfoRow label="Engine" value={detail.engine_type} />
              <InfoRow label="Namespace" value={detail.namespace} />
              <InfoRow label="Health" value={detail.health} />
              <InfoRow label="Sync Status" value={detail.sync_status} />
              <InfoRow label="Revision" value={detail.revision ? detail.revision.substring(0, 12) : '—'} mono />
              {detail.project && <InfoRow label="Project" value={detail.project} />}
              {detail.environment && <InfoRow label="Environment" value={detail.environment} />}
            </div>
            <div className="card p-5 space-y-3">
              <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Capabilities</h3>
              {caps && (
                <div className="grid grid-cols-2 gap-2">
                  <CapItem label="Diff" ok={caps.diff} />
                  <CapItem label="History" ok={caps.history !== 'none'} text={caps.history} />
                  <CapItem label="Resource Tree" ok={caps.resource_tree} />
                  <CapItem label="Events" ok={caps.events} />
                  <CapItem label="Logs" ok={caps.logs} />
                  <CapItem label="Refresh" ok={caps.refresh} />
                  <CapItem label="Auto-Sync" ok={caps.auto_sync} />
                  <CapItem label="Projects" ok={caps.projects} />
                </div>
              )}
            </div>
            {detail.source && (
              <div className="card p-5 space-y-3">
                <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Source</h3>
                <InfoRow label="Repo" value={detail.source.repo_url} mono />
                {detail.source.path && <InfoRow label="Path" value={detail.source.path} mono />}
                <InfoRow label="Target" value={detail.source.target_revision} />
              </div>
            )}
            {detail.destination && (
              <div className="card p-5 space-y-3">
                <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Destination</h3>
                {detail.destination.server && <InfoRow label="Server" value={detail.destination.server} mono />}
                <InfoRow label="Namespace" value={detail.destination.namespace} />
              </div>
            )}
          </div>
        ) : tab === 'tree' ? (
          <div className="card overflow-hidden">
            {tree.length === 0 ? (
              <div className="p-8 text-center text-[13px] text-[var(--text-tertiary)]">No resources found</div>
            ) : (
              <table className="w-full">
                <thead><tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase px-4 py-2">Kind</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase px-4 py-2">Name</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase px-4 py-2">Namespace</th>
                  <th className="text-left text-[11px] font-semibold text-[var(--text-tertiary)] uppercase px-4 py-2">Status</th>
                </tr></thead>
                <tbody className="divide-y divide-[var(--border-light)]">
                  {tree.map((n, i) => (
                    <tr key={`${n.kind}-${n.name}-${i}`} className="hover:bg-[var(--bg)]">
                      <td className="px-4 py-2 text-[12px] font-medium text-[var(--text-primary)]">{n.kind}</td>
                      <td className="px-4 py-2 text-[12px] font-mono text-[var(--text-secondary)]">{n.name}</td>
                      <td className="px-4 py-2 text-[12px] font-mono text-[var(--text-tertiary)]">{n.namespace}</td>
                      <td className="px-4 py-2">{n.health ? healthBadge(n.health) : <span className="text-[11px] text-[var(--text-tertiary)]">{n.status || '—'}</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        ) : tab === 'history' ? (
          <div className="card overflow-hidden">
            {history.length === 0 ? (
              <div className="p-8 text-center text-[13px] text-[var(--text-tertiary)]">No history available</div>
            ) : (
              <div className="divide-y divide-[var(--border-light)]">
                {history.map((h, i) => (
                  <div key={i} className="px-5 py-3 hover:bg-[var(--bg)]">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <span className="text-[12px] font-mono text-[var(--text-primary)]">{h.revision ? h.revision.substring(0, 8) : `#${h.id}`}</span>
                        {healthBadge(h.status)}
                      </div>
                      <span className="text-[11px] text-[var(--text-tertiary)]">{h.deployed_at ? new Date(h.deployed_at).toLocaleString() : ''}</span>
                    </div>
                    {h.message && <p className="text-[11px] text-[var(--text-tertiary)] mt-1">{h.message}</p>}
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : tab === 'events' ? (
          <div className="card overflow-hidden">
            {eventsError ? (
              <div className="p-5 bg-red-500/10 border border-red-500/20 rounded-lg m-4 text-[12px] text-red-500">
                Failed to load events: {eventsError}
              </div>
            ) : events.length === 0 ? (
              <div className="p-8 text-center text-[13px] text-[var(--text-tertiary)]">No events found</div>
            ) : (
              <div className="divide-y divide-[var(--border-light)]">
                {events.map((e, i) => (
                  <div key={i} className="px-5 py-3 hover:bg-[var(--bg)]">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${String(e.type) === 'Normal' ? 'bg-blue-500/15 text-blue-500' : 'bg-yellow-500/15 text-yellow-600'}`}>
                        {String(e.type || 'Normal')}
                      </span>
                      <span className="text-[11px] font-medium text-[var(--text-primary)]">{String(e.reason || '')}</span>
                    </div>
                    <p className="text-[11px] text-[var(--text-secondary)]">{String(e.message || '')}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start gap-2 text-[12px]">
      <span className="text-[var(--text-tertiary)] w-24 shrink-0">{label}:</span>
      <span className={`text-[var(--text-secondary)] break-all ${mono ? 'font-mono' : ''}`}>{value}</span>
    </div>
  );
}

function CapItem({ label, ok, text }: { label: string; ok: boolean; text?: string }) {
  return (
    <div className="flex items-center gap-2 text-[11px]">
      <span className={ok ? 'text-emerald-500' : 'text-[var(--text-tertiary)]'}>{ok ? '✓' : '—'}</span>
      <span className="text-[var(--text-secondary)]">{label}</span>
      {text && <span className="text-[var(--text-tertiary)] text-[10px]">({text})</span>}
    </div>
  );
}
