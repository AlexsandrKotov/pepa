'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRouter } from 'next/navigation';
import { gitops, connections as connectionsAPI, type GitopsRepo, type GitopsResource, type Connection } from '@/lib/api';
import GitRepoPicker, { type GitRepoPickerValue } from '@/components/GitRepoPicker';

const defaultForm = {
  name: '',
  repo_url: '',
  branch: 'main',
  path: '.',
  engine_type: 'auto',
  connection_id: '',
  token: '',
};

function scanStatusBadge(status: string) {
  const styles: Record<string, string> = {
    pending: 'bg-[var(--border-light)] text-[var(--text-secondary)]',
    scanning: 'bg-blue-500/15 text-blue-500 animate-pulse',
    ready: 'bg-emerald-500/15 text-emerald-600',
    error: 'bg-red-500/15 text-red-500',
  };
  const labels: Record<string, string> = {
    pending: 'Pending',
    scanning: 'Scanning...',
    ready: 'Ready',
    error: 'Error',
  };
  return (
    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[status] || styles.pending}`}>
      {labels[status] || status}
    </span>
  );
}

function engineBadge(engine: string) {
  const styles: Record<string, string> = {
    fluxcd: 'bg-purple-500/15 text-purple-600',
    argocd: 'bg-orange-500/15 text-orange-600',
    auto: 'bg-[var(--border-light)] text-[var(--text-secondary)]',
    unknown: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  };
  const labels: Record<string, string> = {
    fluxcd: 'FluxCD',
    argocd: 'ArgoCD',
    auto: 'Auto-detect',
    unknown: 'Unknown',
  };
  return (
    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${styles[engine] || styles.unknown}`}>
      {labels[engine] || engine}
    </span>
  );
}

function kindIcon(kind: string) {
  switch (kind) {
    case 'HelmRelease': return '📦';
    case 'Kustomization': return '🔧';
    case 'Application': return '🚀';
    case 'HelmRepository': return '📋';
    case 'GitRepository': return '🔀';
    default: return '📄';
  }
}

