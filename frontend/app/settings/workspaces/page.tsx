'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { workspaces, organization, setToken, type Workspace, type Organization, type WorkspaceMember } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';

export default function WorkspacesPage() {
  const router = useRouter();
  const { hasPermission } = usePermission();
  const canWrite = hasPermission('settings', 'create') || hasPermission('settings', 'update') || hasPermission('settings', 'delete');
  const [org, setOrg] = useState<Organization | null>(null);
  const [workspacesList, setWorkspacesList] = useState<Workspace[]>([]);
  const [currentWs, setCurrentWs] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editingOrg, setEditingOrg] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [editingWs, setEditingWs] = useState<Workspace | null>(null);
  const [managingMembersWs, setManagingMembersWs] = useState<Workspace | null>(null);
  const [switching, setSwitching] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [orgData, wsData] = await Promise.all([
        organization.get().catch(() => null),
        workspaces.list(),
      ]);
      if (orgData) setOrg(orgData.organization);
      setWorkspacesList(wsData.workspaces || []);
      setCurrentWs(wsData.current_workspace || '');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to load workspaces';
      console.error('Failed to load workspaces:', err);
      setError(msg);
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  const showToast = (message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  };

  // Notify other components (TopBar workspace switcher) that workspaces changed
  const notifyWorkspacesChanged = () => {
    window.dispatchEvent(new CustomEvent('pepa:workspaces-changed'));
  };

  // Switch to a workspace and optionally navigate to a page
  const handleSwitchWorkspace = async (id: string, navigateTo?: string) => {
    if (id === currentWs && !navigateTo) return;
    setSwitching(id);
    try {
      if (id !== currentWs) {
        const res = await workspaces.switch(id);
        setToken(res.token);
        setCurrentWs(id);
        notifyWorkspacesChanged();
      }
      if (navigateTo) {
        router.push(navigateTo);
      } else {
        window.location.reload();
      }
    } catch {
      showToast('Failed to switch workspace', 'error');
    } finally {
      setSwitching(null);
    }
  };

  async function handleDeleteWs(id: string) {
    if (!confirm('Delete this workspace? All data in it will be lost.')) return;
    try {
      await workspaces.delete(id);
      showToast('Workspace deleted', 'success');
      loadData();
      notifyWorkspacesChanged();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed', 'error');
    }
  }

  if (loading) {
    return (
      <div>
        <h1 className="text-2xl font-bold text-[var(--text-primary)] page-title-modern">Workspaces</h1>
        <p className="text-sm text-[var(--text-tertiary)] mt-1 page-subtitle-modern">Loading...</p>
      </div>
    );
  }

  const isDefault = (id: string) => id === '00000000-0000-0000-0000-000000000002';

  return (
    <div>
      {toast && (
        <div className={`mb-4 px-4 py-2.5 rounded-xl text-[13px] page-animate-up ${toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'}`}>
          {toast.message}
        </div>
      )}

      {/* Error state */}
      {error && (
        <div className="mt-4 px-4 py-3 rounded-xl bg-red-500/10 text-red-500 border border-red-500/20 page-animate-up">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <span className="text-[13px]">{error}</span>
            </div>
            <button onClick={loadData} className="text-[12px] font-medium hover:underline">Retry</button>
          </div>
        </div>
      )}

      <div className="page-animate">
        <h1 className="text-2xl font-bold text-[var(--text-primary)] page-title-modern">Workspaces</h1>
        <p className="text-sm text-[var(--text-tertiary)] mt-1 page-subtitle-modern">
          Manage your organization structure: workspaces, teams, users, and environments
        </p>
      </div>

      {/* How Workspaces Work */}
      <div className="card page-animate-up mt-6" style={{ borderRadius: '12px' }}>
        <div className="card-body">
          <div className="flex items-start gap-3">
            <div className="w-8 h-8 rounded-lg bg-blue-500/10 flex items-center justify-center shrink-0">
              <svg className="w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div className="flex-1 text-[12px] text-[var(--text-secondary)] leading-relaxed">
              <p className="font-medium text-[var(--text-primary)] mb-1">How workspaces work</p>
              <p>Each workspace is an <strong>isolated environment</strong> with its own teams, users, services, environments, and connections. Switch to a workspace to manage its resources. Use the <strong>workspace switcher</strong> in the top bar to quickly move between workspaces.</p>
            </div>
          </div>
        </div>
      </div>

      {/* Structure Overview */}
      <div className="card page-animate-up mt-6" style={{ borderRadius: '12px' }}>
        <div className="card-header">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Organization Structure</span>
        </div>
        <div className="card-body">
          <div className="flex items-center gap-2 flex-wrap text-[12px]">
            <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[var(--accent)]/10 text-[var(--accent)] font-medium">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
              {org?.name || 'Organization'}
            </div>
            <svg className="w-4 h-4 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
            <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-500/10 text-blue-600 font-medium">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" /></svg>
              Workspaces ({workspacesList.length})
            </div>
            <svg className="w-4 h-4 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
            <Link href="/settings/teams" className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-emerald-500/10 text-emerald-600 font-medium hover:opacity-80">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
              Teams / Groups
            </Link>
            <svg className="w-4 h-4 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
            <Link href="/settings/users" className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-500/10 text-amber-600 font-medium hover:opacity-80">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
              Users
            </Link>
          </div>
          <div className="mt-3 flex items-center gap-2 flex-wrap text-[12px]">
            <span className="text-[var(--text-tertiary)]">Also per workspace:</span>
            <Link href="/environments" className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-500/10 text-purple-600 font-medium hover:opacity-80">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              Environments
            </Link>
            <Link href="/roles" className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-rose-500/10 text-rose-600 font-medium hover:opacity-80">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg>
              Roles & Permissions
            </Link>
          </div>
        </div>
      </div>

      {/* Organization Info */}
      {org && (
        <div className="card page-animate-up mt-6" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Organization</span>
            {canWrite && (
              <button onClick={() => setEditingOrg(true)} className="text-[12px] text-[var(--accent)] hover:opacity-80">
                Edit
              </button>
            )}
          </div>
          <div className="card-body">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-[var(--accent)]/10 flex items-center justify-center text-[15px] font-semibold text-[var(--accent)]">
                {org.name.charAt(0).toUpperCase()}
              </div>
              <div>
                <h3 className="text-[15px] font-medium text-[var(--text-primary)]">{org.name}</h3>
                <p className="text-[12px] text-[var(--text-tertiary)]">/{org.slug} &middot; {org.plan} plan</p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Workspaces List */}
      <div className="mt-6">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-[15px] font-medium text-[var(--text-primary)]">
            Workspaces <span className="text-[var(--text-tertiary)] font-normal">({workspacesList.length})</span>
          </h2>
          {canWrite && (
            <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm">+ New Workspace</button>
          )}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
          {workspacesList.map((ws) => (
            <div
              key={ws.id}
              className={`card card-body modern-card-hover ${ws.id === currentWs ? 'ring-1 ring-[var(--accent)]' : ''}`}
              style={{ borderRadius: '12px' }}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <div className={`w-8 h-8 rounded-lg flex items-center justify-center text-[12px] font-semibold ${
                    ws.id === currentWs
                      ? 'bg-[var(--accent)] text-white'
                      : 'bg-[var(--bg)] text-[var(--text-secondary)]'
                  }`}>
                    {ws.name.charAt(0).toUpperCase()}
                  </div>
                  <div>
                    <h3 className="text-[14px] font-medium text-[var(--text-primary)]">{ws.name}</h3>
                    <p className="text-[11px] text-[var(--text-tertiary)]">/{ws.slug}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {ws.id === currentWs && (
                    <span className="text-[10px] bg-[var(--accent)]/10 text-[var(--accent)] rounded px-1.5 py-0.5 font-medium">current</span>
                  )}
                  {isDefault(ws.id) && (
                    <span className="text-[10px] text-[var(--text-tertiary)] border border-[var(--border)] rounded px-1.5 py-0.5">default</span>
                  )}
                </div>
              </div>

              {/* Resource counts — clickable to switch + navigate */}
              {ws.counts && (
                <div className="mt-3 grid grid-cols-4 gap-2">
                  <button
                    onClick={() => handleSwitchWorkspace(ws.id, '/settings/teams')}
                    disabled={switching !== null}
                    className="text-center p-2 rounded-lg bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors disabled:opacity-50"
                  >
                    <div className="text-[16px] font-semibold text-[var(--text-primary)]">{ws.counts.teams}</div>
                    <div className="text-[10px] text-[var(--text-tertiary)]">Teams</div>
                  </button>
                  <button
                    onClick={() => handleSwitchWorkspace(ws.id, '/settings/users')}
                    disabled={switching !== null}
                    className="text-center p-2 rounded-lg bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors disabled:opacity-50"
                  >
                    <div className="text-[16px] font-semibold text-[var(--text-primary)]">{ws.counts.users}</div>
                    <div className="text-[10px] text-[var(--text-tertiary)]">Users</div>
                  </button>
                  <button
                    onClick={() => handleSwitchWorkspace(ws.id, '/environments')}
                    disabled={switching !== null}
                    className="text-center p-2 rounded-lg bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors disabled:opacity-50"
                  >
                    <div className="text-[16px] font-semibold text-[var(--text-primary)]">{ws.counts.environments}</div>
                    <div className="text-[10px] text-[var(--text-tertiary)]">Envs</div>
                  </button>
                  <button
                    onClick={() => handleSwitchWorkspace(ws.id, '/services')}
                    disabled={switching !== null}
                    className="text-center p-2 rounded-lg bg-[var(--bg)] hover:bg-[var(--border-light)] transition-colors disabled:opacity-50"
                  >
                    <div className="text-[16px] font-semibold text-[var(--text-primary)]">{ws.counts.services}</div>
                    <div className="text-[10px] text-[var(--text-tertiary)]">Services</div>
                  </button>
                </div>
              )}

              <div className="mt-2 text-[11px] text-[var(--text-tertiary)]">
                Created {new Date(ws.created_at).toLocaleDateString()}
              </div>

              <div className="mt-3 pt-3 border-t border-[var(--border-light)] flex items-center justify-between">
                <div className="flex gap-3">
                  {ws.id === currentWs ? (
                    <span className="text-[11px] text-emerald-500 font-medium flex items-center gap-1">
                      <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" /></svg>
                      Active workspace
                    </span>
                  ) : (
                    <button
                      onClick={() => handleSwitchWorkspace(ws.id)}
                      disabled={switching !== null}
                      className="text-[11px] text-[var(--accent)] hover:underline font-medium disabled:opacity-50"
                    >
                      {switching === ws.id ? 'Switching...' : 'Switch to this workspace'}
                    </button>
                  )}
                </div>
                <div className="flex gap-2">
                  <button onClick={() => setManagingMembersWs(ws)} className="text-[12px] text-[var(--accent)] hover:text-[var(--text-primary)]">
                    Manage Access
                  </button>
                  {!isDefault(ws.id) && (
                    <>
                      <button onClick={() => setEditingWs(ws)} className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
                        Rename
                      </button>
                      <button onClick={() => handleDeleteWs(ws.id)} className="text-[12px] text-red-500 hover:text-red-400">
                        Delete
                      </button>
                    </>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Modals */}
      {editingOrg && org && (
        <OrgEditModal
          org={org}
          onClose={() => setEditingOrg(false)}
          onSaved={() => { setEditingOrg(false); loadData(); showToast('Organization updated', 'success'); }}
          showToast={showToast}
        />
      )}

      {showCreate && (
        <WorkspaceModal
          onClose={() => setShowCreate(false)}
          onSaved={() => { setShowCreate(false); loadData(); notifyWorkspacesChanged(); showToast('Workspace created', 'success'); }}
          showToast={showToast}
        />
      )}

      {editingWs && (
        <WorkspaceModal
          ws={editingWs}
          onClose={() => setEditingWs(null)}
          onSaved={() => { setEditingWs(null); loadData(); notifyWorkspacesChanged(); showToast('Workspace updated', 'success'); }}
          showToast={showToast}
        />
      )}

      {managingMembersWs && (
        <ManageMembersModal
          ws={managingMembersWs}
          onClose={() => setManagingMembersWs(null)}
          showToast={showToast}
        />
      )}
    </div>
  );
}

function OrgEditModal({ org, onClose, onSaved, showToast }: {
  org: Organization;
  onClose: () => void;
  onSaved: () => void;
  showToast: (msg: string, type: 'success' | 'error') => void;
}) {
  const [name, setName] = useState(org.name);
  const [slug, setSlug] = useState(org.slug);
  const [saving, setSaving] = useState(false);

  const autoSlug = (val: string) => val.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

  const handleSave = async () => {
    if (!name.trim() || !slug.trim()) { showToast('Name and slug are required', 'error'); return; }
    setSaving(true);
    try {
      await organization.update({ name, slug });
      onSaved();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed', 'error');
    } finally { setSaving(false); }
  };

  return (
    <ModalShell title="Edit Organization" onClose={onClose}>
      <div className="space-y-4">
        <div>
          <label className="label">Organization Name *</label>
          <input type="text" className="input" value={name} onChange={e => { setName(e.target.value); setSlug(autoSlug(e.target.value)); }} />
        </div>
        <div>
          <label className="label">Slug *</label>
          <input type="text" className="input" value={slug} onChange={e => setSlug(autoSlug(e.target.value))} />
        </div>
      </div>
      <ModalFooter onClose={onClose} onSave={handleSave} saving={saving} saveLabel="Save" />
    </ModalShell>
  );
}

function WorkspaceModal({ ws, onClose, onSaved, showToast }: {
  ws?: Workspace;
  onClose: () => void;
  onSaved: () => void;
  showToast: (msg: string, type: 'success' | 'error') => void;
}) {
  const [name, setName] = useState(ws?.name || '');
  const [slug, setSlug] = useState(ws?.slug || '');
  const [saving, setSaving] = useState(false);

  const autoSlug = (val: string) => val.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

  const handleSave = async () => {
    if (!name.trim() || !slug.trim()) { showToast('Name and slug are required', 'error'); return; }
    setSaving(true);
    try {
      if (ws) {
        await workspaces.update(ws.id, { name, slug });
      } else {
        await workspaces.create({ name, slug });
      }
      onSaved();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed', 'error');
    } finally { setSaving(false); }
  };

  return (
    <ModalShell title={ws ? 'Rename Workspace' : 'New Workspace'} onClose={onClose}>
      <div className="space-y-4">
        <div>
          <label className="label">Workspace Name *</label>
          <input type="text" className="input" placeholder="e.g., Production, Staging, Dev" value={name}
            onChange={e => { setName(e.target.value); if (!ws) setSlug(autoSlug(e.target.value)); }} />
          <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Each workspace is an isolated space with its own teams, users, environments, and services</p>
        </div>
        {!ws && (
          <div>
            <label className="label">Slug *</label>
            <input type="text" className="input" placeholder="e.g., production, staging" value={slug}
              onChange={e => setSlug(autoSlug(e.target.value))} />
          </div>
        )}
      </div>
      <ModalFooter onClose={onClose} onSave={handleSave} saving={saving} saveLabel={ws ? 'Rename' : 'Create'} />
    </ModalShell>
  );
}

// ─── Shared Modal Components ──────────────────────────────────────────────────

function ModalShell({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">{title}</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  );
}

function ModalFooter({ onClose, onSave, saving, saveLabel }: { onClose: () => void; onSave: () => void; saving: boolean; saveLabel: string }) {
  return (
    <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2">
      <button onClick={onClose} className="btn btn-secondary">Cancel</button>
      <button onClick={onSave} disabled={saving} className="btn btn-primary">
        {saving ? 'Saving...' : saveLabel}
      </button>
    </div>
  );
}

function ManageMembersModal({ ws, onClose, showToast }: {
  ws: Workspace;
  onClose: () => void;
  showToast: (msg: string, type: 'success' | 'error') => void;
}) {
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [rolePicker, setRolePicker] = useState<string | null>(null); // user_id being assigned a role
  const [pendingRole, setPendingRole] = useState('viewer');
  const [saving, setSaving] = useState(false);

  const loadMembers = useCallback(async () => {
    setLoading(true);
    try {
      const data = await workspaces.listMembers(ws.id);
      setMembers(data.members || []);
    } catch {
      showToast('Failed to load members', 'error');
    } finally {
      setLoading(false);
    }
  }, [ws.id, showToast]);

  useEffect(() => { loadMembers(); }, [loadMembers]);

  const filtered = members.filter(m =>
    !search || m.name.toLowerCase().includes(search.toLowerCase()) || m.email.toLowerCase().includes(search.toLowerCase())
  );

  // Sort: users with access first, then without
  const sorted = [...filtered].sort((a, b) => {
    if (a.has_access && !b.has_access) return -1;
    if (!a.has_access && b.has_access) return 1;
    return a.name.localeCompare(b.name);
  });

  async function handleAdd(userId: string) {
    setSaving(true);
    try {
      await workspaces.addMember(ws.id, userId, pendingRole);
      showToast('User granted access', 'success');
      setRolePicker(null);
      loadMembers();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed', 'error');
    } finally {
      setSaving(false);
    }
  }

  async function handleRemove(userId: string) {
    if (!confirm('Revoke this user\'s access to this workspace?')) return;
    try {
      await workspaces.removeMember(ws.id, userId);
      showToast('Access revoked', 'success');
      loadMembers();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed', 'error');
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-2xl max-h-[85vh] mx-4 flex flex-col">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between shrink-0">
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Manage Access: {ws.name}</h2>
            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Grant or revoke workspace access for users</p>
          </div>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Search */}
        <div className="px-5 py-3 border-b border-[var(--border-light)] shrink-0">
          <div className="relative">
            <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              placeholder="Search users by name or email..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="w-full pl-9 pr-3 py-2 border border-[var(--border)] rounded-lg text-[13px] bg-[var(--bg)]"
            />
          </div>
        </div>

        {/* User list */}
        <div className="flex-1 overflow-y-auto px-5 py-3">
          {loading ? (
            <div className="text-center py-8 text-[var(--text-tertiary)] text-[13px]">Loading...</div>
          ) : sorted.length === 0 ? (
            <div className="text-center py-8 text-[var(--text-tertiary)] text-[13px]">
              {search ? 'No users found matching your search' : 'No users found'}
            </div>
          ) : (
            <div className="space-y-1">
              {sorted.map(m => (
                <div key={m.user_id} className={`flex items-center justify-between px-3 py-2.5 rounded-lg border transition-colors ${
                  m.has_access
                    ? 'bg-[var(--bg)] border-[var(--border-light)]'
                    : 'border-[var(--border-light)] border-dashed opacity-75 hover:opacity-100'
                }`}>
                  <div className="flex items-center gap-2.5 min-w-0">
                    <div className={`w-8 h-8 rounded-full flex items-center justify-center text-[11px] font-semibold shrink-0 ${
                      m.has_access
                        ? 'bg-[var(--accent)]/10 text-[var(--accent)]'
                        : 'bg-[var(--bg)] text-[var(--text-tertiary)]'
                    }`}>
                      {m.name.charAt(0).toUpperCase()}
                    </div>
                    <div className="min-w-0">
                      <div className="text-[13px] font-medium text-[var(--text-primary)] flex items-center gap-1.5">
                        <span className="truncate">{m.name}</span>
                        {m.is_super_admin && <span className="text-[9px] bg-amber-500/15 text-amber-600 px-1 py-0.5 rounded shrink-0">SA</span>}
                      </div>
                      <div className="text-[11px] text-[var(--text-tertiary)] truncate">{m.email}</div>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0 ml-3">
                    {m.has_access ? (
                      <>
                        <div className="flex gap-1">
                          {m.roles.map(r => (
                            <span key={r} className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--accent)]/10 text-[var(--accent)] font-medium">{r}</span>
                          ))}
                        </div>
                        {!m.is_super_admin && (
                          <button
                            onClick={() => handleRemove(m.user_id)}
                            className="text-[11px] text-red-500 hover:text-red-400 font-medium"
                          >
                            Revoke
                          </button>
                        )}
                      </>
                    ) : (
                      <>
                        {rolePicker === m.user_id ? (
                          <div className="flex items-center gap-1.5">
                            <select
                              value={pendingRole}
                              onChange={e => setPendingRole(e.target.value)}
                              className="px-2 py-1 border border-[var(--border)] rounded text-[11px] bg-[var(--surface)]"
                            >
                              <option value="viewer">Viewer</option>
                              <option value="developer">Developer</option>
                              <option value="admin">Admin</option>
                            </select>
                            <button
                              onClick={() => handleAdd(m.user_id)}
                              disabled={saving}
                              className="px-2.5 py-1 bg-[var(--accent)] text-white rounded text-[11px] font-medium disabled:opacity-50"
                            >
                              {saving ? '...' : 'Grant'}
                            </button>
                            <button
                              onClick={() => setRolePicker(null)}
                              className="text-[11px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)]"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <button
                            onClick={() => { setRolePicker(m.user_id); setPendingRole('viewer'); }}
                            className="text-[11px] text-[var(--accent)] hover:text-[var(--text-primary)] font-medium"
                          >
                            + Grant Access
                          </button>
                        )}
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
