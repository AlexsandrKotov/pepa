'use client';

import { useState, useEffect } from 'react';
import { deployments, type Deployment } from '@/lib/api';
import Link from 'next/link';
import BrandIcon from '@/components/BrandIcon';

const STATUS_COLORS: Record<string, string> = {
  deployed: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  running: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  error: 'bg-red-500/10 text-red-500 border-red-500/20',
  pending: 'bg-amber-500/10 text-amber-600 border-amber-500/20',
  progressing: 'bg-blue-500/10 text-blue-600 border-blue-500/20',
  syncing: 'bg-blue-500/10 text-blue-600 border-blue-500/20',
  rolled_back: 'bg-violet-500/10 text-violet-500 border-violet-500/20',
  cancelled: 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]',
  promoted: 'bg-cyan-500/10 text-cyan-600 border-cyan-500/20',
};

const STRATEGY_COLORS: Record<string, string> = {
  rolling: 'bg-blue-500/10 text-blue-600',
  recreate: 'bg-amber-500/10 text-amber-600',
  blue_green: 'bg-emerald-500/10 text-emerald-600',
  canary: 'bg-violet-500/10 text-violet-500',
};

export default function DeploymentTimelinePage() {
  const [deploymentsList, setDeploymentsList] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [teamFilter, setTeamFilter] = useState('');
  const [limit, setLimit] = useState(50);

  useEffect(() => {
    loadDeployments();
  }, [statusFilter, teamFilter, limit]);

  const loadDeployments = async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { per_page: String(limit) };
      if (statusFilter) params.status = statusFilter;
      if (teamFilter) params.team = teamFilter;
      const data = await deployments.list(params);
      setDeploymentsList(data.deployments || []);
    } catch {
      setDeploymentsList([]);
    }
    setLoading(false);
  };

  // Get unique teams for filter
  const teams = Array.from(new Set(deploymentsList.map(d => d.team_name).filter(Boolean))).sort();

  // Group deployments by date
  const groupedByDate: Record<string, Deployment[]> = {};
  for (const d of deploymentsList) {
    const date = new Date(d.created_at).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
    if (!groupedByDate[date]) groupedByDate[date] = [];
    groupedByDate[date].push(d);
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="page-title-modern">Deployment Timeline</h1>
              <p className="page-subtitle-modern">All deployments across services, teams, and environments</p>
            </div>
            <Link href="/deployments" className="btn btn-secondary text-sm">
              Standard View
            </Link>
          </div>
        </div>

        {/* Filters */}
        <div className="page-animate-up flex flex-wrap gap-3">
          <select
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value)}
            className="px-3 py-1.5 text-[13px] bg-[var(--surface)] border border-[var(--border)] rounded-lg text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20"
          >
            <option value="">All statuses</option>
            <option value="deployed">Deployed</option>
            <option value="failed">Failed</option>
            <option value="pending">Pending</option>
            <option value="progressing">Progressing</option>
            <option value="rolled_back">Rolled Back</option>
            <option value="cancelled">Cancelled</option>
          </select>
          <select
            value={teamFilter}
            onChange={e => setTeamFilter(e.target.value)}
            className="px-3 py-1.5 text-[13px] bg-[var(--surface)] border border-[var(--border)] rounded-lg text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20"
          >
            <option value="">All teams</option>
            {teams.map(t => <option key={t} value={t}>{t}</option>)}
          </select>
          <select
            value={limit}
            onChange={e => setLimit(Number(e.target.value))}
            className="px-3 py-1.5 text-[13px] bg-[var(--surface)] border border-[var(--border)] rounded-lg text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20"
          >
            <option value={25}>Last 25</option>
            <option value={50}>Last 50</option>
            <option value={100}>Last 100</option>
          </select>
        </div>

        {/* Timeline */}
        {loading ? (
          <div className="card card-body text-center py-12">
            <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
              <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <p className="text-[13px]">Loading deployments...</p>
            </div>
          </div>
        ) : deploymentsList.length === 0 ? (
          <div className="card text-center py-16">
            <BrandIcon name="argocd" size={48} style={{ opacity: 0.2, margin: '0 auto 16px' }} />
            <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-2">No deployments found</h3>
            <p className="text-[var(--text-secondary)]">Try adjusting your filters</p>
          </div>
        ) : (
          <div className="space-y-6">
            {Object.entries(groupedByDate).map(([date, deps]) => (
              <div key={date} className="page-animate-up">
                <div className="flex items-center gap-3 mb-3">
                  <h2 className="text-sm font-semibold text-[var(--text-primary)]">{date}</h2>
                  <span className="text-xs text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded-full">{deps.length} deployments</span>
                </div>
                <div className="relative pl-6 border-l-2 border-[var(--border-light)] space-y-3">
                  {deps.map(dep => (
                    <div key={dep.id} className="relative">
                      {/* Timeline dot */}
                      <div className={`absolute -left-[25px] top-3 w-3 h-3 rounded-full border-2 ${
                        dep.status === 'deployed' || dep.status === 'running' ? 'bg-emerald-500 border-emerald-500/30' :
                        dep.status === 'failed' || dep.status === 'error' ? 'bg-red-500 border-red-500/30' :
                        dep.status === 'rolled_back' ? 'bg-violet-500 border-violet-500/30' :
                        'bg-[var(--text-tertiary)] border-[var(--border)]'
                      }`} />
                      <Link
                        href={`/deployments`}
                        className="card block p-4 hover:border-[var(--text-tertiary)] transition-colors"
                        style={{ borderRadius: '12px' }}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <h3 className="text-[14px] font-semibold text-[var(--text-primary)] truncate">
                                {dep.jira_summary || dep.jira_issue_key || `Deployment ${dep.id.slice(0, 8)}`}
                              </h3>
                              <span className={`text-[10px] px-1.5 py-0.5 rounded-full border ${STATUS_COLORS[dep.status] || STATUS_COLORS.pending}`}>
                                {dep.status}
                              </span>
                              {dep.strategy && (
                                <span className={`text-[10px] px-1.5 py-0.5 rounded ${STRATEGY_COLORS[dep.strategy] || 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
                                  {dep.strategy.replace('_', '-')}
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-3 mt-1.5 text-[11px] text-[var(--text-tertiary)]">
                              {dep.team_name && <span>Team: {dep.team_name}</span>}
                              {dep.target_namespace && <span>NS: {dep.target_namespace}</span>}
                              {dep.image_tag && <span>Tag: {dep.image_tag}</span>}
                              {dep.stage && <span className="px-1.5 py-0.5 bg-[var(--border-light)] rounded">{dep.stage}</span>}
                            </div>
                          </div>
                          <div className="text-right text-[11px] text-[var(--text-tertiary)] shrink-0">
                            <div>{new Date(dep.created_at).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })}</div>
                            {dep.created_by && <div>by {dep.created_by}</div>}
                          </div>
                        </div>
                      </Link>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
