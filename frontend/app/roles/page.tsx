'use client';

import { useState, useEffect, useCallback } from 'react';
import { rbac, listTeams, listUsers, type Role, type Team, type User } from '@/lib/api';
import CreateRoleModal from '@/components/CreateRoleModal';
import EditRoleModal from '@/components/EditRoleModal';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';
import ConfirmModal from '@/components/ConfirmModal';

interface RoleWithPerms extends Role {
  permissions: Array<{ id: string; resource: string; action: string; effect: string }>;
}

interface Assignment {
  id: string;
  tenant_id: string;
  user_id?: string;
  team_id?: string;
  role_id: string;
  role_name: string;
  is_active: boolean;
  user_email?: string;
  user_name?: string;
  team_name?: string;
}

const resources = ['entities', 'workflows', 'plugins', 'scorecards', 'audit', 'roles', 'services', 'deployments', 'clusters', 'connections', 'environments', 'pipelines', 'vault', 'gitops', 'settings', 'docker', 'helm', 'discovery', 'import', 'ai', 'jira', 'credentials', 'virtualization'];
const actions = ['read', 'create', 'update', 'delete'];

export default function RolesPage() {
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('roles', 'read')) {
    return <ForbiddenPage resource="roles" />;
  }

  return <RolesPageContent />;
}

