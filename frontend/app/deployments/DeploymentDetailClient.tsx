'use client';

import { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { deployments, clusters, type Deployment, type Cluster } from '@/lib/api';

interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
}

export default function DeploymentDetailPage() {
  const searchParams = useSearchParams();
  const id = searchParams.get('id') as string;

  const [deployment, setDeployment] = useState<Deployment | null>(null);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [history, setHistory] = useState<Deployment[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'timeline' | 'logs' | 'history'>('timeline');

  const loadData = useCallback(async () => {
    try {
      const [dep, hist, logData, clusterData] = await Promise.all([
        deployments.get(id),
        deployments.history(id).catch(() => ({ history: [], total: 0 })),
        deployments.logs(id).catch(() => ({ logs: [], deployment_id: id })),
        clusters.list().catch(() => ({ clusters: [], total: 0 })),
      ]);
      setDeployment(dep);
      setHistory(hist.history || []);
      setLogs(logData.logs || []);
      setClusterList(clusterData.clusters || []);
    } catch (err) {
      console.error('Failed to load deployment:', err);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [loadData]);

  const handlePromote = async () => {
    try { await deployments.promote(id); await loadData(); } catch { /* ignore */ }
  };

  const handleRollback = async () => {
    if (!confirm('Rollback this deployment?')) return;
    try { await deployments.rollback(id); await loadData(); } catch { /* ignore */ }
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Deployment Details</h1>
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      </div></div>
    );
  }

  if (!deployment) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Deployment Not Found</h1>
        <Link href="/deployments" className="text-[var(--accent)] hover:underline">{'\u2190'} Back to Deployments</Link>
      </div></div>
    );
  }

  const statusColor = (s: string) => {
    switch (s) {
      case 'deployed': return 'badge-success';
      case 'promoted': return 'badge-success';
      case 'pending': case 'syncing': return 'badge-warning';
      case 'failed': return 'badge-danger';
      case 'rolled_back': return 'bg-orange-500/10 text-orange-600';
      case 'cancelled': return 'badge-default';
      default: return 'badge-default';
    }
  };

  const chartName = deployment.spec?.chart?.chart_name;
  const chartVersion = deployment.spec?.chart?.chart_version;
  const chartUrl = deployment.spec?.chart?.chart_url;
  const cluster = clusterList.find(c => c.id === deployment.target_cluster_id);
  const displayName = deployment.gitlab_project_name || chartName || deployment.jira_issue_key || 'Deployment';

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="page-title-modern">
              {displayName}
            </h1>
            <span className={`badge ${statusColor(deployment.status)}`}>
              {deployment.status}
            </span>
          </div>
          <p className="page-subtitle-modern">
            {deployment.jira_summary || deployment.gitlab_project_name || chartName || 'Deployment details'}
            {deployment.jira_issue_key && deployment.gitlab_project_name && ` \u00B7 ${deployment.jira_issue_key}`}
            {deployment.team_name && ` \u00B7 ${deployment.team_name}${deployment.stage ? `/${deployment.stage}` : ''}`}
          </p>
        </div>
        <div className="flex gap-2">
          {deployment.status === 'deployed' && (
            <>
              <button onClick={handlePromote} className="btn btn-secondary text-[12px]">
                Promote
              </button>
              <button onClick={handleRollback} className="text-[12px] px-3 py-1.5 bg-orange-500/10 text-orange-600 border border-orange-500/20 rounded-lg hover:bg-orange-500/15">
                Rollback
              </button>
            </>
          )}
          <Link href="/deployments" className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            {'\u2190'} Back
          </Link>
        </div>
      </div>

      {/* Error Banner */}
      {deployment.status === 'failed' && deployment.error_message && (
        <div className="card border-red-500/20 bg-red-500/10">
          <div className="card-body">
            <div className="flex items-start gap-3">
              <span className="text-red-600 text-lg flex-shrink-0">&#10007;</span>
              <div className="flex-1 min-w-0">
                <p className="text-[13px] font-semibold text-red-500 mb-1">Deployment Failed</p>
                <p className="text-[12px] text-red-400 font-mono whitespace-pre-wrap break-all">{deployment.error_message}</p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Info Cards */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        {chartName && (
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Helm Chart</p>
            <p className="text-[12px] font-mono text-[var(--text-primary)] truncate">
              {chartName}{chartVersion && <span className="text-[var(--accent)]"> v{chartVersion}</span>}
            </p>
            {chartUrl && <p className="text-[10px] text-[var(--text-tertiary)] truncate">{chartUrl}</p>}
          </div>
        )}
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Image</p>
          <p className="text-[12px] font-mono text-[var(--text-primary)] truncate">
            {deployment.image_repository ? `${deployment.image_repository}:${deployment.image_tag || 'latest'}` : deployment.image_tag || '-'}
          </p>
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Target</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{deployment.target_namespace || 'default'}</p>
          {cluster && <p className="text-[10px] text-[var(--text-secondary)]">{cluster.name} ({cluster.environment})</p>}
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Created</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{new Date(deployment.created_at).toLocaleString()}</p>
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Promoted By</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{deployment.promoted_by || '-'}</p>
        </div>
      </div>

      {/* Links */}
      <div className="flex gap-4">
        {deployment.gitlab_mr_url && (
          <a href={deployment.gitlab_mr_url} target="_blank" rel="noopener noreferrer"
             className="text-[12px] text-[var(--accent)] hover:underline">
            GitLab MR {'\u2197'}
          </a>
        )}
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-[var(--border)]">
        {(['timeline', 'logs', 'history'] as const).map(tab => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-[12px] font-medium border-b-2 transition-colors ${
              activeTab === tab
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
            }`}
          >
            {tab.charAt(0).toUpperCase() + tab.slice(1)}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      {activeTab === 'timeline' && (
        <div className="card">
          <div className="card-body space-y-3">
            {(() => {
              const events: { time: string; label: string; icon: string; done: boolean }[] = [
                { time: deployment.created_at, label: 'Deployment created', icon: '\u25CB', done: true },
              ];

              if (deployment.status === 'syncing' || deployment.status === 'pending') {
                events.push({ time: deployment.created_at, label: 'Helm chart rendering', icon: '\u21BB', done: false });
              } else if (['deployed', 'promoted', 'rolled_back'].includes(deployment.status)) {
                events.push({ time: deployment.created_at, label: 'Helm chart rendered', icon: '\u21BB', done: true });
                events.push({ time: deployment.created_at, label: 'Resources applied', icon: '\u2713', done: true });
                events.push({ time: deployment.created_at, label: 'All pods ready', icon: '\u2713', done: true });
              }

              if (deployment.status === 'failed') {
                events.push({ time: deployment.updated_at, label: 'Deployment failed', icon: '\u2717', done: true });
              }

              if (deployment.status === 'promoted') {
                events.push({ time: deployment.promoted_at || deployment.created_at, label: 'Promoted to next environment', icon: '\u2B06', done: true });
              }

              if (deployment.status === 'rolled_back') {
                events.push({ time: deployment.updated_at, label: 'Rolled back', icon: '\u21A9', done: true });
              }

              if (deployment.status === 'cancelled') {
                events.push({ time: deployment.updated_at, label: 'Deployment cancelled', icon: '\u2717', done: true });
              }

              return events.map((event, i) => (
                <div key={i} className="flex items-start gap-3">
                  <div className={`w-6 h-6 rounded-full flex items-center justify-center text-[11px] shrink-0 mt-0.5 ${
                    event.done ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-secondary)]'
                  }`}>
                    {event.icon}
                  </div>
                  <div>
                    <p className="text-[13px] text-[var(--text-primary)]">{event.label}</p>
                    <p className="text-[11px] text-[var(--text-tertiary)]">{new Date(event.time).toLocaleString()}</p>
                  </div>
                </div>
              ));
            })()}
          </div>
        </div>
      )}

      {activeTab === 'logs' && (
        <div className="card">
          <div className="card-body">
            {/* Show stored deployment logs */}
            {deployment.logs ? (
              <div className="mb-4">
                <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Deployment Log</p>
                <div className="font-mono text-[12px] bg-[#1a1a2e] text-[#e0e0e0] rounded-lg p-4 max-h-[400px] overflow-y-auto whitespace-pre-wrap break-all leading-relaxed">
                  {deployment.logs}
                </div>
              </div>
            ) : null}

            {/* Show event logs */}
            {logs.length > 0 && (
              <div>
                <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Events</p>
                <div className="font-mono text-[12px] space-y-1 bg-[var(--bg)] rounded-lg p-4 max-h-[300px] overflow-y-auto">
                  {logs.map((log, i) => (
                    <div key={i} className="flex gap-3">
                      <span className="text-[var(--text-tertiary)] shrink-0">{new Date(log.timestamp).toLocaleTimeString()}</span>
                      <span className={`shrink-0 ${log.level === 'error' ? 'text-red-600' : log.level === 'warn' ? 'text-yellow-600' : 'text-green-600'}`}>
                        [{log.level}]
                      </span>
                      <span className="text-[var(--text-primary)]">{log.message}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {!deployment.logs && logs.length === 0 && (
              <p className="text-[13px] text-[var(--text-tertiary)] text-center py-6">No logs available</p>
            )}
          </div>
        </div>
      )}

      {activeTab === 'history' && (
        <div className="card">
          <div className="card-body">
            {history.length <= 1 ? (
              <p className="text-[13px] text-[var(--text-tertiary)] text-center py-6">No deployment history</p>
            ) : (
              <div className="space-y-2">
                {history.filter(h => h.id !== id).map(h => (
                  <Link
                    key={h.id}
                    href={`/deployments?id=${h.id}`}
                    className="flex items-center justify-between p-3 rounded-lg border border-[var(--border)] hover:border-[var(--border)] transition-colors"
                  >
                    <div className="flex items-center gap-3">
                      <span className={`badge ${statusColor(h.status)}`}>{h.status}</span>
                      <span className="text-[12px] font-mono text-[var(--text-primary)]">{h.image_tag || '-'}</span>
                    </div>
                    <span className="text-[11px] text-[var(--text-tertiary)]">{new Date(h.created_at).toLocaleString()}</span>
                  </Link>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
