'use client';

import { useState, useEffect } from 'react';
import { connections as connectionsAPI, type Connection } from '@/lib/api';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

const TYPE_INFO: Record<string, { icon: string; label: string; color: string }> = {
  kubernetes: { icon: 'kubernetes', label: 'Kubernetes', color: '#326CE5' },
  git: { icon: 'git', label: 'Git', color: '#F05032' },
  gitlab: { icon: 'gitlab', label: 'GitLab', color: '#FC6D26' },
  jira: { icon: 'jira', label: 'Jira', color: '#0052CC' },
  ci: { icon: 'cicd', label: 'CI/CD', color: '#10B981' },
  ai: { icon: 'ai', label: 'AI Provider', color: '#8B5CF6' },
  storage: { icon: 'storage', label: 'Storage', color: '#F59E0B' },
};

const STATUS_COLORS: Record<string, string> = {
  connected: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/20',
  disconnected: 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]',
  error: 'bg-red-500/15 text-red-500 border-red-500/20',
};

export default function ConnectionDetailClient({ connectionId }: { connectionId: string }) {
  const router = useRouter();
  const [conn, setConn] = useState<Connection | null>(null);
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);
  const [editing, setEditing] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [browseData, setBrowseData] = useState<unknown>(null);
  const [browseResource, setBrowseResource] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(null);

  // Fetch connection client-side (server-side has no auth token)
  useEffect(() => {
    connectionsAPI.get(connectionId)
      .then(setConn)
      .catch(() => setConn(null))
      .finally(() => setLoading(false));
  }, [connectionId]);

  const [editData, setEditData] = useState({
    name: conn?.name ?? '',
    description: conn?.description ?? '',
    notes: conn?.notes ?? '',
  });

  // Sync editData when connection loads
  useEffect(() => {
    if (conn) {
      setEditData({ name: conn.name, description: conn.description, notes: conn.notes });
    }
  }, [conn]);

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg flex items-center justify-center">
        <div className="flex items-center gap-2 text-[var(--text-tertiary)]">
          <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <p className="text-[13px]">Loading connection...</p>
        </div>
      </div>
    );
  }

  if (!conn) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg flex items-center justify-center">
        <div className="text-center">
          <p className="text-[var(--text-secondary)] mb-4">Connection not found</p>
          <Link href="/connections" className="btn btn-primary">Back to Connections</Link>
        </div>
      </div>
    );
  }

  const typeInfo = TYPE_INFO[conn.type] || { icon: 'plugin', label: conn.type, color: '#666' };

  const handleTest = async () => {
    setTesting(true);
    setFeedback(null);
    try {
      const result = await connectionsAPI.test(conn.id);
      setConn(prev => prev ? { ...prev, status: result.status, last_check_at: new Date().toISOString() } : prev);
      const ok = result.status === 'connected' || result.status === 'ok' || result.status === 'healthy';
      setFeedback({ ok, text: result.message || (ok ? 'Connection test passed' : 'Connection test failed') });
    } catch (err) {
      setFeedback({ ok: false, text: `Test failed: ${err instanceof Error ? err.message : 'Unknown error'}` });
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    setFeedback(null);
    try {
      const updated = await connectionsAPI.update(conn.id, editData);
      setConn(updated);
      setEditing(false);
      setFeedback({ ok: true, text: 'Connection updated successfully' });
    } catch (err) {
      setFeedback({ ok: false, text: `Update failed: ${err instanceof Error ? err.message : 'Unknown error'}` });
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    setFeedback(null);
    try {
      await connectionsAPI.delete(conn.id);
      router.push('/connections');
    } catch (err) {
      setFeedback({ ok: false, text: `Delete failed: ${err instanceof Error ? err.message : 'Unknown error'}` });
      setShowDeleteConfirm(false);
    } finally {
      setDeleting(false);
    }
  };

  const handleBrowse = async (resource: string) => {
    setBrowsing(true);
    setBrowseResource(resource);
    setBrowseData(null);
    try {
      const result = await connectionsAPI.browse(conn.id, resource);
      setBrowseData(result.data);
    } catch (err) {
      setBrowseData({ error: String(err) });
    } finally {
      setBrowsing(false);
    }
  };

  const browseActions: Record<string, { label: string; resource: string }[]> = {
    gitlab: [
      { label: 'List Groups', resource: 'list_groups' },
      { label: 'List Repositories', resource: 'list_repos' },
      { label: 'List Pipelines', resource: 'list_pipelines' },
    ],
    git: [
      { label: 'List Groups', resource: 'list_groups' },
      { label: 'List Repositories', resource: 'list_repos' },
      { label: 'List Pipelines', resource: 'list_pipelines' },
    ],
    jira: [
      { label: 'List Projects', resource: 'list_projects' },
      { label: 'List Issues', resource: 'list_issues' },
    ],
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
    <div className="px-6 py-6 space-y-6">
      <div className="flex items-center gap-2 text-sm text-[var(--text-secondary)] page-animate">
        <Link href="/connections" className="hover:text-[var(--text-primary)]">Connections</Link>
        <span>/</span>
        <span className="text-[var(--text-primary)]">{conn.name}</span>
      </div>

      <div className="bg-[var(--surface)] rounded-xl border border-[var(--border)] overflow-hidden page-animate" style={{ borderRadius: '12px' }}>
        {/* Inline feedback */}
        {feedback && (
          <div className={`mx-6 mt-6 rounded-xl border p-4 flex items-start justify-between gap-3 ${
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
        <div className="p-6 border-b border-[var(--border)]">
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-4">
              <div
                className="w-14 h-14 rounded-xl flex items-center justify-center"
                style={{ backgroundColor: typeInfo.color + '20' }}
              >
                <BrandIcon name={typeInfo.icon} size={28} style={{ color: typeInfo.color }} />
              </div>
              <div>
                {editing ? (
                  <input
                    type="text"
                    value={editData.name}
                    onChange={e => setEditData({ ...editData, name: e.target.value })}
                    className="text-2xl font-bold border border-[var(--border)] rounded-lg px-3 py-1"
                  />
                ) : (
                  <h1 className="page-title-modern !mb-0">{conn.name}</h1>
                )}
                <div className="flex items-center gap-2 mt-1">
                  <span className="text-sm text-[var(--text-tertiary)]">{typeInfo.label}</span>
                  <span className={`text-xs px-2 py-0.5 rounded-full border ${STATUS_COLORS[conn.status] || STATUS_COLORS.disconnected}`}>
                    {conn.status}
                  </span>
                </div>
              </div>
            </div>
            <div className="flex gap-2">
              <button
                onClick={handleTest}
                disabled={testing}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {testing ? 'Testing...' : 'Test Connection'}
              </button>
              {editing ? (
                <>
                  <button
                    onClick={handleSave}
                    className="px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors"
                  >
                    Save
                  </button>
                  <button
                    onClick={() => setEditing(false)}
                    className="px-4 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--bg)] transition-colors"
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <button
                  onClick={() => setEditing(true)}
                  className="px-4 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--bg)] transition-colors"
                >
                  Edit
                </button>
              )}
              <button
                onClick={() => setShowDeleteConfirm(true)}
                className="px-4 py-2 border border-red-500/20 text-red-500 rounded-lg hover:bg-red-500/10 transition-colors"
              >
                Delete
              </button>
            </div>
          </div>
        </div>

        <div className="p-6 space-y-6">
          <div>
            <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-2">Description</h3>
            {editing ? (
              <textarea
                value={editData.description}
                onChange={e => setEditData({ ...editData, description: e.target.value })}
                rows={3}
                className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            ) : (
              <p className="text-[var(--text-secondary)]">{conn.description || 'No description provided'}</p>
            )}
          </div>

          <div>
            <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-2">Configuration</h3>
            <div className="bg-[var(--bg)] rounded-lg p-4 font-mono text-sm">
              {Object.entries(conn.config).map(([key, value]) => (
                <div key={key} className="mb-2">
                  <span className="text-[var(--text-tertiary)]">{key}:</span>{' '}
                  <span className="text-[var(--text-primary)]">
                    {key.toLowerCase().includes('token') || key.toLowerCase().includes('key') || key.toLowerCase().includes('secret')
                      ? '••••••••'
                      : typeof value === 'string' && value.length > 100
                      ? value.substring(0, 100) + '...'
                      : JSON.stringify(value)}
                  </span>
                </div>
              ))}
              {Object.keys(conn.config).length === 0 && (
                <span className="text-[var(--text-tertiary)]">No configuration</span>
              )}
            </div>
          </div>

          <div>
            <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-2">Labels</h3>
            {Object.keys(conn.labels).length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {Object.entries(conn.labels).map(([k, v]) => (
                  <span key={k} className="px-3 py-1 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-lg text-sm">
                    {k} = {v}
                  </span>
                ))}
              </div>
            ) : (
              <p className="text-[var(--text-tertiary)] text-sm">No labels</p>
            )}
          </div>

          <div>
            <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-2">Notes</h3>
            {editing ? (
              <textarea
                value={editData.notes}
                onChange={e => setEditData({ ...editData, notes: e.target.value })}
                rows={4}
                className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            ) : (
              <p className="text-[var(--text-secondary)] whitespace-pre-wrap">{conn.notes || 'No notes'}</p>
            )}
          </div>

          <div className="pt-4 border-t border-[var(--border)]">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-[var(--text-tertiary)]">Created:</span>{' '}
                <span className="text-[var(--text-primary)]">{new Date(conn.created_at).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-[var(--text-tertiary)]">Updated:</span>{' '}
                <span className="text-[var(--text-primary)]">{new Date(conn.updated_at).toLocaleString()}</span>
              </div>
              {conn.last_check_at && (
                <div>
                  <span className="text-[var(--text-tertiary)]">Last tested:</span>{' '}
                  <span className="text-[var(--text-primary)]">{new Date(conn.last_check_at).toLocaleString()}</span>
                </div>
              )}
            </div>
          </div>

          {/* Browse Resources Section */}
          {browseActions[conn.type] && (
            <div className="pt-4 border-t border-[var(--border)]">
              <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-3">Browse Resources</h3>
              <p className="text-xs text-[var(--text-tertiary)] mb-3">
                Explore available resources through this connection. Data is fetched in real-time from the connected service.
              </p>
              <div className="flex flex-wrap gap-2 mb-4">
                {browseActions[conn.type].map((action) => (
                  <button
                    key={action.resource}
                    onClick={() => handleBrowse(action.resource)}
                    disabled={browsing}
                    className="px-3 py-1.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-lg text-sm hover:bg-[var(--border)] disabled:opacity-50 transition-colors"
                  >
                    {browsing && browseResource === action.resource ? 'Loading...' : action.label}
                  </button>
                ))}
              </div>
              {browseData !== null && (
                <div className="bg-[var(--bg)] rounded-lg p-4 max-h-80 overflow-auto">
                  <pre className="text-xs font-mono text-[var(--text-secondary)] whitespace-pre-wrap">
                    {JSON.stringify(browseData, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Delete Confirmation */}
      <ConfirmModal
        open={showDeleteConfirm}
        title="Delete Connection"
        description={`Are you sure you want to delete "${conn.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
    </div>
  );
}
