'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import Link from 'next/link';
import { clusters, deployments, blueprints as blueprintsAPI, blueprintGroups as blueprintGroupsAPI, dockerHosts, type Cluster, type ServiceBlueprint, type BlueprintGroup, type DockerHost } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';

interface PipelineItem {
  id: string;
  blueprint: ServiceBlueprint;
  namespace: string;
  enabled: boolean;
  groupName?: string; // from which group this was added
}

const categoryIcons: Record<string, string> = {
  backend: '⚙️', frontend: '🌐', database: '🗄️', messaging: '📨',
  monitoring: '📊', security: '🔒', cache: '⚡', storage: '💾', general: '📦',
};

export default function PipelineBuilderPage() {
  const [blueprints, setBlueprints] = useState<ServiceBlueprint[]>([]);
  const [pipeline, setPipeline] = useState<PipelineItem[]>([]);
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [selectedClusterId, setSelectedClusterId] = useState('');
  const [globalNamespace, setGlobalNamespace] = useState('default');
  const [deploying, setDeploying] = useState(false);
  const [deployStatus, setDeployStatus] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [deployLog, setDeployLog] = useState<string[]>([]);
  const [bpSearch, setBpSearch] = useState('');
  const [dragIdx, setDragIdx] = useState<number | null>(null);
  const [dragOverIdx, setDragOverIdx] = useState<number | null>(null);
  const [showValuesFor, setShowValuesFor] = useState<string | null>(null);
  const deployLogRef = useRef<HTMLDivElement>(null);

  // Groups & Docker support
  const [groups, setGroups] = useState<BlueprintGroup[]>([]);
  const [dockerHostList, setDockerHostList] = useState<DockerHost[]>([]);
  const [targetType, setTargetType] = useState<'kubernetes' | 'docker'>('kubernetes');
  const [selectedDockerHostId, setSelectedDockerHostId] = useState('');
  const [groupsExpanded, setGroupsExpanded] = useState(true);

  useEffect(() => {
    blueprintsAPI.list().then(res => setBlueprints(res.blueprints || [])).catch(() => {});
    clusters.list().then(d => setClusterList(d.clusters || [])).catch(() => {});
    blueprintGroupsAPI.list().then(res => setGroups(res.groups || [])).catch(() => {});
    dockerHosts.list().then(res => setDockerHostList(res.docker_hosts || [])).catch(() => {});
  }, []);

  const addToPipeline = (bp: ServiceBlueprint, groupName?: string) => {
    if (pipeline.some(p => p.blueprint.id === bp.id)) return;
    setPipeline([...pipeline, {
      id: `pi-${Date.now()}`,
      blueprint: bp,
      namespace: globalNamespace,
      enabled: true,
      groupName,
    }]);
  };

  const addGroupToPipeline = (group: BlueprintGroup) => {
    const newItems: PipelineItem[] = [];
    for (const bp of group.blueprints) {
      if (!pipeline.some(p => p.blueprint.id === bp.id) && !newItems.some(ni => ni.blueprint.id === bp.id)) {
        newItems.push({
          id: `pi-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
          blueprint: bp,
          namespace: globalNamespace,
          enabled: true,
          groupName: group.name,
        });
      }
    }
    if (newItems.length > 0) setPipeline([...pipeline, ...newItems]);
  };

  const removeFromPipeline = (id: string) => {
    setPipeline(pipeline.filter(p => p.id !== id));
  };

  const toggleEnabled = (id: string) => {
    setPipeline(pipeline.map(p => p.id === id ? { ...p, enabled: !p.enabled } : p));
  };

  const updateItemNamespace = (id: string, ns: string) => {
    setPipeline(pipeline.map(p => p.id === id ? { ...p, namespace: ns } : p));
  };

  // Drag & drop reorder
  const handleDragStart = (idx: number) => setDragIdx(idx);
  const handleDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault();
    setDragOverIdx(idx);
  };
  const handleDrop = (idx: number) => {
    if (dragIdx === null || dragIdx === idx) { setDragIdx(null); setDragOverIdx(null); return; }
    const next = [...pipeline];
    const [moved] = next.splice(dragIdx, 1);
    next.splice(idx, 0, moved);
    setPipeline(next);
    setDragIdx(null);
    setDragOverIdx(null);
  };
  const handleDragEnd = () => { setDragIdx(null); setDragOverIdx(null); };

  const moveItem = (idx: number, dir: -1 | 1) => {
    const newIdx = idx + dir;
    if (newIdx < 0 || newIdx >= pipeline.length) return;
    const next = [...pipeline];
    [next[idx], next[newIdx]] = [next[newIdx], next[idx]];
    setPipeline(next);
  };

  const handleDeploy = async () => {
    const enabledItems = pipeline.filter(p => p.enabled);
    if (enabledItems.length === 0) {
      setDeployStatus({ type: 'error', text: 'Add at least one service to the pipeline' });
      return;
    }

    // Validate target
    if (targetType === 'kubernetes' && !selectedClusterId) {
      setDeployStatus({ type: 'error', text: 'Select a target cluster first' });
      return;
    }
    if (targetType === 'docker' && !selectedDockerHostId) {
      setDeployStatus({ type: 'error', text: 'Select a Docker host first' });
      return;
    }

    setDeploying(true);
    setDeployStatus(null);
    const logLines: string[] = [];
    const submittedIds: string[] = [];

    const addLog = (msg: string) => {
      logLines.push(`[${new Date().toLocaleTimeString()}] ${msg}`);
      setDeployLog([...logLines]);
      if (deployLogRef.current) deployLogRef.current.scrollTop = deployLogRef.current.scrollHeight;
    };

    try {
      // Docker target: deploy docker_compose blueprints to Docker host
      if (targetType === 'docker') {
        const dockerItems = enabledItems.filter(i => i.blueprint.source_type === 'docker_compose');
        const nonDockerItems = enabledItems.filter(i => i.blueprint.source_type !== 'docker_compose');
        if (nonDockerItems.length > 0) {
          addLog(`⚠ Skipping ${nonDockerItems.length} non-Compose blueprint(es) (not compatible with Docker target)`);
        }
        if (dockerItems.length === 0) {
          addLog('⚠ No docker_compose blueprints in pipeline to deploy');
          setDeployStatus({ type: 'error', text: 'No docker_compose blueprints to deploy' });
          setDeploying(false);
          return;
        }
        for (let i = 0; i < dockerItems.length; i++) {
          const item = dockerItems[i];
          const bp = item.blueprint;
          addLog(`Deploying ${i + 1}/${dockerItems.length}: ${bp.name} → Docker host`);
          try {
            await blueprintsAPI.deployDocker(bp.id, selectedDockerHostId);
            addLog(`✓ ${bp.name} deployed to Docker host`);
          } catch (err) {
            addLog(`✗ ${bp.name} failed: ${err instanceof Error ? err.message : 'Unknown error'}`);
          }
        }
        setDeployStatus({ type: 'success', text: `Docker deployment complete: ${dockerItems.length} service(s).` });
        setDeploying(false);
        return;
      }

      // Kubernetes target
      for (let i = 0; i < enabledItems.length; i++) {
        const item = enabledItems[i];
        const bp = item.blueprint;

        // Skip docker_compose blueprints when deploying to K8s
        if (bp.source_type === 'docker_compose') {
          addLog(`⚠ Skipping ${bp.name} (docker_compose — use Docker target)`);
          continue;
        }

        addLog(`Deploying ${i + 1}/${enabledItems.length}: ${bp.name} → ${item.namespace}`);

        if (bp.source_type !== 'container' && !bp.chart_name) {
          addLog(`⚠ WARNING: ${bp.name} has no chart_name set. Edit the blueprint to add it.`);
        }

        const result = await deployments.create({
          jira_issue_key: `PIPELINE-${Date.now()}`,
          jira_summary: `Pipeline deploy: ${bp.name}`,
          gitlab_project_name: bp.name,
          image_repository: bp.image ? bp.image.split(':')[0] : bp.chart_url,
          image_tag: bp.image ? bp.image.split(':')[1] || 'latest' : (bp.chart_version || 'latest'),
          target_namespace: item.namespace,
          target_cluster_id: selectedClusterId,
          deploy_type: bp.source_type === 'container' ? 'helm' : bp.source_type.replace('helm_', 'helm-'),
          replicas: bp.replicas,
          strategy: 'rolling',
          spec: {
            containers: bp.image ? [{ name: bp.name, image: bp.image, cpu: bp.cpu, memory: bp.memory, ports: bp.ports.map(p => ({ containerPort: p })) }] : [],
            values_yaml: bp.values_yaml,
            service: { port: bp.ports[0] || 80, type: 'ClusterIP' },
            chart: {
              source_type: bp.source_type,
              chart_url: bp.chart_url,
              chart_name: bp.chart_name,
              chart_version: bp.chart_version,
              chart_path: bp.chart_path,
            },
          },
          status: 'pending',
        });

        if (result?.id) submittedIds.push(result.id);
        addLog(`✓ ${bp.name} submitted (ID: ${(result?.id || 'unknown').slice(0, 8)})`);
      }

      // Poll deployment status for submitted deployments
      if (submittedIds.length > 0) {
        addLog(`\u23F3 Waiting for ${submittedIds.length} deployment(s) to complete...`);
        const maxWait = 120000; // 2 minutes max
        const pollInterval = 3000; // poll every 3s
        const startTime = Date.now();

        while (Date.now() - startTime < maxWait) {
          await new Promise(r => setTimeout(r, pollInterval));
          let allDone = true;

          for (const depId of submittedIds) {
            try {
              const dep = await deployments.get(depId);
              if (dep.status === 'deployed' || dep.status === 'promoted') {
                addLog(`\u2713 ${dep.jira_summary || depId.slice(0, 8)}: ${dep.status}`);
                if (dep.logs) {
                  const lastLines = dep.logs.split('\n').filter(Boolean).slice(-3);
                  lastLines.forEach(l => addLog(`  ${l}`));
                }
              } else if (dep.status === 'failed') {
                addLog(`\u2717 ${dep.jira_summary || depId.slice(0, 8)}: FAILED`);
                if (dep.error_message) addLog(`  Error: ${dep.error_message}`);
                if (dep.logs) {
                  const lastLines = dep.logs.split('\n').filter(Boolean).slice(-5);
                  lastLines.forEach(l => addLog(`  ${l}`));
                }
              } else {
                allDone = false; // still pending/syncing
              }
            } catch {
              // ignore poll errors
            }
          }

          if (allDone) break;
        }

        const timedOut = Date.now() - startTime >= maxWait;
        if (timedOut) {
          addLog(`\u23F1 Timed out waiting for deployments. Check Deployments page for status.`);
        }
      }

      setDeployStatus({ type: 'success', text: `Deployment initiated: ${enabledItems.length} service(s). Check logs below for results.` });
    } catch (err) {
      addLog(`\u2717 Deploy failed: ${err instanceof Error ? err.message : 'Unknown error'}`);
      setDeployStatus({ type: 'error', text: `Deploy failed: ${err instanceof Error ? err.message : 'Unknown error'}` });
    } finally {
      setDeploying(false);
    }
  };

  const filteredBlueprints = bpSearch
    ? blueprints.filter(b => b.name.toLowerCase().includes(bpSearch.toLowerCase()) || b.category.includes(bpSearch.toLowerCase()))
    : blueprints;

  const enabledCount = pipeline.filter(p => p.enabled).length;
  const selectedCluster = clusterList.find(c => c.id === selectedClusterId);
  const selectedDockerHost = dockerHostList.find(h => h.id === selectedDockerHostId);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Pipeline Builder</h1>
            <ConceptHelp term="pipeline" />
          </div>
          <p className="page-subtitle-modern">Compose multi-service deployment pipelines and deploy to clusters or Docker hosts</p>
        </div>
        <div className="flex gap-2">
          <Link href="/blueprint-groups" className="btn btn-secondary text-[12px]">
            Manage Groups
          </Link>
          <Link href="/pipeline-blueprints" className="btn btn-secondary text-[12px]">
            Manage Blueprints
          </Link>
        </div>
      </div>

      <div className="grid grid-cols-12 gap-6 page-animate-up page-delay-1">
        {/* Left: Available Blueprints + Groups */}
        <div className="col-span-3">
          <div className="card sticky top-6">
            {/* Blueprint Groups Section */}
            {groups.length > 0 && (
              <div className="border-b border-[var(--border)]">
                <button
                  onClick={() => setGroupsExpanded(!groupsExpanded)}
                  className="card-header w-full !flex-row !items-center !justify-between"
                >
                  <h2 className="text-[13px] font-medium text-[var(--text-primary)]">📂 Blueprint Groups</h2>
                  <span className="text-[11px] text-[var(--text-tertiary)]">{groupsExpanded ? '▾' : '▸'} {groups.length}</span>
                </button>
                {groupsExpanded && (
                  <div className="px-3 pb-3 space-y-1">
                    {groups.map(group => (
                      <button
                        key={group.id}
                        onClick={() => addGroupToPipeline(group)}
                        className="w-full text-left p-2 rounded-lg border border-[var(--border)] hover:border-[var(--accent)] hover:bg-[var(--accent-subtle)] transition-all text-[12px] cursor-pointer group"
                      >
                        <div className="flex items-center justify-between">
                          <span className="font-medium text-[var(--text-primary)] truncate">{group.name}</span>
                          <span className="text-[10px] text-[var(--text-tertiary)] ml-1 flex-shrink-0">{group.blueprints.length} bp</span>
                        </div>
                        {group.description && (
                          <p className="text-[10px] text-[var(--text-tertiary)] truncate mt-0.5">{group.description}</p>
                        )}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Individual Blueprints */}
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Available Blueprints</h2>
              <span className="text-[11px] text-[var(--text-tertiary)]">{blueprints.length}</span>
            </div>
            <div className="p-3 space-y-2">
              <input
                type="text"
                placeholder="Search..."
                value={bpSearch}
                onChange={e => setBpSearch(e.target.value)}
                className="input text-[12px]"
              />
              <div className="space-y-1 max-h-[60vh] overflow-y-auto">
                {filteredBlueprints.length === 0 && (
                  <p className="text-[12px] text-[var(--text-tertiary)] text-center py-4">
                    {blueprints.length === 0 ? (
                      <>No blueprints. <Link href="/pipeline-blueprints" className="text-[var(--accent)] hover:underline">Create some</Link></>
                    ) : 'No matches'}
                  </p>
                )}
                {filteredBlueprints.map(bp => {
                  const inPipeline = pipeline.some(p => p.blueprint.id === bp.id);
                  return (
                    <button
                      key={bp.id}
                      onClick={() => !inPipeline && addToPipeline(bp)}
                      disabled={inPipeline}
                      className={`w-full text-left p-2.5 rounded-lg border transition-all text-[12px] ${
                        inPipeline
                          ? 'border-emerald-500/20 bg-emerald-500/10 opacity-60 cursor-default'
                          : 'border-[var(--border)] hover:border-[var(--accent)] hover:bg-[var(--accent-subtle)] cursor-pointer'
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-sm">{categoryIcons[bp.category] || '📦'}</span>
                        <div className="flex-1 min-w-0">
                          <div className="font-medium text-[var(--text-primary)] truncate">{bp.name}</div>
                          <div className="text-[10px] text-[var(--text-tertiary)] font-mono truncate">
                            {bp.source_type === 'docker_compose' ? `Compose (${bp.compose_yaml?.split('\n').length || 0} lines)` :
                             bp.source_type === 'container' ? bp.image : bp.chart_url}
                          </div>
                        </div>
                        {inPipeline && <span className="text-green-600 text-[10px]">✓ added</span>}
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        </div>

        {/* Center: Pipeline */}
        <div className="col-span-6 space-y-4">
          {/* Target Config */}
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Target Configuration</h2>
            </div>
            <div className="card-body">
              {/* Target Type Selector */}
              <div className="mb-3">
                <label className="label">Target Type</label>
                <div className="flex gap-2">
                  <button
                    onClick={() => setTargetType('kubernetes')}
                    className={`flex-1 text-[12px] px-3 py-2 rounded-lg border transition-all ${
                      targetType === 'kubernetes'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    ☸️ Kubernetes Cluster
                  </button>
                  <button
                    onClick={() => setTargetType('docker')}
                    className={`flex-1 text-[12px] px-3 py-2 rounded-lg border transition-all ${
                      targetType === 'docker'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    🐙 Docker Host
                  </button>
                </div>
              </div>
              {targetType === 'kubernetes' ? (
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="label">Target Cluster</label>
                    <select value={selectedClusterId} onChange={e => setSelectedClusterId(e.target.value)} className="input">
                      <option value="">Select cluster...</option>
                      {clusterList.map(c => (
                        <option key={c.id} value={c.id}>{c.name} ({c.environment}) — {c.kubernetes_version}</option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="label">Default Namespace</label>
                    <input value={globalNamespace} onChange={e => {
                      setGlobalNamespace(e.target.value);
                      setPipeline(pipeline.map(p => ({ ...p, namespace: e.target.value })));
                    }} className="input" placeholder="default" />
                    <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Applied to all services. Override per-service below.</p>
                  </div>
                </div>
              ) : (
                <div>
                  <label className="label">Docker Host</label>
                  <select value={selectedDockerHostId} onChange={e => setSelectedDockerHostId(e.target.value)} className="input">
                    <option value="">Select Docker host...</option>
                    {dockerHostList.map(h => (
                      <option key={h.id} value={h.id}>{h.name} ({h.host_type}) — {h.status}</option>
                    ))}
                  </select>
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Only docker_compose blueprints will be deployed to the Docker host.</p>
                </div>
              )}
            </div>
          </div>

          {/* Pipeline Steps */}
          <div className="card">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Deployment Pipeline</h2>
              <span className="text-[11px] text-[var(--text-tertiary)]">{enabledCount} service(s) · drag to reorder</span>
            </div>
            <div className="card-body">
              {pipeline.length === 0 ? (
                <div className="text-center py-12">
                  <div className="text-4xl mb-3 opacity-20">🔧</div>
                  <p className="text-[13px] text-[var(--text-secondary)] mb-1">Pipeline is empty</p>
                  <p className="text-[12px] text-[var(--text-tertiary)]">
                    Click blueprints on the left to add them to your deployment pipeline
                  </p>
                </div>
              ) : (
                <div className="space-y-2">
                  {pipeline.map((item, idx) => (
                    <div
                      key={item.id}
                      draggable
                      onDragStart={() => handleDragStart(idx)}
                      onDragOver={(e) => handleDragOver(e, idx)}
                      onDrop={() => handleDrop(idx)}
                      onDragEnd={handleDragEnd}
                      className={`border rounded-lg transition-all ${
                        dragOverIdx === idx && dragIdx !== idx
                          ? 'border-[var(--accent)] border-dashed bg-[var(--accent-subtle)]'
                          : dragIdx === idx
                            ? 'border-[var(--border)] opacity-40'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                      }`}
                    >
                      <div className="flex items-stretch">
                        {/* Drag handle + order */}
                        <div className="flex flex-col items-center justify-center gap-1 px-3 py-3 bg-[var(--border-light)] rounded-l-lg cursor-grab active:cursor-grabbing border-r border-[var(--border)]">
                          <button
                            onClick={() => moveItem(idx, -1)}
                            disabled={idx === 0}
                            className="text-[9px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] disabled:opacity-20 leading-none"
                          >&#9650;</button>
                          <span className="text-[11px] font-bold text-[var(--text-tertiary)] w-5 h-5 flex items-center justify-center bg-[var(--surface)] rounded border border-[var(--border)]">
                            {idx + 1}
                          </span>
                          <button
                            onClick={() => moveItem(idx, 1)}
                            disabled={idx === pipeline.length - 1}
                            className="text-[9px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] disabled:opacity-20 leading-none"
                          >&#9660;</button>
                        </div>

                        {/* Main content */}
                        <div className="flex-1 min-w-0 p-3">
                          {/* Top row: name + namespace + actions */}
                          <div className="flex items-center gap-2">
                            <span className="text-base flex-shrink-0">{categoryIcons[item.blueprint.category] || '📦'}</span>
                            <span className="text-[13px] font-medium text-[var(--text-primary)] truncate">{item.blueprint.name}</span>
                            {!item.enabled && <span className="text-[10px] px-1.5 py-0.5 bg-[var(--border-light)] text-[var(--text-tertiary)] rounded flex-shrink-0">disabled</span>}
                            <div className="ml-auto flex items-center gap-1.5 flex-shrink-0">
                              {targetType === 'kubernetes' && item.blueprint.source_type !== 'docker_compose' && (
                                <input
                                  value={item.namespace}
                                  onChange={e => updateItemNamespace(item.id, e.target.value)}
                                  className="input text-[11px] w-24 py-1"
                                  placeholder="namespace"
                                  title="Override namespace for this service"
                                />
                              )}
                              <button
                                onClick={() => toggleEnabled(item.id)}
                                className={`text-[11px] px-2 py-1 rounded transition-colors ${
                                  item.enabled
                                    ? 'bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15'
                                    : 'bg-[var(--border-light)] text-[var(--text-tertiary)] hover:bg-[var(--border)]'
                                }`}
                                title={item.enabled ? 'Disable (skip on deploy)' : 'Enable'}
                              >
                                {item.enabled ? '✓' : '○'}
                              </button>
                              <button
                                onClick={() => removeFromPipeline(item.id)}
                                className="text-[11px] px-2 py-1 text-red-500 hover:bg-red-500/10 rounded transition-colors"
                                title="Remove from pipeline"
                              >
                                ✕
                              </button>
                            </div>
                          </div>
                          {/* Bottom row: metadata */}
                          <div className="flex items-center gap-2 mt-1.5 flex-wrap">
                            <span className={`px-1.5 py-0.5 rounded text-[9px] font-medium flex-shrink-0 ${
                              item.blueprint.source_type === 'container' ? 'bg-blue-500/10 text-blue-500' :
                              item.blueprint.source_type === 'helm_git' ? 'bg-violet-500/10 text-violet-500' :
                              item.blueprint.source_type === 'helm_http' ? 'bg-orange-500/10 text-orange-600' :
                              item.blueprint.source_type === 'docker_compose' ? 'bg-cyan-500/10 text-cyan-500' :
                              'bg-teal-500/10 text-teal-500'
                            }`}>
                              {item.blueprint.source_type === 'container' ? '🐳 Container' :
                               item.blueprint.source_type === 'helm_git' ? '🔀 Git' :
                               item.blueprint.source_type === 'helm_http' ? '🌐 HTTP' :
                               item.blueprint.source_type === 'docker_compose' ? '🐙 Compose' :
                               '📦 OCI'}
                            </span>
                            {item.groupName && (
                              <span className="px-1.5 py-0.5 rounded text-[9px] bg-indigo-500/10 text-indigo-500 flex-shrink-0" title="From group">
                                📂 {item.groupName}
                              </span>
                            )}
                            <span className="text-[10px] font-mono text-[var(--text-tertiary)] truncate max-w-[200px]">
                              {item.blueprint.source_type === 'docker_compose'
                                ? `Compose (${item.blueprint.compose_yaml?.split('\n').length || 0} lines)`
                                : item.blueprint.source_type === 'container'
                                  ? item.blueprint.image
                                  : item.blueprint.chart_name
                                    ? `${item.blueprint.chart_url}/${item.blueprint.chart_name}`
                                    : item.blueprint.chart_url}
                            </span>
                            {item.blueprint.source_type !== 'container' && item.blueprint.source_type !== 'docker_compose' && !item.blueprint.chart_name && (
                              <span className="text-[9px] text-red-500 flex-shrink-0" title="Chart name not set - edit blueprint">⚠ no chart name</span>
                            )}
                            {item.blueprint.source_type !== 'docker_compose' && (
                              <span className="text-[10px] text-[var(--text-tertiary)] flex-shrink-0 bg-[var(--border-light)] px-1.5 py-0.5 rounded">{item.blueprint.cpu} / {item.blueprint.memory} ×{item.blueprint.replicas}</span>
                            )}
                            {item.blueprint.values_yaml && (
                              <button
                                onClick={() => setShowValuesFor(showValuesFor === item.id ? null : item.id)}
                                className="text-[10px] text-[var(--accent)] hover:underline flex-shrink-0"
                              >
                                {showValuesFor === item.id ? 'hide yaml' : 'show yaml'}
                              </button>
                            )}
                          </div>
                        </div>
                      </div>

                      {/* YAML preview */}
                      {showValuesFor === item.id && item.blueprint.values_yaml && (
                        <div className="px-3 pb-3">
                          <pre className="bg-[var(--border-light)] rounded-lg p-3 text-[11px] font-mono text-[var(--text-secondary)] overflow-auto max-h-[200px] whitespace-pre-wrap">
                            {item.blueprint.values_yaml}
                          </pre>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Deploy Log */}
          {(deployStatus || deployLog.length > 0) && (
            <div className={`card ${deployStatus?.type === 'error' ? 'border-red-500/20' : deployStatus?.type === 'success' ? 'border-emerald-500/20' : ''}`}>
              <div className="card-body">
                {deployStatus && (
                  <div className={`text-[13px] font-medium mb-2 ${deployStatus.type === 'success' ? 'text-emerald-600' : 'text-red-500'}`}>
                    {deployStatus.type === 'success' ? '\u2713' : '\u2717'} {deployStatus.text}
                  </div>
                )}
                {deployLog.length > 0 && (
                  <div ref={deployLogRef} className="font-mono text-[11px] bg-[#1a1a2e] text-[#e0e0e0] rounded-lg p-3 max-h-[300px] overflow-y-auto whitespace-pre-wrap leading-relaxed">
                    {deployLog.map((line, i) => (
                      <div key={i} className={line.includes('\u2717') || line.includes('Error:') ? 'text-red-400' : line.includes('\u2713') ? 'text-green-400' : line.includes('\u26A0') ? 'text-yellow-400' : ''}>
                        {line}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        {/* Right: Summary & Deploy */}
        <div className="col-span-3">
          <div className="card sticky top-6">
            <div className="card-header">
              <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Deploy Summary</h2>
            </div>
            <div className="card-body space-y-4">
              {/* Target info */}
              <div>
                <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">
                  {targetType === 'kubernetes' ? 'Cluster' : 'Docker Host'}
                </p>
                {targetType === 'kubernetes' ? (
                  selectedCluster ? (
                    <div className="flex items-center gap-2">
                      <div className={`w-2 h-2 rounded-full ${selectedCluster.is_active ? 'bg-green-500' : 'bg-gray-300'}`} />
                      <span className="text-[12px] font-medium text-[var(--text-primary)]">{selectedCluster.name}</span>
                    </div>
                  ) : (
                    <p className="text-[12px] text-[var(--text-tertiary)]">Not selected</p>
                  )
                ) : (
                  selectedDockerHost ? (
                    <div className="flex items-center gap-2">
                      <div className={`w-2 h-2 rounded-full ${selectedDockerHost.status === 'connected' ? 'bg-green-500' : 'bg-gray-300'}`} />
                      <span className="text-[12px] font-medium text-[var(--text-primary)]">{selectedDockerHost.name}</span>
                    </div>
                  ) : (
                    <p className="text-[12px] text-[var(--text-tertiary)]">Not selected</p>
                  )
                )}
              </div>

              {/* Pipeline summary */}
              <div>
                <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Services ({enabledCount}/{pipeline.length})</p>
                <div className="space-y-1">
                  {pipeline.map((item, idx) => (
                    <div key={item.id} className={`flex items-center gap-2 text-[11px] ${!item.enabled ? 'opacity-40 line-through' : ''}`}>
                      <span className="text-[var(--text-tertiary)] w-4 text-right">{idx + 1}.</span>
                      <span className="text-[var(--text-primary)] truncate">{item.blueprint.name}</span>
                      <span className="text-[var(--text-tertiary)] ml-auto">{item.namespace}</span>
                    </div>
                  ))}
                  {pipeline.length === 0 && <p className="text-[12px] text-[var(--text-tertiary)]">No services added</p>}
                </div>
              </div>

              {/* Total resources */}
              {enabledCount > 0 && (() => {
                const totals = pipeline.filter(p => p.enabled).reduce((acc, p) => {
                  acc.cpu += parseFloat(p.blueprint.cpu) || 0;
                  acc.mem += parseInt(p.blueprint.memory) || 0;
                  acc.replicas += p.blueprint.replicas;
                  return acc;
                }, { cpu: 0, mem: 0, replicas: 0 });
                return (
                  <div>
                    <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Total Resources</p>
                    <div className="bg-[var(--border-light)] rounded-lg p-2.5 text-[11px] text-[var(--text-secondary)] space-y-0.5">
                      <div>CPU: {totals.cpu}m × {totals.replicas} replicas</div>
                      <div>Memory: {totals.mem}Mi × {totals.replicas} replicas</div>
                    </div>
                  </div>
                );
              })()}

              {/* Deploy button */}
              <button
                onClick={handleDeploy}
                disabled={deploying || (targetType === 'kubernetes' && !selectedClusterId) || (targetType === 'docker' && !selectedDockerHostId) || enabledCount === 0}
                className="btn btn-primary w-full justify-center"
              >
                {deploying ? (
                  <span className="flex items-center gap-2">
                    <svg className="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Deploying...
                  </span>
                ) : (
                  `Deploy ${enabledCount} service(s) → ${targetType === 'kubernetes' ? (selectedCluster?.name || 'cluster') : (selectedDockerHost?.name || 'docker host')}`
                )}
              </button>
              {((targetType === 'kubernetes' && !selectedClusterId) || (targetType === 'docker' && !selectedDockerHostId) || enabledCount === 0) && (
                <p className="text-[10px] text-[var(--text-tertiary)] text-center">
                  {enabledCount === 0 ? 'Add services to the pipeline' : targetType === 'kubernetes' ? 'Select a cluster to deploy' : 'Select a Docker host'}
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}
