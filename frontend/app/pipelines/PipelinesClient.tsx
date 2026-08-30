'use client';

import { useState, useCallback, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { pipelineSources, pipelineRuns, pipelinePresets, connections as connectionsAPI, gitBrowser, type PipelineSource, type PipelineRun, type PipelinePreset, type PipelineRunJob, type Connection, type GitPipeline, type GitPipelineJob, type CIVariable } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import { VaultInput, useVaultPicker } from '@/components/VaultInput';
import GitRepoPicker from '@/components/GitRepoPicker';
import BrandIcon from '@/components/BrandIcon';
import PermissionGuard from '@/components/PermissionGuard';

// ── Constants ──────────────────────────────────────────────

const statusColors: Record<string, string> = {
  pending: 'bg-amber-500/15 text-amber-600',
  running: 'bg-blue-500/15 text-blue-500',
  success: 'bg-emerald-500/15 text-emerald-600',
  failed: 'bg-red-500/15 text-red-500',
  cancelled: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
  error: 'bg-red-500/15 text-red-500',
  timeout: 'bg-orange-500/15 text-orange-500',
  active: 'bg-emerald-500/15 text-emerald-600',
  disabled: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
};

const sourceTypeLabels: Record<string, string> = {
  gitlab_ci: 'GitLab CI',
  ansible: 'Ansible',
  terraform: 'Terraform',
  github_actions: 'GitHub Actions',
  trivy: 'Trivy Scanner',
};

const ENGINE_TYPE_INFO: Record<string, { icon: string; label: string; color: string; bgColor: string; borderColor: string; description: string }> = {
  gitlab_ci: { icon: 'gitlab', label: 'GitLab CI', color: 'text-[var(--accent)]', bgColor: 'bg-[var(--accent-subtle)]', borderColor: 'border-[var(--accent)]/20', description: 'CI/CD pipelines from Git repositories' },
  github_actions: { icon: 'github', label: 'GitHub Actions', color: 'text-[var(--text-primary)]', bgColor: 'bg-[var(--border-light)]', borderColor: 'border-[var(--border)]', description: 'CI/CD workflows from GitHub repositories' },
  ansible: { icon: 'ansible', label: 'Ansible', color: 'text-emerald-600', bgColor: 'bg-emerald-500/10', borderColor: 'border-emerald-500/20', description: 'Playbooks via repository or local path' },
  terraform: { icon: 'terraform', label: 'Terraform', color: 'text-purple-500', bgColor: 'bg-purple-500/10', borderColor: 'border-purple-500/20', description: 'Infrastructure stacks via repository or local path' },
  trivy: { icon: 'trivy', label: 'Trivy Scanner', color: 'text-cyan-600', bgColor: 'bg-cyan-500/10', borderColor: 'border-cyan-500/20', description: 'Security vulnerability scanning for images and code' },
};

const PROVIDER_STATUS_COLORS: Record<string, string> = {
  connected: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/20',
  disconnected: 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]',
  error: 'bg-red-500/15 text-red-500 border-red-500/20',
  ok: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/20',
};

const CONN_TYPE_INFO: Record<string, { icon: string; label: string }> = {
  gitlab: { icon: 'gitlab', label: 'GitLab' },
  git: { icon: 'git', label: 'Git' },
};

function getConnTypeInfo(conn: Connection) {
  const info = CONN_TYPE_INFO[conn.type] || { icon: 'plugin', label: conn.type };
  const provider = (conn.config as Record<string, unknown>)?.provider as string;
  if (provider) {
    const pInfo = CONN_TYPE_INFO[provider];
    if (pInfo) return pInfo;
  }
  return info;
}

// ── Main Component ─────────────────────────────────────────

function StepStatusIcon({ status }: { status: string }) {
  const s = status.toLowerCase();
  if (s === 'success' || s === 'completed') {
    return <span className="text-emerald-500 text-[10px]">&#10003;</span>;
  }
  if (s === 'failed' || s === 'failure') {
    return <span className="text-red-500 text-[10px]">&#10007;</span>;
  }
  if (s === 'running' || s === 'in_progress') {
    return <span className="inline-block w-2.5 h-2.5 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" />;
  }
  if (s === 'cancelled' || s === 'skipped') {
    return <span className="text-[var(--text-tertiary)] text-[10px]">&#8212;</span>;
  }
  // pending / queued / unknown
  return <span className="text-amber-500 text-[10px]">&#9679;</span>;
}

export default function PipelinesClient({
  initialSources,
  initialConnections,
}: {
  initialSources?: PipelineSource[];
  initialConnections?: Connection[];
}) {
  return (
    <PermissionGuard resource="pipelines" action="read">
      <PipelinesClientContent initialSources={initialSources} initialConnections={initialConnections} />
    </PermissionGuard>
  );
}

