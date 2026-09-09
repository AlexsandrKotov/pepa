'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { discovery, type DiscoveredService, type DeploymentInfo } from '@/lib/api';
import ConfirmModal from '@/components/ConfirmModal';
import Tabs from '@/components/Tabs';
import LogViewer from '@/components/LogViewer';

interface ServiceManagementPanelProps {
  service: DiscoveredService;
  onClose: () => void;
  onUpdate: () => void;
}

export default function ServiceManagementPanel({ service, onClose, onUpdate }: ServiceManagementPanelProps) {
  useEscapeKey(onClose);
  const [tab, setTab] = useState<'overview' | 'logs' | 'events'>('overview');
  const [editMode, setEditMode] = useState(false);
  const isDockerContainer = service.source === 'docker-container' || service.source === 'docker';
  // Sources that have no backing Kubernetes Deployment — skip the lookup entirely
  const hasNoK8sDeployment = isDockerContainer || service.source === 'pepa' || service.source === 'manual';
  const [deployInfo, setDeployInfo] = useState<DeploymentInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [logs, setLogs] = useState<string>('');
  const [events, setEvents] = useState<Array<Record<string, unknown>>>([]);
  const [eventsError, setEventsError] = useState<string>('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [logLines, setLogLines] = useState(200);
  const [collapsedSections, setCollapsedSections] = useState<Record<string, boolean>>({
    images: false,
    env: false,
    resources: false,
  });

  // Edit state
  const [editImage, setEditImage] = useState('');
  const [editReplicas, setEditReplicas] = useState(0);
  const [editEnv, setEditEnv] = useState<Array<{ key: string; value: string }>>([]);

  const isMountedRef = useRef(true);

  useEffect(() => {
    return () => { isMountedRef.current = false; };
  }, []);

  const loadDeployInfo = useCallback(async () => {
    if (hasNoK8sDeployment) return; // No K8s Deployment for docker/pepa/manual sources
    setLoading(true);
    try {
      const info = await discovery.k8sGet(service.cluster, service.namespace, service.name);
      if (!isMountedRef.current) return;
      setDeployInfo(info);
      setEditImage(info.image || '');
      setEditReplicas(info.replicas || 0);
      if (info.env) {
        setEditEnv(Object.entries(info.env).map(([key, value]) => ({ key, value })));
      }
    } catch (err) {
      console.error('Failed to load deployment info:', err);
    } finally {
      if (isMountedRef.current) setLoading(false);
    }
  }, [hasNoK8sDeployment, service.cluster, service.namespace, service.name]);

  useEffect(() => {
    loadDeployInfo();
  }, [loadDeployInfo]);

  const loadLogs = useCallback(async (lines?: number) => {
    setLoading(true);
    try {
      const count = lines ?? logLines;
      if (isDockerContainer) {
        const data = await discovery.dockerContainerLogs(service.cluster, service.name, count);
        if (isMountedRef.current) setLogs(data.logs || 'No logs available');
      } else if (hasNoK8sDeployment) {
        if (isMountedRef.current) setLogs('No logs available for this service');
      } else {
        const data = await discovery.k8sLogs(service.cluster, service.namespace, service.name, count);
        if (isMountedRef.current) setLogs(data.logs || 'No logs available');
      }
    } catch (err) {
      if (isMountedRef.current) setLogs(`Error loading logs: ${err}`);
    } finally {
      if (isMountedRef.current) setLoading(false);
    }
  }, [isDockerContainer, hasNoK8sDeployment, service.cluster, service.name, service.namespace, logLines]);

  const loadEvents = useCallback(async () => {
    if (hasNoK8sDeployment) return; // Not applicable for non-K8s services
    setLoading(true);
    setEventsError('');
    try {
      const data = await discovery.k8sEvents(service.cluster, service.namespace, service.name);
      if (isMountedRef.current) setEvents(data.events || []);
    } catch (err) {
      if (isMountedRef.current) setEventsError(err instanceof Error ? err.message : 'Failed to load events');
    } finally {
      if (isMountedRef.current) setLoading(false);
    }
  }, [hasNoK8sDeployment, service.cluster, service.namespace, service.name]);

  useEffect(() => {
    if (tab === 'logs') loadLogs();
    if (tab === 'events') loadEvents();
  }, [tab, loadLogs, loadEvents]);

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

  const toggleSection = (key: string) => setCollapsedSections(prev => ({ ...prev, [key]: !prev[key] }));

  const copyToClipboard = (text: string) => {
    if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      navigator.clipboard.writeText(text).catch(() => {});
    } else if (typeof document !== 'undefined') {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      try { document.execCommand('copy'); } finally { document.body.removeChild(ta); }
    }
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

  const healthColor = service.health === 'healthy' ? 'bg-green-500' : service.health === 'degraded' ? 'bg-yellow-500' : service.health === 'failed' ? 'bg-red-500' : service.health === 'suspended' ? 'bg-orange-500' : 'bg-gray-400';
  const isSuspended = service.health === 'suspended' || service.status === 'suspended';

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/30 z-[110] transition-opacity" onClick={onClose} />
      {/* Slide-out Panel */}
      <div className="fixed top-0 right-0 bottom-0 w-full max-w-2xl bg-[var(--surface)] border-l border-[var(--border)] z-[120] shadow-2xl flex flex-col animate-slide-in-right">
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
              {isSuspended && (
                <span className="badge-sm bg-orange-500/15 text-orange-600 border border-orange-500/20">⏸ suspended</span>
              )}
            </div>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">
              {service.cluster} / {service.namespace} &middot; <span className={isSuspended ? 'text-orange-500 font-medium' : ''}>{service.health}</span>
            </p>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            {!isDockerContainer && (
              <button
                onClick={() => { setEditMode(!editMode); if (!editMode) setTab('overview'); }}
                className={`p-1.5 rounded-lg transition-colors ${editMode ? 'text-[var(--accent)] bg-[var(--accent)]/10' : 'text-[var(--text-tertiary)] hover:text-[var(--accent)] hover:bg-[var(--border-light)]'}`}
                title={editMode ? 'Cancel editing' : 'Edit Service'}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L6.832 19.82a4.5 4.5 0 01-1.897 1.13l-2.685.8.8-2.685a4.5 4.5 0 011.13-1.897L16.863 4.487z" />
                </svg>
              </button>
            )}
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
        <div className="px-5 pt-2 shrink-0">
          <Tabs
            activeKey={tab}
            onChange={(k) => setTab(k as typeof tab)}
            size="sm"
            variant="rounded"
            tabs={[
              { key: 'overview', label: 'Overview', icon: 'discovery' },
              { key: 'logs', label: 'Logs', icon: 'document' },
              { key: 'events', label: 'Events', icon: 'lightning' },
            ]}
          />
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-5">
          {editMode && isDockerContainer ? (
            <p className="text-[13px] text-[var(--text-tertiary)]">Editing is not available for Docker containers. Use Docker Compose to manage container configuration.</p>
          ) : editMode && deployInfo ? (
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
                <button
                  onClick={() => toggleSection('images')}
                  className="flex items-center gap-1.5 text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 hover:text-[var(--text-primary)] transition-colors"
                >
                  <svg className={`w-3 h-3 transition-transform ${collapsedSections.images ? '-rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                  </svg>
                  Images
                  <span className="text-[10px] font-normal normal-case text-[var(--text-tertiary)] ml-1">{(deployInfo.images || [deployInfo.image]).length}</span>
                </button>
                {!collapsedSections.images && (
                  <div className="space-y-1">
                    {(deployInfo.images || [deployInfo.image]).map((img, i) => (
                      <div key={i} className="group flex items-center gap-2 text-[11px] font-mono bg-[var(--border-light)] px-3 py-1.5 rounded text-[var(--text-secondary)]">
                        <span className="truncate flex-1">{img}</span>
                        <button
                          onClick={() => copyToClipboard(img)}
                          className="opacity-0 group-hover:opacity-100 shrink-0 p-0.5 rounded hover:bg-[var(--border)] transition-all"
                          title="Copy image name"
                        >
                          <svg className="w-3 h-3 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                          </svg>
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Environment Variables */}
              {deployInfo.env && Object.keys(deployInfo.env).length > 0 && (
                <div>
                  <button
                    onClick={() => toggleSection('env')}
                    className="flex items-center gap-1.5 text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 hover:text-[var(--text-primary)] transition-colors"
                  >
                    <svg className={`w-3 h-3 transition-transform ${collapsedSections.env ? '-rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                    </svg>
                    Environment Variables
                    <span className="text-[10px] font-normal normal-case text-[var(--text-tertiary)] ml-1">{Object.keys(deployInfo.env).length}</span>
                  </button>
                  {!collapsedSections.env && (
                    <div className="bg-[var(--border-light)] rounded-lg overflow-hidden">
                      {Object.entries(deployInfo.env).map(([key, value]) => (
                        <div key={key} className="group flex border-b border-[var(--border)] last:border-b-0">
                          <span className="text-[11px] font-mono font-medium text-[var(--text-primary)] px-3 py-1.5 bg-[var(--surface)] border-r border-[var(--border)] min-w-[150px]">{key}</span>
                          <span className="text-[11px] font-mono text-[var(--text-secondary)] px-3 py-1.5 truncate flex-1">{value}</span>
                          <button
                            onClick={() => copyToClipboard(`${key}=${value}`)}
                            className="opacity-0 group-hover:opacity-100 shrink-0 px-2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-all"
                            title="Copy"
                          >
                            <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                              <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                            </svg>
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {/* Resources */}
              {(deployInfo.resource_limits || deployInfo.resource_requests) && (
                <div>
                  <button
                    onClick={() => toggleSection('resources')}
                    className="flex items-center gap-1.5 text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 hover:text-[var(--text-primary)] transition-colors"
                  >
                    <svg className={`w-3 h-3 transition-transform ${collapsedSections.resources ? '-rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                    </svg>
                    Resources
                  </button>
                  {!collapsedSections.resources && (
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
                  )}
                </div>
              )}
            </div>
          ) : tab === 'overview' ? (
            <div className="space-y-5">
              {/* Fallback: show discovery metadata when Deployment info is unavailable */}
              <div className="grid grid-cols-2 gap-3">
                <InfoCard label="Status" value={service.status || 'unknown'} />
                <InfoCard label="Health" value={service.health || 'unknown'} />
                <InfoCard label="Replicas" value={`${service.ready_replicas ?? 0}/${service.replicas ?? 0}`} />
                <InfoCard label="Source" value={service.source} />
              </div>
              {service.image && (
                <div>
                  <h3 className="text-[12px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2">Image</h3>
                  <div className="text-[11px] font-mono bg-[var(--border-light)] px-3 py-1.5 rounded text-[var(--text-secondary)] truncate">
                    {service.image}
                  </div>
                </div>
              )}
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
              {!isSuspended && (
                <div className="flex items-start gap-2 px-3 py-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20">
                  <span className="text-amber-600 text-[12px] shrink-0 mt-px">&#9888;</span>
                  <p className="text-[11px] text-amber-600 leading-relaxed">
                    Detailed deployment info is not available. This {service.source === 'fluxcd' ? 'HelmRelease' : 'service'} may not have a matching Kubernetes Deployment resource.
                  </p>
                </div>
              )}
            </div>
          ) : tab === 'logs' ? (
            <LogViewer
              logs={logs}
              loading={loading}
              onRefresh={() => loadLogs()}
              onLineCountChange={(n) => { setLogLines(n); loadLogs(n); }}
              currentLines={logLines}
              title="Container Logs"
            />
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
              ) : eventsError ? (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">
                  Failed to load events: {eventsError}
                </div>
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
