'use client';

import { type GitopsResource } from '@/lib/api';

interface ServiceCardProps {
  resource: GitopsResource;
  selected?: boolean;
  showCluster?: boolean;
  onSelect?: (resource: GitopsResource) => void;
  onSuspend?: (resource: GitopsResource) => void;
  onResume?: (resource: GitopsResource) => void;
  onEdit?: (resource: GitopsResource) => void;
}

function kindIcon(kind: string) {
  switch (kind) {
    case 'HelmRelease': return '📦';
    case 'Kustomization': return '🔧';
    case 'Application': return '🚀';
    default: return '📄';
  }
}

function statusColor(suspended?: boolean) {
  return suspended ? 'bg-[var(--border-light)] text-[var(--text-tertiary)]' : 'bg-emerald-500/10 text-emerald-600';
}

function statusLabel(suspended?: boolean) {
  return suspended ? 'Suspended' : 'Active';
}

// Generate consistent color from cluster name
function clusterColor(cluster: string) {
  const colors = [
    'bg-blue-500/10 text-blue-500',
    'bg-violet-500/10 text-violet-500',
    'bg-orange-500/10 text-orange-500',
    'bg-pink-500/10 text-pink-500',
    'bg-indigo-500/10 text-indigo-500',
    'bg-teal-500/10 text-teal-500',
  ];
  let hash = 0;
  for (let i = 0; i < cluster.length; i++) {
    hash = cluster.charCodeAt(i) + ((hash << 5) - hash);
  }
  return colors[Math.abs(hash) % colors.length];
}

export default function ServiceCard({
  resource,
  selected,
  showCluster,
  onSelect,
  onSuspend,
  onResume,
  onEdit,
}: ServiceCardProps) {
  return (
    <div
      onClick={() => onSelect?.(resource)}
      className={`card p-4 hover:border-[var(--accent)] transition-all cursor-pointer group ${
        selected ? 'border-[var(--accent)] ring-1 ring-[var(--accent)]' : ''
      } ${resource.suspended ? 'opacity-60' : ''}`}
    >
      {/* Header */}
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="text-xl">{kindIcon(resource.kind)}</span>
          <div>
            <h3 className="text-[13px] font-semibold text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              {resource.name}
            </h3>
            {resource.namespace && (
              <span className="text-[10px] text-[var(--text-tertiary)]">ns: {resource.namespace}</span>
            )}
          </div>
        </div>
        <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(resource.suspended)}`}>
          {statusLabel(resource.suspended)}
        </span>
      </div>

      {/* Details */}
      <div className="space-y-1.5 mb-3">
        {resource.chart && (
          <div className="flex items-center gap-2 text-[11px]">
            <span className="text-[var(--text-tertiary)] w-12">Chart:</span>
            <span className="font-mono text-[var(--text-secondary)] truncate">
              {resource.chart}{resource.version ? `@${resource.version}` : ''}
            </span>
          </div>
        )}
        {resource.repo && (
          <div className="flex items-center gap-2 text-[11px]">
            <span className="text-[var(--text-tertiary)] w-12">Source:</span>
            <span className="font-mono text-[var(--text-secondary)] truncate">{resource.repo}</span>
          </div>
        )}
        <div className="flex items-center gap-2 text-[11px]">
          <span className="text-[var(--text-tertiary)] w-12">File:</span>
          <span className="font-mono text-[var(--text-tertiary)] truncate text-[10px]">{resource.file_path}</span>
        </div>
      </div>

      {/* Cluster Badge */}
      {showCluster && resource.cluster && (
        <div className="mb-3">
          <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${clusterColor(resource.cluster)}`}>
            {resource.cluster}
          </span>
        </div>
      )}

      {/* Dependencies */}
      {resource.depends_on && resource.depends_on.length > 0 && (
        <div className="mb-3">
          <span className="text-[10px] text-[var(--text-tertiary)]">
            {resource.depends_on.length} dependencies
          </span>
        </div>
      )}

      {/* Actions */}
      <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]" onClick={e => e.stopPropagation()}>
        {resource.suspended ? (
          <button
            onClick={() => onResume?.(resource)}
            className="text-[11px] px-2.5 py-1 text-green-600 hover:bg-emerald-500/10 rounded-lg transition-colors"
          >
            Resume
          </button>
        ) : (
          <button
            onClick={() => onSuspend?.(resource)}
            className="text-[11px] px-2.5 py-1 text-orange-600 hover:bg-orange-500/10 rounded-lg transition-colors"
          >
            Suspend
          </button>
        )}
        <button
          onClick={() => onEdit?.(resource)}
          className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors"
        >
          Edit YAML
        </button>
      </div>
    </div>
  );
}
