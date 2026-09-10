'use client';
import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import { environments, type EnvironmentOverviewResponse, type EnvironmentOverviewCell, type EnvironmentProblem } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';

export default function EnvironmentOverviewPage() {
  return (
    <PermissionGuard resource="environments" action="read">
      <EnvironmentOverviewContent />
    </PermissionGuard>
  );
}

function EnvironmentOverviewContent() {
  const [data, setData] = useState<EnvironmentOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedProblem, setSelectedProblem] = useState<EnvironmentProblem | null>(null);
  const [filterSeverity, setFilterSeverity] = useState<string>('');

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      const overview = await environments.overview();
      setData(overview);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load overview');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const filteredProblems = data?.problems.filter(p => !filterSeverity || p.severity === filterSeverity) || [];

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6">
          <div className="flex items-center justify-center py-12">
            <div className="loading-spinner" />
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6">
          <div className="card card-body text-center py-12">
            <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">Failed to load overview</h3>
            <p className="text-[12px] text-[var(--text-tertiary)]">{error}</p>
            <button onClick={fetchData} className="btn btn-primary btn-sm mt-4">Retry</button>
          </div>
        </div>
      </div>
    );
  }

  const summary = data?.summary;
  const envs = data?.environments || [];
  const services = data?.services || [];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Environment Overview</h1>
            <p className="page-subtitle-modern">
              Unified view of all services across environments
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Link href="/environments" className="btn btn-secondary btn-sm">
              All Environments
            </Link>
            <button onClick={fetchData} className="btn btn-secondary btn-sm">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Refresh
            </button>
          </div>
        </div>

        {/* Summary Cards */}
        {summary && (
          <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
            <SummaryCard
              title="Total Services"
              value={summary.total_services}
              color="var(--accent)"
            />
            <SummaryCard
              title="Healthy"
              value={summary.healthy_services}
              color="var(--success)"
            />
            <SummaryCard
              title="Degraded"
              value={summary.degraded_services}
              color="var(--warning)"
            />
            <SummaryCard
              title="Drifts"
              value={summary.total_drifts}
              color="var(--info)"
            />
            <SummaryCard
              title="Failed"
              value={summary.failed_deployments}
              color="var(--danger)"
            />
          </div>
        )}

        {/* Problems Panel */}
        {filteredProblems.length > 0 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header flex items-center justify-between">
              <div className="flex items-center gap-2">
                <svg className="w-4 h-4 text-[var(--warning)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
                <h3 className="text-[14px] font-medium text-[var(--text-primary)]">
                  Problems ({filteredProblems.length})
                </h3>
              </div>
              <div className="flex items-center gap-2">
                <select
                  value={filterSeverity}
                  onChange={e => setFilterSeverity(e.target.value)}
                  className="input text-[12px] py-1 px-2"
                >
                  <option value="">All Severities</option>
                  <option value="critical">Critical</option>
                  <option value="warning">Warning</option>
                  <option value="info">Info</option>
                </select>
              </div>
            </div>
            <div className="card-body pt-0">
              <div className="space-y-2 max-h-[200px] overflow-y-auto">
                {filteredProblems.slice(0, 10).map(problem => (
                  <div
                    key={problem.id}
                    onClick={() => setSelectedProblem(problem)}
                    className="flex items-center gap-3 p-2 rounded-lg hover:bg-[var(--bg)] cursor-pointer transition-colors"
                  >
                    <span className={`w-2 h-2 rounded-full shrink-0 ${
                      problem.severity === 'critical' ? 'bg-[var(--danger)]' :
                      problem.severity === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--info)]'
                    }`} />
                    <div className="flex-1 min-w-0">
                      <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">
                        {problem.service_name}
                      </p>
                      <p className="text-[11px] text-[var(--text-tertiary)] truncate">
                        {problem.message} — {problem.env_name}
                      </p>
                    </div>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${
                      problem.type === 'failed_deploy' ? 'bg-[var(--danger)]/10 text-[var(--danger)]' :
                      problem.type === 'drift' ? 'bg-[var(--info)]/10 text-[var(--info)]' :
                      'bg-[var(--warning)]/10 text-[var(--warning)]'
                    }`}>
                      {problem.type.replace('_', ' ')}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Matrix View */}
        {services.length > 0 && envs.length > 0 ? (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Services × Environments</h3>
            </div>
            <div className="card-body pt-0 overflow-x-auto">
              <table className="w-full text-[12px]">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="text-left py-2 px-3 font-medium text-[var(--text-secondary)] sticky left-0 bg-[var(--surface)]">Service</th>
                    {envs.map(env => (
                      <th key={env.id} className="text-center py-2 px-3 font-medium">
                        <Link href={`/environments?id=${env.id}`} className="hover:text-[var(--accent)] transition-colors">
                          <span className="flex items-center justify-center gap-1.5">
                            <span className="w-2 h-2 rounded-full" style={{ backgroundColor: env.color || '#6B7280' }} />
                            <span className="text-[var(--text-primary)]">{env.name}</span>
                          </span>
                        </Link>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {services.map(service => (
                    <tr key={service.service_id} className="border-b border-[var(--border-light)] hover:bg-[var(--bg)] transition-colors">
                      <td className="py-2.5 px-3 sticky left-0 bg-[var(--surface)]">
                        <Link href={`/services?service=${service.service_slug}`} className="flex items-center gap-2 hover:text-[var(--accent)] transition-colors">
                          <span className="font-medium text-[var(--text-primary)]">{service.service_name}</span>
                        </Link>
                      </td>
                      {envs.map(env => {
                        const cell = env.slug ? service.environments[env.slug] : undefined;
                        return (
                          <td key={env.id} className="py-2.5 px-3 text-center">
                            <MatrixCell cell={cell} />
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <div className="text-4xl mb-3 opacity-20">🌍</div>
            <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">No services deployed</h3>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              Deploy services to environments to see them here
            </p>
            <Link href="/services/new" className="btn btn-primary btn-sm mt-4">
              Deploy Service
            </Link>
          </div>
        )}

        {/* Legend */}
        <div className="flex items-center gap-4 text-[11px] text-[var(--text-tertiary)]">
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-full bg-[var(--success)]" />
            Deployed & Healthy
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-full bg-[var(--warning)]" />
            Deploying / Degraded
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-full bg-[var(--danger)]" />
            Failed
          </span>
          <span className="flex items-center gap-1.5">
            <span className="w-3 h-3 rounded-full bg-[var(--text-tertiary)] opacity-30" />
            Not Deployed
          </span>
          <span className="flex items-center gap-1.5">
            <svg className="w-3 h-3 text-[var(--info)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Drift Detected
          </span>
        </div>
      </div>

      {/* Problem Detail Modal */}
      {selectedProblem && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={() => setSelectedProblem(null)}>
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
          <div className="relative w-full max-w-[480px] bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] p-6" onClick={e => e.stopPropagation()}>
            <h2 className="text-[16px] font-semibold text-[var(--text-primary)] mb-4">
              Problem Details
            </h2>
            <div className="space-y-3">
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Service</label>
                <p className="text-[13px] text-[var(--text-primary)]">{selectedProblem.service_name}</p>
              </div>
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Environment</label>
                <p className="text-[13px] text-[var(--text-primary)]">{selectedProblem.env_name}</p>
              </div>
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Type</label>
                <p className="text-[13px] text-[var(--text-primary)] capitalize">{selectedProblem.type.replace('_', ' ')}</p>
              </div>
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Severity</label>
                <span className={`text-[11px] px-2 py-0.5 rounded-full ${
                  selectedProblem.severity === 'critical' ? 'bg-[var(--danger)]/10 text-[var(--danger)]' :
                  selectedProblem.severity === 'warning' ? 'bg-[var(--warning)]/10 text-[var(--warning)]' :
                  'bg-[var(--info)]/10 text-[var(--info)]'
                }`}>
                  {selectedProblem.severity}
                </span>
              </div>
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Message</label>
                <p className="text-[13px] text-[var(--text-primary)]">{selectedProblem.message}</p>
              </div>
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase">Details</label>
                <p className="text-[12px] text-[var(--text-secondary)]">{selectedProblem.details}</p>
              </div>
            </div>
            <div className="flex justify-end pt-4 mt-4 border-t border-[var(--border)]">
              <button onClick={() => setSelectedProblem(null)} className="btn btn-secondary btn-sm">
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function SummaryCard({ title, value, color }: { title: string; value: number; color: string }) {
  return (
    <div className="card" style={{ borderRadius: '12px' }}>
      <div className="card-body p-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ backgroundColor: `${color}15` }}>
            <span className="text-[16px] font-bold" style={{ color }}>{value}</span>
          </div>
          <span className="text-[12px] text-[var(--text-secondary)]">{title}</span>
        </div>
      </div>
    </div>
  );
}

function MatrixCell({ cell }: { cell: EnvironmentOverviewCell | undefined }) {
  if (!cell) {
    return (
      <div className="w-8 h-8 mx-auto rounded-full bg-[var(--text-tertiary)] opacity-20" title="Not deployed" />
    );
  }

  const statusColor = cell.status === 'deployed' && cell.health_status === 'healthy'
    ? 'var(--success)'
    : cell.status === 'deploying'
    ? 'var(--warning)'
    : cell.status === 'failed'
    ? 'var(--danger)'
    : 'var(--text-tertiary)';

  const statusIcon = cell.status === 'deployed' ? '✓' : cell.status === 'deploying' ? '⟳' : cell.status === 'failed' ? '✗' : '—';

  return (
    <div className="relative inline-flex items-center justify-center">
      <div
        className="w-8 h-8 rounded-full flex items-center justify-center text-[11px] font-medium text-white"
        style={{ backgroundColor: statusColor }}
        title={`${cell.status}${cell.image_tag ? ` (${cell.image_tag})` : ''}${cell.drift_count > 0 ? ` — ${cell.drift_count} drift(s)` : ''}`}
      >
        {statusIcon}
      </div>
      {cell.drift_count > 0 && (
        <div className="absolute -top-1 -right-1 w-4 h-4 rounded-full bg-[var(--info)] text-white text-[9px] flex items-center justify-center" title={`${cell.drift_count} drift(s) detected`}>
          {cell.drift_count}
        </div>
      )}
      {cell.engine_type && (
        <div className="absolute -bottom-1 -right-1 w-3.5 h-3.5 rounded-full bg-[var(--surface)] border border-[var(--border)] flex items-center justify-center" title={`Managed by ${cell.engine_type}`}>
          <span className="text-[8px]">{cell.engine_type === 'argocd' ? 'A' : 'F'}</span>
        </div>
      )}
    </div>
  );
}