function RolesPageContent() {
  const [roles, setRoles] = useState<RoleWithPerms[]>([]);
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [editRole, setEditRole] = useState<RoleWithPerms | null>(null);
  const [assignOpen, setAssignOpen] = useState(false);
  const [assignRoleId, setAssignRoleId] = useState<string>('');
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(null);
  const [revokeConfirm, setRevokeConfirm] = useState<string | null>(null);
  const [revoking, setRevoking] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [rolesData, assignData] = await Promise.all([
        rbac.listRoles().catch(() => ({ roles: [], total: 0 })),
        rbac.listAssignments().catch(() => ({ assignments: [], total: 0 })),
      ]);
      const rolesList = rolesData.roles || [];
      const withPerms = await Promise.all(
        rolesList.map(async (role) => {
          const perms = await rbac.getPermissions(role.id).catch(() => ({ permissions: [], total: 0 }));
          const permissions = (perms.permissions || []).map((p) => ({ id: String(p.id), resource: p.resource, action: p.action, effect: p.effect }));
          return { ...role, permissions };
        })
      );
      setRoles(withPerms);
      setAssignments((assignData.assignments || []) as unknown as Assignment[]);
    } catch { /* ignore */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  const handleEdit = (role: RoleWithPerms) => {
    setEditRole(role);
  };

  const handleAssign = (roleId: string) => {
    setAssignRoleId(roleId);
    setAssignOpen(true);
  };

  const handleRevoke = async (assignmentId: string) => {
    setRevokeConfirm(assignmentId);
  };

  const confirmRevoke = async () => {
    if (!revokeConfirm) return;
    setRevoking(true);
    try {
      await rbac.revokeAssignment(revokeConfirm);
      loadData();
      setFeedback({ ok: true, text: 'Assignment revoked' });
    } catch (e: unknown) {
      setFeedback({ ok: false, text: e instanceof Error ? e.message : 'Failed' });
    }
    setRevoking(false);
    setRevokeConfirm(null);
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <h1 className="page-title-modern">Roles &amp; Permissions</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-tertiary)]">Loading roles...</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Roles &amp; Permissions</h1>
            <p className="page-subtitle-modern">{roles.length} roles &middot; {assignments.filter(a => a.is_active).length} active assignments</p>
          </div>
          <button onClick={() => setCreateOpen(true)} className="btn btn-primary">+ Create Role</button>
        </div>
      </div>

      {/* Feedback */}
      {feedback && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 page-animate-up ${
          feedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <div>
            <p className={`text-sm font-medium ${feedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {feedback.ok ? '✓ ' : '⚠ '}{feedback.text}
            </p>
          </div>
          <button onClick={() => setFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      {/* Roles Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 page-animate-up page-delay-1">
        {roles.map((role) => (
          <div key={role.id} className="card card-body modern-card-hover" style={{ borderRadius: '12px' }}>
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <div className={`w-8 h-8 rounded-md flex items-center justify-center text-[12px] font-semibold text-white ${
                  role.is_system ? 'bg-[var(--text-tertiary)]' : 'bg-[var(--accent)]'
                }`}>
                  {role.name.charAt(0)}
                </div>
                <div>
                  <h3 className="text-[14px] font-medium text-[var(--text-primary)]">{role.name}</h3>
                  <p className="text-[12px] text-[var(--text-tertiary)]">{role.slug} &middot; {role.scope}</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {role.is_system && (
                  <span className="text-[11px] text-[var(--text-tertiary)] border border-[var(--border)] rounded px-1.5 py-0.5">system</span>
                )}
                <span className="text-[12px] text-[var(--text-tertiary)]">{role.permissions.length} perms</span>
              </div>
            </div>

            {role.description && (
              <p className="text-[13px] text-[var(--text-secondary)] mb-3">{role.description}</p>
            )}

            {/* Permission Matrix */}
            <div className="overflow-x-auto">
              <table className="w-full text-[12px]">
                <thead>
                  <tr className="border-b border-[var(--border-light)]">
                    <th className="text-left py-1.5 pr-3 text-[var(--text-tertiary)] font-normal">Resource</th>
                    {actions.map(a => (
                      <th key={a} className="text-center py-1.5 px-2 text-[var(--text-tertiary)] font-normal capitalize">{a}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {resources.map(res => (
                    <tr key={res} className="border-b border-[var(--border-light)] last:border-0">
                      <td className="py-1.5 pr-3 text-[var(--text-secondary)] capitalize">{res}</td>
                      {actions.map(act => {
                        const has = role.permissions.some(
                          p => p.resource === res && p.action === act && p.effect === 'allow'
                        );
                        return (
                          <td key={act} className="text-center py-1.5 px-2">
                            {has ? (
                              <span className="inline-block w-4 h-4 rounded bg-[var(--accent)] text-white text-[10px] leading-4">&#10003;</span>
                            ) : (
                              <span className="inline-block w-4 h-4 rounded bg-[var(--border-light)] text-[var(--text-tertiary)] text-[10px] leading-4">&mdash;</span>
                            )}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Role assignments count + actions */}
            <div className="mt-3 pt-3 border-t border-[var(--border-light)] flex items-center justify-between">
              <span className="text-[12px] text-[var(--text-tertiary)]">
                {assignments.filter(a => a.role_id === role.id && a.is_active).length} active assignments
              </span>
              <div className="flex gap-2">
                <button onClick={() => handleAssign(role.id)} className="text-[12px] text-[var(--accent)] hover:text-[var(--text-primary)]">
                  Assign
                </button>
                <button onClick={() => handleEdit(role)} className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
                  Edit
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Assignments */}
      <div>
        <h2 className="section-title mb-3">Assignments</h2>
        {assignments.filter(a => a.is_active).length === 0 ? (
          <div className="card card-body text-center py-8">
            <p className="text-[13px] text-[var(--text-tertiary)]">No role assignments yet. Assign roles to users or teams.</p>
          </div>
        ) : (
          <div className="card overflow-hidden">
            <table className="w-full text-[13px]">
              <thead>
                <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--text-secondary)]">Target</th>
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--text-secondary)]">Type</th>
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--text-secondary)]">Role</th>
                  <th className="text-left px-4 py-2.5 font-medium text-[var(--text-secondary)]">Status</th>
                  <th className="text-right px-4 py-2.5 font-medium text-[var(--text-secondary)]">Actions</th>
                </tr>
              </thead>
              <tbody>
                {assignments.filter(a => a.is_active).map(a => (
                  <tr key={String(a.id)} className="border-b border-[var(--border-light)] last:border-0">
                    <td className="px-4 py-2.5 text-[var(--text-primary)]">
                      {a.team_id ? (
                        <span className="font-medium">{a.team_name || 'Unknown Team'}</span>
                      ) : (
                        <div>
                          <div className="font-medium">{a.user_name || 'Unknown User'}</div>
                          {a.user_email && <div className="text-[11px] text-[var(--text-tertiary)]">{a.user_email}</div>}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <span className={`text-[11px] rounded px-1.5 py-0.5 ${
                        a.team_id ? 'bg-blue-500/10 text-blue-500' : 'bg-amber-500/10 text-amber-500'
                      }`}>
                        {a.team_id ? 'Team' : 'Direct'}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-[var(--text-primary)]">{a.role_name}</td>
                    <td className="px-4 py-2.5">
                      <span className={`text-[11px] rounded px-1.5 py-0.5 ${
                        a.is_active ? 'badge-success' : 'badge-danger'
                      }`}>
                        {a.is_active ? 'active' : 'revoked'}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <button onClick={() => handleRevoke(a.id)} className="text-red-500 text-[12px] hover:text-[var(--text-primary)]">
                        Revoke
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <CreateRoleModal open={createOpen} onClose={() => setCreateOpen(false)} onCreated={loadData} />
      <EditRoleModal open={!!editRole} role={editRole} onClose={() => setEditRole(null)} onUpdated={loadData} />
      <AssignRoleModal open={assignOpen} roleId={assignRoleId} roles={roles} onClose={() => setAssignOpen(false)} onAssigned={loadData} />

      {/* Revoke Confirmation */}
      <ConfirmModal
        open={!!revokeConfirm}
        title="Revoke this assignment?"
        description="This role assignment will be revoked immediately. This action cannot be undone."
        confirmLabel="Revoke"
        variant="danger"
        loading={revoking}
        onConfirm={confirmRevoke}
        onCancel={() => setRevokeConfirm(null)}
      />
      </div>
    </div>
  );
}

function AssignRoleModal({ open, roleId, roles, onClose, onAssigned }: {
  open: boolean;
  roleId: string;
  roles: RoleWithPerms[];
  onClose: () => void;
  onAssigned: () => void;
}) {
  const [assignType, setAssignType] = useState<'team' | 'user'>('team');
  const [users, setUsers] = useState<User[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [selectedUser, setSelectedUser] = useState('');
  const [selectedTeam, setSelectedTeam] = useState('');
  const [selectedRole, setSelectedRole] = useState(roleId);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open) {
      setSelectedRole(roleId);
      listUsers().then(data => setUsers(data.users)).catch(() => {});
      listTeams().then(data => setTeams(data.teams)).catch(() => {});
    }
  }, [open, roleId]);

  const handleSubmit = async () => {
    setSaving(true);
    setError('');
    try {
      if (assignType === 'team') {
        if (!selectedTeam) { setError('Select a team'); return; }
        await rbac.assignTeamRole(selectedTeam, selectedRole);
      } else {
        if (!selectedUser) { setError('Select a user'); return; }
        await rbac.assignRole(selectedUser, selectedRole);
      }
      onAssigned();
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to assign role');
    } finally {
      setSaving(false);
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Assign Role</h2>
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
            <label className="label">Assign to</label>
            <div className="flex gap-2">
              <button
                onClick={() => setAssignType('team')}
                className={`flex-1 py-2 rounded-lg text-[13px] font-medium transition-colors ${
                  assignType === 'team' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--border-light)]'
                }`}
              >
                Team (recommended)
              </button>
              <button
                onClick={() => setAssignType('user')}
                className={`flex-1 py-2 rounded-lg text-[13px] font-medium transition-colors ${
                  assignType === 'user' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--border-light)]'
                }`}
              >
                User (exception)
              </button>
            </div>
            {assignType === 'user' && (
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1.5">
                Direct assignment is for exceptions. Prefer team-based roles.
              </p>
            )}
          </div>

          <div>
            <label className="label">Role</label>
            <select className="input" value={selectedRole} onChange={e => setSelectedRole(e.target.value)}>
              {roles.map(r => (
                <option key={r.id} value={r.id}>{r.name}</option>
              ))}
            </select>
          </div>

          {assignType === 'team' ? (
            <div>
              <label className="label">Team</label>
              <select className="input" value={selectedTeam} onChange={e => setSelectedTeam(e.target.value)}>
                <option value="">Select team...</option>
                {teams.map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>
          ) : (
            <div>
              <label className="label">User</label>
              <select className="input" value={selectedUser} onChange={e => setSelectedUser(e.target.value)}>
                <option value="">Select user...</option>
                {users.filter(u => u.is_active).map(u => (
                  <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
                ))}
              </select>
            </div>
          )}
        </div>

        <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button onClick={handleSubmit} disabled={saving} className="btn btn-primary">
            {saving ? 'Assigning...' : 'Assign'}
          </button>
        </div>
      </div>
    </div>
  );
}