function PipelinesClientContent({
  initialSources,
  initialConnections,
}: {
  initialSources?: PipelineSource[];
  initialConnections?: Connection[];
}) {
  const searchParams = useSearchParams();
  const [activeTab, setActiveTab] = useState<'engines' | 'providers'>(
    searchParams.get('tab') === 'providers' ? 'providers' : 'engines'
  );
  const [sources, setSources] = useState<PipelineSource[]>(initialSources ?? []);
  const [connections, setConnections] = useState(initialConnections ?? []);
  const [loading, setLoading] = useState(false);
  const [connectionsLoading, setConnectionsLoading] = useState(!initialConnections);
  const [selectedSource, setSelectedSource] = useState<PipelineSource | null>(null);
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [presets, setPresets] = useState<PipelinePreset[]>([]);
  const [detailTab, setDetailTab] = useState<'runs' | 'presets'>('runs');
  const [triggering, setTriggering] = useState(false);
  const [showTriggerModal, setShowTriggerModal] = useState(false);
  const [triggerParams, setTriggerParams] = useState<Record<string, string>>({});
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [createModalInitialType, setCreateModalInitialType] = useState<string>('gitlab_ci');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState({ source: '', status: '', type: '' });
  const [testing, setTesting] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string; hint?: string } | null>(null);

  // Providers tab state: project selection and pipeline browsing
  const [selectedProviderConn, setSelectedProviderConn] = useState<Connection | null>(null);
  const [pickerValue, setPickerValue] = useState<Partial<{ connection_id: string; group_id: string; repo_id: string; repo_url: string; repo_full_name: string; branch: string }>>({});
  const [pipelines, setPipelines] = useState<GitPipeline[]>([]);
  const [loadingPipelines, setLoadingPipelines] = useState(false);

  // Provider tab: trigger, jobs, logs, presets
  const [showProviderTrigger, setShowProviderTrigger] = useState(false);

  // Escape key closes modals
  const anyModalOpen = showCreateModal || showTriggerModal || showProviderTrigger;
  useEscapeKey(() => {
    if (showProviderTrigger) setShowProviderTrigger(false);
    else if (showTriggerModal) { setShowTriggerModal(false); setEngineCIVariables([]); }
    else if (showCreateModal) setShowCreateModal(false);
  }, anyModalOpen);
  const [providerTriggerRef, setProviderTriggerRef] = useState('');
  const [providerTriggerVars, setProviderTriggerVars] = useState<Record<string, string>>({});
  const [providerTriggering, setProviderTriggering] = useState(false);
  const [providerTriggerNewKey, setProviderTriggerNewKey] = useState('');
  const [providerTriggerNewVal, setProviderTriggerNewVal] = useState('');
  const [showProviderPresetSave, setShowProviderPresetSave] = useState(false);
  const [providerPresetName, setProviderPresetName] = useState('');
  const [providerPresetDesc, setProviderPresetDesc] = useState('');
  const [providerPresets, setProviderPresets] = useState<{ name: string; description: string; ref: string; variables: Record<string, string> }[]>([]);
  const [expandedPipelineId, setExpandedPipelineId] = useState<number | null>(null);
  const [pipelineJobs, setPipelineJobs] = useState<GitPipelineJob[]>([]);
  const [loadingJobs, setLoadingJobs] = useState(false);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [jobsErrorHint, setJobsErrorHint] = useState<string | null>(null);
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const [jobLog, setJobLog] = useState<string>('');
  const [loadingLog, setLoadingLog] = useState(false);
  const [ciVariables, setCIVariables] = useState<CIVariable[]>([]);
  const [loadingCIVars, setLoadingCIVariables] = useState(false);
  const [engineCIVariables, setEngineCIVariables] = useState<CIVariable[]>([]);
  const [loadingEngineCIVars, setLoadingEngineCIVariables] = useState(false);

  // Engine runs: sync + expandable jobs/steps
  const [syncing, setSyncing] = useState(false);
  const [expandedRunId, setExpandedRunId] = useState<string | null>(null);
  const [runJobs, setRunJobs] = useState<Record<string, PipelineRunJob[]>>({});
  const [loadingRunJobs, setLoadingRunJobs] = useState<string | null>(null);
  const [expandedJobKey, setExpandedJobKey] = useState<string | null>(null);

  // Fetch connections client-side (server-side has no auth token)
  useEffect(() => {
    if (!connectionsLoading) return;
    connectionsAPI.list()
      .then(data => {
        const allConns = data.connections || [];
        console.log('[Pipelines] All connections from API:', allConns.map(c => ({ id: c.id, name: c.name, type: c.type })));
        // Filter to git/gitlab connections that can provide CI/CD pipelines
        const gitConns = allConns.filter(c => c.type === 'git' || c.type === 'gitlab');
        console.log('[Pipelines] Filtered git/gitlab connections:', gitConns.map(c => ({ id: c.id, name: c.name, type: c.type })));
        setConnections(gitConns);
      })
      .catch((err) => {
        console.error('[Pipelines] Failed to fetch connections:', err);
        setConnections([]);
      })
      .finally(() => setConnectionsLoading(false));
  }, [connectionsLoading]);

  const loadSources = useCallback(async () => {
    try {
      const data = await pipelineSources.list();
      setSources(data.sources || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch sources on mount when no SSR data is provided
  useEffect(() => {
    if (initialSources === undefined) {
      loadSources();
    }
  }, []);

  const loadRuns = useCallback(async (sourceId: string) => {
    try {
      const data = await pipelineRuns.list(sourceId);
      setRuns(data.runs || []);
    } catch { setRuns([]); }
  }, []);

  const loadPresets = useCallback(async (sourceId: string) => {
    try {
      const data = await pipelinePresets.list(sourceId);
      setPresets(data.presets || []);
    } catch { setPresets([]); }
  }, []);

  const selectSource = async (source: PipelineSource) => {
    setSelectedSource(source);
    setDetailTab('runs');
    await loadRuns(source.id);
    await loadPresets(source.id);
  };

  const handleTrigger = async () => {
    if (!selectedSource) return;
    setTriggering(true);
    try {
      const params: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(triggerParams)) {
        if (v.trim()) params[k] = v;
      }
      await pipelineRuns.trigger(selectedSource.id, { parameters: params });
      setShowTriggerModal(false);
      setTriggerParams({});
      await loadRuns(selectedSource.id);
      setFeedback({ ok: true, text: 'Pipeline run triggered successfully' });
    } catch (err) {
      setFeedback({ ok: false, text: friendlyError(err).message });
    } finally {
      setTriggering(false);
    }
  };

  const handleRefresh = async (runId: string) => {
    if (!selectedSource) return;
    try {
      await pipelineRuns.refresh(selectedSource.id, runId);
      await loadRuns(selectedSource.id);
      setFeedback({ ok: true, text: 'Pipeline run refreshed' });
    } catch (err) { setFeedback({ ok: false, text: friendlyError(err).message }); }
  };

  const handleCancel = async (runId: string) => {
    if (!selectedSource) return;
    if (!confirm('Cancel this pipeline run?')) return;
    try {
      await pipelineRuns.cancel(selectedSource.id, runId);
      await loadRuns(selectedSource.id);
      setFeedback({ ok: true, text: 'Pipeline run cancelled' });
    } catch (err) { setFeedback({ ok: false, text: friendlyError(err).message }); }
  };

  // Open trigger modal for Engines tab - dynamically load CI variables if connection available
  const openEngineTriggerModal = async (source: PipelineSource) => {
    const schema = source.parameter_schema as Record<string, Record<string, unknown>> | undefined;
    const defaults: Record<string, string> = {};
    if (schema?.properties) {
      for (const [k, v] of Object.entries(schema.properties)) {
        if (v && typeof v === 'object' && 'default' in v) defaults[k] = String(v.default);
      }
    }
    setEngineCIVariables([]);
    setTriggerParams(defaults);
    setShowTriggerModal(true);

    // Dynamically load CI variables via plugin if source has a connection
    if (source.connection_id && source.source_type === 'gitlab_ci') {
      const projectId = source.config?.project_id as string;
      const ref = source.config?.ref as string || 'main';
      if (projectId) {
        setLoadingEngineCIVariables(true);
        try {
          const data = await gitBrowser.parseCIConfig(source.connection_id, projectId, ref);
          setEngineCIVariables(data.variables || []);
          // Pre-fill defaults from CI variables
          for (const v of (data.variables || [])) {
            if (v.value && !defaults[v.key]) {
              defaults[v.key] = v.value;
            }
          }
          setTriggerParams({ ...defaults });
        } catch {
          // Silently fall back to stored schema
        } finally {
          setLoadingEngineCIVariables(false);
        }
      }
    }
  };

  const handleResolveSchema = async (source: PipelineSource) => {
    try {
      await pipelineSources.resolveSchema(source.id);
      await loadSources();
      setFeedback({ ok: true, text: 'Schema resolved successfully' });
    } catch (err) { setFeedback({ ok: false, text: friendlyError(err).message }); }
  };

  const handleDeleteSource = async (source: PipelineSource) => {
    if (!confirm(`Delete engine "${source.name}"?`)) return;
    try {
      await pipelineSources.delete(source.id);
      if (selectedSource?.id === source.id) setSelectedSource(null);
      await loadSources();
      setFeedback({ ok: true, text: 'Engine deleted' });
    } catch (err) { setFeedback({ ok: false, text: friendlyError(err).message }); }
  };

  const handleSyncRuns = async () => {
    if (!selectedSource) return;
    setSyncing(true);
    try {
      const result = await pipelineRuns.sync(selectedSource.id);
      await loadRuns(selectedSource.id);
      setFeedback({ ok: true, text: `Synced ${result.synced} runs from remote` });
    } catch (err) { setFeedback({ ok: false, text: friendlyError(err).message }); }
    finally { setSyncing(false); }
  };

  const toggleRunExpansion = async (runId: string) => {
    if (expandedRunId === runId) {
      setExpandedRunId(null);
      return;
    }
    setExpandedRunId(runId);
    if (!runJobs[runId] && selectedSource) {
      setLoadingRunJobs(runId);
      try {
        const data = await pipelineRuns.jobs(selectedSource.id, runId);
        setRunJobs(prev => ({ ...prev, [runId]: data.jobs || [] }));
      } catch {
        setRunJobs(prev => ({ ...prev, [runId]: [] }));
      } finally {
        setLoadingRunJobs(null);
      }
    }
  };

  const toggleJobExpansion = (key: string) => {
    setExpandedJobKey(prev => prev === key ? null : key);
  };

  // Provider handlers
  const handleTestProvider = async (id: string) => {
    setTesting(id);
    try {
      const result = await connectionsAPI.test(id);
      setConnections(prev => prev.map(c => c.id === id ? { ...c, status: result.status } : c));
      const ok = result.status === 'connected' || result.status === 'ok' || result.status === 'healthy';
      setFeedback({ ok, text: ok ? `Connection works: ${result.message}` : `Test failed: ${result.message}` });
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Test failed: ${fe.message}`, hint: fe.hint });
    } finally {
      setTesting(null);
    }
  };

  const handleSelectProviderConn = (conn: Connection) => {
    setSelectedProviderConn(conn);
    setPipelines([]);
    setPickerValue({ connection_id: conn.id });
    setProviderPresets([]);
    setExpandedPipelineId(null);
  };

  const handlePickerChange = async (value: { connection_id: string; group_id: string; repo_id: string; repo_url: string; repo_full_name: string; branch: string }) => {
    setPickerValue(value);
    setExpandedPipelineId(null);
    setPipelineJobs([]);
    if (value.repo_id && value.connection_id) {
      setLoadingPipelines(true);
      try {
        const data = await gitBrowser.listPipelines(value.connection_id, value.repo_id);
        setPipelines(data.pipelines || []);
      } catch {
        setPipelines([]);
      } finally {
        setLoadingPipelines(false);
      }
      // Load presets for this connection+repo
      try {
        const raw = localStorage.getItem(`pepa:provider-presets:${value.connection_id}:${value.repo_id}`);
        if (raw) setProviderPresets(JSON.parse(raw));
        else setProviderPresets([]);
      } catch { setProviderPresets([]); }
    } else {
      setPipelines([]);
      setProviderPresets([]);
    }
  };

  // Provider tab: preset load/save (localStorage per connection+repo)
  const presetStorageKey = () => `pepa:provider-presets:${pickerValue.connection_id}:${pickerValue.repo_id}`;

  const saveProviderPreset = () => {
    if (!providerPresetName.trim()) return;
    const updated = [...providerPresets, {
      name: providerPresetName,
      description: providerPresetDesc,
      ref: providerTriggerRef,
      variables: { ...providerTriggerVars },
    }];
    setProviderPresets(updated);
    localStorage.setItem(presetStorageKey(), JSON.stringify(updated));
    setShowProviderPresetSave(false);
    setProviderPresetName('');
    setProviderPresetDesc('');
  };

  const deleteProviderPreset = (idx: number) => {
    const updated = providerPresets.filter((_, i) => i !== idx);
    setProviderPresets(updated);
    localStorage.setItem(presetStorageKey(), JSON.stringify(updated));
  };

  // Provider tab: trigger pipeline
  const handleProviderTrigger = async () => {
    if (!selectedProviderConn || !pickerValue.repo_id) return;
    setProviderTriggering(true);
    try {
      const result = await gitBrowser.triggerPipeline(
        selectedProviderConn.id,
        pickerValue.repo_id,
        providerTriggerRef || 'main',
        providerTriggerVars,
      );
      setShowProviderTrigger(false);
      setProviderTriggerVars({});
      setProviderTriggerRef('');
      // Refresh pipelines list
      const data = await gitBrowser.listPipelines(selectedProviderConn.id, pickerValue.repo_id);
      setPipelines(data.pipelines || []);
      setFeedback({ ok: true, text: `Pipeline #${result.id} triggered successfully` });
    } catch (err) {
      setFeedback({ ok: false, text: `Trigger failed: ${friendlyError(err).message}` });
    } finally {
      setProviderTriggering(false);
    }
  };

  // Provider tab: view jobs for a pipeline
  const handleViewJobs = async (pipelineId: number) => {
    if (!selectedProviderConn || !pickerValue.repo_id) return;
    if (expandedPipelineId === pipelineId) {
      setExpandedPipelineId(null);
      setPipelineJobs([]);
      setSelectedJobId(null);
      setJobLog('');
      return;
    }
    setExpandedPipelineId(pipelineId);
    setLoadingJobs(true);
    setSelectedJobId(null);
    setJobLog('');
    setJobsError(null);
    setJobsErrorHint(null);
    try {
      const data = await gitBrowser.getPipelineJobs(selectedProviderConn.id, pickerValue.repo_id, pipelineId);
      setPipelineJobs(data.jobs || []);
    } catch (err) {
      setPipelineJobs([]);
      const fe = friendlyError(err);
      setJobsError(fe.message);
      setJobsErrorHint(fe.hint || null);
    } finally {
      setLoadingJobs(false);
    }
  };

  // Provider tab: view job log
  const handleViewJobLog = async (jobId: number) => {
    if (!selectedProviderConn || !pickerValue.repo_id) return;
    setSelectedJobId(jobId);
    setLoadingLog(true);
    setJobLog('');
    try {
      const data = await gitBrowser.getJobLog(selectedProviderConn.id, pickerValue.repo_id, jobId);
      setJobLog(data.log || 'No log output available.');
    } catch (err) {
      setJobLog(`Failed to load log: ${friendlyError(err).message}`);
    } finally {
      setLoadingLog(false);
    }
  };

  // Open trigger modal and load CI variables from the repo
  const openProviderTriggerModal = async (ref?: string, presetVars?: Record<string, string>) => {
    if (!selectedProviderConn || !pickerValue.repo_id) return;
    setProviderTriggerRef(ref || pickerValue.branch || 'main');
    setProviderTriggerVars({});
    setCIVariables([]);
    setShowProviderTrigger(true);
    setLoadingCIVariables(true);
    try {
      const data = await gitBrowser.parseCIConfig(selectedProviderConn.id, pickerValue.repo_id, ref || pickerValue.branch || 'main');
      setCIVariables(data.variables || []);
      // If preset values provided, use them; otherwise pre-fill from CI config defaults
      if (presetVars) {
        setProviderTriggerVars({ ...presetVars });
      } else {
        const prefill: Record<string, string> = {};
        for (const v of (data.variables || [])) {
          if (v.value) prefill[v.key] = v.value;
        }
        setProviderTriggerVars(prefill);
      }
    } catch {
      setCIVariables([]);
    } finally {
      setLoadingCIVariables(false);
    }
  };

  const filteredSources = sources.filter(s => {
    if (filters.source && s.id !== filters.source) return false;
    if (filters.status && s.status !== filters.status) return false;
    if (filters.type && s.source_type !== filters.type) return false;
    return true;
  });

  const connectedCount = connections.filter(c => c.status === 'connected' || c.status === 'ok').length;

  if (loading) {
    return <div className="flex items-center justify-center h-64"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" /></div>;
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Pipelines</h1>
            <p className="page-subtitle-modern">Manage CI/CD providers, automation engines, and track runs</p>
          </div>
          <div className="flex gap-2">
            <Link href="/pipeline-builder" className="px-4 py-2 rounded-lg text-sm font-medium border bg-[var(--surface)] border-[var(--border)] text-[var(--text-secondary)] hover:bg-[var(--bg)]">
              Pipeline Builder
            </Link>
            {activeTab === 'engines' && (
              <button onClick={() => setShowCreateModal(true)} className="btn btn-primary">
                Add Engine
              </button>
            )}
            {activeTab === 'providers' && (
              <Link href="/connections" className="btn btn-primary">
                + Add Git Connection
              </Link>
            )}
          </div>
        </div>

        {/* Feedback */}
        {feedback && (
          <div className={`rounded-lg border p-3 flex items-start justify-between gap-3 mb-4 ${feedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'}`}>
            <div>
              <p className={`text-sm font-medium ${feedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
                {feedback.ok ? '✓ ' : '⚠ '}{feedback.text}
              </p>
              {feedback.hint && <p className="text-xs text-red-400 mt-1">{feedback.hint}</p>}
            </div>
            <button onClick={() => setFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
          </div>
        )}

        {/* Tabs */}
        <div className="border-b border-[var(--border)] page-animate-up page-delay-1">
          <div className="flex gap-1">
            <button
              onClick={() => setActiveTab('engines')}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${activeTab === 'engines' ? 'border-blue-600 text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-secondary)]'}`}
            >
              Engines
              {sources.length > 0 && <span className="ml-1.5 px-1.5 py-0.5 text-xs bg-[var(--border-light)] rounded-full">{sources.length}</span>}
            </button>
            <button
              onClick={() => setActiveTab('providers')}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${activeTab === 'providers' ? 'border-blue-600 text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-secondary)]'}`}
            >
              Providers
              {connections.length > 0 && <span className="ml-1.5 px-1.5 py-0.5 text-xs bg-[var(--border-light)] rounded-full">{connections.length}</span>}
            </button>
          </div>
        </div>

        {/* ── Engines Tab ──────────────────────────────────── */}
        {activeTab === 'engines' && (
          <>
            {/* Filters */}
            <div className="flex items-center gap-2 mb-4">
              <button
                onClick={() => setShowFilters(!showFilters)}
                className={`px-3 py-1.5 rounded-lg text-sm font-medium border ${showFilters ? 'bg-[var(--accent-subtle)] border-[var(--accent)] text-[var(--accent)]' : 'bg-[var(--surface)] border-[var(--border)] text-[var(--text-secondary)] hover:bg-[var(--bg)]'}`}
              >
                <span className="flex items-center gap-2">
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" /></svg>
                  Filters
                  {(filters.source || filters.status || filters.type) && <span className="w-2 h-2 bg-blue-600 rounded-full" />}
                </span>
              </button>
            </div>

            {showFilters && (
              <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-4 mb-4">
                <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                  <div>
                    <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">Engine</label>
                    <select value={filters.source} onChange={e => setFilters(f => ({ ...f, source: e.target.value }))} className="w-full px-3 py-2 border border-[var(--border)] rounded-md text-sm">
                      <option value="">All Engines</option>
                      {sources.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">Status</label>
                    <select value={filters.status} onChange={e => setFilters(f => ({ ...f, status: e.target.value }))} className="w-full px-3 py-2 border border-[var(--border)] rounded-md text-sm">
                      <option value="">All Statuses</option>
                      <option value="active">Active</option>
                      <option value="disabled">Disabled</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">Engine Type</label>
                    <select value={filters.type} onChange={e => setFilters(f => ({ ...f, type: e.target.value }))} className="w-full px-3 py-2 border border-[var(--border)] rounded-md text-sm">
                      <option value="">All Types</option>
                      <option value="gitlab_ci">GitLab CI</option>
                      <option value="github_actions">GitHub Actions</option>
                      <option value="ansible">Ansible</option>
                      <option value="terraform">Terraform</option>
                      <option value="trivy">Trivy Scanner</option>
                    </select>
                  </div>
                  <div className="flex items-end">
                    <button onClick={() => setFilters({ source: '', status: '', type: '' })} className="px-4 py-2 text-sm text-[var(--text-secondary)] hover:bg-[var(--border-light)] rounded-md w-full">Clear All</button>
                  </div>
                </div>
              </div>
            )}
            
            {/* Empty state: three engine type cards */}
            {sources.length === 0 && (
              <div className="space-y-6">
                <div className="text-center">
                  <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-1">Connect your automation engines</h3>
                  <p className="text-sm text-[var(--text-secondary)]">Each engine type can be connected independently — Ansible and Terraform support both repository and local paths</p>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  {Object.entries(ENGINE_TYPE_INFO).map(([type, info]) => (
                    <div key={type} className={`rounded-xl border-2 ${info.borderColor} ${info.bgColor} p-6 text-center space-y-3`}>
                      <div className="text-4xl"><BrandIcon name={info.icon} size={36} /></div>
                      <div>
                        <p className={`text-base font-semibold ${info.color}`}>{info.label}</p>
                        <p className="text-xs text-[var(--text-secondary)] mt-1">{info.description}</p>
                      </div>
                      <button
                        onClick={() => { setCreateModalInitialType(type); setShowCreateModal(true); }}
                        className={`px-4 py-2 rounded-lg text-sm font-medium border bg-[var(--surface)] ${info.borderColor} ${info.color} hover:opacity-80 transition-opacity`}
                      >
                        Connect {info.label}
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
            
            {sources.length > 0 && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              {/* Sources list */}
              <div className="lg:col-span-1">
                <div className="bg-[var(--surface)] rounded-lg shadow-sm border">
                  <div className="p-4 border-b">
                    <h2 className="text-sm font-semibold text-[var(--text-primary)]">Automation Engines</h2>
                  </div>
                  <div className="divide-y">
                    {filteredSources.length === 0 && (
                      <div className="p-6 text-center text-sm text-[var(--text-secondary)]">
                        {sources.length === 0 ? 'No engines configured.' : 'No engines match the filters.'}
                      </div>
                    )}
                    {filteredSources.map(source => (
                      <div
                        key={source.id}
                        onClick={() => selectSource(source)}
                        className={`p-4 cursor-pointer hover:bg-[var(--bg)] transition-colors ${selectedSource?.id === source.id ? 'bg-[var(--accent-subtle)] border-l-4 border-l-[var(--accent)]' : ''}`}
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2.5">
                            <span className={`inline-flex items-center justify-center w-8 h-8 rounded-lg text-sm ${ENGINE_TYPE_INFO[source.source_type]?.bgColor || 'bg-[var(--bg)]'}`}>
                              <BrandIcon name={ENGINE_TYPE_INFO[source.source_type]?.icon || 'plugin'} size={16} />
                            </span>
                            <div>
                              <p className="text-sm font-medium text-[var(--text-primary)]">{source.name}</p>
                              <p className="text-xs text-[var(--text-secondary)] mt-0.5">
                                <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${ENGINE_TYPE_INFO[source.source_type]?.bgColor || 'bg-[var(--border-light)]'} ${ENGINE_TYPE_INFO[source.source_type]?.color || 'text-[var(--text-secondary)]'}`}>
                                  {sourceTypeLabels[source.source_type] || source.source_type}
                                </span>
                                {source.description && <span className="ml-1">— {source.description}</span>}
                              </p>
                            </div>
                          </div>
                          <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[source.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                            {source.status}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              {/* Detail panel */}
              <div className="lg:col-span-2">
                {!selectedSource ? (
                  <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-12 text-center">
                    <div className="text-4xl mb-3 opacity-30"><BrandIcon name="cicd" size={36} /></div>
                    <p className="text-[var(--text-secondary)] mb-4">Select an engine to view runs and presets</p>
                    <button onClick={() => setShowCreateModal(true)} className="text-sm text-[var(--accent)] hover:underline mt-2">
                      Or add a new engine →
                    </button>
                  </div>
                ) : (
                  <div className="space-y-4">
                    <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-4">
                      <div className="flex items-center justify-between">
                        <div>
                          <h2 className="text-lg font-semibold text-[var(--text-primary)]">{selectedSource.name}</h2>
                          <p className="text-sm text-[var(--text-secondary)]">{sourceTypeLabels[selectedSource.source_type]} pipeline</p>
                        </div>
                        <div className="flex gap-2">
                          <button
                            onClick={() => openEngineTriggerModal(selectedSource)}
                            className="px-3 py-1.5 bg-green-600 text-white rounded-md hover:bg-green-700 text-sm font-medium"
                          >
                            Run Pipeline
                          </button>
                          <button
                            onClick={handleSyncRuns}
                            disabled={syncing}
                            className="px-3 py-1.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-md hover:bg-[var(--border)] text-sm disabled:opacity-50"
                          >
                            {syncing ? 'Syncing...' : 'Sync Runs'}
                          </button>
                          <button onClick={() => handleResolveSchema(selectedSource)} className="px-3 py-1.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-md hover:bg-[var(--border)] text-sm">Refresh Schema</button>
                          <button onClick={() => handleDeleteSource(selectedSource)} className="px-3 py-1.5 bg-red-500/10 text-red-500 rounded-md hover:bg-red-500/20 text-sm">Delete</button>
                        </div>
                      </div>
                      {selectedSource.last_error && <p className="mt-2 text-sm text-red-600">Error: {selectedSource.last_error}</p>}
                    </div>

                    <div className="bg-[var(--surface)] rounded-lg shadow-sm border">
                      <div className="border-b flex">
                        <button onClick={() => setDetailTab('runs')} className={`px-4 py-3 text-sm font-medium border-b-2 ${detailTab === 'runs' ? 'border-blue-600 text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-secondary)]'}`}>
                          Runs ({runs.length})
                        </button>
                        <button onClick={() => setDetailTab('presets')} className={`px-4 py-3 text-sm font-medium border-b-2 ${detailTab === 'presets' ? 'border-blue-600 text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-secondary)]'}`}>
                          Presets ({presets.length})
                        </button>
                      </div>
                      <div className="p-4">
                        {detailTab === 'runs' && (
                          <div className="space-y-2">
                            {runs.length === 0 && (
                              <div className="text-center py-6">
                                <p className="text-sm text-[var(--text-secondary)] mb-2">No runs yet</p>
                                <button
                                  onClick={handleSyncRuns}
                                  disabled={syncing}
                                  className="text-sm text-[var(--accent)] hover:underline disabled:opacity-50"
                                >
                                  {syncing ? 'Syncing...' : 'Sync runs from remote →'}
                                </button>
                              </div>
                            )}
                            {runs.map(run => {
                              const isExpanded = expandedRunId === run.id;
                              const jobs = runJobs[run.id];
                              const isLoadingJobs = loadingRunJobs === run.id;
                              return (
                                <div key={run.id} className="bg-[var(--bg)] rounded-md overflow-hidden">
                                  <div
                                    className="flex items-center justify-between p-3 cursor-pointer hover:bg-[var(--border-light)] transition-colors"
                                    onClick={() => toggleRunExpansion(run.id)}
                                  >
                                    <div className="flex items-center gap-2 flex-1 min-w-0">
                                      <svg className={`w-3.5 h-3.5 text-[var(--text-tertiary)] transition-transform ${isExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                                      </svg>
                                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[run.status] || 'bg-[var(--border-light)]'}`}>{run.status}</span>
                                      <span className="text-sm text-[var(--text-primary)] truncate">#{run.external_run_id || run.id.slice(0, 8)}</span>
                                      <span className="text-xs text-[var(--text-tertiary)]">
                                        {new Date(run.created_at).toLocaleString()}
                                        {run.duration_ms ? ` — ${(run.duration_ms / 1000).toFixed(1)}s` : ''}
                                      </span>
                                    </div>
                                    <div className="flex gap-1" onClick={e => e.stopPropagation()}>
                                      {run.external_url && (
                                        <a href={run.external_url} target="_blank" rel="noopener noreferrer" className="px-2 py-1 text-xs text-[var(--accent)] hover:underline">External</a>
                                      )}
                                      {(run.status === 'running' || run.status === 'pending') && (
                                        <>
                                          <button onClick={() => handleRefresh(run.id)} className="px-2 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--border)] rounded">Refresh</button>
                                          <button onClick={() => handleCancel(run.id)} className="px-2 py-1 text-xs text-red-500 hover:bg-red-500/10 rounded">Cancel</button>
                                        </>
                                      )}
                                    </div>
                                  </div>

                                  {/* Expanded: Jobs & Steps */}
                                  {isExpanded && (
                                    <div className="border-t px-3 py-2 space-y-2">
                                      {isLoadingJobs && (
                                        <div className="flex items-center gap-2 py-2">
                                          <div className="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-blue-600" />
                                          <span className="text-xs text-[var(--text-secondary)]">Loading jobs...</span>
                                        </div>
                                      )}
                                      {!isLoadingJobs && jobs && jobs.length === 0 && (
                                        <p className="text-xs text-[var(--text-tertiary)] py-1">No jobs recorded for this run</p>
                                      )}
                                      {!isLoadingJobs && jobs && jobs.map((job, ji) => {
                                        const jobKey = `${run.id}:${job.id || ji}`;
                                        const jobExpanded = expandedJobKey === jobKey;
                                        const steps = job.steps || [];
                                        return (
                                          <div key={job.id || ji} className="ml-4">
                                            <div
                                              className="flex items-center gap-2 py-1.5 cursor-pointer hover:bg-[var(--surface)] rounded px-2 -mx-2 transition-colors"
                                              onClick={() => steps.length > 0 ? toggleJobExpansion(jobKey) : undefined}
                                            >
                                              {steps.length > 0 && (
                                                <svg className={`w-3 h-3 text-[var(--text-tertiary)] transition-transform ${jobExpanded ? 'rotate-90' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                                                </svg>
                                              )}
                                              {steps.length === 0 && <span className="w-3" />}
                                              <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${statusColors[job.status] || 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>{job.status}</span>
                                              <span className="text-xs font-medium text-[var(--text-primary)]">{job.name}</span>
                                              {job.runner_name && <span className="text-[10px] text-[var(--text-tertiary)]">{job.runner_name}</span>}
                                              {job.log_url && (
                                                <a href={job.log_url} target="_blank" rel="noopener noreferrer" className="text-[10px] text-[var(--accent)] hover:underline ml-auto" onClick={e => e.stopPropagation()}>logs</a>
                                              )}
                                            </div>

                                            {/* Steps */}
                                            {jobExpanded && steps.length > 0 && (
                                              <div className="ml-8 mt-1 space-y-0.5">
                                                {steps.map((step, si) => (
                                                  <div key={si} className="flex items-center gap-2 py-0.5">
                                                    <StepStatusIcon status={step.status} />
                                                    <span className="text-[11px] text-[var(--text-secondary)]">{step.name}</span>
                                                  </div>
                                                ))}
                                              </div>
                                            )}
                                          </div>
                                        );
                                      })}
                                    </div>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        )}
                        {detailTab === 'presets' && (
                          <div className="space-y-2">
                            {presets.length === 0 && <p className="text-sm text-[var(--text-secondary)] text-center py-4">No presets saved</p>}
                            {presets.map(preset => (
                              <div key={preset.id} className="flex items-center justify-between p-3 bg-[var(--bg)] rounded-md">
                                <div>
                                  <p className="text-sm font-medium text-[var(--text-primary)]">{preset.name}</p>
                                  <p className="text-xs text-[var(--text-secondary)]">
                                    Used {preset.use_count} time{preset.use_count !== 1 ? 's' : ''}
                                    {preset.description && ` — ${preset.description}`}
                                  </p>
                                </div>
                                <button
                                  onClick={() => {
                                    const params = preset.parameters as Record<string, unknown>;
                                    const strParams: Record<string, string> = {};
                                    for (const [k, v] of Object.entries(params)) strParams[k] = String(v);
                                    setTriggerParams(strParams);
                                    setShowTriggerModal(true);
                                  }}
                                  className="px-3 py-1.5 bg-green-600 text-white rounded-md hover:bg-green-700 text-xs font-medium"
                                >
                                  Run with Preset
                                </button>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
            )}
          </>
        )}

        {/* ── Providers Tab ────────────────────────────────── */}
        {activeTab === 'providers' && (
          <div className="space-y-4">
            {/* Summary */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-2xl"><BrandIcon name="plugin" size={24} /></span>
                  <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-600">{connectedCount} connected</span>
                </div>
                <p className="text-2xl font-bold text-[var(--text-primary)]">{connections.length}</p>
                <p className="text-xs text-[var(--text-secondary)] mt-0.5">Git Connections</p>
              </div>
              <Link href="/pipeline-builder" className="bg-[var(--surface)] rounded-lg shadow-sm border p-4 hover:border-blue-400 transition-colors">
                <div className="text-2xl mb-2"><BrandIcon name="plugin" size={24} /></div>
                <p className="text-sm font-semibold text-[var(--text-primary)]">Pipeline Builder</p>
                <p className="text-xs text-[var(--text-secondary)] mt-0.5">Compose multi-step deployment pipelines</p>
              </Link>
              <Link href="/pipeline-blueprints" className="bg-[var(--surface)] rounded-lg shadow-sm border p-4 hover:border-blue-400 transition-colors">
                <div className="text-2xl mb-2"><BrandIcon name="plugin" size={24} /></div>
                <p className="text-sm font-semibold text-[var(--text-primary)]">Blueprints</p>
                <p className="text-xs text-[var(--text-secondary)] mt-0.5">Reusable service pipeline templates</p>
              </Link>
            </div>

            {connectionsLoading ? (
              <div className="text-center py-16 bg-[var(--surface)] rounded-lg shadow-sm border">
                <div className="animate-pulse text-5xl mb-4 opacity-30">🔀</div>
                <p className="text-[var(--text-secondary)]">Loading connections...</p>
              </div>
            ) : connections.length === 0 ? (
              <div className="text-center py-16 bg-[var(--surface)] rounded-lg shadow-sm border">
                <div className="text-5xl mb-4 opacity-30">🔀</div>
                <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-2">No Git connections configured</h3>
                <p className="text-[var(--text-secondary)] mb-6">Add a GitLab or Git connection in the Connections page to browse CI/CD pipelines</p>
                <Link href="/connections" className="btn btn-primary">
                  Go to Connections
                </Link>
              </div>
            ) : (
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                {/* Connection list */}
                <div className="lg:col-span-1">
                  <div className="bg-[var(--surface)] rounded-lg shadow-sm border">
                    <div className="p-4 border-b">
                      <h2 className="text-sm font-semibold text-[var(--text-primary)]">Git Connections</h2>
                      <p className="text-xs text-[var(--text-secondary)] mt-0.5">Select to browse CI/CD pipelines</p>
                    </div>
                    <div className="divide-y">
                      {connections.map(conn => {
                        const info = getConnTypeInfo(conn);
                        const url = (conn.config as Record<string, unknown>)?.url as string || '';
                        return (
                          <div
                            key={conn.id}
                            onClick={() => handleSelectProviderConn(conn)}
                            className={`p-4 cursor-pointer hover:bg-[var(--bg)] transition-colors ${selectedProviderConn?.id === conn.id ? 'bg-[var(--accent-subtle)] border-l-4 border-l-[var(--accent)]' : ''}`}
                          >
                            <div className="flex items-center justify-between">
                              <div className="flex items-center gap-3">
                                <span className="text-xl"><BrandIcon name={info.icon} size={20} /></span>
                                <div>
                                  <p className="text-sm font-medium text-[var(--text-primary)]">{conn.name}</p>
                                  <p className="text-xs text-[var(--text-secondary)]">{info.label}{url ? ` — ${url}` : ''}</p>
                                </div>
                              </div>
                              <span className={`text-xs px-2 py-0.5 rounded-full border ${PROVIDER_STATUS_COLORS[conn.status] || PROVIDER_STATUS_COLORS.disconnected}`}>
                                {conn.status}
                              </span>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </div>

                {/* Detail panel */}
                <div className="lg:col-span-2">
                  {!selectedProviderConn ? (
                    <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-12 text-center">
                      <div className="text-4xl mb-3 opacity-30"><BrandIcon name="gitlab" size={36} /></div>
                      <p className="text-[var(--text-secondary)] mb-2">Select a connection to browse its projects and CI/CD pipelines</p>
                    </div>
                  ) : (
                    <div className="space-y-4">
                      {/* Connection info */}
                      <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-4">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3">
                            <span className="text-2xl"><BrandIcon name={getConnTypeInfo(selectedProviderConn).icon} size={24} /></span>
                            <div>
                              <h2 className="text-lg font-semibold text-[var(--text-primary)]">{selectedProviderConn.name}</h2>
                              <p className="text-sm text-[var(--text-secondary)]">
                                {getConnTypeInfo(selectedProviderConn).label}
                                {(selectedProviderConn.config as Record<string, unknown>)?.url ? ` — ${String((selectedProviderConn.config as Record<string, unknown>).url)}` : ''}
                              </p>
                            </div>
                          </div>
                          <button
                            onClick={() => handleTestProvider(selectedProviderConn.id)}
                            disabled={testing === selectedProviderConn.id}
                            className="px-3 py-1.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-md hover:bg-[var(--border)] text-sm"
                          >
                            {testing === selectedProviderConn.id ? 'Testing...' : 'Test Connection'}
                          </button>
                        </div>
                      </div>

                      {/* Project picker */}
                      <div className="bg-[var(--surface)] rounded-lg shadow-sm border p-4">
                        <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">Select Project</h3>
                        <GitRepoPicker
                          value={pickerValue}
                          onChange={handlePickerChange}
                          label=""
                          gitConnections={[selectedProviderConn]}
                          showBranch={true}
                        />
                      </div>

                      {/* Pipelines list */}
                      {pickerValue.repo_id && (
                        <div className="bg-[var(--surface)] rounded-lg shadow-sm border">
                          <div className="p-4 border-b flex items-center justify-between">
                            <div>
                              <h3 className="text-sm font-semibold text-[var(--text-primary)]">
                                CI/CD Pipelines
                                {pipelines.length > 0 && <span className="ml-1.5 px-1.5 py-0.5 text-xs bg-[var(--border-light)] rounded-full">{pipelines.length}</span>}
                              </h3>
                              {pickerValue.repo_full_name && (
                                <p className="text-xs text-[var(--text-secondary)] mt-0.5">{pickerValue.repo_full_name}</p>
                              )}
                            </div>
                            <div className="flex gap-2">
                              {providerPresets.length > 0 && (
                                <select
                                  onChange={async (e) => {
                                    const preset = providerPresets[Number(e.target.value)];
                                    if (preset) {
                                      await openProviderTriggerModal(preset.ref, preset.variables as Record<string, string>);
                                    }
                                    e.target.value = '';
                                  }}
                                  className="px-3 py-1.5 border rounded-md text-sm bg-[var(--surface)]"
                                  defaultValue=""
                                >
                                  <option value="" disabled>Load Preset...</option>
                                  {providerPresets.map((p, i) => (
                                    <option key={i} value={i}>{p.name}</option>
                                  ))}
                                </select>
                              )}
                              <button
                                onClick={() => openProviderTriggerModal()}
                                className="px-3 py-1.5 bg-green-600 text-white rounded-md hover:bg-green-700 text-sm font-medium"
                              >
                                Run Pipeline
                              </button>
                            </div>
                          </div>
                          <div className="p-4">
                            {loadingPipelines ? (
                              <div className="flex items-center justify-center py-8">
                                <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600" />
                              </div>
                            ) : pipelines.length === 0 ? (
                              <div className="text-center py-8">
                                <div className="text-3xl mb-2 opacity-30"><BrandIcon name="cicd" size={28} /></div>
                                <p className="text-sm text-[var(--text-secondary)]">No pipelines found for this project</p>
                                <p className="text-xs text-[var(--text-tertiary)] mt-1">Make sure the project has a CI/CD configuration (.gitlab-ci.yml, GitHub/Gitea Actions workflows in .github/workflows or .gitea/workflows)</p>
                              </div>
                            ) : (
                              <div className="space-y-2">
                                {pipelines.map(pipeline => (
                                  <div key={pipeline.id}>
                                    <div className="flex items-center justify-between p-3 bg-[var(--bg)] rounded-md">
                                      <div className="flex-1 min-w-0">
                                        <div className="flex items-center gap-2">
                                          <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[pipeline.status] || 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
                                            {pipeline.status}
                                          </span>
                                          <span className="text-sm text-[var(--text-primary)] font-mono">#{pipeline.id}</span>
                                          {pipeline.ref && <span className="text-xs text-[var(--text-secondary)]">{pipeline.ref}</span>}
                                        </div>
                                        {pipeline.sha && (
                                          <p className="text-xs text-[var(--text-tertiary)] mt-0.5 font-mono">{pipeline.sha.slice(0, 8)}</p>
                                        )}
                                      </div>
                                      <div className="flex gap-1 shrink-0">
                                        <button
                                          onClick={() => handleViewJobs(pipeline.id)}
                                          className={`px-2 py-1 text-xs rounded ${expandedPipelineId === pipeline.id ? 'bg-[var(--accent-subtle)] text-[var(--accent)]' : 'text-[var(--text-secondary)] hover:bg-[var(--border)]'}`}
                                        >
                                          {expandedPipelineId === pipeline.id ? 'Hide Jobs' : 'Jobs'}
                                        </button>
                                        <button
                                          onClick={() => openProviderTriggerModal(pipeline.ref)}
                                          className="px-2 py-1 text-xs text-emerald-600 hover:bg-emerald-500/10 rounded"
                                        >
                                          Run
                                        </button>
                                        {pipeline.url && (
                                          <a href={pipeline.url} target="_blank" rel="noopener noreferrer" className="px-2 py-1 text-xs text-[var(--accent)] hover:underline">
                                            View
                                          </a>
                                        )}
                                      </div>
                                    </div>
                                    {/* Expanded jobs panel */}
                                    {expandedPipelineId === pipeline.id && (
                                      <div className="ml-4 mt-1 mb-2 p-3 bg-[var(--surface)] border rounded-md">
                                        {loadingJobs ? (
                                          <div className="flex items-center justify-center py-4">
                                            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600" />
                                          </div>
                                        ) : jobsError ? (
                                          <div className="text-center py-3">
                                            <p className="text-sm text-red-500">Failed to load jobs</p>
                                            <p className="text-xs text-red-400 mt-1">{jobsError}</p>
                                            {jobsErrorHint && (
                                              <p className="text-xs text-amber-600 mt-1.5 font-medium">{jobsErrorHint}</p>
                                            )}
                                            <div className="flex items-center justify-center gap-2 mt-2">
                                              <button
                                                onClick={() => handleViewJobs(pipeline.id)}
                                                className="px-3 py-1 text-xs text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded border border-[var(--border)]"
                                              >
                                                Retry
                                              </button>
                                              {jobsErrorHint?.includes('Connections') && (
                                                <Link
                                                  href="/connections"
                                                  className="px-3 py-1 text-xs text-white bg-blue-600 hover:bg-blue-700 rounded"
                                                >
                                                  Go to Connections
                                                </Link>
                                              )}
                                            </div>
                                          </div>
                                        ) : pipelineJobs.length === 0 ? (
                                          <p className="text-sm text-[var(--text-secondary)] text-center py-2">No jobs found for this pipeline</p>
                                        ) : (
                                          <div className="space-y-1">
                                            {pipelineJobs.map(job => (
                                              <div key={job.id} className="flex items-center justify-between p-2 bg-[var(--bg)] rounded text-sm">
                                                <div className="flex items-center gap-2 flex-1 min-w-0">
                                                  <span className={`px-1.5 py-0.5 rounded text-xs font-medium ${statusColors[job.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                                                    {job.status}
                                                  </span>
                                                  <span className="text-[var(--text-primary)] truncate">{job.name}</span>
                                                  <span className="text-xs text-[var(--text-tertiary)]">{job.stage}</span>
                                                  {job.duration > 0 && <span className="text-xs text-[var(--text-tertiary)]">{(job.duration).toFixed(1)}s</span>}
                                                </div>
                                                <button
                                                  onClick={() => handleViewJobLog(job.id)}
                                                  className={`px-2 py-0.5 text-xs rounded shrink-0 ${selectedJobId === job.id ? 'bg-[var(--accent-subtle)] text-[var(--accent)]' : 'text-[var(--accent)] hover:bg-[var(--accent-subtle)]'}`}
                                                >
                                                  Log
                                                </button>
                                              </div>
                                            ))}
                                          </div>
                                        )}
                                        {/* Job log viewer */}
                                        {selectedJobId !== null && (
                                          <div className="mt-2 border rounded-md bg-gray-900 text-gray-100 p-3 max-h-80 overflow-auto">
                                            {loadingLog ? (
                                              <div className="flex items-center justify-center py-4">
                                                <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-400" />
                                              </div>
                                            ) : (
                                              <pre className="text-xs font-mono whitespace-pre-wrap break-all">{jobLog}</pre>
                                            )}
                                          </div>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                ))}
                              </div>
                            )}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Modals */}
      {showCreateModal && (
        <CreateSourceModal
          onClose={() => setShowCreateModal(false)}
          connections={connections}
          initialType={createModalInitialType}
          onCreated={async (sourceId) => {
            setShowCreateModal(false);
            await loadSources();
            if (sourceId) {
              try { await pipelineSources.resolveSchema(sourceId); await loadSources(); } catch {}
            }
          }}
        />
      )}

      {showTriggerModal && selectedSource && (
        <TriggerModal
          source={selectedSource}
          params={triggerParams}
          setParams={setTriggerParams}
          triggering={triggering}
          onTrigger={handleTrigger}
          onClose={() => { setShowTriggerModal(false); setEngineCIVariables([]); }}
          onSavedPreset={() => { loadPresets(selectedSource.id); }}
          ciVariables={engineCIVariables}
          loadingCIVars={loadingEngineCIVars}
        />
      )}

      {/* Provider Tab Trigger Modal */}
      {showProviderTrigger && selectedProviderConn && pickerValue.repo_id && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-[var(--surface)] rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[90vh] flex flex-col">
            <div className="p-6 pb-3 shrink-0">
              <h3 className="text-lg font-semibold mb-1">Run Pipeline</h3>
              <p className="text-xs text-[var(--text-secondary)]">
                {pickerValue.repo_full_name} via {selectedProviderConn.name}
              </p>
            </div>

            <div className="flex-1 overflow-y-auto px-6 space-y-3">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Ref (branch/tag)</label>
                <input
                  type="text"
                  value={providerTriggerRef}
                  onChange={e => setProviderTriggerRef(e.target.value)}
                  className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                  placeholder="main"
                />
              </div>

              <div className="pt-2 border-t">
                <p className="text-sm font-medium text-[var(--text-secondary)] mb-2">Variables</p>
                {loadingCIVars ? (
                  <div className="flex items-center gap-2 py-3">
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />
                    <span className="text-xs text-[var(--text-secondary)]">Loading CI variables from repository...</span>
                  </div>
                ) : ciVariables.filter(cv => {
                    // Filter out variables parsed from CI config files without meaningful descriptions
                    if (!cv.description) return true;
                    const generic = [
                      'From .gitlab-ci.yml',
                      'From GitHub Actions workflow',
                      'From Gitea Actions workflow',
                      'From bitbucket-pipelines.yml',
                      'From workflow:rules:variables',
                    ];
                    return !generic.includes(cv.description);
                  }).length > 0 ? (
                  <div className="space-y-2 mb-3">
                    {ciVariables.filter(cv => {
                      // Filter out variables parsed from CI config files without meaningful descriptions
                      if (!cv.description) return true;
                      const generic = [
                        'From .gitlab-ci.yml',
                        'From GitHub Actions workflow',
                        'From Gitea Actions workflow',
                        'From bitbucket-pipelines.yml',
                        'From workflow:rules:variables',
                      ];
                      return !generic.includes(cv.description);
                    }).map(ciVar => {
                      const isFileType = ciVar.type === 'file';
                      const hasOptions = ciVar.options && ciVar.options.length > 0;
                      const typeLabel = isFileType ? 'File' : hasOptions ? 'Choice' : 'Variable';
                      const typeBadgeColor = isFileType ? 'bg-amber-500/15 text-amber-600' : hasOptions ? 'bg-purple-500/15 text-purple-500' : 'bg-sky-500/15 text-sky-500';
                      const hasLongDesc = ciVar.description && ciVar.description.length > 50;
                      return (
                        <div key={ciVar.key} className="p-2.5 bg-[var(--bg)] rounded-md border border-[var(--border)]">
                          <div className={`flex items-center gap-2 ${hasLongDesc ? 'mb-1' : 'mb-1.5'}`}>
                            <span className={`text-xs px-1.5 py-0.5 rounded font-medium shrink-0 ${typeBadgeColor}`}>{typeLabel}</span>
                            <span className="text-sm font-mono font-medium text-[var(--text-primary)] shrink-0">{ciVar.key}</span>
                            {ciVar.required && <span className="text-xs text-red-500 shrink-0">*</span>}
                          </div>
                          {ciVar.description && (
                            <div className={`text-xs text-[var(--text-secondary)] ${hasLongDesc ? 'mb-1.5 whitespace-pre-line leading-relaxed' : 'mb-1.5'}`} title={ciVar.description}>
                              {ciVar.description}
                            </div>
                          )}
                          {isFileType ? (
                            <div className="flex items-center gap-2">
                              <input
                                type="text"
                                value={providerTriggerVars[ciVar.key] ?? ''}
                                onChange={e => setProviderTriggerVars(prev => ({ ...prev, [ciVar.key]: e.target.value }))}
                                className="flex-1 px-2.5 py-1.5 border rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                                placeholder="File content or path..."
                              />
                              <span className="text-xs text-[var(--text-tertiary)] shrink-0" title="This variable will be stored as a file in the CI runner">file</span>
                            </div>
                          ) : hasOptions ? (
                            <select
                              value={providerTriggerVars[ciVar.key] ?? ciVar.value ?? ''}
                              onChange={e => setProviderTriggerVars(prev => ({ ...prev, [ciVar.key]: e.target.value }))}
                              className="w-full px-2.5 py-1.5 border rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 bg-[var(--surface)]"
                            >
                              <option value="">-- Select --</option>
                              {ciVar.options!.map(opt => (
                                <option key={opt} value={opt}>{opt}</option>
                              ))}
                            </select>
                          ) : (
                            <input
                              type="text"
                              value={providerTriggerVars[ciVar.key] ?? ciVar.value ?? ''}
                              onChange={e => setProviderTriggerVars(prev => ({ ...prev, [ciVar.key]: e.target.value }))}
                              className="w-full px-2.5 py-1.5 border rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                              placeholder={ciVar.value || 'Enter value...'}
                            />
                          )}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="text-xs text-[var(--text-tertiary)] mb-2">No CI variables found. Add custom variables below.</p>
                )}

                {/* Additional custom variables */}
                <div className="flex gap-2">
                  <input type="text" placeholder="key" value={providerTriggerNewKey} onChange={e => setProviderTriggerNewKey(e.target.value)} className="flex-1 px-3 py-2 border rounded-md text-sm" />
                  <input type="text" placeholder="value" value={providerTriggerNewVal} onChange={e => setProviderTriggerNewVal(e.target.value)} className="flex-1 px-3 py-2 border rounded-md text-sm" />
                  <button
                    onClick={() => {
                      if (providerTriggerNewKey) {
                        setProviderTriggerVars(prev => ({ ...prev, [providerTriggerNewKey]: providerTriggerNewVal }));
                        setProviderTriggerNewKey('');
                        setProviderTriggerNewVal('');
                      }
                    }}
                    className="px-3 py-2 bg-[var(--border-light)] rounded-md text-sm hover:bg-[var(--border)]"
                  >
                    + Add
                  </button>
                </div>
                {/* Show extra custom variables not in ciVariables */}
                {Object.entries(providerTriggerVars).filter(([key]) => !ciVariables.some(cv => cv.key === key)).length > 0 && (
                  <div className="mt-2 space-y-1">
                    <p className="text-xs text-[var(--text-secondary)] font-medium">Custom variables:</p>
                    {Object.entries(providerTriggerVars).filter(([key]) => !ciVariables.some(cv => cv.key === key)).map(([key, val]) => (
                      <div key={key} className="flex items-center gap-2">
                        <span className="text-sm font-mono text-[var(--text-secondary)] min-w-[100px]">{key}</span>
                        <input
                          type="text"
                          value={val}
                          onChange={e => setProviderTriggerVars(prev => ({ ...prev, [key]: e.target.value }))}
                          className="flex-1 px-3 py-1.5 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                        />
                        <button
                          onClick={() => setProviderTriggerVars(prev => { const n = { ...prev }; delete n[key]; return n; })}
                          className="px-2 py-1 text-xs text-red-500 hover:bg-red-500/10 rounded"
                        >
                          ✕
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Save preset section */}
              {showProviderPresetSave ? (
                <div className="pt-2 border-t">
                  <p className="text-sm font-medium text-[var(--text-secondary)] mb-2">Save as Preset</p>
                  <div className="space-y-2">
                    <input type="text" placeholder="Preset name" value={providerPresetName} onChange={e => setProviderPresetName(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
                    <input type="text" placeholder="Description (optional)" value={providerPresetDesc} onChange={e => setProviderPresetDesc(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
                    <div className="flex gap-2">
                      <button onClick={saveProviderPreset} disabled={!providerPresetName.trim()} className="px-3 py-1.5 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50">Save</button>
                      <button onClick={() => setShowProviderPresetSave(false)} className="px-3 py-1.5 text-[var(--text-secondary)] text-sm hover:bg-[var(--border)] rounded-md">Cancel</button>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="pt-2 border-t">
                  <button onClick={() => setShowProviderPresetSave(true)} className="text-sm text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-md px-3 py-1.5">
                    Save Parameters as Preset
                  </button>
                </div>
              )}
            </div>

            <div className="flex justify-end gap-2 p-6 pt-3 border-t shrink-0">
              <button onClick={() => setShowProviderTrigger(false)} className="px-4 py-2 text-[var(--text-secondary)] bg-[var(--border-light)] rounded-md hover:bg-[var(--border)] text-sm">Cancel</button>
              <button onClick={handleProviderTrigger} disabled={providerTriggering} className="px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 text-sm font-medium disabled:opacity-50">
                {providerTriggering ? 'Triggering...' : 'Trigger Pipeline'}
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
}


// ── Create Source Modal ────────────────────────────────────

function CreateSourceModal({ onClose, onCreated, connections, initialType }: { onClose: () => void; onCreated: (sourceId?: string) => void; connections: Connection[]; initialType?: string }) {
  useEscapeKey(onClose);
  const [name, setName] = useState('');
  const [sourceType, setSourceType] = useState(initialType || 'gitlab_ci');
  const [description, setDescription] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [selectedProviderId, setSelectedProviderId] = useState('');
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();

  const [glProjectId, setGlProjectId] = useState('');
  const [glBaseUrl, setGlBaseUrl] = useState('https://gitlab.com');
  const [glToken, setGlToken] = useState('');
  const [glRef, setGlRef] = useState('main');

  const [ansRepoUrl, setAnsRepoUrl] = useState('');
  const [ansLocalPath, setAnsLocalPath] = useState('');
  const [ansSourceMode, setAnsSourceMode] = useState<'repo' | 'local'>('repo');
  const [ansPlaybook, setAnsPlaybook] = useState('site.yml');
  const [ansInventory, setAnsInventory] = useState('inventory');
  const [ansToken, setAnsToken] = useState('');

  const [tfRepoUrl, setTfRepoUrl] = useState('');
  const [tfLocalPath, setTfLocalPath] = useState('');
  const [tfSourceMode, setTfSourceMode] = useState<'repo' | 'local'>('repo');
  const [tfWorkDir, setTfWorkDir] = useState('.');
  const [tfToken, setTfToken] = useState('');

  // GitHub Actions form state
  const [ghOwner, setGhOwner] = useState('');
  const [ghRepo, setGhRepo] = useState('');
  const [ghToken, setGhToken] = useState('');
  const [ghWorkflow, setGhWorkflow] = useState('');
  const [ghRef, setGhRef] = useState('main');

  // Trivy form state
  const [trivyTarget, setTrivyTarget] = useState('');
  const [trivyScanType, setTrivyScanType] = useState<'image' | 'fs' | 'repo' | 'config'>('image');
  const [trivySeverity, setTrivySeverity] = useState('MEDIUM,HIGH,CRITICAL');
  const [trivyIgnoreUnfixed, setTrivyIgnoreUnfixed] = useState(false);

  const selectedProvider = connections.find(c => c.id === selectedProviderId) || null;

  const handleProviderSelect = (providerId: string) => {
    setSelectedProviderId(providerId);
    const provider = connections.find(c => c.id === providerId);
    if (!provider) return;
    const cfg = provider.config as Record<string, unknown>;
    // Auto-detect source type from provider
    if (provider.type === 'gitlab') setSourceType('gitlab_ci');
    // Auto-fill URL and token from provider config
    if (cfg.url) setGlBaseUrl(String(cfg.url));
    if (cfg.token) setGlToken(String(cfg.token));
  };

  const getConfig = (): Record<string, string> => {
    let baseConfig: Record<string, string>;
    switch (sourceType) {
      case 'gitlab_ci':
        baseConfig = { project_id: glProjectId, base_url: glBaseUrl, token: glToken, ref: glRef };
        break;
      case 'ansible':
        baseConfig = ansSourceMode === 'local'
          ? { local_path: ansLocalPath, playbook: ansPlaybook, inventory: ansInventory }
          : { repo_url: ansRepoUrl, playbook: ansPlaybook, inventory: ansInventory, token: ansToken };
        break;
      case 'terraform':
        baseConfig = tfSourceMode === 'local'
          ? { local_path: tfLocalPath, working_dir: tfWorkDir }
          : { repo_url: tfRepoUrl, working_dir: tfWorkDir, token: tfToken };
        break;
      case 'github_actions':
        baseConfig = { owner: ghOwner, repo: ghRepo, token: ghToken, workflow: ghWorkflow, ref: ghRef };
        break;
      case 'trivy':
        baseConfig = { target: trivyTarget, scan_type: trivyScanType, severity: trivySeverity, ignore_unfixed: trivyIgnoreUnfixed ? 'true' : 'false' };
        break;
      default:
        baseConfig = {};
    }
    for (const [field, ref] of Object.entries(vaultRefs)) {
      if (ref) baseConfig[field] = ref;
    }
    return baseConfig;
  };

  const validate = () => {
    if (!name) return 'Name is required';
    if (sourceType === 'gitlab_ci' && (!glProjectId || (!glToken && !vaultRefs.token))) return 'Project ID and Token are required for GitLab CI';
    if (sourceType === 'ansible' && ansSourceMode === 'repo' && (!ansRepoUrl || !ansPlaybook)) return 'Repository URL and Playbook are required for Ansible';
    if (sourceType === 'ansible' && ansSourceMode === 'local' && (!ansLocalPath || !ansPlaybook)) return 'Local Path and Playbook are required for Ansible';
    if (sourceType === 'terraform' && tfSourceMode === 'repo' && !tfRepoUrl) return 'Repository URL is required for Terraform';
    if (sourceType === 'terraform' && tfSourceMode === 'local' && !tfLocalPath) return 'Local Path is required for Terraform';
    if (sourceType === 'github_actions' && (!ghOwner || !ghRepo || (!ghToken && !vaultRefs.token) || !ghWorkflow)) return 'Owner, Repo, Token, and Workflow are required for GitHub Actions';
    if (sourceType === 'trivy' && !trivyTarget) return 'Target is required for Trivy';
    return '';
  };

  const handleCreate = async () => {
    const validationError = validate();
    if (validationError) { setError(validationError); return; }
    setSaving(true);
    setError('');
    try {
      const payload: { name: string; source_type: string; description?: string; connection_id?: string; config?: Record<string, unknown> } = {
        name,
        source_type: sourceType,
        description,
        config: getConfig(),
      };
      if (selectedProviderId) {
        payload.connection_id = selectedProviderId;
      }
      await pipelineSources.create(payload).then(source => {
        onCreated(source?.id);
      });
    } catch (err) {
      setError(friendlyError(err).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-[var(--surface)] rounded-lg shadow-xl max-w-md w-full mx-4 p-6 max-h-[90vh] overflow-y-auto">
        <h3 className="text-lg font-semibold mb-4">Add Engine</h3>
        {error && <p className="text-sm text-red-600 mb-3">{error}</p>}
        <div className="space-y-3">
          {/* Engine type visual selector */}
          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Engine Type</label>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
              {Object.entries(ENGINE_TYPE_INFO).map(([type, info]) => (
                <button
                  key={type}
                  type="button"
                  onClick={() => setSourceType(type)}
                  className={`p-3 rounded-lg border-2 text-left transition-all ${sourceType === type ? `${info.borderColor} ${info.bgColor}` : 'border-[var(--border)] hover:border-[var(--border)] bg-[var(--surface)]'}`}
                >
                  <div className="text-xl mb-1"><BrandIcon name={info.icon} size={20} /></div>
                  <div className={`text-sm font-medium ${sourceType === type ? info.color : 'text-[var(--text-primary)]'}`}>{info.label}</div>
                  <div className="text-xs text-[var(--text-secondary)] mt-0.5 leading-tight">{info.description}</div>
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Name</label>
            <input type="text" value={name} onChange={e => setName(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" placeholder="My Automation Engine" />
          </div>

          {/* Provider selector */}
          {connections.length > 0 && (
            <div>
              <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Git Connection <span className="text-xs text-[var(--text-tertiary)] font-normal">(optional)</span></label>
              <select
                value={selectedProviderId}
                onChange={e => handleProviderSelect(e.target.value)}
                className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="">— Direct connection (no Git provider) —</option>
                {connections.map(c => {
                  const info = getConnTypeInfo(c);
                  return (
                    <option key={c.id} value={c.id}>
                      {c.name} ({info.label})
                    </option>
                  );
                })}
              </select>
              {selectedProvider && (
                <p className="text-xs text-green-600 mt-1">
                  ✓ URL and token will be filled from provider &quot;{selectedProvider.name}&quot;
                </p>
              )}
            </div>
          )}

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Description</label>
            <input type="text" value={description} onChange={e => setDescription(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="Optional description" />
          </div>

          {sourceType === 'gitlab_ci' && (
            <>
              <div className="pt-2 border-t"><p className="text-xs text-[var(--text-secondary)] mb-2">GitLab CI Configuration</p></div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Project ID <span className="text-red-500">*</span></label>
                <input type="text" value={glProjectId} onChange={e => setGlProjectId(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="12345" />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">Numeric project ID from your GitLab instance</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">
                  GitLab URL
                  {selectedProvider && <span className="text-xs text-green-600 ml-1 font-normal">(from provider)</span>}
                </label>
                <input type="text" value={glBaseUrl} onChange={e => setGlBaseUrl(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
              </div>
              <VaultInput label="Access Token" field="token" value={glToken} onChange={setGlToken} vaultRef={vaultRefs.token} onOpenVault={onOpenVaultPicker} onRemoveVault={removeVaultRef} placeholder="glpat-..." required />
              {selectedProvider && <p className="text-xs text-green-600 -mt-2">Token filled from provider</p>}
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Default Branch</label>
                <input type="text" value={glRef} onChange={e => setGlRef(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
              </div>
            </>
          )}

          {sourceType === 'ansible' && (
            <>
              <div className="pt-2 border-t"><p className="text-xs text-[var(--text-secondary)] mb-2">Ansible Configuration</p></div>
              {/* Source mode toggle */}
              <div className="flex rounded-lg border border-[var(--border)] overflow-hidden">
                <button
                  type="button"
                  onClick={() => setAnsSourceMode('repo')}
                  className={`flex-1 px-3 py-1.5 text-xs font-medium transition-colors ${ansSourceMode === 'repo' ? 'bg-emerald-500/10 text-emerald-600 border-r border-[var(--border)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)] border-r border-[var(--border)]'}`}
                >
                  📦 Repository
                </button>
                <button
                  type="button"
                  onClick={() => setAnsSourceMode('local')}
                  className={`flex-1 px-3 py-1.5 text-xs font-medium transition-colors ${ansSourceMode === 'local' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)]'}`}
                >
                  💻 Local Path
                </button>
              </div>
              {ansSourceMode === 'repo' ? (
                <>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Repository URL <span className="text-red-500">*</span></label>
                    <input type="text" value={ansRepoUrl} onChange={e => setAnsRepoUrl(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="https://github.com/org/ansible-repo.git" />
                  </div>
                  <VaultInput label="Git/Repo Token" field="token" value={ansToken} onChange={setAnsToken} vaultRef={vaultRefs.token} onOpenVault={onOpenVaultPicker} onRemoveVault={removeVaultRef} placeholder="Optional: for private repos" />
                  {selectedProvider && <p className="text-xs text-green-600 -mt-2">Token filled from provider</p>}
                </>
              ) : (
                <div>
                  <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Local Path <span className="text-red-500">*</span></label>
                  <input type="text" value={ansLocalPath} onChange={e => setAnsLocalPath(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm font-mono" placeholder="/opt/ansible/playbooks" />
                  <p className="text-xs text-[var(--text-tertiary)] mt-1">Absolute path to a local directory with playbooks on the server</p>
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Playbook <span className="text-red-500">*</span></label>
                <input type="text" value={ansPlaybook} onChange={e => setAnsPlaybook(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="site.yml" />
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Inventory</label>
                <input type="text" value={ansInventory} onChange={e => setAnsInventory(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="inventory" />
              </div>
            </>
          )}

          {sourceType === 'terraform' && (
            <>
              <div className="pt-2 border-t"><p className="text-xs text-[var(--text-secondary)] mb-2">Terraform Configuration</p></div>
              {/* Source mode toggle */}
              <div className="flex rounded-lg border border-[var(--border)] overflow-hidden">
                <button
                  type="button"
                  onClick={() => setTfSourceMode('repo')}
                  className={`flex-1 px-3 py-1.5 text-xs font-medium transition-colors ${tfSourceMode === 'repo' ? 'bg-purple-500/10 text-purple-500 border-r border-[var(--border)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)] border-r border-[var(--border)]'}`}
                >
                  📦 Repository
                </button>
                <button
                  type="button"
                  onClick={() => setTfSourceMode('local')}
                  className={`flex-1 px-3 py-1.5 text-xs font-medium transition-colors ${tfSourceMode === 'local' ? 'bg-purple-500/10 text-purple-500' : 'bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)]'}`}
                >
                  💻 Local Path
                </button>
              </div>
              {tfSourceMode === 'repo' ? (
                <>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Repository URL <span className="text-red-500">*</span></label>
                    <input type="text" value={tfRepoUrl} onChange={e => setTfRepoUrl(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="https://github.com/org/terraform-repo.git" />
                  </div>
                  <VaultInput label="Git/Repo Token" field="token" value={tfToken} onChange={setTfToken} vaultRef={vaultRefs.token} onOpenVault={onOpenVaultPicker} onRemoveVault={removeVaultRef} placeholder="Optional: for private repos" />
                  {selectedProvider && <p className="text-xs text-green-600 -mt-2">Token filled from provider</p>}
                </>
              ) : (
                <div>
                  <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Local Path <span className="text-red-500">*</span></label>
                  <input type="text" value={tfLocalPath} onChange={e => setTfLocalPath(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm font-mono" placeholder="/opt/terraform/stacks" />
                  <p className="text-xs text-[var(--text-tertiary)] mt-1">Absolute path to a local directory with .tf files on the server</p>
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Working Directory</label>
                <input type="text" value={tfWorkDir} onChange={e => setTfWorkDir(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="." />
              </div>
            </>
          )}

          {sourceType === 'github_actions' && (
            <>
              <div className="pt-2 border-t"><p className="text-xs text-[var(--text-secondary)] mb-2">GitHub Actions Configuration</p></div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Owner <span className="text-red-500">*</span></label>
                <input type="text" value={ghOwner} onChange={e => setGhOwner(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="octocat" />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">GitHub repository owner (user or organization)</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Repository <span className="text-red-500">*</span></label>
                <input type="text" value={ghRepo} onChange={e => setGhRepo(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="hello-world" />
              </div>
              <VaultInput label="GitHub Token" field="token" value={ghToken} onChange={setGhToken} vaultRef={vaultRefs.token} onOpenVault={onOpenVaultPicker} onRemoveVault={removeVaultRef} placeholder="ghp_..." required />
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Workflow File <span className="text-red-500">*</span></label>
                <input type="text" value={ghWorkflow} onChange={e => setGhWorkflow(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="ci.yml" />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">Workflow filename from .github/workflows/</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Ref (branch/tag)</label>
                <input type="text" value={ghRef} onChange={e => setGhRef(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="main" />
              </div>
            </>
          )}

          {sourceType === 'trivy' && (
            <>
              <div className="pt-2 border-t"><p className="text-xs text-[var(--text-secondary)] mb-2">Trivy Scanner Configuration</p></div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Target <span className="text-red-500">*</span></label>
                <input type="text" value={trivyTarget} onChange={e => setTrivyTarget(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="nginx:latest" />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">Image name, filesystem path, or repository URL</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Scan Type</label>
                <select value={trivyScanType} onChange={e => setTrivyScanType(e.target.value as 'image' | 'fs' | 'repo' | 'config')} className="w-full px-3 py-2 border rounded-md text-sm">
                  <option value="image">Image</option>
                  <option value="fs">Filesystem</option>
                  <option value="repo">Repository</option>
                  <option value="config">Config (IaC)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Severity Levels</label>
                <input type="text" value={trivySeverity} onChange={e => setTrivySeverity(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" placeholder="UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL" />
              </div>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={trivyIgnoreUnfixed} onChange={e => setTrivyIgnoreUnfixed(e.target.checked)} className="rounded" />
                <span className="text-sm text-[var(--text-secondary)]">Ignore unfixed vulnerabilities</span>
              </label>
            </>
          )}
        </div>
        <div className="flex justify-end gap-2 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-[var(--text-secondary)] bg-[var(--border-light)] rounded-md hover:bg-[var(--border)] text-sm">Cancel</button>
          <button onClick={handleCreate} disabled={saving} className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm font-medium disabled:opacity-50">
            {saving ? 'Creating...' : 'Create'}
          </button>
        </div>
      </div>
      {VaultPicker}
    </div>
  );
}

// ── Trigger Modal ──────────────────────────────────────────

interface SchemaProperty {
  type?: string;
  description?: string;
  default?: unknown;
  enum?: string[];
}

function TriggerModal({
  source, params, setParams, triggering, onTrigger, onClose, onSavedPreset,
  ciVariables, loadingCIVars,
}: {
  source: PipelineSource;
  params: Record<string, string>;
  setParams: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  triggering: boolean;
  onTrigger: () => void;
  onClose: () => void;
  onSavedPreset: () => void;
  ciVariables?: CIVariable[];
  loadingCIVars?: boolean;
}) {
  useEscapeKey(onClose);
  const [showSavePreset, setShowSavePreset] = useState(false);
  const [presetName, setPresetName] = useState('');
  const [presetDesc, setPresetDesc] = useState('');
  const [savingPreset, setSavingPreset] = useState(false);
  const [presetError, setPresetError] = useState('');
  const [newKey, setNewKey] = useState('');
  const [newVal, setNewVal] = useState('');

  const schema = source.parameter_schema as { properties?: Record<string, SchemaProperty>; required?: string[] } | undefined;
  const properties = schema?.properties || {};
  const requiredFields = new Set(schema?.required || []);

  // Use dynamically loaded CI variables if available
  const hasCIVars = ciVariables && ciVariables.length > 0;
  // Filter out generic descriptions from CI variables (same as Providers tab)
  const genericDescriptions = ['From .gitlab-ci.yml', 'From GitHub Actions workflow', 'From Gitea Actions workflow', 'From bitbucket-pipelines.yml', 'From workflow:rules:variables'];
  const filteredCIVars = hasCIVars ? ciVariables.filter(cv => {
    if (!cv.description) return true;
    return !genericDescriptions.includes(cv.description);
  }) : [];

  const handleSavePreset = async () => {
    if (!presetName.trim()) return;
    setSavingPreset(true);
    try {
      const paramValues: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(params)) {
        if (v.trim()) paramValues[k] = v;
      }
      await pipelinePresets.create(source.id, { name: presetName, description: presetDesc, parameters: paramValues });
      onSavedPreset();
      setShowSavePreset(false);
      setPresetName('');
      setPresetDesc('');
    } catch (err) { setPresetError(friendlyError(err).message); }
    finally { setSavingPreset(false); }
  };

  const renderInput = (key: string, prop: SchemaProperty) => {
    const value = params[key] ?? (prop.default != null ? String(prop.default) : '');
    const isRequired = requiredFields.has(key);

    if (prop.enum && prop.enum.length > 0) {
      return (
        <select value={value} onChange={e => setParams(prev => ({ ...prev, [key]: e.target.value }))} className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
          <option value="">-- Select --</option>
          {prop.enum.map(opt => <option key={opt} value={opt}>{opt}</option>)}
        </select>
      );
    }
    if (prop.type === 'boolean') {
      return (
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={value === 'true'} onChange={e => setParams(prev => ({ ...prev, [key]: e.target.checked ? 'true' : 'false' }))} className="rounded" />
          <span className="text-sm text-[var(--text-secondary)]">{value === 'true' ? 'Yes' : 'No'}</span>
        </label>
      );
    }
    if (prop.type === 'number') {
      return (
        <input type="number" value={value} onChange={e => setParams(prev => ({ ...prev, [key]: e.target.value }))} className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" placeholder={prop.default != null ? String(prop.default) : ''} />
      );
    }
    return (
      <input type="text" value={value} onChange={e => setParams(prev => ({ ...prev, [key]: e.target.value }))} className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" placeholder={prop.default != null ? String(prop.default) : ''} />
    );
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-[var(--surface)] rounded-lg shadow-xl max-w-lg w-full mx-4 max-h-[90vh] flex flex-col">
        <div className="p-6 pb-3 shrink-0">
          <h3 className="text-lg font-semibold mb-1">Run Pipeline: {source.name}</h3>
          <p className="text-xs text-[var(--text-secondary)]">
            {source.source_type === 'gitlab_ci' ? 'GitLab CI' : source.source_type === 'github_actions' ? 'GitHub Actions' : source.source_type === 'ansible' ? 'Ansible' : source.source_type === 'terraform' ? 'Terraform' : source.source_type === 'trivy' ? 'Trivy Scanner' : source.source_type}
            {' '}&mdash; Parameters auto-detected from config files
          </p>
        </div>

        <div className="flex-1 overflow-y-auto px-6 space-y-3">
          {/* Loading CI variables */}
          {loadingCIVars && (
            <div className="flex items-center gap-2 py-3">
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600" />
              <span className="text-xs text-[var(--text-secondary)]">Loading CI variables from repository...</span>
            </div>
          )}

          {/* Dynamic CI variables (from plugin) */}
          {hasCIVars && !loadingCIVars && (
            <>
              {filteredCIVars.map(cv => (
                <div key={cv.key}>
                  <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">
                    {cv.key}
                    <span className="ml-1.5 text-xs font-normal text-[var(--text-tertiary)]">
                      {cv.type === 'file' ? 'file' : 'env_var'}
                    </span>
                  </label>
                  {cv.description && !genericDescriptions.includes(cv.description) && (
                    <p className="text-xs text-[var(--text-secondary)] mb-1">{cv.description}</p>
                  )}
                  {cv.options && cv.options.length > 0 ? (
                    <select
                      value={params[cv.key] ?? cv.value ?? ''}
                      onChange={e => setParams(prev => ({ ...prev, [cv.key]: e.target.value }))}
                      className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                    >
                      <option value="">-- Select --</option>
                      {cv.options.map(opt => <option key={opt} value={opt}>{opt}</option>)}
                    </select>
                  ) : (
                    <input
                      type="text"
                      value={params[cv.key] ?? cv.value ?? ''}
                      onChange={e => setParams(prev => ({ ...prev, [cv.key]: e.target.value }))}
                      className="w-full px-3 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                      placeholder={cv.value || ''}
                    />
                  )}
                </div>
              ))}
            </>
          )}

          {/* Schema-based properties (fallback when no CI vars loaded) */}
          {!hasCIVars && !loadingCIVars && (
            <>
              {Object.keys(properties).length === 0 && (
                <p className="text-sm text-[var(--text-secondary)]">No parameters defined. Click &quot;Refresh Schema&quot; to auto-detect parameters.</p>
              )}
              {Object.entries(properties).map(([key, prop]) => (
                <div key={key}>
                  <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">
                    {key}{requiredFields.has(key) && <span className="text-red-500 ml-0.5">*</span>}
                  </label>
                  {prop.description && <p className="text-xs text-[var(--text-secondary)] mb-1">{prop.description}</p>}
                  {renderInput(key, prop)}
                </div>
              ))}
            </>
          )}
          <div className="pt-2 border-t">
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Add Parameter</label>
            <div className="flex gap-2">
              <input type="text" placeholder="key" value={newKey} onChange={e => setNewKey(e.target.value)} className="flex-1 px-3 py-2 border rounded-md text-sm" />
              <input type="text" placeholder="value" value={newVal} onChange={e => setNewVal(e.target.value)} className="flex-1 px-3 py-2 border rounded-md text-sm" />
              <button onClick={() => { if (newKey) { setParams(prev => ({ ...prev, [newKey]: newVal })); setNewKey(''); setNewVal(''); } }} className="px-3 py-2 bg-[var(--border-light)] rounded-md text-sm hover:bg-[var(--border)]">Add</button>
            </div>
          </div>
        </div>

        {showSavePreset && (
          <div className="mx-6 mt-2 p-3 bg-[var(--bg)] rounded-lg border shrink-0">
            <p className="text-sm font-medium text-[var(--text-secondary)] mb-2">Save as Preset</p>
            {presetError && <div className="mb-2 p-2 rounded-md bg-red-500/10 border border-red-500/20 text-xs text-red-400">{presetError}</div>}
            <div className="space-y-2">
              <input type="text" placeholder="Preset name" value={presetName} onChange={e => setPresetName(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
              <input type="text" placeholder="Description (optional)" value={presetDesc} onChange={e => setPresetDesc(e.target.value)} className="w-full px-3 py-2 border rounded-md text-sm" />
              <div className="flex gap-2">
                <button onClick={handleSavePreset} disabled={savingPreset || !presetName.trim()} className="px-3 py-1.5 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50">{savingPreset ? 'Saving...' : 'Save Preset'}</button>
                <button onClick={() => setShowSavePreset(false)} className="px-3 py-1.5 text-[var(--text-secondary)] text-sm hover:bg-[var(--border)] rounded-md">Cancel</button>
              </div>
            </div>
          </div>
        )}

        <div className="flex justify-between items-center p-6 pt-3 border-t shrink-0">
          <button onClick={() => setShowSavePreset(!showSavePreset)} className="px-3 py-2 text-sm text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-md">
            {showSavePreset ? 'Hide Preset Form' : 'Save as Preset'}
          </button>
          <div className="flex gap-2">
            <button onClick={onClose} className="px-4 py-2 text-[var(--text-secondary)] bg-[var(--border-light)] rounded-md hover:bg-[var(--border)] text-sm">Cancel</button>
            <button onClick={onTrigger} disabled={triggering} className="px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 text-sm font-medium disabled:opacity-50">
              {triggering ? 'Triggering...' : 'Trigger Pipeline'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

