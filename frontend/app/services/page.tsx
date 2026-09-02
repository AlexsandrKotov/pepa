'use client';

import { useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { services, discovery, type Service, type DiscoveredService } from '@/lib/api';
import { useDebounce } from '@/hooks/useDebounce';
import dynamic from 'next/dynamic';
import ConfirmModal from '@/components/ConfirmModal';
import ConceptHelp from '@/components/ConceptHelp';
import BrandIcon from '@/components/BrandIcon';
import ServiceDetailClient from './ServiceDetailClient';

const ServiceManagementPanel = dynamic(() => import('@/components/ServiceManagementPanel'), { ssr: false });

function ServicesPageContent() {
  const searchParams = useSearchParams();
  const serviceId = searchParams.get('id');

  if (serviceId) {
    return <ServiceDetailClient />;
  }
  return <ServicesList />;
}

export default function ServicesPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <ServicesPageContent />
    </Suspense>
  );
}

function ServicesList() {
  const [servicesList, setServicesList] = useState<Service[]>([]);
  const [discoveredServices, setDiscoveredServices] = useState<DiscoveredService[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [clusterFilter, setClusterFilter] = useState('');
  const [sourceFilter, setSourceFilter] = useState('');
  const [viewMode, setViewMode] = useState<'table' | 'cards' | 'cluster'>('table');
  const [sortKey, setSortKey] = useState<string>('name');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [managingService, setManagingService] = useState<DiscoveredService | null>(null);
  const [showFilters, setShowFilters] = useState(false);
  const [healthFilter, setHealthFilter] = useState('');
  const [clusters, setClusters] = useState<Record<string, number>>({});


  const debouncedSearch = useDebounce(search, 300);
  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<Service | null>(null);

  const [deleting, setDeleting] = useState(false);
  const [syncing, setSyncing] = useState(false);

  useEffect(() => {
    loadData();
  }, [debouncedSearch, statusFilter]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadData = useCallback(async () => {
    try {
      const params: Record<string, string> = {};
      if (debouncedSearch) params.search = debouncedSearch;
      if (statusFilter) params.status = statusFilter;

      const [svcData, discData] = await Promise.all([
        services.list(params).catch(() => ({ items: [], total: 0 })),
        discovery.services().catch(() => ({ services: [], sources: {}, clusters: {}, total: 0 })),
      ]);
      setServicesList(svcData.items || []);
      setDiscoveredServices(discData.services || []);
      setClusters(discData.clusters || {});
    } catch (err) {
      console.error('Failed to load services:', err);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, statusFilter]);

  const handleSync = useCallback(async () => {
    setSyncing(true);
    try {
      await discovery.sync();
      await loadData();
    } catch (err) {
      console.error('Sync failed:', err);
    } finally {
      setSyncing(false);
    }
  }, [loadData]);

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await services.delete(deleteTarget.id);
      setDeleteTarget(null);
      loadData();
    } catch (err) {
      console.error('Delete failed:', err);
    } finally {
      setDeleting(false);
    }
  };

  const requestDelete = (svc: DiscoveredService) => {
    const key = `${svc.name}:${svc.namespace}:${svc.cluster || 'default'}`;
    const pepaSvc = pepaServiceMap.get(key);
    if (pepaSvc) {
      setDeleteTarget(pepaSvc);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'active': case 'running': case 'healthy': return 'bg-emerald-500/15 text-emerald-600';
      case 'deploying': case 'progressing': return 'bg-amber-500/15 text-amber-600';
      case 'configured': return 'bg-blue-500/15 text-blue-500';
      case 'error': case 'failed': case 'degraded': return 'bg-red-500/15 text-red-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
    }
  };

  const sourceBadge = (source: string) => {
    const colors: Record<string, string> = {
      pepa: 'bg-blue-500/10 text-blue-500',
      argocd: 'bg-purple-500/10 text-purple-500',
      fluxcd: 'bg-emerald-500/10 text-emerald-600',
      docker: 'bg-cyan-500/10 text-cyan-500',
      'docker-container': 'bg-teal-500/10 text-teal-500',
      manual: 'bg-[var(--bg)] text-[var(--text-secondary)]',
    };
    const icons: Record<string, string> = { pepa: 'argocd', argocd: 'argocd', fluxcd: 'fluxcd', docker: 'docker', 'docker-container': 'docker', manual: 'storage' };
    return (
      <span className={`text-[10px] px-1.5 py-0.5 rounded inline-flex items-center gap-1 ${colors[source] || 'bg-[var(--bg)] text-[var(--text-secondary)]'}`}>
        <BrandIcon name={icons[source] || 'default'} size={12} /> {source}
      </span>
    );
  };

  // Build unified service list with deduplication
  // Priority: pepa > argocd > fluxcd > manual
  const { unifiedServices, pepaServiceMap } = useMemo(() => {
    const sourcePriority: Record<string, number> = { pepa: 4, argocd: 3, fluxcd: 2, docker: 2, 'docker-container': 2, manual: 1 };
    const serviceMap = new Map<string, DiscoveredService>();
    const pepaMap = new Map<string, Service>();

    const discoveredClusterMap = new Map<string, string>();
    for (const ds of discoveredServices) {
      const key = `${ds.name}:${ds.namespace}`;
      discoveredClusterMap.set(key, ds.cluster || 'default');
    }

    for (const s of servicesList) {
      const key = `${s.name}:${s.namespace}`;
      const clusterName = discoveredClusterMap.get(key) || 'default';
      const unifiedKey = `${s.name}:${s.namespace}:${clusterName}`;
      serviceMap.set(unifiedKey, {
        name: s.name,
        namespace: s.namespace,
        cluster: clusterName,
        source: 'pepa' as const,
        status: s.status,
        health: s.status === 'active' || s.status === 'running' ? 'healthy' : s.status === 'error' || s.status === 'failed' ? 'failed' : s.status === 'deploying' ? 'progressing' : s.status === 'configured' ? 'healthy' : 'unknown',
        replicas: 0,
        ready_replicas: 0,
        image: '-',
        last_updated: s.updated_at,
        labels: {},
        sync_status: 'synced',
      });
      pepaMap.set(unifiedKey, s);
    }

    for (const ds of discoveredServices) {
      const key = `${ds.name}:${ds.namespace}:${ds.cluster || 'default'}`;
      const existing = serviceMap.get(key);
      if (!existing) {
        serviceMap.set(key, ds);
      } else if (sourcePriority[ds.source] > sourcePriority[existing.source]) {
        serviceMap.set(key, ds);
      }
    }

    return { unifiedServices: Array.from(serviceMap.values()) as DiscoveredService[], pepaServiceMap: pepaMap };
  }, [servicesList, discoveredServices]);

  // Apply filters
  const filteredServices = useMemo(() => {
    return unifiedServices.filter(s => {
      if (clusterFilter && (s.cluster || 'default') !== clusterFilter) return false;
      if (sourceFilter && s.source !== sourceFilter) return false;
      if (statusFilter && s.status !== statusFilter) return false;
      if (healthFilter && s.health !== healthFilter) return false;
      if (debouncedSearch && !s.name.toLowerCase().includes(debouncedSearch.toLowerCase()) && !s.namespace.toLowerCase().includes(debouncedSearch.toLowerCase())) return false;
      return true;
    });
  }, [unifiedServices, clusterFilter, sourceFilter, statusFilter, healthFilter, debouncedSearch]);

  // Sort
  const sortedServices = useMemo(() => {
    return [...filteredServices].sort((a, b) => {
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
  }, [filteredServices, sortKey, sortDir]);

  // Group by cluster
  const groupedByCluster = useMemo(() => {
    const grouped: Record<string, DiscoveredService[]> = {};
    for (const svc of sortedServices) {
      const cluster = svc.cluster || 'default';
      if (!grouped[cluster]) grouped[cluster] = [];
      grouped[cluster].push(svc);
    }
    return grouped;
  }, [sortedServices]);

  // Health summary
  const healthCounts = useMemo(() => {
    const counts = { healthy: 0, degraded: 0, progressing: 0, failed: 0, unknown: 0 };
    unifiedServices.forEach(s => {
      const h = s.health as keyof typeof counts;
      if (h in counts) counts[h]++;
      else counts.unknown++;
    });
    return counts;
  }, [unifiedServices]);
  const totalServices = unifiedServices.length;

  const clusterNames = Object.keys(clusters);
  const infraCount = clusterNames.length;
  const hasActiveFilters = debouncedSearch || statusFilter || clusterFilter || sourceFilter || healthFilter;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-5">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Services</h1>
            <ConceptHelp term="service" />
          </div>
          <p className="page-subtitle-modern">
            Manage and monitor services across your infrastructure
            {discoveredServices.length > 0 && <span> &middot; {discoveredServices.length} discovered</span>}
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleSync}
            disabled={syncing}
            className="btn btn-secondary btn-sm disabled:opacity-50"
          >
            {syncing ? 'Syncing...' : 'Sync Now'}
          </button>
          <Link href="/discovery" className="btn btn-secondary btn-sm">
            Discovery
          </Link>
          <Link href="/services/new" className="btn btn-primary" data-tour="services-create">
            + New Service
          </Link>
        </div>
      </div>

      {/* Running Services Section */}
      <div className="space-y-4 page-animate-up page-delay-1">
        {/* Stats bar */}
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 text-[13px] text-[var(--text-secondary)]">
            <span className="font-semibold text-[var(--text-primary)]">{totalServices}</span>
            <span>services across {infraCount || 1} {infraCount === 1 ? 'host' : 'hosts'}</span>
          </div>
        </div>

      {/* Health Summary Bar */}
      {totalServices > 0 && (
        <div className="card card-body py-3" style={{ borderRadius: '12px' }}>
          <div className="flex items-center gap-4">
            {/* Bar */}
            <div className="flex-1 h-2 rounded-full bg-[var(--border-light)] overflow-hidden flex">
              {healthCounts.healthy > 0 && (
                <button
                  onClick={() => setHealthFilter(healthFilter === 'healthy' ? '' : 'healthy')}
                  className={`h-full transition-opacity ${healthFilter && healthFilter !== 'healthy' ? 'opacity-30' : 'opacity-100'}`}
                  style={{ width: `${(healthCounts.healthy / totalServices) * 100}%`, background: '#22c55e' }}
                  title={`${healthCounts.healthy} healthy`}
                />
              )}
              {healthCounts.degraded > 0 && (
                <button
                  onClick={() => setHealthFilter(healthFilter === 'degraded' ? '' : 'degraded')}
                  className={`h-full transition-opacity ${healthFilter && healthFilter !== 'degraded' ? 'opacity-30' : 'opacity-100'}`}
                  style={{ width: `${(healthCounts.degraded / totalServices) * 100}%`, background: '#eab308' }}
                  title={`${healthCounts.degraded} degraded`}
                />
              )}
              {healthCounts.progressing > 0 && (
                <button
                  onClick={() => setHealthFilter(healthFilter === 'progressing' ? '' : 'progressing')}
                  className={`h-full transition-opacity ${healthFilter && healthFilter !== 'progressing' ? 'opacity-30' : 'opacity-100'}`}
                  style={{ width: `${(healthCounts.progressing / totalServices) * 100}%`, background: '#3b82f6' }}
                  title={`${healthCounts.progressing} progressing`}
                />
              )}
              {healthCounts.failed > 0 && (
                <button
                  onClick={() => setHealthFilter(healthFilter === 'failed' ? '' : 'failed')}
                  className={`h-full transition-opacity ${healthFilter && healthFilter !== 'failed' ? 'opacity-30' : 'opacity-100'}`}
                  style={{ width: `${(healthCounts.failed / totalServices) * 100}%`, background: '#ef4444' }}
                  title={`${healthCounts.failed} failed`}
                />
              )}
              {healthCounts.unknown > 0 && (
                <button
                  onClick={() => setHealthFilter(healthFilter === 'unknown' ? '' : 'unknown')}
                  className={`h-full transition-opacity ${healthFilter && healthFilter !== 'unknown' ? 'opacity-30' : 'opacity-100'}`}
                  style={{ width: `${(healthCounts.unknown / totalServices) * 100}%`, background: '#9ca3af' }}
                  title={`${healthCounts.unknown} unknown`}
                />
              )}
            </div>
            {/* Legend */}
            <div className="flex items-center gap-3 text-[11px] shrink-0">
              <button onClick={() => setHealthFilter(healthFilter === 'healthy' ? '' : 'healthy')} className={`flex items-center gap-1 ${healthFilter === 'healthy' ? 'font-semibold' : ''}`}>
                <span className="w-2 h-2 rounded-full bg-green-500" /> {healthCounts.healthy}
              </button>
              <button onClick={() => setHealthFilter(healthFilter === 'degraded' ? '' : 'degraded')} className={`flex items-center gap-1 ${healthFilter === 'degraded' ? 'font-semibold' : ''}`}>
                <span className="w-2 h-2 rounded-full bg-yellow-500" /> {healthCounts.degraded}
              </button>
              <button onClick={() => setHealthFilter(healthFilter === 'progressing' ? '' : 'progressing')} className={`flex items-center gap-1 ${healthFilter === 'progressing' ? 'font-semibold' : ''}`}>
                <span className="w-2 h-2 rounded-full bg-blue-500" /> {healthCounts.progressing}
              </button>
              <button onClick={() => setHealthFilter(healthFilter === 'failed' ? '' : 'failed')} className={`flex items-center gap-1 ${healthFilter === 'failed' ? 'font-semibold' : ''}`}>
                <span className="w-2 h-2 rounded-full bg-red-500" /> {healthCounts.failed}
              </button>
              <button onClick={() => setHealthFilter(healthFilter === 'unknown' ? '' : 'unknown')} className={`flex items-center gap-1 ${healthFilter === 'unknown' ? 'font-semibold' : ''}`}>
                <span className="w-2 h-2 rounded-full bg-gray-400" /> {healthCounts.unknown}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Toolbar: Search + View Mode + Sort */}
      <div className="space-y-3">
        <div className="flex flex-wrap gap-3 items-center">
          {/* Search */}
          <div className="relative flex-1 min-w-[200px]">
            <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              placeholder="Search services..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="input pl-8 flex-1 min-w-[200px]"
            />
            {search && (
              <button onClick={() => setSearch('')} className="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>

          {/* View mode switcher */}
          <div className="flex items-center rounded-lg border border-[var(--border)] overflow-hidden">
            {(['table', 'cards', 'cluster'] as const).map(mode => (
              <button
                key={mode}
                onClick={() => setViewMode(mode)}
                className={`px-3 py-1.5 text-[11px] font-medium transition-colors outline-none ${viewMode === mode ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}
              >
                {mode === 'table' ? 'Table' : mode === 'cards' ? 'Cards' : 'By Cluster'}
              </button>
            ))}
          </div>

          {/* Filters toggle */}
          <button
            onClick={() => setShowFilters(!showFilters)}
            data-tour="services-filter"
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
            {hasActiveFilters && (
              <span className="w-4 h-4 rounded-full bg-[var(--accent)] text-white text-[9px] flex items-center justify-center font-bold">
                {[debouncedSearch, statusFilter, clusterFilter, sourceFilter, healthFilter].filter(Boolean).length}
              </span>
            )}
          </button>

          {hasActiveFilters && (
            <button
              onClick={() => { setStatusFilter(''); setClusterFilter(''); setSourceFilter(''); setSearch(''); setHealthFilter(''); }}
              className="text-[11px] text-[var(--accent)] hover:underline"
            >
              Clear all
            </button>
          )}
        </div>

        {/* Expandable filters */}
        {showFilters && (
          <div className="flex flex-wrap gap-3 pt-3 border-t border-[var(--border-light)]">
            <div className="flex flex-col gap-1">
              <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Source</label>
              <select value={sourceFilter} onChange={e => setSourceFilter(e.target.value)} className="select w-36">
                <option value="">All Sources</option>
                <option value="pepa">PEPA</option>
                <option value="argocd">ArgoCD</option>
                <option value="fluxcd">FluxCD</option>
                <option value="docker">Docker</option>
                <option value="docker-container">Docker Container</option>
                <option value="manual">Manual</option>
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Cluster</label>
              <select value={clusterFilter} onChange={e => setClusterFilter(e.target.value)} className="select w-40">
                <option value="">All Clusters</option>
                {clusterNames.map(name => (
                  <option key={name} value={name}>{name} ({clusters[name]})</option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Status</label>
              <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className="select w-40">
                <option value="">All Statuses</option>
                <option value="configured">Configured</option>
                <option value="active">Active</option>
                <option value="running">Running</option>
                <option value="deploying">Deploying</option>
                <option value="error">Error</option>
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Health</label>
              <select value={healthFilter} onChange={e => setHealthFilter(e.target.value)} className="select w-36">
                <option value="">All Health</option>
                <option value="healthy">Healthy</option>
                <option value="degraded">Degraded</option>
                <option value="progressing">Progressing</option>
                <option value="failed">Failed</option>
                <option value="unknown">Unknown</option>
              </select>
            </div>
          </div>
        )}

        {/* Results count + sort */}
        <div className="flex items-center justify-between">
          <div className="text-[12px] text-[var(--text-tertiary)]">
            {filteredServices.length} services
            {hasActiveFilters && <span className="ml-1">(filtered from {totalServices})</span>}
          </div>
          {viewMode === 'table' && (
            <div className="flex items-center gap-2 text-[11px]">
              <span className="text-[var(--text-tertiary)]">Sort:</span>
              {['name', 'cluster', 'health', 'replicas', 'source'].map(col => (
                <button
                  key={col}
                  onClick={() => { if (sortKey === col) setSortDir(d => d === 'asc' ? 'desc' : 'asc'); else { setSortKey(col); setSortDir('asc'); } }}
                  className={`px-2 py-0.5 rounded transition-colors ${sortKey === col ? 'bg-[var(--accent)] text-white font-medium' : 'text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)]'}`}
                >
                  {col.charAt(0).toUpperCase() + col.slice(1)}
                  {sortKey === col && (sortDir === 'desc' ? ' \u2193' : ' \u2191')}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Content */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
            <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            <p className="text-[13px]">Loading services...</p>
          </div>
        </div>
      ) : sortedServices.length === 0 ? (
        <div className="card card-body text-center py-12">
          {hasActiveFilters ? (
            <>
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">No services match your filters</p>
              <p className="text-[12px] text-[var(--text-tertiary)] mb-4">Try adjusting your search or filters</p>
              <button onClick={() => { setStatusFilter(''); setClusterFilter(''); setSourceFilter(''); setSearch(''); setHealthFilter(''); }} className="btn btn-secondary">Clear all filters</button>
            </>
          ) : (
            <>
              <div className="text-4xl mb-3 opacity-30">📦</div>
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">No services yet</p>
              <p className="text-[12px] text-[var(--text-tertiary)] mb-4">Create a service or connect ArgoCD/FluxCD to discover deployed services</p>
              <div className="flex gap-2 justify-center">
                <Link href="/services/new" className="btn btn-primary">+ Create Service</Link>
                <Link href="/discovery" className="btn btn-secondary">Open Discovery</Link>
              </div>
            </>
          )}
        </div>
      ) : viewMode === 'table' ? (
        /* TABLE VIEW */
        <div className="card">
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th style={{ width: 28 }}></th>
                  <th>Service</th>
                  <th>Cluster</th>
                  <th>Namespace</th>
                  <th>Source</th>
                  <th>Status</th>
                  <th>Health</th>
                  <th>Replicas</th>
                  <th>Image</th>
                  <th style={{ width: 60 }}></th>
                </tr>
              </thead>
              <tbody>
                {sortedServices.map((svc, idx) => {
                  const healthDot = svc.health === 'healthy' ? 'bg-green-500' : svc.health === 'degraded' ? 'bg-yellow-500' : svc.health === 'failed' ? 'bg-red-500' : 'bg-gray-400';
                  const isPepa = svc.source === 'pepa';
                  return (
                    <tr
                      key={`${svc.source}-${svc.name}-${idx}`}
                      className="cursor-pointer hover:bg-[var(--border-light)] transition-colors"
                      onClick={() => setManagingService(svc)}
                    >
                      <td><div className={`w-2 h-2 rounded-full ${healthDot}`} /></td>
                      <td><span className="font-medium text-[var(--text-primary)]">{svc.name}</span></td>
                      <td className="text-[12px] text-[var(--text-secondary)]">{svc.cluster || 'default'}</td>
                      <td className="text-[12px] text-[var(--text-secondary)]">{svc.namespace}</td>
                      <td>{sourceBadge(svc.source)}</td>
                      <td>
                        <span className={`text-[12px] font-medium ${svc.status === 'running' || svc.status === 'active' ? 'text-green-600' : svc.status === 'error' || svc.status === 'failed' ? 'text-red-600' : svc.status === 'configured' ? 'text-blue-600' : 'text-[var(--text-secondary)]'}`}>
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
                      <td className="text-[11px] text-[var(--text-tertiary)] truncate max-w-[120px] font-mono">
                        {svc.image !== '-' ? svc.image : ''}
                      </td>
                      <td>
                        {isPepa && (
                          <button
                            onClick={(e) => { e.stopPropagation(); requestDelete(svc); }}
                            className="text-[11px] px-2 py-0.5 rounded text-red-500 hover:bg-red-500/10 transition-colors"
                            title="Delete service"
                          >
                            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                              <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                            </svg>
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      ) : viewMode === 'cards' ? (
        /* CARDS VIEW */
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {sortedServices.map((svc, idx) => {
            const hbc = svc.health === 'healthy' ? 'bg-green-500' : svc.health === 'degraded' ? 'bg-yellow-500' : svc.health === 'progressing' ? 'bg-blue-500' : svc.health === 'failed' ? 'bg-red-500' : 'bg-gray-300';
            return (
              <div key={`${svc.source}-${svc.name}-${idx}`} className="card group cursor-pointer hover:border-[var(--accent)]/40 hover:shadow-sm transition-all" onClick={() => setManagingService(svc)}>
                <div className={`h-0.5 w-full rounded-t-md ${hbc}`} />
                <div className="card-body py-3 space-y-2">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <div className={`w-2 h-2 rounded-full shrink-0 ${hbc}`} />
                      <div className="min-w-0">
                        <h3 className="text-[13px] font-medium text-[var(--text-primary)] truncate">{svc.name}</h3>
                        <div className="text-[11px] text-[var(--text-tertiary)]">{svc.cluster || 'default'} / {svc.namespace}</div>
                      </div>
                    </div>
                    {sourceBadge(svc.source)}
                  </div>
                  <div className="flex items-center gap-3 text-[11px]">
                    <div className="flex items-center gap-1">
                      <span className="text-[var(--text-tertiary)]">Status:</span>
                      <span className={`font-medium ${svc.status === 'running' || svc.status === 'active' ? 'text-green-600' : 'text-[var(--text-secondary)]'}`}>{svc.status}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <span className="text-[var(--text-tertiary)]">Replicas:</span>
                      <span className="font-medium text-[var(--text-primary)]">{svc.replicas > 0 ? `${svc.ready_replicas}/${svc.replicas}` : '-'}</span>
                    </div>
                  </div>
                  {svc.image && svc.image !== '-' && (
                    <div className="text-[10px] text-[var(--text-tertiary)] truncate font-mono" title={svc.image}>{svc.image}</div>
                  )}
                  <div className="pt-2 border-t border-[var(--border-light)] flex items-center justify-between">
                    <span className="text-[10px] text-[var(--text-tertiary)]">Click to manage</span>
                    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <span className="text-[10px] px-2 py-0.5 rounded bg-[var(--accent)] text-white">Manage</span>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        /* GROUPED BY CLUSTER VIEW */
        <div className="space-y-6">
          {Object.entries(groupedByCluster).map(([clusterName, clusterServices]) => (
            <div key={clusterName}>
              <div className="flex items-center gap-2 mb-3">
                <div className="w-2.5 h-2.5 rounded-full bg-[var(--accent)]" />
                <h2 className="text-[14px] font-semibold text-[var(--text-primary)]">{clusterName}</h2>
                <span className="text-[11px] text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded">{clusterServices.length} services</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {clusterServices.map((svc, idx) => {
                  const hbc = svc.health === 'healthy' ? 'bg-green-500' : svc.health === 'degraded' ? 'bg-yellow-500' : svc.health === 'progressing' ? 'bg-blue-500' : svc.health === 'failed' ? 'bg-red-500' : 'bg-gray-300';
                  return (
                    <div key={`${svc.source}-${svc.name}-${idx}`} className="card group cursor-pointer hover:border-[var(--accent)]/40 hover:shadow-sm transition-all" onClick={() => setManagingService(svc)}>
                      <div className={`h-0.5 w-full rounded-t-md ${hbc}`} />
                      <div className="card-body py-3 space-y-2">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-2 min-w-0">
                            <div className={`w-2 h-2 rounded-full shrink-0 ${hbc}`} />
                            <div className="min-w-0">
                              <h3 className="text-[13px] font-medium text-[var(--text-primary)] truncate">{svc.name}</h3>
                              <div className="text-[11px] text-[var(--text-tertiary)]">{svc.namespace}</div>
                            </div>
                          </div>
                          {sourceBadge(svc.source)}
                        </div>
                        <div className="flex items-center gap-3 text-[11px]">
                          <span className={`font-medium ${svc.status === 'running' || svc.status === 'active' ? 'text-green-600' : 'text-[var(--text-secondary)]'}`}>{svc.status}</span>
                          <span className="text-[var(--text-tertiary)]">{svc.replicas > 0 ? `${svc.ready_replicas}/${svc.replicas}` : '-'}</span>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Service Management Slide-out Panel */}
      {managingService && (
        <ServiceManagementPanel
          service={managingService}
          onClose={() => setManagingService(null)}
          onUpdate={loadData}
        />
      )}

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        open={!!deleteTarget}
        title="Delete Service"
        description={`Are you sure you want to delete "${deleteTarget?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
        icon={
          <div className="w-12 h-12 rounded-full bg-red-500/15 flex items-center justify-center">
            <svg className="w-6 h-6 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
            </svg>
          </div>
        }
      />
      </div>
      </div>
    </div>
  );
}
