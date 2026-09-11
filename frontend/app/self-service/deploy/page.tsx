'use client';
import { useState, useCallback, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { selfService, deployments, clusters, type Cluster, type Environment, type ProjectDetection, type SelfServiceDeployment } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import BrandIcon from '@/components/BrandIcon';

type SourceType = 'git' | 'docker' | 'helm' | 'yaml';

export default function SelfServiceDeployPage() {
  return (
    <PermissionGuard resource="self_service" action="create">
      <SelfServiceDeployContent />
    </PermissionGuard>
  );
}

function SelfServiceDeployContent() {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [sourceType, setSourceType] = useState<SourceType>('git');
  // Git source
  const [gitRepoUrl, setGitRepoUrl] = useState('');
  const [gitBranch, setGitBranch] = useState('main');
  // Docker source
  const [dockerImage, setDockerImage] = useState('');
  const [dockerTag, setDockerTag] = useState('latest');
  // Helm source
  const [helmChartUrl, setHelmChartUrl] = useState('');
  const [helmChartName, setHelmChartName] = useState('');
  const [helmChartVersion, setHelmChartVersion] = useState('');
  const [helmSourceType, setHelmSourceType] = useState('helm_http');
  // YAML source
  const [rawYaml, setRawYaml] = useState('');
  const [yamlFormat, setYamlFormat] = useState<'k8s' | 'compose'>('k8s');
  // Common
  const [selectedEnv, setSelectedEnv] = useState<string>('');
  const [selectedClusterId, setSelectedClusterId] = useState<string>('');
  const [namespace, setNamespace] = useState('default');
  const [blueprintType, setBlueprintType] = useState('auto');
  const [detection, setDetection] = useState<ProjectDetection | null>(null);
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [detecting, setDetecting] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [myDeployments, setMyDeployments] = useState<SelfServiceDeployment[]>([]);

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 4000);
  }, []);

  // Load environments and clusters
  useEffect(() => {
    selfService.environments()
      .then(res => setEnvironments(res.environments || []))
      .catch(() => {});
    clusters.list()
      .then(res => setClusterList(res.clusters || []))
      .catch(() => {});
  }, []);

  // Load my deployments
  useEffect(() => {
    selfService.list()
      .then(res => setMyDeployments(res.deployments || []))
      .catch(() => {});
  }, []);

  const handleDetect = useCallback(async () => {
    if (sourceType === 'git' && !gitRepoUrl.trim()) return;
    if (sourceType === 'docker' && !dockerImage.trim()) return;
    if (sourceType === 'helm' && !helmChartName.trim() && !helmChartUrl.trim()) return;
    if (sourceType === 'yaml' && !rawYaml.trim()) return;
    setDetecting(true);
    setError(null);
    try {
      if (sourceType === 'git') {
        const result = await selfService.detect(gitRepoUrl, gitBranch);
        setDetection(result);
        setBlueprintType(result.suggested_blueprint || 'docker_compose');
      } else {
        setDetection(null);
      }
      setStep(2);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Detection failed');
    } finally {
      setDetecting(false);
    }
  }, [sourceType, gitRepoUrl, gitBranch, dockerImage, helmChartName, helmChartUrl, rawYaml]);

  const handleDeploy = useCallback(async () => {
    if (!selectedEnv && !selectedClusterId) {
      setError('Please select a target environment or cluster');
      return;
    }
    setDeploying(true);
    setError(null);
    try {
      if (sourceType === 'git') {
        // Use self-service API for Git-based deploys
        const result = await selfService.deploy({
          git_repo_url: gitRepoUrl,
          git_branch: gitBranch,
          environment_id: selectedEnv,
          blueprint_type: blueprintType,
        });
        showToast(`Deployment started! ID: ${result.id.slice(0, 8)}`, 'success');
      } else {
        // Use deployments API for non-Git sources
        const spec: Record<string, unknown> = {};
        if (sourceType === 'docker') {
          spec.containers = [{ name: 'main', image: `${dockerImage}:${dockerTag}`, cpu: '100m', memory: '128Mi', ports: [{ containerPort: 8080 }] }];
        } else if (sourceType === 'helm') {
          spec.chart = { source_type: helmSourceType, chart_url: helmChartUrl, chart_name: helmChartName, chart_version: helmChartVersion || undefined };
        } else if (sourceType === 'yaml') {
          spec.values_yaml = rawYaml;
        }
        await deployments.create({
          gitlab_project_name: sourceType === 'docker' ? (dockerImage.split('/').pop() || 'app') : helmChartName || 'app',
          target_namespace: namespace,
          target_cluster_id: selectedClusterId || undefined,
          deploy_type: sourceType === 'helm' ? 'helm' : 'raw',
          replicas: 1,
          strategy: 'rolling',
          spec,
          status: 'pending',
          timeout_seconds: 300,
        });
        showToast('Deployment created!', 'success');
      }
      setStep(4);
      // Refresh deployments list
      const res = await selfService.list();
      setMyDeployments(res.deployments || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deployment failed');
      showToast('Deployment failed', 'error');
    } finally {
      setDeploying(false);
    }
  }, [sourceType, gitRepoUrl, gitBranch, dockerImage, dockerTag, helmChartUrl, helmChartName, helmChartVersion, helmSourceType, rawYaml, selectedEnv, selectedClusterId, namespace, blueprintType, showToast]);

  const handleCancel = useCallback(async (id: string) => {
    try {
      await selfService.cancel(id);
      showToast('Deployment cancelled', 'success');
      const res = await selfService.list();
      setMyDeployments(res.deployments || []);
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Cancel failed', 'error');
    }
  }, [showToast]);

  const resetForm = () => {
    setStep(1);
    setSourceType('git');
    setGitRepoUrl('');
    setGitBranch('main');
    setDockerImage('');
    setDockerTag('latest');
    setHelmChartUrl('');
    setHelmChartName('');
    setHelmChartVersion('');
    setRawYaml('');
    setDetection(null);
    setSelectedEnv('');
    setSelectedClusterId('');
    setError(null);
  };

  const sourceTypes: { value: SourceType; label: string; desc: string; icon: string }[] = [
    { value: 'git', label: 'Git Repository', desc: 'GitHub, GitLab, Bitbucket', icon: 'git' },
    { value: 'docker', label: 'Docker Image', desc: 'Any container image', icon: 'docker' },
    { value: 'helm', label: 'Helm Chart', desc: 'Chart repositories', icon: 'helm' },
    { value: 'yaml', label: 'Raw YAML', desc: 'K8s manifests or Compose', icon: 'kubernetes' },
  ];

  const canProceed = () => {
    switch (sourceType) {
      case 'git': return gitRepoUrl.trim().length > 0;
      case 'docker': return dockerImage.trim().length > 0;
      case 'helm': return helmChartUrl.trim().length > 0 || helmChartName.trim().length > 0;
      case 'yaml': return rawYaml.trim().length > 0;
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && (
          <div className={`fixed top-4 right-4 z-50 px-4 py-2.5 rounded-lg text-[13px] shadow-lg ${
            toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
          }`}>
            {toast.message}
          </div>
        )}

        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Deploy Application</h1>
            <p className="page-subtitle-modern">
              Deploy from any source — Git, Docker, Helm, or raw manifests
            </p>
          </div>
        </div>

        {/* Progress Steps */}
        <div className="flex items-center gap-2">
          {['Source', 'Configure', 'Environment', 'Deploy'].map((label, i) => (
            <div key={label} className="flex items-center gap-2">
              <div className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[12px] font-medium transition-colors ${
                step === i + 1 ? 'bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/20' :
                step > i + 1 ? 'bg-[var(--success)]/10 text-[var(--success)]' :
                'bg-[var(--bg)] text-[var(--text-tertiary)]'
              }`}>
                <span className={`w-5 h-5 rounded-full flex items-center justify-center text-[10px] ${
                  step > i + 1 ? 'bg-[var(--success)] text-white' :
                  step === i + 1 ? 'bg-[var(--accent)] text-white' :
                  'bg-[var(--border)] text-[var(--text-tertiary)]'
                }`}>
                  {step > i + 1 ? '✓' : i + 1}
                </span>
                {label}
              </div>
              {i < 3 && <div className="w-8 h-px bg-[var(--border)]" />}
            </div>
          ))}
        </div>

        {error && (
          <div className="card p-3 border-[var(--danger)]/20 bg-[var(--danger)]/5">
            <p className="text-[12px] text-[var(--danger)]">{error}</p>
          </div>
        )}

        {/* Step 1: Source */}
        {step === 1 && (
          <div className="space-y-4">
            {/* Source Type Selector */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <div className="card-header">
                <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Deployment Source</h3>
                <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Choose how you want to deploy your application</p>
              </div>
              <div className="card-body">
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
                  {sourceTypes.map(st => (
                    <button
                      key={st.value}
                      onClick={() => setSourceType(st.value)}
                      className={`p-3 rounded-lg border text-left transition-all ${
                        sourceType === st.value
                          ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                          : 'border-[var(--border)] hover:border-[var(--accent)]/50 hover:bg-[var(--bg)]'
                      }`}
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <BrandIcon name={st.icon} size={16} />
                        <span className="text-[12px] font-medium text-[var(--text-primary)]">{st.label}</span>
                      </div>
                      <p className="text-[10px] text-[var(--text-tertiary)]">{st.desc}</p>
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Source-specific form */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <div className="card-header">
                <h3 className="text-[14px] font-medium text-[var(--text-primary)] flex items-center gap-2">
                  <BrandIcon name={sourceTypes.find(s => s.value === sourceType)?.icon || 'git'} size={16} />
                  {sourceTypes.find(s => s.value === sourceType)?.label}
                </h3>
              </div>
              <div className="card-body space-y-4">
                {/* Git Repository */}
                {sourceType === 'git' && (
                  <>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Repository URL</label>
                      <input
                        value={gitRepoUrl}
                        onChange={e => setGitRepoUrl(e.target.value)}
                        placeholder="https://github.com/org/repo.git"
                        className="input text-[13px] w-full font-mono"
                      />
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                        Supports GitHub, GitLab, Bitbucket, and self-hosted Git repositories
                      </p>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Branch</label>
                      <input
                        value={gitBranch}
                        onChange={e => setGitBranch(e.target.value)}
                        placeholder="main"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                  </>
                )}

                {/* Docker Image */}
                {sourceType === 'docker' && (
                  <>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Image</label>
                      <input
                        value={dockerImage}
                        onChange={e => setDockerImage(e.target.value)}
                        placeholder="nginx, redis, registry.example.com/myapp"
                        className="input text-[13px] w-full font-mono"
                      />
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                        Docker image from Docker Hub, GHCR, or any registry
                      </p>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Tag</label>
                      <input
                        value={dockerTag}
                        onChange={e => setDockerTag(e.target.value)}
                        placeholder="latest"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                  </>
                )}

                {/* Helm Chart */}
                {sourceType === 'helm' && (
                  <>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Chart Source Type</label>
                      <select value={helmSourceType} onChange={e => setHelmSourceType(e.target.value)} className="input text-[13px] w-full">
                        <option value="helm_http">HTTP Repository</option>
                        <option value="helm_oci">OCI Registry</option>
                        <option value="helm_git">Git Repository</option>
                      </select>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Repository URL</label>
                      <input
                        value={helmChartUrl}
                        onChange={e => setHelmChartUrl(e.target.value)}
                        placeholder="https://charts.bitnami.com/bitnami"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Chart Name</label>
                        <input
                          value={helmChartName}
                          onChange={e => setHelmChartName(e.target.value)}
                          placeholder="postgresql"
                          className="input text-[13px] w-full"
                        />
                      </div>
                      <div>
                        <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Version</label>
                        <input
                          value={helmChartVersion}
                          onChange={e => setHelmChartVersion(e.target.value)}
                          placeholder="latest"
                          className="input text-[13px] w-full font-mono"
                        />
                      </div>
                    </div>
                  </>
                )}

                {/* Raw YAML */}
                {sourceType === 'yaml' && (
                  <>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Format</label>
                      <div className="flex gap-2">
                        <button
                          onClick={() => setYamlFormat('k8s')}
                          className={`px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-all ${
                            yamlFormat === 'k8s' ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]' : 'border-[var(--border)] text-[var(--text-secondary)]'
                          }`}
                        >
                          Kubernetes Manifests
                        </button>
                        <button
                          onClick={() => setYamlFormat('compose')}
                          className={`px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-all ${
                            yamlFormat === 'compose' ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]' : 'border-[var(--border)] text-[var(--text-secondary)]'
                          }`}
                        >
                          Docker Compose
                        </button>
                      </div>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">
                        {yamlFormat === 'k8s' ? 'Kubernetes YAML Manifests' : 'Docker Compose YAML'}
                      </label>
                      <textarea
                        value={rawYaml}
                        onChange={e => setRawYaml(e.target.value)}
                        placeholder={yamlFormat === 'k8s' 
                          ? 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-app\nspec:\n  ...'
                          : 'version: "3"\nservices:\n  web:\n    image: nginx\n    ports:\n      - "80:80"'}
                        className="input text-[12px] font-mono w-full"
                        rows={12}
                        spellCheck={false}
                      />
                    </div>
                  </>
                )}

                <div className="flex justify-end pt-2">
                  <button
                    onClick={handleDetect}
                    disabled={!canProceed() || detecting}
                    className="btn btn-primary btn-sm"
                  >
                    {detecting ? (
                      <>
                        <div className="loading-spinner w-3 h-3 mr-1.5" />
                        Analyzing...
                      </>
                    ) : (
                      <>
                        Next
                        <svg className="w-4 h-4 ml-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                        </svg>
                      </>
                    )}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Step 2: Configure */}
        {step === 2 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Project Configuration</h3>
            </div>
            <div className="card-body space-y-4">
              {detection && (
                <div className="p-3 rounded-lg bg-[var(--bg)] border border-[var(--border)]">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-[12px] font-medium text-[var(--text-primary)]">Detected:</span>
                    <span className="text-[11px] px-2 py-0.5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)] capitalize">
                      {detection.detected_type}
                    </span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      ({Math.round(detection.confidence * 100)}% confidence)
                    </span>
                  </div>
                  {detection.indicators.length > 0 && (
                    <ul className="text-[11px] text-[var(--text-tertiary)] space-y-0.5">
                      {detection.indicators.map((ind, i) => (
                        <li key={i}>• {ind}</li>
                      ))}
                    </ul>
                  )}
                </div>
              )}

              {/* Source summary */}
              <div className="p-3 rounded-lg bg-[var(--bg)] border border-[var(--border)]">
                <div className="flex items-center gap-2 mb-1">
                  <BrandIcon name={sourceTypes.find(s => s.value === sourceType)?.icon || 'git'} size={14} />
                  <span className="text-[12px] font-medium text-[var(--text-primary)]">Source: {sourceTypes.find(s => s.value === sourceType)?.label}</span>
                </div>
                <p className="text-[11px] font-mono text-[var(--text-tertiary)]">
                  {sourceType === 'git' && `${gitRepoUrl} (${gitBranch})`}
                  {sourceType === 'docker' && `${dockerImage}:${dockerTag}`}
                  {sourceType === 'helm' && `${helmChartName || helmChartUrl}${helmChartVersion ? ` v${helmChartVersion}` : ''}`}
                  {sourceType === 'yaml' && `${yamlFormat === 'k8s' ? 'Kubernetes' : 'Compose'} manifest (${rawYaml.length} chars)`}
                </p>
              </div>

              {sourceType === 'git' && (
                <div>
                  <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Blueprint Type</label>
                  <select
                    value={blueprintType}
                    onChange={e => setBlueprintType(e.target.value)}
                    className="select text-[13px] w-full"
                  >
                    <option value="auto">Auto-detect</option>
                    <option value="docker_compose">Docker Compose</option>
                    <option value="helm">Helm Chart</option>
                    <option value="raw_k8s">Raw Kubernetes Manifests</option>
                  </select>
                </div>
              )}

              {(sourceType === 'docker' || sourceType === 'helm' || sourceType === 'yaml') && (
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Namespace</label>
                    <input
                      value={namespace}
                      onChange={e => setNamespace(e.target.value)}
                      placeholder="default"
                      className="input text-[13px] w-full"
                    />
                  </div>
                  <div>
                    <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Target Cluster</label>
                    <select value={selectedClusterId} onChange={e => setSelectedClusterId(e.target.value)} className="input text-[13px] w-full">
                      <option value="">Select cluster...</option>
                      {clusterList.map(c => <option key={c.id} value={c.id}>{c.name} ({c.environment})</option>)}
                    </select>
                  </div>
                </div>
              )}

              <div className="flex justify-between pt-2">
                <button onClick={() => setStep(1)} className="btn btn-secondary btn-sm">
                  <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                  </svg>
                  Back
                </button>
                <button onClick={() => setStep(3)} className="btn btn-primary btn-sm">
                  Next
                  <svg className="w-4 h-4 ml-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Step 3: Environment */}
        {step === 3 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Target Environment</h3>
            </div>
            <div className="card-body space-y-4">
              {/* Cluster selection for non-Git sources */}
              {(sourceType === 'docker' || sourceType === 'helm' || sourceType === 'yaml') && (
                <div className="mb-4">
                  <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-2 block">Target Cluster</label>
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {clusterList.map(cluster => (
                      <button
                        key={cluster.id}
                        onClick={() => setSelectedClusterId(cluster.id)}
                        className={`p-3 rounded-lg border text-left transition-all ${
                          selectedClusterId === cluster.id
                            ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                            : 'border-[var(--border)] hover:border-[var(--accent)]/50 hover:bg-[var(--bg)]'
                        }`}
                      >
                        <div className="flex items-center gap-2 mb-1">
                          <span className={`w-3 h-3 rounded-full ${cluster.status === 'connected' ? 'bg-[var(--success)]' : 'bg-[var(--text-tertiary)]'}`} />
                          <span className="text-[13px] font-medium text-[var(--text-primary)]">{cluster.name}</span>
                        </div>
                        {cluster.environment && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)]">{cluster.environment}</span>
                        )}
                      </button>
                    ))}
                  </div>
                  {clusterList.length === 0 && (
                    <p className="text-[11px] text-[var(--text-tertiary)] text-center py-4">No clusters configured</p>
                  )}
                </div>
              )}

              {/* Environment selection for Git source */}
              {sourceType === 'git' && (
                <>
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {environments.map(env => (
                      <button
                        key={env.id}
                        onClick={() => setSelectedEnv(env.id)}
                        className={`p-3 rounded-lg border text-left transition-all ${
                          selectedEnv === env.id
                            ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                            : 'border-[var(--border)] hover:border-[var(--accent)]/50 hover:bg-[var(--bg)]'
                        }`}
                      >
                        <div className="flex items-center gap-2 mb-1">
                          <span className="w-3 h-3 rounded-full" style={{ backgroundColor: env.color || '#6B7280' }} />
                          <span className="text-[13px] font-medium text-[var(--text-primary)]">{env.name}</span>
                        </div>
                        {env.description && (
                          <p className="text-[11px] text-[var(--text-tertiary)] line-clamp-2">{env.description}</p>
                        )}
                        <div className="flex items-center gap-2 mt-1.5">
                          {env.type && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)] capitalize">{env.type}</span>
                          )}
                          {env.cluster && (
                            <span className="text-[10px] font-mono text-[var(--text-tertiary)]">{env.cluster}</span>
                          )}
                        </div>
                      </button>
                    ))}
                  </div>

                  {environments.length === 0 && (
                    <div className="text-center py-6">
                      <p className="text-[12px] text-[var(--text-tertiary)]">No environments available</p>
                    </div>
                  )}
                </>
              )}

              <div className="flex justify-between pt-2">
                <button onClick={() => setStep(2)} className="btn btn-secondary btn-sm">
                  <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                  </svg>
                  Back
                </button>
                <button
                  onClick={handleDeploy}
                  disabled={(!selectedEnv && !selectedClusterId) || deploying}
                  className="btn btn-primary btn-sm"
                >
                  {deploying ? (
                    <>
                      <div className="loading-spinner w-3 h-3 mr-1.5" />
                      Deploying...
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                      </svg>
                      Deploy
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Step 4: Result */}
        {step === 4 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-body text-center py-8">
              <div className="w-16 h-16 rounded-full bg-[var(--success)]/10 flex items-center justify-center mx-auto mb-4">
                <svg className="w-8 h-8 text-[var(--success)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <h3 className="text-[16px] font-semibold text-[var(--text-primary)] mb-1">Deployment Started!</h3>
              <p className="text-[12px] text-[var(--text-tertiary)] mb-4">
                Your application is being deployed to the selected target
              </p>
              <div className="flex items-center justify-center gap-3">
                <button onClick={() => router.push('/deployments')} className="btn btn-primary btn-sm">
                  View Deployments
                </button>
                <button onClick={resetForm} className="btn btn-secondary btn-sm">
                  Deploy Another
                </button>
              </div>
            </div>
          </div>
        )}

        {/* My Deployments */}
        {myDeployments.length > 0 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">My Deployments</h3>
            </div>
            <div className="card-body pt-0">
              <div className="space-y-2">
                {myDeployments.slice(0, 5).map(dep => (
                  <div key={dep.id} className="flex items-center gap-3 p-2.5 rounded-lg bg-[var(--bg)] border border-[var(--border-light)]">
                    <div className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                      dep.status === 'deployed' ? 'bg-[var(--success)]' :
                      dep.status === 'failed' ? 'bg-[var(--danger)]' :
                      dep.status === 'cancelled' ? 'bg-[var(--text-tertiary)]' :
                      'bg-[var(--warning)] animate-pulse'
                    }`} />
                    <div className="flex-1 min-w-0">
                      <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">
                        {dep.git_repo_url ? dep.git_repo_url.split('/').pop()?.replace('.git', '') : dep.blueprint_type || 'deployment'}
                      </p>
                      <p className="text-[11px] text-[var(--text-tertiary)]">
                        {dep.git_branch || dep.blueprint_type} — {new Date(dep.created_at).toLocaleString()}
                      </p>
                    </div>
                    <span className={`text-[10px] px-2 py-0.5 rounded-full capitalize ${
                      dep.status === 'deployed' ? 'bg-[var(--success)]/10 text-[var(--success)]' :
                      dep.status === 'failed' ? 'bg-[var(--danger)]/10 text-[var(--danger)]' :
                      dep.status === 'cancelled' ? 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]' :
                      'bg-[var(--warning)]/10 text-[var(--warning)]'
                    }`}>
                      {dep.status}
                    </span>
                    {(dep.status === 'pending' || dep.status === 'deploying') && (
                      <button
                        onClick={() => handleCancel(dep.id)}
                        className="text-[11px] text-[var(--danger)] hover:underline"
                      >
                        Cancel
                      </button>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
