'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useSearchParams, useRouter } from 'next/navigation';
import { gitops, type GitopsRepo, type GitopsResource, type GitopsFileNode, type GitopsClusterInfo } from '@/lib/api';
import ServiceCard from '@/components/gitops/ServiceCard';
import ResourceTree from '@/components/gitops/ResourceTree';

type ViewMode = 'grid' | 'table' | 'tree';
type KindFilter = 'all' | 'HelmRelease' | 'Kustomization' | 'Application';

export default function GitOpsRepoPage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const repoId = searchParams.get('id') as string;

  const [repo, setRepo] = useState<GitopsRepo | null>(null);
  const [resources, setResources] = useState<GitopsResource[]>([]);
  const [tree, setTree] = useState<GitopsFileNode | null>(null);
  const [clusterInfo, setClusterInfo] = useState<GitopsClusterInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);

  // Filters
  const [selectedEnvironment, setSelectedEnvironment] = useState<string>('all');
  const [selectedCluster, setSelectedCluster] = useState<string>('all');
  const [kindFilter, setKindFilter] = useState<KindFilter>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>('grid');

  // Modals
  const [selectedResource, setSelectedResource] = useState<GitopsResource | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [editEnabled, setEditEnabled] = useState(false); // view-only by default
  const [editedYaml, setEditedYaml] = useState('');
  const [commitMessage, setCommitMessage] = useState('');
  const [committing, setCommitting] = useState(false);
  const [commitResult, setCommitResult] = useState<{ commit_sha: string; branch: string } | null>(null);

  // Collapsed kind sections in grid view
  const [collapsedKinds, setCollapsedKinds] = useState<Set<string>>(new Set());
  const toggleKindSection = (kind: string) => {
    setCollapsedKinds(prev => {
      const next = new Set(prev);
      if (next.has(kind)) next.delete(kind); else next.add(kind);
      return next;
    });
  };

  // Suspend/Resume
  const [suspendModal, setSuspendModal] = useState<{ resource: GitopsResource; suspend: boolean } | null>(null);
  const [suspendCommitMsg, setSuspendCommitMsg] = useState('');
  const [suspending, setSuspending] = useState(false);

  // Create
  const [showCreateModal, setShowCreateModal] = useState(false);

  // Escape key closes modals
  const anyModalOpen = editMode || suspendModal !== null || showCreateModal;
  useEscapeKey(() => {
    if (showCreateModal) setShowCreateModal(false);
    else if (suspendModal) setSuspendModal(null);
    else if (editMode) setEditMode(false);
  }, anyModalOpen);

  const loadRepo = useCallback(async () => {
    try {
      const data = await gitops.getRepo(repoId);
      setRepo(data);
    } catch {
      router.push('/gitops');
    }
  }, [repoId, router]);

  const loadResources = useCallback(async () => {
    try {
      const data = await gitops.listResources(repoId);
      setResources(data.resources || []);
      setTree(data.tree || null);
      setClusterInfo(data.clusters || []);
    } catch { /* ignore */ }
  }, [repoId]);

  const loadAll = useCallback(async () => {
    setLoading(true);
    await Promise.all([loadRepo(), loadResources()]);
    setLoading(false);
  }, [loadRepo, loadResources]);

  useEffect(() => { loadAll(); }, [loadAll]);

  const handleScan = async () => {
    setScanning(true);
    try {
      await gitops.scanRepo(repoId);
      await loadAll();
    } catch { /* ignore */ }
    setScanning(false);
  };

  // Non-environment directory names (Kustomize structural dirs, etc.)
  const nonEnvNames = new Set(['base', 'services', 'overlays', 'overlay', 'clusters', 'environments', 'envs', 'env', 'components', 'teams']);

  // Extract environments from multiple sources
  const environments = Array.from(new Set([
    ...clusterInfo.map(c => c.environment).filter((e): e is string => !!e && !nonEnvNames.has(e.toLowerCase())),
    ...resources.map(r => r.environment).filter((e): e is string => !!e && !nonEnvNames.has(e.toLowerCase())),
  ])).sort();
  
  // Get clusters for selected environment
  const clustersInEnv = selectedEnvironment === 'all' 
    ? [] 
    : clusterInfo.find(c => c.environment === selectedEnvironment)?.sub_clusters ||
      // Fallback: extract from resources
      Array.from(new Set(
        resources.filter(r => r.environment === selectedEnvironment && r.cluster)
          .map(r => r.cluster!)
      )).sort().map(name => ({
        name,
        resource_count: resources.filter(r => r.environment === selectedEnvironment && r.cluster === name).length,
      }));

  // Filter resources
  const filteredResources = resources.filter(r => {
    if (selectedEnvironment !== 'all' && r.environment !== selectedEnvironment) return false;
    if (selectedCluster !== 'all' && r.cluster !== selectedCluster) return false;
    if (kindFilter !== 'all' && r.kind !== kindFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return r.name.toLowerCase().includes(q) ||
             r.namespace.toLowerCase().includes(q) ||
             r.file_path.toLowerCase().includes(q);
    }
    return true;
  });

  // Group by kind for grid view
  const groupedResources = filteredResources.reduce<Record<string, GitopsResource[]>>((acc, r) => {
    (acc[r.kind] ||= []).push(r);
    return acc;
  }, {});

  // Suspend/Resume handlers
  const handleSuspendClick = (resource: GitopsResource) => {
    setSuspendModal({ resource, suspend: true });
    setSuspendCommitMsg(`gitops(PEPA): suspend ${resource.kind}/${resource.name}`);
  };

  const handleResumeClick = (resource: GitopsResource) => {
    setSuspendModal({ resource, suspend: false });
    setSuspendCommitMsg(`gitops(PEPA): resume ${resource.kind}/${resource.name}`);
  };

  const handleSuspendConfirm = async () => {
    if (!suspendModal) return;
    setSuspending(true);
    try {
      await gitops.suspendResource(repoId, suspendModal.resource.name, {
        file_path: suspendModal.resource.file_path,
        suspend: suspendModal.suspend,
        commit_message: suspendCommitMsg,
        resource_kind: suspendModal.resource.kind,
        resource_name: suspendModal.resource.name,
      });
      await loadResources();
      setSuspendModal(null);
    } catch { /* ignore */ }
    setSuspending(false);
  };

  // Edit handlers
  const handleEditClick = (resource: GitopsResource) => {
    setSelectedResource(resource);
    setEditedYaml(resource.raw_yaml || '');
    setEditEnabled(false); // start in view-only mode
    setEditMode(true);
    setCommitResult(null);
  };

  const handleSaveEdit = async () => {
    if (!selectedResource) return;
    setCommitting(true);
    try {
      const msg = commitMessage || `gitops(PEPA): update ${selectedResource.kind}/${selectedResource.name}`;
      const result = await gitops.editValues(repoId, selectedResource.name, {
        file_path: selectedResource.file_path,
        full_yaml: editedYaml,
        commit_message: msg,
      });
      setCommitResult(result);
      setEditMode(false);
      await loadResources();
    } catch { /* ignore */ }
    setCommitting(false);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-[var(--text-tertiary)]">Loading...</p>
      </div>
    );
  }

  if (!repo) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-[var(--text-tertiary)]">Repository not found</p>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button
            onClick={() => router.push('/gitops')}
            className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
          >
            ← Back
          </button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="page-title-modern">{repo.name}</h1>
              <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                repo.scan_status === 'ready' ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-secondary)]'
              }`}>
                {repo.scan_status}
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-tertiary)] font-mono">
              {repo.repo_url} · {repo.branch} · {repo.engine_type}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleScan}
            disabled={scanning}
            className="btn text-[12px] disabled:opacity-50"
          >
            {scanning ? 'Scanning...' : 'Re-scan'}
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="btn btn-primary text-[12px]"
          >
            + New Service
          </button>
        </div>
      </div>

      {/* Environment Tabs */}
      {environments.length > 0 && (
        <div className="flex items-center gap-2 border-b border-[var(--border)] pb-2">
          <span className="text-[11px] text-[var(--text-tertiary)] mr-2">Environments:</span>
          <button
            onClick={() => { setSelectedEnvironment('all'); setSelectedCluster('all'); }}
            className={`px-3 py-1 rounded-lg text-[11px] font-medium transition-colors ${
              selectedEnvironment === 'all'
                ? 'bg-[var(--accent)] text-white'
                : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--accent-subtle)]'
            }`}
          >
            All
          </button>
          {environments.map(env => (
            <button
              key={env}
              onClick={() => { setSelectedEnvironment(env); setSelectedCluster('all'); }}
              className={`px-3 py-1 rounded-lg text-[11px] font-medium transition-colors ${
                selectedEnvironment === env
                  ? 'bg-[var(--accent)] text-white'
                  : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--accent-subtle)]'
              }`}
            >
              {env}
            </button>
          ))}
        </div>
      )}

      {/* Cluster Sub-Tabs (when environment is selected) */}
      {selectedEnvironment !== 'all' && clustersInEnv.length > 0 && (
        <div className="flex items-center gap-2 border-b border-[var(--border)] pb-2">
          <span className="text-[11px] text-[var(--text-tertiary)] mr-2">Clusters:</span>
          <button
            onClick={() => setSelectedCluster('all')}
            className={`px-3 py-1 rounded-lg text-[11px] font-medium transition-colors ${
              selectedCluster === 'all'
                ? 'bg-[var(--accent)] text-white'
                : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--accent-subtle)]'
            }`}
          >
            All
          </button>
          {clustersInEnv.map(cluster => (
            <button
              key={cluster.name}
              onClick={() => setSelectedCluster(cluster.name)}
              className={`px-3 py-1 rounded-lg text-[11px] font-medium transition-colors ${
                selectedCluster === cluster.name
                  ? 'bg-[var(--accent)] text-white'
                  : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--accent-subtle)]'
              }`}
            >
              {cluster.name}
              <span className="ml-1 text-[9px] opacity-70">({cluster.resource_count})</span>
            </button>
          ))}
        </div>
      )}

      {/* Kind Tabs and Search */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {(['all', 'HelmRelease', 'Kustomization', 'Application'] as const).map(kind => (
            <button
              key={kind}
              onClick={() => setKindFilter(kind)}
              className={`px-3 py-1 rounded-lg text-[11px] font-medium transition-colors ${
                kindFilter === kind
                  ? 'bg-[var(--accent)] text-white'
                  : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--accent-subtle)]'
              }`}
            >
              {kind === 'all' ? 'All' : kind}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <input
            type="text"
            placeholder="Search services..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="text-[12px] px-3 py-1.5 border border-[var(--border)] rounded-lg bg-[var(--bg)] text-[var(--text-primary)] w-48"
          />
          <div className="flex items-center gap-1 border border-[var(--border)] rounded-lg p-0.5">
            <button
              onClick={() => setViewMode('tree')}
              className={`px-2 py-1 rounded text-[11px] ${viewMode === 'tree' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-tertiary)]'}`}
            >
              Tree
            </button>
            <button
              onClick={() => setViewMode('grid')}
              className={`px-2 py-1 rounded text-[11px] ${viewMode === 'grid' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-tertiary)]'}`}
            >
              Grid
            </button>
            <button
              onClick={() => setViewMode('table')}
              className={`px-2 py-1 rounded text-[11px] ${viewMode === 'table' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-tertiary)]'}`}
            >
              Table
            </button>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="flex items-center gap-4 text-[11px] text-[var(--text-tertiary)]">
        <span>{filteredResources.length} services</span>
        <span>·</span>
        <span>{filteredResources.filter(r => r.suspended).length} suspended</span>
        {selectedEnvironment !== 'all' && (
          <>
            <span>·</span>
            <span>Environment: {selectedEnvironment}</span>
          </>
        )}
        {selectedCluster !== 'all' && (
          <>
            <span>·</span>
            <span>Cluster: {selectedCluster}</span>
          </>
        )}
      </div>

      {/* Content */}
      {filteredResources.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">📦</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No services found</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            {resources.length === 0 ? 'Scan the repository to discover services' : 'Try adjusting your filters'}
          </p>
          {resources.length === 0 && (
            <button onClick={handleScan} disabled={scanning} className="btn btn-primary text-[12px]">
              {scanning ? 'Scanning...' : 'Scan Repository'}
            </button>
          )}
        </div>
      ) : viewMode === 'tree' ? (
        <div className="space-y-4">
          {/* Cluster Info Summary */}
          {clusterInfo.length > 0 && (
            <div className="card p-4">
              <h3 className="text-[12px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-3">
                Cluster Hierarchy
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {clusterInfo.map(cluster => (
                  <div key={cluster.name} className="bg-[var(--bg)] rounded-lg p-3">
                    <div className="flex items-center gap-2 mb-2">
                      <span className="text-lg">🌍</span>
                      <span className="text-[13px] font-semibold text-[var(--text-primary)]">{cluster.name}</span>
                      <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--accent-subtle)] text-[var(--accent)] font-medium">
                        {cluster.resource_count} resources
                      </span>
                    </div>
                    {cluster.sub_clusters && cluster.sub_clusters.length > 0 && (
                      <div className="ml-6 space-y-1">
                        {cluster.sub_clusters.map(sub => (
                          <div key={sub.name} className="flex items-center gap-2 text-[11px]">
                            <span className="text-[var(--text-tertiary)]">📍</span>
                            <span className="text-[var(--text-secondary)]">{sub.name}</span>
                            <span className="text-[var(--text-tertiary)]">({sub.resource_count})</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Tree View */}
          {tree && (
            <ResourceTree
              tree={tree}
              onResourceSelect={setSelectedResource}
              selectedResource={selectedResource}
            />
          )}
        </div>
      ) : viewMode === 'grid' ? (
        <div className="space-y-4">
          {Object.entries(groupedResources).map(([kind, items]) => (
            <div key={kind} className="border border-[var(--border)] rounded-lg overflow-hidden">
              <button
                onClick={() => toggleKindSection(kind)}
                className="w-full flex items-center gap-2 px-4 py-2.5 bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors text-left"
              >
                <span className={`text-[10px] text-[var(--text-tertiary)] transition-transform ${collapsedKinds.has(kind) ? '' : 'rotate-90'}`}>
                  ▶
                </span>
                <span className="text-sm">{kind === 'HelmRelease' ? '📦' : kind === 'Kustomization' ? '🔧' : '🚀'}</span>
                <span className="text-[12px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider">{kind}</span>
                <span className="text-[11px] text-[var(--text-tertiary)] font-normal">({items.length})</span>
              </button>
              {!collapsedKinds.has(kind) && (
                <div className="p-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {items.map((r, idx) => (
                      <ServiceCard
                        key={`${r.name}-${idx}`}
                        resource={r}
                        selected={selectedResource === r}
                        showCluster={environments.length > 1}
                        onSelect={setSelectedResource}
                        onSuspend={handleSuspendClick}
                        onResume={handleResumeClick}
                        onEdit={handleEditClick}
                      />
                    ))}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Name</th>
                <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Kind</th>
                <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Namespace</th>
                {environments.length > 1 && <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Environment</th>}
                {clustersInEnv.length > 0 && <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Cluster</th>}
                <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Status</th>
                <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filteredResources.map((r, idx) => (
                <tr key={`${r.name}-${idx}`} className="border-b border-[var(--border-light)] hover:bg-[var(--bg)]">
                  <td className="px-4 py-2 text-[12px] font-medium text-[var(--text-primary)]">{r.name}</td>
                  <td className="px-4 py-2 text-[11px] text-[var(--text-secondary)]">{r.kind}</td>
                  <td className="px-4 py-2 text-[11px] text-[var(--text-secondary)]">{r.namespace}</td>
                  {environments.length > 1 && <td className="px-4 py-2 text-[11px] text-[var(--text-secondary)]">{r.environment || '-'}</td>}
                  {clustersInEnv.length > 0 && <td className="px-4 py-2 text-[11px] text-[var(--text-secondary)]">{r.cluster || '-'}</td>}
                  <td className="px-4 py-2">
                    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                      r.suspended ? 'bg-[var(--border-light)] text-[var(--text-secondary)]' : 'bg-emerald-500/15 text-emerald-600'
                    }`}>
                      {r.suspended ? 'Suspended' : 'Active'}
                    </span>
                  </td>
                  <td className="px-4 py-2">
                    <div className="flex items-center gap-2">
                      {r.suspended ? (
                        <button onClick={() => handleResumeClick(r)} className="text-[11px] text-green-600 hover:underline">Resume</button>
                      ) : (
                        <button onClick={() => handleSuspendClick(r)} className="text-[11px] text-orange-600 hover:underline">Suspend</button>
                      )}
                      <button onClick={() => handleEditClick(r)} className="text-[11px] text-[var(--accent)] hover:underline">Edit</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* View / Edit Modal */}
      {editMode && selectedResource && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setEditMode(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-3xl mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <div>
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                  {editEnabled ? 'Edit' : 'View'}: {selectedResource.name}
                </h2>
                <p className="text-[11px] text-[var(--text-tertiary)]">{selectedResource.file_path}</p>
              </div>
              <div className="flex items-center gap-2">
                {!editEnabled && (
                  <button
                    onClick={() => setEditEnabled(true)}
                    className="btn text-[12px] px-3 py-1"
                  >
                    Edit YAML
                  </button>
                )}
                <button onClick={() => { setEditMode(false); setEditEnabled(false); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {commitResult && (
                <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-lg">
                  <p className="text-[12px] text-emerald-600 font-medium">Commit successful!</p>
                  <p className="text-[11px] text-emerald-500 font-mono">SHA: {commitResult.commit_sha?.substring(0, 8)}</p>
                </div>
              )}

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="label text-[11px]">YAML Content</label>
                  {!editEnabled && (
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--border-light)] text-[var(--text-tertiary)] font-medium">Read-only</span>
                  )}
                </div>
                <textarea
                  value={editedYaml}
                  onChange={e => setEditedYaml(e.target.value)}
                  readOnly={!editEnabled}
                  className={`w-full h-80 px-3 py-2 text-[11px] font-mono border rounded-lg resize-y focus:outline-none ${
                    editEnabled
                      ? 'bg-[var(--bg)] border-[var(--border)] text-[var(--text-primary)] focus:border-[var(--accent)]'
                      : 'bg-[var(--bg)] border-[var(--border-light)] text-[var(--text-secondary)] cursor-default'
                  }`}
                  spellCheck={false}
                />
              </div>

              {editEnabled && (
                <div>
                  <label className="label text-[11px]">Commit Message</label>
                  <input
                    type="text"
                    value={commitMessage}
                    onChange={e => setCommitMessage(e.target.value)}
                    placeholder={`gitops(PEPA): update ${selectedResource.kind}/${selectedResource.name}`}
                    className="input text-[12px]"
                  />
                </div>
              )}
            </div>

            {editEnabled && (
              <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
                <button onClick={() => { setEditMode(false); setEditEnabled(false); }} className="btn text-[12px]">Cancel</button>
                <button
                  onClick={handleSaveEdit}
                  disabled={committing || editedYaml === selectedResource.raw_yaml}
                  className="btn btn-primary text-[12px] disabled:opacity-50"
                >
                  {committing ? 'Saving...' : 'Save & Commit'}
                </button>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Suspend/Resume Modal */}
      {suspendModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setSuspendModal(null)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-sm mx-4">
            <div className="p-5">
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)] text-center mb-1">
                {suspendModal.suspend ? 'Suspend Service' : 'Resume Service'}
              </h3>
              <p className="text-[13px] text-[var(--text-secondary)] text-center">
                Are you sure you want to {suspendModal.suspend ? 'suspend' : 'resume'} {suspendModal.resource.kind}/{suspendModal.resource.name}?
              </p>
              <div className="mt-3">
                <label className="label text-[11px]">Commit Message</label>
                <input
                  type="text"
                  value={suspendCommitMsg}
                  onChange={e => setSuspendCommitMsg(e.target.value)}
                  className="input text-[12px]"
                />
              </div>
            </div>
            <div className="flex items-center gap-2 px-5 py-3 bg-[var(--bg)] border-t border-[var(--border-light)]">
              <button onClick={() => setSuspendModal(null)} disabled={suspending} className="btn btn-secondary flex-1 justify-center text-[12px]">
                Cancel
              </button>
              <button
                onClick={handleSuspendConfirm}
                disabled={suspending}
                className={`flex-1 justify-center text-[12px] font-medium rounded-md px-3 py-1.5 ${
                  suspendModal.suspend
                    ? 'bg-orange-500 text-white hover:bg-orange-600'
                    : 'btn btn-primary'
                }`}
              >
                {suspending ? 'Processing...' : suspendModal.suspend ? 'Suspend' : 'Resume'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create Modal Placeholder */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowCreateModal(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4">
            <div className="px-5 py-3 border-b border-[var(--border)]">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create New Service</h2>
            </div>
            <div className="px-5 py-4 text-center text-[var(--text-tertiary)] text-[12px]">
              Create service form coming soon...
            </div>
            <div className="flex justify-end px-5 py-3 border-t border-[var(--border)]">
              <button onClick={() => setShowCreateModal(false)} className="btn text-[12px]">Close</button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
