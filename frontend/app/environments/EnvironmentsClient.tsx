'use client';
import { useState, useCallback, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { environments, type Environment } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import ConfirmModal from '@/components/ConfirmModal';

export default function EnvironmentsClient({ initialEnvironments }: { initialEnvironments?: Environment[] }) {
  return (
    <PermissionGuard resource="environments" action="read">
      <EnvironmentsClientContent initialEnvironments={initialEnvironments} />
    </PermissionGuard>
  );
}

function EnvironmentsClientContent({ initialEnvironments }: { initialEnvironments?: Environment[] }) {
  const [envs, setEnvs] = useState<Environment[]>(initialEnvironments ?? []);
  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<Environment | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<Environment | null>(null);
  const [deleting, setDeleting] = useState(false);

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const refresh = useCallback(async () => {
    try {
      const res = await environments.list();
      setEnvs(res.environments || []);
    } catch { /* ignore */ }
  }, []);

  // Fetch on mount — server-side data may be empty due to missing auth token
  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleDelete = async (env: Environment) => {
    setDeleteConfirm(env);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    setDeleting(true);
    try {
      await environments.delete(deleteConfirm.id);
      showToast('Environment deleted', 'success');
      await refresh();
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    }
    setDeleting(false);
    setDeleteConfirm(null);
  };

  const handleSave = async (data: { name: string; slug?: string; type?: string; cluster?: string; namespace?: string; description?: string; color?: string; is_default?: boolean }) => {
    try {
      if (editing) {
        await environments.update(editing.id, data);
        showToast('Environment updated', 'success');
      } else {
        await environments.create({ ...data, status: 'active' });
        showToast('Environment created', 'success');
      }
      setShowCreate(false);
      setEditing(null);
      await refresh();
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && (
        <div className={`fixed top-4 right-4 z-50 px-4 py-2.5 rounded-lg text-[13px] shadow-lg ${
          toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
        }`}>
          {toast.message}
        </div>
      )}

      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Environments</h1>
          <p className="page-subtitle-modern">
            {envs.length} environment{envs.length !== 1 ? 's' : ''} configured
          </p>
        </div>
        <button onClick={() => { setEditing(null); setShowCreate(true); }} className="btn btn-primary btn-sm">
          + New Environment
        </button>
      </div>

      {/* Environments grid */}
      {envs.length === 0 ? (
        <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
          <div className="text-4xl mb-3 opacity-20">🌍</div>
          <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">No environments</h3>
          <p className="text-[12px] text-[var(--text-tertiary)]">
            Create environments to organize your deployments (dev, staging, production, etc.)
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {envs.map(env => (
            <Link key={env.id} href={`/environments?id=${env.id}`} className="card modern-card-hover block" style={{ borderRadius: '12px' }}>
              <div className="card-header flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span
                    className="w-3 h-3 rounded-full shrink-0"
                    style={{ backgroundColor: env.color || '#6B7280' }}
                  />
                  <div>
                    <span className="text-[14px] font-medium text-[var(--text-primary)]">{env.name}</span>
                    {env.is_default && (
                      <span className="ml-1.5 text-[10px] px-1.5 py-0.5 rounded-full bg-blue-500/10 text-blue-500 border border-blue-500/20">
                        default
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <button
                    onClick={(e) => { e.preventDefault(); setEditing(env); setShowCreate(true); }}
                    className="text-[11px] text-[var(--accent)] hover:underline px-1.5 py-0.5"
                  >
                    Edit
                  </button>
                  <button
                    onClick={(e) => { e.preventDefault(); handleDelete(env); }}
                    className="text-[11px] text-red-400 hover:text-red-600 px-1.5 py-0.5"
                  >
                    Delete
                  </button>
                </div>
              </div>
              <div className="card-body pt-0 space-y-2">
                {env.description && (
                  <p className="text-[12px] text-[var(--text-secondary)]">{env.description}</p>
                )}
                <div className="flex items-center gap-2 flex-wrap">
                  {env.slug && (
                    <span className="text-[11px] font-mono bg-[var(--bg)] px-1.5 py-0.5 rounded text-[var(--text-tertiary)]">{env.slug}</span>
                  )}
                  {env.type && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)] capitalize">{env.type}</span>
                  )}
                  {env.status && (
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${
                      env.status === 'active' ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-secondary)]'
                    }`}>{env.status}</span>
                  )}
                </div>
                <div className="text-[11px] text-[var(--text-tertiary)]">
                  Created: {new Date(env.created_at).toLocaleDateString()}
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}

      {/* Create/Edit Modal */}
      {showCreate && (
        <EnvModal
          env={editing}
          onSave={handleSave}
          onClose={() => { setShowCreate(false); setEditing(null); }}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteConfirm}
        title={`Delete environment "${deleteConfirm?.name || ''}"?`}
        description="This environment will be permanently removed. This action cannot be undone."
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

function EnvModal({ env, onSave, onClose }: {
  env: Environment | null;
  onSave: (data: { name: string; slug?: string; type?: string; cluster?: string; namespace?: string; description?: string; color?: string; is_default?: boolean }) => void;
  onClose: () => void;
}) {
  const [name, setName] = useState(env?.name || '');
  const [slug, setSlug] = useState(env?.slug || '');
  const [type, setType] = useState(env?.type || '');
  const [cluster, setCluster] = useState(env?.cluster || '');
  const [namespace, setNamespace] = useState(env?.namespace || '');
  const [description, setDescription] = useState(env?.description || '');
  const [color, setColor] = useState(env?.color || '#3B82F6');
  const [isDefault, setIsDefault] = useState(env?.is_default || false);
  useEscapeKey(onClose);

  const COLORS = ['#3B82F6', '#10B981', '#F59E0B', '#EF4444', '#8B5CF6', '#EC4899', '#06B6D4', '#6B7280'];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div className="relative w-full max-w-[480px] bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] p-6" onClick={e => e.stopPropagation()}>
        <h2 className="text-[16px] font-semibold text-[var(--text-primary)] mb-4">
          {env ? 'Edit Environment' : 'New Environment'}
        </h2>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Name</label>
              <input value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Production" className="input text-[13px] w-full" />
            </div>
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Slug</label>
              <input value={slug} onChange={e => setSlug(e.target.value)} placeholder="e.g. production" className="input text-[13px] font-mono w-full" />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Auto-generated if empty</p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Type</label>
              <input value={type} onChange={e => setType(e.target.value)} placeholder="e.g. development" className="input text-[13px] w-full" />
            </div>
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Cluster</label>
              <input value={cluster} onChange={e => setCluster(e.target.value)} placeholder="e.g. prod-cluster-1" className="input text-[13px] w-full" />
            </div>
          </div>
          <div>
            <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Namespace</label>
            <input value={namespace} onChange={e => setNamespace(e.target.value)} placeholder="e.g. default" className="input text-[13px] font-mono w-full" />
          </div>
          <div>
            <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Description</label>
            <textarea value={description} onChange={e => setDescription(e.target.value)} rows={2} placeholder="What is this environment for?" className="input text-[13px] w-full" />
          </div>
          <div>
            <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Color</label>
            <div className="flex items-center gap-2">
              {COLORS.map(c => (
                <button
                  key={c}
                  onClick={() => setColor(c)}
                  className={`w-6 h-6 rounded-full border-2 transition-all ${color === c ? 'border-[var(--text-primary)] scale-110' : 'border-transparent'}`}
                  style={{ backgroundColor: c }}
                />
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="isDefault" checked={isDefault} onChange={e => setIsDefault(e.target.checked)} className="rounded" />
            <label htmlFor="isDefault" className="text-[12px] text-[var(--text-secondary)]">Default environment</label>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-4 mt-4 border-t border-[var(--border)]">
          <button onClick={onClose} className="btn btn-secondary btn-sm">Cancel</button>
          <button onClick={() => onSave({ name, slug: slug || undefined, type: type || undefined, cluster: cluster || undefined, namespace: namespace || undefined, description: description || undefined, color, is_default: isDefault })} className="btn btn-primary btn-sm" disabled={!name.trim()}>
            {env ? 'Update' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
