'use client';

import { useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { discovery, gitopsApplications, dockerServices, dockerHosts, type DiscoveredService, type GitOpsAppSummary, type DockerService, type DiscoveredDockerContainer, type DockerHost } from '@/lib/api';
import { useDebounce } from '@/hooks/useDebounce';
import Tabs from '@/components/Tabs';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

function WorkloadsPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeTab = searchParams.get('tab') || 'kubernetes';

  const handleTabChange = useCallback((tab: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set('tab', tab);
    router.replace(`?${params.toString()}`, { scroll: false });
  }, [searchParams, router]);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Workloads</h1>
            <p className="page-subtitle-modern">All running workloads across Kubernetes, GitOps, and Docker</p>
          </div>
        </div>

        {/* Tabs */}
        <Tabs
          activeKey={activeTab}
          onChange={handleTabChange}
          variant="pills"
          tabs={[
            { key: 'kubernetes', label: 'Kubernetes', icon: 'kubernetes' },
            { key: 'gitops', label: 'GitOps Releases', icon: 'gitops' },
            { key: 'docker', label: 'Docker Containers', icon: 'docker' },
          ]}
          className="page-animate-up"
        />

        {/* Tab Content */}
        <div className="page-animate-up page-delay-1">
          {activeTab === 'kubernetes' && <KubernetesTab />}
          {activeTab === 'gitops' && <GitOpsTab />}
          {activeTab === 'docker' && <DockerTab />}
        </div>
      </div>
    </div>
  );
}

export default function WorkloadsPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <WorkloadsPageContent />
    </Suspense>
  );
}

// ─── Kubernetes Tab ─────────────────────────────────────────────────────────

