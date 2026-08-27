'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useSearchParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { services, discovery, clusters as clustersApi, environments as environmentsApi, type Service, type ServiceDeployment, type Cluster, type Environment } from '@/lib/api';

export default function ServiceDetailPage() {
  const searchParams = useSearchParams();
  const serviceId = searchParams.get('id') as string;

  const [service, setService] = useState<Service | null>(null);
  const [deployments, setDeployments] = useState<ServiceDeployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [mgmtTab, setMgmtTab] = useState<'scale' | 'image' | 'logs' | null>(null);
  const [mgmtLoading, setMgmtLoading] = useState(false);
  const [mgmtResult, setMgmtResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [actionFeedback, setActionFeedback] = useState<{ ok: boolean; text: string } | null>(null);
  const [scaleReplicas, setScaleReplicas] = useState(1);
  const [newImage, setNewImage] = useState('');
  const [logs, setLogs] = useState('');
  const [envList, setEnvList] = useState<Environment[]>([]);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  useEscapeKey(() => {
    if (showDeleteConfirm && !deleting) setShowDeleteConfirm(false);
  }, showDeleteConfirm);
  const [deleting, setDeleting] = useState(false);
  const router = useRouter();

  const loadData = useCallback(async () => {
    try {
      const data = await services.get(serviceId);
      setService(data.service);
      setDeployments(data.deployments || []);
      
      // Load clusters for management actions
      const [clData, envData] = await Promise.all([
        clustersApi.list(),
        environmentsApi.list().catch(() => ({ environments: [], total: 0 })),
      ]);
      setClusterList(clData.clusters || []);
      setEnvList(envData.environments || []);
    } catch (err) {
      console.error('Failed to load service:', err);
    } finally {
      setLoading(false);
    }
  }, [serviceId]);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [loadData]);

  const handleDeploy = async (environment: string) => {
    setActionFeedback(null);
    try {
      const clusterId = service?.target_clusters?.[0] || '';
      await services.deploy(serviceId, { environment, cluster_id: clusterId, deploy_type: environment === 'dev' ? 'manual' : 'automatic' });
      await loadData();
      setActionFeedback({ ok: true, text: `Deploy to ${environment} initiated` });
    } catch (err) {
      setActionFeedback({ ok: false, text: 'Deploy failed: ' + (err as Error).message });
    }
  };

  const handleVerify = async (deploymentId: string) => {
    setActionFeedback(null);
    try {
      await services.verify(serviceId, deploymentId);
      await loadData();
      setActionFeedback({ ok: true, text: 'Verification initiated' });
    } catch (err) {
      setActionFeedback({ ok: false, text: 'Verify failed: ' + (err as Error).message });
    }
  };

  const handlePromote = async (deploymentId: string) => {
    setActionFeedback(null);
    try {
      await services.promote(serviceId, deploymentId);
      await loadData();
      setActionFeedback({ ok: true, text: 'Promotion initiated' });
    } catch (err) {
      setActionFeedback({ ok: false, text: 'Promote failed: ' + (err as Error).message });
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    setActionFeedback(null);
    try {
      await services.delete(serviceId);
      router.push('/services');
    } catch (err) {
      console.error('Delete failed:', err);
      setActionFeedback({ ok: false, text: 'Failed to delete service: ' + (err as Error).message });
    } finally {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  };

  // Management helpers
  const getClusterName = () => {
    if (!service?.target_clusters?.length || !clusterList.length) return '';
    const c = clusterList.find(cl => cl.id === service.target_clusters[0]);
    return c?.name || '';
  };

  const handleScale = async () => {
    const cluster = getClusterName();
    if (!cluster || !service) return;
    setMgmtLoading(true);
    setMgmtResult(null);
    try {
      const res = await discovery.k8sScale(cluster, service.namespace, service.slug, scaleReplicas);
      setMgmtResult({ ok: true, text: res.message });
    } catch (err) {
      setMgmtResult({ ok: false, text: (err as Error).message });
    } finally {
      setMgmtLoading(false);
    }
  };

  const handleUpdateImage = async () => {
    const cluster = getClusterName();
    if (!cluster || !service || !newImage) return;
    setMgmtLoading(true);
    setMgmtResult(null);
    try {
      const res = await discovery.k8sUpdate(cluster, service.namespace, service.slug, { image: newImage });
      setMgmtResult({ ok: true, text: res.message });
    } catch (err) {
      setMgmtResult({ ok: false, text: (err as Error).message });
    } finally {
      setMgmtLoading(false);
    }
  };

  const handleRestart = async () => {
    const cluster = getClusterName();
    if (!cluster || !service) return;
    setMgmtLoading(true);
    setMgmtResult(null);
    try {
      const res = await discovery.k8sRestart(cluster, service.namespace, service.slug);
      setMgmtResult({ ok: true, text: res.message });
    } catch (err) {
      setMgmtResult({ ok: false, text: (err as Error).message });
    } finally {
      setMgmtLoading(false);
    }
  };

  const handleViewLogs = async () => {
    const cluster = getClusterName();
    if (!cluster || !service) return;
    setMgmtLoading(true);
    try {
      const res = await discovery.k8sLogs(cluster, service.namespace, service.slug, 200);
      setLogs(res.logs || 'No logs available');
      setMgmtTab('logs');
    } catch (err) {
      setMgmtResult({ ok: false, text: (err as Error).message });
    } finally {
      setMgmtLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Service Details</h1>
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      </div></div>
    );
  }

  if (!service) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Service Not Found</h1>
        <Link href="/services" className="text-[var(--accent)] hover:underline">← Back to Services</Link>
      </div></div>
    );
  }

  const environments = envList.length > 0
    ? envList.map(e => e.slug || e.name.toLowerCase().replace(/\s+/g, '-'))
    : ['dev', 'testing', 'staging', 'production'];
  const getDeployment = (env: string) => deployments.find(d => d.environment === env);

  const statusIcon = (status: string) => {
    switch (status) {
      case 'deployed': return '✓';
      case 'deploying': return '⟳';
      case 'promoted': return '→';
      case 'failed': return '✕';
      case 'pending': return '○';
      default: return '·';
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'deployed': return 'bg-green-500';
      case 'deploying': return 'bg-yellow-500 animate-pulse';
      case 'promoted': return 'bg-blue-500';
      case 'failed': return 'bg-red-500';
      case 'pending': return 'bg-gray-300';
      default: return 'bg-gray-200';
    }
  };

  const verificationBadge = (vStatus: string) => {
    switch (vStatus) {
      case 'verified': return <span className="badge badge-success">Verified</span>;
      case 'pending': return <span className="badge badge-default">Pending</span>;
      case 'failed': return <span className="badge badge-danger">Failed</span>;
      default: return <span className="badge badge-default">{vStatus}</span>;
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="page-title-modern">{service.name}</h1>
            <span className={`badge ${
              service.status === 'active' ? 'badge-success' :
              service.status === 'deploying' ? 'badge-warning' :
              service.status === 'configured' ? 'bg-blue-500/15 text-blue-500' :
              service.status === 'failed' ? 'badge-danger' :
              'badge-default'
            }`}>
              {service.status}
            </span>
          </div>
          <p className="page-subtitle">{service.description || 'No description'}</p>
        </div>
        <div className="flex items-center gap-3">
          <Link href="/services" className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            ← Back to Services
          </Link>
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="text-[12px] px-3 py-1 rounded border border-red-500/20 text-red-500 hover:bg-red-500/10 transition-colors"
          >
            Delete
          </button>
        </div>
      </div>

      {/* Action Feedback */}
      {actionFeedback && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 page-animate-up ${
          actionFeedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <div>
            <p className={`text-sm font-medium ${actionFeedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {actionFeedback.ok ? '✓ ' : '⚠ '}{actionFeedback.text}
            </p>
          </div>
          <button onClick={() => setActionFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      {/* Service Info */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Template</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{service.language || '-'}</p>
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Namespace</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{service.namespace}</p>
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Strategy</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{service.deployment_strategy}</p>
        </div>
        <div className="card card-body">
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Created</p>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">{new Date(service.created_at).toLocaleDateString()}</p>
        </div>
      </div>

      {/* Deployment Pipeline */}
      <div className="card">
        <div className="card-header">
          <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Deployment Pipeline</h2>
          <span className="text-[11px] text-[var(--text-tertiary)]">{environments.join(' → ')}</span>
        </div>
        <div className="card-body">
          {/* Pipeline visualization */}
          <div className="flex items-center gap-2 mb-6">
            {environments.map((env, i) => {
              const dep = getDeployment(env);
              const isActive = dep && dep.status !== 'pending';
              return (
                <div key={env} className="flex items-center gap-2 flex-1">
                  <div className={`flex-1 p-3 rounded-lg border text-center transition-all ${
                    isActive ? 'border-emerald-500/20 bg-emerald-500/10' : 'border-[var(--border)] bg-[var(--bg)]'
                  }`}>
                    <div className={`w-6 h-6 rounded-full mx-auto mb-1 flex items-center justify-center text-white text-[11px] ${statusColor(dep?.status || 'pending')}`}>
                      {statusIcon(dep?.status || 'pending')}
                    </div>
                    <p className="text-[11px] font-medium text-[var(--text-primary)] capitalize">{env}</p>
                    {dep && (
                      <div className="mt-1">
                        <p className="text-[10px] text-[var(--text-tertiary)]">{dep.deploy_type}</p>
                        {dep.pods_total > 0 && (
                          <p className="text-[10px] text-[var(--text-secondary)]">
                            {dep.pods_ready}/{dep.pods_total} pods
                          </p>
                        )}
                      </div>
                    )}
                  </div>
                  {i < environments.length - 1 && (
                    <div className={`w-4 h-0.5 ${dep?.status === 'promoted' ? 'bg-green-400' : 'bg-[var(--border)]'}`} />
                  )}
                </div>
              );
            })}
          </div>

          {/* Environment details */}
          <div className="space-y-3">
            {environments.map(env => {
              const dep = getDeployment(env);
              return (
                <div key={env} className="flex items-center justify-between p-3 rounded-lg border border-[var(--border-light)]">
                  <div className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full ${statusColor(dep?.status || 'pending')}`} />
                    <div>
                      <p className="text-[12px] font-medium text-[var(--text-primary)] capitalize">{env}</p>
                      {dep && (
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="text-[10px] text-[var(--text-tertiary)]">{dep.branch || 'N/A'}</span>
                          {dep.image_tag && <span className="text-[10px] text-[var(--text-tertiary)]">:{dep.image_tag}</span>}
                          {dep.flux_synced && <span className="text-[10px] text-green-600">FluxCD ✓</span>}
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    {dep && verificationBadge(dep.verification_status)}
                    {!dep || dep.status === 'pending' ? (
                      <button
                        onClick={() => handleDeploy(env)}
                        className="text-[11px] px-3 py-1 bg-[var(--accent)] text-white rounded hover:bg-blue-700"
                      >
                        Deploy
                      </button>
                    ) : dep.status === 'deployed' && dep.verification_status !== 'verified' ? (
                      <div className="flex gap-1">
                        <button
                          onClick={() => handleVerify(dep.id)}
                          className="text-[11px] px-2 py-1 bg-green-600 text-white rounded hover:bg-green-700"
                        >
                          Verify
                        </button>
                        <button
                          onClick={() => handlePromote(dep.id)}
                          className="text-[11px] px-2 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
                        >
                          Promote →
                        </button>
                      </div>
                    ) : dep.status === 'deployed' ? (
                      <button
                        onClick={() => handlePromote(dep.id)}
                        className="text-[11px] px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
                      >
                        Promote →
                      </button>
                    ) : dep.status === 'deploying' ? (
                      <span className="text-[11px] text-yellow-600">Deploying...</span>
                    ) : (
                      <span className="text-[11px] text-[var(--text-tertiary)]">{dep.status}</span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* Management Actions */}
      {service.status === 'active' && getClusterName() && (
        <div className="card">
          <div className="card-header">
            <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Management</h2>
            <span className="text-[11px] text-[var(--text-tertiary)]">Cluster: {getClusterName()}</span>
          </div>
          <div className="card-body">
            {mgmtResult && (
              <div className={`mb-4 p-3 rounded-lg border text-[12px] ${mgmtResult.ok ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600' : 'bg-red-500/10 border-red-500/20 text-red-500'}`}>
                {mgmtResult.ok ? '✓' : '⚠'} {mgmtResult.text}
                <button onClick={() => setMgmtResult(null)} className="ml-2 text-[11px] underline">dismiss</button>
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {/* Scale */}
              <div className="p-3 border border-[var(--border-light)] rounded-lg">
                <p className="text-[11px] text-[var(--text-tertiary)] mb-2">Scale Replicas</p>
                <div className="flex items-center gap-2">
                  <input
                    type="number"
                    min={0}
                    max={20}
                    value={scaleReplicas}
                    onChange={e => setScaleReplicas(Number(e.target.value))}
                    className="input w-20 text-center"
                  />
                  <button
                    onClick={handleScale}
                    disabled={mgmtLoading}
                    className="btn btn-primary text-[11px] px-3 py-1.5"
                  >
                    {mgmtLoading ? '...' : 'Apply'}
                  </button>
                </div>
              </div>

              {/* Update Image */}
              <div className="p-3 border border-[var(--border-light)] rounded-lg">
                <p className="text-[11px] text-[var(--text-tertiary)] mb-2">Update Image</p>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    value={newImage}
                    onChange={e => setNewImage(e.target.value)}
                    placeholder="nginx:1.25"
                    className="input flex-1 text-[11px]"
                  />
                  <button
                    onClick={handleUpdateImage}
                    disabled={mgmtLoading || !newImage}
                    className="btn btn-primary text-[11px] px-3 py-1.5"
                  >
                    Update
                  </button>
                </div>
              </div>

              {/* Restart & Logs */}
              <div className="p-3 border border-[var(--border-light)] rounded-lg">
                <p className="text-[11px] text-[var(--text-tertiary)] mb-2">Actions</p>
                <div className="flex items-center gap-2">
                  <button
                    onClick={handleRestart}
                    disabled={mgmtLoading}
                    className="btn btn-secondary text-[11px] px-3 py-1.5"
                  >
                    ⟳ Restart
                  </button>
                  <button
                    onClick={handleViewLogs}
                    disabled={mgmtLoading}
                    className="btn btn-secondary text-[11px] px-3 py-1.5"
                  >
                    📋 Logs
                  </button>
                </div>
              </div>
            </div>

            {/* Logs viewer */}
            {mgmtTab === 'logs' && logs && (
              <div className="mt-4">
                <div className="flex items-center justify-between mb-2">
                  <p className="text-[12px] font-medium text-[var(--text-primary)]">Container Logs</p>
                  <button onClick={() => { setMgmtTab(null); setLogs(''); }} className="text-[11px] text-[var(--accent)] hover:underline">Close</button>
                </div>
                <pre className="bg-[#1e1e1e] text-[#d4d4d4] p-4 rounded-lg text-[11px] font-mono overflow-auto max-h-[400px] whitespace-pre-wrap">
                  {logs}
                </pre>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Deployments Section */}
      <div className="card">
        <div className="card-header flex items-center justify-between">
          <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Deployment History</h2>
        </div>
        <div className="card-body">
          {deployments.length === 0 ? (
            <div className="text-center py-8">
              <p className="text-[12px] text-[var(--text-tertiary)] mb-3">
                No deployments yet. Use the Deploy buttons above to deploy this service to an environment.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {deployments.map(dep => (
                <div key={dep.id} className="flex items-center justify-between p-3 border border-[var(--border-light)] rounded-lg">
                  <div className="flex items-center gap-3">
                    <span className={`w-2 h-2 rounded-full ${
                      dep.status === 'deployed' ? 'bg-green-500' :
                      dep.status === 'deploying' ? 'bg-yellow-500 animate-pulse' :
                      dep.status === 'failed' ? 'bg-red-500' :
                      'bg-gray-300'
                    }`} />
                    <div>
                      <p className="text-[12px] font-medium text-[var(--text-primary)] capitalize">{dep.environment}</p>
                      <p className="text-[10px] text-[var(--text-tertiary)]">
                        {dep.deploy_type} {dep.image_tag ? `• ${dep.image_tag}` : ''}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <span className={`text-[11px] font-medium ${
                        dep.status === 'deployed' ? 'text-green-600' :
                        dep.status === 'deploying' ? 'text-yellow-600' :
                        dep.status === 'failed' ? 'text-red-600' :
                        'text-[var(--text-secondary)]'
                      }`}>
                        {dep.status}
                      </span>
                      {dep.pods_ready > 0 && (
                        <p className="text-[10px] text-[var(--text-tertiary)]">{dep.pods_ready}/{dep.pods_total} pods</p>
                      )}
                    </div>
                    <span className="text-[10px] text-[var(--text-tertiary)]">
                      {new Date(dep.created_at).toLocaleDateString()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Delete Confirmation */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="fixed inset-0 bg-black/30" onClick={() => !deleting && setShowDeleteConfirm(false)} />
          <div className="relative bg-[var(--surface)] rounded-lg shadow-xl p-6 w-full max-w-sm mx-4">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 rounded-full bg-red-500/15 flex items-center justify-center shrink-0">
                <svg className="w-5 h-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                </svg>
              </div>
              <div>
                <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">Delete Service</h3>
                <p className="text-[12px] text-[var(--text-secondary)]">Are you sure you want to delete &quot;{service.name}&quot;? This cannot be undone.</p>
              </div>
            </div>
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDeleteConfirm(false)} disabled={deleting} className="text-[12px] px-3 py-1.5 rounded border border-[var(--border)] text-[var(--text-secondary)] hover:bg-[var(--border-light)] disabled:opacity-50">Cancel</button>
              <button onClick={handleDelete} disabled={deleting} className="text-[12px] px-3 py-1.5 rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50">{deleting ? 'Deleting...' : 'Delete'}</button>
            </div>
          </div>
        </div>
      )}

      {/* Links */}
      <div className="grid grid-cols-2 gap-4">
        {service.gitlab_project_url && (
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">GitLab Project</p>
            <a href={service.gitlab_project_url} target="_blank" rel="noopener noreferrer" className="text-[12px] text-[var(--accent)] hover:underline break-all">
              {service.gitlab_project_url}
            </a>
          </div>
        )}
        {service.helm_chart_url && (
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Helm Chart</p>
            <a href={service.helm_chart_url} target="_blank" rel="noopener noreferrer" className="text-[12px] text-[var(--accent)] hover:underline break-all">
              {service.helm_chart_url}
            </a>
          </div>
        )}
      </div>
      </div>
    </div>
  );
}
