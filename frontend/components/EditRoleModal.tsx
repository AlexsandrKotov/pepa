'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { rbac, type Role } from '@/lib/api';

interface EditRoleModalProps {
  open: boolean;
  role: Role | null;
  onClose: () => void;
  onUpdated: () => void;
}

const resources = ['entities', 'workflows', 'plugins', 'scorecards', 'audit', 'plugin_activity', 'roles', 'services', 'deployments', 'clusters', 'connections', 'environments', 'pipelines', 'vault', 'gitops', 'settings', 'policies', 'docker', 'helm', 'discovery', 'import', 'ai', 'jira', 'credentials', 'virtualization'];
const actions = ['read', 'create', 'update', 'delete'];

export default function EditRoleModal({ open, role, onClose, onUpdated }: EditRoleModalProps) {
  useEscapeKey(onClose, open);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [scope, setScope] = useState('tenant');
  const [permissions, setPermissions] = useState<Record<string, string[]>>({});
  const [existingPerms, setExistingPerms] = useState<Array<{ id: string; resource: string; action: string }>>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (role && open) {
      setName(role.name);
      setDescription(role.description || '');
      setScope(role.scope || 'tenant');
      loadPermissions(role.id);
    }
  }, [role, open]);

  const loadPermissions = async (roleId: string) => {
    try {
      const data = await rbac.getPermissions(roleId);
      const perms = data.permissions || [];
      setExistingPerms(perms.map(p => ({ id: p.id, resource: p.resource, action: p.action })));
      
      // Build permission map for UI
      const permMap: Record<string, string[]> = {};
      perms.forEach(p => {
        if (p.effect === 'allow') {
          if (!permMap[p.resource]) permMap[p.resource] = [];
          permMap[p.resource].push(p.action);
        }
      });
      setPermissions(permMap);
    } catch { /* ignore */ }
  };

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
    if (!role) return;
    
    setSaving(true);
    setError('');
    try {
      // Update role metadata
      await rbac.updateRole(role.id, { name: name.trim(), description, scope });
      
      // Calculate permission changes
      const currentPerms = new Set(
        existingPerms.filter(p => p.resource && p.action).map(p => `${p.resource}:${p.action}`)
      );
      const desiredPerms = new Set<string>();
      Object.entries(permissions).forEach(([resource, acts]) => {
        acts.forEach(action => desiredPerms.add(`${resource}:${action}`));
      });

      // Add new permissions
      for (const perm of desiredPerms) {
        if (!currentPerms.has(perm)) {
          const [resource, action] = perm.split(':');
          await rbac.addPermission(role.id, resource, action);
        }
      }

      // Remove old permissions
      for (const perm of currentPerms) {
        if (!desiredPerms.has(perm)) {
          const [resource, action] = perm.split(':');
          const existingPerm = existingPerms.find(p => p.resource === resource && p.action === action);
          if (existingPerm) {
            await rbac.removePermission(role.id, existingPerm.id);
          }
        }
      }

      onUpdated();
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to update role');
    } finally {
      setSaving(false);
    }
  };

  if (!open || !role) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg max-h-[90vh] overflow-y-auto mx-4">
        <div className="sticky top-0 bg-[var(--surface)] border-b border-[var(--border)] px-5 py-3 flex items-center justify-between z-10">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Edit Role</h2>
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

          {role.is_system && (
            <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20 text-[12px] text-blue-500">
              This is a system role. You can customize its permissions.
            </div>
          )}

          <div>
            <label className="label">Role Name *</label>
            <input type="text" className="input" placeholder="e.g., Developer" value={name}
              onChange={e => setName(e.target.value)} />
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
            {saving ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  );
}
