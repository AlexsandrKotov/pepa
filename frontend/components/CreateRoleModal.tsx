'use client';

import { useState } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { rbac, type Role } from '@/lib/api';

interface CreateRoleModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

const resources = ['entities', 'workflows', 'plugins', 'scorecards', 'audit', 'roles', 'services', 'deployments', 'clusters', 'connections', 'environments', 'pipelines', 'vault', 'gitops', 'settings', 'policies', 'docker', 'helm', 'discovery', 'import', 'ai', 'jira', 'credentials', 'virtualization'];
const actions = ['read', 'create', 'update', 'delete'];

export default function CreateRoleModal({ open, onClose, onCreated }: CreateRoleModalProps) {
  useEscapeKey(onClose, open);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [scope, setScope] = useState('tenant');
  const [permissions, setPermissions] = useState<Record<string, string[]>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const autoSlug = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

  const togglePermission = (resource: string, action: string) => {
    setPermissions(prev => {
      const current = prev[resource] || [];
      const next = current.includes(action)
        ? current.filter(a => a !== action)
        : [...current, action];
      return { ...prev, [resource]: next };
    });
  };

  const hasPerm = (resource: string, action: string) => (permissions[resource] || []).includes(action);

  const handleSubmit = async () => {
    if (!name.trim()) { setError('Name is required'); return; }
    setSaving(true);
    setError('');
    try {
      const role = await rbac.createRole({ name: name.trim(), slug: slug || autoSlug(name), description, scope });
      // Add permissions
      const roleId = (role as unknown as Record<string, unknown>).id as string;
      if (roleId) {
        for (const [resource, acts] of Object.entries(permissions)) {
          for (const action of acts) {
            await rbac.addPermission(roleId, resource, action);
          }
        }
      }
      setName(''); setSlug(''); setDescription(''); setPermissions({});
      onCreated();
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create role');
    } finally {
      setSaving(false);
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg max-h-[90vh] overflow-y-auto mx-4">
        <div className="sticky top-0 bg-[var(--surface)] border-b border-[var(--border)] px-5 py-3 flex items-center justify-between z-10">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create Role</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div className="p-5 space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>
          )}

          <div>
            <label className="label">Role Name *</label>
            <input type="text" className="input" placeholder="e.g., Developer" value={name}
              onChange={e => { setName(e.target.value); if (!slug) setSlug(autoSlug(e.target.value)); }} />
          </div>

          <div>
            <label className="label">Slug</label>
            <input type="text" className="input" placeholder="e.g., developer" value={slug}
              onChange={e => setSlug(e.target.value)} />
          </div>

          <div>
            <label className="label">Description</label>
            <textarea className="input" rows={2} placeholder="What can this role do?"
              value={description} onChange={e => setDescription(e.target.value)} />
          </div>

          <div>
            <label className="label">Scope</label>
            <select className="input" value={scope} onChange={e => setScope(e.target.value)}>
              <option value="tenant">Tenant</option>
              <option value="global">Global</option>
              <option value="organization">Organization</option>
            </select>
          </div>

          {/* Permission Matrix */}
          <div>
            <label className="label">Permissions</label>
            <div className="border border-[var(--border)] rounded-lg overflow-hidden">
              <table className="w-full text-[12px]">
                <thead>
                  <tr className="bg-[var(--bg)] border-b border-[var(--border)]">
                    <th className="text-left py-2 px-3 text-[var(--text-secondary)] font-medium">Resource</th>
                    {actions.map(a => (
                      <th key={a} className="text-center py-2 px-2 text-[var(--text-secondary)] font-medium capitalize">{a}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {resources.map(res => (
                    <tr key={res} className="border-b border-[var(--border-light)] last:border-0">
                      <td className="py-1.5 px-3 text-[var(--text-primary)] capitalize">{res}</td>
                      {actions.map(act => (
                        <td key={act} className="text-center py-1.5 px-2">
                          <button
                            onClick={() => togglePermission(res, act)}
                            className={`w-6 h-6 rounded transition-colors ${
                              hasPerm(res, act)
                                ? 'bg-[var(--accent)] text-white'
                                : 'bg-[var(--border-light)] text-[var(--text-tertiary)] hover:bg-[var(--border)]'
                            }`}
                          >
                            {hasPerm(res, act) ? '✓' : ''}
                          </button>
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div className="sticky bottom-0 bg-[var(--surface)] border-t border-[var(--border)] px-5 py-3 flex justify-end gap-2">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button onClick={handleSubmit} disabled={saving || !name.trim()} className="btn btn-primary">
            {saving ? 'Creating...' : 'Create Role'}
          </button>
        </div>
      </div>
    </div>
  );
}
