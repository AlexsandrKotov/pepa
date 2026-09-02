'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { listUsers, createUser, updateUser, deactivateUser, resetUserPassword, rbac, type User, type Role } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import ConfirmModal from '@/components/ConfirmModal';
import { usePermission } from '@/hooks/usePermission';

export default function UsersPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <UsersPageContent />
    </PermissionGuard>
  );
}

function UsersPageContent() {
  const { hasPermission } = usePermission();
  const canWrite = hasPermission('settings', 'create') || hasPermission('settings', 'update') || hasPermission('settings', 'delete');
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [resetPasswordUser, setResetPasswordUser] = useState<User | null>(null);
  const [confirmAction, setConfirmAction] = useState<{ title: string; message: string; onConfirm: () => void } | null>(null);
  const [error, setError] = useState('');

  const loadUsers = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listUsers(search || undefined);
      setUsers(data.users);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users');
    } finally {
      setLoading(false);
    }
  }, [search]);

  useEffect(() => {
    loadUsers();
  }, [loadUsers]);

  async function handleDeactivate(id: string) {
    setConfirmAction({
      title: 'Deactivate User',
      message: 'Are you sure you want to deactivate this user? They will lose access to the platform.',
      onConfirm: async () => {
        try {
          await deactivateUser(id);
          loadUsers();
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Failed to deactivate user');
        }
      },
    });
  }

  async function handleToggleActive(user: User) {
    const action = user.is_active ? 'disable' : 'enable';
    setConfirmAction({
      title: `${user.is_active ? 'Disable' : 'Enable'} User`,
      message: `Are you sure you want to ${action} ${user.name}?`,
      onConfirm: async () => {
        try {
          await updateUser(user.id, { is_active: !user.is_active });
          loadUsers();
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Failed to update user');
        }
      },
    });
  }

  const roleLabels: Record<string, string> = {
    admin: 'Admin',
    developer: 'Developer',
    viewer: 'Viewer',
  };

  function roleBadgeColor(role: string) {
    switch (role) {
      case 'admin': return 'bg-purple-500/15 text-purple-600';
      case 'developer': return 'bg-blue-500/15 text-blue-600';
      case 'viewer': return 'bg-gray-500/15 text-gray-600';
      default: return 'bg-gray-500/15 text-gray-600';
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text-primary)] page-title-modern">Users</h1>
          <p className="text-sm text-[var(--text-tertiary)] mt-1 page-subtitle-modern">Manage user accounts and access</p>
        </div>
        {canWrite && (
          <button
            onClick={() => setShowCreate(true)}
            className="btn btn-primary"
          >
            Create User
          </button>
        )}
      </div>

      {error && (
        <div className="bg-red-500/10 border-red-500/20 text-red-500 px-4 py-3 rounded mb-4 text-sm">{error}</div>
      )}

      <div className="mb-4">
        <input
          type="text"
          placeholder="Search users..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="input max-w-md"
        />
      </div>

      {loading ? (
        <div className="text-center py-12 text-[var(--text-tertiary)]">Loading...</div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-[var(--bg)] border-b border-[var(--border)]">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-[var(--text-secondary)]">Name</th>
                <th className="text-left px-4 py-3 font-medium text-[var(--text-secondary)]">Email</th>
                <th className="text-left px-4 py-3 font-medium text-[var(--text-secondary)]">Roles</th>
                <th className="text-left px-4 py-3 font-medium text-[var(--text-secondary)]">Status</th>
                <th className="text-left px-4 py-3 font-medium text-[var(--text-secondary)]">Last Login</th>
                <th className="text-right px-4 py-3 font-medium text-[var(--text-secondary)]">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {users.map((user) => (
                <tr key={user.id} className="hover:bg-[var(--bg)]">
                  <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                    <div className="flex items-center gap-2">
                      {user.name}
                      {user.is_super_admin && (
                        <span className="px-1.5 py-0.5 text-[10px] font-semibold bg-amber-500/15 text-amber-600 rounded">SUPER ADMIN</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">{user.email}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {(user.roles || []).map((role) => (
                        <span key={role} className={`inline-flex px-2 py-0.5 text-xs rounded-full ${roleBadgeColor(role)}`}>
                          {roleLabels[role] || role}
                        </span>
                      ))}
                      {(!user.roles || user.roles.length === 0) && (
                        <span className="text-[var(--text-tertiary)] text-xs">No roles</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex px-2 py-1 text-xs rounded-full ${user.is_active ? 'bg-emerald-500/15 text-emerald-600' : 'bg-red-500/15 text-red-500'}`}>
                      {user.is_active ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-tertiary)] text-xs">
                    {user.last_login_at ? new Date(user.last_login_at).toLocaleString() : 'Never'}
                  </td>
                  <td className="px-4 py-3 text-right space-x-2">
                    {!user.is_super_admin && canWrite && (
                      <>
                        <button onClick={() => setEditingUser(user)} className="text-[12px] text-[var(--accent)] hover:text-[var(--text-primary)]">
                          Edit Roles
                        </button>
                        <button onClick={() => handleToggleActive(user)} className="text-[12px] text-[var(--accent)] hover:text-[var(--text-primary)]">
                          {user.is_active ? 'Disable' : 'Enable'}
                        </button>
                        <button onClick={() => setResetPasswordUser(user)} className="text-[12px] text-emerald-600 hover:text-emerald-500">
                          Reset Password
                        </button>
                        <button onClick={() => handleDeactivate(user.id)} className="text-[12px] text-red-500 hover:text-red-400">
                          Delete
                        </button>
                      </>
                    )}
                    {user.is_super_admin && (
                      <span className="text-[var(--text-tertiary)] text-xs italic">Protected</span>
                    )}
                  </td>
                </tr>
              ))}
              {users.length === 0 && (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-[var(--text-tertiary)]">No users found</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      {showCreate && <CreateUserModal onClose={() => setShowCreate(false)} onCreated={loadUsers} />}
      {editingUser && <EditUserRolesModal user={editingUser} onClose={() => setEditingUser(null)} onSaved={loadUsers} />}
      {resetPasswordUser && <ResetPasswordModal user={resetPasswordUser} onClose={() => setResetPasswordUser(null)} onSaved={loadUsers} />}
      {confirmAction && (
        <ConfirmModal
          open={!!confirmAction}
          title={confirmAction.title}
          description={confirmAction.message}
          confirmLabel="Confirm"
          variant="danger"
          onConfirm={async () => { await confirmAction.onConfirm(); setConfirmAction(null); }}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  );
}

function CreateUserModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  useEscapeKey(onClose);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [selectedRoles, setSelectedRoles] = useState<string[]>(['viewer']);
  const [roles, setRoles] = useState<Role[]>([]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    rbac.listRoles().then(data => {
      setRoles(data.roles);
      // Default to first role if available
      if (data.roles.length > 0 && selectedRoles[0] === 'viewer') {
        const viewerRole = data.roles.find(r => r.slug === 'viewer');
        if (viewerRole) setSelectedRoles(['viewer']);
        else setSelectedRoles([data.roles[0].slug]);
      }
    }).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function toggleRole(slug: string) {
    setSelectedRoles(prev =>
      prev.includes(slug) ? prev.filter(r => r !== slug) : [...prev, slug]
    );
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    if (selectedRoles.length === 0) {
      setError('Select at least one role');
      return;
    }
    setSaving(true);
    try {
      await createUser({ name, email, password, roles: selectedRoles });
      onCreated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create user');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4 max-h-[90vh] overflow-y-auto">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between sticky top-0 bg-[var(--surface)] z-10">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create User</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="p-5 space-y-4">
            {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}
            <div>
              <label className="label">Name</label>
              <input type="text" className="input" value={name} onChange={(e) => setName(e.target.value)} required placeholder="John Doe" />
            </div>
            <div>
              <label className="label">Email</label>
              <input type="email" className="input" value={email} onChange={(e) => setEmail(e.target.value)} required placeholder="john@example.com" />
            </div>
            <div>
              <label className="label">Password</label>
              <input type="password" className="input" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} placeholder="Min 8 characters" />
            </div>
            <div>
              <label className="label">Roles</label>
              {roles.length === 0 ? (
                <div className="text-[13px] text-[var(--text-tertiary)] py-2">Loading roles...</div>
              ) : (
                <div className="space-y-1.5">
                  {roles.map((role) => (
                    <label key={role.id} className="flex items-center gap-2.5 px-3 py-2 rounded-lg border border-[var(--border)] cursor-pointer hover:bg-[var(--bg)] transition-colors">
                      <input
                        type="checkbox"
                        checked={selectedRoles.includes(role.slug)}
                        onChange={() => toggleRole(role.slug)}
                        className="rounded border-[var(--border)]"
                      />
                      <div className="flex-1">
                        <div className="text-[13px] font-medium text-[var(--text-primary)]">{role.name}</div>
                        {role.description && <div className="text-[11px] text-[var(--text-tertiary)]">{role.description}</div>}
                      </div>
                    </label>
                  ))}
                </div>
              )}
            </div>
          </div>
          <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2 sticky bottom-0 bg-[var(--surface)]">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={saving} className="btn btn-primary">
              {saving ? 'Creating...' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function EditUserRolesModal({ user, onClose, onSaved }: { user: User; onClose: () => void; onSaved: () => void }) {
  useEscapeKey(onClose);
  const [selectedRoles, setSelectedRoles] = useState<string[]>(user.roles || []);
  const [roles, setRoles] = useState<Role[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    rbac.listRoles().then(data => setRoles(data.roles)).catch(() => {});
  }, []);

  function toggleRole(slug: string) {
    setSelectedRoles(prev =>
      prev.includes(slug) ? prev.filter(r => r !== slug) : [...prev, slug]
    );
  }

  async function handleSave() {
    if (selectedRoles.length === 0) {
      setError('Select at least one role');
      return;
    }
    setSaving(true);
    setError('');
    try {
      await updateUser(user.id, { roles: selectedRoles });
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update roles');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4 max-h-[90vh] overflow-y-auto">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between sticky top-0 bg-[var(--surface)] z-10">
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Edit Roles</h2>
            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">{user.name} ({user.email})</p>
          </div>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div className="p-5 space-y-4">
          {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}
          {roles.length === 0 ? (
            <div className="text-[13px] text-[var(--text-tertiary)] py-2">Loading roles...</div>
          ) : (
            <div className="space-y-1.5">
              {roles.map((role) => (
                <label key={role.id} className="flex items-center gap-2.5 px-3 py-2 rounded-lg border border-[var(--border)] cursor-pointer hover:bg-[var(--bg)] transition-colors">
                  <input
                    type="checkbox"
                    checked={selectedRoles.includes(role.slug)}
                    onChange={() => toggleRole(role.slug)}
                    className="rounded border-[var(--border)]"
                  />
                  <div className="flex-1">
                    <div className="text-[13px] font-medium text-[var(--text-primary)]">{role.name}</div>
                    {role.description && <div className="text-[11px] text-[var(--text-tertiary)]">{role.description}</div>}
                  </div>
                </label>
              ))}
            </div>
          )}
        </div>
        <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2 sticky bottom-0 bg-[var(--surface)]">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button onClick={handleSave} disabled={saving} className="btn btn-primary">
            {saving ? 'Saving...' : 'Save Roles'}
          </button>
        </div>
      </div>
    </div>
  );
}

function ResetPasswordModal({ user, onClose, onSaved }: { user: User; onClose: () => void; onSaved: () => void }) {
  useEscapeKey(onClose);
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }
    setSaving(true);
    try {
      await resetUserPassword(user.id, newPassword);
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between">
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Reset Password</h2>
            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">{user.name} ({user.email})</p>
          </div>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="p-5 space-y-4">
            {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}
            <div>
              <label className="label">New Password</label>
              <input type="password" className="input" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} required minLength={8} placeholder="Min 8 characters" />
            </div>
            <div>
              <label className="label">Confirm Password</label>
              <input type="password" className="input" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} required placeholder="Re-enter password" />
            </div>
            <div className="px-0 py-1">
              <p className="text-[11px] text-[var(--text-tertiary)]">Must contain uppercase, lowercase, digit, and special character.</p>
            </div>
          </div>
          <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={saving} className="btn btn-primary">
              {saving ? 'Resetting...' : 'Reset Password'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
