'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { discovery, type DiscoveredService, type DeploymentInfo } from '@/lib/api';
import ConfirmModal from '@/components/ConfirmModal';

interface ServiceManagementPanelProps {
  service: DiscoveredService;
  onClose: () => void;
  onUpdate: () => void;
}

export default function ServiceManagementPanel({ service, onClose, onUpdate }: ServiceManagementPanelProps) {
  useEscapeKey(onClose);
  const [tab, setTab] = useState<'overview' | 'logs' | 'events' | 'edit'>('overview');
  const isDockerContainer = service.source === 'docker-container' || service.source === 'docker';
  const [deployInfo, setDeployInfo] = useState<DeploymentInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [logs, setLogs] = useState<string>('');
  const [events, setEvents] = useState<Array<Record<string, unknown>>>([]);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Edit state
  const [editImage, setEditImage] = useState('');
  const [editReplicas, setEditReplicas] = useState(0);
  const [editEnv, setEditEnv] = useState<Array<{ key: string; value: string }>>([]);

  useEffect(() => {
    loadDeployInfo();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const loadDeployInfo = async () => {
    if (isDockerContainer) return; // Not applicable for Docker containers
    setLoading(true);
    try {
      const info = await discovery.k8sGet(service.cluster, service.namespace, service.name);
      setDeployInfo(info);
      setEditImage(info.image || '');
      setEditReplicas(info.replicas || 0);
      if (info.env) {
        setEditEnv(Object.entries(info.env).map(([key, value]) => ({ key, value })));
      }
    } catch (err) {
      console.error('Failed to load deployment info:', err);
    } finally {
      setLoading(false);
    }
  };

  const loadLogs = async () => {
    setLoading(true);
    try {
      if (isDockerContainer) {
        const data = await discovery.dockerContainerLogs(service.cluster, service.name, 200);
        setLogs(data.logs || 'No logs available');
      } else {
        const data = await discovery.k8sLogs(service.cluster, service.namespace, service.name, 200);
        setLogs(data.logs || 'No logs available');
      }
    } catch (err) {
      setLogs(`Error loading logs: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const loadEvents = async () => {
    if (isDockerContainer) return; // Not applicable for Docker containers
    setLoading(true);
    try {
      const data = await discovery.k8sEvents(service.cluster, service.namespace, service.name);
      setEvents(data.events || []);
    } catch (err) {
      console.error('Failed to load events:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (tab === 'logs') loadLogs();
    if (tab === 'events') loadEvents();
  }, [tab]); // eslint-disable-line react-hooks/exhaustive-deps

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text });
    setTimeout(() => setMessage(null), 4000);
  };

  const handleScale = async (replicas: number) => {
    setActionLoading('scale');
    // Optimistically update the UI immediately
    if (deployInfo) {
      setDeployInfo({ ...deployInfo, replicas });
    }
    try {
      const result = await discovery.k8sScale(service.cluster, service.namespace, service.name, replicas);
      showMessage('success', result.message);
      setEditReplicas(replicas);
      await loadDeployInfo();
      onUpdate();
    } catch (err) {
      showMessage('error', `Scale failed: ${err}`);
      // Revert optimistic update on failure
      await loadDeployInfo();
    } finally {
      setActionLoading(null);
    }
  };

  const handleRestart = async () => {
    setActionLoading('restart');
    try {
      const result = await discovery.k8sRestart(service.cluster, service.namespace, service.name);
      showMessage('success', result.message);
      await loadDeployInfo();
      onUpdate();
    } catch (err) {
      showMessage('error', `Restart failed: ${err}`);
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async () => {
    setShowDeleteConfirm(true);
  };

  const confirmDelete = async () => {
    setDeleting(true);
    setActionLoading('delete');
    try {
      const result = await discovery.k8sDelete(service.cluster, service.namespace, service.name);
      showMessage('success', result.message);
      onUpdate();
      setTimeout(onClose, 1500);
    } catch (err) {
      showMessage('error', `Delete failed: ${err}`);
    } finally {
      setActionLoading(null);
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  };

  const handleUpdate = async () => {
    setActionLoading('update');
    try {
      const envMap: Record<string, string> = {};
      editEnv.forEach(e => { if (e.key) envMap[e.key] = e.value; });
      const result = await discovery.k8sUpdate(service.cluster, service.namespace, service.name, {
        image: editImage !== deployInfo?.image ? editImage : undefined,
        env: envMap,
        replicas: editReplicas !== deployInfo?.replicas ? editReplicas : undefined,
      });
      showMessage('success', result.message);
      await loadDeployInfo();
      onUpdate();
    } catch (err) {
      showMessage('error', `Update failed: ${err}`);
    } finally {
      setActionLoading(null);
    }
  };

  const addEnvVar = () => setEditEnv([...editEnv, { key: '', value: '' }]);
  const removeEnvVar = (idx: number) => setEditEnv(editEnv.filter((_, i) => i !== idx));
  const updateEnvVar = (idx: number, field: 'key' | 'value', val: string) => {
    const newEnv = [...editEnv];
    newEnv[idx] = { ...newEnv[idx], [field]: val };
    setEditEnv(newEnv);
  };

  // ESC to close
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') onClose();
  }, [onClose]);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = '';
    };
  }, [handleKeyDown]);

  const healthColor = service.health === 'healthy' ? 'bg-green-500' : service.health === 'degraded' ? 'bg-yellow-500' : service.health === 'failed' ? 'bg-red-500' : 'bg-gray-400';

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/30 z-[110] transition-opacity" onClick={onClose} />
      {/* Slide-out Panel */}
      <div className="fixed top-0 right-0 bottom-0 w-full max-w-xl bg-[var(--surface)] border-l border-[var(--border)] z-[120] shadow-2xl flex flex-col animate-slide-in-right">
        {/* Health indicator bar */}
        <div className={`h-1 w-full ${healthColor}`} />
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)] truncate">{service.name}</h2>
              <span className={`badge-sm ${service.source === 'pepa' ? 'badge-accent' : service.source === 'argocd' ? 'badge-warning' : 'badge-info'}`}>
                {service.source}
              </span>
            </div>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">
              {service.cluster} / {service.namespace} &middot; {service.health}
            </p>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <button
              onClick={() => window.open(`/services?q=${encodeURIComponent(service.name)}`, '_blank')}
              className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--accent)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
              title="Open in Services page"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 6H5.25A2.25 2.25 0 003 8.25v10.5A2.25 2.25 0 005.25 21h10.5A2.25 2.25 0 0018 18.75V10.5m-10.5 6L21 3m0 0h-5.25M21 3v5.25" />
              </svg>
            </button>
            <button onClick={onClose} className="p-1.5 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--border-light)] rounded-lg transition-colors" title="Close (Esc)">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Message */}
        {message && (
          <div className={`mx-5 mt-3 px-3 py-2 rounded-lg text-[12px] ${message.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'}`}>
            {message.text}
          </div>
        )}

        {/* Tabs */}
        <div className="flex gap-0.5 px-5 pt-2 border-b border-[var(--border)] shrink-0">
          {(['overview', 'logs', 'events', 'edit'] as const).map(t => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-2 text-[12px] font-medium transition-colors ${tab === t ? 'text-[var(--accent)] border-b-2 border-[var(--accent)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
            >
              {t === 'overview' ? 'Overview' : t === 'logs' ? 'Logs' : t === 'events' ? 'Events' : 'Edit'}
            </button>
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-5">
          {loading && tab === 'overview' ? (
            <p className="text-[13px] text-[var(--text-secondary)]">Loading...</p>
          ) : tab === 'overview' && isDockerContainer ? (
            <div className="space-y-5">
              {/* Docker Container Info */}
              <div className="grid grid-cols-2 gap-3">
                <InfoCard label="Status" value={service.status} />
                <InfoCard label="Health" value={service.health} />
                <InfoCard label="Image" value={service.image || 'N/A'} />
                <InfoCard label="Host" value={service.cluster} />
              </div>
              {service.labels && Object.keys(service.labels).length > 0 && (
                <div>
                  <h3 className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2">Labels</h3>
                  <div className="bg-[var(--border-light)] rounded-lg overflow-hidden">
                    {Object.entries(service.labels).slice(0, 20).map(([key, value]) => (
                      <div key={key} className="flex border-b border-[var(--border)] last:border-b-0">
                        <span className="text-[11px] font-mono font-medium text-[var(--text-primary)] px-3 py-1.5 bg-[var(--surface)] border-r border-[var(--border)] min-w-[150px]">{key}</span>
                        <span className="text-[11px] font-mono text-[var(--text-secondary)] px-3 py-1.5 truncate">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : tab === 'overview' && deployInfo ? (
            <div className="space-y-5">
              {/* Quick Actions */}
              <div className="flex flex-wrap gap-2">
                <div className="flex items-center gap-2 bg-[var(--border-light)] rounded-lg px-3 py-2">
                  <span className="text-[11px] text-[var(--text-tertiary)]">Scale:</span>
                  <button
                    onClick={() => handleScale(Math.max(0, (deployInfo.replicas || 1) - 1))}
                    disabled={actionLoading === 'scale'}
                    className="btn-sm bg-[var(--surface)] border border-[var(--border)] text-[12px] px-2 py-0.5 rounded hover:bg-[var(--border-light)]"
                  >-1</button>
                  <span className="text-[13px] font-semibold text-[var(--text-primary)] min-w-[24px] text-center">{deployInfo.replicas}</span>
                  <button
                    onClick={() => handleScale((deployInfo.replicas || 0) + 1)}
                    disabled={actionLoading === 'scale'}
                    className="btn-sm bg-[var(--surface)] border border-[var(--border)] text-[12px] px-2 py-0.5 rounded hover:bg-[var(--border-light)]"
                  >+1</button>
                  <button
                    onClick={() => handleScale(0)}
                    disabled={actionLoading === 'scale'}
                    className="btn-sm bg-yellow-500/10 border border-yellow-500/20 text-yellow-600 text-[11px] px-2 py-0.5 rounded hover:bg-yellow-500/15"
                  >Stop</button>
                </div>
                <button
                  onClick={handleRestart}
                  disabled={actionLoading === 'restart'}
                  className="btn-sm bg-blue-500/10 border border-blue-500/20 text-blue-500 text-[12px] px-3 py-1.5 rounded-lg hover:bg-blue-500/15 disabled:opacity-50"
                >
                  {actionLoading === 'restart' ? 'Restarting...' : '🔄 Restart'}
                </button>
                <button
                  onClick={handleDelete}
                  disabled={actionLoading === 'delete'}
                  className="btn-sm bg-red-500/10 border border-red-500/20 text-red-500 text-[12px] px-3 py-1.5 rounded-lg hover:bg-red-500/15 disabled:opacity-50"
                >
                  {actionLoading === 'delete' ? 'Deleting...' : '🗑 Delete'}
                </button>
              </div>

              {/* Info Grid */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                <InfoCard label="Replicas" value={`${deployInfo.ready_replicas}/${deployInfo.replicas}`} />
                <InfoCard label="Strategy" value={deployInfo.strategy || 'RollingUpdate'} />
                <InfoCard label="Created" value={deployInfo.created_at ? new Date(deployInfo.created_at).toLocaleDateString() : 'N/A'} />
                <InfoCard label="Available" value={`${deployInfo.available_replicas || 0}`} />
              </div>

              {/* Images */}
              <div>
                <h3 className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2">Images</h3>
                <div className="space-y-1">
                  {(deployInfo.images || [deployInfo.image]).map((img, i) => (
                    <div key={i} className="text-[11px] font-mono bg-[var(--border-light)] px-3 py-1.5 rounded text-[var(--text-secondary)] truncate">
                      {img}
                    </div>
                  ))}
                </div>
              </div>

              {/* Environment Variables */}
              {deployInfo.env && Object.keys(deployInfo.env).length > 0 && (
                <div>
                  <h3 className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2">Environment Variables</h3>
                  <div className="bg-[var(--border-light)] rounded-lg overflow-hidden">
                    {Object.entries(deployInfo.env).map(([key, value]) => (
                      <div key={key} className="flex border-b border-[var(--border)] last:border-b-0">
                        <span className="text-[11px] font-mono font-medium text-[var(--text-primary)] px-3 py-1.5 bg-[var(--surface)] border-r border-[var(--border)] min-w-[150px]">{key}</span>
                        <span className="text-[11px] font-mono text-[var(--text-secondary)] px-3 py-1.5 truncate">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Resources */}
              {(deployInfo.resource_limits || deployInfo.resource_requests) && (
                <div>
                  <h3 className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2">Resources</h3>
                  <div className="grid grid-cols-2 gap-3">
                    {deployInfo.resource_requests && (
                      <div className="bg-[var(--border-light)] rounded-lg p-3">
                        <span className="text-[10px] text-[var(--text-tertiary)] uppercase">Requests</span>
                        {Object.entries(deployInfo.resource_requests).map(([k, v]) => (
                          <div key={k} className="text-[11px] font-mono text-[var(--text-secondary)]">{k}: {v}</div>
                        ))}
                      </div>
                    )}
                    {deployInfo.resource_limits && (
                      <div className="bg-[var(--border-light)] rounded-lg p-3">
                        <span className="text-[10px] text-[var(--text-tertiary)] uppercase">Limits</span>
                        {Object.entries(deployInfo.resource_limits).map(([k, v]) => (
                          <div key={k} className="text-[11px] font-mono text-[var(--text-secondary)]">{k}: {v}</div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          ) : tab === 'logs' ? (
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-[12px] font-semibold text-[var(--text-secondary)]">Container Logs</h3>
                <button
                  onClick={loadLogs}
                  className="text-[11px] text-[var(--accent)] hover:underline"
                >
                  🔄 Refresh
                </button>
              </div>
              {loading ? (
                <p className="text-[13px] text-[var(--text-secondary)]">Loading logs...</p>
              ) : (
                <pre className="bg-[#1e1e2e] text-[#cdd6f4] rounded-lg p-4 text-[11px] font-mono overflow-auto max-h-[500px] whitespace-pre-wrap">
                  {logs || 'No logs available'}
                </pre>
              )}
            </div>
          ) : tab === 'events' ? (
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-[12px] font-semibold text-[var(--text-secondary)]">Kubernetes Events</h3>
                {!isDockerContainer && (
                  <button
                    onClick={loadEvents}
                    className="text-[11px] text-[var(--accent)] hover:underline"
                  >
                    🔄 Refresh
                  </button>
                )}
              </div>
              {isDockerContainer ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">Events are not available for Docker containers</p>
              ) : loading ? (
                <p className="text-[13px] text-[var(--text-secondary)]">Loading events...</p>
              ) : events.length === 0 ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">No events found</p>
              ) : (
                <div className="space-y-2">
                  {events.map((event, i) => (
                    <div key={i} className="flex gap-3 bg-[var(--border-light)] rounded-lg px-3 py-2">
                      <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium h-fit ${(event.type as string) === 'Normal' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-yellow-500/10 text-yellow-600'}`}>
                        {event.type as string}
                      </span>
                      <div className="flex-1 min-w-0">
                        <div className="text-[12px] font-medium text-[var(--text-primary)]">{event.reason as string}</div>
                        <div className="text-[11px] text-[var(--text-secondary)]">{event.message as string}</div>
                      </div>
                      <span className="text-[10px] text-[var(--text-tertiary)] whitespace-nowrap">
                        {(event.count as number) > 1 ? `${event.count}x ` : ''}{event.lastTimestamp ? new Date(event.lastTimestamp as string).toLocaleString() : ''}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : tab === 'edit' && deployInfo ? (
            <div className="space-y-5">
              {/* Image */}
              <div>
                <label className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-1 block">Container Image</label>
                <input
                  type="text"
                  value={editImage}
                  onChange={e => setEditImage(e.target.value)}
                  className="input text-[12px] font-mono w-full"
                  placeholder="e.g., nginx:1.25"
                />
              </div>

              {/* Replicas */}
              <div>
                <label className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-1 block">Replicas</label>
                <input
                  type="number"
                  value={editReplicas}
                  onChange={e => setEditReplicas(parseInt(e.target.value) || 0)}
                  min={0}
                  className="input text-[12px] w-32"
                />
              </div>

              {/* Environment Variables */}
              <div>
                <div className="flex items-center justify-between mb-2">
                  <label className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider">Environment Variables</label>
                  <button onClick={addEnvVar} className="text-[11px] text-[var(--accent)] hover:underline">+ Add</button>
                </div>
                <div className="space-y-2">
                  {editEnv.map((env, idx) => (
                    <div key={idx} className="flex gap-2 items-center">
                      <input
                        type="text"
                        value={env.key}
                        onChange={e => updateEnvVar(idx, 'key', e.target.value)}
                        placeholder="KEY"
                        className="input text-[11px] font-mono flex-1"
                      />
                      <input
                        type="text"
                        value={env.value}
                        onChange={e => updateEnvVar(idx, 'value', e.target.value)}
                        placeholder="value"
                        className="input text-[11px] font-mono flex-1"
                      />
                      <button onClick={() => removeEnvVar(idx)} className="text-red-500 hover:text-red-400 text-sm">&times;</button>
                    </div>
                  ))}
                </div>
              </div>

              {/* Save button */}
              <button
                onClick={handleUpdate}
                disabled={actionLoading === 'update'}
                className="btn btn-primary disabled:opacity-50"
              >
                {actionLoading === 'update' ? 'Saving...' : '💾 Save Changes'}
              </button>
            </div>
          ) : tab === 'edit' && isDockerContainer ? (
            <p className="text-[13px] text-[var(--text-tertiary)]">Editing is not available for Docker containers. Use Docker Compose to manage container configuration.</p>
          ) : null}
        </div>
      </div>

      {/* Delete Confirmation */}
      <ConfirmModal
        open={showDeleteConfirm}
        title={`Delete deployment ${service.namespace}/${service.name}?`}
        description="This deployment will be permanently removed from the cluster. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-[var(--border-light)] rounded-lg px-3 py-2">
      <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">{label}</div>
      <div className="text-[13px] font-semibold text-[var(--text-primary)]">{value}</div>
    </div>
  );
}
