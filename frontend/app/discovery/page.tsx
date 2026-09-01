'use client';

import { useState, useEffect, useCallback } from 'react';
import { discovery, type DiscoveredService } from '@/lib/api';
import { useDebounce } from '@/hooks/useDebounce';
import ConceptHelp from '@/components/ConceptHelp';
import ResizableTable, { type ColumnDef } from '@/components/ResizableTable';
import ServiceManagementPanel from '@/components/ServiceManagementPanel';
import BrandIcon from '@/components/BrandIcon';
import GearIcon from '@/components/GearIcon';

export default function DiscoveryPage() {
  const [services, setServices] = useState<DiscoveredService[]>([]);
  const [sources, setSources] = useState<Record<string, number>>({});
  const [clusters, setClusters] = useState<Record<string, number>>({});
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [k8sClusterCount, setK8sClusterCount] = useState(0);
  const [k8sNamespaceCount, setK8sNamespaceCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [totalUnfiltered, setTotalUnfiltered] = useState(0);

  // Filters
  const [searchFilter, setSearchFilter] = useState('');
  const debouncedSearch = useDebounce(searchFilter, 300);
  const [sourceFilter, setSourceFilter] = useState('');
  const [clusterFilter, setClusterFilter] = useState('');
  const [namespaceFilter, setNamespaceFilter] = useState('');
  const [healthFilter, setHealthFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [viewMode, setViewMode] = useState<'cluster' | 'source' | 'flat'>('cluster');
  const [showFilters, setShowFilters] = useState(false);

  // FluxCD action state
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Service management panel
  const [managingService, setManagingService] = useState<DiscoveredService | null>(null);

  const loadServices = useCallback(async () => {
    try {
      const params: Record<string, string> = {};
      if (debouncedSearch) params.search = debouncedSearch;
      if (sourceFilter) params.source = sourceFilter;
      if (clusterFilter) params.cluster = clusterFilter;
      if (namespaceFilter) params.namespace = namespaceFilter;
      if (healthFilter) params.health = healthFilter;
      if (statusFilter) params.status = statusFilter;

      const data = await discovery.services(params);
      setServices(data.services || []);
      setSources(data.sources || {});
      setClusters(data.clusters || {});
      setNamespaces(Object.keys(data.namespaces || {}));
      setK8sClusterCount(data.k8s_clusters ?? Object.keys(data.clusters || {}).length);
      setK8sNamespaceCount(data.k8s_namespaces ?? Object.keys(data.namespaces || {}).length);
      setTotalUnfiltered(data.total_unfiltered || 0);
    } catch (err) {
      console.error('Failed to load services:', err);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, sourceFilter, clusterFilter, namespaceFilter, healthFilter, statusFilter]);

  useEffect(() => {
    loadServices();
  }, [loadServices]);

  const handleSync = async () => {
    setSyncing(true);
    try {
      await discovery.sync();
      await loadServices();
    } catch (err) {
      console.error('Sync failed:', err);
    } finally {
      setSyncing(false);
    }
  };

  const clearFilters = () => {
    setSearchFilter('');
    setSourceFilter('');
    setClusterFilter('');
    setNamespaceFilter('');
    setHealthFilter('');
    setStatusFilter('');
  };

  const hasActiveFilters = debouncedSearch || sourceFilter || clusterFilter || namespaceFilter || healthFilter || statusFilter;

  // Active filter chips for display
  const activeFilterChips: { label: string; onClear: () => void }[] = [];
  if (sourceFilter) activeFilterChips.push({ label: `Source: ${sourceFilter}`, onClear: () => setSourceFilter('') });
  if (clusterFilter) activeFilterChips.push({ label: `Cluster: ${clusterFilter}`, onClear: () => setClusterFilter('') });
  if (namespaceFilter) activeFilterChips.push({ label: `NS: ${namespaceFilter}`, onClear: () => setNamespaceFilter('') });
  if (healthFilter) activeFilterChips.push({ label: `Health: ${healthFilter}`, onClear: () => setHealthFilter('') });
  if (statusFilter) activeFilterChips.push({ label: `Status: ${statusFilter}`, onClear: () => setStatusFilter('') });

  // FluxCD actions
  const handleFluxcdAction = async (action: 'suspend' | 'resume' | 'reconcile' | 'delete', svc: DiscoveredService) => {
    if (action === 'delete' && !confirm(`Delete HelmRelease ${svc.namespace}/${svc.name} from cluster ${svc.cluster}?`)) {
      return;
    }
    const key = `${svc.cluster}-${svc.namespace}-${svc.name}-${action}`;
    setActionLoading(key);
    setActionMessage(null);
    try {
      let result;
      if (action === 'suspend') result = await discovery.fluxcdSuspend(svc.cluster, svc.namespace, svc.name);
      else if (action === 'resume') result = await discovery.fluxcdResume(svc.cluster, svc.namespace, svc.name);
      else if (action === 'reconcile') result = await discovery.fluxcdReconcile(svc.cluster, svc.namespace, svc.name);
      else if (action === 'delete') result = await discovery.fluxcdDelete(svc.cluster, svc.namespace, svc.name);
      setActionMessage({ type: 'success', text: result?.message || `Action ${action} completed` });
      setTimeout(() => setActionMessage(null), 3000);
      // Reload after action
      await loadServices();
    } catch (err) {
      setActionMessage({ type: 'error', text: `Failed to ${action}: ${err}` });
      setTimeout(() => setActionMessage(null), 5000);
    } finally {
      setActionLoading(null);
    }
  };

  // Group by cluster
  const groupedByCluster: Record<string, DiscoveredService[]> = {};
  for (const svc of services) {
    const cluster = svc.cluster || 'default';
    if (!groupedByCluster[cluster]) groupedByCluster[cluster] = [];
    groupedByCluster[cluster].push(svc);
  }

  const sourceColors: Record<string, string> = {
    pepa: 'bg-blue-500/10 text-blue-500',
    argocd: 'bg-violet-500/10 text-violet-500',
    fluxcd: 'bg-emerald-500/10 text-emerald-600',
    docker: 'bg-cyan-500/10 text-cyan-500',
    'docker-container': 'bg-teal-500/10 text-teal-500',
    manual: 'bg-[var(--border-light)] text-[var(--text-secondary)]',
  };

  const sourceIcons: Record<string, string> = {
    pepa: 'argocd',
    argocd: 'argocd',
    fluxcd: 'fluxcd',
    docker: 'docker',
    'docker-container': 'docker',
    manual: 'storage',
  };

  const healthColors: Record<string, string> = {
    healthy: 'text-green-600',
    degraded: 'text-yellow-600',
    progressing: 'text-blue-600',
    unknown: 'text-[var(--text-tertiary)]',
  };

  const clusterNames = Object.keys(clusters);
  const activeFilterCount = [debouncedSearch, sourceFilter, clusterFilter, namespaceFilter, healthFilter, statusFilter].filter(Boolean).length;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Service Discovery</h1>
            <ConceptHelp term="discovery" />
          </div>
          <p className="page-subtitle-modern">
            View and manage deployed services across clusters from PEPA, ArgoCD, FluxCD
          </p>
        </div>
        <button
          onClick={handleSync}
          disabled={syncing}
          className="btn btn-primary disabled:opacity-50"
        >
          {syncing ? 'Syncing...' : 'Sync Now'}
        </button>
      </div>

      {/* Action message */}
      {actionMessage && (
        <div className={`card card-body py-2 ${actionMessage.type === 'success' ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'}`}>
          <p className={`text-[12px] ${actionMessage.type === 'success' ? 'text-emerald-600' : 'text-red-500'}`}>
            {actionMessage.text}
          </p>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-2 md:grid-cols-6 gap-3 page-animate-up page-delay-1">
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            Total {hasActiveFilters && <span className="text-[10px]">/ {totalUnfiltered}</span>}
          </div>
          <div className="text-[24px] font-bold text-[var(--text-primary)]">
            {services.length}
          </div>
        </div>
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            Clusters
          </div>
          <div className="text-[24px] font-bold text-[var(--text-primary)]">
            {k8sClusterCount}
          </div>
        </div>
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            Namespaces
          </div>
          <div className="text-[24px] font-bold text-[var(--text-primary)]">
            {k8sNamespaceCount}
          </div>
        </div>
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            FluxCD
          </div>
          <div className="text-[24px] font-bold text-green-600">
            {sources.fluxcd || 0}
          </div>
        </div>
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            Docker
          </div>
          <div className="text-[24px] font-bold text-cyan-600">
            {(sources.docker || 0) + (sources['docker-container'] || 0)}
          </div>
        </div>
        <div className="modern-stat-card">
          <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
            PEPA
          </div>
          <div className="text-[24px] font-bold text-blue-600">
            {sources.pepa || 0}
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="card card-body page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
        <div className="space-y-3">
          {/* View mode + search */}
          <div className="flex flex-wrap items-center gap-3">
            {/* View Mode */}
            <div className="flex gap-1 bg-[var(--border-light)] rounded-lg p-0.5">
              <button
                onClick={() => setViewMode('cluster')}
                className={`px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${viewMode === 'cluster' ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
              >
                By Cluster
              </button>
              <button
                onClick={() => setViewMode('source')}
                className={`px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${viewMode === 'source' ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
              >
                By Source
              </button>
              <button
                onClick={() => setViewMode('flat')}
                className={`px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${viewMode === 'flat' ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
              >
                Flat List
              </button>
            </div>

            <div className="h-5 w-px bg-[var(--border)]" />

            {/* Search */}
            <div className="relative flex-1 min-w-[200px]">
              <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                type="text"
                placeholder="Search by name or namespace..."
                value={searchFilter}
                onChange={e => setSearchFilter(e.target.value)}
                className="input !py-1.5 !text-[12px] w-full !pl-8"
              />
              {searchFilter && (
                <button
                  onClick={() => setSearchFilter('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
                >
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              )}
            </div>

            {/* Filter toggle */}
            <button
              onClick={() => setShowFilters(!showFilters)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-colors ${
                showFilters || hasActiveFilters
                  ? 'border-[var(--accent)] text-[var(--accent)] bg-[var(--accent-subtle)]'
                  : 'border-[var(--border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)]'
              }`}
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
              </svg>
              Filters
              {activeFilterCount > 0 && (
                <span className="w-4 h-4 rounded-full bg-[var(--accent)] text-white text-[9px] flex items-center justify-center font-bold">
                  {activeFilterCount}
                </span>
              )}
            </button>

            {hasActiveFilters && (
              <button
                onClick={clearFilters}
                className="text-[11px] text-[var(--accent)] hover:underline flex items-center gap-1"
              >
                <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
                Clear all
              </button>
            )}
          </div>

          {/* Active filter chips */}
          {activeFilterChips.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5">
              {activeFilterChips.map((chip, i) => (
                <span key={i} className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] bg-[var(--accent-subtle)] text-[var(--accent)] border border-[var(--accent)]/20">
                  {chip.label}
                  <button onClick={chip.onClear} className="hover:text-[var(--text-primary)]">
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </span>
              ))}
            </div>
          )}

          {/* Collapsible Filter dropdowns */}
          {showFilters && (
            <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-[var(--border-light)]">
              {/* Source Filter */}
              <div className="flex flex-col gap-1">
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Source</label>
                <select
                  value={sourceFilter}
                  onChange={e => setSourceFilter(e.target.value)}
                  className="select !py-1.5 !text-[12px] w-36"
                >
                  <option value="">All Sources</option>
                  <option value="pepa">🚀 PEPA</option>
                  <option value="argocd">⛵ ArgoCD</option>
                  <option value="fluxcd">🔷 FluxCD</option>
                  <option value="docker">🐳 Docker</option>
                  <option value="docker-container">📦 Docker Containers</option>
                  <option value="manual">📦 Manual</option>
                </select>
              </div>

              {/* Cluster Filter */}
              <div className="flex flex-col gap-1">
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Cluster</label>
                <select
                  value={clusterFilter}
                  onChange={e => setClusterFilter(e.target.value)}
                  className="select !py-1.5 !text-[12px] w-40"
                >
                  <option value="">All Clusters</option>
                  {clusterNames.map(name => (
                    <option key={name} value={name}>{name} ({clusters[name]})</option>
                  ))}
                </select>
              </div>

              {/* Namespace Filter */}
              <div className="flex flex-col gap-1">
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Namespace</label>
                <select
                  value={namespaceFilter}
                  onChange={e => setNamespaceFilter(e.target.value)}
                  className="select !py-1.5 !text-[12px] w-44"
                >
                  <option value="">All Namespaces</option>
                  {namespaces.sort().map(ns => (
                    <option key={ns} value={ns}>{ns}</option>
                  ))}
                </select>
              </div>

              {/* Health Filter */}
              <div className="flex flex-col gap-1">
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Health</label>
                <select
                  value={healthFilter}
                  onChange={e => setHealthFilter(e.target.value)}
                  className="select !py-1.5 !text-[12px] w-36"
                >
                  <option value="">All Health</option>
                  <option value="healthy">🟢 Healthy</option>
                  <option value="degraded">🟡 Degraded</option>
                  <option value="progressing">🔵 Progressing</option>
                  <option value="unknown">⚪ Unknown</option>
                </select>
              </div>

              {/* Status Filter */}
              <div className="flex flex-col gap-1">
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Status</label>
                <select
                  value={statusFilter}
                  onChange={e => setStatusFilter(e.target.value)}
                  className="select !py-1.5 !text-[12px] w-36"
                >
                  <option value="">All Status</option>
                  <option value="running">Running</option>
                  <option value="deploying">Deploying</option>
                  <option value="failed">Failed</option>
                  <option value="unknown">Unknown</option>
                </select>
              </div>
            </div>
          )}

          {/* Results count */}
          <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)]">
            <span>
              {services.length} service{services.length !== 1 ? 's' : ''}
              {hasActiveFilters && <span className="ml-1">(filtered from {totalUnfiltered})</span>}
            </span>
            {loading && <span className="animate-pulse">Loading...</span>}
          </div>
        </div>
      </div>

      {/* Content */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-secondary)]">Loading services...</p>
        </div>
      ) : services.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30">🔍</div>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1">
            {hasActiveFilters ? 'No services match your filters' : 'No services found'}
          </p>
          <p className="text-[12px] text-[var(--text-tertiary)]">
            {hasActiveFilters
              ? 'Try adjusting your filters or clear them.'
              : 'Services deployed via PEPA, ArgoCD, FluxCD or Kubernetes will appear here. Click "Sync Now" to discover services.'}
          </p>
        </div>
      ) : viewMode === 'cluster' ? (
        <div className="space-y-6">
          {Object.entries(groupedByCluster).map(([clusterName, clusterServices]) => (
            <div key={clusterName}>
              <div className="flex items-center gap-2 mb-3">
                <div className="w-2.5 h-2.5 rounded-full bg-[var(--accent)]" />
                <h2 className="text-[14px] font-semibold text-[var(--text-primary)]">{clusterName}</h2>
                <span className="text-[11px] text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded">
                  {clusterServices.length} services
                </span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {clusterServices.map((svc, idx) => (
                  <ServiceCard
                    key={`${svc.source}-${svc.name}-${idx}`}
                    svc={svc}
                    sourceColors={sourceColors}
                    sourceIcons={sourceIcons}
                    healthColors={healthColors}
                    actionLoading={actionLoading}
                    onAction={handleFluxcdAction}
                    onManage={setManagingService}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : viewMode === 'source' ? (
        <div className="space-y-6">
          {(['pepa', 'argocd', 'fluxcd', 'docker', 'docker-container', 'manual'] as const).map(source => {
            const sourceServices = services.filter(s => s.source === source);
            if (sourceServices.length === 0) return null;
            return (
              <div key={source}>
                <div className="flex items-center gap-2 mb-3">
                  <BrandIcon name={sourceIcons[source]} size={18} />
                  <h2 className="text-[14px] font-semibold text-[var(--text-primary)]">{source.toUpperCase()}</h2>
                  <span className="text-[11px] text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded">
                    {sourceServices.length} services
                  </span>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                  {sourceServices.map((svc, idx) => (
                    <ServiceCard
                      key={`${svc.source}-${svc.name}-${idx}`}
                      svc={svc}
                      sourceColors={sourceColors}
                      sourceIcons={sourceIcons}
                      healthColors={healthColors}
                      actionLoading={actionLoading}
                      onAction={handleFluxcdAction}
                      onManage={setManagingService}
                    />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <DiscoveryTable
          services={services}
          sourceColors={sourceColors}
          sourceIcons={sourceIcons}
          healthColors={healthColors}
          actionLoading={actionLoading}
          onAction={handleFluxcdAction}
          onManage={setManagingService}
        />
      )}

      {/* Service Management Panel */}
      {managingService && (
        <ServiceManagementPanel
          service={managingService}
          onClose={() => setManagingService(null)}
          onUpdate={loadServices}
        />
      )}
      </div>
    </div>
  );
}

function FluxcdActions({ svc, actionLoading, onAction }: {
  svc: DiscoveredService;
  actionLoading: string | null;
  onAction: (action: 'suspend' | 'resume' | 'reconcile' | 'delete', svc: DiscoveredService) => void;
}) {
  const key = `${svc.cluster}-${svc.namespace}-${svc.name}`;
  return (
    <div className="flex gap-1">
      <button
        onClick={() => onAction('suspend', svc)}
        disabled={actionLoading === `${key}-suspend`}
        className="text-[10px] px-1.5 py-0.5 rounded bg-yellow-500/10 text-yellow-600 hover:bg-yellow-500/15 disabled:opacity-50"
        title="Suspend reconciliation"
      >
        {actionLoading === `${key}-suspend` ? '...' : '⏸'}
      </button>
      <button
        onClick={() => onAction('resume', svc)}
        disabled={actionLoading === `${key}-resume`}
        className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15 disabled:opacity-50"
        title="Resume reconciliation"
      >
        {actionLoading === `${key}-resume` ? '...' : '▶'}
      </button>
      <button
        onClick={() => onAction('reconcile', svc)}
        disabled={actionLoading === `${key}-reconcile`}
        className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 hover:bg-blue-500/15 disabled:opacity-50"
        title="Force reconcile"
      >
        {actionLoading === `${key}-reconcile` ? '...' : '🔄'}
      </button>
      <button
        onClick={() => onAction('delete', svc)}
        disabled={actionLoading === `${key}-delete`}
        className="text-[10px] px-1.5 py-0.5 rounded bg-red-500/10 text-red-500 hover:bg-red-500/15 disabled:opacity-50"
        title="Delete HelmRelease"
      >
        {actionLoading === `${key}-delete` ? '...' : '🗑'}
      </button>
    </div>
  );
}

function ServiceCard({ svc, sourceColors, sourceIcons, healthColors, actionLoading, onAction, onManage }: {
  svc: DiscoveredService;
  sourceColors: Record<string, string>;
  sourceIcons: Record<string, string>;
  healthColors: Record<string, string>;
  actionLoading: string | null;
  onAction: (action: 'suspend' | 'resume' | 'reconcile' | 'delete', svc: DiscoveredService) => void;
  onManage: (svc: DiscoveredService) => void;
}) {
  const healthBarColor = svc.health === 'healthy' ? 'bg-green-500' : svc.health === 'degraded' ? 'bg-yellow-500' : svc.health === 'progressing' ? 'bg-blue-500' : svc.health === 'failed' ? 'bg-red-500' : 'bg-gray-300';

  return (
    <div
      className="card group cursor-pointer hover:border-[var(--accent)]/40 hover:shadow-sm transition-all"
      onClick={() => onManage(svc)}
    >
      {/* Health bar */}
      <div className={`h-0.5 w-full rounded-t-md ${healthBarColor}`} />
      <div className="card-body py-3 space-y-2">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2 min-w-0">
            <div className={`w-2 h-2 rounded-full shrink-0 ${healthBarColor}`} />
            <div className="min-w-0">
              <h3 className="text-[13px] font-medium text-[var(--text-primary)] truncate">{svc.name}</h3>
              <div className="text-[11px] text-[var(--text-tertiary)]">
                {svc.cluster || 'default'} / {svc.namespace}
              </div>
            </div>
          </div>
          <span className={`text-[10px] px-1.5 py-0.5 rounded shrink-0 ${sourceColors[svc.source]}`}>
            {svc.source}
          </span>
        </div>

        {/* Stats row */}
        <div className="flex items-center gap-3 text-[11px]">
          <div className="flex items-center gap-1">
            <span className="text-[var(--text-tertiary)]">Status:</span>
            <span className={`font-medium ${svc.status === 'running' ? 'text-green-600' : 'text-[var(--text-secondary)]'}`}>
              {svc.status}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-[var(--text-tertiary)]">Replicas:</span>
            <span className="font-medium text-[var(--text-primary)]">
              {svc.ready_replicas}/{svc.replicas}
            </span>
          </div>
        </div>

        {/* Image */}
        {svc.image && svc.image !== '-' && (
          <div className="text-[10px] text-[var(--text-tertiary)] truncate font-mono" title={svc.image}>
            {svc.image}
          </div>
        )}

        {/* Footer: sync + updated */}
        <div className="flex items-center justify-between text-[10px] text-[var(--text-tertiary)]">
          <span className={`px-1.5 py-0.5 rounded ${svc.sync_status === 'synced' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-yellow-500/10 text-yellow-600'}`}>
            {svc.sync_status}
          </span>
          <span>{new Date(svc.last_updated).toLocaleString()}</span>
        </div>

        {/* FluxCD Actions */}
        {svc.source === 'fluxcd' && (
          <div className="pt-2 border-t border-[var(--border-light)]" onClick={e => e.stopPropagation()}>
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-[var(--text-tertiary)] mr-1">FluxCD:</span>
              <FluxcdActions svc={svc} actionLoading={actionLoading} onAction={onAction} />
            </div>
          </div>
        )}

        {/* Hover quick actions */}
        <div className="pt-2 border-t border-[var(--border-light)] flex items-center justify-between">
          <span className="text-[10px] text-[var(--text-tertiary)]">Click to manage</span>
          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity" onClick={e => e.stopPropagation()}>
            <button
              onClick={() => onManage(svc)}
              className="text-[10px] px-2 py-0.5 rounded bg-[var(--accent)] text-white hover:opacity-90 transition-opacity"
              title="Open management panel"
            >
              Manage
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function DiscoveryTable({ services, sourceColors, sourceIcons, healthColors, actionLoading, onAction, onManage }: {
  services: DiscoveredService[];
  sourceColors: Record<string, string>;
  sourceIcons: Record<string, string>;
  healthColors: Record<string, string>;
  actionLoading: string | null;
  onAction: (action: 'suspend' | 'resume' | 'reconcile' | 'delete', svc: DiscoveredService) => void;
  onManage: (svc: DiscoveredService) => void;
}) {
  const [sortKey, setSortKey] = useState<string>('name');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  const handleSort = (key: string) => {
    if (sortKey === key) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  };

  const sorted = [...services].sort((a, b) => {
    const dir = sortDir === 'asc' ? 1 : -1;
    switch (sortKey) {
      case 'name': return dir * a.name.localeCompare(b.name);
      case 'cluster': return dir * (a.cluster || '').localeCompare(b.cluster || '');
      case 'namespace': return dir * a.namespace.localeCompare(b.namespace);
      case 'source': return dir * a.source.localeCompare(b.source);
      case 'status': return dir * a.status.localeCompare(b.status);
      case 'health': return dir * a.health.localeCompare(b.health);
      case 'replicas': return dir * (a.replicas - b.replicas);
      default: return 0;
    }
  });

  const SortHeader = ({ col, label }: { col: string; label: string }) => (
    <button
      onClick={() => handleSort(col)}
      className="flex items-center gap-1 text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wider hover:text-[var(--text-primary)] transition-colors"
    >
      {label}
      {sortKey === col && (
        <svg className={`w-3 h-3 transition-transform ${sortDir === 'desc' ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" />
        </svg>
      )}
    </button>
  );

  const columns: ColumnDef[] = [
    {
      key: 'name',
      label: 'Service',
      width: 180,
      minWidth: 100,
      render: (svc: DiscoveredService) => (
        <span className="font-medium text-[var(--text-primary)]">{svc.name}</span>
      ),
    },
    {
      key: 'cluster',
      label: 'Cluster',
      width: 120,
      minWidth: 80,
      render: (svc: DiscoveredService) => (
        <span className="text-[12px] text-[var(--text-secondary)]">{svc.cluster || 'default'}</span>
      ),
    },
    {
      key: 'namespace',
      label: 'Namespace',
      width: 140,
      minWidth: 80,
      render: (svc: DiscoveredService) => (
        <span className="text-[12px] text-[var(--text-secondary)]">{svc.namespace}</span>
      ),
    },
    {
      key: 'source',
      label: 'Source',
      width: 110,
      minWidth: 80,
      render: (svc: DiscoveredService) => (
        <span className={`text-[10px] px-2 py-0.5 rounded-full inline-flex items-center gap-1 ${sourceColors[svc.source]}`}>
          <BrandIcon name={sourceIcons[svc.source] || 'default'} size={12} /> {svc.source}
        </span>
      ),
    },
    {
      key: 'status',
      label: 'Status',
      width: 100,
      minWidth: 70,
      render: (svc: DiscoveredService) => (
        <span className={`text-[12px] font-medium ${svc.status === 'running' ? 'text-green-600' : 'text-[var(--text-secondary)]'}`}>
          {svc.status}
        </span>
      ),
    },
    {
      key: 'health',
      label: 'Health',
      width: 100,
      minWidth: 70,
      render: (svc: DiscoveredService) => (
        <span className={`text-[12px] font-medium ${healthColors[svc.health] || 'text-[var(--text-secondary)]'}`}>
          {svc.health}
        </span>
      ),
    },
    {
      key: 'replicas',
      label: 'Replicas',
      width: 90,
      minWidth: 60,
      render: (svc: DiscoveredService) => (
        <span className="text-[12px] text-[var(--text-secondary)]">
          {svc.ready_replicas}/{svc.replicas}
        </span>
      ),
    },
    {
      key: 'actions',
      label: 'Actions',
      width: 180,
      minWidth: 120,
      render: (svc: DiscoveredService) => (
        <div className="flex gap-1 items-center">
          {svc.source === 'fluxcd' && (
            <FluxcdActions svc={svc} actionLoading={actionLoading} onAction={onAction} />
          )}
          <button
            onClick={() => onManage(svc)}
            className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--accent)] hover:text-white transition-colors"
            title="Manage service"
          >
            <GearIcon className="w-3.5 h-3.5" />
          </button>
        </div>
      ),
    },
  ];

  return (
    <div>
      {/* Sort bar */}
      <div className="flex items-center gap-3 px-3 py-2 border-b border-[var(--border)] bg-[var(--border-light)]/50 rounded-t-md overflow-x-auto">
        <span className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider shrink-0">Sort by:</span>
        {['name', 'cluster', 'namespace', 'source', 'health', 'replicas'].map(col => (
          <button
            key={col}
            onClick={() => handleSort(col)}
            className={`text-[11px] px-2 py-0.5 rounded transition-colors ${sortKey === col ? 'bg-[var(--accent)] text-white font-medium' : 'text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)]'}`}
          >
            {col.charAt(0).toUpperCase() + col.slice(1)}
            {sortKey === col && (sortDir === 'desc' ? ' ↓' : ' ↑')}
          </button>
        ))}
      </div>
      <ResizableTable
        columns={columns}
        data={sorted}
        rowKey={(svc, idx) => `${svc.source}-${svc.name}-${idx}`}
      />
    </div>
  );
}
