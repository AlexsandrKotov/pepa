'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { registryRepositories, type RegistryRepository } from '@/lib/api';
import { VaultInput, useVaultPicker } from '@/components/VaultInput';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';
import ConfirmModal from '@/components/ConfirmModal';
import BrandIcon from '@/components/BrandIcon';

const REGISTRY_TYPES: { value: RegistryRepository['registry_type']; label: string; icon: string; desc: string }[] = [
  { value: 'docker', label: 'Docker Hub', icon: 'docker', desc: 'Docker Hub registry' },
  { value: 'ghcr', label: 'GHCR', icon: 'github', desc: 'GitHub Container Registry' },
  { value: 'harbor', label: 'Harbor', icon: 'services', desc: 'Harbor registry' },
  { value: 'ecr', label: 'ECR', icon: 'storage', desc: 'AWS Elastic Container Registry' },
  { value: 'gcr', label: 'GCR', icon: 'storage', desc: 'Google Container Registry' },
  { value: 'acr', label: 'ACR', icon: 'storage', desc: 'Azure Container Registry' },
  { value: 'other', label: 'Other', icon: 'default', desc: 'Custom registry' },
];

const defaultForm = {
  name: '', description: '', registry_type: 'docker' as RegistryRepository['registry_type'],
  url: '', username: '', password: '', token: '',
  is_default: false,
};

