'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { listTeams, createTeam, deleteTeam, listTeamMembers, addTeamMember, removeTeamMember, listUsers, type Team, type TeamMember, type User } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import { usePermission } from '@/hooks/usePermission';

export default function TeamsPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <TeamsPageContent />
    </PermissionGuard>
  );
}

function TeamsPageContent() {
  const { hasPermission } = usePermission();
  const canWrite = hasPermission('settings', 'create') || hasPermission('settings', 'update') || hasPermission('settings', 'delete');
  const [teams, setTeams] = useState<Team[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null);
  const [error, setError] = useState('');
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => { loadTeams(); }, []);

  async function loadTeams(background = false) {
    if (!background) setLoading(true);
    try {
      const data = await listTeams();
      setTeams(data.teams);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this team?')) return;
    try {
      await deleteTeam(id);
      await loadTeams(true);
      if (selectedTeam?.id === id) setSelectedTeam(null);
      setFeedback({ ok: true, text: 'Team deleted' });
    } catch (err) {
      setFeedback({ ok: false, text: err instanceof Error ? err.message : 'Failed' });
    }
  }

  // Build tree structure
  const rootTeams = teams.filter(t => !t.parent_team_id);
  const getChildTeams = (parentId: string) => teams.filter(t => t.parent_team_id === parentId);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text-primary)] page-title-modern">Teams & Groups</h1>
          <p className="text-sm text-[var(--text-tertiary)] mt-1 page-subtitle-modern">Organize users into teams with hierarchical groups</p>
        </div>
        {canWrite && (
          <button onClick={() => setShowCreate(true)} className="btn btn-primary">
            Create Team
          </button>
        )}
      </div>

      {error && <div className="bg-red-500/10 border-red-500/20 text-red-500 px-4 py-3 rounded mb-4 text-sm">{error}</div>}

      {feedback && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 mb-4 ${
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

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Team list */}
        <div className="lg:col-span-1">
          <div className="bg-[var(--surface)] border border-[var(--border)] rounded-lg">
            <div className="px-4 py-3 border-b border-[var(--border)] font-medium text-sm text-[var(--text-secondary)]">Teams</div>
            {loading ? (
              <div className="p-4 text-center text-[var(--text-tertiary)] text-sm">Loading...</div>
            ) : rootTeams.length === 0 ? (
              <div className="p-4 text-center text-[var(--text-tertiary)] text-sm">No teams yet</div>
            ) : (
              <div className="divide-y divide-gray-100">
                {rootTeams.map(team => (
                  <TeamTreeItem
                    key={team.id}
                    team={team}
                    depth={0}
                    getChildTeams={getChildTeams}
                    selected={selectedTeam?.id === team.id}
                    onSelect={setSelectedTeam}
                    onDelete={handleDelete}
                    canWrite={canWrite}
                  />
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Team details */}
        <div className="lg:col-span-2">
          {selectedTeam ? (
            <TeamDetail team={selectedTeam} onRefresh={loadTeams} canWrite={canWrite} />
          ) : (
            <div className="bg-[var(--surface)] border border-[var(--border)] rounded-lg p-8 text-center text-[var(--text-tertiary)]">
              Select a team to view details and manage members
            </div>
          )}
        </div>
      </div>

      {showCreate && <CreateTeamModal onClose={() => setShowCreate(false)} onCreated={() => loadTeams(true)} teams={teams} />}
    </div>
  );
}

function TeamTreeItem({ team, depth, getChildTeams, selected, onSelect, onDelete, canWrite }: {
  team: Team; depth: number; getChildTeams: (id: string) => Team[];
  selected: boolean; onSelect: (t: Team) => void; onDelete: (id: string) => void; canWrite: boolean;
}) {
  const children = getChildTeams(team.id);
  return (
    <div>
      <div
        className={`flex items-center justify-between px-4 py-2 cursor-pointer hover:bg-[var(--bg)] ${selected ? 'bg-[var(--accent-subtle)] border-l-2 border-[var(--accent)]' : ''}`}
        style={{ paddingLeft: `${16 + depth * 16}px` }}
        onClick={() => onSelect(team)}
      >
        <div>
          <div className="text-sm font-medium text-[var(--text-primary)]">{team.name}</div>
          {team.description && <div className="text-xs text-[var(--text-tertiary)]">{team.description}</div>}
        </div>
        {canWrite && (
          <button onClick={(e) => { e.stopPropagation(); onDelete(team.id); }} className="text-red-400 hover:text-[var(--text-secondary)] text-xs">
            Delete
          </button>
        )}
      </div>
      {children.map(child => (
        <TeamTreeItem key={child.id} team={child} depth={depth + 1} getChildTeams={getChildTeams} selected={selected} onSelect={onSelect} onDelete={onDelete} canWrite={canWrite} />
      ))}
    </div>
  );
}

function TeamDetail({ team, onRefresh, canWrite }: { team: Team; onRefresh: () => void; canWrite: boolean }) {
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddMember, setShowAddMember] = useState(false);
  const [memberFeedback, setMemberFeedback] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => { loadMembers(); }, [team.id]);

  async function loadMembers() {
    setLoading(true);
    try {
      const data = await listTeamMembers(team.id);
      setMembers(data.members);
    } catch { /* ignore */ }
    setLoading(false);
  }

  async function handleRemoveMember(userId: string) {
    try {
      await removeTeamMember(team.id, userId);
      loadMembers();
      setMemberFeedback({ ok: true, text: 'Member removed' });
    } catch (err) {
      setMemberFeedback({ ok: false, text: err instanceof Error ? err.message : 'Failed' });
    }
  }

  return (
    <div className="bg-[var(--surface)] border border-[var(--border)] rounded-lg">
      <div className="px-4 py-4 border-b border-[var(--border)]">
        <h2 className="text-lg font-bold text-[var(--text-primary)]">{team.name}</h2>
        {team.description && <p className="text-sm text-[var(--text-tertiary)] mt-1">{team.description}</p>}
        <p className="text-xs text-[var(--text-tertiary)] mt-1">Slug: {team.slug}</p>
      </div>

      {memberFeedback && (
        <div className={`mx-4 mt-4 rounded-xl border p-3 flex items-start justify-between gap-3 ${
          memberFeedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <p className={`text-sm font-medium ${memberFeedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
            {memberFeedback.ok ? '✓ ' : '⚠ '}{memberFeedback.text}
          </p>
          <button onClick={() => setMemberFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      <div className="px-4 py-3 border-b border-[var(--border)] flex items-center justify-between">
        <h3 className="text-sm font-medium text-[var(--text-secondary)]">Members ({members.length})</h3>
        {canWrite && (
          <button onClick={() => setShowAddMember(true)} className="text-[var(--accent)] text-sm hover:text-[var(--text-primary)]">+ Add Member</button>
        )}
      </div>

      {loading ? (
        <div className="p-4 text-center text-[var(--text-tertiary)] text-sm">Loading...</div>
      ) : members.length === 0 ? (
        <div className="p-4 text-center text-[var(--text-tertiary)] text-sm">No members</div>
      ) : (
        <div className="divide-y divide-gray-100">
          {members.map(m => (
            <div key={m.id} className="px-4 py-3 flex items-center justify-between">
              <div>
                <div className="text-sm font-medium text-[var(--text-primary)]">{m.name || m.email}</div>
                <div className="text-xs text-[var(--text-tertiary)]">{m.email} &middot; {m.role}</div>
              </div>
              {canWrite && (
                <button onClick={() => handleRemoveMember(m.user_id)} className="text-red-500 text-xs hover:text-[var(--text-primary)]">Remove</button>
              )}
            </div>
          ))}
        </div>
      )}

      {showAddMember && <AddMemberModal teamId={team.id} onClose={() => setShowAddMember(false)} onAdded={loadMembers} />}
    </div>
  );
}

function AddMemberModal({ teamId, onClose, onAdded }: { teamId: string; onClose: () => void; onAdded: () => void }) {
  useEscapeKey(onClose);
  const [users, setUsers] = useState<User[]>([]);
  const [selectedUser, setSelectedUser] = useState('');
  const [role, setRole] = useState('member');
  const [error, setError] = useState('');

  useEffect(() => {
    listUsers().then(data => setUsers(data.users)).catch(() => {});
  }, []);

  async function handleAdd() {
    if (!selectedUser) return;
    try {
      await addTeamMember(teamId, selectedUser, role);
      onAdded();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Add Member</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div className="p-5 space-y-4">
          {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}
          <div>
            <label className="label">User</label>
            <select className="input" value={selectedUser} onChange={(e) => setSelectedUser(e.target.value)}>
              <option value="">Select user...</option>
              {users.filter(u => u.is_active).map(u => (
                <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
              ))}
            </select>
          </div>
          <div>
            <label className="label">Role</label>
            <select className="input" value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="member">Member</option>
              <option value="lead">Lead</option>
            </select>
          </div>
        </div>
        <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button onClick={handleAdd} className="btn btn-primary">Add</button>
        </div>
      </div>
    </div>
  );
}

function CreateTeamModal({ onClose, onCreated, teams }: { onClose: () => void; onCreated: () => void; teams: Team[] }) {
  useEscapeKey(onClose);
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [parentId, setParentId] = useState('');
  const [error, setError] = useState('');

  function autoSlug(n: string) { return n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''); }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await createTeam({ name, slug: slug || autoSlug(name), description, parent_team_id: parentId || undefined });
      onCreated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create Team</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="p-5 space-y-4">
            {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}
            <div>
              <label className="label">Name</label>
              <input type="text" className="input" value={name} onChange={(e) => setName(e.target.value)} required placeholder="Team name" />
            </div>
            <div>
              <label className="label">Slug</label>
              <input type="text" className="input" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder={autoSlug(name || 'team-name')} />
            </div>
            <div>
              <label className="label">Description</label>
              <input type="text" className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Optional description" />
            </div>
            <div>
              <label className="label">Parent Team (optional)</label>
              <select className="input" value={parentId} onChange={(e) => setParentId(e.target.value)}>
                <option value="">None (root team)</option>
                {teams.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
            </div>
          </div>
          <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancel</button>
            <button type="submit" className="btn btn-primary">Create</button>
          </div>
        </form>
      </div>
    </div>
  );
}
