'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { helmRepositories, type HelmRepository, type HelmChart, type HelmChartVersion } from '@/lib/api';
import { VaultInput, VaultPickerModal, useVaultPicker } from '@/components/VaultInput';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

const defaultForm = {
  name: '', description: '', repo_type: 'http' as HelmRepository['repo_type'],
  url: '', username: '', password: '', token: '', ssh_key: '', ca_cert: '',
  is_default: false,
};

export default function HelmRepositoriesPage() {
  const { isAdmin, hasPermission, loading: permLoading } = usePermission();
  const [repos, setRepos] = useState<HelmRepository[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<HelmRepository | null>(null);
  const [form, setForm] = useState({ ...defaultForm });
  const [error, setError] = useState('');
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();
  // Chart browser state
  const [browseRepo, setBrowseRepo] = useState<HelmRepository | null>(null);
  const [charts, setCharts] = useState<HelmChart[]>([]);
  const [chartError, setChartError] = useState<string>('');
  const [loadingCharts, setLoadingCharts] = useState(false);
  const [selectedChart, setSelectedChart] = useState<HelmChart | null>(null);
  const [chartVersions, setChartVersions] = useState<HelmChartVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);

  useEscapeKey(() => {
    if (selectedChart) { setSelectedChart(null); setChartVersions([]); }
    else if (browseRepo) setBrowseRepo(null);
    else if (showForm) { setShowForm(false); setEditing(null); }
  }, showForm || browseRepo !== null || selectedChart !== null);

  const load = async () => {
    try {
      const res = await helmRepositories.list();
      setRepos(res.helm_repositories || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  useEffect(() => { if (isAdmin) load(); }, [isAdmin]);

  const openCreate = () => {
    setEditing(null);
    setForm({ ...defaultForm });
    setError('');
    setShowForm(true);
  };

  const openEdit = (r: HelmRepository) => {
    setEditing(r);
    setForm({
      name: r.name, description: r.description, repo_type: r.repo_type,
      url: r.url, username: r.username,
      password: '', token: '', ssh_key: '', ca_cert: '',
      is_default: r.is_default,
    });
    setError('');
    setShowForm(true);
  };

  const handleSave = async () => {
    setError('');
    try {
      // Merge vault references into form data
      const merged = { ...form };
      for (const [field, ref] of Object.entries(vaultRefs)) {
        if (ref) (merged as Record<string, unknown>)[field] = ref;
      }
      if (editing) {
        await helmRepositories.update(editing.id, merged);
      } else {
        await helmRepositories.create(merged);
      }
      setShowForm(false);
      setVaultRefs({});
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this Helm repository?')) return;
    try {
      await helmRepositories.delete(id);
      load();
    } catch { /* ignore */ }
  };

  const openChartBrowser = async (repo: HelmRepository) => {
    setBrowseRepo(repo);
    setCharts([]);
    setSelectedChart(null);
    setChartVersions([]);
    setChartError('');
    setLoadingCharts(true);
    try {
      const data = await helmRepositories.listCharts(repo.id);
      setCharts(data.charts || []);
    } catch (err) {
      console.error('Failed to load charts:', err);
      setChartError(err instanceof Error ? err.message : 'Failed to load charts');
    } finally {
      setLoadingCharts(false);
    }
  };

  const selectChart = async (chart: HelmChart) => {
    if (!browseRepo) return;
    setSelectedChart(chart);
    setChartVersions([]);
    setLoadingVersions(true);
    try {
      const data = await helmRepositories.listChartVersions(browseRepo.id, chart.name);
      setChartVersions(data.versions || []);
    } catch (err) {
      console.error('Failed to load versions:', err);
    } finally {
      setLoadingVersions(false);
    }
  };

  const repoTypeLabel = (t: string) => {
    switch (t) {
      case 'git': return '🔀 Git';
      case 'http': return '🌐 HTTP';
      case 'oci': return '📦 OCI';
      default: return t;
    }
  };

  const repoTypeIcon = (t: string) => {
    switch (t) {
      case 'git': return '🔀';
      case 'http': return '🌐';
      case 'oci': return '📦';
      default: return '📋';
    }
  };

  if (permLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('helm', 'read')) {
    return <ForbiddenPage resource="helm_repositories" />;
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Helm Repositories</h1>
          <p className="page-subtitle-modern">Configure chart registries with credentials for private Helm charts</p>
        </div>
        <button onClick={openCreate} className="btn btn-primary">+ Add Repository</button>
      </div>

      {/* Info */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up page-delay-1">
        <p className="text-[13px] text-blue-500">
          <span className="font-medium">Helm Repositories</span> store connection details and credentials for your chart registries.
          Once configured, select a repository when creating <span className="font-medium">Blueprints</span> and just specify the chart name or path.
        </p>
      </div>

      {/* Grid */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      ) : repos.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">📋</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No Helm repositories configured</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Add chart registries (Git, HTTP, OCI) with credentials for private charts
          </p>
          <button onClick={openCreate} className="btn btn-primary">+ Add First Repository</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 page-animate-up page-delay-2">
          {repos.map(r => (
            <div key={r.id} className="card p-5 hover:border-[var(--accent)] transition-colors group modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{repoTypeIcon(r.repo_type)}</span>
                  <div>
                    <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{r.name}</h3>
                    <span className="text-[10px] text-[var(--text-tertiary)]">{repoTypeLabel(r.repo_type)}</span>
                  </div>
                </div>
                {r.is_default && (
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-blue-500/10 text-blue-500 font-medium">default</span>
                )}
              </div>

              {r.description && (
                <p className="text-[12px] text-[var(--text-secondary)] mb-3 line-clamp-2">{r.description}</p>
              )}

              <div className="space-y-1.5 mb-4">
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-12">URL:</span>
                  <span className="font-mono text-[var(--text-secondary)] truncate">{r.url}</span>
                </div>
                {r.username && (
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-12">Auth:</span>
                    <span className="text-[var(--text-secondary)]">{r.username} · credentials set</span>
                  </div>
                )}
              </div>

              <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                <button onClick={() => openChartBrowser(r)} className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">Browse Charts</button>
                <button onClick={() => openEdit(r)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--bg)] rounded-lg transition-colors">Edit</button>
                <button onClick={() => handleDelete(r.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create/Edit Modal */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                {editing ? 'Edit Repository' : 'Add Helm Repository'}
              </h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
              )}

              <div>
                <label className="label">Name *</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="my-private-charts" />
              </div>

              <div>
                <label className="label">Description</label>
                <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="Company Helm chart repository" />
              </div>

              <div>
                <label className="label">Repository Type</label>
                <div className="grid grid-cols-3 gap-2">
                  {([
                    { value: 'git', label: '🔀 Git', desc: 'Git repository' },
                    { value: 'http', label: '🌐 HTTP', desc: 'Chart museum' },
                    { value: 'oci', label: '📦 OCI', desc: 'OCI registry' },
                  ] as const).map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => setForm({ ...form, repo_type: opt.value })}
                      className={`p-2.5 rounded-lg border text-left transition-all ${
                        form.repo_type === opt.value
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

              <div>
                <label className="label">Repository URL *</label>
                <input
                  value={form.url}
                  onChange={e => setForm({ ...form, url: e.target.value })}
                  className="input font-mono text-[12px]"
                  placeholder={
                    form.repo_type === 'git' ? 'https://github.com/org/charts-repo' :
                    form.repo_type === 'oci' ? 'oci://registry.example.com/charts' :
                    'https://charts.example.com'
                  }
                />
              </div>

              {/* Credentials */}
              <div className="space-y-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Authentication (for private repositories)</p>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="label text-[11px]">Username</label>
                    <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} className="input text-[12px]" placeholder="admin" />
                  </div>
                  <VaultInput
                    label="Password"
                    field="password"
                    value={form.password}
                    onChange={v => setForm({ ...form, password: v })}
                    vaultRef={vaultRefs.password}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder={editing ? '(unchanged)' : 'password'}
                  />
                </div>

                <VaultInput
                  label="Token (alternative to username/password)"
                  field="token"
                  value={form.token}
                  onChange={v => setForm({ ...form, token: v })}
                  vaultRef={vaultRefs.token}
                  onOpenVault={onOpenVaultPicker}
                  onRemoveVault={removeVaultRef}
                  placeholder={editing ? '(unchanged)' : 'ghp_xxxx or access token'}
                />

                {form.repo_type === 'git' && (
                  <VaultInput
                    label="SSH Private Key"
                    field="ssh_key"
                    value={form.ssh_key}
                    onChange={v => setForm({ ...form, ssh_key: v })}
                    vaultRef={vaultRefs.ssh_key}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder={editing ? '(unchanged)' : '-----BEGIN OPENSSH PRIVATE KEY-----'}
                    isTextarea
                  />
                )}

                {(form.repo_type === 'http' || form.repo_type === 'oci') && (
                  <VaultInput
                    label="CA Certificate (for self-signed TLS)"
                    field="ca_cert"
                    value={form.ca_cert}
                    onChange={v => setForm({ ...form, ca_cert: v })}
                    vaultRef={vaultRefs.ca_cert}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder={editing ? '(unchanged)' : '-----BEGIN CERTIFICATE-----'}
                    isTextarea
                  />
                )}
              </div>

              <div className="flex items-center gap-2">
                <input type="checkbox" id="is_default" checked={form.is_default} onChange={e => setForm({ ...form, is_default: e.target.checked })} className="rounded" />
                <label htmlFor="is_default" className="text-[12px] text-[var(--text-secondary)]">Set as default repository</label>
              </div>
            </div>

            <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
              <button
                onClick={handleSave}
                disabled={!form.name.trim() || !form.url.trim()}
                className="btn btn-primary"
              >
                {editing ? 'Save Changes' : 'Add Repository'}
              </button>
            </div>
          </div>
        </div>
      )}

      {VaultPicker}

      {/* Chart Browser Modal */}
      {browseRepo && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setBrowseRepo(null)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <div>
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Charts in {browseRepo.name}</h2>
                <p className="text-[11px] text-[var(--text-tertiary)]">{browseRepo.url}</p>
              </div>
              <button onClick={() => setBrowseRepo(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4">
              {loadingCharts ? (
                <div className="flex items-center gap-2 py-8 justify-center">
                  <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-[var(--accent)]" />
                  <span className="text-sm text-[var(--text-tertiary)]">Loading charts from repository...</span>
                </div>
              ) : chartError ? (
                <div className="text-center py-8">
                  <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-lg">
                    <p className="text-red-500 text-sm font-medium">Failed to fetch charts</p>
                    <p className="text-red-400 text-[12px] mt-2 font-mono text-left break-all">{chartError}</p>
                  </div>
                  <button
                    onClick={() => browseRepo && openChartBrowser(browseRepo)}
                    className="mt-3 text-[12px] text-[var(--accent)] hover:underline"
                  >
                    Retry
                  </button>
                </div>
              ) : charts.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-[var(--text-tertiary)] text-sm">No charts found in this repository</p>
                  <p className="text-[var(--text-tertiary)] text-[11px] mt-1">The repository may be empty</p>
                </div>
              ) : (
                <div className="space-y-4">
                  {/* Chart list */}
                  <div>
                    <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">{charts.length} charts available</p>
                    <div className="grid grid-cols-1 gap-2 max-h-48 overflow-y-auto">
                      {charts.map(c => (
                        <button
                          key={c.name}
                          onClick={() => selectChart(c)}
                          className={`text-left p-3 rounded-lg border transition-all ${
                            selectedChart?.name === c.name
                              ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                              : 'border-[var(--border)] hover:border-[var(--accent)]/50'
                          }`}
                        >
                          <div className="flex items-center justify-between">
                            <span className="text-[13px] font-medium text-[var(--text-primary)]">{c.name}</span>
                            <span className="text-[11px] text-[var(--text-tertiary)]">v{String(c.latest_version).replace(/^v/, '')}</span>
                          </div>
                          {c.description && (
                            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5 line-clamp-1">{c.description}</p>
                          )}
                        </button>
                      ))}
                    </div>
                  </div>

                  {/* Selected chart versions with download */}
                  {selectedChart && (
                    <div className="pt-4 border-t border-[var(--border-light)]">
                      <div className="flex items-center justify-between mb-2">
                        <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider">
                          Versions of <span className="text-[var(--text-primary)] font-medium">{selectedChart.name}</span>
                        </p>
                      </div>
                      {loadingVersions ? (
                        <div className="flex items-center gap-2 py-4 justify-center">
                          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-[var(--accent)]" />
                          <span className="text-[12px] text-[var(--text-tertiary)]">Loading versions...</span>
                        </div>
                      ) : (
                        <div className="space-y-1.5 max-h-48 overflow-y-auto">
                          {chartVersions.map(v => (
                            <div key={v.version} className="flex items-center justify-between p-2.5 rounded-lg bg-[var(--bg)] border border-[var(--border-light)]">
                              <div>
                                <span className="text-[12px] font-medium text-[var(--text-primary)]">{v.version}</span>
                                {v.app_version && (
                                  <span className="text-[11px] text-[var(--text-tertiary)] ml-2">app: {v.app_version}</span>
                                )}
                                {v.deprecated && (
                                  <span className="text-[10px] text-orange-500 ml-2">deprecated</span>
                                )}
                              </div>
                              <button
                                onClick={() => {
                                  const url = helmRepositories.downloadChartURL(browseRepo.id, selectedChart.name, v.version);
                                  window.open(url, '_blank');
                                }}
                                className="flex items-center gap-1 text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors"
                                title="Download chart .tgz"
                              >
                                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                                </svg>
                                Download
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex justify-end px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setBrowseRepo(null)} className="btn btn-secondary">Close</button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
