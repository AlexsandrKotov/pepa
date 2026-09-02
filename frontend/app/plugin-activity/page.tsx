'use client';

import { useState, useEffect, useCallback } from 'react';
import { pluginActivity, listUsers, type SSHCommandEntry, type PluginActionEntry } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

type Tab = 'ssh' | 'actions';

function formatTime(dateStr: string) {
  const d = new Date(dateStr);
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function formatRelativeTime(dateStr: string) {
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return 'just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffH = Math.floor(diffMin / 60);
  if (diffH < 24) return `${diffH}h ago`;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

const ACTION_LABELS: Record<string, string> = {
  create_vm: 'Create VM',
  delete_vm: 'Delete VM',
  start_vm: 'Start VM',
  stop_vm: 'Stop VM',
  shutdown_vm: 'Shutdown VM',
  reboot_vm: 'Reboot VM',
  suspend_vm: 'Suspend VM',
  create_container: 'Create Container',
  delete_container: 'Delete Container',
  start_container: 'Start Container',
  stop_container: 'Stop Container',
  deploy_docker: 'Deploy Docker',
  create_snapshot: 'Create Snapshot',
  delete_snapshot: 'Delete Snapshot',
  revert_snapshot: 'Revert Snapshot',
  create_bucket: 'Create Bucket',
  delete_bucket: 'Delete Bucket',
  upload_object: 'Upload Object',
  delete_object: 'Delete Object',
};

function getActionStyle(action: string) {
  if (action.startsWith('create_snapshot') || action.startsWith('delete_snapshot') || action.startsWith('revert_snapshot')) return { bg: 'bg-purple-500/15', text: 'text-purple-500' };
  if (action.startsWith('create_bucket') || action.startsWith('upload_object')) return { bg: 'bg-emerald-500/15', text: 'text-emerald-600' };
  if (action.startsWith('delete_bucket') || action.startsWith('delete_object')) return { bg: 'bg-red-500/15', text: 'text-red-500' };
  if (action.startsWith('create') || action.startsWith('deploy')) return { bg: 'bg-emerald-500/15', text: 'text-emerald-600' };
  if (action.startsWith('delete')) return { bg: 'bg-red-500/15', text: 'text-red-500' };
  if (action.startsWith('start') || action.startsWith('resume')) return { bg: 'bg-blue-500/15', text: 'text-blue-500' };
  if (action.startsWith('stop') || action.startsWith('shutdown') || action.startsWith('suspend')) return { bg: 'bg-orange-500/10', text: 'text-orange-600' };
  if (action.startsWith('reboot') || action.startsWith('revert')) return { bg: 'bg-amber-500/15', text: 'text-amber-600' };
  return { bg: 'bg-[var(--border-light)]', text: 'text-[var(--text-secondary)]' };
}

function getPluginStyle(pluginName: string) {
  switch (pluginName) {
    case 'proxmox': return 'bg-orange-500/15 text-orange-600';
    case 'vmware': return 'bg-blue-500/15 text-blue-500';
    case 's3': return 'bg-cyan-500/15 text-cyan-600';
    default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
  }
}

export default function PluginActivityPage() {
  const { isAdmin, hasPermission, loading: permLoading } = usePermission();

  if (permLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('plugin_activity', 'read')) {
    return <ForbiddenPage resource="plugin activity" />;
  }

  return <PluginActivityContent />;
}

function PluginActivityContent() {
  const [tab, setTab] = useState<Tab>('actions');
  const [userMap, setUserMap] = useState<Map<string, string>>(new Map());

  // SSH Commands state
  const [sshCommands, setSSHCommands] = useState<SSHCommandEntry[]>([]);
  const [sshTotal, setSSHTotal] = useState(0);
  const [sshPage, setSSHPage] = useState(1);
  const [sshLoading, setSSHLoading] = useState(false);
  const [sshError, setSSHError] = useState<string | null>(null);

  // Plugin Actions state
  const [actions, setActions] = useState<PluginActionEntry[]>([]);
  const [actionsTotal, setActionsTotal] = useState(0);
  const [actionsPage, setActionsPage] = useState(1);
  const [actionsLoading, setActionsLoading] = useState(false);
  const [actionsError, setActionsError] = useState<string | null>(null);
  const [pluginFilter, setPluginFilter] = useState('');

  // Fetch users for ID → name resolution
  useEffect(() => {
    listUsers().then(res => {
      const m = new Map<string, string>();
      for (const u of res.users) {
        m.set(u.id, u.name || u.email);
      }
      setUserMap(m);
    }).catch(() => {});
  }, []);

  const getUserDisplay = useCallback((userId?: string): string => {
    if (!userId) return 'system';
    if (userMap.has(userId)) return userMap.get(userId)!;
    return userId.slice(0, 8) + '...';
  }, [userMap]);

  // Fetch SSH commands
  const fetchSSH = useCallback(async () => {
    setSSHLoading(true);
    setSSHError(null);
    try {
      const data = await pluginActivity.listSSHCommands({ page: String(sshPage) });
      setSSHCommands(data.items || []);
      setSSHTotal(data.total || 0);
    } catch (err) {
      setSSHError(err instanceof Error ? err.message : 'Failed to load SSH commands');
    }
    setSSHLoading(false);
  }, [sshPage]);

  // Fetch plugin actions
  const fetchActions = useCallback(async () => {
    setActionsLoading(true);
    setActionsError(null);
    try {
      const params: Record<string, string> = { page: String(actionsPage) };
      if (pluginFilter) params.plugin_name = pluginFilter;
      const data = await pluginActivity.listPluginActions(params);
      setActions(data.items || []);
      setActionsTotal(data.total || 0);
    } catch (err) {
      setActionsError(err instanceof Error ? err.message : 'Failed to load plugin actions');
    }
    setActionsLoading(false);
  }, [actionsPage, pluginFilter]);

  useEffect(() => { if (tab === 'ssh') fetchSSH(); }, [tab, fetchSSH]);
  useEffect(() => { if (tab === 'actions') fetchActions(); }, [tab, fetchActions]);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Plugin Activity</h1>
            <p className="page-subtitle-modern">
              {tab === 'ssh'
                ? `${sshTotal.toLocaleString()} SSH commands recorded`
                : `${actionsTotal.toLocaleString()} plugin actions recorded`}
            </p>
          </div>
          <button
            onClick={() => { tab === 'ssh' ? fetchSSH() : fetchActions(); }}
            className="btn btn-secondary text-[12px] px-3 py-1.5"
            disabled={tab === 'ssh' ? sshLoading : actionsLoading}
          >
            {(tab === 'ssh' ? sshLoading : actionsLoading) ? '...' : 'Refresh'}
          </button>
        </div>

        {/* Tabs */}
        <div className="page-animate-up page-delay-1 flex items-center gap-1 border-b border-[var(--border-light)]">
          <button
            onClick={() => setTab('actions')}
            className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${
              tab === 'actions'
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
            }`}
          >
            Plugin Actions
            {actionsTotal > 0 && <span className="ml-2 text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded-full">{actionsTotal}</span>}
          </button>
          <button
            onClick={() => setTab('ssh')}
            className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors ${
              tab === 'ssh'
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
            }`}
          >
            SSH Commands
            {sshTotal > 0 && <span className="ml-2 text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded-full">{sshTotal}</span>}
          </button>
        </div>

        {/* Plugin filter for actions tab */}
        {tab === 'actions' && (
          <div className="flex items-center gap-3">
            <select
              value={pluginFilter}
              onChange={e => { setPluginFilter(e.target.value); setActionsPage(1); }}
              className="text-[12px] border border-[var(--border)] rounded-lg px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]"
            >
              <option value="">All plugins</option>
              <option value="proxmox">Proxmox</option>
              <option value="vmware">VMware</option>
              <option value="s3">S3</option>
            </select>
            {pluginFilter && (
              <button onClick={() => { setPluginFilter(''); setActionsPage(1); }}
                className="text-[12px] text-[var(--accent)] hover:underline">Clear filter</button>
            )}
          </div>
        )}

        {/* Error banners */}
        {tab === 'ssh' && sshError && (
          <div className="px-4 py-2.5 rounded-xl text-[13px] bg-red-500/10 text-red-500 border border-red-500/20">{sshError}</div>
        )}
        {tab === 'actions' && actionsError && (
          <div className="px-4 py-2.5 rounded-xl text-[13px] bg-red-500/10 text-red-500 border border-red-500/20">{actionsError}</div>
        )}

        {/* SSH Commands Table */}
        {tab === 'ssh' && (
          <div className="page-animate-up page-delay-2">
            <div className="table-container" style={{ borderRadius: '12px' }}>
              <table>
                <thead>
                  <tr>
                    <th style={{ width: '140px' }}>User</th>
                    <th style={{ width: '140px' }}>Host</th>
                    <th style={{ width: '80px' }}>SSH User</th>
                    <th>Command</th>
                    <th style={{ width: '100px' }}>Time</th>
                  </tr>
                </thead>
                <tbody>
                  {sshCommands.length === 0 && !sshLoading ? (
                    <tr>
                      <td colSpan={5} className="text-center py-10">
                        <div className="text-3xl mb-2 opacity-20">🖥️</div>
                        <p className="text-[13px] text-[var(--text-secondary)] mb-1">No SSH commands recorded</p>
                        <p className="text-[12px] text-[var(--text-tertiary)]">Commands executed via the SSH terminal will appear here</p>
                      </td>
                    </tr>
                  ) : sshCommands.length === 0 && sshLoading ? (
                    <tr><td colSpan={5} className="text-center py-10 text-[var(--text-tertiary)] text-[13px]">Loading...</td></tr>
                  ) : (
                    sshCommands.map(cmd => (
                      <tr key={cmd.id} className="hover:bg-[var(--bg)]">
                        <td>
                          <span className="text-[12px] text-[var(--text-primary)]">{getUserDisplay(cmd.user_id)}</span>
                        </td>
                        <td>
                          <span className="text-[12px] text-[var(--text-primary)] font-mono" title={cmd.host_id}>{cmd.host_name}</span>
                        </td>
                        <td>
                          <span className="text-[12px] text-[var(--text-secondary)] font-mono">{cmd.username}</span>
                        </td>
                        <td>
                          <code className="text-[12px] text-[var(--text-primary)] bg-[var(--bg)] px-2 py-0.5 rounded font-mono">{cmd.command}</code>
                        </td>
                        <td>
                          <span className="text-[11px] text-[var(--text-tertiary)]" title={formatRelativeTime(cmd.created_at)}>
                            {formatTime(cmd.created_at)}
                          </span>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {sshTotal > 50 && (
              <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)] mt-4">
                <span>Page {sshPage}</span>
                <div className="flex items-center gap-2">
                  <button onClick={() => setSSHPage(p => Math.max(1, p - 1))} disabled={sshPage <= 1}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Prev</button>
                  <button onClick={() => setSSHPage(p => p + 1)} disabled={sshCommands.length < 50}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Next</button>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Plugin Actions Table */}
        {tab === 'actions' && (
          <div className="page-animate-up page-delay-2">
            <div className="table-container" style={{ borderRadius: '12px' }}>
              <table>
                <thead>
                  <tr>
                    <th style={{ width: '80px' }}>Plugin</th>
                    <th style={{ width: '140px' }}>Action</th>
                    <th style={{ width: '80px' }}>Type</th>
                    <th style={{ width: '140px' }}>User</th>
                    <th style={{ width: '70px' }}>Status</th>
                    <th style={{ width: '120px' }}>IP</th>
                    <th style={{ width: '100px' }}>Time</th>
                  </tr>
                </thead>
                <tbody>
                  {actions.length === 0 && !actionsLoading ? (
                    <tr>
                      <td colSpan={7} className="text-center py-10">
                        <div className="text-3xl mb-2 opacity-20">🔌</div>
                        <p className="text-[13px] text-[var(--text-secondary)] mb-1">No plugin actions recorded</p>
                        <p className="text-[12px] text-[var(--text-tertiary)]">VM and container operations will appear here</p>
                      </td>
                    </tr>
                  ) : actions.length === 0 && actionsLoading ? (
                    <tr><td colSpan={7} className="text-center py-10 text-[var(--text-tertiary)] text-[13px]">Loading...</td></tr>
                  ) : (
                    actions.map(act => {
                      const style = getActionStyle(act.action);
                      return (
                        <tr key={act.id} className="hover:bg-[var(--bg)]">
                          <td>
                            <span className={`text-[11px] font-medium px-2 py-0.5 rounded ${getPluginStyle(act.plugin_name)}`}>
                              {act.plugin_name}
                            </span>
                          </td>
                          <td>
                            <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium ${style.bg} ${style.text}`}>
                              {ACTION_LABELS[act.action] || act.action}
                            </span>
                          </td>
                          <td>
                            <span className="text-[12px] text-[var(--text-secondary)] capitalize">{act.entity_type}</span>
                          </td>
                          <td>
                            <span className="text-[12px] text-[var(--text-primary)]">{getUserDisplay(act.user_id)}</span>
                          </td>
                          <td>
                            <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium ${
                              act.status === 'success' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'
                            }`}>
                              {act.status}
                            </span>
                          </td>
                          <td>
                            <span className="font-mono text-[11px] text-[var(--text-tertiary)]">{act.ip_address || '\u2014'}</span>
                          </td>
                          <td>
                            <span className="text-[11px] text-[var(--text-tertiary)]" title={formatRelativeTime(act.created_at)}>
                              {formatTime(act.created_at)}
                            </span>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {actionsTotal > 50 && (
              <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)] mt-4">
                <span>Page {actionsPage}</span>
                <div className="flex items-center gap-2">
                  <button onClick={() => setActionsPage(p => Math.max(1, p - 1))} disabled={actionsPage <= 1}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Prev</button>
                  <button onClick={() => setActionsPage(p => p + 1)} disabled={actions.length < 50}
                    className="px-3 py-1 rounded-md border border-[var(--border)] disabled:opacity-30 hover:bg-[var(--border-light)]">Next</button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
