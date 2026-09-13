'use client';

import { useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { services, type Service } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';
import ConfirmModal from '@/components/ConfirmModal';
import ServiceDetailClient from './ServiceDetailClient';
import { SkeletonTable } from '@/components/Skeleton';
import { useUrlFilters } from '@/hooks/useUrlFilters';
import FilterBar from '@/components/filters/FilterBar';
import FilterChips, { type ActiveChip } from '@/components/filters/FilterChips';
import QuickFilter from '@/components/filters/QuickFilter';
import SearchInput from '@/components/filters/SearchInput';
import { fieldLabel, valueLabel, valueTone } from '@/lib/filter-labels';

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
  const router = useRouter();
  const [servicesList, setServicesList] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [viewMode, setViewMode] = useState<'table' | 'cards'>('table');
  const [sortKey, setSortKey] = useState<string>('name');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  const filters = useUrlFilters({
    single: ['search', 'status'],
  });
  const searchValue = filters.get('search');
  const statusValue = filters.get('status');

  const [deleteTarget, setDeleteTarget] = useState<Service | null>(null);
  const [deleting, setDeleting] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const params: Record<string, string> = {};
      if (searchValue) params.search = searchValue;
      if (statusValue) params.status = statusValue;
      const data = await services.list(params).catch(() => ({ items: [], total: 0 }));
      setServicesList(data.items || []);
    } catch (err) {
      console.error('Failed to load services:', err);
    } finally {
      setLoading(false);
    }
  }, [searchValue, statusValue]);

  useEffect(() => {
    loadData();
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

  const statusColor = (status: string) => {
    switch (status) {
      case 'active': case 'running': case 'healthy': return 'bg-emerald-500/15 text-emerald-600';
      case 'deploying': case 'progressing': return 'bg-amber-500/15 text-amber-600';
      case 'configured': return 'bg-blue-500/15 text-blue-500';
      case 'error': case 'failed': return 'bg-red-500/15 text-red-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
    }
  };

  const filteredServices = useMemo(() => {
    return [...servicesList].sort((a, b) => {
      const dir = sortDir === 'asc' ? 1 : -1;
      switch (sortKey) {
        case 'name': return dir * a.name.localeCompare(b.name);
        case 'status': return dir * a.status.localeCompare(b.status);
        case 'updated': return dir * (new Date(a.updated_at || 0).getTime() - new Date(b.updated_at || 0).getTime());
        default: return 0;
      }
    });
  }, [servicesList, sortKey, sortDir]);

  const totalServices = servicesList.length;
  const statusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    servicesList.forEach(s => { counts[s.status] = (counts[s.status] || 0) + 1; });
    return counts;
  }, [servicesList]);

  const statusOptions = useMemo(() => [
    { value: 'active', label: 'Active', tone: 'success' as const },
    { value: 'running', label: 'Running', tone: 'success' as const },
    { value: 'deploying', label: 'Deploying', tone: 'info' as const },
    { value: 'configured', label: 'Configured', tone: 'info' as const },
    { value: 'error', label: 'Error', tone: 'danger' as const },
    { value: 'failed', label: 'Failed', tone: 'danger' as const },
  ], []);

  const chips: ActiveChip[] = [];
  if (searchValue) {
    chips.push({ id: 'search', field: fieldLabel('search'), label: `"${searchValue}"`, onRemove: () => filters.set('search', '') });
  }
  if (statusValue) {
    chips.push({ id: 'status', field: fieldLabel('status'), label: valueLabel(statusValue), tone: valueTone(statusValue), onRemove: () => filters.set('status', '') });
  }

  const resultSummary = `${filteredServices.length} service${filteredServices.length !== 1 ? 's' : ''}`;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-5">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Service Catalog</h1>
              <ConceptHelp term="service" />
            </div>
            <p className="page-subtitle-modern">
              Manage your logical service definitions
              {totalServices > 0 && <span> &middot; {totalServices} services</span>}
            </p>
          </div>
          <div className="flex gap-2">
            <Link href="/workloads" className="btn btn-secondary btn-sm">
              View Workloads
            </Link>
            <Link href="/services/new" className="btn btn-primary" data-tour="services-create">
              + New Service
            </Link>
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-4 gap-4 page-animate-up">
          {[
            { label: 'Total Services', value: totalServices, color: 'text-[var(--text-primary)]' },
            { label: 'Active', value: (statusCounts['active'] || 0) + (statusCounts['running'] || 0), color: 'text-emerald-600' },
            { label: 'Deploying', value: statusCounts['deploying'] || 0, color: 'text-amber-600' },
            { label: 'Failed', value: (statusCounts['error'] || 0) + (statusCounts['failed'] || 0), color: 'text-red-600' },
          ].map(s => (
            <div key={s.label} className="card card-body py-3 flex items-center gap-3">
              <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
              <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
            </div>
          ))}
        </div>

        {/* Filters */}
        <FilterBar
          search={
            <SearchInput
              value={searchValue}
              onCommit={value => filters.set('search', value)}
              placeholder="Search services..."
              label="Search services"
              loading={loading}
            />
          }
          quick={
            <QuickFilter
              field="status"
              label={fieldLabel('status')}
              options={statusOptions}
              value={statusValue}
              onChange={value => filters.set('status', value)}
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

        {/* View mode + sort row */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {/* View mode switcher */}
            <div className="flex items-center rounded-lg border border-[var(--border)] overflow-hidden">
              {(['table', 'cards'] as const).map(mode => (
                <button
                  key={mode}
                  onClick={() => setViewMode(mode)}
                  className={`px-3 py-1.5 text-[11px] font-medium transition-colors outline-none ${viewMode === mode ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}
                >
                  {mode === 'table' ? 'Table' : 'Cards'}
                </button>
              ))}
            </div>
          </div>
          {viewMode === 'table' && (
            <div className="flex items-center gap-2 text-[11px]">
              <span className="text-[var(--text-tertiary)]">Sort:</span>
              {[
                { key: 'name', label: 'Name' },
                { key: 'status', label: 'Status' },
                { key: 'updated', label: 'Updated' },
              ].map(col => (
                <button
                  key={col.key}
                  onClick={() => { if (sortKey === col.key) setSortDir(d => d === 'asc' ? 'desc' : 'asc'); else { setSortKey(col.key); setSortDir('asc'); } }}
                  className={`px-2 py-0.5 rounded transition-colors ${sortKey === col.key ? 'bg-[var(--accent)] text-white font-medium' : 'text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)]'}`}
                >
                  {col.label}
                  {sortKey === col.key && (sortDir === 'desc' ? ' \u2193' : ' \u2191')}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Content */}
        {loading ? (
          <SkeletonTable rows={6} cols={4} />
        ) : filteredServices.length === 0 ? (
          <div className="card card-body text-center py-12">
            {(searchValue || statusValue) ? (
              <>
                <p className="text-[13px] text-[var(--text-secondary)] mb-1">No services match your filters</p>
                <p className="text-[12px] text-[var(--text-tertiary)] mb-4">Try adjusting your search or filters</p>
                <button onClick={() => filters.clear()} className="btn btn-secondary">Clear all filters</button>
              </>
            ) : (
              <>
                <div className="text-4xl mb-3 opacity-30">&#x1F4E6;</div>
                <p className="text-[13px] text-[var(--text-secondary)] mb-1">No services yet</p>
                <p className="text-[12px] text-[var(--text-tertiary)] mb-4">Create a service to start tracking your application catalog</p>
                <div className="flex gap-2 justify-center">
                  <Link href="/services/new" className="btn btn-primary">+ Create Service</Link>
                  <Link href="/workloads" className="btn btn-secondary">View Workloads</Link>
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
                    <th>Service</th>
                    <th>Namespace</th>
                    <th>Status</th>
                    <th>Updated</th>
                    <th style={{ width: 60 }}></th>
                  </tr>
                </thead>
                <tbody>
                  {filteredServices.map((svc) => (
                    <tr
                      key={svc.id}
                      className="cursor-pointer hover:bg-[var(--border-light)] transition-colors"
                      onClick={() => router.push(`/services?id=${svc.id}`)}
                    >
                      <td><span className="font-medium text-[var(--text-primary)]">{svc.name}</span></td>
                      <td className="text-[12px] text-[var(--text-secondary)]">{svc.namespace || '-'}</td>
                      <td>
                        <span className={`text-[12px] px-2 py-0.5 rounded-full font-medium ${statusColor(svc.status)}`}>
                          {svc.status}
                        </span>
                      </td>
                      <td className="text-[12px] text-[var(--text-tertiary)]">
                        {svc.updated_at ? new Date(svc.updated_at).toLocaleDateString() : '-'}
                      </td>
                      <td>
                        <button
                          onClick={(e) => { e.stopPropagation(); setDeleteTarget(svc); }}
                          className="text-[11px] px-2 py-0.5 rounded text-red-500 hover:bg-red-500/10 transition-colors"
                          title="Delete service"
                        >
                          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                          </svg>
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          /* CARDS VIEW */
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
            {filteredServices.map((svc) => {
              const sc = svc.status === 'active' || svc.status === 'running' ? 'bg-green-500' : svc.status === 'deploying' ? 'bg-blue-500' : svc.status === 'error' || svc.status === 'failed' ? 'bg-red-500' : 'bg-gray-400';
              return (
                <div key={svc.id} className="card group cursor-pointer hover:border-[var(--accent)]/40 hover:shadow-sm transition-all" onClick={() => router.push(`/services?id=${svc.id}`)}>
                  <div className={`h-0.5 w-full rounded-t-md ${sc}`} />
                  <div className="card-body py-3 space-y-2">
                    <div className="flex items-start justify-between">
                      <div className="flex items-center gap-2 min-w-0">
                        <div className={`w-2 h-2 rounded-full shrink-0 ${sc}`} />
                        <div className="min-w-0">
                          <h3 className="text-[13px] font-medium text-[var(--text-primary)] truncate">{svc.name}</h3>
                          {svc.namespace && <div className="text-[11px] text-[var(--text-tertiary)]">{svc.namespace}</div>}
                        </div>
                      </div>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${statusColor(svc.status)}`}>{svc.status}</span>
                    </div>
                    <div className="pt-2 border-t border-[var(--border-light)] flex items-center justify-between">
                      <span className="text-[10px] text-[var(--text-tertiary)]">
                        {svc.updated_at ? `Updated ${new Date(svc.updated_at).toLocaleDateString()}` : ''}
                      </span>
                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                        <span className="text-[10px] px-2 py-0.5 rounded bg-[var(--accent)] text-white">View</span>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
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
  );
}
