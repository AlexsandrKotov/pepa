'use client';

import React, { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import ConceptHelp from '@/components/ConceptHelp';
import GearIcon from '@/components/GearIcon';
import GitRepoPicker from '@/components/GitRepoPicker';
import { helmRepositories, blueprints as blueprintsAPI, blueprintGroups as blueprintGroupsAPI, type ServiceBlueprint, type HelmRepository, type HelmChart, type HelmChartVersion, type BlueprintGroup } from '@/lib/api';

const categoryIcons: Record<string, React.ReactNode> = {
  backend: <GearIcon className="w-4 h-4" />, frontend: '🌐', database: '🗄️', messaging: '📨',
  monitoring: '📊', security: '🔒', cache: '⚡', storage: '💾', general: '📦',
};

export default function PipelineBlueprintsPage() {
  const [blueprints, setBlueprints] = useState<ServiceBlueprint[]>([]);
  const [groups, setGroups] = useState<BlueprintGroup[]>([]);
  const [helmRepos, setHelmRepos] = useState<HelmRepository[]>([]);
  const [repoCharts, setRepoCharts] = useState<HelmChart[]>([]);
  const [chartVersions, setChartVersions] = useState<HelmChartVersion[]>([]);
  const [loadingCharts, setLoadingCharts] = useState(false);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<ServiceBlueprint | null>(null);
  const [search, setSearch] = useState('');

  // Form state
  const [form, setForm] = useState({
    name: '', description: '', source_type: 'container' as ServiceBlueprint['source_type'],
    helm_repo_id: '', image: '', chart_url: '', chart_name: '', chart_version: '', chart_path: '',
    namespace: 'default',
    values_yaml: '', cpu: '100m', memory: '128Mi', replicas: 1,
    ports: '8080', category: 'general',
    group_ids: [] as string[],
    compose_yaml: '',
  });
  const [gitInputMode, setGitInputMode] = useState<'picker' | 'manual'>('picker');

  useEscapeKey(() => {
    if (showForm) { setShowForm(false); setEditing(null); }
  }, showForm);

  useEffect(() => {
    blueprintsAPI.list().then(res => setBlueprints(res.blueprints || [])).catch(() => {});
    helmRepositories.list().then(res => setHelmRepos(res.helm_repositories || [])).catch(() => {});
    blueprintGroupsAPI.list().then(res => setGroups(res.groups || [])).catch(() => {});
  }, []);

  const openCreate = () => {
    setEditing(null);
    setForm({
      name: '', description: '', source_type: 'container',
      helm_repo_id: '', image: '', chart_url: '', chart_name: '', chart_version: '', chart_path: '',
      namespace: 'default', values_yaml: '', cpu: '100m', memory: '128Mi', replicas: 1, ports: '8080', category: 'general',
      group_ids: [], compose_yaml: '',
    });
    setRepoCharts([]);
    setChartVersions([]);
    setShowForm(true);
  };

  const openEdit = async (bp: ServiceBlueprint) => {
    setEditing(bp);
    setForm({
      name: bp.name, description: bp.description, source_type: bp.source_type || 'container',
      helm_repo_id: bp.helm_repo_id || '', image: bp.image, chart_url: bp.chart_url || '', chart_name: bp.chart_name || '', chart_version: bp.chart_version || '', chart_path: bp.chart_path || '',
      namespace: bp.namespace, values_yaml: bp.values_yaml,
      cpu: bp.cpu, memory: bp.memory, replicas: bp.replicas,
      ports: bp.ports.join(', '), category: bp.category,
      group_ids: bp.group_ids || [],
      compose_yaml: bp.compose_yaml || '',
    });
    setRepoCharts([]);
    setChartVersions([]);
    // Load charts if editing a blueprint with a helm repo
    if (bp.helm_repo_id) {
      try {
        setLoadingCharts(true);
        const data = await helmRepositories.listCharts(bp.helm_repo_id);
        setRepoCharts(data.charts || []);
      } catch { /* ignore */ }
      setLoadingCharts(false);
      // Load versions for the current chart
      if (bp.chart_path) {
        try {
          setLoadingVersions(true);
          const vData = await helmRepositories.listChartVersions(bp.helm_repo_id, bp.chart_path);
          setChartVersions(vData.versions || []);
        } catch { /* ignore */ }
        setLoadingVersions(false);
      }
    }
    setShowForm(true);
  };

  const handleSave = async () => {
    if (!form.name.trim()) return;
    // Validate: container needs image, helm needs chart_url or helm_repo_id, docker_compose needs compose_yaml
    if (form.source_type === 'container' && !form.image.trim()) return;
    if (form.source_type !== 'container' && form.source_type !== 'docker_compose' && !form.chart_url.trim() && !form.helm_repo_id) return;
    if (form.source_type === 'docker_compose' && !form.compose_yaml.trim()) return;
    const ports = form.ports.split(',').map(p => parseInt(p.trim())).filter(p => !isNaN(p));

    // If helm_repo_id is set, resolve the chart_url from the repo
    let chartUrl = form.chart_url.trim();
    if (!chartUrl && form.helm_repo_id) {
      const repo = helmRepos.find(r => r.id === form.helm_repo_id);
      if (repo) chartUrl = repo.url;
    }

    const payload = {
      name: form.name.trim(),
      description: form.description.trim(),
      source_type: form.source_type,
      helm_repo_id: form.helm_repo_id || null,
      image: form.image.trim(),
      chart_url: chartUrl,
      chart_name: form.chart_name.trim(),
      chart_version: form.chart_version.trim(),
      chart_path: form.chart_path.trim(),
      namespace: form.namespace.trim() || 'default',
      values_yaml: form.values_yaml,
      cpu: form.cpu, memory: form.memory, replicas: form.replicas,
      ports, category: form.category,
      group_ids: form.group_ids,
      compose_yaml: form.compose_yaml,
    };

    try {
      if (editing) {
        const updated = await blueprintsAPI.update(editing.id, payload);
        setBlueprints(blueprints.map(b => b.id === editing.id ? updated : b));
      } else {
        const created = await blueprintsAPI.create(payload);
        setBlueprints([...blueprints, created]);
      }
      setShowForm(false);
    } catch (err) {
      console.error('Failed to save blueprint:', err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this blueprint?')) return;
    try {
      await blueprintsAPI.delete(id);
      setBlueprints(blueprints.filter(b => b.id !== id));
    } catch (err) {
      console.error('Failed to delete blueprint:', err);
    }
  };

  const handleDuplicate = async (bp: ServiceBlueprint) => {
    try {
      const dup = await blueprintsAPI.create({
        ...bp,
        name: `${bp.name} (copy)`,
      } as any);
      setBlueprints([...blueprints, dup]);
    } catch (err) {
      console.error('Failed to duplicate blueprint:', err);
    }
  };

  const filtered = search
    ? blueprints.filter(b =>
        b.name.toLowerCase().includes(search.toLowerCase()) ||
        b.category.toLowerCase().includes(search.toLowerCase()) ||
        b.image.toLowerCase().includes(search.toLowerCase()) ||
        b.chart_url.toLowerCase().includes(search.toLowerCase())
      )
    : blueprints;

  const categories = [...new Set(blueprints.map(b => b.category))];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Pipeline Blueprints</h1>
            <ConceptHelp term="blueprint" />
          </div>
          <p className="page-subtitle-modern">Pre-configured service templates for deployment pipelines</p>
        </div>
        <div className="flex gap-2">
          <Link href="/blueprint-groups" className="btn btn-secondary text-[12px]">
            Groups
          </Link>
          <Link href="/pipeline-builder" className="btn btn-secondary text-[12px]">
            Open Pipeline Builder →
          </Link>
          <button onClick={openCreate} className="btn btn-primary">
            + New Blueprint
          </button>
        </div>
      </div>

      {/* Info */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up page-delay-1">
        <p className="text-[13px] text-blue-500">
          <span className="font-medium">Blueprints</span> are reusable service definitions with pre-filled values.yaml, images, and resource configs.
          Use them in the <Link href="/pipeline-builder" className="underline font-medium">Pipeline Builder</Link> to compose and deploy multi-service pipelines to any cluster.
        </p>
      </div>

      {/* Search */}
      <div className="flex gap-3 page-animate-up page-delay-1">
        <input
          type="text"
          placeholder="Search blueprints..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="input flex-1"
        />
        {categories.length > 1 && (
          <div className="flex gap-1">
            {categories.map(cat => (
              <span key={cat} className="text-[11px] px-2 py-1 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-lg">
                {categoryIcons[cat] || '📦'} {cat}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Grid */}
      {blueprints.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">📋</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No blueprints yet</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Create service blueprints with pre-configured values.yaml to use in deployment pipelines
          </p>
          <button onClick={openCreate} className="btn btn-primary">+ Create First Blueprint</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 page-animate-up page-delay-2">
          {filtered.map(bp => (
            <div key={bp.id} className="card p-5 hover:border-[var(--accent)] transition-colors group modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{categoryIcons[bp.category] || '📦'}</span>
                  <div>
                    <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{bp.name}</h3>
                    <span className="text-[10px] text-[var(--text-tertiary)]">{bp.category}</span>
                  </div>
                </div>
              </div>
              {bp.description && (
                <p className="text-[12px] text-[var(--text-secondary)] mb-3 line-clamp-2">{bp.description}</p>
              )}
              <div className="space-y-1.5 mb-4">
                {/* Source type badge */}
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-16">Source:</span>
                  <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
                    bp.source_type === 'container' ? 'bg-blue-500/10 text-blue-500' :
                    bp.source_type === 'helm_git' ? 'bg-purple-500/10 text-purple-500' :
                    bp.source_type === 'helm_http' ? 'bg-orange-500/10 text-orange-500' :
                    bp.source_type === 'docker_compose' ? 'bg-cyan-500/10 text-cyan-500' :
                    'bg-emerald-500/10 text-emerald-500'
                  }`}>
                    {bp.source_type === 'container' ? '🐳 Container' :
                     bp.source_type === 'helm_git' ? '🔀 Helm Git' :
                     bp.source_type === 'helm_http' ? '🌐 Helm HTTP' :
                     bp.source_type === 'docker_compose' ? '🐙 Compose' :
                     '📦 Helm OCI'}
                  </span>
                </div>
                {bp.source_type === 'docker_compose' ? (
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-16">Compose:</span>
                    <span className="text-cyan-500 font-medium">{bp.compose_yaml.split('\n').filter(l => l.trim() && !l.trim().startsWith('#')).length} lines</span>
                  </div>
                ) : bp.source_type === 'container' ? (
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-16">Image:</span>
                    <span className="font-mono text-[var(--text-secondary)] truncate">{bp.image || '—'}</span>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-[var(--text-tertiary)] w-16">Chart:</span>
                      <span className="font-mono text-[var(--text-secondary)] truncate">{bp.chart_url}</span>
                    </div>
                    {bp.chart_version && (
                      <div className="flex items-center gap-2 text-[11px]">
                        <span className="text-[var(--text-tertiary)] w-16">Version:</span>
                        <span className="text-[var(--text-secondary)]">{bp.chart_version}</span>
                      </div>
                    )}
                    {bp.image && (
                      <div className="flex items-center gap-2 text-[11px]">
                        <span className="text-[var(--text-tertiary)] w-16">Image:</span>
                        <span className="font-mono text-[var(--text-secondary)] truncate">{bp.image}</span>
                      </div>
                    )}
                  </>
                )}
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-16">NS:</span>
                  <span className="text-[var(--text-secondary)]">{bp.namespace}</span>
                </div>
                <div className="flex items-center gap-2 text-[11px]">
                  <span className="text-[var(--text-tertiary)] w-16">Resources:</span>
                  <span className="text-[var(--text-secondary)]">{bp.cpu} / {bp.memory} × {bp.replicas}</span>
                </div>
                {bp.ports.length > 0 && (
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-16">Ports:</span>
                    <div className="flex gap-1">
                      {bp.ports.map(p => (
                        <span key={p} className="px-1.5 py-0.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded text-[10px] font-mono">{p}</span>
                      ))}
                    </div>
                  </div>
                )}
                {bp.values_yaml && (
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-16">values.yaml:</span>
                    <span className="text-green-600 font-medium">{bp.values_yaml.split('\n').filter(l => l.trim() && !l.trim().startsWith('#')).length} lines</span>
                  </div>
                )}
              </div>
              <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                <button onClick={() => openEdit(bp)} className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">Edit</button>
                <button onClick={() => handleDuplicate(bp)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded-lg transition-colors">Duplicate</button>
                <button onClick={() => handleDelete(bp.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create/Edit Modal */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                {editing ? 'Edit Blueprint' : 'New Blueprint'}
              </h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Name *</label>
                  <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="nginx-webserver" />
                </div>
                <div>
                  <label className="label">Category</label>
                  <select value={form.category} onChange={e => setForm({ ...form, category: e.target.value })} className="input">
                    {Object.keys(categoryIcons).map((k) => (
                      <option key={k} value={k}>{k}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div>
                <label className="label">Description</label>
                <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="Frontend web server" />
              </div>

              {/* Source Type */}
              <div>
                <label className="label">Source Type</label>
                <div className="grid grid-cols-5 gap-2">
                  {[
                    { value: 'container', label: '🐳 Container', desc: 'Docker image' },
                    { value: 'helm_git', label: '🔀 Helm Git', desc: 'Chart from Git' },
                    { value: 'helm_http', label: '🌐 Helm HTTP', desc: 'Chart .tgz URL' },
                    { value: 'helm_oci', label: '📦 Helm OCI', desc: 'OCI registry' },
                    { value: 'docker_compose', label: '🐙 Compose', desc: 'Docker Compose' },
                  ].map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => {
                        const newType = opt.value as ServiceBlueprint['source_type'];
                        setForm({
                          ...form,
                          source_type: newType,
                          // Reset source-specific fields when switching type
                          image: newType === 'container' ? form.image : '',
                          chart_url: '',
                          chart_version: '',
                          chart_path: '',
                          helm_repo_id: '',
                        });
                        setRepoCharts([]);
                        setChartVersions([]);
                      }}
                      className={`p-2.5 rounded-lg border text-left transition-all ${
                        form.source_type === opt.value
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

              {/* === Container: Docker image === */}
              {form.source_type === 'container' && (
                <div>
                  <label className="label">Docker Image *</label>
                  <input value={form.image} onChange={e => setForm({ ...form, image: e.target.value })} className="input font-mono text-[12px]" placeholder="registry.example.com/nginx:1.25" />
                </div>
              )}

              {/* === Helm Git: Git repo URL + subpath === */}
              {form.source_type === 'helm_git' && (
                <div className="space-y-3">
                  <div className="space-y-1.5">
                    <div className="flex gap-1.5">
                      <button
                        type="button"
                        onClick={() => setGitInputMode('picker')}
                        className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${gitInputMode === 'picker' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                      >
                        Browse
                      </button>
                      <button
                        type="button"
                        onClick={() => setGitInputMode('manual')}
                        className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${gitInputMode === 'manual' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                      >
                        Manual URL
                      </button>
                    </div>
                    {gitInputMode === 'picker' ? (
                      <GitRepoPicker
                        label="Git Repository *"
                        value={{ repo_url: form.chart_url }}
                        onChange={(v) => setForm({ ...form, chart_url: v.repo_url })}
                      />
                    ) : (
                      <div>
                        <label className="label">Git Repository URL *</label>
                        <input value={form.chart_url} onChange={e => setForm({ ...form, chart_url: e.target.value })} className="input font-mono text-[12px]" placeholder="https://github.com/org/repo" />
                      </div>
                    )}
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Chart Subpath <span className="text-[var(--text-tertiary)]">(for monorepos)</span></label>
                      <input value={form.chart_path} onChange={e => setForm({ ...form, chart_path: e.target.value })} className="input font-mono text-[12px]" placeholder="charts/myapp" />
                    </div>
                    <div>
                      <label className="label">Chart Version</label>
                      <input value={form.chart_version} onChange={e => setForm({ ...form, chart_version: e.target.value })} className="input font-mono text-[12px]" placeholder="1.2.0" />
                    </div>
                  </div>
                  <div>
                    <label className="label">Container Image <span className="text-[var(--text-tertiary)]">(optional, override in values.yaml)</span></label>
                    <input value={form.image} onChange={e => setForm({ ...form, image: e.target.value })} className="input font-mono text-[12px]" placeholder="registry.example.com/myapp:1.2.0" />
                  </div>
                </div>
              )}

              {/* === Helm HTTP: Helm Repository selector + chart picker or direct URL === */}
              {form.source_type === 'helm_http' && (
                <div className="space-y-3">
                  {/* Helm Repository selector (HTTP repos only) */}
                  <div>
                    <label className="label">
                      Helm Repository
                      {helmRepos.filter(r => r.repo_type === 'http').length === 0 && (
                        <span className="text-[var(--text-tertiary)] font-normal ml-2">
                          (none configured — <Link href="/helm-repositories" className="text-[var(--accent)] hover:underline">add one</Link>)
                        </span>
                      )}
                    </label>
                    <select
                      value={form.helm_repo_id}
                      onChange={async (e) => {
                        const repoId = e.target.value;
                        const repo = helmRepos.find(r => r.id === repoId);
                        setForm({
                          ...form,
                          helm_repo_id: repoId,
                          chart_url: repo ? repo.url : form.chart_url,
                          chart_path: '',
                          chart_version: '',
                        });
                        setRepoCharts([]);
                        setChartVersions([]);
                        if (repoId) {
                          setLoadingCharts(true);
                          try {
                            const data = await helmRepositories.listCharts(repoId);
                            setRepoCharts(data.charts || []);
                          } catch {
                            setRepoCharts([]);
                          } finally {
                            setLoadingCharts(false);
                          }
                        }
                      }}
                      className="input"
                    >
                      <option value="">— Select repository or enter URL manually —</option>
                      {helmRepos
                        .filter(r => r.repo_type === 'http')
                        .map(r => (
                          <option key={r.id} value={r.id}>
                            {r.name} ({r.url}){r.is_default ? ' ★' : ''}
                          </option>
                        ))
                      }
                    </select>
                  </div>

                  {/* Chart selection */}
                  <div>
                    <label className="label">
                      {form.helm_repo_id
                        ? 'Chart Name *'
                        : 'Chart Package URL *'
                      }
                    </label>
                    {form.helm_repo_id ? (
                      loadingCharts ? (
                        <div className="flex items-center gap-2 py-2">
                          <div className="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-[var(--accent)]" />
                          <span className="text-[12px] text-[var(--text-tertiary)]">Loading charts...</span>
                        </div>
                      ) : repoCharts.length > 0 ? (
                        <select
                          value={form.chart_path}
                          onChange={async (e) => {
                            const chartName = e.target.value;
                            // Auto-populate chart_name from chart_path selection
                            setForm({ ...form, chart_path: chartName, chart_name: chartName, chart_version: '' });
                            setChartVersions([]);
                            if (chartName && form.helm_repo_id) {
                              setLoadingVersions(true);
                              try {
                                const data = await helmRepositories.listChartVersions(form.helm_repo_id, chartName);
                                setChartVersions(data.versions || []);
                              } catch {
                                setChartVersions([]);
                              } finally {
                                setLoadingVersions(false);
                              }
                            }
                          }}
                          className="input font-mono text-[12px]"
                        >
                          <option value="">Select a chart...</option>
                          {repoCharts.map(c => (
                            <option key={c.name} value={c.name}>
                              {c.name}{c.latest_version ? ` (v${String(c.latest_version).replace(/^v/, '')})` : ''}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <input
                          value={form.chart_path}
                          onChange={e => setForm({ ...form, chart_path: e.target.value })}
                          className="input font-mono text-[12px]"
                          placeholder="myapp"
                        />
                      )
                    ) : (
                      <input
                        value={form.chart_url}
                        onChange={e => setForm({ ...form, chart_url: e.target.value })}
                        className="input font-mono text-[12px]"
                        placeholder="https://charts.example.com/myapp-1.2.0.tgz"
                      />
                    )}
                  </div>

                  {/* Chart name override (optional, auto-filled from chart selection) */}
                  <div>
                    <label className="label">Chart Name Override <span className="text-[var(--text-tertiary)]">(optional)</span></label>
                    <input
                      value={form.chart_name}
                      onChange={e => setForm({ ...form, chart_name: e.target.value })}
                      className="input font-mono text-[12px]"
                      placeholder="Auto-filled from chart selection"
                    />
                    <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Override the chart name if different from the selected chart. Used as: repo/{'{'}chart_name{'}'}</p>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Chart Version</label>
                      {loadingVersions ? (
                        <div className="flex items-center gap-2 py-2">
                          <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
                          <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
                        </div>
                      ) : chartVersions.length > 0 ? (
                        <select
                          value={form.chart_version}
                          onChange={e => setForm({ ...form, chart_version: e.target.value })}
                          className="input font-mono text-[12px]"
                        >
                          <option value="">Latest</option>
                          {chartVersions.map(v => (
                            <option key={v.version} value={v.version}>
                              v{String(v.version).replace(/^v/, '')}{v.app_version ? ` (app: ${v.app_version})` : ''}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <input value={form.chart_version} onChange={e => setForm({ ...form, chart_version: e.target.value })} className="input font-mono text-[12px]" placeholder="1.2.0" />
                      )}
                    </div>
                    <div>
                      <label className="label">Container Image <span className="text-[var(--text-tertiary)]">(optional)</span></label>
                      <input value={form.image} onChange={e => setForm({ ...form, image: e.target.value })} className="input font-mono text-[12px]" placeholder="registry.example.com/myapp:1.2.0" />
                    </div>
                  </div>
                </div>
              )}

              {/* === Helm OCI: Helm Repository selector + chart picker === */}
              {form.source_type === 'helm_oci' && (
                <div className="space-y-3">
                  {/* Helm Repository selector (OCI repos only) */}
                  <div>
                    <label className="label">
                      Helm Repository
                      {helmRepos.filter(r => r.repo_type === 'oci').length === 0 && (
                        <span className="text-[var(--text-tertiary)] font-normal ml-2">
                          (none configured — <Link href="/helm-repositories" className="text-[var(--accent)] hover:underline">add one</Link>)
                        </span>
                      )}
                    </label>
                    <select
                      value={form.helm_repo_id}
                      onChange={async (e) => {
                        const repoId = e.target.value;
                        const repo = helmRepos.find(r => r.id === repoId);
                        setForm({
                          ...form,
                          helm_repo_id: repoId,
                          chart_url: repo ? repo.url : form.chart_url,
                          chart_path: '',
                          chart_version: '',
                        });
                        setRepoCharts([]);
                        setChartVersions([]);
                        if (repoId) {
                          setLoadingCharts(true);
                          try {
                            const data = await helmRepositories.listCharts(repoId);
                            setRepoCharts(data.charts || []);
                          } catch {
                            setRepoCharts([]);
                          } finally {
                            setLoadingCharts(false);
                          }
                        }
                      }}
                      className="input"
                    >
                      <option value="">— Select repository or enter URL manually —</option>
                      {helmRepos
                        .filter(r => r.repo_type === 'oci')
                        .map(r => (
                          <option key={r.id} value={r.id}>
                            {r.name} ({r.url}){r.is_default ? ' ★' : ''}
                          </option>
                        ))
                      }
                    </select>
                  </div>

                  {/* Chart selection */}
                  <div>
                    <label className="label">
                      {form.helm_repo_id
                        ? 'Chart Name *'
                        : 'OCI Chart URL *'
                      }
                    </label>
                    {form.helm_repo_id ? (
                      loadingCharts ? (
                        <div className="flex items-center gap-2 py-2">
                          <div className="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-[var(--accent)]" />
                          <span className="text-[12px] text-[var(--text-tertiary)]">Loading charts...</span>
                        </div>
                      ) : repoCharts.length > 0 ? (
                        <select
                          value={form.chart_path}
                          onChange={async (e) => {
                            const chartName = e.target.value;
                            setForm({ ...form, chart_path: chartName, chart_version: '' });
                            setChartVersions([]);
                            if (chartName && form.helm_repo_id) {
                              setLoadingVersions(true);
                              try {
                                const data = await helmRepositories.listChartVersions(form.helm_repo_id, chartName);
                                setChartVersions(data.versions || []);
                              } catch {
                                setChartVersions([]);
                              } finally {
                                setLoadingVersions(false);
                              }
                            }
                          }}
                          className="input font-mono text-[12px]"
                        >
                          <option value="">Select a chart...</option>
                          {repoCharts.map(c => (
                            <option key={c.name} value={c.name}>
                              {c.name}{c.latest_version ? ` (v${String(c.latest_version).replace(/^v/, '')})` : ''}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <input
                          value={form.chart_path}
                          onChange={e => setForm({ ...form, chart_path: e.target.value })}
                          className="input font-mono text-[12px]"
                          placeholder="myapp"
                        />
                      )
                    ) : (
                      <input
                        value={form.chart_url}
                        onChange={e => setForm({ ...form, chart_url: e.target.value })}
                        className="input font-mono text-[12px]"
                        placeholder="oci://registry.example.com/charts/myapp"
                      />
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Chart Version</label>
                      {loadingVersions ? (
                        <div className="flex items-center gap-2 py-2">
                          <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
                          <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
                        </div>
                      ) : chartVersions.length > 0 ? (
                        <select
                          value={form.chart_version}
                          onChange={e => setForm({ ...form, chart_version: e.target.value })}
                          className="input font-mono text-[12px]"
                        >
                          <option value="">Latest</option>
                          {chartVersions.map(v => (
                            <option key={v.version} value={v.version}>
                              v{String(v.version).replace(/^v/, '')}{v.app_version ? ` (app: ${v.app_version})` : ''}
                            </option>
                          ))}
                        </select>
                      ) : (
                        <input value={form.chart_version} onChange={e => setForm({ ...form, chart_version: e.target.value })} className="input font-mono text-[12px]" placeholder="1.2.0" />
                      )}
                    </div>
                    <div>
                      <label className="label">Container Image <span className="text-[var(--text-tertiary)]">(optional)</span></label>
                      <input value={form.image} onChange={e => setForm({ ...form, image: e.target.value })} className="input font-mono text-[12px]" placeholder="registry.example.com/myapp:1.2.0" />
                    </div>
                  </div>
                </div>
              )}

              {/* === Docker Compose: compose YAML === */}
              {form.source_type === 'docker_compose' && (
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <label className="label mb-0">docker-compose.yml *</label>
                    <label className="text-[11px] text-[var(--accent)] hover:underline cursor-pointer inline-flex items-center gap-1">
                      <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                      </svg>
                      Upload .yaml
                      <input
                        type="file"
                        accept=".yaml,.yml"
                        className="hidden"
                        onChange={async (e) => {
                          const file = e.target.files?.[0];
                          if (!file) return;
                          const text = await file.text();
                          setForm({ ...form, compose_yaml: text });
                        }}
                      />
                    </label>
                  </div>
                  <textarea
                    value={form.compose_yaml}
                    onChange={e => setForm({ ...form, compose_yaml: e.target.value })}
                    className="input font-mono text-[12px] w-full"
                    rows={10}
                    spellCheck={false}
                    placeholder={`version: '3.8'\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "80:80"\n    environment:\n      - ENV=production`}
                  />
                </div>
              )}

              {/* Group assignment (multi-select) */}
              <div>
                <label className="label">Blueprint Groups</label>
                {groups.length === 0 ? (
                  <p className="text-[11px] text-[var(--text-tertiary)]">
                    No groups yet. <Link href="/blueprint-groups" className="text-[var(--accent)] hover:underline">Create a group</Link>
                  </p>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {groups.map(g => {
                      const selected = form.group_ids.includes(g.id);
                      return (
                        <button
                          key={g.id}
                          type="button"
                          onClick={() => {
                            const next = selected
                              ? form.group_ids.filter(id => id !== g.id)
                              : [...form.group_ids, g.id];
                            setForm({ ...form, group_ids: next });
                          }}
                          className={`text-[11px] px-2.5 py-1 rounded-lg border transition-all cursor-pointer ${
                            selected
                              ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)]'
                              : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                          }`}
                        >
                          {selected ? '✓ ' : ''}{g.name}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>

              {form.source_type !== 'docker_compose' && (
              <>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="label">Namespace</label>
                  <input value={form.namespace} onChange={e => setForm({ ...form, namespace: e.target.value })} className="input" placeholder="default" />
                </div>
                <div>
                  <label className="label">CPU</label>
                  <input value={form.cpu} onChange={e => setForm({ ...form, cpu: e.target.value })} className="input" placeholder="100m" />
                </div>
                <div>
                  <label className="label">Memory</label>
                  <input value={form.memory} onChange={e => setForm({ ...form, memory: e.target.value })} className="input" placeholder="128Mi" />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="label">Replicas</label>
                  <input type="number" value={form.replicas} onChange={e => setForm({ ...form, replicas: Number(e.target.value) || 1 })} className="input" min={1} />
                </div>
                <div>
                  <label className="label">Ports <span className="text-[var(--text-tertiary)]">(comma-separated)</span></label>
                  <input value={form.ports} onChange={e => setForm({ ...form, ports: e.target.value })} className="input" placeholder="8080, 8443" />
                </div>
              </div>
              </>
              )}

              {form.source_type !== 'docker_compose' && (
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="label mb-0">values.yaml</label>
                  <label className="text-[11px] text-[var(--accent)] hover:underline cursor-pointer inline-flex items-center gap-1">
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                    </svg>
                    Upload .yaml
                    <input
                      type="file"
                      accept=".yaml,.yml"
                      className="hidden"
                      onChange={async (e) => {
                        const file = e.target.files?.[0];
                        if (!file) return;
                        const text = await file.text();
                        setForm({ ...form, values_yaml: text });
                      }}
                    />
                  </label>
                </div>
                <textarea
                  value={form.values_yaml}
                  onChange={e => setForm({ ...form, values_yaml: e.target.value })}
                  className="input font-mono text-[12px] w-full"
                  rows={10}
                  spellCheck={false}
                  placeholder={`# Paste your Helm values.yaml here\nreplicaCount: 1\n\nimage:\n  repository: nginx\n  tag: "1.25"\n\nservice:\n  type: ClusterIP\n  port: 80`}
                />
              </div>
              )}
            </div>

            <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
              <button
                onClick={handleSave}
                disabled={!form.name.trim() || (form.source_type === 'container' ? !form.image.trim() : form.source_type === 'docker_compose' ? !form.compose_yaml.trim() : (!form.chart_url.trim() && !form.helm_repo_id))}
                className="btn btn-primary"
              >
                {editing ? 'Save Changes' : 'Create Blueprint'}
              </button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
