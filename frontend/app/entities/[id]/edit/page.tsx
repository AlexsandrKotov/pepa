'use client';

import { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { entities, entityTypes, type EntityType, type Entity } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';

const statusOptions = [
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' },
  { value: 'deprecated', label: 'Deprecated' },
];

export default function EditEntityPage() {
  return (
    <Suspense fallback={<div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6"><h1 className="page-title-modern">Edit Entity</h1><div className="card card-body text-center py-12 page-animate" style={{ borderRadius: '12px' }}><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div></div></div>}>
      <EditEntityForm />
    </Suspense>
  );
}

function EditEntityForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const entityId = searchParams.get('id');

  const [types, setTypes] = useState<EntityType[]>([]);
  const [entity, setEntity] = useState<Entity | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Form state
  const [name, setName] = useState('');
  const [typeKey, setTypeKey] = useState('');
  const [description, setDescription] = useState('');
  const [status, setStatus] = useState('active');
  const [externalId, setExternalId] = useState('');
  const [metadataEntries, setMetadataEntries] = useState<{ key: string; value: string }[]>([{ key: '', value: '' }]);

  useEffect(() => {
    if (!entityId) return;
    Promise.all([
      entities.get(entityId).catch(() => null),
      entityTypes.list().then(d => d.entity_types || []).catch(() => []),
    ]).then(([ent, types]) => {
      setEntity(ent);
      setTypes(types.filter(t => t.is_enabled));
      if (ent) {
        setName(ent.name);
        setTypeKey(ent.type_key);
        setDescription(ent.description || '');
        setStatus(ent.status);
        setExternalId(ent.external_id || '');
        const meta = ent.metadata || {};
        const entries = Object.entries(meta).map(([key, value]) => ({ key, value: String(value) }));
        setMetadataEntries(entries.length > 0 ? entries : [{ key: '', value: '' }]);
      }
    }).finally(() => setLoading(false));
  }, [entityId]);

  const addMetadataRow = () => setMetadataEntries([...metadataEntries, { key: '', value: '' }]);

  const updateMetadataKey = (index: number, key: string) => {
    const updated = [...metadataEntries];
    updated[index] = { ...updated[index], key };
    setMetadataEntries(updated);
  };

  const updateMetadataValue = (index: number, value: string) => {
    const updated = [...metadataEntries];
    updated[index] = { ...updated[index], value };
    setMetadataEntries(updated);
  };

  const removeMetadataRow = (index: number) => {
    setMetadataEntries(metadataEntries.filter((_, i) => i !== index));
  };

  const buildMetadata = (): Record<string, unknown> | undefined => {
    const entries = metadataEntries.filter(e => e.key.trim());
    if (entries.length === 0) return undefined;
    const meta: Record<string, unknown> = {};
    for (const e of entries) {
      try { meta[e.key] = JSON.parse(e.value); } catch { meta[e.key] = e.value; }
    }
    return meta;
  };

  const canSubmit = name.trim() && typeKey && !saving;

  const handleSave = async () => {
    if (!canSubmit || !entityId) return;
    setSaving(true);
    try {
      const payload: Record<string, unknown> = {
        name: name.trim(),
        description: description.trim() || undefined,
        status,
      };
      const metadata = buildMetadata();
      if (metadata) payload.metadata = metadata;

      const updated = await entities.update(entityId, payload);
      setEntity(updated);
      setToast({ message: 'Entity saved successfully', type: 'success' });
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to save', type: 'error' });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!entityId) return;
    setDeleting(true);
    try {
      await entities.delete(entityId);
      setToast({ message: 'Entity deleted', type: 'success' });
      setTimeout(() => router.push('/entities'), 600);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to delete', type: 'error' });
      setDeleting(false);
    }
  };

  if (loading) return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6"><h1 className="page-title-modern">Edit Entity</h1>
      <div className="card card-body text-center py-12"><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div></div>
    </div>
  );

  if (!entity) return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg flex items-center justify-center py-20">
      <div className="text-center">
        <p className="text-[14px] font-medium text-[var(--text-primary)] mb-1">Entity not found</p>
        <Link href="/entities" className="btn btn-primary mt-4">Back to Entities</Link>
      </div>
    </div>
  );

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
        <ConfirmModal
          open={showDeleteConfirm}
          title="Delete Entity"
          description={`Are you sure you want to delete "${entity.name}"? This action cannot be undone.`}
          confirmLabel="Delete"
          variant="danger"
          onConfirm={handleDelete}
          onCancel={() => setShowDeleteConfirm(false)}
        />

        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Edit Entity</h1>
              <span className="badge badge-default">{entity.type_key}</span>
            </div>
            <p className="page-subtitle-modern">
              <span className="text-mono text-[11px]">{entity.id.slice(0, 8)}</span>
              {entity.external_id && <span className="ml-2">External: {entity.external_id}</span>}
            </p>
          </div>
          <Link href={`/entities?id=${entity.id}`} className="btn btn-secondary btn-sm">Back to Entity</Link>
        </div>

        {/* Form */}
        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Entity Details</span>
          </div>
          <div className="card-body space-y-5">
            {/* Name */}
            <div>
              <label className="label">Name <span className="text-red-500">*</span></label>
              <input
                value={name}
                onChange={e => setName(e.target.value)}
                className="input"
                placeholder="Entity name"
              />
            </div>

            {/* Type (read-only) */}
            <div>
              <label className="label">Type</label>
              <div className="text-[13px] text-[var(--text-secondary)] py-1.5 px-3 rounded-lg bg-[var(--bg)] border border-[var(--border-light)]">
                {types.find(t => t.type_key === typeKey)?.display_name || typeKey}
                <span className="text-[11px] text-[var(--text-tertiary)] ml-2">(cannot be changed)</span>
              </div>
            </div>

            {/* Description */}
            <div>
              <label className="label">Description</label>
              <textarea
                value={description}
                onChange={e => setDescription(e.target.value)}
                className="input min-h-[60px] resize-y"
                placeholder="Brief description..."
              />
            </div>

            {/* Status */}
            <div>
              <label className="label">Status</label>
              <div className="flex gap-2">
                {statusOptions.map(opt => (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setStatus(opt.value)}
                    className={`px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-all ${
                      status === opt.value
                        ? opt.value === 'active' ? 'border-emerald-500/50 bg-emerald-500/10 text-emerald-600'
                          : opt.value === 'deprecated' ? 'border-red-500/50 bg-red-500/10 text-red-500'
                          : 'border-amber-500/50 bg-amber-500/10 text-amber-600'
                        : 'border-[var(--border-light)] text-[var(--text-tertiary)] hover:border-[var(--border)]'
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>

            {/* External ID (read-only, used for deduplication) */}
            {entity.external_id && (
              <div>
                <label className="label">External ID</label>
                <input
                  value={externalId}
                  readOnly
                  className="input font-mono text-[12px] bg-[var(--bg-tertiary)] cursor-not-allowed"
                  placeholder="External system identifier"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Immutable identifier used for deduplication with external systems</p>
              </div>
            )}
          </div>
        </div>

        {/* Metadata */}
        <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Metadata</span>
            <button type="button" onClick={addMetadataRow} className="text-[11px] text-[var(--accent)] hover:underline">+ Add field</button>
          </div>
          <div className="card-body space-y-2">
            {metadataEntries.map((entry, i) => (
              <div key={i} className="flex gap-2 items-center">
                <input
                  value={entry.key}
                  onChange={e => updateMetadataKey(i, e.target.value)}
                  className="input flex-1"
                  placeholder="Key"
                />
                <input
                  value={entry.value}
                  onChange={e => updateMetadataValue(i, e.target.value)}
                  className="input flex-1"
                  placeholder="Value"
                />
                <button type="button" onClick={() => removeMetadataRow(i)} className="text-[var(--text-tertiary)] hover:text-red-500 text-[14px] px-1">✕</button>
              </div>
            ))}
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between page-animate-up page-delay-3">
          <button
            onClick={() => setShowDeleteConfirm(true)}
            disabled={deleting}
            className="btn btn-secondary text-red-500 hover:bg-red-500/10"
          >
            {deleting ? 'Deleting...' : 'Delete Entity'}
          </button>
          <div className="flex gap-3">
            <Link href={`/entities?id=${entityId}`} className="btn btn-secondary">Cancel</Link>
            <button
              onClick={handleSave}
              disabled={!canSubmit}
              className="btn btn-primary"
            >
              {saving ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
