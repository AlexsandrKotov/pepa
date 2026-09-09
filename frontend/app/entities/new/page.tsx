'use client';

import { useState, useEffect, Suspense } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { entities, entityTypes, type EntityType } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConceptHelp from '@/components/ConceptHelp';

const statusOptions = [
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' },
  { value: 'deprecated', label: 'Deprecated' },
];

export default function NewEntityPage() {
  return (
    <Suspense fallback={<div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6"><h1 className="page-title-modern">Create Entity</h1><div className="card card-body text-center py-12 page-animate" style={{ borderRadius: '12px' }}><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div></div></div>}>
      <NewEntityForm />
    </Suspense>
  );
}

function NewEntityForm() {
  const router = useRouter();
  const [types, setTypes] = useState<EntityType[]>([]);
  const [loadingTypes, setLoadingTypes] = useState(true);
  const [creating, setCreating] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Form state
  const [name, setName] = useState('');
  const [typeKey, setTypeKey] = useState('');
  const [description, setDescription] = useState('');
  const [status, setStatus] = useState('active');
  const [metadataEntries, setMetadataEntries] = useState<{ key: string; value: string }[]>([{ key: '', value: '' }]);

  useEffect(() => {
    entityTypes.list()
      .then(data => setTypes((data.entity_types || []).filter(t => t.is_enabled)))
      .catch(() => setTypes([]))
      .finally(() => setLoadingTypes(false));
  }, []);

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
      // Try to parse as JSON, otherwise use as string
      try { meta[e.key] = JSON.parse(e.value); } catch { meta[e.key] = e.value; }
    }
    return meta;
  };

  const canSubmit = name.trim() && typeKey && !creating;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setCreating(true);
    try {
      const payload: Record<string, unknown> = {
        name: name.trim(),
        type_key: typeKey,
        status,
      };
      if (description.trim()) payload.description = description.trim();
      const metadata = buildMetadata();
      if (metadata) payload.metadata = metadata;

      const entity = await entities.create(payload);
      setToast({ message: `Entity "${entity.name}" created`, type: 'success' });
      setTimeout(() => router.push(`/entities?id=${entity.id}`), 600);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to create entity', type: 'error' });
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        {/* Header */}
        <div className="page-animate">
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Create Entity</h1>
            <ConceptHelp term="entity" />
          </div>
          <p className="page-subtitle-modern">Register a new service, team, environment, or custom resource</p>
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
                placeholder="e.g. payment-service, backend-team, staging"
                autoFocus
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Unique name for this entity in the catalog</p>
            </div>

            {/* Type */}
            <div>
              <label className="label">Type <span className="text-red-500">*</span></label>
              {loadingTypes ? (
                <div className="text-[12px] text-[var(--text-tertiary)] py-2">Loading types...</div>
              ) : (
                <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                  {types.map(t => (
                    <button
                      key={t.type_key}
                      type="button"
                      onClick={() => setTypeKey(t.type_key)}
                      className={`p-3 rounded-lg border text-left transition-all ${
                        typeKey === t.type_key
                          ? 'border-[var(--accent)] bg-[var(--accent)]/5 ring-1 ring-[var(--accent)]'
                          : 'border-[var(--border-light)] hover:border-[var(--border)] hover:bg-[var(--bg)]'
                      }`}
                    >
                      <div className="text-[12px] font-medium text-[var(--text-primary)]">{t.display_name}</div>
                      <div className="text-[10px] text-[var(--text-tertiary)] mt-0.5">{t.category}</div>
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Description */}
            <div>
              <label className="label">Description</label>
              <textarea
                value={description}
                onChange={e => setDescription(e.target.value)}
                className="input min-h-[60px] resize-y"
                placeholder="Brief description of this entity..."
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
          </div>
        </div>

        {/* Metadata */}
        <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Metadata</span>
            <button type="button" onClick={addMetadataRow} className="text-[11px] text-[var(--accent)] hover:underline">+ Add field</button>
          </div>
          <div className="card-body space-y-2">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-3">Custom key-value properties (e.g. owner, repository, health_endpoint)</p>
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
                {metadataEntries.length > 1 && (
                  <button type="button" onClick={() => removeMetadataRow(i)} className="text-[var(--text-tertiary)] hover:text-red-500 text-[14px] px-1">✕</button>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-3 page-animate-up page-delay-3">
          <button
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="btn btn-primary"
          >
            {creating ? 'Creating...' : 'Create Entity'}
          </button>
          <Link href="/entities" className="btn btn-secondary">Cancel</Link>
        </div>
      </div>
    </div>
  );
}