export default function GitOpsPage() {
  const router = useRouter();
  const [repos, setRepos] = useState<GitopsRepo[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<GitopsRepo | null>(null);
  const [form, setForm] = useState({ ...defaultForm });
  const [error, setError] = useState('');
  const [scanning, setScanning] = useState<string | null>(null);
  const [gitInputMode, setGitInputMode] = useState<'picker' | 'manual'>('picker');
  const [gitConnections, setGitConnections] = useState<Connection[]>([]);

  useEscapeKey(() => {
    if (showForm) setShowForm(false);
  }, showForm);

  // Resource viewer state
  const [selectedRepo, setSelectedRepo] = useState<GitopsRepo | null>(null);
  const [resources, setResources] = useState<GitopsResource[]>([]);
  const [loadingResources, setLoadingResources] = useState(false);
  const [selectedResource, setSelectedResource] = useState<GitopsResource | null>(null);
  const [resourceFilter, setResourceFilter] = useState('');

  // YAML Editor state
  const [editMode, setEditMode] = useState(false);
  const [editedYaml, setEditedYaml] = useState('');
  const [commitMessage, setCommitMessage] = useState('');
  const [committing, setCommitting] = useState(false);
  const [commitResult, setCommitResult] = useState<{ commit_sha: string; branch: string; diff: string; mr_needed: boolean; mr_url?: string } | null>(null);
  const [previewDiff, setPreviewDiff] = useState('');
  const [showDiffPreview, setShowDiffPreview] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await gitops.listRepos();
      setRepos(res.repos || []);
    } catch { /* ignore */ }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  // Load git connections for the repo picker
  useEffect(() => {
    (async () => {
      try {
        const data = await connectionsAPI.list();
        const gitConns = (data.connections || data || []).filter(
          (c: Connection) => c.type === 'git' || c.type === 'gitlab'
        );
        setGitConnections(gitConns);
      } catch { /* ignore */ }
    })();
  }, []);

  const openCreate = () => {
    setEditing(null);
    setForm({ ...defaultForm });
    setError('');
    setGitInputMode(gitConnections.length > 0 ? 'picker' : 'manual');
    setShowForm(true);
  };

  const openEdit = (r: GitopsRepo) => {
    setEditing(r);
    setForm({
      name: r.name,
      repo_url: r.repo_url,
      branch: r.branch,
      path: r.path,
      engine_type: r.engine_type,
      connection_id: r.connection_id || '',
      token: '',
    });
    setError('');
    setGitInputMode('manual');
    setShowForm(true);
  };

  const handleSave = async () => {
    setError('');
    if (!form.name || !form.repo_url) {
      setError('Name and Repository URL are required');
      return;
    }
    try {
      const data: Record<string, string> = {
        name: form.name,
        repo_url: form.repo_url,
        branch: form.branch,
        path: form.path,
        engine_type: form.engine_type,
      };
      if (form.connection_id) data.connection_id = form.connection_id;
      if (form.token) data.token = form.token;

      if (editing) {
        await gitops.updateRepo(editing.id, data);
      } else {
        await gitops.createRepo(data as Parameters<typeof gitops.createRepo>[0]);
      }
      setShowForm(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Remove this GitOps repository?')) return;
    try {
      await gitops.deleteRepo(id);
      if (selectedRepo?.id === id) {
        setSelectedRepo(null);
        setResources([]);
        setSelectedResource(null);
      }
      load();
    } catch { /* ignore */ }
  };

  const handleScan = async (repo: GitopsRepo) => {
    setScanning(repo.id);
    try {
      await gitops.scanRepo(repo.id);
      await load();
      // If this repo is selected, refresh resources
      if (selectedRepo?.id === repo.id) {
        await loadResources(repo);
      }
    } catch { /* ignore */ }
    setScanning(null);
  };

  const loadResources = async (repo: GitopsRepo) => {
    setSelectedRepo(repo);
    setSelectedResource(null);
    setEditMode(false);
    setCommitResult(null);
    setPreviewDiff('');
    setShowDiffPreview(false);
    setLoadingResources(true);
    try {
      const res = await gitops.listResources(repo.id);
      setResources(res.resources || []);
    } catch {
      setResources([]);
    }
    setLoadingResources(false);
  };

  const filteredResources = resourceFilter
    ? resources.filter(r =>
        r.kind.toLowerCase().includes(resourceFilter.toLowerCase()) ||
        r.name.toLowerCase().includes(resourceFilter.toLowerCase()) ||
        r.namespace.toLowerCase().includes(resourceFilter.toLowerCase())
      )
    : resources;

  // Generate suggested commit message based on resource changes
  const generateSuggestedCommitMessage = (resource: GitopsResource, originalYaml: string, newYaml: string): string => {
    const changes: string[] = [];
    
    // Simple diff detection
    if (originalYaml !== newYaml) {
      // Check for common changes
      if (newYaml.includes('replicas:') && !originalYaml.includes('replicas:')) {
        changes.push('replica count');
      }
      if (newYaml.includes('image:') && originalYaml.includes('image:')) {
        const oldImage = originalYaml.match(/image:\s*(.+)/)?.[1]?.trim();
        const newImage = newYaml.match(/image:\s*(.+)/)?.[1]?.trim();
        if (oldImage !== newImage) {
          changes.push('image tag');
        }
      }
      if (newYaml.includes('version:') && originalYaml.includes('version:')) {
        const oldVersion = originalYaml.match(/version:\s*(.+)/)?.[1]?.trim();
        const newVersion = newYaml.match(/version:\s*(.+)/)?.[1]?.trim();
        if (oldVersion !== newVersion) {
          changes.push('chart version');
        }
      }
    }

    const changeDesc = changes.length > 0 ? changes.join(', ') : 'values';
    return `gitops(PEPA): update ${changeDesc} in ${resource.kind}/${resource.name}`;
  };

  const handleEditYaml = () => {
    if (selectedResource?.raw_yaml) {
      setEditedYaml(selectedResource.raw_yaml);
      setEditMode(true);
      setCommitResult(null);
      setPreviewDiff('');
      setShowDiffPreview(false);
    }
  };

  const handleCancelEdit = () => {
    setEditMode(false);
    setEditedYaml('');
    setCommitMessage('');
    setPreviewDiff('');
    setShowDiffPreview(false);
  };

  const handlePreviewDiff = async () => {
    if (!selectedRepo || !selectedResource) return;
    try {
      const res = await gitops.previewDiff(selectedRepo.id, selectedResource.name, {
        file_path: selectedResource.file_path,
        full_yaml: editedYaml,
      });
      setPreviewDiff(res.diff);
      setShowDiffPreview(true);
    } catch (err) {
      console.error('Failed to preview diff:', err);
    }
  };

  const handleCommit = async () => {
    if (!selectedRepo || !selectedResource) return;
    setCommitting(true);
    try {
      const msg = commitMessage || generateSuggestedCommitMessage(selectedResource, selectedResource.raw_yaml || '', editedYaml);
      const res = await gitops.editValues(selectedRepo.id, selectedResource.name, {
        file_path: selectedResource.file_path,
        full_yaml: editedYaml,
        commit_message: msg,
      });
      setCommitResult(res);
      setEditMode(false);
      // Refresh resources to show updated state
      await loadResources(selectedRepo);
    } catch (err) {
      console.error('Failed to commit:', err);
    }
    setCommitting(false);
  };

  const groupedResources = filteredResources.reduce<Record<string, GitopsResource[]>>((acc, r) => {
    (acc[r.kind] ||= []).push(r);
    return acc;
  }, {});

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">GitOps</h1>
          <p className="page-subtitle-modern">Bind manifest repositories to discover and manage FluxCD and ArgoCD resources</p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => router.push('/gitops/drift')} className="btn btn-secondary text-[12px]">
            Drift Detection
          </button>
          <button onClick={openCreate} className="btn btn-primary">+ Add Repository</button>
        </div>
      </div>

      {/* Info */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
        <p className="text-[13px] text-blue-500">
          <span className="font-medium">GitOps Repositories</span> connect PEPA to your manifest repositories.
          Scan for FluxCD HelmReleases, Kustomizations, and ArgoCD Applications, then edit values and track deployments.
        </p>
      </div>

      {/* Repo Grid */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      ) : repos.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">🔀</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No GitOps repositories configured</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Add a manifest repository to discover FluxCD and ArgoCD resources
          </p>
          <button onClick={openCreate} className="btn btn-primary">+ Add First Repository</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {repos.map(r => (
            <div
              key={r.id}
              className={`card p-5 hover:border-[var(--accent)] transition-colors group cursor-pointer ${selectedRepo?.id === r.id ? 'border-[var(--accent)] ring-1 ring-[var(--accent)]' : ''}`}
              onClick={() => router.push(`/gitops/repos?id=${r.id}`)}
            >
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span className="text-xl">🔀</span>
                  <div>
                    <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{r.name}</h3>
                    <span className="text-[10px] font-mono text-[var(--text-tertiary)] truncate block max-w-[180px]">{r.repo_url}</span>
                  </div>
                </div>
                {scanStatusBadge(r.scan_status)}
              </div>

              <div className="space-y-1.5 mb-4">
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-14">Branch:</span>
                  <span className="font-mono text-[var(--text-secondary)]">{r.branch}</span>
                </div>
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-14">Path:</span>
                  <span className="font-mono text-[var(--text-secondary)]">{r.path}</span>
                </div>
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-14">Engine:</span>
                  {engineBadge(r.engine_type)}
                </div>
                {r.scan_error && (
                  <div className="text-[10px] text-red-500 mt-1 truncate" title={r.scan_error}>
                    Error: {r.scan_error}
                  </div>
                )}
                {r.last_scanned_at && (
                  <div className="text-[10px] text-[var(--text-tertiary)]">
                    Last scanned: {new Date(r.last_scanned_at).toLocaleString()}
                  </div>
                )}
              </div>

              <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]" onClick={e => e.stopPropagation()}>
                <button
                  onClick={() => handleScan(r)}
                  disabled={scanning === r.id}
                  className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50"
                >
                  {scanning === r.id ? 'Scanning...' : 'Scan'}
                </button>
                <button onClick={() => router.push(`/gitops/repos?id=${r.id}`)} className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">
                  Manage
                </button>
                <button onClick={() => openEdit(r)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--bg)] rounded-lg transition-colors">Edit</button>
                <button onClick={() => handleDelete(r.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Resources Panel */}
      {selectedRepo && (
        <div className="card border border-[var(--border)] rounded-xl overflow-hidden">
          <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] bg-[var(--surface)]">
            <div className="flex items-center gap-3">
              <h2 className="text-[14px] font-semibold text-[var(--text-primary)]">
                Resources: {selectedRepo.name}
              </h2>
              {engineBadge(selectedRepo.engine_type)}
              <span className="text-[11px] text-[var(--text-tertiary)]">{resources.length} resources</span>
            </div>
            <div className="flex items-center gap-2">
              <input
                type="text"
                placeholder="Filter..."
                value={resourceFilter}
                onChange={e => setResourceFilter(e.target.value)}
                className="text-[12px] px-2.5 py-1 border border-[var(--border)] rounded-lg bg-[var(--bg)] text-[var(--text-primary)] w-48"
              />
              <button
                onClick={() => handleScan(selectedRepo)}
                disabled={scanning === selectedRepo.id}
                className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50"
              >
                {scanning === selectedRepo.id ? 'Scanning...' : 'Re-scan'}
              </button>
              <button onClick={() => { setSelectedRepo(null); setResources([]); setSelectedResource(null); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-lg">&times;</button>
            </div>
          </div>

          <div className="flex">
            {/* Resource List */}
            <div className={`${selectedResource ? 'w-1/2 border-r border-[var(--border)]' : 'w-full'} transition-all`}>
              {loadingResources ? (
                <div className="p-8 text-center text-[13px] text-[var(--text-tertiary)]">Loading resources...</div>
              ) : Object.keys(groupedResources).length === 0 ? (
                <div className="p-8 text-center">
                  <p className="text-[13px] text-[var(--text-tertiary)]">
                    {resources.length === 0 ? 'No resources found. Click Scan to discover manifests.' : 'No resources match the filter.'}
                  </p>
                </div>
              ) : (
                <div className="divide-y divide-[var(--border-light)]">
                  {Object.entries(groupedResources).map(([kind, items]) => (
                    <div key={kind}>
                      <div className="px-4 py-2 bg-[var(--bg)] text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider flex items-center gap-2">
                        <span>{kindIcon(kind)}</span>
                        <span>{kind}</span>
                        <span className="text-[var(--text-tertiary)] font-normal">({items.length})</span>
                      </div>
                      {items.map((r, idx) => (
                        <button
                          key={`${r.name}-${idx}`}
                          onClick={() => { setSelectedResource(r); setEditMode(false); setCommitResult(null); setPreviewDiff(''); setShowDiffPreview(false); }}
                          className={`w-full text-left px-4 py-2.5 hover:bg-[var(--bg)] transition-colors border-b border-[var(--border-light)] ${selectedResource === r ? 'bg-[var(--accent-subtle)]' : ''}`}
                        >
                          <div className="flex items-center justify-between">
                            <div>
                              <span className="text-[12px] font-medium text-[var(--text-primary)]">{r.name}</span>
                              {r.namespace && (
                                <span className="text-[10px] text-[var(--text-tertiary)] ml-2">ns: {r.namespace}</span>
                              )}
                            </div>
                            {r.chart && (
                              <span className="text-[10px] font-mono text-[var(--text-tertiary)]">
                                {r.chart}{r.version ? `@${r.version}` : ''}
                              </span>
                            )}
                          </div>
                          <div className="text-[10px] text-[var(--text-tertiary)] font-mono mt-0.5">{r.file_path}</div>
                        </button>
                      ))}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Resource Detail */}
            {selectedResource && (
              <div className="w-1/2 overflow-y-auto max-h-[600px]">
                <div className="px-5 py-3 border-b border-[var(--border-light)] flex items-center justify-between sticky top-0 bg-[var(--surface)] z-10">
                  <div>
                    <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{selectedResource.name}</h3>
                    <span className="text-[10px] text-[var(--text-tertiary)]">{selectedResource.kind} &middot; {selectedResource.api_version}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    {!editMode && selectedResource.raw_yaml && (
                      <button
                        onClick={handleEditYaml}
                        className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors"
                      >
                        Edit YAML
                      </button>
                    )}
                    <button onClick={() => { setSelectedResource(null); setEditMode(false); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-lg">&times;</button>
                  </div>
                </div>

                {/* Commit Result Banner */}
                {commitResult && (
                  <div className="mx-5 mt-4 p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-lg">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-emerald-600">✓</span>
                      <span className="text-[12px] font-medium text-emerald-600">Commit successful</span>
                    </div>
                    <div className="text-[11px] text-emerald-500 space-y-0.5">
                      <div>SHA: <span className="font-mono">{commitResult.commit_sha?.substring(0, 8)}</span></div>
                      <div>Branch: <span className="font-mono">{commitResult.branch}</span></div>
                      {commitResult.mr_needed && (
                        <div className="text-orange-500">MR needed: {commitResult.mr_url}</div>
                      )}
                    </div>
                    <button
                      onClick={() => setCommitResult(null)}
                      className="text-[10px] text-emerald-500 hover:text-emerald-600 mt-2"
                    >
                      Dismiss
                    </button>
                  </div>
                )}

                {/* YAML Editor */}
                {editMode ? (
                  <div className="px-5 py-4 space-y-4">
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider">Edit YAML</h4>
                        <button
                          onClick={handleCancelEdit}
                          className="text-[10px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
                        >
                          Cancel
                        </button>
                      </div>
                      <textarea
                        value={editedYaml}
                        onChange={e => {
                          setEditedYaml(e.target.value);
                          // Update suggested commit message when content changes
                          if (selectedResource && !commitMessage) {
                            setCommitMessage(generateSuggestedCommitMessage(selectedResource, selectedResource.raw_yaml || '', e.target.value));
                          }
                        }}
                        className="w-full h-64 px-3 py-2 text-[11px] font-mono bg-[var(--bg)] border border-[var(--border)] rounded-lg text-[var(--text-primary)] resize-y focus:outline-none focus:border-[var(--accent)]"
                        spellCheck={false}
                      />
                    </div>

                    {/* Diff Preview */}
                    {showDiffPreview && previewDiff && (
                      <div>
                        <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Changes Preview</h4>
                        <pre className="bg-[var(--bg)] rounded-lg p-3 text-[10px] font-mono text-[var(--text-secondary)] overflow-x-auto max-h-48 overflow-y-auto whitespace-pre-wrap border border-[var(--border)]">
                          {previewDiff}
                        </pre>
                      </div>
                    )}

                    {/* Commit Message */}
                    <div>
                      <label className="label text-[11px]">Commit Message</label>
                      <input
                        type="text"
                        value={commitMessage}
                        onChange={e => setCommitMessage(e.target.value)}
                        placeholder={selectedResource ? generateSuggestedCommitMessage(selectedResource, selectedResource.raw_yaml || '', editedYaml) : 'gitops(PEPA): update resource'}
                        className="input text-[12px]"
                      />
                      <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                        Leave empty to use suggested message. In the future, AI will generate contextual messages.
                      </p>
                    </div>

                    {/* Action Buttons */}
                    <div className="flex items-center gap-2 pt-2 border-t border-[var(--border-light)]">
                      <button
                        onClick={handlePreviewDiff}
                        disabled={editedYaml === selectedResource.raw_yaml}
                        className="text-[11px] px-3 py-1.5 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        Preview Changes
                      </button>
                      <button
                        onClick={handleCommit}
                        disabled={committing || editedYaml === selectedResource.raw_yaml}
                        className="btn btn-primary text-[11px] px-3 py-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {committing ? 'Committing...' : 'Save & Commit'}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="px-5 py-4 space-y-4">
                  {/* Summary */}
                  <div className="space-y-2">
                    <DetailRow label="Kind" value={selectedResource.kind} />
                    <DetailRow label="Name" value={selectedResource.name} />
                    {selectedResource.namespace && <DetailRow label="Namespace" value={selectedResource.namespace} />}
                    <DetailRow label="File" value={selectedResource.file_path} mono />
                    {selectedResource.chart && <DetailRow label="Chart" value={`${selectedResource.chart}${selectedResource.version ? ` @ ${selectedResource.version}` : ''}`} />}
                    {selectedResource.repo && <DetailRow label="Source Ref" value={selectedResource.repo} />}
                  </div>

                  {/* ArgoCD Source */}
                  {selectedResource.source && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">ArgoCD Source</h4>
                      <div className="bg-[var(--bg)] rounded-lg p-3 space-y-1.5">
                        <DetailRow label="Repo" value={selectedResource.source.repo_url} mono />
                        <DetailRow label="Path" value={selectedResource.source.path} mono />
                        <DetailRow label="Target" value={selectedResource.source.target_revision} />
                        {selectedResource.source.helm?.value_files && (
                          <DetailRow label="Value Files" value={selectedResource.source.helm.value_files.join(', ')} mono />
                        )}
                      </div>
                    </div>
                  )}

                  {/* ArgoCD Destination */}
                  {selectedResource.dest && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Destination</h4>
                      <div className="bg-[var(--bg)] rounded-lg p-3 space-y-1.5">
                        <DetailRow label="Server" value={selectedResource.dest.server} mono />
                        <DetailRow label="Namespace" value={selectedResource.dest.namespace} />
                      </div>
                    </div>
                  )}

                  {/* Values From */}
                  {selectedResource.values_from && selectedResource.values_from.length > 0 && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Values From</h4>
                      <div className="space-y-1">
                        {selectedResource.values_from.map((vf, i) => (
                          <div key={i} className="bg-[var(--bg)] rounded-lg px-3 py-2 text-[11px]">
                            <span className="font-medium text-[var(--text-primary)]">{vf.kind}</span>
                            <span className="text-[var(--text-secondary)]"> / {vf.name}</span>
                            {vf.values_key && <span className="text-[var(--text-tertiary)]"> (key: {vf.values_key})</span>}
                            {vf.target_path && <span className="text-[var(--text-tertiary)]"> → {vf.target_path}</span>}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Depends On */}
                  {selectedResource.depends_on && selectedResource.depends_on.length > 0 && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Dependencies</h4>
                      <div className="flex flex-wrap gap-1.5">
                        {selectedResource.depends_on.map((dep, i) => (
                          <span key={i} className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--border-light)] text-[var(--text-secondary)] font-mono">{dep}</span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Labels */}
                  {selectedResource.labels && Object.keys(selectedResource.labels).length > 0 && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Labels</h4>
                      <div className="flex flex-wrap gap-1.5">
                        {Object.entries(selectedResource.labels).map(([k, v]) => (
                          <span key={k} className="text-[10px] px-2 py-0.5 rounded-full bg-blue-500/10 text-blue-500 font-mono">{k}={v}</span>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Inline Values */}
                  {selectedResource.values && Object.keys(selectedResource.values).length > 0 && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Inline Values</h4>
                      <pre className="bg-[var(--bg)] rounded-lg p-3 text-[11px] font-mono text-[var(--text-secondary)] overflow-x-auto max-h-64 overflow-y-auto">
                        {JSON.stringify(selectedResource.values, null, 2)}
                      </pre>
                    </div>
                  )}

                  {/* Raw YAML */}
                  {selectedResource.raw_yaml && (
                    <div>
                      <h4 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider mb-2">Raw YAML</h4>
                      <pre className="bg-[var(--bg)] rounded-lg p-3 text-[11px] font-mono text-[var(--text-secondary)] overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap">
                        {selectedResource.raw_yaml}
                      </pre>
                    </div>
                  )}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Create/Edit Modal */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                {editing ? 'Edit GitOps Repository' : 'Add GitOps Repository'}
              </h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
              )}

              <div>
                <label className="label">Name *</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="production-manifests" />
              </div>

              <div>
                <label className="label">Repository *</label>
                <div className="flex gap-1.5 mb-2">
                  {gitConnections.length > 0 && (
                    <button
                      type="button"
                      onClick={() => setGitInputMode('picker')}
                      className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${
                        gitInputMode === 'picker' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'
                      }`}
                    >
                      Browse
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => setGitInputMode('manual')}
                    className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${
                      gitInputMode === 'manual' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'
                    }`}
                  >
                    Manual URL
                  </button>
                </div>

                {gitInputMode === 'picker' ? (
                  <GitRepoPicker
                    label=""
                    showBranch
                    gitConnections={gitConnections}
                    value={{ repo_url: form.repo_url, connection_id: form.connection_id }}
                    onChange={(v: GitRepoPickerValue) => {
                      setForm(f => ({
                        ...f,
                        repo_url: v.repo_url,
                        connection_id: v.connection_id,
                        branch: v.branch || f.branch,
                      }));
                    }}
                  />
                ) : (
                  <>
                    <input
                      value={form.repo_url}
                      onChange={e => setForm({ ...form, repo_url: e.target.value })}
                      className="input font-mono text-[12px]"
                      placeholder="https://github.com/org/flux-manifests.git"
                    />
                    <div className="space-y-3 mt-3">
                      <p className="text-[11px] text-[var(--text-tertiary)]">Authentication (for private repositories)</p>
                      <div>
                        <label className="label text-[11px]">Access Token</label>
                        <input
                          type="password"
                          value={form.token}
                          onChange={e => setForm({ ...form, token: e.target.value })}
                          className="input text-[12px]"
                          placeholder={editing ? '(unchanged)' : 'glpat-xxx or github_pat_xxx'}
                        />
                      </div>
                    </div>
                  </>
                )}
                {gitInputMode === 'picker' && form.connection_id && (
                  <p className="text-[11px] text-green-600 mt-1">
                    ✓ Authentication from selected git connection
                  </p>
                )}
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Branch</label>
                  <input value={form.branch} onChange={e => setForm({ ...form, branch: e.target.value })} className="input font-mono text-[12px]" placeholder="main" />
                </div>
                <div>
                  <label className="label">Path</label>
                  <input value={form.path} onChange={e => setForm({ ...form, path: e.target.value })} className="input font-mono text-[12px]" placeholder="./clusters/production" />
                </div>
              </div>

              <div>
                <label className="label">Engine Type</label>
                <div className="grid grid-cols-3 gap-2">
                  {([
                    { value: 'auto', label: 'Auto', desc: 'Detect automatically' },
                    { value: 'fluxcd', label: 'FluxCD', desc: 'Flux v2 CRDs' },
                    { value: 'argocd', label: 'ArgoCD', desc: 'Application CRD' },
                  ] as const).map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => setForm({ ...form, engine_type: opt.value })}
                      className={`p-2.5 rounded-lg border text-left transition-all ${
                        form.engine_type === opt.value
                          ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                          : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                      }`}
                    >
                      <div className="text-[12px] font-medium">{opt.label}</div>
                      <div className="text-[10px] text-[var(--text-tertiary)]">{opt.desc}</div>
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn text-[12px]">Cancel</button>
              <button onClick={handleSave} className="btn btn-primary text-[12px]">
                {editing ? 'Save Changes' : 'Add Repository'}
              </button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-start gap-2 text-[11px]">
      <span className="text-[var(--text-tertiary)] w-20 shrink-0">{label}:</span>
      <span className={`text-[var(--text-secondary)] break-all ${mono ? 'font-mono' : ''}`}>{value}</span>
    </div>
  );
}
