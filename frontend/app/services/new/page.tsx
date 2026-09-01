'use client';

import React, { useState, useEffect, useMemo, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { services, serviceTemplates, clusters, helmRepositories, integrations, blueprints as blueprintsAPI, dockerHosts, dockerServices, connections, type ServiceTemplate, type Cluster, type HelmRepository, type HelmChart as HelmChartType, type HelmChartVersion, type ServiceBlueprint, type DockerHost, type Connection } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';
import GitRepoPicker from '@/components/GitRepoPicker';
import BrandIcon from '@/components/BrandIcon';
import { friendlyError } from '@/lib/errors';

// Error boundary to catch rendering errors
interface EBState { error: Error | null }
class NewServiceErrorBoundary extends React.Component<{ children: React.ReactNode }, EBState> {
  constructor(props: { children: React.ReactNode }) {
    super(props);
    this.state = { error: null };
  }
  static getDerivedStateFromError(error: Error): EBState {
    return { error };
  }
  componentDidCatch(error: Error) {
    console.error('[PEPA] Deploy page error:', error.message, error.stack);
  }
  render() {
    if (this.state.error) {
      const e = this.state.error;
      return (
        <div className="-mx-6 -my-6 min-h-full page-mesh-bg flex flex-col items-center justify-center py-20 text-center page-animate">
          <div className="w-16 h-16 rounded-full bg-red-500/10 flex items-center justify-center mb-4">
            <svg className="w-8 h-8 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
            </svg>
          </div>
          <h2 className="text-[16px] font-semibold text-[var(--text-primary)] mb-1">Deploy page error</h2>
          <p className="text-[13px] text-[var(--text-secondary)] mb-1 max-w-md">{e.message}</p>
          <p className="text-[11px] text-[var(--text-tertiary)] mb-4 font-mono">{e.stack?.split('\n').slice(0, 3).join(' | ')}</p>
          <div className="flex items-center gap-3 mt-4">
            <button onClick={() => this.setState({ error: null })} className="btn btn-primary">Try again</button>
            <Link href="/" className="btn btn-secondary">Go to Dashboard</Link>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

type Step = 'template' | 'configure' | 'deploy' | 'review';

export default function NewServicePage() {
  return (
    <NewServiceErrorBoundary>
      <Suspense fallback={<div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6"><h1 className="page-title-modern">Create Service</h1><div className="card card-body text-center py-12 page-animate" style={{ borderRadius: '12px' }}><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div></div></div>}>
        <NewServiceForm />
      </Suspense>
    </NewServiceErrorBoundary>
  );
}

function NewServiceForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const preselectedTemplate = searchParams.get('template');

  const [step, setStep] = useState<Step>('template');
  const [templates, setTemplates] = useState<ServiceTemplate[]>([]);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState<ServiceTemplate | null>(null);
  const [selectedBlueprint, setSelectedBlueprint] = useState<ServiceBlueprint | null>(null);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [templateCategory, setTemplateCategory] = useState('all');
  const [templateSearch, setTemplateSearch] = useState('');
  const [blueprints, setBlueprints] = useState<ServiceBlueprint[]>([]);

  // Helm repo chart picker state
  const [helmRepoList, setHelmRepoList] = useState<HelmRepository[]>([]);
  const [helmCharts, setHelmCharts] = useState<(HelmChartType & { repoId: string; repoName: string })[]>([]);
  const [helmChartVersions, setHelmChartVersions] = useState<HelmChartVersion[]>([]);
  const [selectedHelmRepoChart, setSelectedHelmRepoChart] = useState(''); // "repoId:chartName"
  const [loadingHelmCharts, setLoadingHelmCharts] = useState(false);
  const [helmRepoErrors, setHelmRepoErrors] = useState<Record<string, string>>({});
  const [helmInputMode, setHelmInputMode] = useState<'picker' | 'manual'>('picker');

  // GitLab integration picker state
  const [gitlabIntegrations, setGitlabIntegrations] = useState<{ id: string; name: string; url: string }[]>([]);
  const [gitInputMode, setGitInputMode] = useState<'picker' | 'manual'>('picker');

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [namespace, setNamespace] = useState('default');
  const [strategy, setStrategy] = useState('rolling');
  const [selectedClusters, setSelectedClusters] = useState<string[]>([]);
  const [gitlabUrl, setGitlabUrl] = useState('');
  const [helmUrl, setHelmUrl] = useState('');
  const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>([]);
  const [valuesYaml, setValuesYaml] = useState('');
  const [envMode, setEnvMode] = useState<'kv' | 'yaml'>('kv');
  const [cpu, setCpu] = useState('100m');
  const [memory, setMemory] = useState('128Mi');
  const [replicas, setReplicas] = useState(1);
  const [error, setError] = useState<{ message: string; hint?: string } | null>(null);
  const [savingBlueprint, setSavingBlueprint] = useState(false);
  const [blueprintSaved, setBlueprintSaved] = useState(false);

  // Docker Compose Import state
  const [composeYaml, setComposeYaml] = useState('');
  const [composeSource, setComposeSource] = useState<'yaml' | 'folder' | 'git'>('yaml');
  const [composeFolderPath, setComposeFolderPath] = useState('');
  const [composeGitUrl, setComposeGitUrl] = useState('');
  const [composeDeployTarget, setComposeDeployTarget] = useState<'local' | 'host'>('local');
  const [composeHostId, setComposeHostId] = useState('');
  const [dockerHostList, setDockerHostList] = useState<DockerHost[]>([]);

  useEffect(() => {
    loadAll();
    loadHelmRepos();
    loadGitlabIntegrations();
  }, []);

  const loadAll = async () => {
    try {
      const [bpData, clusterData, connData] = await Promise.all([
        blueprintsAPI.list().catch(() => ({ blueprints: [] })),
        clusters.list().catch(() => ({ clusters: [], total: 0 })),
        connections.list('kubernetes').catch(() => ({ connections: [], total: 0 })),
      ]);
      const allBp = bpData.blueprints || [];
      setBlueprints(allBp);

      // Also populate templates from system blueprints for backward compat
      const systemBp = allBp.filter((bp: ServiceBlueprint) => bp.is_system);
      const tmplList: ServiceTemplate[] = systemBp.map((bp: ServiceBlueprint) => ({
        id: bp.id,
        tenant_id: bp.tenant_id || '',
        name: bp.name,
        slug: bp.slug || bp.name.toLowerCase().replace(/\s+/g, '-'),
        description: bp.description || '',
        category: bp.category || 'general',
        icon: bp.icon,
        language: bp.language,
        framework: bp.framework,
        tags: bp.tags || [],
        helm_chart: bp.helm_chart,
        resource_defaults: bp.resource_defaults,
        default_values: bp.default_values as Record<string, string> | undefined,
        is_enabled: bp.is_enabled,
        is_system: bp.is_system,
        created_at: bp.created_at,
      }));
      setTemplates(tmplList);

      // Merge clusters from /clusters and Kubernetes connections
      const allClusters = [...(clusterData.clusters || [])];
      const k8sConnections = connData.connections || [];
      for (const conn of k8sConnections) {
        if (!allClusters.some(c => c.name === conn.name)) {
          allClusters.push({
            id: conn.id,
            tenant_id: '',
            name: conn.name,
            description: conn.description || '',
            environment: 'connection',
            api_server_url: '',
            flux_installed: false,
            status: conn.status || 'active',
            node_count: 0,
            kubernetes_version: '',
            is_active: true,
            created_at: '',
            updated_at: '',
          } as Cluster);
        }
      }
      setClusterList(allClusters);

      // Auto-select preselected template (by slug from system blueprints)
      if (preselectedTemplate) {
        const tmpl = tmplList.find((t: ServiceTemplate) => t.slug === preselectedTemplate);
        if (tmpl) {
          setSelectedTemplate(tmpl);
          setStep('configure');
          if (tmpl.resource_defaults) {
            const defaults = tmpl.resource_defaults as Record<string, unknown>;
            if (defaults.cpu) setCpu(defaults.cpu as string);
            if (defaults.memory) setMemory(defaults.memory as string);
            if (defaults.replicas) setReplicas(defaults.replicas as number);
          }
        }
      }
    } catch (err) {
      console.error('Failed to load data:', err);
    } finally {
      setLoading(false);
    }
  };

  const loadHelmRepos = async () => {
    try {
      const data = await helmRepositories.list().catch(() => ({ helm_repositories: [], total: 0 }));
      const repos = data.helm_repositories || [];
      setHelmRepoList(repos);
      // Load charts from all repos
      setLoadingHelmCharts(true);
      const allCharts: (HelmChartType & { repoId: string; repoName: string })[] = [];
      const errors: Record<string, string> = {};
      await Promise.all(repos.map(async (repo) => {
        try {
          const chartsData = await helmRepositories.listCharts(repo.id);
          const charts = (chartsData.charts || []).map(c => ({ ...c, repoId: repo.id, repoName: repo.name }));
          allCharts.push(...charts);
        } catch (err) {
          errors[repo.id] = err instanceof Error ? err.message : 'Failed to load charts';
        }
      }));
      setHelmCharts(allCharts);
      setHelmRepoErrors(errors);
    } catch {}
    setLoadingHelmCharts(false);
  };

  const loadGitlabIntegrations = async () => {
    try {
      const data = await integrations.list({ type: 'gitlab' }).catch(() => ({ integrations: [], total: 0 }));
      const glList = (data.integrations || []).map(i => ({ id: i.id, name: i.name, url: i.url }));
      setGitlabIntegrations(glList);
    } catch {}
  };

  const loadDockerHosts = async () => {
    try {
      const data = await dockerHosts.list().catch(() => ({ docker_hosts: [] }));
      setDockerHostList(data.docker_hosts || []);
      if ((data.docker_hosts || []).length > 0 && !composeHostId) {
        setComposeHostId(data.docker_hosts[0].id);
      }
    } catch {}
  };

  const isComposeImport = selectedTemplate?.slug === 'docker-compose-import';

  const handleHelmRepoChartChange = async (value: string) => {
    setSelectedHelmRepoChart(value);
    if (!value) { setHelmUrl(''); setHelmChartVersions([]); return; }
    const [repoId, chartName] = value.split(':');
    const chart = helmCharts.find(c => c.repoId === repoId && c.name === chartName);
    if (chart) {
      const repo = helmRepoList.find(r => r.id === repoId);
      // Build URL from repo + chart
      if (repo) {
        setHelmUrl(`${repo.url}/${chartName}`);
      }
    }
    // Load versions
    try {
      const versionsData = await helmRepositories.listChartVersions(repoId, chartName);
      setHelmChartVersions(versionsData.versions || []);
    } catch { setHelmChartVersions([]); }
  };

  const handleSelectTemplate = (tmpl: ServiceTemplate) => {
    setSelectedTemplate(tmpl);
    setSelectedBlueprint(null);
    setStep('configure');
    if (tmpl.resource_defaults) {
      const defaults = tmpl.resource_defaults as Record<string, unknown>;
      if (defaults.cpu) setCpu(defaults.cpu as string);
      if (defaults.memory) setMemory(defaults.memory as string);
      if (defaults.replicas) setReplicas(defaults.replicas as number);
    }
    // Pre-fill default_values as env vars
    if (tmpl.default_values && Object.keys(tmpl.default_values).length > 0) {
      setEnvVars(Object.entries(tmpl.default_values).map(([k, v]) => ({ key: k.toUpperCase().replace(/[.]/g, '_'), value: v })));
    }
    // Auto-fill Helm Chart URL from template
    if (tmpl.helm_chart) {
      if (tmpl.helm_chart.repo_url && tmpl.helm_chart.chart_name) {
        setHelmUrl(`${tmpl.helm_chart.repo_url}/${tmpl.helm_chart.chart_name}`);
      } else if (tmpl.helm_chart.image) {
        setHelmUrl(tmpl.helm_chart.image);
      }
    }
    // Auto-fill GitLab URL from template helm_chart.repo_url (if it looks like a git repo)
    if (tmpl.helm_chart?.repo_url && /git(lab|hub)/i.test(tmpl.helm_chart.repo_url)) {
      setGitlabUrl(tmpl.helm_chart.repo_url);
    }
    // Load Docker hosts for compose import template
    if (tmpl.slug === 'docker-compose-import') {
      loadDockerHosts();
    }
  };

  const handleSelectBlueprint = (bp: ServiceBlueprint) => {
    setSelectedBlueprint(bp);
    setSelectedTemplate(null);
    setStep('configure');
    setCpu(bp.cpu || '100m');
    setMemory(bp.memory || '128Mi');
    setReplicas(bp.replicas || 1);
    if (bp.values_yaml) {
      setEnvMode('yaml');
      setValuesYaml(bp.values_yaml);
    }
  };

  const handleForkBlueprint = async (bp: ServiceBlueprint) => {
    try {
      const forked = await blueprintsAPI.fork(bp.id, { name: `${bp.name} (custom)` });
      // Refresh blueprints list
      const res = await blueprintsAPI.list();
      setBlueprints(res.blueprints || []);
      // Select the forked blueprint
      handleSelectBlueprint(forked);
    } catch (err) {
      setError({ message: `Failed to fork blueprint: ${friendlyError(err)}` });
    }
  };

  // Category definitions
  const categories = [
    { key: 'all', label: 'All', icon: 'dashboard' },
    { key: 'backend', label: 'Backend', icon: 'cicd' },
    { key: 'frontend', label: 'Frontend', icon: 'discovery' },
    { key: 'data', label: 'Data', icon: 'storage' },
    { key: 'infrastructure', label: 'Infra', icon: 'kubernetes' },
    { key: 'messaging', label: 'Messaging', icon: 'slack' },
    { key: 'ml', label: 'ML', icon: 'ai' },
    { key: 'devops', label: 'DevOps', icon: 'gitlab' },
    { key: 'import', label: 'Import', icon: 'storage' },
    { key: 'blueprints', label: 'My Blueprints', icon: 'vault' },
  ];

  // Filtered templates
  const filteredTemplates = useMemo(() => {
    let list = templates;
    if (templateCategory !== 'all' && templateCategory !== 'blueprints') {
      list = list.filter(t => t.category === templateCategory);
    }
    if (templateSearch.trim()) {
      const q = templateSearch.toLowerCase();
      list = list.filter(t =>
        t.name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.tags?.some(tag => tag.toLowerCase().includes(q)) ||
        t.language?.toLowerCase().includes(q) ||
        t.framework?.toLowerCase().includes(q)
      );
    }
    return list;
  }, [templates, templateCategory, templateSearch]);

  const filteredBlueprints = useMemo(() => {
    // Only show user-created blueprints (not system)
    const userBp = blueprints.filter(bp => !bp.is_system);
    if (templateCategory !== 'blueprints' && templateCategory !== 'all') return [];
    if (!templateSearch.trim()) return userBp;
    const q = templateSearch.toLowerCase();
    return userBp.filter(bp =>
      bp.name.toLowerCase().includes(q) ||
      bp.description?.toLowerCase().includes(q) ||
      bp.category?.toLowerCase().includes(q)
    );
  }, [blueprints, templateCategory, templateSearch]);

  // System blueprints count (for category badge)
  const systemBlueprintCount = useMemo(() => blueprints.filter(bp => bp.is_system).length, [blueprints]);
  const userBlueprintCount = useMemo(() => blueprints.filter(bp => !bp.is_system).length, [blueprints]);

  const addEnvVar = () => setEnvVars([...envVars, { key: '', value: '' }]);
  const removeEnvVar = (idx: number) => setEnvVars(envVars.filter((_, i) => i !== idx));
  const updateEnvVar = (idx: number, field: 'key' | 'value', val: string) => {
    const updated = [...envVars];
    updated[idx][field] = val;
    setEnvVars(updated);
  };

  const toggleCluster = (id: string) => {
    setSelectedClusters(prev =>
      prev.includes(id) ? prev.filter(c => c !== id) : [...prev, id]
    );
  };

  // Auto-generate slug from name for the create request
  const generateSlug = (n: string): string =>
    n.toLowerCase().trim()
      .replace(/[^a-z0-9\s-]/g, '')
      .replace(/[\s_]+/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '');

  const handleCreate = async () => {
    setCreating(true);
    setError(null);
    try {
      const envVarsObj: Record<string, string> = {};

      // If YAML mode, parse YAML into flat key-value pairs
      if (envMode === 'yaml' && valuesYaml.trim()) {
        try {
          const parsed = parseYamlToEnvVars(valuesYaml);
          Object.assign(envVarsObj, parsed);
        } catch (e) {
          setError({ message: 'Invalid YAML format', hint: 'Check your values.yaml syntax' });
          setCreating(false);
          return;
        }
      } else {
        envVars.filter(e => e.key).forEach(e => { envVarsObj[e.key] = e.value; });
      }

      // ── Docker Compose Import flow ──
      if (isComposeImport) {
        if (!name.trim()) {
          setError({ message: 'Service name is required' });
          setCreating(false);
          return;
        }
        if (composeSource === 'yaml' && !composeYaml.trim()) {
          setError({ message: 'Docker Compose YAML is required' });
          setCreating(false);
          return;
        }
        if (composeSource === 'folder' && !composeFolderPath.trim()) {
          setError({ message: 'Folder path is required' });
          setCreating(false);
          return;
        }
        if (composeSource === 'git' && !composeGitUrl.trim()) {
          setError({ message: 'Git repository URL is required' });
          setCreating(false);
          return;
        }

        const composeData = {
          name: name.trim(),
          ...(composeSource === 'folder'
            ? { folder_path: composeFolderPath.trim() }
            : composeSource === 'git'
              ? { git_url: composeGitUrl.trim() }
              : { compose_yaml: composeYaml }),
          env_vars: envVarsObj,
        };

        if (composeDeployTarget === 'local') {
          await dockerServices.deployLocal(composeData);
        } else {
          if (!composeHostId) {
            setError({ message: 'Please select a Docker host' });
            setCreating(false);
            return;
          }
          await dockerServices.create({ ...composeData, docker_host_id: composeHostId });
        }
        router.push('/docker-services');
        return;
      }

      // ── Standard service creation flow ──
      const metadata: Record<string, unknown> = {};
      if (envMode === 'yaml' && valuesYaml.trim()) {
        metadata.values_yaml = valuesYaml;
      }

      const svc = await services.create({
        template_slug: selectedTemplate?.slug || (selectedBlueprint ? 'custom-container' : undefined),
        name,
        slug: generateSlug(name),
        description: selectedBlueprint ? (description || selectedBlueprint.description) : description,
        namespace,
        deployment_strategy: strategy,
        language: selectedTemplate?.language || 'any',
        framework: selectedTemplate?.framework || 'none',
        gitlab_project_url: selectedBlueprint?.chart_url || gitlabUrl,
        helm_chart_url: helmUrl,
        resource_config: { cpu, memory, replicas },
        environment_variables: envVarsObj,
        target_clusters: selectedClusters,
        metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
      });

      router.push(`/services?id=${svc.id}`);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setCreating(false);
    }
  };

  const handleSaveAsBlueprint = async () => {
    if (!name.trim()) return;
    setSavingBlueprint(true);
    setError(null);
    try {
      const envVarsObj: Record<string, string> = {};
      if (envMode === 'yaml' && valuesYaml.trim()) {
        try {
          const parsed = parseYamlToEnvVars(valuesYaml);
          Object.assign(envVarsObj, parsed);
        } catch { /* ignore */ }
      } else {
        envVars.filter(e => e.key).forEach(e => { envVarsObj[e.key] = e.value; });
      }

      const image = selectedBlueprint?.image || helmUrl || selectedTemplate?.helm_chart?.image || '';
      const ports = selectedBlueprint?.ports?.length ? selectedBlueprint.ports : [8080];

      await blueprintsAPI.create({
        name: name.trim(),
        description: description || selectedTemplate?.description || '',
        source_type: image ? 'container' : (selectedBlueprint?.source_type || 'container'),
        image,
        chart_url: selectedBlueprint?.chart_url || '',
        chart_name: selectedBlueprint?.chart_name || '',
        chart_version: selectedBlueprint?.chart_version || '',
        namespace: namespace || 'default',
        cpu,
        memory,
        replicas,
        ports,
        category: selectedTemplate?.category || 'general',
        values_yaml: envMode === 'yaml' ? valuesYaml : Object.entries(envVarsObj).map(([k, v]) => `${k}=${v}`).join('\n'),
      });

      setBlueprintSaved(true);
      setTimeout(() => setBlueprintSaved(false), 3000);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setSavingBlueprint(false);
    }
  };

  // Parse YAML to flat env vars (e.g., database.host -> DATABASE_HOST)
  const parseYamlToEnvVars = (yaml: string): Record<string, string> => {
    const result: Record<string, string> = {};
    const lines = yaml.split('\n');
    const stack: { indent: number; prefix: string }[] = [];

    for (const line of lines) {
      if (!line.trim() || line.trim().startsWith('#')) continue;

      const indent = line.search(/\S/);
      const match = line.trim().match(/^([\w.-]+):\s*(.*)$/);
      if (!match) continue;

      const [, key, rawValue] = match;
      const value = rawValue.trim().replace(/^["']|["']$/g, '');

      // Pop stack to find parent
      while (stack.length > 0 && stack[stack.length - 1].indent >= indent) {
        stack.pop();
      }

      const prefix = stack.length > 0 ? stack[stack.length - 1].prefix + '.' : '';

      if (value === '' || value === '|' || value === '>') {
        // Nested object or multiline - push to stack
        stack.push({ indent, prefix: prefix + key });
      } else {
        // Leaf value - create env var
        const envKey = (prefix + key).replace(/[.-]/g, '_').toUpperCase();
        result[envKey] = value;
      }
    }

    return result;
  };

  const steps: { key: Step; label: string }[] = [
    { key: 'template', label: '1. Template' },
    { key: 'configure', label: '2. Configure' },
    { key: 'deploy', label: '3. Deploy' },
    { key: 'review', label: '4. Review' },
  ];

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg"><div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Create Service</h1>
        <div className="card card-body text-center py-12 page-animate" style={{ borderRadius: '12px' }}>
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading templates...</p>
        </div>
      </div></div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
    <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between page-animate">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Create Service</h1>
            <ConceptHelp term="service" />
          </div>
          <p className="page-subtitle-modern">Deploy a new service from a template</p>
        </div>
        <Link href="/services" className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
          ← Back to Services
        </Link>
      </div>

      {/* Intro for beginners */}
      <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4 page-animate-up page-delay-1">
        <p className="text-[13px] text-[var(--text-primary)]">
          <span className="font-medium">A service is anything you run and maintain:</span> a web app,
          an API, a background worker, or a library. Pick a template below — PEPA will scaffold the
          configuration for you, and you can change everything later.
        </p>
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-4">
          <p className="text-[13px] font-medium text-red-500 mb-1">Could not create the service</p>
          {error.hint && <p className="text-[13px] text-red-400 mb-1">{error.hint}</p>}
          <p className="text-[11px] text-red-500">{error.message}</p>
        </div>
      )}

      {/* Step indicator */}
      <div className="flex gap-2">
        {steps.map((s, i) => (
          <div
            key={s.key}
            className={`flex-1 py-2 px-3 rounded-lg text-center text-[12px] font-medium transition-colors ${
              step === s.key
                ? 'bg-[var(--accent)] text-white'
                : steps.findIndex(x => x.key === step) > i
                ? 'bg-emerald-500/15 text-emerald-600'
                : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
            }`}
          >
            {s.label}
          </div>
        ))}
      </div>

      {/* Step 1: Template Selection */}
      {step === 'template' && (
        <div className="space-y-4">
          {/* Search + Category filters */}
          <div className="card">
            <div className="card-body space-y-4">
              {/* Search */}
              <div className="relative">
                <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                <input
                  type="text"
                  value={templateSearch}
                  onChange={e => setTemplateSearch(e.target.value)}
                  placeholder="Search blueprints by name, language, tag..."
                  className="input pl-10 w-full"
                />
              </div>

              {/* Category tabs */}
              <div className="flex gap-1.5 flex-wrap">
                {categories.map(cat => {
                  const count = cat.key === 'all'
                    ? templates.length + userBlueprintCount
                    : cat.key === 'blueprints'
                    ? userBlueprintCount
                    : templates.filter(t => t.category === cat.key).length;
                  if (count === 0 && cat.key !== 'all' && cat.key !== 'blueprints') return null;
                  return (
                    <button
                      key={cat.key}
                      onClick={() => setTemplateCategory(cat.key)}
                      className={`px-3 py-1.5 rounded-lg text-[12px] font-medium transition-all flex items-center gap-1.5 ${
                        templateCategory === cat.key
                          ? 'bg-[var(--accent)] text-white'
                          : 'bg-[var(--border-light)] text-[var(--text-secondary)] hover:bg-[var(--border)]'
                      }`}
                    >
                      <BrandIcon name={cat.icon} size={14} />
                      <span>{cat.label}</span>
                      <span className={`text-[10px] px-1 rounded ${
                        templateCategory === cat.key ? 'bg-[var(--surface)]/20' : 'bg-[var(--border)]'
                      }`}>{count}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          {/* Blueprint cards (if blueprints category or all) */}
          {(templateCategory === 'blueprints' || templateCategory === 'all') && filteredBlueprints.length > 0 && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-[13px] font-medium text-[var(--text-primary)]">My Blueprints</h2>
                <span className="text-[11px] text-[var(--text-tertiary)]">Custom blueprints you created or forked</span>
              </div>
              <div className="card-body">
                <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                  {filteredBlueprints.map(bp => (
                    <button
                      key={bp.id}
                      onClick={() => router.push(`/pipeline-blueprints?edit=${bp.id}`)}
                      className={`p-4 rounded-lg border text-left transition-all hover:shadow-sm ${
                        selectedBlueprint?.id === bp.id
                          ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                          : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                      }`}
                    >
                      <div className="flex items-start justify-between">
                        <span className="text-[10px] px-1.5 py-0.5 bg-amber-500/15 text-amber-600 rounded font-medium">⭐ Blueprint</span>
                        {bp.source_type && (
                          <span className="text-[10px] px-1.5 py-0.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded">
                            <BrandIcon name={bp.source_type === 'container' ? 'docker' : bp.source_type === 'helm_git' ? 'git' : bp.source_type === 'helm_http' ? 'helm' : 'storage'} size={12} />
                          </span>
                        )}
                      </div>
                      <h3 className="text-[13px] font-medium text-[var(--text-primary)] mt-2">{bp.name}</h3>
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-1 line-clamp-2">{bp.description}</p>
                      <div className="flex items-center gap-2 mt-2 text-[10px] text-[var(--text-tertiary)]">
                        {bp.cpu && <span>CPU: {bp.cpu}</span>}
                        {bp.memory && <span>MEM: {bp.memory}</span>}
                        {bp.replicas > 0 && <span>×{bp.replicas}</span>}
                      </div>
                      {bp.values_yaml && (
                        <span className="text-[10px] text-[var(--accent)] mt-1 inline-block">Has values.yaml</span>
                      )}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* Template cards */}
          {(templateCategory !== 'blueprints') && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-[13px] font-medium text-[var(--text-primary)]">
                  {templateCategory === 'all' ? 'System Blueprints' : categories.find(c => c.key === templateCategory)?.label + ' Blueprints'}
                </h2>
                <span className="text-[11px] text-[var(--text-tertiary)]">{filteredTemplates.length} blueprint{filteredTemplates.length !== 1 ? 's' : ''} · Hover to fork</span>
              </div>
              <div className="card-body">
                {filteredTemplates.length === 0 ? (
                  <div className="text-center py-8">
                    <p className="text-[13px] text-[var(--text-tertiary)]">No templates match your search.</p>
                    <button onClick={() => { setTemplateSearch(''); setTemplateCategory('all'); }} className="text-[12px] text-[var(--accent)] hover:underline mt-1">Clear filters</button>
                  </div>
                ) : (
                  <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
                    {filteredTemplates.map(tmpl => {
                      // Find the corresponding system blueprint for fork
                      const sysBp = blueprints.find(b => b.is_system && b.slug === tmpl.slug);
                      return (
                      <div
                        key={tmpl.id}
                        onClick={() => handleSelectTemplate(tmpl)}
                        className={`p-4 rounded-lg border text-left transition-all hover:shadow-sm group relative cursor-pointer ${
                          selectedTemplate?.id === tmpl.id
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <div className="flex items-start justify-between">
                          <span className="text-[20px] leading-none"><BrandIcon name={tmpl.icon || (tmpl.category === 'backend' ? 'cicd' : tmpl.category === 'frontend' ? 'discovery' : tmpl.category === 'data' ? 'storage' : tmpl.category === 'infrastructure' ? 'kubernetes' : tmpl.category === 'messaging' ? 'slack' : tmpl.category === 'ml' ? 'ai' : tmpl.category === 'devops' ? 'gitlab' : 'storage')} size={20} /></span>
                          <div className="flex items-center gap-1">
                            <span className="text-[10px] px-1.5 py-0.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded capitalize">{tmpl.category}</span>
                            {sysBp && (
                              <button
                                onClick={e => { e.stopPropagation(); handleForkBlueprint(sysBp); }}
                                className="text-[10px] px-1.5 py-0.5 bg-[var(--accent-subtle)] text-[var(--accent)] rounded font-medium opacity-0 group-hover:opacity-100 transition-opacity hover:bg-[var(--accent)] hover:text-white"
                                title="Fork to customize"
                              >
                                Fork
                              </button>
                            )}
                          </div>
                        </div>
                        <h3 className="text-[13px] font-medium text-[var(--text-primary)] mt-2 group-hover:text-[var(--accent)] transition-colors">{tmpl.name}</h3>
                        <p className="text-[11px] text-[var(--text-tertiary)] mt-1 line-clamp-2">{tmpl.description}</p>
                        {/* Resource defaults badge */}
                        {tmpl.resource_defaults && (() => {
                          const rd = tmpl.resource_defaults as Record<string, string | number>;
                          return (
                            <div className="flex items-center gap-1.5 mt-2 text-[10px] text-[var(--text-tertiary)]">
                              {rd.cpu && <span className="bg-[var(--border-light)] px-1 rounded">CPU {String(rd.cpu)}</span>}
                              {rd.memory && <span className="bg-[var(--border-light)] px-1 rounded">MEM {String(rd.memory)}</span>}
                              {rd.replicas && <span className="bg-[var(--border-light)] px-1 rounded">×{String(rd.replicas)}</span>}
                            </div>
                          );
                        })()}
                        {/* Public chart / image */}
                        {tmpl.helm_chart && (
                          <div className="mt-2 flex items-center gap-1 text-[10px]">
                            {tmpl.helm_chart.chart_name ? (
                              <span className="bg-emerald-500/10 text-emerald-600 px-1.5 py-0.5 rounded font-medium" title={`Helm: ${tmpl.helm_chart.repo_url}/${tmpl.helm_chart.chart_name}`}>
                                📦 {tmpl.helm_chart.chart_name}{tmpl.helm_chart.chart_version ? ` v${tmpl.helm_chart.chart_version}` : ''}
                              </span>
                            ) : tmpl.helm_chart.image ? (
                              <span className="bg-blue-500/10 text-blue-500 px-1.5 py-0.5 rounded font-medium truncate max-w-[180px]" title={tmpl.helm_chart.image}>
                                🐳 {tmpl.helm_chart.image.split('/').pop()?.split(':')[0]}
                              </span>
                            ) : null}
                          </div>
                        )}
                        {/* Tags */}
                        <div className="flex gap-1 mt-2 flex-wrap">
                          {tmpl.tags?.slice(0, 3).map(tag => (
                            <span key={tag} className="text-[10px] px-1.5 py-0.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded">
                              {tag}
                            </span>
                          ))}
                          {(tmpl.tags?.length || 0) > 3 && (
                            <span className="text-[10px] text-[var(--text-tertiary)]">+{(tmpl.tags?.length || 0) - 3}</span>
                          )}
                        </div>
                        {/* Language/framework */}
                        {(tmpl.language || tmpl.framework) && (
                          <p className="text-[10px] text-[var(--text-tertiary)] mt-1.5">
                            {tmpl.language && <span className="font-medium">{tmpl.language}</span>}
                            {tmpl.language && tmpl.framework && <span> · </span>}
                            {tmpl.framework && tmpl.framework !== 'none' && <span>{tmpl.framework}</span>}
                          </p>
                        )}
                      </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Step 2: Configuration */}
      {step === 'configure' && (
        <div className="space-y-4">
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Service Details</h2>
              <span className="text-[11px] text-[var(--text-tertiary)]">{selectedBlueprint ? `Blueprint: ${selectedBlueprint.name}` : `Template: ${selectedTemplate?.name}`}</span>
            </div>
            <div className="card-body space-y-4">
              <div>
                <label className="label">Service Name *</label>
                <input type="text" value={name} onChange={e => setName(e.target.value)} className="input" placeholder="my-service" />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  {isComposeImport
                    ? 'Name for this Docker Compose stack.'
                    : 'Lowercase letters, digits and dashes. Used in URLs and Kubernetes resources.'}
                  {name && <span className="ml-1 text-green-600">Slug: <code className="bg-[var(--border-light)] px-1 rounded">{generateSlug(name)}</code></span>}
                </p>
              </div>

              {isComposeImport ? (
                <>
                  <div>
                    <label className="label">Description</label>
                    <textarea value={description} onChange={e => setDescription(e.target.value)} className="input" rows={2} placeholder="Brief description of this compose stack" />
                  </div>

                  {/* Compose Source */}
                  <div>
                    <label className="label">Compose Source</label>
                    <div className="grid grid-cols-3 gap-2">
                      <button
                        type="button"
                        onClick={() => setComposeSource('yaml')}
                        className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                          composeSource === 'yaml'
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <span className="text-lg">📝</span>
                        <div>
                          <p className="text-[12px] font-medium text-[var(--text-primary)]">Paste YAML</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">docker-compose.yml</p>
                        </div>
                      </button>
                      <button
                        type="button"
                        onClick={() => setComposeSource('folder')}
                        className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                          composeSource === 'folder'
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <span className="text-lg">📂</span>
                        <div>
                          <p className="text-[12px] font-medium text-[var(--text-primary)]">Local Folder</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">Server path</p>
                        </div>
                      </button>
                      <button
                        type="button"
                        onClick={() => setComposeSource('git')}
                        className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                          composeSource === 'git'
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <span className="text-lg">🔀</span>
                        <div>
                          <p className="text-[12px] font-medium text-[var(--text-primary)]">Git Repo</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">Clone & deploy</p>
                        </div>
                      </button>
                    </div>
                  </div>

                  {composeSource === 'folder' ? (
                    <div>
                      <label className="label">Folder Path on Server *</label>
                      <input value={composeFolderPath} onChange={e => setComposeFolderPath(e.target.value)} className="input font-mono text-[12px]" placeholder="/data/compose-projects/my-app" />
                      <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                        Folder containing <code className="bg-[var(--border-light)] px-1 rounded">docker-compose.yml</code>. Must be accessible from the PEPA api-server container.
                      </p>
                    </div>
                  ) : composeSource === 'git' ? (
                    <div>
                      <label className="label">Git Repository URL *</label>
                      <input value={composeGitUrl} onChange={e => setComposeGitUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://github.com/org/repo.git" />
                      <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                        The repo will be cloned and deployed. Must contain a <code className="bg-[var(--border-light)] px-1 rounded">docker-compose.yml</code> in the root.
                      </p>
                    </div>
                  ) : (
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <label className="label mb-0">docker-compose.yml *</label>
                        <label className="text-[11px] text-[var(--accent)] hover:underline cursor-pointer inline-flex items-center gap-1">
                          <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                          </svg>
                          Upload file
                          <input
                            type="file"
                            accept=".yaml,.yml"
                            className="hidden"
                            onChange={async (e) => {
                              const file = e.target.files?.[0];
                              if (!file) return;
                              const text = await file.text();
                              setComposeYaml(text);
                            }}
                          />
                        </label>
                      </div>
                      <textarea
                        value={composeYaml}
                        onChange={e => setComposeYaml(e.target.value)}
                        className="input font-mono text-[12px] w-full"
                        rows={10}
                        spellCheck={false}
                        placeholder={`version: '3.8'\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "80:80"`}
                      />
                    </div>
                  )}

                  {/* Deploy Target */}
                  <div>
                    <label className="label">Deploy Target</label>
                    <div className="grid grid-cols-2 gap-2">
                      <button
                        type="button"
                        onClick={() => setComposeDeployTarget('local')}
                        className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                          composeDeployTarget === 'local'
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <span className="text-lg">🐳</span>
                        <div>
                          <p className="text-[12px] font-medium text-[var(--text-primary)]">Local Docker</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">unix:///var/run/docker.sock</p>
                        </div>
                      </button>
                      <button
                        type="button"
                        onClick={() => setComposeDeployTarget('host')}
                        className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                          composeDeployTarget === 'host'
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <span className="text-lg">🖥️</span>
                        <div>
                          <p className="text-[12px] font-medium text-[var(--text-primary)]">Registered Host</p>
                          <p className="text-[10px] text-[var(--text-tertiary)]">Remote via TCP/SSH/TLS</p>
                        </div>
                      </button>
                    </div>
                  </div>

                  {composeDeployTarget === 'host' && (
                    <div>
                      <label className="label">Docker Host *</label>
                      <select value={composeHostId} onChange={e => setComposeHostId(e.target.value)} className="input">
                        <option value="">Select host...</option>
                        {dockerHostList.map(h => (
                          <option key={h.id} value={h.id}>{h.name} ({h.status})</option>
                        ))}
                      </select>
                      {dockerHostList.length === 0 && (
                        <p className="text-[11px] text-orange-500 mt-1">
                          No hosts configured. <Link href="/docker-hosts" className="underline">Add a Docker host</Link>
                        </p>
                      )}
                    </div>
                  )}
                </>
              ) : (
                <>
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <label className="label">Namespace</label>
                      <input type="text" value={namespace} onChange={e => setNamespace(e.target.value)} className="input" placeholder="default" />
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-1">The Kubernetes namespace where this service will live.</p>
                    </div>
                  </div>
                  <div>
                    <label className="label">Description</label>
                    <textarea value={description} onChange={e => setDescription(e.target.value)} className="input" rows={2} placeholder="Brief description of the service" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="label">Git Repository URL</label>
                  <div className="space-y-1.5">
                    <div className="flex gap-1.5">
                      <button
                        type="button"
                        onClick={() => setGitInputMode('picker')}
                        className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${gitInputMode === 'picker' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                      >
                        Browse
                      </button>
                      <button
                        type="button"
                        onClick={() => setGitInputMode('manual')}
                        className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${gitInputMode === 'manual' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                      >
                        Manual URL
                      </button>
                    </div>
                    {gitInputMode === 'picker' ? (
                      <GitRepoPicker
                        label=""
                        value={{ repo_url: gitlabUrl }}
                        onChange={(v) => setGitlabUrl(v.repo_url)}
                      />
                    ) : (
                      <input type="text" value={gitlabUrl} onChange={e => setGitlabUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://github.com/org/repo" />
                    )}
                  </div>
                  {selectedTemplate?.helm_chart?.repo_url && /git(lab|hub)/i.test(selectedTemplate?.helm_chart?.repo_url ?? '') ? (
                    <p className="text-[11px] text-green-600 mt-1 flex items-center gap-1">
                      📦 Pre-filled from template: <span className="font-medium">{selectedTemplate?.helm_chart?.repo_url}</span>
                      {selectedTemplate?.helm_chart?.docs_url && (
                        <a href={selectedTemplate?.helm_chart?.docs_url} target="_blank" rel="noopener noreferrer" className="text-[var(--accent)] hover:underline ml-1">(docs)</a>
                      )}
                    </p>
                  ) : (
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                      Where the source code lives (GitHub, GitLab, Bitbucket, etc.). Optional, but enables CI/CD features.
                      {gitlabIntegrations.length > 0 && <span className="text-green-600 ml-1">({gitlabIntegrations.length} connected)</span>}
                    </p>
                  )}
                </div>
                <div>
                  <label className="label">Helm Chart URL / Image</label>
                  {helmRepoList.length > 0 ? (
                    <div className="space-y-1.5">
                      <div className="flex gap-1.5">
                        <button
                          type="button"
                          onClick={() => setHelmInputMode('picker')}
                          className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${helmInputMode === 'picker' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                        >
                          📦 From repo
                        </button>
                        <button
                          type="button"
                          onClick={() => setHelmInputMode('manual')}
                          className={`px-2.5 py-1 rounded text-[11px] font-medium border transition-all ${helmInputMode === 'manual' ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)]'}`}
                        >
                          🔗 Manual URL
                        </button>
                      </div>
                      {helmInputMode === 'picker' ? (
                        <div className="space-y-1.5">
                          <select
                            value={selectedHelmRepoChart}
                            onChange={e => handleHelmRepoChartChange(e.target.value)}
                            className="input"
                          >
                            <option value="">Select a chart...</option>
                            {helmCharts.map(c => (
                              <option key={`${c.repoId}:${c.name}`} value={`${c.repoId}:${c.name}`}>
                                {c.repoName} / {c.name} {c.latest_version && `(v${String(c.latest_version).replace(/^v/, '')})`}
                              </option>
                            ))}
                          </select>
                          {helmChartVersions.length > 0 && (
                            <select
                              onChange={e => {
                                const [repoId, chartName] = selectedHelmRepoChart.split(':');
                                const repo = helmRepoList.find(r => r.id === repoId);
                                if (repo) setHelmUrl(`${repo.url}/${chartName}:${e.target.value}`);
                              }}
                              className="input text-[12px]"
                            >
                              <option value="">Select version...</option>
                              {helmChartVersions.map(v => (
                                <option key={v.version} value={v.version}>v{String(v.version).replace(/^v/, '')}{v.app_version ? ` (app: ${v.app_version})` : ''}</option>
                              ))}
                            </select>
                          )}
                        </div>
                      ) : (
                        <input type="text" value={helmUrl} onChange={e => setHelmUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://charts.bitnami.com/bitnami/postgresql or bitnami/postgresql:18" />
                      )}
                    </div>
                  ) : (
                    <input type="text" value={helmUrl} onChange={e => setHelmUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://charts.bitnami.com/bitnami/postgresql or bitnami/postgresql:18" />
                  )}
                  {selectedTemplate?.helm_chart ? (
                    <p className="text-[11px] text-green-600 mt-1 flex items-center gap-1">
                      {selectedTemplate?.helm_chart?.chart_name ? (
                        <>📦 Pre-filled from template: <span className="font-medium">{selectedTemplate?.helm_chart?.chart_name}</span>{selectedTemplate?.helm_chart?.chart_version && <span> v{selectedTemplate?.helm_chart?.chart_version}</span>}</>
                      ) : selectedTemplate?.helm_chart?.image ? (
                        <>🐳 Pre-filled from template: <span className="font-medium">{selectedTemplate?.helm_chart?.image}</span></>
                      ) : null}
                      {selectedTemplate?.helm_chart?.docs_url && (
                        <a href={selectedTemplate?.helm_chart?.docs_url} target="_blank" rel="noopener noreferrer" className="text-[var(--accent)] hover:underline ml-1">(docs)</a>
                      )}
                    </p>
                  ) : Object.keys(helmRepoErrors).length > 0 ? (
                    <div className="mt-2 p-2 bg-red-500/10 border border-red-500/30 rounded-lg">
                      <p className="text-red-500 text-[11px] font-medium">Some repositories failed to load:</p>
                      {Object.entries(helmRepoErrors).map(([repoId, errMsg]) => {
                        const repo = helmRepoList.find(r => r.id === repoId);
                        return (
                          <p key={repoId} className="text-red-400 text-[11px] mt-1 font-mono">
                            {repo?.name || repoId}: {errMsg}
                          </p>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                      {helmRepoList.length > 0
                        ? 'Pick a chart from your repositories or enter a URL manually.'
                        : 'Public Helm chart URL or Docker image. Add Helm repos in the Helm Repositories page to pick from a list.'}
                    </p>
                  )}
                </div>
              </div>
                </>
              )}
            </div>
          </div>

          {!isComposeImport && (
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Resources</h2>
            </div>
            <div className="card-body">
              <div className="grid grid-cols-3 gap-4">
                <div>
                  <label className="label">CPU</label>
                  <input type="text" value={cpu} onChange={e => setCpu(e.target.value)} className="input" placeholder="100m" />
                </div>
                <div>
                  <label className="label">Memory</label>
                  <input type="text" value={memory} onChange={e => setMemory(e.target.value)} className="input" placeholder="128Mi" />
                </div>
                <div>
                  <label className="label">Replicas</label>
                  <input type="number" value={replicas} onChange={e => setReplicas(Number(e.target.value))} className="input" min={1} />
                </div>
              </div>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-3">
                How much capacity one instance gets. 100m = 0.1 CPU core; 128Mi = 128 MiB RAM.
                Replicas is the number of parallel instances. Defaults are fine for a start.
              </p>
            </div>
          </div>
          )}

          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Environment Variables</h2>
            </div>
            <div className="card-body space-y-3">
              {/* Mode toggle - prominent */}
              <div className="flex gap-2">
                <button
                  onClick={() => setEnvMode('kv')}
                  className={`px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-all ${
                    envMode === 'kv'
                      ? 'bg-[var(--accent)] text-white border-[var(--accent)]'
                      : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:border-[var(--accent)] hover:text-[var(--accent)]'
                  }`}
                >
                  Key-Value Pairs
                </button>
                <button
                  onClick={() => setEnvMode('yaml')}
                  className={`px-3 py-1.5 rounded-lg text-[12px] font-medium border transition-all ${
                    envMode === 'yaml'
                      ? 'bg-[var(--accent)] text-white border-[var(--accent)]'
                      : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:border-[var(--accent)] hover:text-[var(--accent)]'
                  }`}
                >
                  Paste values.yaml
                </button>
              </div>

              {envMode === 'kv' ? (
                <>
                  <div className="flex justify-end">
                    <button onClick={addEnvVar} className="text-[11px] text-[var(--accent)] hover:underline">+ Add variable</button>
                  </div>
                  {envVars.length === 0 && (
                    <p className="text-[12px] text-[var(--text-tertiary)]">No environment variables. Click &quot;+ Add variable&quot; to add one.</p>
                  )}
                  {envVars.map((env, idx) => (
                    <div key={idx} className="flex gap-2 items-center">
                      <input type="text" value={env.key} onChange={e => updateEnvVar(idx, 'key', e.target.value)} className="input flex-1" placeholder="KEY" />
                      <input type="text" value={env.value} onChange={e => updateEnvVar(idx, 'value', e.target.value)} className="input flex-1" placeholder="value" />
                      <button onClick={() => removeEnvVar(idx)} className="text-red-500 text-[12px] hover:text-red-400">✕</button>
                    </div>
                  ))}
                </>
              ) : (
                <>
                  <div className="flex items-center justify-between">
                    <p className="text-[12px] text-[var(--text-secondary)]">
                      Paste a <code className="text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded font-mono">values.yaml</code> file content or upload a file.
                      Nested keys are converted to env vars: <code className="text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded font-mono">database.host</code> → <code className="text-[11px] bg-[var(--border-light)] px-1.5 py-0.5 rounded font-mono">DATABASE_HOST</code>
                    </p>
                  </div>

                  {/* File upload + import buttons */}
                  <div className="flex gap-2">
                    <label className="px-3 py-1.5 rounded-lg text-[12px] font-medium border border-[var(--border)] bg-[var(--surface)] text-[var(--text-secondary)] hover:border-[var(--accent)] hover:text-[var(--accent)] cursor-pointer transition-all inline-flex items-center gap-1.5">
                      <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
                      </svg>
                      Upload .yaml file
                      <input
                        type="file"
                        accept=".yaml,.yml"
                        className="hidden"
                        onChange={async (e) => {
                          const file = e.target.files?.[0];
                          if (!file) return;
                          const text = await file.text();
                          setValuesYaml(text);
                        }}
                      />
                    </label>
                    {envVars.filter(e => e.key).length > 0 && !valuesYaml.trim() && (
                      <button
                        onClick={() => {
                          const yamlLines = envVars.filter(e => e.key).map(e => {
                            const key = e.key.toLowerCase().replace(/_/g, '.');
                            return `${key}: "${e.value}"`;
                          });
                          setValuesYaml(yamlLines.join('\n'));
                        }}
                        className="px-3 py-1.5 rounded-lg text-[12px] font-medium border border-[var(--border)] bg-[var(--surface)] text-[var(--text-secondary)] hover:border-[var(--accent)] hover:text-[var(--accent)] transition-all"
                      >
                        Import from Key-Value →
                      </button>
                    )}
                  </div>

                  <textarea
                    value={valuesYaml}
                    onChange={e => setValuesYaml(e.target.value)}
                    className="input font-mono text-[12px] w-full"
                    rows={14}
                    spellCheck={false}
                    placeholder={`# Paste your values.yaml here
# Example:
replicaCount: 2

image:
  repository: nginx
  tag: "1.25"

database:
  host: postgres.default.svc
  port: "5432"
  name: myapp

env:
  LOG_LEVEL: info
  CACHE_TTL: "300"`}
                  />
                  {valuesYaml.trim() && (
                    <div className="bg-[var(--accent-subtle)] border border-blue-500/20 rounded-lg p-3 mt-1">
                      <p className="text-[10px] text-[var(--accent)] uppercase tracking-wider font-semibold mb-1.5">Parsed Environment Variables ({Object.keys(parseYamlToEnvVars(valuesYaml)).length})</p>
                      <div className="space-y-0.5 max-h-[150px] overflow-auto">
                        {Object.entries(parseYamlToEnvVars(valuesYaml)).map(([key, value]) => (
                          <p key={key} className="font-mono text-[11px] text-[var(--text-secondary)]">
                            <span className="text-[var(--accent)] font-medium">{key}</span>=<span className="text-[var(--text-secondary)]">{value}</span>
                          </p>
                        ))}
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
          </div>

          <div className="flex gap-3 justify-between">
            <div>
              {!isComposeImport && (
                <button
                  onClick={handleSaveAsBlueprint}
                  disabled={!name || savingBlueprint}
                  className="btn btn-secondary text-[12px]"
                  title="Save current configuration as a reusable blueprint"
                >
                  {savingBlueprint ? 'Saving...' : blueprintSaved ? '✓ Saved!' : '💾 Save as Blueprint'}
                </button>
              )}
            </div>
            <div className="flex gap-3">
              <button onClick={() => setStep('template')} className="btn btn-secondary">← Back</button>
              {isComposeImport ? (
                <button
                  onClick={handleCreate}
                  disabled={!name || creating || (composeSource === 'yaml' && !composeYaml.trim()) || (composeSource === 'folder' && !composeFolderPath.trim()) || (composeSource === 'git' && !composeGitUrl.trim())}
                  className="btn btn-primary"
                >
                  {creating ? 'Deploying...' : composeDeployTarget === 'local' ? '🐳 Deploy Locally' : 'Deploy to Host'}
                </button>
              ) : (
                <button
                  onClick={() => setStep('deploy')}
                  disabled={!name}
                  className="btn btn-primary"
                >
                  Next: Deploy →
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Step 3: Deploy */}
      {step === 'deploy' && (
        <div className="space-y-4">
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Deployment Settings</h2>
            </div>
            <div className="card-body space-y-4">
              <div>
                <label className="label">Deployment Strategy</label>
                <select value={strategy} onChange={e => setStrategy(e.target.value)} className="input">
                  <option value="rolling">Rolling Update</option>
                  <option value="canary">Canary</option>
                  <option value="blue-green">Blue-Green</option>
                </select>
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  Rolling replaces instances gradually (recommended). Canary routes a small share of
                  traffic to the new version first. Blue-Green switches between two full copies.
                </p>
              </div>

              <div>
                <label className="label">Target Cluster *</label>
                <select
                  value={selectedClusters.length > 0 ? selectedClusters[0] : ''}
                  onChange={e => {
                    const id = e.target.value;
                    if (id) {
                      setSelectedClusters([id]);
                    } else {
                      setSelectedClusters([]);
                    }
                  }}
                  className="input"
                >
                  <option value="">Select a cluster...</option>
                  {clusterList.map(cluster => (
                    <option key={cluster.id} value={cluster.id}>
                      {cluster.name} ({cluster.environment})
                    </option>
                  ))}
                </select>
                {clusterList.length === 0 && (
                  <p className="text-[11px] text-orange-500 mt-1">
                    No clusters available. <Link href="/connections" className="underline">Add a Kubernetes connection</Link> or <Link href="/clusters" className="underline">register a cluster</Link>.
                  </p>
                )}
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  Choose the Kubernetes cluster where this service will be deployed.
                </p>
              </div>

              <div>
                <label className="label">Namespace</label>
                <input
                  type="text"
                  value={namespace}
                  onChange={e => setNamespace(e.target.value)}
                  className="input"
                  placeholder="default"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  The Kubernetes namespace to deploy into. Will be created if it doesn't exist.
                </p>
              </div>
            </div>
          </div>

          <div className="flex gap-3 justify-end">
            <button onClick={() => setStep('configure')} className="btn btn-secondary">← Back</button>
            <button onClick={() => setStep('review')} className="btn btn-primary">Next: Review →</button>
          </div>
        </div>
      )}

      {/* Step 4: Review */}
      {step === 'review' && (
        <div className="space-y-4">
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Review & Create</h2>
            </div>
            <div className="card-body">
              <div className="grid grid-cols-2 gap-4 text-[12px]">
                <div>
                  <span className="text-[var(--text-tertiary)]">Template</span>
                  <p className="font-medium text-[var(--text-primary)]">{selectedBlueprint ? `⭐ ${selectedBlueprint.name}` : selectedTemplate?.name}</p>
                </div>
                <div>
                  <span className="text-[var(--text-tertiary)]">Name</span>
                  <p className="font-medium text-[var(--text-primary)]">{name}</p>
                </div>
                <div>
                  <span className="text-[var(--text-tertiary)]">Namespace</span>
                  <p className="font-medium text-[var(--text-primary)]">{namespace}</p>
                </div>
                <div>
                  <span className="text-[var(--text-tertiary)]">Strategy</span>
                  <p className="font-medium text-[var(--text-primary)]">{strategy}</p>
                </div>
                <div>
                  <span className="text-[var(--text-tertiary)]">Resources</span>
                  <p className="font-medium text-[var(--text-primary)]">{cpu} CPU, {memory} RAM, {replicas} replicas</p>
                </div>
                <div>
                  <span className="text-[var(--text-tertiary)]">Clusters</span>
                  <p className="font-medium text-[var(--text-primary)]">
                    {selectedClusters.length > 0
                      ? selectedClusters.map(id => clusterList.find(c => c.id === id)?.name).filter(Boolean).join(', ')
                      : 'None selected'}
                  </p>
                </div>
                {description && (
                  <div className="col-span-2">
                    <span className="text-[var(--text-tertiary)]">Description</span>
                    <p className="font-medium text-[var(--text-primary)]">{description}</p>
                  </div>
                )}
                {envMode === 'kv' && envVars.filter(e => e.key).length > 0 && (
                  <div className="col-span-2">
                    <span className="text-[var(--text-tertiary)]">Environment Variables</span>
                    <div className="mt-1 space-y-0.5">
                      {envVars.filter(e => e.key).map((e, i) => (
                        <p key={i} className="font-mono text-[11px] text-[var(--text-secondary)]">{e.key}={e.value}</p>
                      ))}
                    </div>
                  </div>
                )}
                {envMode === 'yaml' && valuesYaml.trim() && (
                  <div className="col-span-2">
                    <span className="text-[var(--text-tertiary)]">values.yaml</span>
                    <pre className="mt-1 bg-[var(--border-light)] rounded-lg p-3 text-[11px] font-mono text-[var(--text-secondary)] overflow-auto max-h-[200px] whitespace-pre-wrap">{valuesYaml}</pre>
                    <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                      {Object.keys(parseYamlToEnvVars(valuesYaml)).length} environment variable(s) will be created
                    </p>
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="flex gap-3 justify-end">
            <button onClick={() => setStep('deploy')} className="btn btn-secondary">← Back</button>
            <button
              onClick={handleCreate}
              disabled={creating || !name}
              className="btn btn-primary"
            >
              {creating ? 'Creating...' : 'Create Service'}
            </button>
          </div>
        </div>
      )}
    </div>
    </div>
  );
}
