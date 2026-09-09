'use client';

import { useState, useEffect, useCallback, Suspense } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { entities, discovery, scorecards, type Entity, type DiscoveredService, type ScorecardResult } from '@/lib/api';
import { Toast } from '@/components/Interactive';
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
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const debouncedSearch = useDebounce(search, 300);

  // Sync status
  const [syncStatus, setSyncStatus] = useState<{ last_synced_at: string | null; status: string } | null>(null);
  // Scorecard levels per entity (level + percentage)
  const [entityScores, setEntityScores] = useState<Record<string, { level: string; pct: number }>>({});

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
    // Load sync status
    entities.syncStatus().then(setSyncStatus).catch(() => {});
  }, [loadData]);

  // Fetch scorecard levels for loaded entities
  useEffect(() => {
    if (items.length === 0) return;
    const scores: Record<string, { level: string; pct: number }> = {};
    const fetchScores = async () => {
      for (const ent of items) {
        try {
          const data = await scorecards.entityScores(ent.id);
          const resultScores = data.scores || [];
          if (resultScores.length > 0) {
            // Use the best score (highest percentage)
            const best = resultScores.reduce((a: ScorecardResult, b: ScorecardResult) => {
              const pctA = a.max_score > 0 ? (a.score / a.max_score) * 100 : 0;
              const pctB = b.max_score > 0 ? (b.score / b.max_score) * 100 : 0;
              return pctB > pctA ? b : a;
            });
            const pct = best.max_score > 0 ? Math.round((best.score / best.max_score) * 100) : 0;
            scores[ent.id] = { level: best.level, pct };
          }
        } catch { /* ignore */ }
      }
      setEntityScores(scores);
    };
    fetchScores();
  }, [items]);

  const handleSync = async () => {
    setSyncing(true);
    try {
      const result = await entities.sync();
      setToast({ message: `Sync complete: ${result.created} created, ${result.updated} updated`, type: 'success' });
      await loadData();
      entities.syncStatus().then(setSyncStatus).catch(() => {});
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Sync failed', type: 'error' });
    } finally {
      setSyncing(false);
    }
  };

  const statusBadge = (status: string) => {
    switch (status) {
      case 'active': return 'badge-success';
      case 'deprecated': return 'badge-danger';
      default: return 'badge-warning';
    }
  };

  const sourceLabel = (ent: Entity) => {
    if (ent.external_id) {
      const source = ent.external_id.split(':')[0];
      return source;
    }
    return 'manual';
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        {/* Import modal */}
        {showImport && (
          <ImportDiscoveryModal
            onClose={() => setShowImport(false)}
            onImported={() => { setShowImport(false); loadData(); }}
          />
        )}

        {/* Header */}
        <div className="page-animate">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2">
                <h1 className="page-title-modern">Entities</h1>
                <ConceptHelp term="entity" />
              </div>
              <p className="page-subtitle-modern">
                All platform entities — services, resources, and components
                {syncStatus?.last_synced_at && (
                  <span className="ml-2 text-[11px] text-[var(--text-tertiary)]">
                    Last sync: {new Date(syncStatus.last_synced_at).toLocaleString()}
                  </span>
                )}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={handleSync}
                disabled={syncing}
                className="btn btn-secondary"
                title="Sync entities from connected clusters and services"
              >
                <svg className={`w-4 h-4 ${syncing ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                {syncing ? 'Syncing...' : 'Sync'}
              </button>
              <button onClick={() => setShowImport(true)} className="btn btn-secondary">
                Import from Discovery
              </button>
              <Link href="/entities/new" className="btn btn-primary">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                </svg>
                Create Entity
              </Link>
            </div>
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
            <option value="environment">Environment</option>
            <option value="api_endpoint">API Endpoint</option>
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
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <div className="text-4xl mb-3 opacity-30">📦</div>
            <p className="text-[14px] font-medium text-[var(--text-primary)] mb-1">No entities found</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mb-4">
              Entities represent services, teams, and infrastructure in your catalog.<br />
              Create manually, import from Discovery, or sync from connected clusters.
            </p>
            <div className="flex gap-2 justify-center">
              <Link href="/entities/new" className="btn btn-primary">Create Entity</Link>
              <button onClick={() => setShowImport(true)} className="btn btn-secondary">Import from Discovery</button>
              <button onClick={handleSync} disabled={syncing} className="btn btn-secondary">
                {syncing ? 'Syncing...' : 'Sync Now'}
              </button>
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
                    <th>Source</th>
                    <th>Status</th>
                    <th>Score</th>
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
                      <td>
                        <span className={`text-[11px] px-1.5 py-0.5 rounded ${
                          sourceLabel(ent) === 'manual' ? 'bg-slate-500/10 text-slate-500' :
                          sourceLabel(ent) === 'argocd' ? 'bg-indigo-500/10 text-indigo-500' :
                          sourceLabel(ent) === 'fluxcd' ? 'bg-purple-500/10 text-purple-500' :
                          sourceLabel(ent) === 'docker' || sourceLabel(ent) === 'docker-container' ? 'bg-cyan-500/10 text-cyan-600' :
                          'bg-emerald-500/10 text-emerald-600'
                        }`}>
                          {sourceLabel(ent)}
                        </span>
                      </td>
                      <td><span className={`badge ${statusBadge(ent.status)}`}>{ent.status}</span></td>
                      <td>
                        {entityScores[ent.id] ? (
                          <span className="flex items-center gap-1.5">
                            <span className={`text-[10px] px-1.5 py-0.5 rounded ${
                              entityScores[ent.id].level === 'platinum' ? 'bg-indigo-500/10 text-indigo-500' :
                              entityScores[ent.id].level === 'gold' ? 'bg-amber-500/10 text-amber-600' :
                              entityScores[ent.id].level === 'silver' ? 'bg-slate-500/10 text-slate-500' :
                              entityScores[ent.id].level === 'bronze' ? 'bg-orange-500/10 text-orange-600' :
                              'bg-[var(--bg-tertiary)] text-[var(--text-tertiary)]'
                            }`}>
                              {entityScores[ent.id].level === 'none' ? '—' : entityScores[ent.id].level}
                            </span>
                            <span className="text-[10px] text-[var(--text-tertiary)]">{entityScores[ent.id].pct}%</span>
                          </span>
                        ) : (
                          <span className="text-[11px] text-[var(--text-tertiary)]">—</span>
                        )}
                      </td>
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

// ── Import from Discovery Modal ────────────────────────────────

function ImportDiscoveryModal({ onClose, onImported }: { onClose: () => void; onImported: () => void }) {
  const [discovered, setDiscovered] = useState<DiscoveredService[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [importing, setImporting] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [sourceFilter, setSourceFilter] = useState('');
  const [existingExternalIds, setExistingExternalIds] = useState<Set<string>>(new Set());

  useEffect(() => {
    const load = async () => {
      try {
        // Fetch discovered services and existing entities in parallel
        const [discData, entData] = await Promise.all([
          discovery.services().catch(() => ({ services: [] })),
          entities.list({ per_page: '1000' }).catch(() => ({ items: [], total: 0, page: 1, per_page: 1000, total_pages: 0 })),
        ]);
        setDiscovered(discData.services || []);
        // Build set of already-imported external_ids (format: {source}:{cluster}:{namespace}:{name})
        const ids = new Set<string>();
        for (const ent of (entData.items || [])) {
          if (ent.external_id) ids.add(ent.external_id);
        }
        setExistingExternalIds(ids);
      } catch {
        setDiscovered([]);
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  const toggleSelect = (index: number) => {
    const next = new Set(selected);
    if (next.has(index)) next.delete(index); else next.add(index);
    setSelected(next);
  };

  const selectAll = () => {
    if (selected.size === filtered.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(filtered.map((_, i) => i)));
    }
  };

  // Filter by source, then exclude already-imported services
  const filtered = discovered.filter(svc => {
    if (sourceFilter && svc.source !== sourceFilter) return false;
    const externalId = `${svc.source}:${svc.cluster}:${svc.namespace}:${svc.name}`;
    if (existingExternalIds.has(externalId)) return false;
    return true;
  });

  // Count how many discovered services match the source filter (before dedup filtering)
  const sourceFilteredCount = sourceFilter
    ? discovered.filter(s => s.source === sourceFilter).length
    : discovered.length;
  const alreadyImportedCount = sourceFilteredCount - filtered.length;

  const handleImport = async () => {
    const items = Array.from(selected).map(i => {
      const svc = filtered[i];
      return { name: svc.name, namespace: svc.namespace, cluster: svc.cluster, source: svc.source };
    });
    if (items.length === 0) return;

    setImporting(true);
    try {
      const result = await entities.importDiscovery(items);
      setToast({ message: `Imported ${result.imported} entities`, type: 'success' });
      setTimeout(onImported, 800);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Import failed', type: 'error' });
    } finally {
      setImporting(false);
    }
  };

  const sourceBadge = (source: string) => {
    const colors: Record<string, string> = {
      argocd: 'bg-indigo-500/10 text-indigo-500',
      fluxcd: 'bg-purple-500/10 text-purple-500',
      pepa: 'bg-emerald-500/10 text-emerald-600',
      docker: 'bg-cyan-500/10 text-cyan-600',
      'docker-container': 'bg-cyan-500/10 text-cyan-600',
    };
    return colors[source] || 'bg-slate-500/10 text-slate-500';
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="card w-full max-w-3xl max-h-[80vh] flex flex-col" style={{ borderRadius: '12px' }} onClick={e => e.stopPropagation()}>
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        <div className="card-header flex items-center justify-between shrink-0">
          <span className="text-[14px] font-medium text-[var(--text-primary)]">Import from Discovery</span>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-[14px]">✕</button>
        </div>

        <div className="card-body flex-1 overflow-y-auto space-y-3">
          {loading ? (
            <div className="text-center py-8 text-[13px] text-[var(--text-tertiary)]">Scanning clusters for services...</div>
          ) : discovered.length === 0 ? (
            <div className="text-center py-8">
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">No services discovered</p>
              <p className="text-[12px] text-[var(--text-tertiary)]">Connect a cluster or add services to see discovered resources here.</p>
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-center py-8">
              <p className="text-[13px] text-[var(--text-secondary)] mb-1">All discovered services are already imported</p>
              <p className="text-[12px] text-[var(--text-tertiary)]">{alreadyImportedCount} service{alreadyImportedCount !== 1 ? 's' : ''} already in your entities list.</p>
            </div>
          ) : (
            <>
              {alreadyImportedCount > 0 && (
                <p className="text-[11px] text-[var(--text-tertiary)]">
                  {alreadyImportedCount} already imported service{alreadyImportedCount !== 1 ? 's' : ''} hidden
                </p>
              )}
              {/* Filters */}
              <div className="flex items-center gap-2">
                <select value={sourceFilter} onChange={e => setSourceFilter(e.target.value)} className="input flex-1">
                  <option value="">All sources</option>
                  <option value="argocd">ArgoCD</option>
                  <option value="fluxcd">FluxCD</option>
                  <option value="pepa">PEPA</option>
                  <option value="docker">Docker</option>
                </select>
                <button onClick={selectAll} className="btn btn-secondary btn-sm text-[11px]">
                  {selected.size === filtered.length ? 'Deselect All' : 'Select All'}
                </button>
              </div>

              {/* Services list */}
              <div className="space-y-1">
                {filtered.map((svc, i) => (
                  <label
                    key={`${svc.cluster}-${svc.namespace}-${svc.name}-${i}`}
                    className={`flex items-center gap-3 p-2.5 rounded-lg border cursor-pointer transition-all ${
                      selected.has(i) ? 'border-[var(--accent)] bg-[var(--accent)]/5' : 'border-[var(--border-light)] hover:border-[var(--border)]'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={selected.has(i)}
                      onChange={() => toggleSelect(i)}
                      className="rounded"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-[12px] font-medium text-[var(--text-primary)] truncate">{svc.name}</span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded ${sourceBadge(svc.source)}`}>{svc.source}</span>
                      </div>
                      <div className="text-[11px] text-[var(--text-tertiary)]">
                        {svc.cluster} / {svc.namespace}
                        {svc.image && <span className="ml-2 font-mono">{svc.image.split('/').pop()?.split(':')[0]}</span>}
                      </div>
                    </div>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded ${
                      svc.health === 'healthy' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-amber-500/10 text-amber-600'
                    }`}>
                      {svc.health || svc.status}
                    </span>
                  </label>
                ))}
              </div>
            </>
          )}
        </div>

        <div className="card-header flex items-center justify-between shrink-0 border-t border-[var(--border-light)]">
          <span className="text-[12px] text-[var(--text-tertiary)]">
            {selected.size} of {filtered.length} selected
          </span>
          <div className="flex gap-2">
            <button onClick={onClose} className="btn btn-secondary btn-sm">Cancel</button>
            <button
              onClick={handleImport}
              disabled={selected.size === 0 || importing}
              className="btn btn-primary btn-sm"
            >
              {importing ? 'Importing...' : `Import ${selected.size > 0 ? `${selected.size} items` : ''}`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
