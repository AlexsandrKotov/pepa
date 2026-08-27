'use client';

import { useState, useEffect, useCallback, Suspense } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { entities, type Entity } from '@/lib/api';
import { useDebounce } from '@/hooks/useDebounce';
import Pagination from '@/components/Pagination';
import ConceptHelp from '@/components/ConceptHelp';
import EntityDetailClient from './EntityDetailClient';

function EntitiesPageContent() {
  const searchParams = useSearchParams();
  const entityId = searchParams.get('id');

  if (entityId) {
    return <EntityDetailClient entityId={entityId} />;
  }

  return <EntitiesList />;
}

export default function EntitiesPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <EntitiesPageContent />
    </Suspense>
  );
}

function EntitiesList() {
  const [items, setItems] = useState<Entity[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [page, setPage] = useState(1);
  const [perPage] = useState(20);
  const [total, setTotal] = useState(0);
  const debouncedSearch = useDebounce(search, 300);

  useEffect(() => {
    setPage(1);
  }, [debouncedSearch, typeFilter]);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { page: String(page), per_page: String(perPage) };
      if (debouncedSearch) params.search = debouncedSearch;
      if (typeFilter) params.type_key = typeFilter;
      const data = await entities.list(params).catch(() => ({ items: [], total: 0, page: 1, per_page: 20, total_pages: 0 }));
      setItems(data.items || []);
      setTotal(data.total || 0);
    } catch (err) {
      console.error('Failed to load entities:', err);
    } finally {
      setLoading(false);
    }
  }, [debouncedSearch, typeFilter, page, perPage]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const statusBadge = (status: string) => {
    switch (status) {
      case 'active': return 'badge-success';
      case 'deprecated': return 'badge-danger';
      default: return 'badge-warning';
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Entities</h1>
              <ConceptHelp term="entity" />
            </div>
            <p className="page-subtitle-modern">All platform entities — services, resources, and components</p>
          </div>
          <a href="/services/new" className="btn btn-primary">
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Register Service
          </a>
        </div>
      </div>

      {/* Filters */}
      <div className="flex gap-3 page-animate-up page-delay-1">
        <input
          type="text"
          placeholder="Search entities..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="input flex-[3]"
        />
        <select
          value={typeFilter}
          onChange={e => setTypeFilter(e.target.value)}
          className="input flex-1 min-w-[140px]"
        >
          <option value="">All types</option>
          <option value="service">Service</option>
          <option value="resource">Resource</option>
          <option value="team">Team</option>
          <option value="user">User</option>
        </select>
      </div>

      {/* List */}
      {loading ? (
        <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
          <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
            <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            <p className="text-[13px]">Loading entities...</p>
          </div>
        </div>
      ) : items.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30">📦</div>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1">No entities found</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-4">
            Entities represent services, teams, and infrastructure in your catalog.<br />
            Register your first service to get started, or import from connected tools.
          </p>
          <div className="flex gap-2 justify-center">
            <a href="/services/new" className="btn btn-primary">Register Service</a>
            <a href="/connections" className="btn btn-secondary">Connect Tools</a>
          </div>
        </div>
      ) : (
        <div className="card" style={{ borderRadius: '12px' }}>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Entity</th>
                  <th>Type</th>
                  <th>Status</th>
                  <th>Sync</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {items.map(ent => (
                  <tr key={ent.id}>
                    <td>
                      <Link href={`/entities?id=${ent.id}`} className="font-medium text-[var(--text-primary)] hover:text-[var(--accent)]">
                        {ent.name}
                      </Link>
                      {ent.description && (
                        <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">{ent.description}</p>
                      )}
                    </td>
                    <td><span className="badge badge-default">{ent.type_key}</span></td>
                    <td><span className={`badge ${statusBadge(ent.status)}`}>{ent.status}</span></td>
                    <td><span className="text-[12px] text-[var(--text-secondary)]">{ent.sync_status || '-'}</span></td>
                    <td className="text-[12px] text-[var(--text-tertiary)]">{new Date(ent.updated_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={page} perPage={perPage} total={total} onPageChange={setPage} />
        </div>
      )}
      </div>
    </div>
  );
}