function KubernetesTab() {
  const [services, setServices] = useState<DiscoveredService[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [clusterFilter, setClusterFilter] = useState('');
  const [namespaceFilter, setNamespaceFilter] = useState('');
  const [healthFilter, setHealthFilter] = useState('');
  const debouncedSearch = useDebounce(search, 300);

  const load = useCallback(async () => {
    try {
      const params: Record<string, string> = {};
      if (debouncedSearch) params.search = debouncedSearch;
      if (clusterFilter) params.cluster = clusterFilter;
      if (namespaceFilter) params.namespace = namespaceFilter;
      if (healthFilter) params.health = healthFilter;
      const data = await discovery.services(params);
      setServices(data.services || []);
    } catch {
      setServices([]);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, clusterFilter, namespaceFilter, healthFilter]);

  useEffect(() => { load(); }, [load]);

  const clusters = useMemo(() => [...new Set(services.map(s => s.cluster || 'default'))].sort(), [services]);
  const namespaces = useMemo(() => [...new Set(services.map(s => s.namespace))].sort(), [services]);

  const stats = useMemo(() => ({
    total: services.length,
    healthy: services.filter(s => s.health === 'healthy').length,
    degraded: services.filter(s => s.health === 'degraded' || s.health === 'failed').length,
    progressing: services.filter(s => s.health === 'progressing').length,
  }), [services]);

  const healthDot = (h: string) => {
    const c = h === 'healthy' ? 'bg-green-500' : h === 'degraded' || h === 'failed' ? 'bg-red-500' : h === 'progressing' ? 'bg-blue-500' : 'bg-gray-400';
    return <div className={`w-2 h-2 rounded-full ${c}`} />;
  };

  return (
    <div className="space-y-4">
      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { label: 'Total Resources', value: stats.total, color: 'text-[var(--text-primary)]' },
          { label: 'Healthy', value: stats.healthy, color: 'text-emerald-600' },
          { label: 'Degraded', value: stats.degraded, color: 'text-red-600' },
          { label: 'Progressing', value: stats.progressing, color: 'text-blue-600' },
        ].map(s => (
          <div key={s.label} className="card card-body py-3 flex items-center gap-3">
            <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
            <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <input type="text" placeholder="Search resources..." value={search} onChange={e => setSearch(e.target.value)} className="input flex-1 max-w-xs" />
        <select value={clusterFilter} onChange={e => setClusterFilter(e.target.value)} className="input w-40">
          <option value="">All Clusters</option>
          {clusters.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
        <select value={namespaceFilter} onChange={e => setNamespaceFilter(e.target.value)} className="input w-40">
          <option value="">All Namespaces</option>
          {namespaces.map(n => <option key={n} value={n}>{n}</option>)}
        </select>
        <select value={healthFilter} onChange={e => setHealthFilter(e.target.value)} className="input w-36">
          <option value="">All Health</option>
          <option value="healthy">Healthy</option>
          <option value="degraded">Degraded</option>
          <option value="progressing">Progressing</option>
        </select>
      </div>

      {/* Table */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading Kubernetes resources...</p>
        </div>
      ) : services.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30">&#x2388;</div>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1">No Kubernetes resources found</p>
          <p className="text-[12px] text-[var(--text-tertiary)]">Connect a Kubernetes cluster to discover workloads</p>
        </div>
      ) : (
        <div className="card">
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 20 }}></th>
                  <th>Name</th>
                  <th>Cluster</th>
                  <th>Namespace</th>
                  <th>Source</th>
                  <th>Status</th>
                  <th>Health</th>
                  <th>Replicas</th>
                </tr>
              </thead>
              <tbody>
                {services.map((svc, idx) => (
                  <tr key={`${svc.source}-${svc.name}-${idx}`} className="hover:bg-[var(--border-light)] transition-colors">
                    <td>{healthDot(svc.health)}</td>
                    <td><span className="font-medium text-[var(--text-primary)]">{svc.name}</span></td>
                    <td className="text-[12px] text-[var(--text-secondary)]">{svc.cluster || 'default'}</td>
                    <td className="text-[12px] text-[var(--text-secondary)]">{svc.namespace}</td>
                    <td>
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] font-medium">
                        {svc.source}
                      </span>
                    </td>
                    <td>
                      <span className={`text-[12px] font-medium ${svc.status === 'running' || svc.status === 'active' ? 'text-green-600' : svc.status === 'error' || svc.status === 'failed' ? 'text-red-600' : 'text-[var(--text-secondary)]'}`}>
                        {svc.status}
                      </span>
                    </td>
                    <td>
                      <span className={`text-[12px] font-medium ${svc.health === 'healthy' ? 'text-green-600' : svc.health === 'degraded' || svc.health === 'failed' ? 'text-red-600' : 'text-[var(--text-secondary)]'}`}>
                        {svc.health}
                      </span>
                    </td>
                    <td className="text-[12px] text-[var(--text-secondary)]">
                      {svc.replicas > 0 ? `${svc.ready_replicas}/${svc.replicas}` : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="px-4 py-2 border-t border-[var(--border)] text-[11px] text-[var(--text-tertiary)]">
            {services.length} resource{services.length !== 1 ? 's' : ''}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── GitOps Tab ─────────────────────────────────────────────────────────────

function GitOpsTab() {
  const router = useRouter();
  const [apps, setApps] = useState<GitOpsAppSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [healthFilter, setHealthFilter] = useState('');
  const [engineFilter, setEngineFilter] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionConfirm, setActionConfirm] = useState<{ app: GitOpsAppSummary; action: string } | null>(null);
  const [actionResult, setActionResult] = useState<{ ok: boolean; text: string } | null>(null);

  const normalizeHealth = (h: string) => {
    const map: Record<string, string> = { Healthy: 'healthy', Ready: 'healthy', Progressing: 'progressing', Reconciling: 'progressing', Degraded: 'degraded', NotReady: 'degraded', Suspended: 'suspended' };
    return map[h] || h;
  };
  const normalizeSync = (s: string) => {
    const map: Record<string, string> = { Synced: 'synced', OutOfSync: 'out_of_sync' };
    return map[s] || s;
  };

  const healthStyles: Record<string, string> = {
    healthy: 'bg-emerald-500/15 text-emerald-600', progressing: 'bg-blue-500/15 text-blue-500',
    degraded: 'bg-red-500/15 text-red-500', suspended: 'bg-yellow-500/15 text-yellow-600',
    unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  const syncStyles: Record<string, string> = {
    synced: 'bg-emerald-500/15 text-emerald-600', out_of_sync: 'bg-orange-500/15 text-orange-500',
    unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };

  const load = useCallback(async () => {
    try {
      const res = await gitopsApplications.list();
      setApps(res.applications || []);
    } catch { setApps([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh
  useEffect(() => {
    const hasProgressing = apps.some(a => normalizeHealth(a.health) === 'progressing' || normalizeSync(a.sync_status) === 'out_of_sync');
    if (!hasProgressing) return;
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [apps, load]);

  const filtered = apps.filter(a => {
    if (search) {
      const q = search.toLowerCase();
      if (!a.name.toLowerCase().includes(q) && !a.namespace.toLowerCase().includes(q)) return false;
    }
    if (healthFilter && normalizeHealth(a.health) !== healthFilter) return false;
    if (engineFilter && a.engine_type !== engineFilter) return false;
    return true;
  });

  const stats = {
    total: apps.length,
    healthy: apps.filter(a => normalizeHealth(a.health) === 'healthy').length,
    degraded: apps.filter(a => normalizeHealth(a.health) === 'degraded').length,
    outOfSync: apps.filter(a => normalizeSync(a.sync_status) === 'out_of_sync').length,
  };

  const handleAction = async (app: GitOpsAppSummary, action: string) => {
    const connId = app.connection_id;
    if (!connId) { setActionResult({ ok: false, text: `Cannot ${action}: connection not found` }); return; }
    setActionLoading(`${app.name}-${action}`);
    setActionResult(null);
    try {
      if (action === 'sync') await gitopsApplications.sync(connId, app.namespace, app.name);
      else if (action === 'refresh') await gitopsApplications.refresh(connId, app.namespace, app.name);
      else if (action === 'terminate') await gitopsApplications.terminate(connId, app.namespace, app.name);
      setActionResult({ ok: true, text: `${action.charAt(0).toUpperCase() + action.slice(1)} triggered for ${app.name}` });
      await load();
    } catch { setActionResult({ ok: false, text: `Failed to ${action} ${app.name}` }); }
    setActionLoading(null);
    setActionConfirm(null);
  };

  const engineBadge = (engine: string) => {
    const s: Record<string, string> = { fluxcd: 'bg-purple-500/15 text-purple-600', argocd: 'bg-orange-500/15 text-orange-600' };
    const l: Record<string, string> = { fluxcd: 'FluxCD', argocd: 'ArgoCD' };
    return <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${s[engine] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>{l[engine] || engine}</span>;
  };

  return (
    <div className="space-y-4">
      {/* Feedback */}
      {actionResult && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 ${actionResult.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'}`}>
          <p className={`text-sm font-medium ${actionResult.ok ? 'text-emerald-600' : 'text-red-500'}`}>
            {actionResult.ok ? '\u2713 ' : '\u26A0 '}{actionResult.text}
          </p>
          <button onClick={() => setActionResult(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]">&#x2715;</button>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { label: 'Total Apps', value: stats.total, color: 'text-[var(--text-primary)]' },
          { label: 'Healthy', value: stats.healthy, color: 'text-emerald-600' },
          { label: 'Degraded', value: stats.degraded, color: 'text-red-600' },
          { label: 'Out of Sync', value: stats.outOfSync, color: 'text-orange-600' },
        ].map(s => (
          <div key={s.label} className="card card-body py-3 flex items-center gap-3">
            <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
            <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <input type="text" placeholder="Search applications..." value={search} onChange={e => setSearch(e.target.value)} className="input flex-1 max-w-xs" />
        <select value={healthFilter} onChange={e => setHealthFilter(e.target.value)} className="input w-40">
          <option value="">All Health</option>
          <option value="healthy">Healthy</option>
          <option value="progressing">Progressing</option>
          <option value="degraded">Degraded</option>
          <option value="suspended">Suspended</option>
        </select>
        <select value={engineFilter} onChange={e => setEngineFilter(e.target.value)} className="input w-40">
          <option value="">All Engines</option>
          <option value="argocd">ArgoCD</option>
          <option value="fluxcd">FluxCD</option>
        </select>
        <Link href="/gitops/applications" className="btn btn-secondary text-xs self-center">Full View</Link>
      </div>

      {/* Table */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading GitOps applications...</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30">&#x1F6E0;&#xFE0F;</div>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1">
            {apps.length === 0 ? 'No GitOps applications found' : 'No applications match your filters'}
          </p>
          <p className="text-[12px] text-[var(--text-tertiary)]">
            {apps.length === 0 ? 'Configure ArgoCD or FluxCD connections to see applications' : 'Try adjusting your filters'}
          </p>
        </div>
      ) : (
        <div className="card">
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Application</th>
                  <th>Namespace</th>
                  <th>Engine</th>
                  <th>Health</th>
                  <th>Sync</th>
                  <th>Revision</th>
                  <th>Environment</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(app => {
                  const nh = normalizeHealth(app.health);
                  const ns = normalizeSync(app.sync_status);
                  const ak = app.name;
                  return (
                    <tr
                      key={`${app.connection_id}-${app.namespace}-${app.name}`}
                      className="hover:bg-[var(--border-light)] transition-colors cursor-pointer"
                      onClick={() => app.connection_id && router.push(`/gitops/applications/${app.connection_id}/${app.namespace}/${app.name}`)}
                    >
                      <td>
                        <div className="font-medium text-[var(--text-primary)]">{app.name}</div>
                        {app.project && <p className="text-[10px] text-[var(--text-tertiary)]">{app.project}</p>}
                      </td>
                      <td className="text-[12px] font-mono text-[var(--text-secondary)]">{app.namespace}</td>
                      <td>{engineBadge(app.engine_type)}</td>
                      <td><span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${healthStyles[nh] || healthStyles.unknown}`}>{nh}</span></td>
                      <td><span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${syncStyles[ns] || syncStyles.unknown}`}>{ns}</span></td>
                      <td className="text-[11px] font-mono text-[var(--text-tertiary)]">{app.revision ? app.revision.slice(0, 8) : '\u2014'}</td>
                      <td>{app.environment ? <span className="text-[11px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] font-medium">{app.environment}</span> : <span className="text-[var(--text-tertiary)]">&mdash;</span>}</td>
                      <td onClick={e => e.stopPropagation()}>
                        <div className="flex gap-1">
                          <button onClick={() => handleAction(app, 'sync')} disabled={actionLoading === `${ak}-sync`} className="text-[11px] px-2 py-1 bg-blue-500/10 text-blue-500 rounded hover:bg-blue-500/15 disabled:opacity-50">
                            {actionLoading === `${ak}-sync` ? '...' : 'Sync'}
                          </button>
                          <button onClick={() => handleAction(app, 'refresh')} disabled={actionLoading === `${ak}-refresh`} className="text-[11px] px-2 py-1 bg-emerald-500/10 text-emerald-600 rounded hover:bg-emerald-500/15 disabled:opacity-50">
                            {actionLoading === `${ak}-refresh` ? '...' : 'Refresh'}
                          </button>
                          <button onClick={() => setActionConfirm({ app, action: 'terminate' })} className="text-[11px] px-2 py-1 bg-red-500/5 text-red-400 rounded hover:bg-red-500/10">
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
          <div className="px-4 py-2 border-t border-[var(--border)] text-[11px] text-[var(--text-tertiary)]">
            {filtered.length} application{filtered.length !== 1 ? 's' : ''}
          </div>
        </div>
      )}

      {actionConfirm && (
        <ConfirmModal
          open title={`Terminate ${actionConfirm.app.name}?`}
          description="This will terminate the running application. This action may be irreversible."
          confirmLabel="Terminate" variant="danger"
          onConfirm={() => handleAction(actionConfirm.app, actionConfirm.action)}
          onCancel={() => setActionConfirm(null)}
        />
      )}
    </div>
  );
}

// ─── Docker Tab ─────────────────────────────────────────────────────────────

function DockerTab() {
  const [services, setServices] = useState<DockerService[]>([]);
  const [hosts, setHosts] = useState<DockerHost[]>([]);
  const [discoveredContainers, setDiscoveredContainers] = useState<{ hostName: string; containers: DiscoveredDockerContainer[] }[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showLogs, setShowLogs] = useState<string | null>(null);
  const [logs, setLogs] = useState('');
  const [logsLoading, setLogsLoading] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const hostName = (id: string | null) => {
    if (!id) return 'Local Docker';
    return hosts.find(h => h.id === id)?.name || 'Unknown';
  };

  const load = useCallback(async () => {
    try {
      const [svcRes, hostRes] = await Promise.all([dockerServices.list(), dockerHosts.list()]);
      setServices(svcRes.docker_services || []);
      setHosts(hostRes.docker_hosts || []);
    } catch { /* ignore */ }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  // Discover containers
  useEffect(() => {
    if (hosts.length === 0) return;
    const discover = async () => {
      const connectedHosts = hosts.filter(h => h.status === 'connected');
      const results: { hostName: string; containers: DiscoveredDockerContainer[] }[] = [];
      for (const host of connectedHosts) {
        try {
          const res = await dockerHosts.containers(host.id);
          if (res.containers?.length) results.push({ hostName: host.name, containers: res.containers });
        } catch { /* skip */ }
      }
      setDiscoveredContainers(results);
    };
    const timer = setTimeout(discover, 500);
    return () => clearTimeout(timer);
  }, [hosts]);

  const handleAction = async (id: string, action: 'start' | 'stop' | 'restart' | 'refresh' | 'rollback') => {
    setActionLoading(id);
    try {
      switch (action) {
        case 'start': await dockerServices.start(id); break;
        case 'stop': await dockerServices.stop(id); break;
        case 'restart': await dockerServices.restart(id); break;
        case 'rollback': await dockerServices.rollback(id); break;
        case 'refresh': break;
      }
      await load();
    } catch { /* ignore */ }
    setActionLoading(null);
  };

  const handleDelete = async () => {
    if (!deleteConfirm) return;
    setDeleting(true);
    try { await dockerServices.delete(deleteConfirm); await load(); }
    catch { /* ignore */ }
    setDeleting(false);
    setDeleteConfirm(null);
  };

  const handleShowLogs = async (id: string) => {
    setShowLogs(id);
    setLogsLoading(true);
    try {
      const res = await dockerServices.logs(id);
      setLogs(res.logs || 'No logs available');
    } catch { setLogs('Failed to load logs'); }
    setLogsLoading(false);
  };

  const filteredServices = useMemo(() => {
    if (!search) return services;
    const q = search.toLowerCase();
    return services.filter(s => s.name.toLowerCase().includes(q) || (s.containers?.[0]?.image || '').toLowerCase().includes(q));
  }, [services, search]);

  const totalDiscovered = discoveredContainers.reduce((sum, dc) => sum + dc.containers.length, 0);

  const statusColor = (status: string) => {
    switch (status) {
      case 'running': return 'bg-emerald-500/15 text-emerald-600';
      case 'stopped': case 'exited': return 'bg-red-500/15 text-red-500';
      case 'restarting': return 'bg-blue-500/15 text-blue-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
    }
  };

  return (
    <div className="space-y-4">
      {/* Stats */}
      <div className="grid grid-cols-3 gap-4">
        <div className="card card-body py-3 flex items-center gap-3">
          <div className="text-[22px] font-bold text-[var(--text-primary)]">{services.length}</div>
          <div className="text-[11px] text-[var(--text-tertiary)]">Managed Services</div>
        </div>
        <div className="card card-body py-3 flex items-center gap-3">
          <div className="text-[22px] font-bold text-emerald-600">{services.filter(s => s.status === 'running').length}</div>
          <div className="text-[11px] text-[var(--text-tertiary)]">Running</div>
        </div>
        <div className="card card-body py-3 flex items-center gap-3">
          <div className="text-[22px] font-bold text-blue-600">{totalDiscovered}</div>
          <div className="text-[11px] text-[var(--text-tertiary)]">Discovered Containers</div>
        </div>
      </div>

      {/* Filters */}
      <div className="flex gap-3">
        <input type="text" placeholder="Search Docker services..." value={search} onChange={e => setSearch(e.target.value)} className="input flex-1 max-w-xs" />
      </div>

      {/* Managed Services */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading Docker services...</p>
        </div>
      ) : (
        <>
          {filteredServices.length === 0 ? (
            <div className="card card-body text-center py-12">
              <div className="text-4xl mb-3 opacity-30">&#x1F433;</div>
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">
                {services.length === 0 ? 'No Docker services found' : 'No services match your search'}
              </p>
              <p className="text-[12px] text-[var(--text-tertiary)]">
                {services.length === 0 ? 'Connect a Docker host to manage containers' : 'Try a different search term'}
              </p>
            </div>
          ) : (
            <div className="card">
              <div className="table-container">
                <table>
                  <thead>
                    <tr>
                      <th>Service</th>
                      <th>Image</th>
                      <th>Host</th>
                      <th>Status</th>
                      <th>Updated</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredServices.map(svc => (
                      <tr key={svc.id} className="hover:bg-[var(--border-light)] transition-colors">
                        <td><span className="font-medium text-[var(--text-primary)]">{svc.name}</span></td>
                        <td className="text-[11px] font-mono text-[var(--text-tertiary)] truncate max-w-[200px]">{svc.containers?.[0]?.image || '-'}</td>
                        <td className="text-[12px] text-[var(--text-secondary)]">{hostName(svc.docker_host_id)}</td>
                        <td><span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${statusColor(svc.status)}`}>{svc.status}</span></td>
                        <td className="text-[11px] text-[var(--text-tertiary)]">{svc.updated_at ? new Date(svc.updated_at).toLocaleDateString() : '-'}</td>
                        <td>
                          <div className="flex gap-1">
                            {svc.status !== 'running' ? (
                              <button onClick={() => handleAction(svc.id, 'start')} disabled={actionLoading === svc.id} className="text-[11px] px-2 py-1 bg-emerald-500/10 text-emerald-600 rounded hover:bg-emerald-500/15 disabled:opacity-50">
                                {actionLoading === svc.id ? '...' : 'Start'}
                              </button>
                            ) : (
                              <>
                                <button onClick={() => handleAction(svc.id, 'restart')} disabled={actionLoading === svc.id} className="text-[11px] px-2 py-1 bg-blue-500/10 text-blue-500 rounded hover:bg-blue-500/15 disabled:opacity-50">
                                  {actionLoading === svc.id ? '...' : 'Restart'}
                                </button>
                                <button onClick={() => handleAction(svc.id, 'stop')} disabled={actionLoading === svc.id} className="text-[11px] px-2 py-1 bg-red-500/10 text-red-500 rounded hover:bg-red-500/15 disabled:opacity-50">
                                  {actionLoading === svc.id ? '...' : 'Stop'}
                                </button>
                              </>
                            )}
                            <button onClick={() => handleAction(svc.id, 'rollback')} disabled={actionLoading === svc.id} className="text-[11px] px-2 py-1 bg-orange-500/10 text-orange-600 rounded hover:bg-orange-500/15 disabled:opacity-50">
                              {actionLoading === svc.id ? '...' : 'Rollback'}
                            </button>
                            <button onClick={() => handleShowLogs(svc.id)} className="text-[11px] px-2 py-1 bg-[var(--border-light)] text-[var(--text-secondary)] rounded hover:bg-[var(--border)]">
                              Logs
                            </button>
                            <button onClick={() => setDeleteConfirm(svc.id)} className="text-[11px] px-2 py-1 bg-red-500/5 text-red-400 rounded hover:bg-red-500/10">
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Discovered Containers */}
          {discoveredContainers.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">Discovered Containers</h3>
              {discoveredContainers.map(({ hostName: hn, containers }) => (
                <div key={hn}>
                  <div className="flex items-center gap-2 mb-2">
                    <BrandIcon name="docker" size={16} />
                    <span className="text-[13px] font-medium text-[var(--text-primary)]">{hn}</span>
                    <span className="text-[11px] text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded">{containers.length} containers</span>
                  </div>
                  <div className="card">
                    <div className="table-container">
                      <table>
                        <thead>
                          <tr>
                            <th>Container</th>
                            <th>Image</th>
                            <th>Status</th>
                            <th>Ports</th>
                          </tr>
                        </thead>
                        <tbody>
                          {containers.map((c, idx) => (
                            <tr key={`${c.name}-${idx}`} className="hover:bg-[var(--border-light)] transition-colors">
                              <td><span className="font-medium text-[var(--text-primary)]">{c.name}</span></td>
                              <td className="text-[11px] font-mono text-[var(--text-tertiary)] truncate max-w-[200px]">{c.image}</td>
                              <td><span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${c.status?.toLowerCase().includes('up') ? 'bg-emerald-500/15 text-emerald-600' : 'bg-red-500/15 text-red-500'}`}>{c.status}</span></td>
                              <td className="text-[11px] font-mono text-[var(--text-tertiary)]">{c.ports || '-'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}

      {/* Logs Modal */}
      {showLogs && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowLogs(null)}>
          <div className="card w-full max-w-3xl max-h-[80vh] flex flex-col" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
              <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">Container Logs</h3>
              <button onClick={() => setShowLogs(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">&#x2715;</button>
            </div>
            <div className="flex-1 overflow-auto p-4">
              {logsLoading ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">Loading logs...</p>
              ) : (
                <pre className="text-[11px] font-mono text-[var(--text-secondary)] whitespace-pre-wrap">{logs}</pre>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirm */}
      <ConfirmModal
        open={!!deleteConfirm}
        title="Delete Docker Service"
        description="Are you sure you want to delete this Docker service? This action cannot be undone."
        confirmLabel="Delete" variant="danger" loading={deleting}
        onConfirm={handleDelete} onCancel={() => setDeleteConfirm(null)}
      />
    </div>
  );
}
