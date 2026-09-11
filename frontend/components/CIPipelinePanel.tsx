'use client';

import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import { pipelineSources, pipelineRuns, type PipelineSource, type PipelineRun } from '@/lib/api';

interface CIPipelinePanelProps {
  /** Filter runs to a specific pipeline source ID. If omitted, shows all sources. */
  sourceId?: string;
  /** Maximum number of runs to show */
  limit?: number;
  /** Compact mode for embedding in detail pages */
  compact?: boolean;
}

function statusColor(status: string) {
  switch (status) {
    case 'success':
    case 'completed':
    case 'merged':
      return 'bg-emerald-500';
    case 'running':
    case 'pending':
    case 'created':
      return 'bg-blue-500 animate-pulse';
    case 'failed':
    case 'error':
      return 'bg-red-500';
    case 'canceled':
    case 'skipped':
      return 'bg-[var(--text-tertiary)]';
    default:
      return 'bg-[var(--text-tertiary)]';
  }
}

function statusBadge(status: string) {
  const styles: Record<string, string> = {
    success: 'bg-emerald-500/15 text-emerald-600',
    completed: 'bg-emerald-500/15 text-emerald-600',
    merged: 'bg-emerald-500/15 text-emerald-600',
    running: 'bg-blue-500/15 text-blue-500',
    pending: 'bg-blue-500/15 text-blue-500',
    created: 'bg-blue-500/15 text-blue-500',
    failed: 'bg-red-500/15 text-red-500',
    error: 'bg-red-500/15 text-red-500',
    canceled: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
    skipped: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  return styles[status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]';
}

function formatDuration(ms?: number) {
  if (!ms) return '';
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remaining}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}

function timeAgo(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffH = Math.floor(diffMin / 60);
  if (diffH < 24) return `${diffH}h ago`;
  const diffD = Math.floor(diffH / 24);
  return `${diffD}d ago`;
}

export default function CIPipelinePanel({ sourceId, limit = 10, compact = false }: CIPipelinePanelProps) {
  const [sources, setSources] = useState<PipelineSource[]>([]);
  const [runs, setRuns] = useState<Map<string, PipelineRun[]>>(new Map());
  const [loading, setLoading] = useState(true);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      let srcs: PipelineSource[];
      if (sourceId) {
        const src = await pipelineSources.get(sourceId).catch(() => null);
        srcs = src ? [src] : [];
      } else {
        const result = await pipelineSources.list({ per_page: '50' }).catch(() => ({ sources: [], total: 0 }));
        srcs = result.sources || [];
      }
      setSources(srcs);

      const runMap = new Map<string, PipelineRun[]>();
      await Promise.all(
        srcs.map(async src => {
          const result = await pipelineRuns.list(src.id, { per_page: String(limit) }).catch(() => ({ runs: [], total: 0 }));
          runMap.set(src.id, result.runs || []);
        })
      );
      setRuns(runMap);
    } catch {
      // Silently fail — panel shows empty state
    } finally {
      setLoading(false);
    }
  }, [sourceId, limit]);

  useEffect(() => { loadData(); }, [loadData]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-6">
        <div className="loading-spinner" />
      </div>
    );
  }

  if (sources.length === 0) {
    return (
      <div className="text-center py-6">
        <p className="text-[12px] text-[var(--text-tertiary)]">No pipeline sources configured.</p>
        <Link href="/pipelines" className="text-[11px] text-[var(--accent)] hover:underline mt-1">
          Set up a pipeline source
        </Link>
      </div>
    );
  }

  const allRuns = Array.from(runs.values()).flat().sort((a, b) =>
    new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  ).slice(0, limit);

  return (
    <div className="space-y-2">
      {allRuns.length === 0 ? (
        <div className="text-center py-4">
          <p className="text-[12px] text-[var(--text-tertiary)]">No pipeline runs yet.</p>
        </div>
      ) : (
        allRuns.map(run => {
          const source = sources.find(s => s.id === run.source_id);
          return (
            <div
              key={run.id}
              className={`flex items-center gap-3 ${compact ? 'p-2' : 'p-3'} rounded-lg border border-[var(--border-light)] hover:bg-[var(--bg)] transition-colors`}
            >
              <span className={`w-2 h-2 rounded-full shrink-0 ${statusColor(run.status)}`} />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">
                    {source?.name || 'Pipeline'}
                  </p>
                  {run.trigger_type && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-tertiary)]">
                      {run.trigger_type}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2 mt-0.5">
                  <span className="text-[10px] text-[var(--text-tertiary)]">{timeAgo(run.created_at)}</span>
                  {(run.duration_ms ?? 0) > 0 && (
                    <span className="text-[10px] text-[var(--text-tertiary)]">{formatDuration(run.duration_ms)}</span>
                  )}
                  {run.triggered_by && (
                    <span className="text-[10px] text-[var(--text-tertiary)]">by {run.triggered_by}</span>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${statusBadge(run.status)}`}>
                  {run.status}
                </span>
                {run.external_url && (
                  <a
                    href={run.external_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[10px] text-[var(--accent)] hover:underline"
                  >
                    View
                  </a>
                )}
              </div>
            </div>
          );
        })
      )}
    </div>
  );
}
