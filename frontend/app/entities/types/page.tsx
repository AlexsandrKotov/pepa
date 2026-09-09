'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { entityTypes, type EntityType } from '@/lib/api';
import { Toast } from '@/components/Interactive';

export default function EntityTypesPage() {
  const [types, setTypes] = useState<EntityType[]>([]);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ type_key: '', display_name: '', category: '', description: '' });

  useEffect(() => {
    entityTypes.list()
      .then(data => setTypes(data.entity_types || []))
      .catch(() => setTypes([]))
      .finally(() => setLoading(false));
  }, []);

  const categoryBadge = (category: string) => {
    const colors: Record<string, string> = {
      compute: 'bg-blue-500/10 text-blue-500',
      organization: 'bg-purple-500/10 text-purple-500',
      deployment: 'bg-emerald-500/10 text-emerald-600',
      interface: 'bg-amber-500/10 text-amber-600',
    };
    return colors[category] || 'bg-slate-500/10 text-slate-500';
  };

  if (loading) return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6"><h1 className="page-title-modern">Entity Types</h1>
      <div className="card card-body text-center py-12"><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div></div>
    </div>
  );

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Entity Types</h1>
            <p className="page-subtitle-modern">Manage the types of entities available in your catalog</p>
          </div>
          <div className="flex gap-2">
            <Link href="/entities" className="btn btn-secondary btn-sm">Back to Entities</Link>
            <button onClick={() => setShowCreate(!showCreate)} className="btn btn-primary btn-sm">+ New Type</button>
          </div>
        </div>

        {showCreate && (
          <div className="card page-animate-up" style={{ borderRadius: '12px' }}>
            <div className="card-header"><span className="text-[13px] font-medium">New Entity Type</span></div>
            <div className="card-body space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div><label className="label">Type Key *</label><input value={form.type_key} onChange={e => setForm({ ...form, type_key: e.target.value })} className="input font-mono" placeholder="e.g. database" /></div>
                <div><label className="label">Display Name *</label><input value={form.display_name} onChange={e => setForm({ ...form, display_name: e.target.value })} className="input" placeholder="e.g. Database" /></div>
                <div><label className="label">Category</label><input value={form.category} onChange={e => setForm({ ...form, category: e.target.value })} className="input" placeholder="e.g. compute, organization" /></div>
                <div><label className="label">Description</label><input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="What this type represents" /></div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => { setShowCreate(false); setForm({ type_key: '', display_name: '', category: '', description: '' }); }} className="btn btn-secondary btn-sm">Cancel</button>
              </div>
            </div>
          </div>
        )}

        <div className="card" style={{ borderRadius: '12px' }}>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Type Key</th>
                  <th>Display Name</th>
                  <th>Category</th>
                  <th>System</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {types.map(t => (
                  <tr key={t.id}>
                    <td><span className="font-mono text-[12px] text-[var(--text-primary)]">{t.type_key}</span></td>
                    <td className="text-[13px] text-[var(--text-primary)]">{t.display_name}</td>
                    <td><span className={`text-[11px] px-1.5 py-0.5 rounded ${categoryBadge(t.category)}`}>{t.category}</span></td>
                    <td>{t.is_system ? <span className="text-[11px] text-[var(--text-tertiary)]">system</span> : <span className="text-[11px] text-[var(--accent)]">custom</span>}</td>
                    <td>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded ${t.is_enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>
                        {t.is_enabled ? 'enabled' : 'disabled'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
