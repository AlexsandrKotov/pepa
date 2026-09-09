'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { gitops, clusters, type GitopsRepo, type GitopsDriftResult, type GitopsDriftEntry, type Cluster, type DriftSchedule, type DriftDetectionLog, type CreateDriftScheduleInput } from '@/lib/api';

type SeverityFilter = '' | 'critical' | 'warning' | 'info';
type DriftTypeFilter = '' | 'suspended' | 'resumed' | 'version' | 'missing' | 'orphaned';

interface RepoMapping {
  clusterId: string;
  overlayPath: string;
}

const CRON_PRESETS = [
  { label: 'Every 15 minutes', value: '*/15 * * * *' },
  { label: 'Every 30 minutes', value: '*/30 * * * *' },
  { label: 'Every hour', value: '0 * * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Every 12 hours', value: '0 */12 * * *' },
  { label: 'Daily at midnight', value: '0 0 * * *' },
  { label: 'Daily at 6 AM', value: '0 6 * * *' },
  { label: 'Weekly (Monday)', value: '0 0 * * 1' },
];

export default function DriftDetectionPage() {
  const [repos, setRepos] = useState<GitopsRepo[]>([]);
  const [allClusters, setAllClusters] = useState<Cluster[]>([]);
  const [overlaysByRepo, setOverlaysByRepo] = useState<Record<string, string[]>>({});
  const [driftResults, setDriftResults] = useState<Map<string, GitopsDriftResult>>(new Map());
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>('');
  const [typeFilter, setTypeFilter] = useState<DriftTypeFilter>('');
  const [expandedRepos, setExpandedRepos] = useState<Set<string>>(new Set());
  const [error, setError] = useState('');
  const [mappingRepoId, setMappingRepoId] = useState<string | null>(null);
  const [mappings, setMappings] = useState<Record<string, RepoMapping>>({});

  // Schedule state
  const [schedules, setSchedules] = useState<DriftSchedule[]>([]);
  const [driftLogs, setDriftLogs] = useState<DriftDetectionLog[]>([]);
  const [showScheduleForm, setShowScheduleForm] = useState(false);
  const [editingSchedule, setEditingSchedule] = useState<DriftSchedule | null>(null);
  const [runningScheduleId, setRunningScheduleId] = useState<string | null>(null);
  const [showLogs, setShowLogs] = useState(false);
  const [showMapping, setShowMapping] = useState(false);
  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set());
  const [scheduleForm, setScheduleForm] = useState<CreateDriftScheduleInput>({
    repo_id: '',
    cluster_id: '',
    scope_path: '',
    name: '',
    description: '',
    cron_expression: '0 */6 * * *',
    enabled: true,
    alert_on_drift: true,
    alert_severity_threshold: 'warning',
  });

  useEffect(() => {
    load();
  }, []);

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const [repoData, clusterData, scheduleData] = await Promise.all([
        gitops.listRepos(),
        clusters.list(),
        gitops.listDriftSchedules().catch(() => ({ schedules: [] })),
      ]);
      const repoList = repoData.repos || [];
      setRepos(repoList);
      setAllClusters(clusterData.clusters || []);
      setSchedules(scheduleData.schedules || []);

      // Restore mappings from per-repo config
      const restored: Record<string, RepoMapping> = {};
      for (const repo of repoList) {
        const clusterId = repo.config?.drift_cluster_id || '';
        const scopePath = repo.config?.drift_scope_path || '';
        restored[repo.id] = { clusterId, overlayPath: scopePath };
      }
      setMappings(restored);

      // Load overlays for each repo
      const overlaysMap: Record<string, string[]> = {};
      await Promise.all(repoList.map(async (repo) => {
        try {
          const data = await gitops.listOverlays(repo.id);
          overlaysMap[repo.id] = data.overlays || [];
        } catch {
          overlaysMap[repo.id] = [];
        }
      }));
      setDriftResults(new Map());
      setExpandedRepos(new Set());
      setSeverityFilter('');
      setTypeFilter('');
      setOverlaysByRepo(overlaysMap);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  const loadLogs = async () => {
    try {
      const data = await gitops.listDriftLogs(50);
      setDriftLogs(data.logs || []);
    } catch {
      // silently fail — logs are optional
    }
  };

  const availableClusters = allClusters.filter(c => c.has_kubeconfig);

  const getMappedCluster = (repo: GitopsRepo): Cluster | null => {
    const mappedClusterId = repo.config?.drift_cluster_id;
    if (!mappedClusterId) return null;
    return allClusters.find(c => c.id === mappedClusterId) || null;
  };

  const isRepoMapped = (repo: GitopsRepo): boolean => {
    return getMappedCluster(repo) !== null;
  };

  const saveMapping = async (repoId: string) => {
    const mapping = mappings[repoId];
    if (!mapping?.clusterId) return;
    setMappingRepoId(repoId);
    try {
      await gitops.updateMapping(repoId, {
        cluster_id: mapping.clusterId,
        scope_path: mapping.overlayPath || undefined,
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save mapping');
    } finally {
      setMappingRepoId(null);
    }
  };

  const unmapCluster = async (repoId: string) => {
    setMappingRepoId(repoId);
    try {
      await gitops.deleteMapping(repoId);
      setMappings(prev => {
        const next = { ...prev };
        if (next[repoId]) next[repoId] = { clusterId: '', overlayPath: '' };
        return next;
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to unmap');
    } finally {
      setMappingRepoId(null);
    }
  };

  const updateMapping = (repoId: string, field: keyof RepoMapping, value: string) => {
    setMappings(prev => ({
      ...prev,
      [repoId]: { ...prev[repoId], [field]: value },
    }));
  };

  const mappedRepos = repos.filter(isRepoMapped);

  const runDriftDetection = async () => {
    setScanning(true);
    setError('');
    const newResults = new Map<string, GitopsDriftResult>();

    for (const repo of mappedRepos) {
      const mappedCluster = getMappedCluster(repo);
      const overlayPath = mappings[repo.id]?.overlayPath || '';
      try {
        const result = await gitops.detectDrift(repo.id, mappedCluster?.id, overlayPath || undefined);
        newResults.set(repo.id, result);
      } catch (err) {
        console.error(`Drift detection failed for ${repo.name}:`, err);
      }
    }

    setDriftResults(newResults);
    const withDrift = new Set<string>();
    newResults.forEach((result, repoId) => {
      if (result.entries.length > 0) withDrift.add(repoId);
    });
    setExpandedRepos(withDrift);
    setScanning(false);
  };

  // Schedule CRUD
  const openCreateSchedule = () => {
    setEditingSchedule(null);
    setScheduleForm({
      repo_id: repos[0]?.id || '',
      cluster_id: '',
      scope_path: '',
      name: '',
      description: '',
      cron_expression: '0 */6 * * *',
      enabled: true,
      alert_on_drift: true,
      alert_severity_threshold: 'warning',
    });
    setShowScheduleForm(true);
  };

  const openEditSchedule = (s: DriftSchedule) => {
    setEditingSchedule(s);
    setScheduleForm({
      repo_id: s.repo_id,
      cluster_id: s.cluster_id,
      scope_path: s.scope_path || '',
      name: s.name,
      description: s.description || '',
      cron_expression: s.cron_expression,
      enabled: s.enabled,
      alert_on_drift: s.alert_on_drift,
      alert_severity_threshold: s.alert_severity_threshold,
    });
    setShowScheduleForm(true);
  };

  const saveSchedule = async () => {
    try {
      if (editingSchedule) {
        await gitops.updateDriftSchedule(editingSchedule.id, scheduleForm);
      } else {
        await gitops.createDriftSchedule(scheduleForm);
      }
      setShowScheduleForm(false);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save schedule');
    }
  };

  const deleteSchedule = async (id: string) => {
    if (!confirm('Delete this drift schedule?')) return;
    try {
      await gitops.deleteDriftSchedule(id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete schedule');
    }
  };

  const toggleScheduleEnabled = async (s: DriftSchedule) => {
    try {
      await gitops.updateDriftSchedule(s.id, { enabled: !s.enabled });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to toggle schedule');
    }
  };

  const runSchedule = async (id: string) => {
    setRunningScheduleId(id);
    setError('');
    try {
      const result = await gitops.runDriftSchedule(id);
      if (result.drift_count > 0) {
        setError(`Drift detected: ${result.drift_count} issues (${result.critical_count} critical, ${result.warning_count} warning, ${result.info_count} info)`);
      }
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Drift detection failed');
    } finally {
      setRunningScheduleId(null);
    }
  };

  const toggleRepo = (repoId: string) => {
    setExpandedRepos(prev => {
      const next = new Set(prev);
      if (next.has(repoId)) next.delete(repoId); else next.add(repoId);
      return next;
    });
  };

  const toggleLog = (logId: string) => {
    setExpandedLogs(prev => {
      const next = new Set(prev);
      if (next.has(logId)) next.delete(logId); else next.add(logId);
      return next;
    });
  };

  // Summary counts
  const allEntries: Array<{ repo: GitopsRepo; entry: GitopsDriftEntry }> = [];
  driftResults.forEach((result, repoId) => {
    const repo = repos.find(r => r.id === repoId);
    if (!repo) return;
    for (const entry of result.entries) {
      allEntries.push({ repo, entry });
    }
  });

  const totalCritical = allEntries.filter(e => e.entry.severity === 'critical').length;
  const totalWarning = allEntries.filter(e => e.entry.severity === 'warning').length;
  const totalInfo = allEntries.filter(e => e.entry.severity === 'info').length;
  const totalSuspended = allEntries.filter(e => e.entry.drift_type === 'suspended').length;

  const severityBadge = (severity: string) => {
    switch (severity) {
      case 'critical': return 'bg-red-500/15 text-red-500 border-red-500/20';
      case 'warning': return 'bg-yellow-500/15 text-yellow-600 border-yellow-500/20';
      case 'info': return 'bg-blue-500/15 text-blue-500 border-blue-500/20';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]';
    }
  };

  const driftTypeIcon = (driftType: string) => {
    switch (driftType) {
      case 'suspended': return { icon: '\u23F8', label: 'Suspended in cluster', color: 'text-red-600' };
      case 'resumed': return { icon: '\u25B6', label: 'Resumed in cluster', color: 'text-yellow-600' };
      case 'version': return { icon: '\u2195', label: 'Version drift', color: 'text-orange-600' };
      case 'missing': return { icon: '\u2715', label: 'Missing from cluster', color: 'text-purple-600' };
      case 'orphaned': return { icon: '?', label: 'Not in Git', color: 'text-blue-600' };
      default: return { icon: '~', label: driftType, color: 'text-[var(--text-tertiary)]' };
    }
  };

  const formatTime = (dateStr?: string) => {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  };

  const getCronLabel = (expr: string) => {
    const preset = CRON_PRESETS.find(p => p.value === expr);
    return preset ? preset.label : expr;
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Drift Detection</h1>
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      </div></div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Drift Detection</h1>
          <p className="page-subtitle-modern">Compare Git desired state vs live cluster state</p>
        </div>
        <div className="flex items-center gap-3">
          <Link href="/gitops" className="btn btn-secondary text-[12px]">
            {'\u2190'} Back to GitOps
          </Link>
          <button
            onClick={runDriftDetection}
            disabled={scanning || mappedRepos.length === 0}
            className="btn btn-primary text-[12px] disabled:opacity-50"
          >
            {scanning ? 'Scanning...' : `Run Drift Detection${mappedRepos.length > 0 ? ` (${mappedRepos.length})` : ''}`}
          </button>
        </div>
      </div>

      {error && (
        <div className="px-4 py-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-500">
          {error}
        </div>
      )}

      {/* ── Scheduled Drift Detection ──────────────────────────── */}
      <div className="card card-body">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Scheduled Drift Detection</h3>
            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">
              Configure automated drift detection with cron schedules and alerting.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={openCreateSchedule}
              className="btn btn-primary text-[11px] py-1"
            >
              + Add Schedule
            </button>
          </div>
        </div>

        {/* Schedule form modal */}
        {showScheduleForm && (
          <div className="mb-4 p-4 rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)]">
            <h4 className="text-[12px] font-semibold text-[var(--text-primary)] mb-3">
              {editingSchedule ? 'Edit Schedule' : 'New Drift Schedule'}
            </h4>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Name</label>
                <input
                  type="text"
                  value={scheduleForm.name}
                  onChange={(e) => setScheduleForm(f => ({ ...f, name: e.target.value }))}
                  placeholder="e.g., Production drift check"
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                />
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Repository</label>
                <select
                  value={scheduleForm.repo_id}
                  onChange={(e) => setScheduleForm(f => ({ ...f, repo_id: e.target.value }))}
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="">Select repository...</option>
                  {repos.map(r => (
                    <option key={r.id} value={r.id}>{r.name} ({r.engine_type})</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Cluster</label>
                <select
                  value={scheduleForm.cluster_id}
                  onChange={(e) => setScheduleForm(f => ({ ...f, cluster_id: e.target.value }))}
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="">Select cluster...</option>
                  {availableClusters.map(c => (
                    <option key={c.id} value={c.id}>{c.name} ({c.environment})</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Scope Path</label>
                <select
                  value={scheduleForm.scope_path || ''}
                  onChange={(e) => setScheduleForm(f => ({ ...f, scope_path: e.target.value }))}
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="">All scopes (entire repo)</option>
                  {scheduleForm.repo_id && (overlaysByRepo[scheduleForm.repo_id] || []).map(o => (
                    <option key={o} value={o + '/'}>{o}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Schedule</label>
                <select
                  value={scheduleForm.cron_expression}
                  onChange={(e) => setScheduleForm(f => ({ ...f, cron_expression: e.target.value }))}
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  {CRON_PRESETS.map(p => (
                    <option key={p.value} value={p.value}>{p.label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium block mb-1">Alert Severity Threshold</label>
                <select
                  value={scheduleForm.alert_severity_threshold}
                  onChange={(e) => setScheduleForm(f => ({ ...f, alert_severity_threshold: e.target.value }))}
                  className="w-full text-[12px] px-2.5 py-1.5 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                >
                  <option value="critical">Critical only</option>
                  <option value="warning">Warning and above</option>
                  <option value="info">All (info and above)</option>
                </select>
              </div>
            </div>
            <div className="flex items-center gap-4 mt-3">
              <label className="flex items-center gap-2 text-[11px] text-[var(--text-secondary)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={scheduleForm.enabled}
                  onChange={(e) => setScheduleForm(f => ({ ...f, enabled: e.target.checked }))}
                  className="rounded"
                />
                Enabled
              </label>
              <label className="flex items-center gap-2 text-[11px] text-[var(--text-secondary)] cursor-pointer">
                <input
                  type="checkbox"
                  checked={scheduleForm.alert_on_drift}
                  onChange={(e) => setScheduleForm(f => ({ ...f, alert_on_drift: e.target.checked }))}
                  className="rounded"
                />
                Send alerts when drift detected
              </label>
            </div>
            <div className="flex items-center gap-2 mt-3">
              <button onClick={saveSchedule} className="btn btn-primary text-[11px] py-1">
                {editingSchedule ? 'Update' : 'Create'}
              </button>
              <button onClick={() => setShowScheduleForm(false)} className="btn btn-secondary text-[11px] py-1">
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Schedule list */}
        {schedules.length > 0 ? (
          <div className="space-y-2">
            {schedules.map(s => {
              const repo = repos.find(r => r.id === s.repo_id);
              const cluster = allClusters.find(c => c.id === s.cluster_id);
              return (
                <div key={s.id} className="p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg-primary)]">
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => toggleScheduleEnabled(s)}
                      className={`shrink-0 w-8 h-5 rounded-full transition-colors relative ${s.enabled ? 'bg-emerald-500' : 'bg-[var(--border)]'}`}
                      title={s.enabled ? 'Enabled — click to disable' : 'Disabled — click to enable'}
                    >
                      <span className={`absolute top-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform ${s.enabled ? 'left-3.5' : 'left-0.5'}`} />
                    </button>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-[12px] font-semibold text-[var(--text-primary)]">{s.name}</span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${s.enabled ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
                          {s.enabled ? 'Active' : 'Paused'}
                        </span>
                        {s.alert_on_drift && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-600 border border-amber-500/20">
                            Alerts on
                          </span>
                        )}
                      </div>
                      <div className="flex items-center gap-3 mt-1">
                        {repo && (
                          <span className="text-[10px] text-[var(--text-tertiary)]">
                            {repo.name} ({repo.engine_type})
                          </span>
                        )}
                        {cluster && (
                          <span className="text-[10px] text-emerald-600">
                            {'\u25CF'} {cluster.name}
                          </span>
                        )}
                        {s.scope_path && (
                          <span className="text-[10px] font-mono text-[var(--text-tertiary)]">{s.scope_path}</span>
                        )}
                        <span className="text-[10px] text-[var(--text-tertiary)]">
                          {getCronLabel(s.cron_expression)}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <div className="text-right">
                        {s.last_run_at && (
                          <p className="text-[10px] text-[var(--text-tertiary)]">
                            Last: {formatTime(s.last_run_at)}
                            {s.last_run_status && (
                              <span className={s.last_run_status === 'success' ? ' text-emerald-600' : ' text-red-500'}>
                                {' '}({s.last_run_status})
                              </span>
                            )}
                          </p>
                        )}
                        {s.next_run_at && s.enabled && (
                          <p className="text-[10px] text-[var(--text-tertiary)]">
                            Next: {formatTime(s.next_run_at)}
                          </p>
                        )}
                        {s.last_drift_count > 0 && (
                          <p className="text-[10px] text-amber-600 font-medium">
                            {s.last_drift_count} drifts detected
                          </p>
                        )}
                      </div>
                      <button
                        onClick={() => runSchedule(s.id)}
                        disabled={runningScheduleId === s.id}
                        className="text-[11px] px-2.5 py-1 rounded bg-[var(--accent)] text-white font-medium disabled:opacity-50"
                      >
                        {runningScheduleId === s.id ? 'Running...' : 'Run Now'}
                      </button>
                      <button
                        onClick={() => openEditSchedule(s)}
                        className="text-[11px] px-2 py-1 rounded border border-[var(--border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => deleteSchedule(s.id)}
                        className="text-[11px] px-2 py-1 rounded text-[var(--text-tertiary)] hover:text-red-500 hover:bg-red-500/10"
                      >
                        {'\u2715'}
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="text-center py-6">
            <p className="text-[12px] text-[var(--text-tertiary)]">
              No scheduled drift detection configured.
              <button onClick={openCreateSchedule} className="text-[var(--accent)] hover:underline ml-1">
                Create your first schedule
              </button>
            </p>
          </div>
        )}

        {/* Drift logs — collapsible section */}
        <div className="mt-4 pt-3 border-t border-[var(--border-light)]">
          <button
            onClick={() => { setShowLogs(!showLogs); if (!showLogs) loadLogs(); }}
            className="flex items-center gap-2 w-full group"
          >
            <svg className={`w-3.5 h-3.5 text-[var(--text-tertiary)] transition-transform ${showLogs ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
            <h4 className="text-[12px] font-semibold text-[var(--text-primary)]">Detection History</h4>
            {driftLogs.length > 0 && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-tertiary)]">{driftLogs.length}</span>
            )}
          </button>
          {showLogs && (
            <div className="mt-2">
              {driftLogs.length > 0 ? (
                <div className="space-y-1.5 max-h-[400px] overflow-y-auto">
                  {driftLogs.map(log => {
                    const isLogExpanded = expandedLogs.has(log.id);
                    return (
                      <div key={log.id} className="rounded bg-[var(--bg-primary)] overflow-hidden">
                        <button
                          onClick={() => toggleLog(log.id)}
                          className="w-full flex items-center gap-3 px-3 py-2 text-[11px] hover:bg-[var(--bg-secondary)] transition-colors"
                        >
                          <svg className={`w-3 h-3 text-[var(--text-tertiary)] transition-transform shrink-0 ${isLogExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                          </svg>
                          <span className={`w-2 h-2 rounded-full shrink-0 ${
                            log.status === 'success' && log.drift_count === 0 ? 'bg-emerald-500' :
                            log.status === 'success' ? 'bg-amber-500' : 'bg-red-500'
                          }`} />
                          <span className="text-[var(--text-secondary)]">{formatTime(log.started_at)}</span>
                          <span className="font-medium text-[var(--text-primary)]">{log.repo_name}</span>
                          <span className="text-[var(--text-tertiary)]">{'\u2192'} {log.cluster_name}</span>
                          <span className={`px-1.5 py-0.5 rounded ${
                            log.triggered_by === 'manual' ? 'bg-blue-500/10 text-blue-500' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
                          }`}>
                            {log.triggered_by}
                          </span>
                          {log.drift_count > 0 ? (
                            <span className="text-amber-600 font-medium">
                              {log.drift_count} drifts ({log.critical_count}C / {log.warning_count}W / {log.info_count}I)
                            </span>
                          ) : (
                            <span className="text-emerald-600">No drift</span>
                          )}
                          {log.status === 'error' && (
                            <span className="text-red-500 ml-auto shrink-0">Error</span>
                          )}
                        </button>
                        {isLogExpanded && (
                          <div className="px-3 pb-2.5 pt-0.5 border-t border-[var(--border-light)] bg-[var(--bg-secondary)]">
                            <div className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-[11px] mt-2">
                              <div className="flex items-center gap-2">
                                <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Status:</span>
                                <span className={log.status === 'success' ? 'text-emerald-600 font-medium' : 'text-red-500 font-medium'}>{log.status}</span>
                              </div>
                              <div className="flex items-center gap-2">
                                <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Triggered by:</span>
                                <span className="text-[var(--text-primary)]">{log.triggered_by}</span>
                              </div>
                              <div className="flex items-center gap-2">
                                <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Cluster:</span>
                                <span className="text-[var(--text-primary)]">{log.cluster_name || '\u2014'}</span>
                              </div>
                              {log.scope_path && (
                                <div className="flex items-center gap-2">
                                  <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Scope:</span>
                                  <span className="text-[var(--text-primary)] font-mono">{log.scope_path}</span>
                                </div>
                              )}
                              <div className="flex items-center gap-2">
                                <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Started:</span>
                                <span className="text-[var(--text-primary)]">{formatTime(log.started_at)}</span>
                              </div>
                              {log.completed_at && (
                                <div className="flex items-center gap-2">
                                  <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Completed:</span>
                                  <span className="text-[var(--text-primary)]">{formatTime(log.completed_at)}</span>
                                </div>
                              )}
                              {log.drift_count > 0 && (
                                <div className="flex items-center gap-2">
                                  <span className="text-[var(--text-tertiary)] uppercase text-[10px] tracking-wider">Breakdown:</span>
                                  <span className="flex items-center gap-2">
                                    {log.critical_count > 0 && <span className="text-red-500">{log.critical_count} critical</span>}
                                    {log.warning_count > 0 && <span className="text-yellow-600">{log.warning_count} warning</span>}
                                    {log.info_count > 0 && <span className="text-blue-500">{log.info_count} info</span>}
                                  </span>
                                </div>
                              )}
                            </div>
                            {log.status === 'error' && log.error_message && (
                              <div className="mt-2 px-2.5 py-1.5 rounded bg-red-500/10 border border-red-500/20">
                                <p className="text-[11px] text-red-500 font-mono break-all">{log.error_message}</p>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              ) : (
                <p className="text-[11px] text-[var(--text-tertiary)] text-center py-3">No detection history yet</p>
              )}
            </div>
          )}
        </div>
      </div>

      {/* ── Cluster & Scope Mapping (collapsible) ───────── */}
      <div className="card overflow-hidden">
        <button
          onClick={() => setShowMapping(!showMapping)}
          className="w-full flex items-center justify-between px-5 py-3 hover:bg-[var(--bg)] transition-colors"
        >
          <div className="flex items-center gap-3">
            <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${showMapping ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
            <div className="text-left">
              <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">Cluster & Scope Mapping</h3>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">
                Map each GitOps repository to a cluster and select a scope path to compare against.
              </p>
            </div>
          </div>
          {repos.length > 0 && (
            <div className="flex items-center gap-2 shrink-0">
              <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${
                mappedRepos.length === repos.length
                  ? 'bg-emerald-500/15 text-emerald-600'
                  : mappedRepos.length > 0
                    ? 'bg-amber-500/15 text-amber-600'
                    : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
              }`}>
                {mappedRepos.length} of {repos.length} mapped
              </span>
            </div>
          )}
        </button>

        {showMapping && (
          <div className="px-5 pb-5 border-t border-[var(--border-light)]">
            <div className="space-y-3 pt-4">
              {repos.map(repo => {
                const mappedCluster = getMappedCluster(repo);
                const isMapped = mappedCluster !== null;
                const isMapping = mappingRepoId === repo.id;
                const overlays = overlaysByRepo[repo.id] || [];
                const mapping = mappings[repo.id] || { clusterId: '', overlayPath: '' };

                return (
                  <div key={repo.id} className="p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg-primary)]">
                    <div className="flex items-center gap-3 mb-3">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-[12px] font-semibold text-[var(--text-primary)]">{repo.name}</span>
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-tertiary)] font-mono">
                            {repo.engine_type}
                          </span>
                          {repo.branch && repo.branch !== 'main' && (
                            <span className="text-[10px] text-[var(--text-tertiary)]">branch: {repo.branch}</span>
                          )}
                        </div>
                        <p className="text-[10px] text-[var(--text-tertiary)] truncate mt-0.5">{repo.repo_url}</p>
                      </div>
                    </div>

                    <div className="flex flex-wrap items-center gap-3">
                      <div className="flex items-center gap-2">
                        <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium">Cluster:</label>
                        {isMapped ? (
                          <div className="flex items-center gap-1.5">
                            <span className="flex items-center gap-1.5 text-[12px] font-medium text-emerald-600">
                              <span className="w-2 h-2 rounded-full bg-green-500" />
                              {mappedCluster!.name}
                              <span className="text-[10px] text-[var(--text-tertiary)] font-normal">({mappedCluster!.environment})</span>
                            </span>
                            <button
                              onClick={() => unmapCluster(repo.id)}
                              disabled={isMapping}
                              className="text-[10px] px-1.5 py-0.5 rounded text-[var(--text-tertiary)] hover:text-red-500 hover:bg-red-500/10 border border-transparent hover:border-red-500/20"
                              title="Unmap cluster"
                            >
                              {'\u2715'}
                            </button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-1.5">
                            <select
                              value={mapping.clusterId || ''}
                              onChange={(e) => updateMapping(repo.id, 'clusterId', e.target.value)}
                              className="text-[11px] px-2 py-1 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)]"
                            >
                              <option value="">Select cluster...</option>
                              {availableClusters.map(c => (
                                <option key={c.id} value={c.id}>{c.name} ({c.environment})</option>
                              ))}
                            </select>
                          </div>
                        )}
                      </div>

                      <div className="flex items-center gap-2">
                        <label className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium">Scope:</label>
                        <select
                          value={mapping.overlayPath || ''}
                          onChange={(e) => updateMapping(repo.id, 'overlayPath', e.target.value)}
                          className="text-[11px] px-2 py-1 rounded border border-[var(--border)] bg-[var(--bg-primary)] text-[var(--text-primary)] max-w-[280px]"
                        >
                          <option value="">All scopes (entire repo)</option>
                          {overlays.map(o => (
                            <option key={o} value={o + '/'}>{o}</option>
                          ))}
                        </select>
                      </div>

                      {!isMapped && mapping.clusterId && (
                        <button
                          onClick={() => saveMapping(repo.id)}
                          disabled={isMapping}
                          className="text-[11px] px-2.5 py-1 rounded bg-[var(--accent)] text-white font-medium disabled:opacity-50"
                        >
                          {isMapping ? 'Saving...' : 'Save Mapping'}
                        </button>
                      )}
                    </div>
                  </div>
                );
              })}

              {repos.length === 0 && (
                <p className="text-[12px] text-[var(--text-tertiary)] text-center py-4">
                  No GitOps repositories configured. <Link href="/gitops" className="text-[var(--accent)] hover:underline">Add a repository</Link>
                </p>
              )}

              {repos.length > 0 && availableClusters.length === 0 && (
                <p className="text-[11px] text-[var(--text-tertiary)] text-center py-2">
                  No clusters with kubeconfig available. <Link href="/clusters" className="text-[var(--accent)] hover:underline">Import a kubeconfig first</Link>
                </p>
              )}
            </div>

            {repos.length > 0 && (
              <div className="mt-4 pt-3 border-t border-[var(--border-light)] flex items-center gap-4">
                {repos.filter(r => !isRepoMapped(r)).length > 0 && (
                  <span className="text-[10px] text-amber-600">
                    {repos.filter(r => !isRepoMapped(r)).map(r => r.name).join(', ')} — not mapped
                  </span>
                )}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Summary Bar */}
      {driftResults.size > 0 && (
        <div className="card card-body py-3">
          <div className="flex items-center gap-6 flex-wrap">
            <span className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider">Drift Summary:</span>
            {totalCritical > 0 && (
              <button
                onClick={() => setSeverityFilter(severityFilter === 'critical' ? '' : 'critical')}
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12px] font-medium border transition-opacity ${severityBadge('critical')} ${severityFilter && severityFilter !== 'critical' ? 'opacity-30' : ''}`}
              >
                <span className="w-2 h-2 rounded-full bg-red-500" />
                {totalCritical} Critical
              </button>
            )}
            {totalWarning > 0 && (
              <button
                onClick={() => setSeverityFilter(severityFilter === 'warning' ? '' : 'warning')}
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12px] font-medium border transition-opacity ${severityBadge('warning')} ${severityFilter && severityFilter !== 'warning' ? 'opacity-30' : ''}`}
              >
                <span className="w-2 h-2 rounded-full bg-yellow-500" />
                {totalWarning} Warning
              </button>
            )}
            {totalInfo > 0 && (
              <button
                onClick={() => setSeverityFilter(severityFilter === 'info' ? '' : 'info')}
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[12px] font-medium border transition-opacity ${severityBadge('info')} ${severityFilter && severityFilter !== 'info' ? 'opacity-30' : ''}`}
              >
                <span className="w-2 h-2 rounded-full bg-blue-500" />
                {totalInfo} Info
              </button>
            )}
            {totalSuspended > 0 && (
              <div className="ml-auto flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-red-500/10 border border-red-500/20">
                <span className="text-[12px]">{'\u23F8'}</span>
                <span className="text-[12px] font-semibold text-red-500">{totalSuspended} suspended outside Git</span>
              </div>
            )}
            {allEntries.length === 0 && (
              <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-emerald-500/10 border border-emerald-500/20">
                <span className="text-[12px]">{'\u2713'}</span>
                <span className="text-[12px] font-semibold text-emerald-600">No drift detected</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Filters */}
      {(severityFilter || typeFilter) && (
        <div className="flex items-center gap-2">
          <span className="text-[11px] text-[var(--text-tertiary)]">Filters:</span>
          {severityFilter && (
            <button onClick={() => setSeverityFilter('')} className="text-[11px] px-2 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--border)]">
              {severityFilter} {'\u00D7'}
            </button>
          )}
          {typeFilter && (
            <button onClick={() => setTypeFilter('')} className="text-[11px] px-2 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--border)]">
              {typeFilter} {'\u00D7'}
            </button>
          )}
          <button onClick={() => { setSeverityFilter(''); setTypeFilter(''); }} className="text-[11px] text-[var(--accent)] hover:underline">
            Clear all
          </button>
        </div>
      )}

      {/* Drift Results */}
      {driftResults.size > 0 ? (
        <div className="space-y-4">
          {repos.map(repo => {
            const result = driftResults.get(repo.id);
            if (!result) return null;

            const mappedCluster = getMappedCluster(repo);
            const overlayPath = mappings[repo.id]?.overlayPath || '';
            const repoEntries = result.entries.filter(e => {
              if (severityFilter && e.severity !== severityFilter) return false;
              if (typeFilter && e.drift_type !== typeFilter) return false;
              return true;
            });

            if (repoEntries.length === 0 && result.entries.length === 0) return null;

            const isExpanded = expandedRepos.has(repo.id);

            return (
              <div key={repo.id} className="card overflow-hidden">
                <button
                  onClick={() => toggleRepo(repo.id)}
                  className="w-full flex items-center justify-between px-5 py-3 hover:bg-[var(--bg)] transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${isExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                    </svg>
                    <span className="text-[13px] font-semibold text-[var(--text-primary)]">{repo.name}</span>
                    {mappedCluster && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 border border-emerald-500/20">
                        {'\u25CF'} {mappedCluster.name}
                      </span>
                    )}
                    {overlayPath && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 border border-blue-500/20 font-mono">
                        {overlayPath}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {result.summary.critical > 0 && (
                      <span className="text-[11px] px-2 py-0.5 rounded-full bg-red-500/15 text-red-500 font-medium">
                        {result.summary.critical} critical
                      </span>
                    )}
                    {result.summary.warning > 0 && (
                      <span className="text-[11px] px-2 py-0.5 rounded-full bg-yellow-500/15 text-yellow-600 font-medium">
                        {result.summary.warning} warning
                      </span>
                    )}
                    {result.entries.length === 0 && (
                      <span className="text-[11px] px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-600 font-medium">
                        No drift
                      </span>
                    )}
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      {result.summary.total_compared} resources compared
                    </span>
                  </div>
                </button>

                {isExpanded && repoEntries.length > 0 && (
                  <div className="border-t border-[var(--border-light)]">
                    <div className="divide-y divide-[var(--border-light)]">
                      {repoEntries.map((entry, idx) => {
                        const typeInfo = driftTypeIcon(entry.drift_type);
                        return (
                          <div key={idx} className="px-5 py-3 flex items-start gap-4">
                            <div className={`w-2 h-2 rounded-full mt-1.5 shrink-0 ${
                              entry.severity === 'critical' ? 'bg-red-500' :
                              entry.severity === 'warning' ? 'bg-yellow-500' : 'bg-blue-500'
                            }`} />
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2 mb-1">
                                <span className={`text-[13px] font-medium ${typeInfo.color}`}>
                                  {typeInfo.icon} {entry.kind}/{entry.name}
                                </span>
                                <span className="text-[11px] text-[var(--text-tertiary)]">ns: {entry.namespace}</span>
                                {entry.cluster && (
                                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-tertiary)]">
                                    {entry.cluster}
                                  </span>
                                )}
                                <span className={`text-[10px] px-1.5 py-0.5 rounded-full border font-medium ${severityBadge(entry.severity)}`}>
                                  {entry.severity}
                                </span>
                              </div>
                              <p className="text-[12px] text-[var(--text-secondary)]">{entry.description}</p>
                              <div className="flex items-center gap-4 mt-1.5">
                                <div className="flex items-center gap-1.5">
                                  <span className="text-[10px] text-[var(--text-tertiary)] uppercase">Git:</span>
                                  <span className="text-[11px] font-mono text-[var(--text-primary)]">{entry.git_value || '-'}</span>
                                </div>
                                <div className="flex items-center gap-1.5">
                                  <span className="text-[10px] text-[var(--text-tertiary)] uppercase">Cluster:</span>
                                  <span className="text-[11px] font-mono text-[var(--text-primary)]">{entry.cluster_value || '-'}</span>
                                </div>
                              </div>
                              {entry.file_path && (
                                <div className="mt-1">
                                  <span className="text-[10px] text-[var(--text-tertiary)] font-mono">{entry.file_path}</span>
                                </div>
                              )}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {isExpanded && repoEntries.length === 0 && result.entries.length > 0 && (
                  <div className="border-t border-[var(--border-light)] px-5 py-4 text-center">
                    <p className="text-[12px] text-[var(--text-tertiary)]">No entries match the current filters</p>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="card card-body text-center py-16">
          <div className="text-[40px] mb-3 opacity-30">{'\uD83D\uDD0D'}</div>
          <h3 className="text-[14px] font-semibold text-[var(--text-primary)] mb-1">
            {mappedRepos.length === 0 && repos.length > 0 ? 'No clusters mapped' : 'No drift scan results yet'}
          </h3>
          <p className="text-[12px] text-[var(--text-tertiary)] max-w-md mx-auto">
            {mappedRepos.length === 0 && repos.length > 0 ? (
              <>Map a cluster and select a scope path above to enable drift detection.</>
            ) : (
              <>Run drift detection to compare the desired state in your Git repositories against the actual state in your Kubernetes clusters.</>
            )}
          </p>
          {repos.length === 0 && (
            <Link href="/gitops" className="inline-block mt-4 text-[12px] text-[var(--accent)] hover:underline">
              Add a GitOps repository first {'\u2192'}
            </Link>
          )}
        </div>
      )}
      </div>
    </div>
  );
}
