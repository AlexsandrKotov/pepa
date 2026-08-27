'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { gitops, clusters, type GitopsRepo, type GitopsDriftResult, type GitopsDriftEntry, type Cluster } from '@/lib/api';

type SeverityFilter = '' | 'critical' | 'warning' | 'info';
type DriftTypeFilter = '' | 'suspended' | 'resumed' | 'version' | 'missing' | 'orphaned';

interface RepoMapping {
  clusterId: string;
  overlayPath: string;
}

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

  useEffect(() => {
    load();
  }, []);

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const [repoData, clusterData] = await Promise.all([
        gitops.listRepos(),
        clusters.list(),
      ]);
      const repoList = repoData.repos || [];
      setRepos(repoList);
      setAllClusters(clusterData.clusters || []);

      // Restore mappings from per-repo config (drift_cluster_id, drift_scope_path)
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

  const toggleRepo = (repoId: string) => {
    setExpandedRepos(prev => {
      const next = new Set(prev);
      if (next.has(repoId)) next.delete(repoId); else next.add(repoId);
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
      case 'suspended': return { icon: '⏸', label: 'Suspended in cluster', color: 'text-red-600' };
      case 'resumed': return { icon: '▶', label: 'Resumed in cluster', color: 'text-yellow-600' };
      case 'version': return { icon: '↕', label: 'Version drift', color: 'text-orange-600' };
      case 'missing': return { icon: '✕', label: 'Missing from cluster', color: 'text-purple-600' };
      case 'orphaned': return { icon: '?', label: 'Not in Git', color: 'text-blue-600' };
      default: return { icon: '~', label: driftType, color: 'text-[var(--text-tertiary)]' };
    }
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

      {/* Cluster & Scope Mapping */}
      <div className="card card-body">
        <h3 className="text-[13px] font-semibold text-[var(--text-primary)] mb-1">Cluster & Scope Mapping</h3>
        <p className="text-[11px] text-[var(--text-tertiary)] mb-4">
          Map each GitOps repository to a cluster and select a scope path to compare against. Only resources under the selected scope will be checked.
        </p>

        <div className="space-y-3">
          {repos.map(repo => {
            const mappedCluster = getMappedCluster(repo);
            const isMapped = mappedCluster !== null;
            const isMapping = mappingRepoId === repo.id;
            const overlays = overlaysByRepo[repo.id] || [];
            const mapping = mappings[repo.id] || { clusterId: '', overlayPath: '' };

            return (
              <div key={repo.id} className="p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg-primary)]">
                {/* Repo header row */}
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

                {/* Mapping controls */}
                <div className="flex flex-wrap items-center gap-3">
                  {/* Cluster selector */}
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
                          ✕
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

                  {/* Scope path selector */}
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

                  {/* Save button (show when cluster selected but not yet mapped) */}
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

        {/* Summary */}
        {repos.length > 0 && (
          <div className="mt-4 pt-3 border-t border-[var(--border-light)] flex items-center gap-4">
            <span className="text-[11px] text-[var(--text-tertiary)]">
              {mappedRepos.length} of {repos.length} repos mapped
            </span>
            {repos.filter(r => !isRepoMapped(r)).length > 0 && (
              <span className="text-[10px] text-amber-600">
                {repos.filter(r => !isRepoMapped(r)).map(r => r.name).join(', ')} — not mapped
              </span>
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
                <span className="text-[12px]">⏸</span>
                <span className="text-[12px] font-semibold text-red-500">{totalSuspended} suspended outside Git</span>
              </div>
            )}
            {allEntries.length === 0 && (
              <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-emerald-500/10 border border-emerald-500/20">
                <span className="text-[12px]">&#10003;</span>
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
              {severityFilter} ×
            </button>
          )}
          {typeFilter && (
            <button onClick={() => setTypeFilter('')} className="text-[11px] px-2 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--border)]">
              {typeFilter} ×
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
                        ● {mappedCluster.name}
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
                            <div className="shrink-0">
                              {entry.drift_type === 'suspended' && (
                                <span className="text-[10px] px-2 py-1 rounded bg-red-500/10 text-red-500 border border-red-500/20">CLI suspend detected</span>
                              )}
                              {entry.drift_type === 'missing' && (
                                <span className="text-[10px] px-2 py-1 rounded bg-violet-500/10 text-violet-500 border border-violet-500/20">Not deployed</span>
                              )}
                              {entry.drift_type === 'orphaned' && (
                                <span className="text-[10px] px-2 py-1 rounded bg-blue-500/10 text-blue-500 border border-blue-500/20">Manual deploy?</span>
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
          <div className="text-[40px] mb-3 opacity-30">&#128269;</div>
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
              Add a GitOps repository first →
            </Link>
          )}
        </div>
      )}

      {/* How it works */}
      <div className="card card-body">
        <h3 className="text-[13px] font-semibold text-[var(--text-primary)] mb-3">How drift detection works</h3>
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 text-[12px]">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="w-5 h-5 rounded-full bg-[var(--accent)] text-white flex items-center justify-center text-[10px] font-bold">1</span>
              <span className="font-medium text-[var(--text-primary)]">Map cluster</span>
            </div>
            <p className="text-[var(--text-tertiary)] pl-7">Select which cluster corresponds to each GitOps repository.</p>
          </div>
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="w-5 h-5 rounded-full bg-[var(--accent)] text-white flex items-center justify-center text-[10px] font-bold">2</span>
              <span className="font-medium text-[var(--text-primary)]">Select scope</span>
            </div>
            <p className="text-[var(--text-tertiary)] pl-7">Choose a scope path (e.g. <code className="px-1 rounded bg-[var(--border-light)] text-[10px]">overlays/staging</code>, <code className="px-1 rounded bg-[var(--border-light)] text-[10px]">clusters/production</code>) to scope drift detection.</p>
          </div>
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="w-5 h-5 rounded-full bg-[var(--accent)] text-white flex items-center justify-center text-[10px] font-bold">3</span>
              <span className="font-medium text-[var(--text-primary)]">Compare states</span>
            </div>
            <p className="text-[var(--text-tertiary)] pl-7">PEPA reads Git manifests and queries the live cluster (FluxCD CRDs) to find differences.</p>
          </div>
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="w-5 h-5 rounded-full bg-[var(--accent)] text-white flex items-center justify-center text-[10px] font-bold">4</span>
              <span className="font-medium text-[var(--text-primary)]">Detect & alert</span>
            </div>
            <p className="text-[var(--text-tertiary)] pl-7">Detects suspend changes, version mismatches, missing or orphaned resources.</p>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}
