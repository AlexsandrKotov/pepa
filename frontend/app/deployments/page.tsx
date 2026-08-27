'use client';
import { useState, useEffect, useRef, Suspense } from 'react';
import { useSearchParams, usePathname, useRouter } from 'next/navigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { deployments, clusters, helmRepositories, type Deployment, type DeploymentContainer, type Cluster, type HelmRepository, type HelmChart as HelmChartType, type HelmChartVersion } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';
import BrandIcon from '@/components/BrandIcon';
import DeploymentDetailClient from './DeploymentDetailClient';

function DeploymentsPageContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const deploymentId = searchParams.get('id');
  const autoCreate = searchParams.get('create') === 'true';

  // Legacy ?create=true links are normalized to the dedicated route
  useEffect(() => {
    if (autoCreate && !deploymentId) {
      router.replace('/deployments/new');
    }
  }, [autoCreate, deploymentId, router]);

  if (deploymentId) {
    return <DeploymentDetailClient />;
  }
  return <DeploymentsList autoCreate={autoCreate} />;
}

export default function DeploymentsPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <DeploymentsPageContent />
    </Suspense>
  );
}

const statusColors: Record<string, string> = {
  pending: 'bg-[var(--border-light)] text-[var(--text-secondary)]',
  pr_created: 'bg-blue-500/10 text-blue-500',
  merged: 'bg-violet-500/10 text-violet-500',
  syncing: 'bg-amber-500/15 text-amber-600',
  deployed: 'bg-emerald-500/15 text-emerald-600',
  promoted: 'bg-emerald-500/15 text-emerald-600',
  rolled_back: 'bg-orange-500/10 text-orange-600',
  cancelled: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  failed: 'bg-red-500/15 text-red-500',
};

const statusIcons: Record<string, string> = {
  pending: '\u25CB',
  pr_created: '\u25CC',
  merged: '\u25C9',
  syncing: '\u21BB',
  deployed: '\u2713',
  promoted: '\u2B06',
  rolled_back: '\u21A9',
  cancelled: '\u2717',
  failed: '\u2717',
};

