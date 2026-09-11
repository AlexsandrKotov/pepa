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

  // Deployment configuration
  const [replicas, setReplicas] = useState(1);
  const [strategy, setStrategy] = useState<'rolling' | 'recreate'>('rolling');
  const [timeoutSeconds, setTimeoutSeconds] = useState(300);
  // Network
  const [servicePort, setServicePort] = useState(80);
  const [serviceType, setServiceType] = useState('ClusterIP');
  const [ingressEnabled, setIngressEnabled] = useState(false);
  const [ingressHost, setIngressHost] = useState('');
  // Health
  const [livenessPath, setLivenessPath] = useState('/healthz');
  const [readinessPath, setReadinessPath] = useState('/ready');
  // Container resources
  const [containerCpu, setContainerCpu] = useState('100m');
  const [containerMemory, setContainerMemory] = useState('128Mi');
  const [containerPort, setContainerPort] = useState(8080);
  // Environment variables
  const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>([]);
  // Collapsible sections
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({ deployment: true, network: false, health: false, advanced: false });

  const toggleSection = (key: string) => setOpenSections(prev => ({ ...prev, [key]: !prev[key] }));

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
      // Build common spec
      const envRecord: Record<string, string> = {};
      envVars.filter(e => e.key.trim()).forEach(e => { envRecord[e.key] = e.value; });
      const spec: Record<string, unknown> = {};

      // Only include containers for Docker source (matching deployments page pattern)
      if (sourceType === 'docker' && dockerImage.trim()) {
        spec.containers = [{
          name: 'main',
          image: `${dockerImage}:${dockerTag}`,
          cpu: containerCpu,
          memory: containerMemory,
          ports: [{ containerPort }],
          ...(Object.keys(envRecord).length > 0 ? { env: envRecord } : {}),
        }];
      }

      spec.service = { port: servicePort, type: serviceType };
      spec.health = { livenessPath, readinessPath, port: servicePort };
      if (ingressEnabled) {
        spec.ingress = { enabled: true, host: ingressHost };
      }

      if (sourceType === 'git') {
        // Use self-service API for Git-based deploys
        const blueprintConfig: Record<string, unknown> = {
          replicas, strategy, timeout_seconds: timeoutSeconds,
          service_port: servicePort, service_type: serviceType,
          container_port: containerPort, cpu: containerCpu, memory: containerMemory,
          liveness_path: livenessPath, readiness_path: readinessPath,
        };
        if (ingressEnabled) blueprintConfig.ingress = { enabled: true, host: ingressHost };
        if (Object.keys(envRecord).length > 0) blueprintConfig.env = envRecord;
        const result = await selfService.deploy({
          git_repo_url: gitRepoUrl,
          git_branch: gitBranch,
          environment_id: selectedEnv,
          blueprint_type: blueprintType,
          blueprint_config: blueprintConfig,
        });
        showToast(`Deployment started! ID: ${result.id.slice(0, 8)}`, 'success');
      } else {
        // Use deployments API for non-Git sources
        if (sourceType === 'helm') {
          spec.chart = { source_type: helmSourceType, chart_url: helmChartUrl, chart_name: helmChartName, chart_version: helmChartVersion || undefined };
        } else if (sourceType === 'yaml') {
          spec.values_yaml = rawYaml;
        }
        await deployments.create({
          gitlab_project_name: sourceType === 'docker' ? (dockerImage.split('/').pop() || 'app') : helmChartName || 'app',
          target_namespace: namespace,
          target_cluster_id: selectedClusterId || undefined,
          deploy_type: sourceType === 'helm' ? 'helm' : 'raw',
          replicas,
          strategy,
          spec,
          status: 'pending',
          timeout_seconds: timeoutSeconds,
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
  }, [sourceType, gitRepoUrl, gitBranch, dockerImage, dockerTag, helmChartUrl, helmChartName, helmChartVersion, helmSourceType, rawYaml, selectedEnv, selectedClusterId, namespace, blueprintType, replicas, strategy, timeoutSeconds, servicePort, serviceType, ingressEnabled, ingressHost, livenessPath, readinessPath, containerCpu, containerMemory, containerPort, envVars, showToast]);

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
    setReplicas(1);
    setStrategy('rolling');
    setTimeoutSeconds(300);
    setServicePort(80);
    setServiceType('ClusterIP');
    setIngressEnabled(false);
    setIngressHost('');
    setLivenessPath('/healthz');
    setReadinessPath('/ready');
    setContainerCpu('100m');
    setContainerMemory('128Mi');
    setContainerPort(8080);
    setEnvVars([]);
    setOpenSections({ deployment: true, network: false, health: false, advanced: false });
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
          <div className="space-y-4">
            {/* Source summary card */}
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

                {/* Blueprint type for Git */}
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

                {/* Namespace / Cluster for non-Git */}
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
              </div>
            </div>

            {/* Deployment Configuration — collapsible sections */}
            {/* Section: Deployment */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <button
                type="button"
                onClick={() => toggleSection('deployment')}
                className="card-header w-full flex items-center justify-between cursor-pointer hover:bg-[var(--bg)]/50 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-lg bg-blue-500/10 flex items-center justify-center">
                    <svg className="w-3.5 h-3.5 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                  </div>
                  <div className="text-left">
                    <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Deployment</h3>
                    <p className="text-[10px] text-[var(--text-tertiary)]">Replicas, strategy, resources</p>
                  </div>
                </div>
                <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${openSections.deployment ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
              </button>
              {openSections.deployment && (
                <div className="card-body pt-0 space-y-4">
                  <div className="grid grid-cols-3 gap-3">
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Replicas</label>
                      <div className="flex items-center gap-2">
                        <button type="button" onClick={() => setReplicas(Math.max(1, replicas - 1))} className="w-7 h-7 rounded-lg border border-[var(--border)] flex items-center justify-center text-[var(--text-secondary)] hover:bg-[var(--bg)] text-sm">−</button>
                        <input
                          type="number"
                          min={1}
                          max={20}
                          value={replicas}
                          onChange={e => setReplicas(Math.max(1, parseInt(e.target.value) || 1))}
                          className="input text-[13px] w-14 text-center"
                        />
                        <button type="button" onClick={() => setReplicas(Math.min(20, replicas + 1))} className="w-7 h-7 rounded-lg border border-[var(--border)] flex items-center justify-center text-[var(--text-secondary)] hover:bg-[var(--bg)] text-sm">+</button>
                      </div>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Strategy</label>
                      <select value={strategy} onChange={e => setStrategy(e.target.value as 'rolling' | 'recreate')} className="input text-[13px] w-full">
                        <option value="rolling">Rolling Update</option>
                        <option value="recreate">Recreate</option>
                      </select>
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Container Port</label>
                      <input
                        type="number"
                        value={containerPort}
                        onChange={e => setContainerPort(parseInt(e.target.value) || 8080)}
                        className="input text-[13px] w-full font-mono"
                        placeholder="8080"
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 flex items-center gap-1">
                        <svg className="w-3 h-3 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                        CPU Request
                      </label>
                      <input
                        value={containerCpu}
                        onChange={e => setContainerCpu(e.target.value)}
                        placeholder="100m"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 flex items-center gap-1">
                        <svg className="w-3 h-3 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" /></svg>
                        Memory Request
                      </label>
                      <input
                        value={containerMemory}
                        onChange={e => setContainerMemory(e.target.value)}
                        placeholder="128Mi"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Section: Network */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <button
                type="button"
                onClick={() => toggleSection('network')}
                className="card-header w-full flex items-center justify-between cursor-pointer hover:bg-[var(--bg)]/50 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                    <svg className="w-3.5 h-3.5 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
                    </svg>
                  </div>
                  <div className="text-left">
                    <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Network</h3>
                    <p className="text-[10px] text-[var(--text-tertiary)]">Service, ingress, ports</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {ingressEnabled && <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-600">Ingress ON</span>}
                  <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${openSections.network ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
                </div>
              </button>
              {openSections.network && (
                <div className="card-body pt-0 space-y-4">
                  <div className="grid grid-cols-3 gap-3">
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Service Port</label>
                      <input
                        type="number"
                        value={servicePort}
                        onChange={e => setServicePort(parseInt(e.target.value) || 80)}
                        className="input text-[13px] w-full font-mono"
                        placeholder="80"
                      />
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Service Type</label>
                      <select value={serviceType} onChange={e => setServiceType(e.target.value)} className="input text-[13px] w-full">
                        <option value="ClusterIP">ClusterIP</option>
                        <option value="NodePort">NodePort</option>
                        <option value="LoadBalancer">LoadBalancer</option>
                      </select>
                    </div>
                    <div className="flex items-end">
                      <label className="flex items-center gap-2 cursor-pointer select-none">
                        <input
                          type="checkbox"
                          checked={ingressEnabled}
                          onChange={e => setIngressEnabled(e.target.checked)}
                          className="w-4 h-4 rounded border-[var(--border)] accent-[var(--accent)]"
                        />
                        <span className="text-[12px] font-medium text-[var(--text-secondary)]">Enable Ingress</span>
                      </label>
                    </div>
                  </div>
                  {ingressEnabled && (
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Ingress Host</label>
                      <input
                        value={ingressHost}
                        onChange={e => setIngressHost(e.target.value)}
                        placeholder="app.example.com"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Section: Health Checks */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <button
                type="button"
                onClick={() => toggleSection('health')}
                className="card-header w-full flex items-center justify-between cursor-pointer hover:bg-[var(--bg)]/50 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-lg bg-amber-500/10 flex items-center justify-center">
                    <svg className="w-3.5 h-3.5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z" />
                    </svg>
                  </div>
                  <div className="text-left">
                    <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Health Checks</h3>
                    <p className="text-[10px] text-[var(--text-tertiary)]">Liveness and readiness probes</p>
                  </div>
                </div>
                <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${openSections.health ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
              </button>
              {openSections.health && (
                <div className="card-body pt-0 space-y-4">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 flex items-center gap-1">
                        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
                        Liveness Probe
                      </label>
                      <input
                        value={livenessPath}
                        onChange={e => setLivenessPath(e.target.value)}
                        placeholder="/healthz"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                    <div>
                      <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 flex items-center gap-1">
                        <span className="w-1.5 h-1.5 rounded-full bg-blue-500" />
                        Readiness Probe
                      </label>
                      <input
                        value={readinessPath}
                        onChange={e => setReadinessPath(e.target.value)}
                        placeholder="/ready"
                        className="input text-[13px] w-full font-mono"
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Section: Advanced */}
            <div className="card" style={{ borderRadius: '12px' }}>
              <button
                type="button"
                onClick={() => toggleSection('advanced')}
                className="card-header w-full flex items-center justify-between cursor-pointer hover:bg-[var(--bg)]/50 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-lg bg-violet-500/10 flex items-center justify-center">
                    <svg className="w-3.5 h-3.5 text-violet-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                      <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                    </svg>
                  </div>
                  <div className="text-left">
                    <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Advanced</h3>
                    <p className="text-[10px] text-[var(--text-tertiary)]">Timeout, environment variables</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {envVars.length > 0 && <span className="text-[9px] px-1.5 py-0.5 rounded-full bg-violet-500/10 text-violet-600">{envVars.length} env{envVars.length !== 1 ? 's' : ''}</span>}
                  <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${openSections.advanced ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
                </div>
              </button>
              {openSections.advanced && (
                <div className="card-body pt-0 space-y-4">
                  <div>
                    <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Deployment Timeout</label>
                    <div className="flex items-center gap-2">
                      <input
                        type="number"
                        min={30}
                        max={3600}
                        step={30}
                        value={timeoutSeconds}
                        onChange={e => setTimeoutSeconds(Math.max(30, parseInt(e.target.value) || 300))}
                        className="input text-[13px] w-28 font-mono"
                      />
                      <span className="text-[11px] text-[var(--text-tertiary)]">seconds</span>
                      <div className="flex gap-1 ml-2">
                        {[{ label: '2m', val: 120 }, { label: '5m', val: 300 }, { label: '10m', val: 600 }, { label: '30m', val: 1800 }].map(p => (
                          <button
                            key={p.val}
                            type="button"
                            onClick={() => setTimeoutSeconds(p.val)}
                            className={`px-2 py-1 rounded-md text-[10px] font-medium border transition-all ${
                              timeoutSeconds === p.val ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]' : 'border-[var(--border)] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                            }`}
                          >
                            {p.label}
                          </button>
                        ))}
                      </div>
                    </div>
                  </div>

                  {/* Environment Variables */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <label className="text-[12px] font-medium text-[var(--text-secondary)]">Environment Variables</label>
                      <button
                        type="button"
                        onClick={() => setEnvVars([...envVars, { key: '', value: '' }])}
                        className="text-[11px] px-2 py-1 rounded-md border border-[var(--border)] text-[var(--text-secondary)] hover:bg-[var(--bg)] flex items-center gap-1"
                      >
                        <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" /></svg>
                        Add Variable
                      </button>
                    </div>
                    {envVars.length === 0 ? (
                      <p className="text-[11px] text-[var(--text-tertiary)] py-2">No environment variables configured</p>
                    ) : (
                      <div className="space-y-2">
                        {envVars.map((env, idx) => (
                          <div key={idx} className="flex items-center gap-2">
                            <input
                              value={env.key}
                              onChange={e => {
                                const updated = [...envVars];
                                updated[idx] = { ...updated[idx], key: e.target.value };
                                setEnvVars(updated);
                              }}
                              placeholder="KEY"
                              className="input text-[12px] font-mono flex-1"
                            />
                            <input
                              value={env.value}
                              onChange={e => {
                                const updated = [...envVars];
                                updated[idx] = { ...updated[idx], value: e.target.value };
                                setEnvVars(updated);
                              }}
                              placeholder="value"
                              className="input text-[12px] font-mono flex-[2]"
                            />
                            <button
                              type="button"
                              onClick={() => setEnvVars(envVars.filter((_, i) => i !== idx))}
                              className="w-7 h-7 rounded-lg border border-[var(--border)] flex items-center justify-center text-[var(--text-tertiary)] hover:text-[var(--danger)] hover:border-[var(--danger)]/30 transition-colors"
                            >
                              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Navigation */}
            <div className="flex justify-between">
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
