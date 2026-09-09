'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { entities, scorecards, type Entity, type Relationship } from '@/lib/api';
import { EntityEditButton, Toast } from '@/components/Interactive';

interface Props {
  entityId: string;
}

export default function EntityDetailClient({ entityId }: Props) {
  const [entity, setEntity] = useState<Entity | null>(null);
  const [relationships, setRelationships] = useState<Relationship[]>([]);
  const [relatedEntityNames, setRelatedEntityNames] = useState<Record<string, string>>({});
  const [nodes, setNodes] = useState<Array<{ id: string; name: string; type: string }>>([]);
  const [edges, setEdges] = useState<Array<{ id: string; source: string; target: string }>>([]);
  const [scores, setScores] = useState<Array<{ id: string; level: string; score: number; max_score: number; pass_count: number; total_rules: number }>>([]);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Add relationship form
  const [showAddRel, setShowAddRel] = useState(false);
  const [relTarget, setRelTarget] = useState('');
  const [relType, setRelType] = useState('depends_on');
  const [allEntities, setAllEntities] = useState<Entity[]>([]);
  const [loadingEntities, setLoadingEntities] = useState(false);
  const [evaluating, setEvaluating] = useState(false);

  useEffect(() => {
    Promise.all([
      entities.get(entityId).catch(() => null),
      entities.relationships(entityId).catch(() => ({ relationships: [] })),
      entities.graph(entityId, 2).catch(() => ({ nodes: [], edges: [] })),
      scorecards.entityScores(entityId).catch(() => ({ scores: [], total: 0 })),
    ]).then(([entityData, relsData, graphData, scoresData]) => {
      setEntity(entityData);
      const rels = relsData.relationships || [];
      setRelationships(rels);
      setNodes((graphData.nodes || []) as Array<{ id: string; name: string; type: string }>);
      setEdges((graphData.edges || []) as Array<{ id: string; source: string; target: string }>);
      setScores(scoresData.scores || []);

      // Fetch names for related entities
      const relatedIds = rels.map(r => r.target_id === entityId ? r.source_id : r.target_id);
      if (relatedIds.length > 0) {
        const namesMap: Record<string, string> = {};
        Promise.all(relatedIds.map(id => entities.get(id).catch(() => null))).then(results => {
          results.forEach((e, i) => {
            if (e) namesMap[relatedIds[i]] = e.name;
          });
          setRelatedEntityNames(namesMap);
        });
      }
    }).finally(() => setLoading(false));
  }, [entityId]);

  const openAddRelationship = async () => {
    setShowAddRel(true);
    if (allEntities.length === 0) {
      setLoadingEntities(true);
      try {
        const data = await entities.list({ per_page: '100' });
        setAllEntities((data.items || []).filter(e => e.id !== entityId));
      } catch { /* ignore */ }
      finally { setLoadingEntities(false); }
    }
  };

  const handleAddRelationship = async () => {
    if (!relTarget || !relType) return;
    try {
      await entities.createRelationship(entityId, { target_id: relTarget, type_key: relType });
      // Refresh relationships
      const relsData = await entities.relationships(entityId);
      setRelationships(relsData.relationships || []);
      // Fetch new entity name
      const targetEntity = await entities.get(relTarget).catch(() => null);
      if (targetEntity) {
        setRelatedEntityNames(prev => ({ ...prev, [relTarget]: targetEntity.name }));
      }
      setToast({ message: 'Relationship added', type: 'success' });
      setShowAddRel(false);
      setRelTarget('');
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to add relationship', type: 'error' });
    }
  };

  const handleDeleteRelationship = async (relId: string) => {
    try {
      await entities.deleteRelationship(relId);
      setRelationships(prev => prev.filter(r => r.id !== relId));
      setToast({ message: 'Relationship removed', type: 'success' });
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to delete', type: 'error' });
    }
  };

  const handleEvaluate = async () => {
    setEvaluating(true);
    try {
      // Evaluate all enabled scorecards for this entity
      const scData = await scorecards.list();
      const activeScorecards = (scData.scorecards || []).filter(s => s.enabled);
      for (const sc of activeScorecards) {
        await scorecards.evaluate(sc.id, entityId).catch(() => null);
      }
      // Refresh scores
      const scoresData = await scorecards.entityScores(entityId).catch(() => ({ scores: [], total: 0 }));
      setScores(scoresData.scores || []);
      setToast({ message: `Evaluated ${activeScorecards.length} scorecard(s)`, type: 'success' });
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Evaluation failed', type: 'error' });
    } finally {
      setEvaluating(false);
    }
  };

  if (loading) return <div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>;

  if (!entity) {
    return (
      <div className="empty-state">
        <h3>Entity not found</h3>
        <p>This entity may have been removed.</p>
        <Link href="/entities" className="btn btn-primary mt-4">Back to Entities</Link>
      </div>
    );
  }

  const relTypes = [
    { value: 'depends_on', label: 'Depends On' },
    { value: 'owned_by', label: 'Owned By' },
    { value: 'deployed_to', label: 'Deployed To' },
    { value: 'part_of', label: 'Part Of' },
  ];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        {/* Header */}
        <div className="card page-animate" style={{ borderRadius: '12px' }}>
          <div className="card-body">
            <div className="flex items-start justify-between">
              <div className="flex items-start gap-3">
                <div className="w-10 h-10 rounded-lg bg-[var(--border-light)] flex items-center justify-center text-[15px] font-semibold text-[var(--text-secondary)]">
                  {entity.name.charAt(0).toUpperCase()}
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <h1 className="text-[16px] font-semibold text-[var(--text-primary)]">{entity.name}</h1>
                    <span className="badge badge-default">{entity.type_key}</span>
                    <span className={`badge ${
                      entity.status === 'active' ? 'badge-success' :
                      entity.status === 'deprecated' ? 'badge-danger' :
                      'badge-warning'
                    }`}>
                      {entity.status}
                    </span>
                    {entity.external_id && (
                      <span className="text-[10px] text-[var(--text-tertiary)] font-mono">ext: {entity.external_id.split(':').pop()}</span>
                    )}
                  </div>
                  {entity.description && <p className="text-[13px] text-[var(--text-secondary)] mt-1">{entity.description}</p>}
                  <div className="flex items-center gap-3 mt-2 text-[11px] text-[var(--text-tertiary)]">
                    <span className="text-mono">{entity.id.slice(0, 8)}</span>
                    <span>Created {new Date(entity.created_at).toLocaleDateString()}</span>
                    {entity.sync_status && <span>Sync: {entity.sync_status}</span>}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleEvaluate}
                  disabled={evaluating}
                  className="btn btn-secondary btn-sm"
                  title="Evaluate all scorecards for this entity"
                >
                  {evaluating ? 'Evaluating...' : '📋 Evaluate'}
                </button>
                <Link href={`/entities/${entity.id}/edit?id=${entity.id}`} className="btn btn-secondary btn-sm">Edit</Link>
                <Link href="/entities" className="btn btn-secondary btn-sm">Back</Link>
              </div>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-3 gap-4">
          {/* Metadata */}
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Metadata</h2>
            </div>
            <div className="card-body">
              {entity.metadata && Object.keys(entity.metadata).length > 0 ? (
                <dl className="space-y-2">
                  {Object.entries(entity.metadata).map(([key, value]) => (
                    <div key={key} className="flex justify-between items-start gap-2">
                      <dt className="text-[12px] text-[var(--text-tertiary)] shrink-0">{key}</dt>
                      <dd className="text-[13px] text-[var(--text-primary)] font-medium text-right truncate" title={String(value)}>{String(value)}</dd>
                    </div>
                  ))}
                </dl>
              ) : (
                <p className="text-[13px] text-[var(--text-tertiary)]">No metadata</p>
              )}
            </div>
          </div>

          {/* Relationships */}
          <div className="card">
            <div className="card-header flex items-center justify-between">
              <div className="flex items-center gap-2">
                <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Relationships</h2>
                <span className="text-[12px] text-[var(--text-tertiary)]">{relationships.length}</span>
              </div>
              <button onClick={openAddRelationship} className="text-[11px] text-[var(--accent)] hover:underline">+ Add</button>
            </div>
            <div className="card-body">
              {/* Add relationship form */}
              {showAddRel && (
                <div className="mb-3 p-3 rounded-lg border border-[var(--border-light)] space-y-2">
                  <select value={relType} onChange={e => setRelType(e.target.value)} className="input text-[12px]">
                    {relTypes.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
                  </select>
                  {loadingEntities ? (
                    <div className="text-[11px] text-[var(--text-tertiary)] py-1">Loading entities...</div>
                  ) : (
                    <select value={relTarget} onChange={e => setRelTarget(e.target.value)} className="input text-[12px]">
                      <option value="">Select entity...</option>
                      {allEntities.map(e => (
                        <option key={e.id} value={e.id}>{e.name} ({e.type_key})</option>
                      ))}
                    </select>
                  )}
                  <div className="flex gap-2">
                    <button onClick={handleAddRelationship} disabled={!relTarget} className="btn btn-primary btn-sm text-[11px]">Add</button>
                    <button onClick={() => { setShowAddRel(false); setRelTarget(''); }} className="btn btn-secondary btn-sm text-[11px]">Cancel</button>
                  </div>
                </div>
              )}

              {relationships.length === 0 && !showAddRel ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">No relationships</p>
              ) : (
                <div className="space-y-2">
                  {relationships.map((r) => {
                    const targetId = r.target_id === entityId ? r.source_id : r.target_id;
                    return (
                      <div key={r.id} className="flex items-center justify-between p-2 rounded hover:bg-[var(--bg)] transition-colors group">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="badge badge-default text-[10px]">{r.type_key}</span>
                          <Link
                            href={`/entities?id=${targetId}`}
                            className="text-[12px] text-[var(--text-primary)] hover:text-[var(--accent)] truncate"
                          >
                            {relatedEntityNames[targetId] || targetId.slice(0, 8)}
                          </Link>
                        </div>
                        <button
                          onClick={() => handleDeleteRelationship(r.id)}
                          className="text-[var(--text-tertiary)] hover:text-red-500 text-[11px] opacity-0 group-hover:opacity-100 transition-opacity"
                          title="Remove relationship"
                        >✕</button>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

          {/* Scorecards */}
          <div className="card">
            <div className="card-header flex items-center justify-between">
              <div className="flex items-center gap-2">
                <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Scorecards</h2>
                <span className="text-[12px] text-[var(--text-tertiary)]">{scores.length}</span>
              </div>
            </div>
            <div className="card-body">
              {scores.length === 0 ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">
                  No evaluations yet
                  <button onClick={handleEvaluate} disabled={evaluating} className="block mt-1 text-[var(--accent)] hover:underline text-[12px]">
                    Run evaluation
                  </button>
                </p>
              ) : (
                <div className="space-y-3">
                  {scores.map((sc) => {
                    const pct = sc.max_score > 0 ? Math.round((sc.score / sc.max_score) * 100) : 0;
                    return (
                      <div key={sc.id}>
                        <div className="flex items-center justify-between mb-1">
                          <span className={`badge ${levelBadge(sc.level)}`}>{sc.level}</span>
                          <span className="text-[12px] text-[var(--text-secondary)]">{sc.pass_count}/{sc.total_rules} passed</span>
                        </div>
                        <div className="progress-bar">
                          <div
                            className={`progress-bar-fill ${pct >= 75 ? 'bg-emerald-500' : pct >= 50 ? 'bg-amber-500' : 'bg-red-500'}`}
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                        <div className="text-[11px] text-[var(--text-tertiary)] mt-0.5 text-right">{pct}%</div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Graph Stats */}
        {(nodes.length > 0 || edges.length > 0) && (
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Graph</h2>
            </div>
            <div className="card-body flex items-center gap-6">
              <div>
                <span className="text-[20px] font-semibold text-[var(--text-primary)]">{nodes.length}</span>
                <span className="text-[12px] text-[var(--text-tertiary)] ml-2">nodes</span>
              </div>
              <div>
                <span className="text-[20px] font-semibold text-[var(--text-primary)]">{edges.length}</span>
                <span className="text-[12px] text-[var(--text-tertiary)] ml-2">edges</span>
              </div>
              <div className="text-[11px] text-[var(--text-tertiary)] ml-auto">
                Graph visualization coming soon
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function levelBadge(level: string) {
  switch (level) {
    case 'platinum': return 'badge-accent';
    case 'gold': return 'badge-warning';
    case 'silver': return 'badge-default';
    case 'bronze': return 'badge-info';
    default: return 'badge-danger';
  }
}