export function DeploymentsList({ autoCreate }: { autoCreate?: boolean }) {
  const pathname = usePathname();
  const router = useRouter();
  const [deployList, setDeployList] = useState<Deployment[]>([]);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [pageTab, setPageTab] = useState<'all' | 'create'>(autoCreate ? 'create' : 'all');

  // Switch tab and keep the URL in sync so the sidebar highlight matches.
  // /deployments → list, /deployments/new → create form.
  const switchTab = (tab: 'all' | 'create') => {
    setPageTab(tab);
    if (tab === 'create' && pathname === '/deployments') {
      router.push('/deployments/new');
    } else if (tab === 'all' && pathname === '/deployments/new') {
      router.push('/deployments');
    }
  };

  // Auto-switch to create tab when ?create=true is in URL
  useEffect(() => {
    if (autoCreate && !loading) {
      setPageTab('create');
    }
  }, [autoCreate, loading]);
  const [creating, setCreating] = useState(false);
  const [showLogs, setShowLogs] = useState<string | null>(null);
  const [createFeedback, setCreateFeedback] = useState<{ ok: boolean; text: string } | null>(null);

  useEscapeKey(() => {
    if (showLogs) setShowLogs(null);
    else if (pageTab === 'create') switchTab('all');
  }, pageTab === 'create' || showLogs !== null);
  const [logsData, setLogsData] = useState<{ logs: { timestamp: string; level: string; message: string }[]; status: string; error_message: string } | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);

  const [form, setForm] = useState({
    jira_issue_key: '', jira_summary: '', gitlab_project_name: '',
    target_namespace: 'default', image_tag: '', image_repository: '', created_by: '',
  });
  const [selectedClusterId, setSelectedClusterId] = useState<string>('');
  const [deployType, setDeployType] = useState('helm');
  const [replicas, setReplicas] = useState(1);
  const [strategy, setStrategy] = useState('rolling');
  const [containers, setContainers] = useState<DeploymentContainer[]>([
    { name: 'main', image: '', cpu: '100m', memory: '128Mi', ports: [{ containerPort: 8080 }] }
  ]);
  const [valuesYaml, setValuesYaml] = useState('');
  const [envMode, setEnvMode] = useState<'kv' | 'yaml'>('kv');
  const [servicePort, setServicePort] = useState(80);
  const [serviceType, setServiceType] = useState('ClusterIP');
  const [ingressEnabled, setIngressEnabled] = useState(false);
  const [ingressHost, setIngressHost] = useState('');
  const [livenessPath, setLivenessPath] = useState('/healthz');
  const [readinessPath, setReadinessPath] = useState('/ready');
  const [modalTab, setModalTab] = useState<'basic' | 'containers' | 'values' | 'network' | 'strategy'>('basic');
  const [timeoutSeconds, setTimeoutSeconds] = useState(300);

  // Helm chart fields
  const [chartSourceType, setChartSourceType] = useState('helm_http');
  const [chartUrl, setChartUrl] = useState('');
  const [chartName, setChartName] = useState('');
  const [chartVersion, setChartVersion] = useState('');

  // Helm repo chart picker state
  const [helmRepoList, setHelmRepoList] = useState<HelmRepository[]>([]);
  const [helmCharts, setHelmCharts] = useState<(HelmChartType & { repoId: string; repoName: string })[]>([]);
  const [helmChartVersions, setHelmChartVersions] = useState<HelmChartVersion[]>([]);
  const [selectedHelmRepoChart, setSelectedHelmRepoChart] = useState(''); // "repoId:chartName"
  const [loadingHelmCharts, setLoadingHelmCharts] = useState(false);
  const [helmInputMode, setHelmInputMode] = useState<'picker' | 'manual'>('picker');

  const loadHelmRepos = async () => {
    try {
      const data = await helmRepositories.list().catch(() => ({ helm_repositories: [], total: 0 }));
      const repos = data.helm_repositories || [];
      setHelmRepoList(repos);
      setLoadingHelmCharts(true);
      const allCharts: (HelmChartType & { repoId: string; repoName: string })[] = [];
      await Promise.all(repos.map(async (repo) => {
        try {
          const chartsData = await helmRepositories.listCharts(repo.id);
          const charts = (chartsData.charts || []).map(c => ({ ...c, repoId: repo.id, repoName: repo.name }));
          allCharts.push(...charts);
        } catch { /* ignore */ }
      }));
      setHelmCharts(allCharts);
    } catch {}
    setLoadingHelmCharts(false);
  };

  const handleHelmRepoChartChange = async (value: string) => {
    setSelectedHelmRepoChart(value);
    if (!value) { setChartUrl(''); setChartName(''); setChartVersion(''); setHelmChartVersions([]); return; }
    const [repoId, cName] = value.split(':');
    const chart = helmCharts.find(c => c.repoId === repoId && c.name === cName);
    if (chart) {
      const repo = helmRepoList.find(r => r.id === repoId);
      setChartName(cName);
      if (repo) {
        setChartUrl(repo.url);
        setChartSourceType(repo.repo_type === 'oci' ? 'helm_oci' : 'helm_http');
      }
    }
    try {
      const versionsData = await helmRepositories.listChartVersions(repoId, cName);
      setHelmChartVersions(versionsData.versions || []);
    } catch { setHelmChartVersions([]); }
  };

  const refresh = async () => {
    try {
      const [d, c] = await Promise.all([deployments.list(), clusters.list()]);
      setDeployList(d.deployments || []);
      setClusterList(c.clusters || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  useEffect(() => { refresh(); }, []);

  // Lazy-load Helm repos only when user selects Helm deploy type
  const helmReposLoaded = useRef(false);
  useEffect(() => {
    if (deployType === 'helm' && !helmReposLoaded.current) {
      helmReposLoaded.current = true;
      loadHelmRepos();
    }
  }, [deployType]);

  const handleCreate = async () => {
    setCreating(true);
    try {
      // Build spec from form data
      const spec: Record<string, unknown> = {};

      // Containers
      const containerList = containers.filter(c => c.image);
      if (containerList.length > 0) {
        spec.containers = containerList;
      }

      // values.yaml
      if (valuesYaml.trim()) {
        spec.values_yaml = valuesYaml;
      }

      // Service
      spec.service = { port: servicePort, type: serviceType };

      // Ingress
      if (ingressEnabled) {
        spec.ingress = { enabled: true, host: ingressHost };
      }

      // Health checks
      spec.health = { livenessPath, readinessPath, port: servicePort };

      // Chart info for Helm deploys
      if (deployType === 'helm' && chartUrl) {
        spec.chart = {
          source_type: chartSourceType,
          chart_url: chartUrl,
          chart_name: chartName,
          chart_version: chartVersion || undefined,
        };
      }

      await deployments.create({
        ...form,
        target_cluster_id: selectedClusterId || undefined,
        deploy_type: deployType,
        replicas,
        strategy,
        spec,
        status: 'pending',
        timeout_seconds: timeoutSeconds,
      });
      switchTab('all');
      setForm({ jira_issue_key: '', jira_summary: '', gitlab_project_name: '', target_namespace: 'default', image_tag: '', image_repository: '', created_by: '' });
      setContainers([{ name: 'main', image: '', cpu: '100m', memory: '128Mi', ports: [{ containerPort: 8080 }] }]);
      setValuesYaml('');
      setChartUrl('');
      setChartName('');
      setChartVersion('');
      setModalTab('basic');
      await refresh();
      setCreateFeedback({ ok: true, text: 'Deployment created successfully' });
    } catch (e: unknown) {
      setCreateFeedback({ ok: false, text: 'Failed: ' + (e instanceof Error ? e.message : 'Error') });
    }
    setCreating(false);
  };

  const addContainer = () => {
    setContainers([...containers, { name: `sidecar-${containers.length}`, image: '', cpu: '50m', memory: '64Mi', ports: [] }]);
  };
  const removeContainer = (idx: number) => {
    if (containers.length <= 1) return;
    setContainers(containers.filter((_, i) => i !== idx));
  };
  const updateContainer = (idx: number, field: keyof DeploymentContainer, val: unknown) => {
    const updated = [...containers];
    updated[idx] = { ...updated[idx], [field]: val };
    setContainers(updated);
  };
  const addContainerPort = (idx: number) => {
    const updated = [...containers];
    updated[idx].ports = [...(updated[idx].ports || []), { containerPort: 8080 }];
    setContainers(updated);
  };
  const updateContainerPort = (cIdx: number, pIdx: number, port: number) => {
    const updated = [...containers];
    updated[cIdx].ports = [...(updated[cIdx].ports || [])];
    updated[cIdx].ports![pIdx] = { containerPort: port };
    setContainers(updated);
  };
  const removeContainerPort = (cIdx: number, pIdx: number) => {
    const updated = [...containers];
    updated[cIdx].ports = (updated[cIdx].ports || []).filter((_, i) => i !== pIdx);
    setContainers(updated);
  };

  const handlePromote = async (id: string) => {
    try { await deployments.promote(id); await refresh(); } catch { /* ignore */ }
  };

  const handleRollback = async (id: string) => {
    if (!confirm('Rollback this deployment?')) return;
    try { await deployments.rollback(id); await refresh(); } catch { /* ignore */ }
  };

  const handleCancel = async (id: string) => {
    if (!confirm('Cancel this deployment?')) return;
    try { await deployments.cancel(id); await refresh(); } catch { /* ignore */ }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this deployment record? This cannot be undone.')) return;
    try { await deployments.delete(id); await refresh(); } catch { /* ignore */ }
  };

  const handleShowLogs = async (id: string) => {
    setShowLogs(id);
    setLogsLoading(true);
    setLogsData(null);
    try {
      const res = await deployments.logs(id);
      setLogsData({
        logs: res.logs || [],
        status: res.status || '',
        error_message: res.error_message || '',
      });
    } catch {
      setLogsData({ logs: [], status: 'error', error_message: 'Failed to fetch logs' });
    }
    setLogsLoading(false);
  };

  // Auto-refresh logs when deployment is in progress
  useEffect(() => {
    if (!showLogs || !logsData) return;
    const isActive = logsData.status === 'pending' || logsData.status === 'syncing';
    if (!isActive) return;
    const interval = setInterval(async () => {
      try {
        const res = await deployments.logs(showLogs);
        setLogsData({
          logs: res.logs || [],
          status: res.status || '',
          error_message: res.error_message || '',
        });
      } catch { /* ignore */ }
    }, 3000);
    return () => clearInterval(interval);
  }, [showLogs, logsData?.status]);

  const filtered = statusFilter
    ? deployList.filter(d => d.status === statusFilter)
    : deployList;

  const stats = {
    total: deployList.length,
    deployed: deployList.filter(d => d.status === 'deployed' || d.status === 'promoted').length,
    inProgress: deployList.filter(d => d.status === 'pending' || d.status === 'syncing').length,
    failed: deployList.filter(d => d.status === 'failed').length,
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">{pageTab === 'create' ? 'New Deployment' : 'Deployments'}</h1>
            <ConceptHelp term="deployment" />
          </div>
          <p className="page-subtitle-modern">Deploy applications directly to Kubernetes clusters</p>
        </div>
        {pageTab === 'all' && (
          <button onClick={() => switchTab('create')} className="btn btn-primary">
            + New Deployment
          </button>
        )}
      </div>

      {/* Feedback */}
      {createFeedback && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 page-animate-up ${
          createFeedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <div>
            <p className={`text-sm font-medium ${createFeedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {createFeedback.ok ? '✓ ' : '⚠ '}{createFeedback.text}
            </p>
          </div>
          <button onClick={() => setCreateFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      {pageTab === 'all' && (<>
      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 page-animate-up page-delay-1">
        {[
          { label: 'Total', value: stats.total, color: 'text-[var(--text-primary)]', icon: 'dashboard', iconClass: 'stat-icon-blue' },
          { label: 'Deployed', value: stats.deployed, color: 'text-green-600', icon: 'argocd', iconClass: 'stat-icon-green' },
          { label: 'In Progress', value: stats.inProgress, color: 'text-yellow-600', icon: 'cicd', iconClass: 'stat-icon-amber' },
          { label: 'Failed', value: stats.failed, color: 'text-red-600', icon: 'vault', iconClass: 'stat-icon-red' },
        ].map(s => (
          <div key={s.label} className="modern-stat-card flex items-center gap-3">
            <div className={`w-10 h-10 rounded-xl ${s.iconClass} flex items-center justify-center text-white text-sm`}>
              <BrandIcon name={s.icon} size={20} monochrome />
            </div>
            <div>
              <div className={`text-[22px] font-bold ${s.color}`}>{s.value}</div>
              <div className="text-[11px] text-[var(--text-tertiary)]">{s.label}</div>
            </div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="flex gap-3 page-animate-up page-delay-2">
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="input w-48"
        >
          <option value="">All statuses</option>
          <option value="pending">Pending</option>
          <option value="syncing">Syncing</option>
          <option value="deployed">Deployed</option>
          <option value="promoted">Promoted</option>
          <option value="rolled_back">Rolled Back</option>
          <option value="failed">Failed</option>
        </select>
      </div>

      {/* Table */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading deployments...</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30 flex items-center justify-center">
            <BrandIcon name="argocd" size={48} />
          </div>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1">No deployments yet</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-4">Create a deployment to start the GitOps pipeline</p>
          <button onClick={() => switchTab('create')} className="btn btn-primary inline-block">+ New Deployment</button>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-[var(--border)]" style={{ borderRadius: '12px' }}>
          <table style={{ minWidth: 1200 }}>
            <colgroup>
              <col style={{ width: 220, minWidth: 180 }} />
              <col style={{ width: 200, minWidth: 160 }} />
              <col style={{ width: 160, minWidth: 130 }} />
              <col style={{ width: 90, minWidth: 80 }} />
              <col style={{ width: 130, minWidth: 110 }} />
              <col style={{ width: 100, minWidth: 90 }} />
              <col style={{ width: 'auto', minWidth: 220 }} />
            </colgroup>
            <thead>
              <tr>
                <th className="!px-3">Deployment</th>
                <th className="!px-3">Chart / Image</th>
                <th className="!px-3">Target</th>
                <th className="!px-3">Type</th>
                <th className="!px-3">Status</th>
                <th className="!px-3">Created</th>
                <th className="!px-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(d => {
                const cluster = clusterList.find(c => c.id === d.target_cluster_id);
                const chartName = d.spec?.chart?.chart_name;
                const chartVersion = d.spec?.chart?.chart_version;
                const displayName = d.gitlab_project_name || chartName || d.jira_issue_key || 'Unnamed deployment';
                const displayImage = d.image_repository
                  ? `${d.image_repository}:${d.image_tag || 'latest'}`
                  : d.image_tag
                    ? `:${d.image_tag}`
                    : chartName
                      ? `${chartName}${chartVersion ? ` v${chartVersion}` : ''}`
                      : '-';
                return (
                <tr key={d.id} className="border-b border-[var(--border-light)] last:border-b-0 hover:bg-[var(--bg)]">
                  <td className="!px-2 !py-2.5">
                    <Link href={`/deployments?id=${d.id}`} className="font-medium text-[var(--text-primary)] hover:text-[var(--accent)]">
                      {displayName}
                    </Link>
                    {d.jira_summary && <p className="text-[11px] text-[var(--text-tertiary)] truncate max-w-[200px]">{d.jira_summary}</p>}
                    {d.jira_issue_key && d.gitlab_project_name && (
                      <p className="text-[10px] text-[var(--accent)]">{d.jira_issue_key}</p>
                    )}
                    {d.team_name && (
                      <p className="text-[10px] text-[var(--text-tertiary)]">Team: {d.team_name}{d.stage ? ` / ${d.stage}` : ''}</p>
                    )}
                  </td>
                  <td className="!px-2 !py-2.5">
                    {chartName ? (
                      <>
                        <p className="text-[12px] font-mono text-[var(--text-primary)] truncate max-w-[180px]">
                          {chartName}{chartVersion && <span className="text-[var(--accent)]"> v{chartVersion}</span>}
                        </p>
                        {d.spec?.chart?.chart_url && (
                          <p className="text-[10px] text-[var(--text-tertiary)] truncate max-w-[180px]">{d.spec.chart.chart_url}</p>
                        )}
                      </>
                    ) : null}
                    {displayImage !== '-' && !chartName && (
                      <p className="text-[12px] font-mono text-[var(--text-primary)] truncate max-w-[180px]">{displayImage}</p>
                    )}
                    {displayImage !== '-' && chartName && d.image_repository && (
                      <p className="text-[10px] font-mono text-[var(--text-tertiary)] truncate max-w-[180px]">{displayImage}</p>
                    )}
                    {d.spec?.containers && d.spec.containers.length > 1 && (
                      <p className="text-[10px] text-[var(--accent)]">{d.spec.containers.length} containers</p>
                    )}
                    {d.gitlab_mr_url && (
                      <a href={d.gitlab_mr_url} className="text-[11px] text-[var(--accent)] hover:underline" target="_blank">
                        MR #{d.gitlab_mr_id}
                      </a>
                    )}
                    {!chartName && displayImage === '-' && <span className="text-[12px] text-[var(--text-tertiary)]">-</span>}
                  </td>
                  <td className="!px-2 !py-2.5">
                    <p className="text-[12px] text-[var(--text-primary)]">{d.target_namespace || 'default'}</p>
                    {cluster ? (
                      <p className="text-[11px] text-[var(--text-secondary)] font-medium">{cluster.name}</p>
                    ) : d.target_cluster_id ? (
                      <p className="text-[11px] text-[var(--text-tertiary)]">{d.target_cluster_id.slice(0, 8)}</p>
                    ) : (
                      <p className="text-[11px] text-[var(--text-tertiary)]">No cluster</p>
                    )}
                  </td>
                  <td className="whitespace-nowrap !px-2 !py-2.5">
                    <span className="text-[11px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] font-medium">
                      {d.deploy_type || 'helm'}
                    </span>
                  </td>
                  <td className="!px-2 !py-2.5">
                    <span className={`badge ${statusColors[d.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                      {statusIcons[d.status] || '\u25CB'} {d.status}
                    </span>
                    {(d.replicas ?? 0) > 0 && (
                      <p className="text-[10px] text-[var(--text-tertiary)] mt-0.5 whitespace-nowrap">
                        {d.replicas} replica(s)
                      </p>
                    )}
                    {d.status === 'failed' && d.error_message && (
                      <p className="text-[10px] text-red-600 mt-1 truncate max-w-[200px]" title={d.error_message}>
                        {d.error_message}
                      </p>
                    )}
                  </td>
                  <td className="text-[12px] text-[var(--text-tertiary)] whitespace-nowrap !px-2 !py-2.5">
                    {new Date(d.created_at).toLocaleDateString()}
                  </td>
                  <td className="!px-2 !py-2.5">
                    <div className="flex gap-1">
                      <button onClick={() => handleShowLogs(d.id)} className="text-[11px] px-2 py-1 bg-blue-500/10 text-blue-500 rounded hover:bg-blue-500/15">
                        Logs
                      </button>
                      {(d.status === 'deployed') && (
                        <>
                          <button onClick={() => handlePromote(d.id)} className="text-[11px] px-2 py-1 bg-emerald-500/10 text-emerald-600 rounded hover:bg-emerald-500/15">
                            Promote
                          </button>
                          <button onClick={() => handleRollback(d.id)} className="text-[11px] px-2 py-1 bg-orange-500/10 text-orange-600 rounded hover:bg-orange-500/15">
                            Rollback
                          </button>
                        </>
                      )}
                      {(d.status === 'pending' || d.status === 'syncing') && (
                        <button onClick={() => handleCancel(d.id)} className="text-[11px] px-2 py-1 bg-red-500/10 text-red-500 rounded hover:bg-red-500/15">
                          Cancel
                        </button>
                      )}
                      <button onClick={() => handleDelete(d.id)} className="text-[11px] px-2 py-1 bg-red-500/5 text-red-400 rounded hover:bg-red-500/10" title="Delete deployment record">
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      </>)}

      {/* Create Tab Content */}
      {pageTab === 'create' && (
        <div className="card">
          <div className="card-body space-y-4">
            {/* Tabs */}
            <div className="flex gap-1 border-b border-[var(--border)]">
              {([
                { key: 'basic' as const, label: 'Basic' },
                { key: 'containers' as const, label: 'Containers' },
                { key: 'values' as const, label: 'values.yaml' },
                { key: 'network' as const, label: 'Network' },
                { key: 'strategy' as const, label: 'Strategy' },
              ]).map(t => (
                <button
                  key={t.key}
                  onClick={() => setModalTab(t.key)}
                  className={`px-3 py-2 text-[12px] font-medium border-b-2 transition-colors outline-none ${
                    modalTab === t.key
                      ? 'border-[var(--accent)] text-[var(--accent)]'
                      : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>
        
            {/* Content */}
            <div className="space-y-4">
              {/* Tab: Basic */}
              {modalTab === 'basic' && (
                <div className="space-y-3">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Jira Issue Key</label>
                      <input value={form.jira_issue_key} onChange={e => setForm({ ...form, jira_issue_key: e.target.value })} className="input" placeholder="PEPA-101" />
                    </div>
                    <div>
                      <label className="label">Project Name</label>
                      <input value={form.gitlab_project_name} onChange={e => setForm({ ...form, gitlab_project_name: e.target.value })} className="input" placeholder="my-service" />
                    </div>
                  </div>
                  <div>
                    <label className="label">Jira Summary</label>
                    <input value={form.jira_summary} onChange={e => setForm({ ...form, jira_summary: e.target.value })} className="input" placeholder="Brief description" />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Target Namespace</label>
                      <input value={form.target_namespace} onChange={e => setForm({ ...form, target_namespace: e.target.value })} className="input" placeholder="default" />
                    </div>
                    <div>
                      <label className="label">Target Cluster</label>
                      <select value={selectedClusterId} onChange={e => setSelectedClusterId(e.target.value)} className="input">
                        <option value="">Select cluster...</option>
                        {clusterList.map(c => <option key={c.id} value={c.id}>{c.name} ({c.environment})</option>)}
                      </select>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Deploy Type</label>
                      <select value={deployType} onChange={e => setDeployType(e.target.value)} className="input">
                        <option value="helm">Helm</option>
                        <option value="kustomize">Kustomize</option>
                        <option value="raw">Raw Kubernetes</option>
                      </select>
                    </div>
                    <div>
                      <label className="label">Replicas</label>
                      <input type="number" value={replicas} onChange={e => setReplicas(Number(e.target.value) || 1)} className="input" min={1} />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="label">Image Repository (primary)</label>
                      <input value={form.image_repository} onChange={e => setForm({ ...form, image_repository: e.target.value })} className="input" placeholder="registry.example.com/myapp" />
                    </div>
                    <div>
                      <label className="label">Image Tag (primary)</label>
                      <input value={form.image_tag} onChange={e => setForm({ ...form, image_tag: e.target.value })} className="input" placeholder="v1.2.3" />
                    </div>
                  </div>

                  {/* Helm Chart Configuration — shown when deploy type is helm */}
                  {deployType === 'helm' && (
                    <div className="border border-blue-500/20 bg-blue-500/10 rounded-lg p-3 space-y-3">
                      <div className="flex items-center justify-between">
                        <p className="text-[11px] font-semibold text-[var(--accent)] uppercase tracking-wider">Helm Chart</p>
                        {helmRepoList.length > 0 && (
                          <div className="flex gap-1">
                            <button
                              onClick={() => setHelmInputMode('picker')}
                              className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${helmInputMode === 'picker' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                            >
                              Browse
                            </button>
                            <button
                              onClick={() => setHelmInputMode('manual')}
                              className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${helmInputMode === 'manual' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                            >
                              Manual
                            </button>
                          </div>
                        )}
                      </div>
                      {helmInputMode === 'picker' ? (
                        <div className="space-y-3">
                          <div>
                            <label className="text-[11px] text-[var(--text-tertiary)]">Chart</label>
                            {loadingHelmCharts ? (
                              <div className="flex items-center gap-2 py-2">
                                <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
                                <span className="text-[11px] text-[var(--text-tertiary)]">Loading charts...</span>
                              </div>
                            ) : (
                              <select
                                value={selectedHelmRepoChart}
                                onChange={e => handleHelmRepoChartChange(e.target.value)}
                                className="input text-[12px]"
                              >
                                <option value="">Select a chart...</option>
                                {helmCharts.map(c => (
                                  <option key={`${c.repoId}:${c.name}`} value={`${c.repoId}:${c.name}`}>
                                    {c.repoName} / {c.name} {c.latest_version && `(v${String(c.latest_version).replace(/^v/, '')})`}
                                  </option>
                                ))}
                              </select>
                            )}
                          </div>
                          {helmChartVersions.length > 0 && (
                            <div>
                              <label className="text-[11px] text-[var(--text-tertiary)]">Version</label>
                              <select
                                value={chartVersion}
                                onChange={e => setChartVersion(e.target.value)}
                                className="input text-[12px]"
                              >
                                <option value="">Latest</option>
                                {helmChartVersions.map(v => (
                                  <option key={v.version} value={v.version}>v{String(v.version).replace(/^v/, '')}{v.app_version ? ` (app: ${v.app_version})` : ''}</option>
                                ))}
                              </select>
                            </div>
                          )}
                          {chartUrl && (
                            <div className="flex items-center gap-2 text-[11px] text-[var(--text-tertiary)]">
                              <span className="font-medium">Source:</span>
                              <span className="font-mono truncate">{chartUrl}{chartName ? `/${chartName}` : ''}</span>
                            </div>
                          )}
                        </div>
                      ) : (
                        <div className="space-y-3">
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <label className="text-[11px] text-[var(--text-tertiary)]">Chart Source Type</label>
                              <select value={chartSourceType} onChange={e => setChartSourceType(e.target.value)} className="input text-[12px]">
                                <option value="helm_http">HTTP Repository</option>
                                <option value="helm_oci">OCI Registry</option>
                                <option value="helm_git">Git Repository</option>
                              </select>
                            </div>
                            <div>
                              <label className="text-[11px] text-[var(--text-tertiary)]">Chart Version</label>
                              <input value={chartVersion} onChange={e => setChartVersion(e.target.value)} className="input text-[12px]" placeholder="1.2.3" />
                            </div>
                          </div>
                          <div>
                            <label className="text-[11px] text-[var(--text-tertiary)]">Chart Repository URL</label>
                            <input value={chartUrl} onChange={e => setChartUrl(e.target.value)} className="input text-[12px] font-mono" placeholder="https://charts.bitnami.com/bitnami" />
                          </div>
                          <div>
                            <label className="text-[11px] text-[var(--text-tertiary)]">Chart Name</label>
                            <input value={chartName} onChange={e => setChartName(e.target.value)} className="input text-[12px]" placeholder="postgresql" />
                          </div>
                        </div>
                      )}
                      {helmRepoList.length === 0 && (
                        <p className="text-[11px] text-[var(--text-tertiary)]">
                          No Helm repositories configured. <Link href="/helm-repositories" className="text-[var(--accent)] hover:underline">Add one</Link> to browse charts.
                        </p>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* Tab: Containers */}
              {modalTab === 'containers' && (
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <p className="text-[12px] text-[var(--text-secondary)]">Define all containers for this deployment (main app + sidecars)</p>
                    <button onClick={addContainer} className="text-[11px] text-[var(--accent)] hover:underline font-medium">+ Add Container</button>
                  </div>
                  {containers.map((ctr, idx) => (
                    <div key={idx} className="border border-[var(--border)] rounded-lg p-4 space-y-3 relative">
                      <div className="flex items-center justify-between">
                        <span className="text-[12px] font-semibold text-[var(--text-primary)]">Container {idx + 1}: {ctr.name || 'unnamed'}</span>
                        {containers.length > 1 && (
                          <button onClick={() => removeContainer(idx)} className="text-[11px] text-red-500 hover:text-red-400">Remove</button>
                        )}
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-[11px] text-[var(--text-tertiary)]">Name</label>
                          <input value={ctr.name} onChange={e => updateContainer(idx, 'name', e.target.value)} className="input text-[12px]" placeholder="main" />
                        </div>
                        <div>
                          <label className="text-[11px] text-[var(--text-tertiary)]">Image</label>
                          <input value={ctr.image} onChange={e => updateContainer(idx, 'image', e.target.value)} className="input text-[12px] font-mono" placeholder="registry.example.com/app:v1.2.3" />
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="text-[11px] text-[var(--text-tertiary)]">CPU</label>
                          <input value={ctr.cpu} onChange={e => updateContainer(idx, 'cpu', e.target.value)} className="input text-[12px]" placeholder="100m" />
                        </div>
                        <div>
                          <label className="text-[11px] text-[var(--text-tertiary)]">Memory</label>
                          <input value={ctr.memory} onChange={e => updateContainer(idx, 'memory', e.target.value)} className="input text-[12px]" placeholder="128Mi" />
                        </div>
                      </div>
                      <div>
                        <div className="flex items-center justify-between mb-1">
                          <label className="text-[11px] text-[var(--text-tertiary)]">Ports</label>
                          <button onClick={() => addContainerPort(idx)} className="text-[10px] text-[var(--accent)] hover:underline">+ Port</button>
                        </div>
                        <div className="flex gap-2 flex-wrap">
                          {(ctr.ports || []).map((p, pIdx) => (
                            <div key={pIdx} className="flex items-center gap-1">
                              <input
                                type="number"
                                value={p.containerPort}
                                onChange={e => updateContainerPort(idx, pIdx, Number(e.target.value))}
                                className="input text-[11px] w-20"
                                placeholder="8080"
                              />
                              <button onClick={() => removeContainerPort(idx, pIdx)} className="text-red-400 text-[10px] hover:text-red-600">{'\u2715'}</button>
                            </div>
                          ))}
                          {(ctr.ports || []).length === 0 && <span className="text-[11px] text-[var(--text-tertiary)]">No ports defined</span>}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {/* Tab: values.yaml */}
              {modalTab === 'values' && (
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-[12px] text-[var(--text-secondary)]">Paste your Helm values.yaml for this deployment</p>
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">This will be used as-is for Helm-based deployments</p>
                    </div>
                    <div className="flex gap-1">
                      <button
                        onClick={() => setEnvMode('kv')}
                        className={`text-[11px] px-2 py-1 rounded ${envMode === 'kv' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)]'}`}
                      >
                        Key-Value
                      </button>
                      <button
                        onClick={() => setEnvMode('yaml')}
                        className={`text-[11px] px-2 py-1 rounded ${envMode === 'yaml' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)]'}`}
                      >
                        YAML
                      </button>
                    </div>
                  </div>
                  <textarea
                    value={valuesYaml}
                    onChange={e => setValuesYaml(e.target.value)}
                    className="input font-mono text-[12px] w-full"
                    rows={16}
                    spellCheck={false}
                    placeholder={`# Paste your values.yaml here
# Example:
replicaCount: ${replicas}

image:
  repository: ${form.image_repository || 'registry.example.com/myapp'}
  tag: "${form.image_tag || 'v1.2.3'}"

service:
  type: ${serviceType}
  port: ${servicePort}

resources:
  limits:
    cpu: 200m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi`}
                  />
                </div>
              )}

              {/* Tab: Network */}
              {modalTab === 'network' && (
                <div className="space-y-4">
                  <div className="border border-[var(--border)] rounded-lg p-4 space-y-3">
                    <h3 className="text-[12px] font-semibold text-[var(--text-primary)]">Service</h3>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="text-[11px] text-[var(--text-tertiary)]">Service Port</label>
                        <input type="number" value={servicePort} onChange={e => setServicePort(Number(e.target.value))} className="input text-[12px]" />
                      </div>
                      <div>
                        <label className="text-[11px] text-[var(--text-tertiary)]">Service Type</label>
                        <select value={serviceType} onChange={e => setServiceType(e.target.value)} className="input text-[12px]">
                          <option value="ClusterIP">ClusterIP</option>
                          <option value="NodePort">NodePort</option>
                          <option value="LoadBalancer">LoadBalancer</option>
                        </select>
                      </div>
                    </div>
                  </div>
                  <div className="border border-[var(--border)] rounded-lg p-4 space-y-3">
                    <div className="flex items-center gap-2">
                      <input type="checkbox" checked={ingressEnabled} onChange={e => setIngressEnabled(e.target.checked)} className="rounded" />
                      <h3 className="text-[12px] font-semibold text-[var(--text-primary)]">Ingress</h3>
                    </div>
                    {ingressEnabled && (
                      <div>
                        <label className="text-[11px] text-[var(--text-tertiary)]">Host</label>
                        <input value={ingressHost} onChange={e => setIngressHost(e.target.value)} className="input text-[12px]" placeholder="myapp.example.com" />
                      </div>
                    )}
                  </div>
                  <div className="border border-[var(--border)] rounded-lg p-4 space-y-3">
                    <h3 className="text-[12px] font-semibold text-[var(--text-primary)]">Health Checks</h3>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="text-[11px] text-[var(--text-tertiary)]">Liveness Path</label>
                        <input value={livenessPath} onChange={e => setLivenessPath(e.target.value)} className="input text-[12px]" placeholder="/healthz" />
                      </div>
                      <div>
                        <label className="text-[11px] text-[var(--text-tertiary)]">Readiness Path</label>
                        <input value={readinessPath} onChange={e => setReadinessPath(e.target.value)} className="input text-[12px]" placeholder="/ready" />
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {/* Tab: Strategy */}
              {modalTab === 'strategy' && (
                <div className="space-y-4">
                  <div>
                    <label className="label">Deployment Strategy</label>
                    <div className="grid grid-cols-3 gap-3">
                      {[
                        { value: 'rolling', label: 'Rolling Update', desc: 'Gradually replace instances' },
                        { value: 'canary', label: 'Canary', desc: 'Route traffic to new version' },
                        { value: 'blue-green', label: 'Blue-Green', desc: 'Switch between full copies' },
                      ].map(s => (
                        <button
                          key={s.value}
                          onClick={() => setStrategy(s.value)}
                          className={`p-3 rounded-lg border text-left transition-all ${
                            strategy === s.value
                              ? 'border-[var(--border)] bg-[var(--accent-subtle)]'
                              : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                          }`}
                        >
                          <div className="text-[12px] font-medium text-[var(--text-primary)]">{s.label}</div>
                          <div className="text-[10px] text-[var(--text-tertiary)] mt-0.5">{s.desc}</div>
                        </button>
                      ))}
                    </div>
                  </div>
                  <div>
                    <label className="label">Replicas</label>
                    <input type="number" value={replicas} onChange={e => setReplicas(Number(e.target.value) || 1)} className="input w-32" min={1} />
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Number of pod instances to run</p>
                  </div>
                  <div>
                    <label className="label">Timeout (seconds)</label>
                    <input type="number" value={timeoutSeconds} onChange={e => setTimeoutSeconds(Number(e.target.value) || 300)} className="input w-32" min={30} max={3600} />
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Max time to wait for deployment to complete (default: 300s / 5min)</p>
                  </div>
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between pt-4 border-t border-[var(--border)]">
              <div className="text-[11px] text-[var(--text-tertiary)]">
                {containers.filter(c => c.image).length} container(s)
                {valuesYaml.trim() ? ' \u00B7 values.yaml' : ''}
                {' \u00B7 '}{replicas} replica(s)
              </div>
              <div className="flex gap-2">
                <button onClick={() => switchTab('all')} className="btn btn-secondary text-[12px]">Cancel</button>
                <button onClick={handleCreate} disabled={creating} className="btn btn-primary text-[12px]">
                  {creating ? 'Creating...' : 'Create Deployment'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Logs Modal */}
      {showLogs && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowLogs(null)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-3xl mx-4 max-h-[80vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border-light)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                Deployment Logs
                {logsData && (
                  <span className={`ml-2 text-[11px] font-normal px-2 py-0.5 rounded badge ${statusColors[logsData.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                    {logsData.status || 'unknown'}
                  </span>
                )}
              </h2>
              <button onClick={() => setShowLogs(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">{'\u2715'}</button>
            </div>
            <div className="flex-1 overflow-auto p-5">
              {logsLoading ? (
                <div className="flex items-center gap-2 py-8 justify-center">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-[var(--accent)]" />
                  <span className="text-[13px] text-[var(--text-tertiary)]">Loading logs...</span>
                </div>
              ) : logsData && logsData.error_message && logsData.logs.length === 0 ? (
                <div className="card border-red-500/20 bg-red-500/10">
                  <div className="card-body">
                    <p className="text-[13px] font-semibold text-red-500 mb-1">Error</p>
                    <p className="text-[12px] text-red-400 font-mono">{logsData.error_message}</p>
                  </div>
                </div>
              ) : logsData && logsData.logs.length > 0 ? (
                <div className="space-y-3">
                  {/* Error banner if deployment failed */}
                  {logsData.error_message && (
                    <div className="border border-red-500/20 bg-red-500/10 rounded-lg p-3">
                      <p className="text-[12px] font-semibold text-red-500 mb-1">Deployment Failed</p>
                      <p className="text-[11px] text-red-400 font-mono whitespace-pre-wrap break-all">{logsData.error_message}</p>
                    </div>
                  )}
                  {/* Log entries */}
                  <div className="font-mono text-[12px] bg-[#1a1a2e] text-[#e0e0e0] rounded-lg p-4 max-h-[60vh] overflow-y-auto whitespace-pre-wrap break-all leading-relaxed">
                    {logsData.logs.map((log, i) => (
                      <div key={i} className="flex gap-3 py-0.5">
                        <span className="text-[#888] shrink-0">{new Date(log.timestamp).toLocaleTimeString()}</span>
                        <span className={`shrink-0 font-semibold ${
                          log.level === 'error' ? 'text-red-400' : log.level === 'warn' ? 'text-yellow-400' : 'text-green-400'
                        }`}>
                          [{log.level}]
                        </span>
                        <span className="text-[#e0e0e0]">{log.message}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <p className="text-[13px] text-[var(--text-tertiary)] text-center py-6">No logs available yet. Logs will appear once the deployment starts.</p>
              )}
            </div>
            <div className="flex items-center justify-between px-5 py-3 border-t border-[var(--border-light)] shrink-0">
              <Link href={`/deployments?id=${showLogs}`} className="text-[12px] text-[var(--accent)] hover:underline">
                View full details {'\u2197'}
              </Link>
              <button onClick={() => setShowLogs(null)} className="btn btn-secondary text-[12px]">Close</button>
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
