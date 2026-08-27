'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { clusters, Cluster, ClusterHealth, ClusterNode, K8sNamespace, K8sResource, FluxResource, ArgoResource } from '@/lib/api';
import { Toast } from '@/components/Interactive';

export default function ClusterDetailPage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const clusterId = searchParams.get('id') as string;
  const [cluster, setCluster] = useState<Cluster | null>(null);
  const [health, setHealth] = useState<ClusterHealth | null>(null);
  const [nodes, setNodes] = useState<ClusterNode[]>([]);
  const [namespaces, setNamespaces] = useState<K8sNamespace[]>([]);
  const [resources, setResources] = useState<K8sResource[]>([]);
  const [fluxResources, setFluxResources] = useState<FluxResource[]>([]);
  const [argoResources, setArgoResources] = useState<ArgoResource[]>([]);
  const [gitops, setGitops] = useState<{ fluxcd: boolean; argocd: boolean; flux_count: number; argo_count: number } | null>(null);
  const [selectedNs, setSelectedNs] = useState<string>('');
  const [tab, setTab] = useState<'overview' | 'nodes' | 'resources' | 'flux' | 'argocd' | 'kubeconfig'>('overview');
  const [loading, setLoading] = useState(true);
  const [tabLoading, setTabLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [loadErrors, setLoadErrors] = useState<Record<string, string>>({});
  const loadedTabs = useRef<Set<string>>(new Set(['overview']));
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Edit state
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [editDesc, setEditDesc] = useState('');
  const [editLabels, setEditLabels] = useState('');
  const [editNotes, setEditNotes] = useState('');

  // Test connection state
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null);
  const [testApiUrl, setTestApiUrl] = useState('');
  const [showTestPanel, setShowTestPanel] = useState(false);

  useEffect(() => {
    loadCluster();
  }, [clusterId]);

  // Auto-refresh interval
  useEffect(() => {
    if (autoRefresh && cluster?.has_kubeconfig) {
      intervalRef.current = setInterval(() => {
        refreshCurrentTab();
      }, 30000);
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [autoRefresh, tab, cluster?.has_kubeconfig]);

  const refreshCurrentTab = useCallback(async () => {
    if (!cluster?.has_kubeconfig) return;
    setTabLoading(true);
    try {
      if (tab === 'overview' || tab === 'nodes') await loadNodes();
      if (tab === 'overview' || tab === 'resources') await loadResources();
      if (tab === 'overview' || tab === 'flux') await loadFluxResources();
      if (tab === 'overview' || tab === 'argocd') await loadArgoResources();
      if (tab === 'overview') await loadNamespaces();
      clusters.health(clusterId).then(h => setHealth(h)).catch(() => {});
      setLastUpdated(new Date());
    } finally {
      setTabLoading(false);
    }
  }, [tab, clusterId, cluster?.has_kubeconfig]);

  const loadCluster = async () => {
    try {
      const data = await clusters.get(clusterId);
      setCluster(data);
      setEditName(data.name);
      setEditDesc(data.description || '');
      setEditLabels(Object.entries(data.labels || {}).map(([k, v]) => `${k}:${v}`).join(', '));
      setEditNotes(data.notes || '');
      setTestApiUrl(data.api_server_url || '');
      setLastUpdated(new Date());

      // Load health data
      try {
        const h = await clusters.health(clusterId);
        setHealth(h);
      } catch {
        setLoadErrors(prev => ({ ...prev, health: 'Failed to load health data' }));
      }

      // Load overview data immediately
      if (data.has_kubeconfig) {
        await loadNodes();
        await loadNamespaces();
        // Detect GitOps engines
        try {
          const g = await clusters.gitops(clusterId);
          setGitops(g);
        } catch { /* ignore */ }
      }
    } catch (err) {
      setToast({ message: 'Failed to load cluster', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  // Lazy-load tab data when tab changes
  useEffect(() => {
    if (!cluster?.has_kubeconfig || loading) return;
    if (loadedTabs.current.has(tab)) return;
    loadedTabs.current.add(tab);

    setTabLoading(true);
    (async () => {
      try {
        if (tab === 'nodes') await loadNodes();
        if (tab === 'resources') await loadResources();
        if (tab === 'flux') await loadFluxResources();
        if (tab === 'argocd') await loadArgoResources();
        if (tab === 'resources') await loadNamespaces();
      } finally {
        setTabLoading(false);
      }
    })();
  }, [tab, cluster?.has_kubeconfig, loading]);

  const loadNodes = async () => {
    try {
      const data = await clusters.nodes(clusterId);
      setNodes(data.nodes || []);
    } catch (err) {
      setLoadErrors(prev => ({ ...prev, nodes: 'Failed to load nodes' }));
    }
  };

  const loadNamespaces = async () => {
    try {
      const data = await clusters.namespaces(clusterId);
      setNamespaces(data.namespaces || []);
    } catch (err) {
      setLoadErrors(prev => ({ ...prev, namespaces: 'Failed to load namespaces' }));
    }
  };

  const loadResources = async (ns?: string) => {
    try {
      const data = await clusters.resources(clusterId, ns);
      setResources(data.resources || []);
    } catch (err) {
      setLoadErrors(prev => ({ ...prev, resources: 'Failed to load resources' }));
    }
  };

  const loadFluxResources = async () => {
    try {
      const data = await clusters.fluxResources(clusterId);
      setFluxResources(data.resources || []);
    } catch (err) {
      setLoadErrors(prev => ({ ...prev, flux: 'Failed to load FluxCD resources' }));
    }
  };

  const loadArgoResources = async () => {
    try {
      const data = await clusters.argoResources(clusterId);
      setArgoResources(data.resources || []);
    } catch (err) {
      setLoadErrors(prev => ({ ...prev, argocd: 'Failed to load ArgoCD resources' }));
    }
  };

  const handleNamespaceChange = (ns: string) => {
    setSelectedNs(ns);
    loadResources(ns || undefined);
  };

  const handleReconcile = async (namespace: string, name: string, kind: string) => {
    try {
      await clusters.reconcile(clusterId, namespace, name, kind);
      setToast({ message: `Reconcile triggered for ${kind}/${name}`, type: 'success' });
      loadFluxResources();
    } catch (err) {
      setToast({ message: 'Failed to trigger reconcile', type: 'error' });
    }
  };

  const handleSave = async () => {
    if (!cluster) return;
    const labels: Record<string, string> = {};
    editLabels.split(',').forEach(pair => {
      const [key, val] = pair.split(':').map(s => s.trim());
      if (key && val) labels[key] = val;
    });
    try {
      await clusters.update(clusterId, {
        ...cluster,
        name: editName,
        description: editDesc,
        labels,
        notes: editNotes,
      });
      setEditing(false);
      loadCluster();
      setToast({ message: 'Cluster updated', type: 'success' });
    } catch (err) {
      setToast({ message: 'Failed to update cluster', type: 'error' });
    }
  };

  const handleTestConnection = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await clusters.test(clusterId, testApiUrl || undefined);
      setTestResult({ status: result.status, message: result.message });
      // Reload cluster data after successful test
      if (result.status === 'connected') {
        loadCluster();
      }
    } catch (err) {
      setTestResult({ status: 'error', message: 'Failed to test connection' });
    } finally {
      setTesting(false);
    }
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Cluster Details</h1>
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading cluster...</p>
        </div>
      </div></div>
    );
  }

  if (!cluster) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Cluster Not Found</h1>
        <Link href="/clusters" className="text-[var(--accent)] hover:underline">{'\u2190'} Back to Clusters</Link>
      </div></div>
    );
  }

  const statusDot: Record<string, string> = {
    connected: 'bg-green-500',
    disconnected: 'bg-red-500',
    syncing: 'bg-yellow-500',
    pending: 'bg-gray-400',
    Ready: 'bg-green-500',
    Failed: 'bg-red-500',
    Reconciling: 'bg-yellow-500',
    Active: 'bg-green-500',
  };

  const statusBadge: Record<string, string> = {
    connected: 'badge-success',
    disconnected: 'badge-danger',
    syncing: 'badge-warning',
    pending: 'badge-default',
    healthy: 'badge-success',
    degraded: 'badge-warning',
    unhealthy: 'badge-danger',
  };

  const kindColors: Record<string, string> = {
    Deployment: 'text-blue-600',
    Service: 'text-green-600',
    ConfigMap: 'text-yellow-600',
    Secret: 'text-red-600',
    HelmRelease: 'text-purple-600',
    Kustomization: 'text-cyan-600',
  };

  const tabs = [
    { id: 'overview' as const, label: 'Overview' },
    ...(cluster.has_kubeconfig ? [
      { id: 'nodes' as const, label: 'Nodes' },
      { id: 'resources' as const, label: 'Resources' },
      ...(gitops?.fluxcd ? [{ id: 'flux' as const, label: `FluxCD (${gitops.flux_count})` }] : []),
      ...(gitops?.argocd ? [{ id: 'argocd' as const, label: `ArgoCD (${gitops.argo_count})` }] : []),
      { id: 'kubeconfig' as const, label: 'Kubeconfig' },
    ] : []),
  ];

  const parsePercent = (s: string) => parseInt(s.replace('%', '')) || 0;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Breadcrumb */}
      <div className="page-animate flex items-center gap-2 text-[13px] text-[var(--text-tertiary)]">
        <Link href="/clusters" className="hover:text-[var(--text-secondary)]">Clusters</Link>
        <span>/</span>
        <span className="text-[var(--text-secondary)]">{cluster.name}</span>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className={`w-3 h-3 rounded-full ${statusDot[cluster.status] || 'bg-gray-400'}`} />
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title">{cluster.name}</h1>
              <span className={`badge ${statusBadge[cluster.status] || 'badge-default'}`}>
                {cluster.status}
              </span>
            </div>
            <p className="page-subtitle">
              {cluster.environment} &middot; {cluster.api_server_url || 'No API server'} &middot; {cluster.kubernetes_version || 'N/A'}
            </p>
          </div>
        </div>
        {!editing ? (
          <div className="flex gap-2">
            <button onClick={() => setShowTestPanel(!showTestPanel)} className="btn btn-secondary btn-sm">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
              Test
            </button>
            <button onClick={() => setEditing(true)} className="btn btn-secondary btn-sm">Edit</button>
          </div>
        ) : (
          <div className="flex gap-2">
            <button onClick={() => setEditing(false)} className="btn btn-ghost btn-sm">Cancel</button>
            <button onClick={handleSave} className="btn btn-primary btn-sm">Save</button>
          </div>
        )}
      </div>

      {/* Test Connection Panel */}
      {showTestPanel && (
        <div className="card">
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Test Connection</span>
            <button onClick={() => setShowTestPanel(false)} className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div className="card-body space-y-3">
            <div>
              <label className="label">API Server URL</label>
              <input
                type="text"
                value={testApiUrl}
                onChange={(e) => setTestApiUrl(e.target.value)}
                className="input"
                placeholder="https://k8s-api.example.com:6443"
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                Override the API server URL from kubeconfig. Use this if the cluster IP in kubeconfig is not reachable from PEPA.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={handleTestConnection}
                disabled={testing}
                className="btn btn-primary btn-sm disabled:opacity-50"
              >
                {testing ? 'Testing...' : 'Test Connection'}
              </button>
              {testResult && (
                <span className={`text-[12px] font-medium ${testResult.status === 'connected' ? 'text-green-600' : 'text-red-600'}`}>
                  {testResult.status === 'connected' ? 'Connected' : 'Failed'}
                </span>
              )}
            </div>
            {testResult && (
              <div className={`p-3 rounded-lg text-[12px] ${testResult.status === 'connected' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-600' : 'bg-red-500/10 border border-red-500/20 text-red-500'}`}>
                {testResult.message}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Health & Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {health ? (
          <>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Health</p>
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${health.status === 'healthy' ? 'bg-green-500' : health.status === 'degraded' ? 'bg-yellow-500' : 'bg-red-500'}`} />
                <span className="text-[14px] font-medium text-[var(--text-primary)] capitalize">{health.status}</span>
              </div>
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">CPU Usage</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{health.cpu_usage}</p>
              {health.cpu_usage !== 'N/A' && (
                <div className="progress-bar mt-2">
                  <div className="progress-bar-fill bg-blue-500" style={{ width: health.cpu_usage }} />
                </div>
              )}
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Memory Usage</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{health.memory_usage}</p>
              {health.memory_usage !== 'N/A' && (
                <div className="progress-bar mt-2">
                  <div className="progress-bar-fill bg-green-500" style={{ width: health.memory_usage }} />
                </div>
              )}
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Pod Usage</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{health.pod_usage}</p>
              {health.pod_usage !== 'N/A' && (
                <div className="progress-bar mt-2">
                  <div className="progress-bar-fill bg-purple-500" style={{ width: health.pod_usage }} />
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Namespaces</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{namespaces.length}</p>
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Resources</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{resources.length}</p>
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">FluxCD</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{fluxResources.length}</p>
            </div>
            <div className="card card-body">
              <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Nodes</p>
              <p className="text-[14px] font-medium text-[var(--text-primary)]">{cluster.node_count}</p>
            </div>
          </>
        )}
      </div>

      {/* Auto-refresh & Last Updated */}
      {cluster.has_kubeconfig && (
        <div className="flex items-center justify-between text-[12px] text-[var(--text-tertiary)]">
          <div className="flex items-center gap-3">
            {lastUpdated && (
              <span>Last updated: {Math.max(0, Math.round((Date.now() - lastUpdated.getTime()) / 1000))}s ago</span>
            )}
            {tabLoading && <span className="text-[var(--accent)]">Refreshing...</span>}
          </div>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              className="w-3.5 h-3.5 accent-[var(--accent)]"
            />
            <span>Auto-refresh (30s)</span>
          </label>
        </div>
      )}

      {/* Component Checks */}
      {health && health.checks && (
        <div className="card">
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Component Health</span>
          </div>
          <div className="card-body">
            <div className="flex flex-wrap gap-4">
              {Object.entries(health.checks).map(([key, ok]) => (
                <div key={key} className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${ok ? 'bg-green-500' : 'bg-red-500'}`} />
                  <span className="text-[12px] text-[var(--text-secondary)] capitalize">{key.replace('_', ' ')}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 border-b border-[var(--border)]">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-4 py-2 text-[12px] font-medium border-b-2 transition-colors outline-none ${
              tab === t.id
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Overview Tab */}
      {tab === 'overview' && (
        <div className="space-y-4">
          <div className="card">
            <div className="card-header">
              <span className="text-[13px] font-medium text-[var(--text-primary)]">Cluster Information</span>
            </div>
            <div className="card-body space-y-4">
              {/* Name */}
              <div>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Name</p>
                {editing ? (
                  <input type="text" value={editName} onChange={(e) => setEditName(e.target.value)} className="input" />
                ) : (
                  <p className="text-[13px] text-[var(--text-primary)] font-medium">{cluster.name}</p>
                )}
              </div>

              {/* Description */}
              <div>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Description</p>
                {editing ? (
                  <input type="text" value={editDesc} onChange={(e) => setEditDesc(e.target.value)} className="input" placeholder="Cluster description..." />
                ) : (
                  <p className="text-[13px] text-[var(--text-secondary)]">{cluster.description || 'No description'}</p>
                )}
              </div>

              {/* Environment & Status */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Environment</p>
                  <span className={`text-[11px] px-2 py-0.5 rounded border ${
                    cluster.environment === 'production' ? 'bg-red-500/15 text-red-500 border-red-500/20' :
                    cluster.environment === 'staging' ? 'bg-amber-500/15 text-amber-600 border-amber-500/20' :
                    'bg-emerald-500/15 text-emerald-600 border-emerald-500/20'
                  }`}>
                    {cluster.environment}
                  </span>
                </div>
                <div>
                  <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Status</p>
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${statusDot[cluster.status] || 'bg-gray-400'}`} />
                    <span className="text-[13px] text-[var(--text-primary)] capitalize">{cluster.status}</span>
                  </div>
                </div>
              </div>

              {/* Labels */}
              <div>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Labels</p>
                {editing ? (
                  <input type="text" value={editLabels} onChange={(e) => setEditLabels(e.target.value)} className="input" placeholder="region:eu-west, team:platform" />
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {Object.entries(cluster.labels || {}).length > 0 ? (
                      Object.entries(cluster.labels || {}).map(([key, val]) => (
                        <span key={key} className="text-[11px] px-2 py-0.5 rounded bg-[var(--border-light)] border border-[var(--border)] text-[var(--text-secondary)]">
                          {key}<span className="text-[var(--text-tertiary)]">:</span>{val}
                        </span>
                      ))
                    ) : (
                      <span className="text-[13px] text-[var(--text-tertiary)]">No labels</span>
                    )}
                  </div>
                )}
              </div>

              {/* Notes */}
              <div>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Notes</p>
                {editing ? (
                  <textarea value={editNotes} onChange={(e) => setEditNotes(e.target.value)} rows={3} className="input resize-none" placeholder="Internal notes..." />
                ) : (
                  <p className="text-[13px] text-[var(--text-secondary)] whitespace-pre-wrap">{cluster.notes || 'No notes'}</p>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Nodes Tab */}
      {tab === 'nodes' && cluster.has_kubeconfig && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Cluster Nodes</h3>
            <button onClick={() => clusters.nodes(clusterId).then(n => setNodes(n.nodes || []))} className="btn btn-secondary btn-sm">
              Refresh
            </button>
          </div>
          {nodes.length > 0 ? (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {nodes.map((node) => (
                <div key={node.name} className="card">
                  <div className="card-header">
                    <div className="flex items-center gap-2">
                      <div className={`w-2 h-2 rounded-full ${node.status === 'Ready' ? 'bg-green-500' : 'bg-red-500'}`} />
                      <span className="text-[13px] font-medium text-[var(--text-primary)]">{node.name}</span>
                    </div>
                    <span className={`badge ${node.status === 'Ready' ? 'badge-success' : 'badge-danger'}`}>
                      {node.status}
                    </span>
                  </div>
                  <div className="card-body space-y-3">
                    <div className="flex flex-wrap gap-2">
                      <span className="text-[11px] px-2 py-0.5 rounded bg-blue-500/10 text-blue-500 border border-blue-500/20">
                        {node.roles}
                      </span>
                      <span className="text-[11px] px-2 py-0.5 rounded bg-[var(--border-light)] text-[var(--text-secondary)] border border-[var(--border)]">
                        {node.kubernetes_version}
                      </span>
                    </div>
                    <div className="space-y-2">
                      <div>
                        <div className="flex justify-between text-[11px] mb-1">
                          <span className="text-[var(--text-tertiary)]">CPU</span>
                          <span className="text-[var(--text-secondary)]">{node.cpu_usage} ({node.cpu_capacity} cores)</span>
                        </div>
                        <div className="progress-bar">
                          <div className="progress-bar-fill bg-blue-500" style={{ width: node.cpu_usage }} />
                        </div>
                      </div>
                      <div>
                        <div className="flex justify-between text-[11px] mb-1">
                          <span className="text-[var(--text-tertiary)]">Memory</span>
                          <span className="text-[var(--text-secondary)]">{node.memory_usage} ({node.memory_capacity})</span>
                        </div>
                        <div className="progress-bar">
                          <div className="progress-bar-fill bg-green-500" style={{ width: node.memory_usage }} />
                        </div>
                      </div>
                    </div>
                    <div className="text-[11px] text-[var(--text-tertiary)]">
                      {node.os_image} &middot; {node.container_runtime}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="card empty-state">
              <h3>No nodes available</h3>
              <p>Upload a kubeconfig to view cluster nodes</p>
            </div>
          )}
        </div>
      )}

      {/* Resources Tab */}
      {tab === 'resources' && cluster.has_kubeconfig && (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <select
              value={selectedNs}
              onChange={(e) => handleNamespaceChange(e.target.value)}
              className="select"
              style={{ width: 'auto', minWidth: 160 }}
            >
              <option value="">All Namespaces</option>
              {namespaces.map(ns => (
                <option key={ns.name} value={ns.name}>{ns.name}</option>
              ))}
            </select>
            <button onClick={() => loadResources(selectedNs || undefined)} className="btn btn-secondary btn-sm">
              Refresh
            </button>
          </div>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>Name</th>
                  <th>Namespace</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {resources.map((r, i) => (
                  <tr key={i}>
                    <td className={`font-mono text-[12px] ${kindColors[r.kind] || 'text-[var(--text-secondary)]'}`}>{r.kind}</td>
                    <td className="text-[var(--text-primary)] font-medium">{r.name}</td>
                    <td className="text-[var(--text-tertiary)]">{r.namespace}</td>
                    <td>
                      <span className="flex items-center gap-1.5">
                        <span className={`w-1.5 h-1.5 rounded-full ${statusDot[r.status] || 'bg-gray-400'}`} />
                        <span className="text-[12px] text-[var(--text-secondary)]">{r.status}</span>
                      </span>
                    </td>
                  </tr>
                ))}
                {resources.length === 0 && (
                  <tr><td colSpan={4} className="text-center py-8 text-[var(--text-tertiary)]">No resources found</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* FluxCD Tab */}
      {tab === 'flux' && cluster.has_kubeconfig && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-medium text-[var(--text-primary)]">FluxCD Resources</h3>
            <button onClick={loadFluxResources} className="btn btn-secondary btn-sm">Refresh</button>
          </div>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>Name</th>
                  <th>Namespace</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {fluxResources.map((r, i) => (
                  <tr key={`${r.kind}/${r.namespace}/${r.name}` || i}>
                    <td className={`font-mono text-[12px] ${kindColors[r.kind] || 'text-[var(--text-secondary)]'}`}>{r.kind}</td>
                    <td className="text-[var(--text-primary)] font-medium">{r.name}</td>
                    <td className="text-[var(--text-tertiary)]">{r.namespace}</td>
                    <td>
                      <span className="flex items-center gap-1.5">
                        <span className={`w-1.5 h-1.5 rounded-full ${statusDot[r.status] || 'bg-gray-400'}`} />
                        <span className="text-[12px] text-[var(--text-secondary)]">{r.status}</span>
                      </span>
                    </td>
                    <td>
                      <button
                        onClick={() => handleReconcile(r.namespace, r.name, r.kind)}
                        className="text-[12px] text-[var(--accent)] hover:underline"
                      >
                        Reconcile
                      </button>
                    </td>
                  </tr>
                ))}
                {fluxResources.length === 0 && (
                  <tr><td colSpan={5} className="text-center py-8 text-[var(--text-tertiary)]">No FluxCD resources found</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Kubeconfig Tab */}
      {tab === 'argocd' && cluster.has_kubeconfig && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-medium text-[var(--text-primary)]">ArgoCD Resources</h3>
            <button onClick={loadArgoResources} className="btn btn-secondary btn-sm">Refresh</button>
          </div>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>Name</th>
                  <th>Namespace</th>
                  <th>Health</th>
                  <th>Sync</th>
                  <th>Destination</th>
                  <th>Target Rev</th>
                </tr>
              </thead>
              <tbody>
                {argoResources.map((r, i) => {
                  const healthColor: Record<string, string> = {
                    Healthy: 'text-emerald-600',
                    healthy: 'text-emerald-600',
                    Degraded: 'text-red-500',
                    degraded: 'text-red-500',
                    Progressing: 'text-yellow-600',
                    progressing: 'text-yellow-600',
                    Suspended: 'text-gray-500',
                    suspended: 'text-gray-500',
                    Missing: 'text-orange-500',
                    missing: 'text-orange-500',
                    Unknown: 'text-[var(--text-tertiary)]',
                    unknown: 'text-[var(--text-tertiary)]',
                  };
                  const syncColor: Record<string, string> = {
                    Synced: 'text-emerald-600',
                    synced: 'text-emerald-600',
                    OutOfSync: 'text-yellow-600',
                    out_of_sync: 'text-yellow-600',
                    Unknown: 'text-[var(--text-tertiary)]',
                    unknown: 'text-[var(--text-tertiary)]',
                  };
                  return (
                    <tr key={`${r.kind}/${r.namespace}/${r.name}` || i}>
                      <td className={`font-mono text-[12px] ${r.kind === 'Application' ? 'text-blue-600' : 'text-purple-600'}`}>{r.kind}</td>
                      <td className="text-[var(--text-primary)] font-medium">{r.name}</td>
                      <td className="text-[var(--text-tertiary)]">{r.namespace}</td>
                      <td>
                        <span className={`text-[12px] font-medium ${healthColor[r.health || ''] || 'text-[var(--text-tertiary)]'}`}>
                          {r.health || 'Unknown'}
                        </span>
                      </td>
                      <td>
                        <span className={`text-[12px] font-medium ${syncColor[r.sync_status || ''] || 'text-[var(--text-tertiary)]'}`}>
                          {r.sync_status || 'Unknown'}
                        </span>
                      </td>
                      <td className="text-[var(--text-tertiary)] text-[12px]">{r.destination || '-'}</td>
                      <td className="text-[var(--text-tertiary)] text-[12px] font-mono">{r.target_revision || '-'}</td>
                    </tr>
                  );
                })}
                {argoResources.length === 0 && (
                  <tr><td colSpan={7} className="text-center py-8 text-[var(--text-tertiary)]">No ArgoCD resources found</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ArgoCD Tab */}
      {tab === 'argocd' && cluster.has_kubeconfig && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-[13px] font-medium text-[var(--text-primary)]">ArgoCD Resources</h3>
            <button onClick={loadArgoResources} className="btn btn-secondary btn-sm">Refresh</button>
          </div>
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>Name</th>
                  <th>Namespace</th>
                  <th>Health</th>
                  <th>Sync</th>
                  <th>Destination</th>
                  <th>Target Rev</th>
                </tr>
              </thead>
              <tbody>
                {argoResources.map((r, i) => {
                  const healthColor: Record<string, string> = {
                    Healthy: 'text-emerald-600', Degraded: 'text-red-500',
                    Progressing: 'text-yellow-600', Suspended: 'text-gray-500',
                    Missing: 'text-orange-500', Unknown: 'text-[var(--text-tertiary)]',
                  };
                  const syncColor: Record<string, string> = {
                    Synced: 'text-emerald-600', OutOfSync: 'text-yellow-600',
                    Unknown: 'text-[var(--text-tertiary)]',
                  };
                  return (
                    <tr key={`${r.kind}/${r.namespace}/${r.name}` || i}>
                      <td className={`font-mono text-[12px] ${r.kind === 'Application' ? 'text-blue-600' : 'text-purple-600'}`}>{r.kind}</td>
                      <td className="text-[var(--text-primary)] font-medium">{r.name}</td>
                      <td className="text-[var(--text-tertiary)]">{r.namespace}</td>
                      <td>
                        <span className={`text-[12px] font-medium ${healthColor[r.health || 'Unknown'] || 'text-[var(--text-tertiary)]'}`}>
                          {r.health || 'Unknown'}
                        </span>
                      </td>
                      <td>
                        <span className={`text-[12px] font-medium ${syncColor[r.sync_status || 'Unknown'] || 'text-[var(--text-tertiary)]'}`}>
                          {r.sync_status || 'Unknown'}
                        </span>
                      </td>
                      <td className="text-[var(--text-tertiary)] text-[12px]">{r.destination || '-'}</td>
                      <td className="text-[var(--text-tertiary)] text-[12px] font-mono">{r.target_revision || '-'}</td>
                    </tr>
                  );
                })}
                {argoResources.length === 0 && (
                  <tr><td colSpan={7} className="text-center py-8 text-[var(--text-tertiary)]">No ArgoCD resources found</td></tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Kubeconfig Tab */}
      {tab === 'kubeconfig' && cluster.has_kubeconfig && (
        <div className="card">
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Kubeconfig</span>
            <span className="badge badge-success">Connected</span>
          </div>
          <div className="card-body">
            <div className="bg-[var(--border-light)] rounded-lg p-4 font-mono text-[12px] text-[var(--text-secondary)] max-h-64 overflow-auto">
              <pre>{'apiVersion: v1\nkind: Config\n# Kubeconfig is stored securely (encrypted at rest)\n# Use kubectl with this cluster to manage resources'}</pre>
            </div>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-3">
              Kubeconfig is encrypted and stored securely. The actual content is not displayed for security reasons.
            </p>
          </div>
        </div>
      )}

      {/* No kubeconfig message */}
      {!cluster.has_kubeconfig && tab !== 'overview' && (
        <div className="card empty-state">
          <svg className="w-12 h-12 mx-auto text-[var(--text-tertiary)] mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
          <h3>No kubeconfig uploaded</h3>
          <p>Upload a kubeconfig to view namespaces and resources</p>
        </div>
      )}
      </div>
    </div>
  );
}
