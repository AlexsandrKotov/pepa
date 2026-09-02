'use client';

import { useState, useEffect, useCallback } from 'react';
import { getBase } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';
import { Terminal } from './Terminal';
import ConfirmModal from '@/components/ConfirmModal';

interface SSHHost {
  id: string;
  name: string;
  hostname: string;
  port: number;
  username: string;
  auth_method: string;
  has_ssh_key: boolean;
  has_password: boolean;
  tags: string[];
  description: string;
  group_ids: string[];
  created_at: string;
  updated_at: string;
}

interface SSHHostGroup {
  id: string;
  name: string;
  description: string;
  color: string;
  host_count: number;
  created_at: string;
  updated_at: string;
}

type View = 'hosts' | 'terminal';

export default function RemoteConsolePage() {
  const [view, setView] = useState<View>('hosts');
  const [hosts, setHosts] = useState<SSHHost[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAddHost, setShowAddHost] = useState(false);
  const [editingHost, setEditingHost] = useState<SSHHost | null>(null);
  const [activeHost, setActiveHost] = useState<SSHHost | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [password, setPassword] = useState('');
  const [showPasswordDialog, setShowPasswordDialog] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [terminalKey, setTerminalKey] = useState(0);
  const [groups, setGroups] = useState<SSHHostGroup[]>([]);
  const [filterGroup, setFilterGroup] = useState<string | null>(null);
  const [showGroupDialog, setShowGroupDialog] = useState(false);
  const [editingGroup, setEditingGroup] = useState<SSHHostGroup | null>(null);
  const [deleteHostConfirm, setDeleteHostConfirm] = useState<string | null>(null);
  const [deleteGroupConfirm, setDeleteGroupConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const { isAdmin } = usePermission();

  const fetchHosts = useCallback(async () => {
    try {
      const res = await fetch(`${getBase()}/api/v1/ssh-hosts`, { credentials: 'include' });
      if (!res.ok) throw new Error('Failed to fetch hosts');
      const data = await res.json();
      setHosts(data.hosts || []);
    } catch (err) {
      setToast({ message: String(err), type: 'error' });
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchGroups = useCallback(async () => {
    try {
      const res = await fetch(`${getBase()}/api/v1/ssh-host-groups`, { credentials: 'include' });
      if (!res.ok) {
        console.warn('Failed to fetch groups:', res.status);
        return;
      }
      const data = await res.json();
      setGroups(data.groups || []);
    } catch (err) { 
      console.warn('Failed to fetch groups:', err);
    }
  }, []);

  useEffect(() => { fetchHosts(); fetchGroups(); }, [fetchHosts, fetchGroups]);

  const handleConnect = (host: SSHHost) => {
    if (host.auth_method === 'ldap_passthrough' && !host.has_password) {
      setActiveHost(host);
      setShowPasswordDialog(true);
    } else {
      setActiveHost(host);
      setView('terminal');
    }
  };

  const handlePasswordConnect = () => {
    if (!activeHost) return;
    setConnecting(true);
    setView('terminal');
    setShowPasswordDialog(false);
  };

  const handleDisconnect = () => {
    setView('hosts');
    setActiveHost(null);
    setPassword('');
    setConnecting(false);
  };

  const handleReconnect = () => {
    // Increment key to force React to re-mount the Terminal component
    setTerminalKey(k => k + 1);
  };

  const handleDeleteHost = async (id: string) => {
    setDeleteHostConfirm(id);
  };

  const confirmDeleteHost = async () => {
    if (!deleteHostConfirm) return;
    setDeleting(true);
    try {
      const res = await fetch(`${getBase()}/api/v1/ssh-hosts/${deleteHostConfirm}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) throw new Error('Failed to delete host');
      setToast({ message: 'Host deleted', type: 'success' });
      fetchHosts();
      fetchGroups();
    } catch (err) {
      setToast({ message: String(err), type: 'error' });
    }
    setDeleting(false);
    setDeleteHostConfirm(null);
  };

  const handleDeleteGroup = async (id: string) => {
    setDeleteGroupConfirm(id);
  };

  const confirmDeleteGroup = async () => {
    if (!deleteGroupConfirm) return;
    setDeleting(true);
    try {
      const res = await fetch(`${getBase()}/api/v1/ssh-host-groups/${deleteGroupConfirm}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      if (!res.ok) throw new Error('Failed to delete group');
      setToast({ message: 'Group deleted', type: 'success' });
      fetchGroups();
      fetchHosts();
    } catch (err) {
      setToast({ message: String(err), type: 'error' });
    }
    setDeleting(false);
    setDeleteGroupConfirm(null);
  };

  const handleTestConnection = async (id: string) => {
    setToast(null);
    try {
      const res = await fetch(`${getBase()}/api/v1/ssh-hosts/${id}/test`, {
        method: 'POST',
        credentials: 'include',
      });
      const data = await res.json();
      setToast({ message: data.message || 'Connection test completed', type: data.status === 'connected' ? 'success' : 'error' });
    } catch (err) {
      setToast({ message: String(err), type: 'error' });
    }
  };

  if (view === 'terminal' && activeHost) {
    return (
      <div className="-mx-6 -my-6 h-[calc(100vh-52px)]">
        <Terminal
          key={terminalKey}
          host={activeHost}
          password={password}
          onDisconnect={handleDisconnect}
          onReconnect={handleReconnect}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {toast && (
        <div className={`p-3 rounded-lg border text-sm ${toast.type === 'success' ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-500' : 'bg-red-500/10 border-red-500/20 text-red-500'}`}>
          {toast.message}
          <button onClick={() => setToast(null)} className="ml-2 text-xs opacity-60 hover:opacity-100">Dismiss</button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between page-animate">
        <div>
          <h1 className="page-title-modern">Remote Console</h1>
          <p className="page-subtitle-modern">SSH terminal access to your infrastructure</p>
        </div>
        {isAdmin && (
          <div className="flex items-center gap-2">
            <button onClick={() => { setEditingGroup(null); setShowGroupDialog(true); }} className="btn btn-secondary btn-sm">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
              </svg>
              Groups
            </button>
            <button onClick={() => { setEditingHost(null); setShowAddHost(true); }} className="btn btn-primary btn-sm">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
              </svg>
              Add Host
            </button>
          </div>
        )}
      </div>

      {/* Info Banner */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up">
        <p className="text-sm text-blue-500">
          <strong>LDAP Passthrough:</strong> When enabled, use your PEPA login credentials to authenticate SSH sessions.
          Your password is never stored — it's used only for the active session.
        </p>
      </div>

      {/* Group Filter Chips */}
      {groups.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 page-animate-up">
          <button
            onClick={() => setFilterGroup(null)}
            className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${
              filterGroup === null ? 'bg-[var(--primary)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--bg-hover)]'
            }`}
          >
            All ({hosts.length})
          </button>
          {groups.map(g => (
            <button
              key={g.id}
              onClick={() => setFilterGroup(filterGroup === g.id ? null : g.id)}
              className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${
                filterGroup === g.id ? 'text-white' : 'hover:opacity-80'
              }`}
              style={filterGroup === g.id ? { backgroundColor: g.color } : { backgroundColor: g.color + '20', color: g.color }}
            >
              {g.name} ({g.host_count})
            </button>
          ))}
        </div>
      )}

      {/* Hosts List */}
      {loading ? (
        <div className="text-center py-12 text-[var(--text-tertiary)]">Loading hosts...</div>
      ) : (() => {
        const filtered = filterGroup ? hosts.filter(h => h.group_ids?.includes(filterGroup)) : hosts;
        if (filtered.length === 0) return (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-body text-center text-[var(--text-tertiary)] py-12">
              <svg className="w-12 h-12 mx-auto mb-4 text-[var(--text-tertiary)] opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
              </svg>
              <p className="mb-2">{filterGroup ? 'No hosts in this group' : 'No SSH hosts configured'}</p>
              {isAdmin && !filterGroup && <p className="text-xs">Click &quot;Add Host&quot; to get started</p>}
            </div>
          </div>
        );
        return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filtered.map(host => (
            <div key={host.id} className="card modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="card-body">
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${host.auth_method === 'ldap_passthrough' ? 'bg-blue-400' : host.auth_method === 'key' ? 'bg-green-400' : 'bg-amber-400'}`} />
                    <h3 className="font-medium text-[var(--text-primary)]">{host.name}</h3>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${
                    host.auth_method === 'ldap_passthrough' ? 'bg-blue-500/10 text-blue-500' :
                    host.auth_method === 'key' ? 'bg-green-500/10 text-green-500' :
                    'bg-amber-500/10 text-amber-500'
                  }`}>
                    {host.auth_method === 'ldap_passthrough' ? 'LDAP' : host.auth_method === 'key' ? 'SSH Key' : 'Password'}
                  </span>
                </div>

                <div className="space-y-1 text-xs text-[var(--text-tertiary)] mb-4">
                  <div className="flex items-center gap-1.5">
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" />
                    </svg>
                    <span className="font-mono">{host.hostname}:{host.port}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                    </svg>
                    <span>{host.username}@{host.hostname}</span>
                  </div>
                  {host.description && <p className="mt-2 text-[var(--text-secondary)]">{host.description}</p>}
                  {host.group_ids && host.group_ids.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-2">
                      {host.group_ids.map(gid => {
                        const group = groups.find(g => g.id === gid);
                        if (!group) return null;
                        return (
                          <span
                            key={gid}
                            className="px-1.5 py-0.5 rounded text-[10px] font-medium"
                            style={{ backgroundColor: group.color + '20', color: group.color }}
                          >
                            {group.name}
                          </span>
                        );
                      })}
                    </div>
                  )}
                  {host.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-2">
                      {host.tags.map(tag => (
                        <span key={tag} className="px-1.5 py-0.5 bg-[var(--bg)] rounded text-[10px]">{tag}</span>
                      ))}
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  <button
                    onClick={() => handleConnect(host)}
                    className="flex-1 btn btn-primary btn-sm justify-center"
                  >
                    <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                    Connect
                  </button>
                  {isAdmin && (
                    <>
                      <button
                        onClick={() => handleTestConnection(host.id)}
                        className="btn btn-secondary btn-sm"
                        title="Test connection"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                      </button>
                      <button
                        onClick={() => { setEditingHost(host); setShowAddHost(true); }}
                        className="btn btn-secondary btn-sm"
                        title="Edit"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                        </svg>
                      </button>
                      <button
                        onClick={() => handleDeleteHost(host.id)}
                        className="btn btn-secondary btn-sm text-red-400 hover:text-red-300"
                        title="Delete"
                      >
                        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                      </button>
                    </>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      );
      })()}

      {/* Add/Edit Host Dialog */}
      {showAddHost && (
        <HostFormDialog
          host={editingHost}
          groups={groups}
          onClose={() => { setShowAddHost(false); setEditingHost(null); }}
          onSaved={() => { setShowAddHost(false); setEditingHost(null); fetchHosts(); fetchGroups(); }}
        />
      )}

      {/* Group Management Dialog */}
      {showGroupDialog && (
        <GroupDialog
          group={editingGroup}
          onClose={() => { setShowGroupDialog(false); setEditingGroup(null); }}
          onSaved={() => { setShowGroupDialog(false); setEditingGroup(null); fetchGroups(); fetchHosts(); }}
          onDelete={(id) => { setShowGroupDialog(false); setEditingGroup(null); handleDeleteGroup(id); }}
        />
      )}

      {/* Delete Host Confirmation */}
      <ConfirmModal
        open={!!deleteHostConfirm}
        title="Delete this host?"
        description="This host will be permanently removed. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDeleteHost}
        onCancel={() => setDeleteHostConfirm(null)}
      />

      {/* Delete Group Confirmation */}
      <ConfirmModal
        open={!!deleteGroupConfirm}
        title="Delete this group?"
        description="Hosts in this group will not be deleted. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDeleteGroup}
        onCancel={() => setDeleteGroupConfirm(null)}
      />

      {/* Password Dialog for LDAP Passthrough */}
      {showPasswordDialog && activeHost && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-[var(--surface)] border border-[var(--border)] rounded-xl p-6 w-[400px] shadow-2xl">
            <h3 className="text-lg font-medium text-[var(--text-primary)] mb-4">
              Enter LDAP Password
            </h3>
            <p className="text-sm text-[var(--text-secondary)] mb-4">
              Enter your LDAP password to connect to <strong>{activeHost.hostname}</strong> as <strong>{activeHost.username}</strong>.
              Your password is used only for this session and is never stored.
            </p>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="Your LDAP password"
              className="input w-full mb-4"
              autoFocus
              onKeyDown={e => { if (e.key === 'Enter' && password) handlePasswordConnect(); }}
            />
            <div className="flex justify-end gap-2">
              <button onClick={() => { setShowPasswordDialog(false); setPassword(''); }} className="btn btn-secondary">
                Cancel
              </button>
              <button
                onClick={handlePasswordConnect}
                disabled={!password || connecting}
                className="btn btn-primary"
              >
                {connecting ? 'Connecting...' : 'Connect'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Host Form Dialog ─────────────────────────────────────────────────────────

function HostFormDialog({ host, groups, onClose, onSaved }: {
  host: SSHHost | null;
  groups: SSHHostGroup[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState({
    name: host?.name || '',
    hostname: host?.hostname || '',
    port: host?.port || 22,
    username: host?.username || 'root',
    auth_method: host?.auth_method || 'password',
    ssh_key: '',
    password: '',
    tags: host?.tags?.join(', ') || '',
    description: host?.description || '',
  });
  const [selectedGroups, setSelectedGroups] = useState<string[]>(host?.group_ids || []);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    if (!form.name || !form.hostname) {
      setError('Name and hostname are required');
      return;
    }
    setSaving(true);
    setError('');

    const body = {
      name: form.name,
      hostname: form.hostname,
      port: form.port,
      username: form.username,
      auth_method: form.auth_method,
      ssh_key: form.ssh_key,
      password: form.password,
      tags: form.tags.split(',').map(t => t.trim()).filter(Boolean),
      description: form.description,
    };

    try {
      const url = host ? `${getBase()}/api/v1/ssh-hosts/${host.id}` : `${getBase()}/api/v1/ssh-hosts`;
      const method = host ? 'PUT' : 'POST';
      const res = await fetch(url, {
        method,
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to save host');
      }
      const result = await res.json().catch(() => ({}));
      onSaved();
      // Save group assignments
      const hostId = host ? host.id : result.id;
      if (hostId) {
        try {
          await fetch(`${getBase()}/api/v1/ssh-hosts/${hostId}/groups`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ group_ids: selectedGroups }),
          });
        } catch { /* groups are secondary */ }
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };

  const toggleGroup = (gid: string) => {
    setSelectedGroups(prev => prev.includes(gid) ? prev.filter(id => id !== gid) : [...prev, gid]);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-[var(--surface)] border border-[var(--border)] rounded-xl p-6 w-[500px] shadow-2xl max-h-[90vh] overflow-y-auto">
        <h3 className="text-lg font-medium text-[var(--text-primary)] mb-4">
          {host ? 'Edit Host' : 'Add SSH Host'}
        </h3>

        {error && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-sm text-red-500">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Name *</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input mt-1" placeholder="Production Server" />
            </div>
            <div>
              <label className="label">Hostname *</label>
              <input value={form.hostname} onChange={e => setForm({ ...form, hostname: e.target.value })} className="input mt-1 font-mono" placeholder="192.168.1.100" />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Port</label>
              <input type="number" value={form.port} onChange={e => setForm({ ...form, port: parseInt(e.target.value) || 22 })} className="input mt-1" />
            </div>
            <div>
              <label className="label">Username</label>
              <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} className="input mt-1 font-mono" placeholder="root" />
            </div>
          </div>

          <div>
            <label className="label">Authentication Method</label>
            <select value={form.auth_method} onChange={e => setForm({ ...form, auth_method: e.target.value })} className="input mt-1">
              <option value="password">Password</option>
              <option value="key">SSH Key</option>
              <option value="ldap_passthrough">LDAP Passthrough</option>
            </select>
            <p className="text-xs text-[var(--text-tertiary)] mt-1">
              {form.auth_method === 'ldap_passthrough' && 'Users will enter their LDAP password when connecting'}
              {form.auth_method === 'key' && 'Store an SSH private key for this host'}
              {form.auth_method === 'password' && 'Store a password for this host (encrypted)'}
            </p>
          </div>

          {form.auth_method === 'key' && (
            <div>
              <label className="label">SSH Private Key</label>
              <textarea
                value={form.ssh_key}
                onChange={e => setForm({ ...form, ssh_key: e.target.value })}
                className="input mt-1 font-mono text-xs h-24 resize-none"
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              />
            </div>
          )}

          {form.auth_method === 'password' && (
            <div>
              <label className="label">Password</label>
              <input
                type="password"
                value={form.password}
                onChange={e => setForm({ ...form, password: e.target.value })}
                className="input mt-1"
                placeholder={host ? '(unchanged)' : 'Enter password'}
              />
            </div>
          )}

          <div>
            <label className="label">Tags (comma-separated)</label>
            <input value={form.tags} onChange={e => setForm({ ...form, tags: e.target.value })} className="input mt-1" placeholder="production, web-server" />
          </div>

          <div>
            <label className="label">Description</label>
            <textarea
              value={form.description}
              onChange={e => setForm({ ...form, description: e.target.value })}
              className="input mt-1 h-16 resize-none"
              placeholder="Optional description..."
            />
          </div>

          {groups.length > 0 && (
            <div>
              <label className="label">Groups</label>
              <div className="flex flex-wrap gap-2 mt-1">
                {groups.map(g => (
                  <button
                    key={g.id}
                    type="button"
                    onClick={() => toggleGroup(g.id)}
                    className={`px-2.5 py-1 rounded-lg text-xs font-medium border transition-colors ${
                      selectedGroups.includes(g.id)
                        ? 'border-transparent text-white'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                    style={selectedGroups.includes(g.id) ? { backgroundColor: g.color } : {}}
                  >
                    {g.name}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button onClick={handleSubmit} disabled={saving} className="btn btn-primary">
            {saving ? 'Saving...' : (host ? 'Update' : 'Add Host')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Group Dialog ─────────────────────────────────────────────────────────────

function GroupDialog({ group, onClose, onSaved, onDelete }: {
  group: SSHHostGroup | null;
  onClose: () => void;
  onSaved: () => void;
  onDelete: (id: string) => void;
}) {
  const [form, setForm] = useState({
    name: group?.name || '',
    description: group?.description || '',
    color: group?.color || '#7aa2f7',
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    if (!form.name) {
      setError('Group name is required');
      return;
    }
    setSaving(true);
    setError('');

    try {
      const url = group ? `${getBase()}/api/v1/ssh-host-groups/${group.id}` : `${getBase()}/api/v1/ssh-host-groups`;
      const method = group ? 'PUT' : 'POST';
      const res = await fetch(url, {
        method,
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || 'Failed to save group');
      }
      onSaved();
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };

  const presetColors = ['#7aa2f7', '#f7768e', '#9ece6a', '#e0af68', '#bb9af7', '#7dcfff', '#ff9e64'];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-[var(--surface)] border border-[var(--border)] rounded-xl p-6 w-[450px] shadow-2xl">
        <h3 className="text-lg font-medium text-[var(--text-primary)] mb-4">
          {group ? 'Edit Group' : 'New Group'}
        </h3>

        {error && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-sm text-red-500">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <label className="label">Name *</label>
            <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input mt-1" placeholder="Production Servers" />
          </div>

          <div>
            <label className="label">Description</label>
            <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input mt-1" placeholder="Optional description" />
          </div>

          <div>
            <label className="label">Color</label>
            <div className="flex items-center gap-2 mt-1">
              {presetColors.map(c => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setForm({ ...form, color: c })}
                  className={`w-7 h-7 rounded-full transition-transform ${form.color === c ? 'ring-2 ring-offset-2 ring-offset-[var(--surface)] ring-[var(--text-secondary)]' : ''}`}
                  style={{ backgroundColor: c }}
                />
              ))}
              <input
                type="color"
                value={form.color}
                onChange={e => setForm({ ...form, color: e.target.value })}
                className="w-10 h-10 rounded cursor-pointer border border-[var(--border)] ml-2"
                title="Choose custom color"
              />
            </div>
          </div>
        </div>

        <div className="flex justify-between gap-2 mt-6">
          <div>
            {group && (
              <button onClick={() => onDelete(group.id)} className="btn btn-secondary text-red-400 hover:text-red-300">
                Delete Group
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <button onClick={onClose} className="btn btn-secondary">Cancel</button>
            <button onClick={handleSubmit} disabled={saving} className="btn btn-primary">
              {saving ? 'Saving...' : (group ? 'Update' : 'Create')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
