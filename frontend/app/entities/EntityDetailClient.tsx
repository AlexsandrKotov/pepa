'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { entities, scorecards, type Entity } from '@/lib/api';
import { EntityEditButton } from '@/components/Interactive';

interface Props {
  entityId: string;
}

export default function EntityDetailClient({ entityId }: Props) {
  const [entity, setEntity] = useState<Entity | null>(null);
  const [relationships, setRelationships] = useState<Array<{ id: string; source_id: string; target_id: string; type_key: string }>>([]);
  const [nodes, setNodes] = useState<Array<{ id: string; name: string; type: string }>>([]);
  const [edges, setEdges] = useState<Array<{ id: string; source: string; target: string }>>([]);
  const [scores, setScores] = useState<Array<{ id: string; level: string; score: number; max_score: number; pass_count: number; total_rules: number }>>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      entities.get(entityId).catch(() => null),
      entities.relationships(entityId).catch(() => ({ relationships: [] })),
      entities.graph(entityId, 2).catch(() => ({ nodes: [], edges: [] })),
      scorecards.entityScores(entityId).catch(() => ({ scores: [], total: 0 })),
    ]).then(([entityData, relsData, graphData, scoresData]) => {
      setEntity(entityData);
      setRelationships(relsData.relationships || []);
      setNodes((graphData.nodes || []) as Array<{ id: string; name: string; type: string }>);
      setEdges((graphData.edges || []) as Array<{ id: string; source: string; target: string }>);
      setScores(scoresData.scores || []);
    }).finally(() => setLoading(false));
  }, [entityId]);

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

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="card page-animate" style={{ borderRadius: '12px' }}>
        <div className="card-body">
          <div className="flex items-start justify-between">
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded bg-[var(--border-light)] flex items-center justify-center text-[14px] font-medium text-[var(--text-secondary)]">
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
              <EntityEditButton entity={entity} />
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
                  <div key={key} className="flex justify-between items-start">
                    <dt className="text-[12px] text-[var(--text-tertiary)]">{key}</dt>
                    <dd className="text-[13px] text-[var(--text-primary)] font-medium">{String(value)}</dd>
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
          <div className="card-header">
            <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Relationships</h2>
            <span className="text-[12px] text-[var(--text-tertiary)]">{relationships.length}</span>
          </div>
          <div className="card-body">
            {relationships.length === 0 ? (
              <p className="text-[13px] text-[var(--text-tertiary)]">No relationships</p>
            ) : (
              <div className="space-y-2">
                {relationships.map((r) => {
                  const targetId = r.target_id === entity.id ? r.source_id : r.target_id;
                  return (
                    <Link
                      key={r.id}
                      href={`/entities?id=${targetId}`}
                      className="flex items-center justify-between p-2 rounded hover:bg-[var(--bg)] transition-colors"
                    >
                      <span className="badge badge-default">{r.type_key}</span>
                      <span className="text-mono text-[var(--text-tertiary)]">{targetId.slice(0, 8)}</span>
                    </Link>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Scorecards */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Scorecards</h2>
            <span className="text-[12px] text-[var(--text-tertiary)]">{scores.length}</span>
          </div>
          <div className="card-body">
            {scores.length === 0 ? (
              <p className="text-[13px] text-[var(--text-tertiary)]">No evaluations</p>
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