export default function RegistryRepositoriesPage() {
  const { isAdmin, hasPermission, loading: permLoading } = usePermission();
  const canWrite = isAdmin || hasPermission('registry', 'write');
  const [repos, setRepos] = useState<RegistryRepository[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<RegistryRepository | null>(null);
  const [form, setForm] = useState({ ...defaultForm });
  const [error, setError] = useState('');
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();
  // Image browser state
  const [browseRepo, setBrowseRepo] = useState<RegistryRepository | null>(null);
  const [images, setImages] = useState<string[]>([]);
  const [imageError, setImageError] = useState<string>('');
  const [loadingImages, setLoadingImages] = useState(false);
  const [selectedImage, setSelectedImage] = useState<string | null>(null);
  const [imageTags, setImageTags] = useState<string[]>([]);
  const [loadingTags, setLoadingTags] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [visibleCount, setVisibleCount] = useState(20);
  const imagesCache = useRef<Record<string, string[]>>({});
  const tagsCache = useRef<Record<string, string[]>>({});
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [saving, setSaving] = useState(false);

  useEscapeKey(() => {
    if (selectedImage) { setSelectedImage(null); setImageTags([]); }
    else if (browseRepo) setBrowseRepo(null);
    else if (showForm) { setShowForm(false); setEditing(null); }
  }, showForm || browseRepo !== null || selectedImage !== null);

  const load = useCallback(async () => {
    try {
      const res = await registryRepositories.list();
      setRepos(res.registry_repositories || []);
    } catch {
      setRepos([]);
    }
    setLoading(false);
  }, []);

  useEffect(() => { if (isAdmin) load(); }, [isAdmin, load]);

  const openCreate = () => {
    setEditing(null);
    setForm({ ...defaultForm });
    setError('');
    setShowForm(true);
  };

  const openEdit = (r: RegistryRepository) => {
    setEditing(r);
    setVaultRefs({});
    setForm({
      name: r.name, description: r.description, registry_type: r.registry_type,
      url: r.url, username: r.username,
      password: '', token: '',
      is_default: r.is_default,
    });
    setError('');
    setShowForm(true);
  };

  const handleSave = async () => {
    setError('');
    setSaving(true);
    try {
      const merged = { ...form };
      for (const [field, ref] of Object.entries(vaultRefs)) {
        if (ref) (merged as Record<string, unknown>)[field] = ref;
      }
      if (editing) {
        await registryRepositories.update(editing.id, merged);
        // Invalidate cached images/tags for this registry since credentials or URL may have changed
        delete imagesCache.current[editing.id];
        Object.keys(tagsCache.current).forEach(k => { if (k.startsWith(editing.id + ':')) delete tagsCache.current[k]; });
      } else {
        await registryRepositories.create(merged);
      }
      setShowForm(false);
      setVaultRefs({});
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const requestDelete = (id: string) => {
    setDeleteConfirm(id);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    setDeleting(true);
    try {
      await registryRepositories.delete(deleteConfirm);
      load();
    } catch (err) {
      console.error('Failed to delete:', err);
    } finally {
      setDeleting(false);
      setDeleteConfirm(null);
    }
  };

  const openImageBrowser = async (repo: RegistryRepository) => {
    setBrowseRepo(repo);
    setSelectedImage(null);
    setImageTags([]);
    setImageError('');
    setSearchQuery('');
    setVisibleCount(20);
    // Use cached images if available
    if (imagesCache.current[repo.id]) {
      setImages(imagesCache.current[repo.id]);
      setLoadingImages(false);
    } else {
      setImages([]);
      setLoadingImages(true);
      try {
        const data = await registryRepositories.listImages(repo.id);
        const imgs = data.images || [];
        imagesCache.current[repo.id] = imgs;
        setImages(imgs);
      } catch (err) {
        console.error('Failed to load images:', err);
        setImageError(err instanceof Error ? err.message : 'Failed to load images');
      } finally {
        setLoadingImages(false);
      }
    }
  };

  const selectImage = async (image: string) => {
    if (!browseRepo) return;
    setSelectedImage(image);
    const cacheKey = `${browseRepo.id}:${image}`;
    if (tagsCache.current[cacheKey]) {
      setImageTags(tagsCache.current[cacheKey]);
      setLoadingTags(false);
    } else {
      setImageTags([]);
      setLoadingTags(true);
      try {
        const data = await registryRepositories.listTags(browseRepo.id, image);
        const tags = data.tags || [];
        tagsCache.current[cacheKey] = tags;
        setImageTags(tags);
      } catch (err) {
        console.error('Failed to load tags:', err);
      } finally {
        setLoadingTags(false);
      }
    }
  };

  const registryTypeInfo = (t: string) => {
    const found = REGISTRY_TYPES.find(rt => rt.value === t);
    return { icon: found?.icon ?? 'default', label: found?.label ?? t };
  };

  const registryHost = (url: string) => url.replace(/^https?:\/\//, '').replace(/\/$/, '');

  if (permLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('registry', 'read')) {
    return <ForbiddenPage resource="registry_repositories" />;
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Registry Repositories</h1>
          <p className="page-subtitle-modern">Configure container image registries for pulling and scanning images</p>
        </div>
        {canWrite && <button onClick={openCreate} className="btn btn-primary">+ Add Registry</button>}
      </div>

      {/* Info */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up page-delay-1">
        <p className="text-[13px] text-blue-500">
          <span className="font-medium">Registry Repositories</span> store connection details and credentials for your container image registries.
          Once configured, use them for <span className="font-medium">Security Scanning</span> (Trivy), <span className="font-medium">Blueprints</span>, and <span className="font-medium">Deployments</span>.
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
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No registries configured</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Add container registries (Docker Hub, GHCR, Harbor, ECR, GCR, ACR) with credentials
          </p>
          <button onClick={openCreate} className="btn btn-primary">+ Add First Registry</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 page-animate-up page-delay-2">
          {repos.map(r => (
            <div key={r.id} className="card p-5 hover:border-[var(--accent)] transition-colors group modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-2">
                  <BrandIcon name={registryTypeInfo(r.registry_type).icon} size={22} />
                  <div>
                    <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{r.name}</h3>
                    <span className="text-[10px] text-[var(--text-tertiary)] flex items-center gap-1"><BrandIcon name={registryTypeInfo(r.registry_type).icon} size={12} /> {registryTypeInfo(r.registry_type).label}</span>
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
                    <span className="text-[var(--text-secondary)]">{r.username} &middot; credentials set</span>
                  </div>
                )}
              </div>

              <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                <button onClick={() => openImageBrowser(r)} className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">Browse Images</button>
                {canWrite && <button onClick={() => openEdit(r)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--bg)] rounded-lg transition-colors">Edit</button>}
                {canWrite && <button onClick={() => requestDelete(r.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto">Delete</button>}
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
                {editing ? 'Edit Registry' : 'Add Registry'}
              </h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
              )}

              <div>
                <label className="label">Name *</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="my-registry" />
              </div>

              <div>
                <label className="label">Description</label>
                <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="Company container image registry" />
              </div>

              <div>
                <label className="label">Registry Type</label>
                <div className="grid grid-cols-2 gap-2">
                  {REGISTRY_TYPES.map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => {
                        const urlDefaults: Record<string, string> = {
                          docker: 'https://registry-1.docker.io',
                          ghcr: 'https://ghcr.io',
                          gcr: 'https://gcr.io',
                        };
                        const newForm = { ...form, registry_type: opt.value };
                        if (!form.url || Object.values(urlDefaults).includes(form.url)) {
                          newForm.url = urlDefaults[opt.value] || '';
                        }
                        setForm(newForm);
                      }}
                      className={`p-2.5 rounded-lg border text-left transition-all ${
                        form.registry_type === opt.value
                          ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                          : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                      }`}
                    >
                      <div className="flex items-center gap-1.5 text-[12px] font-medium"><BrandIcon name={opt.icon} size={14} /> {opt.label}</div>
                      <div className="text-[10px] text-[var(--text-tertiary)]">{opt.desc}</div>
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="label">Registry URL *</label>
                <input
                  value={form.url}
                  onChange={e => setForm({ ...form, url: e.target.value })}
                  className="input font-mono text-[12px]"
                  placeholder={
                    form.registry_type === 'docker' ? 'https://registry-1.docker.io' :
                    form.registry_type === 'ghcr' ? 'https://ghcr.io' :
                    form.registry_type === 'harbor' ? 'https://harbor.example.com' :
                    'https://registry.example.com'
                  }
                />
              </div>

              {/* Credentials */}
              <div className="space-y-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Authentication (for private registries)</p>

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
                  label="Token / PAT"
                  field="token"
                  value={form.token}
                  onChange={v => setForm({ ...form, token: v })}
                  vaultRef={vaultRefs.token}
                  onOpenVault={onOpenVaultPicker}
                  onRemoveVault={removeVaultRef}
                  placeholder={editing ? '(unchanged)' : 'glpat-xxxx, ghp_xxxx, or access token'}
                />

                {/* Registry-specific auth hints */}
                {(form.registry_type === 'other' || form.registry_type === 'docker' || form.registry_type === 'ghcr') && (
                  <div className="bg-amber-500/10 border border-amber-500/20 rounded-lg p-2.5">
                    <p className="text-[11px] text-amber-600 dark:text-amber-400">
                      {form.registry_type === 'other' && form.url.includes('gitlab') && (
                        <><span className="font-medium">GitLab:</span> Use a <span className="font-medium">Personal Access Token</span> (PAT) with <code className="text-[10px] bg-amber-500/10 px-1 rounded">read_api</code> + <code className="text-[10px] bg-amber-500/10 px-1 rounded">read_registry</code> scopes. Regular passwords do not work when 2FA is enabled.</>
                      )}
                      {form.registry_type === 'other' && !form.url.includes('gitlab') && (
                        <>For custom registries, use a <span className="font-medium">Token/PAT</span> if the registry requires it. Password works for most standard Docker registries.</>
                      )}
                      {form.registry_type === 'docker' && (
                        <>Docker Hub: Use an <span className="font-medium">Access Token</span> (recommended) or password. Generate tokens at hub.docker.com → Account Settings → Security.</>
                      )}
                      {form.registry_type === 'ghcr' && (
                        <>GHCR: Use a GitHub <span className="font-medium">Personal Access Token</span> with <code className="text-[10px] bg-amber-500/10 px-1 rounded">read:packages</code> scope. Username must be your GitHub username.</>
                      )}
                    </p>
                  </div>
                )}
              </div>

              <div className="flex items-center gap-2">
                <input type="checkbox" id="is_default" checked={form.is_default} onChange={e => setForm({ ...form, is_default: e.target.checked })} className="rounded" />
                <label htmlFor="is_default" className="text-[12px] text-[var(--text-secondary)]">Set as default registry</label>
              </div>
            </div>

            <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
              <button
                onClick={handleSave}
                disabled={saving || !form.name.trim() || !form.url.trim()}
                className="btn btn-primary"
              >
                {saving ? 'Saving...' : editing ? 'Save Changes' : 'Add Registry'}
              </button>
            </div>
          </div>
        </div>
      )}

      {VaultPicker}

      {/* Image Browser Modal */}
      {browseRepo && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setBrowseRepo(null)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <div>
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Images in {browseRepo.name}</h2>
                <p className="text-[11px] text-[var(--text-tertiary)]">{browseRepo.url}</p>
              </div>
              <button onClick={() => setBrowseRepo(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4">
              {loadingImages ? (
                <div className="flex items-center gap-2 py-8 justify-center">
                  <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-[var(--accent)]" />
                  <span className="text-sm text-[var(--text-tertiary)]">Loading images from registry...</span>
                </div>
              ) : imageError ? (
                <div className="text-center py-8">
                  <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-lg">
                    <p className="text-red-500 text-sm font-medium">Failed to fetch images</p>
                    <p className="text-red-400 text-[12px] mt-2 font-mono text-left break-all">{imageError}</p>
                  </div>
                  <button
                    onClick={() => browseRepo && openImageBrowser(browseRepo)}
                    className="mt-3 text-[12px] text-[var(--accent)] hover:underline"
                  >
                    Retry
                  </button>
                </div>
              ) : images.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-[var(--text-tertiary)] text-sm">No images found in this registry</p>
                  <p className="text-[var(--text-tertiary)] text-[11px] mt-1">The registry may be empty or credentials may be insufficient</p>
                </div>
              ) : (
                <div className="space-y-4">
                  <div>
                    <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">{images.length} images available</p>
                    {images.length > 5 && (
                      <div className="relative mb-2">
                        <svg className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                        </svg>
                        <input
                          type="text"
                          value={searchQuery}
                          onChange={e => { setSearchQuery(e.target.value); setVisibleCount(20); }}
                          className="input pl-8 text-[12px]"
                          placeholder="Search images..."
                        />
                      </div>
                    )}
                    {(() => {
                      const filtered = images.filter(img => !searchQuery || img.toLowerCase().includes(searchQuery.toLowerCase()));
                      const visible = filtered.slice(0, visibleCount);
                      return (
                        <>
                          <div className="grid grid-cols-1 gap-2 max-h-60 overflow-y-auto">
                            {visible.map(img => (
                              <button
                                key={img}
                                onClick={() => selectImage(img)}
                                className={`text-left p-3 rounded-lg border transition-all ${
                                  selectedImage === img
                                    ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                                    : 'border-[var(--border)] hover:border-[var(--accent)]/50'
                                }`}
                              >
                                <div className="flex items-center justify-between">
                                  <span className="text-[13px] font-medium text-[var(--text-primary)] font-mono">{img}</span>
                                  {selectedImage === img && (
                                    <span className="text-[10px] text-[var(--accent)]">selected</span>
                                  )}
                                </div>
                              </button>
                            ))}
                          </div>
                          {filtered.length === 0 && searchQuery ? (
                            <p className="text-[12px] text-[var(--text-tertiary)] mt-2">No images match &quot;{searchQuery}&quot;</p>
                          ) : visibleCount < filtered.length ? (
                            <button
                              onClick={() => setVisibleCount(c => c + 20)}
                              className="mt-2 text-[12px] text-[var(--accent)] hover:underline"
                            >
                              Show more ({filtered.length - visibleCount} remaining)
                            </button>
                          ) : null}
                        </>
                      );
                    })()}
                  </div>

                  {selectedImage && (
                    <div className="pt-4 border-t border-[var(--border-light)]">
                      <div className="flex items-center justify-between mb-2">
                        <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider">
                          Tags for <span className="text-[var(--text-primary)] font-medium font-mono">{selectedImage}</span>
                        </p>
                      </div>
                      {loadingTags ? (
                        <div className="flex items-center gap-2 py-4 justify-center">
                          <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-[var(--accent)]" />
                          <span className="text-[12px] text-[var(--text-tertiary)]">Loading tags...</span>
                        </div>
                      ) : imageTags.length === 0 ? (
                        <p className="text-[12px] text-[var(--text-tertiary)] py-2">No tags found</p>
                      ) : (
                        <div className="space-y-1.5 max-h-48 overflow-y-auto">
                          {imageTags.map(tag => (
                            <div key={tag} className="flex items-center justify-between p-2.5 rounded-lg bg-[var(--bg)] border border-[var(--border-light)]">
                              <span className="text-[12px] font-medium text-[var(--text-primary)] font-mono">{tag}</span>
                              <button
                                onClick={() => {
                                  const fullRef = `${registryHost(browseRepo.url)}/${selectedImage}:${tag}`;
                                  navigator.clipboard.writeText(fullRef);
                                }}
                                className="flex items-center gap-1 text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors"
                                title="Copy image reference"
                              >
                                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                </svg>
                                Copy
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

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteConfirm}
        title="Delete this registry?"
        description="This registry will be permanently removed. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm(null)}
      />
      </div>
    </div>
  );
}
