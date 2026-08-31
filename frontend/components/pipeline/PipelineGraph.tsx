'use client';

import { useMemo } from 'react';
import type { WorkflowGraph, WorkflowGraphJob } from '@/lib/api';
import type { PipelineRunJob } from '@/lib/api';

interface PipelineGraphProps {
  graph: WorkflowGraph;
  runJobs?: PipelineRunJob[];
  onJobClick?: (jobName: string) => void;
  compact?: boolean;
}

// Map job status to color classes
function statusColor(status?: string): string {
  switch (status?.toLowerCase()) {
    case 'success':
    case 'completed':
      return 'bg-emerald-500/15 text-emerald-600 border-emerald-500/30';
    case 'failure':
    case 'failed':
      return 'bg-red-500/15 text-red-500 border-red-500/30';
    case 'running':
    case 'in_progress':
      return 'bg-blue-500/15 text-blue-500 border-blue-500/30';
    case 'pending':
    case 'queued':
      return 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]';
    case 'skipped':
    case 'cancelled':
    case 'canceled':
      return 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)] opacity-60';
    default:
      return 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]';
  }
}

function statusIcon(status?: string): string {
  switch (status?.toLowerCase()) {
    case 'success':
    case 'completed':
      return '\u2713';
    case 'failure':
    case 'failed':
      return '\u2717';
    case 'running':
    case 'in_progress':
      return '\u25CF';
    case 'skipped':
    case 'cancelled':
    case 'canceled':
      return '\u2014';
    default:
      return '\u25CB';
  }
}

// Build a lookup from job name to run job status
function buildJobStatusMap(runJobs?: PipelineRunJob[]): Map<string, PipelineRunJob> {
  const map = new Map<string, PipelineRunJob>();
  if (!runJobs) return map;
  for (const job of runJobs) {
    map.set(job.name, job);
  }
  return map;
}

// Group jobs by stage and order stages
function groupByStage(jobs: WorkflowGraphJob[], stages: { name: string; order: number }[]): { stage: string; jobs: WorkflowGraphJob[] }[] {
  const stageOrder = new Map(stages.map(s => [s.name, s.order]));
  const groups = new Map<string, WorkflowGraphJob[]>();

  for (const job of jobs) {
    const stage = job.stage || 'default';
    if (!groups.has(stage)) {
      groups.set(stage, []);
    }
    groups.get(stage)!.push(job);
  }

  const result = Array.from(groups.entries()).map(([stage, jobs]) => ({
    stage,
    jobs: [...jobs].sort((a, b) => a.name.localeCompare(b.name)),
  }));

  result.sort((a, b) => {
    const oa = stageOrder.get(a.stage) ?? 99;
    const ob = stageOrder.get(b.stage) ?? 99;
    return oa - ob;
  });

  return result;
}

// Check if stage is a numeric level (from GitHub Actions topological sorting)
function isNumericStage(stage: string): boolean {
  return /^\d+$/.test(stage);
}

export default function PipelineGraph({ graph, runJobs, onJobClick, compact = false }: PipelineGraphProps) {
  const jobStatusMap = useMemo(() => buildJobStatusMap(runJobs), [runJobs]);
  const stageGroups = useMemo(() => groupByStage(graph.jobs || [], graph.stages || []), [graph.jobs, graph.stages]);

  if (!graph.jobs || graph.jobs.length === 0) {
    return (
      <div className="text-center py-8">
        <p className="text-sm text-[var(--text-secondary)]">No jobs found in workflow</p>
      </div>
    );
  }

  const nodeSize = compact ? 'min-w-[120px]' : 'min-w-[160px]';
  const textSize = compact ? 'text-[10px]' : 'text-xs';

  return (
    <div className="overflow-x-auto">
      <div className="flex items-start gap-2 min-w-min py-4 px-2">
        {stageGroups.map((group, gi) => (
          <div key={group.stage} className="flex items-start gap-2">
            {/* Stage column */}
            <div className="flex flex-col gap-2">
              {/* Stage header - show meaningful name or just the level */}
              <div className={`text-center ${textSize} font-semibold text-[var(--text-secondary)] uppercase tracking-wider px-2 py-1`}>
                {isNumericStage(group.stage) ? `Level ${parseInt(group.stage) + 1}` : group.stage}
              </div>
              {/* Job nodes */}
              {group.jobs.map(job => {
                const runJob = jobStatusMap.get(job.name);
                const status = runJob?.status;
                const duration = runJob?.duration_ms;
                return (
                  <div
                    key={job.name}
                    className={`${nodeSize} rounded-lg border p-2 cursor-pointer transition-all hover:scale-[1.02] hover:shadow-sm ${statusColor(status)}`}
                    onClick={() => onJobClick?.(job.name)}
                    title={job.if ? `Condition: ${job.if}` : job.name}
                  >
                    <div className="flex items-center gap-1.5">
                      <span className={`text-sm ${status ? '' : 'opacity-40'}`}>
                        {statusIcon(status)}
                      </span>
                      <span className={`${textSize} font-medium truncate`}>{job.name}</span>
                    </div>
                    {(duration != null || job.runs_on) && !compact && (
                      <div className={`mt-1 ${textSize} opacity-70 flex items-center gap-2`}>
                        {duration != null && (
                          <span>{(duration / 1000).toFixed(1)}s</span>
                        )}
                        {job.runs_on && (
                          <span className="truncate">{job.runs_on}</span>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>

            {/* Arrow to next stage */}
            {gi < stageGroups.length - 1 && (
              <div className="flex items-center self-center pt-5">
                <svg width="24" height="12" viewBox="0 0 24 12" className="text-[var(--border)]">
                  <line x1="0" y1="6" x2="18" y2="6" stroke="currentColor" strokeWidth="1.5" />
                  <polygon points="18,2 24,6 18,10" fill="currentColor" />
                </svg>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

// Mini inline graph for run expansion
export function MiniPipelineGraph({ graph, runJobs }: { graph: WorkflowGraph; runJobs?: PipelineRunJob[] }) {
  return <PipelineGraph graph={graph} runJobs={runJobs} compact />;
}
