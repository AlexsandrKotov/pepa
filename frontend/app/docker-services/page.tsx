'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { dockerHosts, dockerServices, type DockerHost, type DockerService, type DiscoveredDockerContainer } from '@/lib/api';

export default function DockerServicesPage() {
  const [services, setServices] = useState<DockerService[]>([]);
  const [hosts, setHosts] = useState<DockerHost[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [showLogs, setShowLogs] = useState<string | null>(null);

  useEscapeKey(() => {
    if (showLogs) setShowLogs(null);
    else if (showForm) setShowForm(false);
  }, showForm || showLogs !== null);
  const [logs, setLogs] = useState('');
  const [logsLoading, setLogsLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [error, setError] = useState('');

  // Discovered containers from Docker hosts
  const [discoveredContainers, setDiscoveredContainers] = useState<{ hostName: string; containers: DiscoveredDockerContainer[] }[]>([]);
  const [discovering, setDiscovering] = useState(false);
  const [showDiscovered, setShowDiscovered] = useState(true);

  // Form state
  const [form, setForm] = useState({
    docker_host_id: '', name: '', compose_yaml: '', folder_path: '',
    source: 'yaml' as 'yaml' | 'folder',
    env_mode: 'yaml' as 'yaml' | 'kv',
    env_key: '', env_value: '', env_vars: {} as Record<string, string>,
  });

  const load = async () => {
    try {
      const [svcRes, hostRes] = await Promise.all([
        dockerServices.list(),
        dockerHosts.list(),
      ]);
      setServices(svcRes.docker_services || []);
      setHosts(hostRes.docker_hosts || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  const discoverContainers = async () => {
    setDiscovering(true);
    try {
      const connectedHosts = hosts.filter(h => h.status === 'connected');
      const results: { hostName: string; containers: DiscoveredDockerContainer[] }[] = [];
      for (const host of connectedHosts) {
        try {
          const res = await dockerHosts.containers(host.id);
          if (res.containers && res.containers.length > 0) {
            results.push({ hostName: host.name, containers: res.containers });
          }
        } catch { /* skip host errors */ }
      }
      setDiscoveredContainers(results);
    } catch { /* ignore */ }
    setDiscovering(false);
  };

  useEffect(() => {
    load().then(() => {
      // Auto-discover containers after loading hosts
      setTimeout(() => discoverContainers(), 500);
    });
  }, []);

  const hostName = (id: string | null) => {
    if (!id) return 'Local Docker';
    return hosts.find(h => h.id === id)?.name || 'Unknown';
  };

  const openDeploy = () => {
    setForm({
      docker_host_id: hosts[0]?.id || '', name: '', compose_yaml: '', folder_path: '',
      source: 'yaml', env_mode: 'yaml', env_key: '', env_value: '', env_vars: {},
    });
    setError('');
    setShowForm(true);
  };

  const addEnvVar = () => {
    if (!form.env_key.trim()) return;
    setForm({ ...form, env_vars: { ...form.env_vars, [form.env_key.trim()]: form.env_value }, env_key: '', env_value: '' });
  };

  // Deploy target: 'local' or a Docker host ID
  const [deployTarget, setDeployTarget] = useState<'local' | 'host'>('local');

  const handleDeploy = async () => {
    setError('');
    if (deployTarget === 'host' && !form.docker_host_id) {
      setError('Please select a Docker host');
      return;
    }
    if (!form.name.trim()) {
      setError('Service name is required');
      return;
    }
    if (form.source === 'folder' && !form.folder_path.trim()) {
      setError('Project folder path is required');
      return;
    }
    if (form.source === 'yaml' && !form.compose_yaml.trim()) {
      setError('Compose YAML is required');
      return;
    }
    try {
      const payload = {
        name: form.name.trim(),
        compose_yaml: form.source === 'yaml' ? form.compose_yaml : undefined,
        folder_path: form.source === 'folder' ? form.folder_path.trim() : undefined,
        env_vars: form.env_vars,
      };
      if (deployTarget === 'local') {
        await dockerServices.deployLocal(payload);
      } else {
        await dockerServices.create({ ...payload, docker_host_id: form.docker_host_id });
      }
      setShowForm(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deploy failed');
    }
  };

  const handleAction = async (id: string, action: 'start' | 'stop' | 'restart' | 'refresh' | 'delete') => {
    if (action === 'delete') {
      if (!confirm('Stop and remove this service?')) return;
    }
    setActionLoading(id);
    try {
      switch (action) {
        case 'start': await dockerServices.start(id); break;
        case 'stop': await dockerServices.stop(id); break;
        case 'restart': await dockerServices.restart(id); break;
        case 'refresh': await dockerServices.refresh(id); break;
        case 'delete': await dockerServices.delete(id); break;
      }
      load();
    } catch { /* ignore */ }
    setActionLoading(null);
  };

  const handleShowLogs = async (id: string) => {
    setShowLogs(id);
    setLogsLoading(true);
    try {
      const res = await dockerServices.logs(id);
      setLogs(res.logs || 'No logs available');
    } catch {
      setLogs('Failed to fetch logs');
    }
    setLogsLoading(false);
  };

  const statusColor = (s: string) => {
    switch (s) {
      case 'running': return 'bg-emerald-500/15 text-emerald-600';
      case 'stopped': return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
      case 'error': return 'bg-red-500/15 text-red-500';
      case 'deploying': return 'bg-blue-500/15 text-blue-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
    }
  };

  const containerStateColor = (s: string) => {
    if (s === 'running') return 'text-emerald-600';
    if (s === 'exited' || s === 'stopped') return 'text-[var(--text-tertiary)]';
    return 'text-orange-500';
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Docker Services</h1>
          <p className="page-subtitle-modern">Deploy and manage Docker Compose stacks on your Docker hosts</p>
        </div>
        <div className="flex gap-2">
          <Link href="/docker-hosts" className="btn btn-secondary text-[12px]">Manage Hosts</Link>
          <button
            onClick={() => { discoverContainers(); }}
            disabled={discovering || hosts.filter(h => h.status === 'connected').length === 0}
            className="btn btn-secondary text-[12px] disabled:opacity-50"
          >
            {discovering ? 'Discovering...' : '🔍 Discover Containers'}
          </button>
          <button onClick={openDeploy} className="btn btn-primary">
            + Deploy Service
          </button>
        </div>
      </div>

      {hosts.length === 0 && !loading && (
        <div className="bg-orange-500/10 border border-orange-500/20 rounded-lg p-4">
          <p className="text-[13px] text-orange-500">
            No Docker hosts configured. <Link href="/docker-hosts" className="underline font-medium">Add a Docker host</Link> to deploy services.
          </p>
        </div>
      )}

      {/* Services list */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      ) : services.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">📦</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No services deployed</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Deploy a Docker Compose stack to get started
          </p>
          <button onClick={openDeploy} className="btn btn-primary">+ Deploy First Service</button>
        </div>
      ) : (
        <div className="space-y-4">
          {services.map(svc => (
            <div key={svc.id} className="card p-5 modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="flex items-start justify-between mb-3">
                <div className="flex items-center gap-3">
                  <span className="text-xl">📦</span>
                  <div>
                    <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">{svc.name}</h3>
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      Host: {hostName(svc.docker_host_id)}
                      {svc.folder_path && <span className="ml-2 text-[var(--text-tertiary)]">📂 {svc.folder_path}</span>}
                    </span>
                  </div>
                </div>
                <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(svc.status)}`}>
                  {svc.status}
                </span>
              </div>

              {/* Containers */}
              {svc.containers && svc.containers.length > 0 && (
                <div className="mb-3">
                  <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1.5">Containers</p>
                  <div className="bg-[var(--border-light)] rounded-lg overflow-hidden">
                    <table className="w-full text-[11px]">
                      <thead>
                        <tr className="border-b border-[var(--border)]">
                          <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Name</th>
                          <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Image</th>
                          <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">State</th>
                          <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Ports</th>
                        </tr>
                      </thead>
                      <tbody>
                        {svc.containers.map((c, i) => (
                          <tr key={i} className="border-b border-[var(--border)] last:border-0">
                            <td className="px-3 py-1.5 font-mono text-[var(--text-primary)]">{c.name}</td>
                            <td className="px-3 py-1.5 font-mono text-[var(--text-secondary)] truncate max-w-[200px]">{c.image}</td>
                            <td className={`px-3 py-1.5 font-medium ${containerStateColor(c.state)}`}>{c.state}</td>
                            <td className="px-3 py-1.5 font-mono text-[var(--text-secondary)]">{c.ports || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Actions */}
              <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                <button
                  onClick={() => handleAction(svc.id, 'refresh')}
                  disabled={actionLoading === svc.id}
                  className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50"
                >
                  Refresh
                </button>
                {svc.status === 'running' && (
                  <>
                    <button
                      onClick={() => handleAction(svc.id, 'restart')}
                      disabled={actionLoading === svc.id}
                      className="text-[11px] px-2.5 py-1 text-orange-500 hover:bg-orange-500/10 rounded-lg transition-colors disabled:opacity-50"
                    >
                      Restart
                    </button>
                    <button
                      onClick={() => handleAction(svc.id, 'stop')}
                      disabled={actionLoading === svc.id}
                      className="text-[11px] px-2.5 py-1 text-orange-500 hover:bg-orange-500/10 rounded-lg transition-colors disabled:opacity-50"
                    >
                      Stop
                    </button>
                  </>
                )}
                {svc.status === 'stopped' && (
                  <button
                    onClick={() => handleAction(svc.id, 'start')}
                    disabled={actionLoading === svc.id}
                    className="text-[11px] px-2.5 py-1 text-emerald-600 hover:bg-emerald-500/10 rounded-lg transition-colors disabled:opacity-50"
                  >
                    Start
                  </button>
                )}
                <button
                  onClick={() => handleShowLogs(svc.id)}
                  className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded-lg transition-colors"
                >
                  Logs
                </button>
                <button
                  onClick={() => handleAction(svc.id, 'delete')}
                  disabled={actionLoading === svc.id}
                  className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto disabled:opacity-50"
                >
                  {actionLoading === svc.id ? 'Processing...' : 'Remove'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Discovered Containers Section */}
      {hosts.filter(h => h.status === 'connected').length > 0 && (
        <div className="card overflow-hidden">
          <button
            onClick={() => setShowDiscovered(!showDiscovered)}
            className="w-full flex items-center justify-between px-5 py-3 hover:bg-[var(--border-light)] transition-colors"
          >
            <div className="flex items-center gap-2">
              <span className="text-lg">🐳</span>
              <div className="text-left">
                <h2 className="text-[14px] font-semibold text-[var(--text-primary)]">Discovered Containers</h2>
                <p className="text-[11px] text-[var(--text-tertiary)]">
                  Containers running on your Docker hosts (not managed by PEPA)
                  {discoveredContainers.length > 0 && (
                    <span className="ml-1">— {discoveredContainers.reduce((sum, g) => sum + g.containers.length, 0)} found</span>
                  )}
                </p>
              </div>
            </div>
            <svg className={`w-4 h-4 text-[var(--text-tertiary)] transition-transform ${showDiscovered ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {showDiscovered && (
            <div className="px-5 pb-4 space-y-4">
              {discovering ? (
                <div className="text-center py-6">
                  <p className="text-[12px] text-[var(--text-tertiary)]">Discovering containers on connected hosts...</p>
                </div>
              ) : discoveredContainers.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-[12px] text-[var(--text-tertiary)]">
                    No additional containers found. All containers are either managed by PEPA or no containers are running.
                  </p>
                </div>
              ) : (
                discoveredContainers.map((group, gi) => (
                  <div key={gi}>
                    <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2">
                      🐳 {group.hostName} — {group.containers.length} container{group.containers.length !== 1 ? 's' : ''}
                    </p>
                    <div className="bg-[var(--border-light)] rounded-lg overflow-hidden">
                      <table className="w-full text-[11px]">
                        <thead>
                          <tr className="border-b border-[var(--border)]">
                            <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Name</th>
                            <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Image</th>
                            <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">State</th>
                            <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Status</th>
                            <th className="text-left px-3 py-1.5 text-[var(--text-tertiary)] font-medium">Ports</th>
                          </tr>
                        </thead>
                        <tbody>
                          {group.containers.map((c) => (
                            <tr key={c.id} className="border-b border-[var(--border)] last:border-0">
                              <td className="px-3 py-1.5 font-mono text-[var(--text-primary)]">{c.name}</td>
                              <td className="px-3 py-1.5 font-mono text-[var(--text-secondary)] truncate max-w-[200px]">{c.image}</td>
                              <td className={`px-3 py-1.5 font-medium ${containerStateColor(c.state)}`}>{c.state}</td>
                              <td className="px-3 py-1.5 text-[var(--text-secondary)]">{c.status}</td>
                              <td className="px-3 py-1.5 font-mono text-[var(--text-secondary)]">{c.ports || '—'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      )}

      {/* Deploy Modal */}
      {showForm && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-2xl mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Deploy Docker Compose Service</h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
              )}

              {/* Deploy Source */}
              <div>
                <label className="label">Source</label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setForm({ ...form, source: 'yaml' })}
                    className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                      form.source === 'yaml'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                        : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    <span className="text-lg">📝</span>
                    <div>
                      <p className="text-[12px] font-medium text-[var(--text-primary)]">Paste YAML</p>
                      <p className="text-[10px] text-[var(--text-tertiary)]">Paste or upload compose file</p>
                    </div>
                  </button>
                  <button
                    type="button"
                    onClick={() => setForm({ ...form, source: 'folder' })}
                    className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                      form.source === 'folder'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] ring-1 ring-[var(--accent)]'
                        : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    <span className="text-lg">📂</span>
                    <div>
                      <p className="text-[12px] font-medium text-[var(--text-primary)]">Project Folder</p>
                      <p className="text-[10px] text-[var(--text-tertiary)]">Server-side folder path</p>
                    </div>
                  </button>
                </div>
              </div>

              {/* Deploy Target */}
              <div>
                <label className="label">Deploy Target</label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setDeployTarget('local')}
                    className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                      deployTarget === 'local'
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
                    onClick={() => setDeployTarget('host')}
                    className={`flex items-center gap-2.5 px-4 py-3 rounded-lg border text-left transition-all ${
                      deployTarget === 'host'
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

              {deployTarget === 'host' && (
                <div>
                  <label className="label">Docker Host *</label>
                  <select value={form.docker_host_id} onChange={e => setForm({ ...form, docker_host_id: e.target.value })} className="input">
                    <option value="">Select host...</option>
                    {hosts.map(h => (
                      <option key={h.id} value={h.id}>{h.name} ({h.status})</option>
                    ))}
                  </select>
                  {hosts.length === 0 && (
                    <p className="text-[11px] text-orange-500 mt-1">
                      No hosts configured. <Link href="/docker-hosts" className="underline">Add a Docker host</Link>
                    </p>
                  )}
                </div>
              )}

              <div>
                <label className="label">Service Name *</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="my-service" />
              </div>

              {form.source === 'folder' ? (
                <div>
                  <label className="label">Project Folder Path *</label>
                  <input
                    value={form.folder_path}
                    onChange={e => setForm({ ...form, folder_path: e.target.value })}
                    className="input font-mono text-[12px]"
                    placeholder="/opt/projects/my-app"
                  />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                    Full path to the project folder containing docker-compose.yml on the server
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
                          setForm({ ...form, compose_yaml: text });
                        }}
                      />
                    </label>
                  </div>
                  <textarea
                    value={form.compose_yaml}
                    onChange={e => setForm({ ...form, compose_yaml: e.target.value })}
                    className="input font-mono text-[12px] w-full"
                    rows={12}
                    spellCheck={false}
                    placeholder={`version: '3.8'\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "80:80"\n    environment:\n      - ENV=production`}
                  />
                </div>
              )}

              {/* Environment Variables */}
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <button
                    onClick={() => setForm({ ...form, env_mode: 'yaml' })}
                    className={`text-[11px] px-3 py-1 rounded-lg border transition-colors ${form.env_mode === 'yaml' ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)]' : 'border-[var(--border)] text-[var(--text-tertiary)]'}`}
                  >
                    Key-Value
                  </button>
                </div>
                {Object.keys(form.env_vars).length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mb-2">
                    {Object.entries(form.env_vars).map(([k, v]) => (
                      <span key={k} className="inline-flex items-center gap-1 text-[10px] px-2 py-1 bg-[var(--border-light)] rounded-lg">
                        <span className="font-mono font-medium">{k}={v}</span>
                        <button onClick={() => {
                          const next = { ...form.env_vars };
                          delete next[k];
                          setForm({ ...form, env_vars: next });
                        }} className="text-red-400 hover:text-red-600">&times;</button>
                      </span>
                    ))}
                  </div>
                )}
                <div className="flex gap-2">
                  <input value={form.env_key} onChange={e => setForm({ ...form, env_key: e.target.value })} className="input text-[12px] flex-1" placeholder="KEY" />
                  <input value={form.env_value} onChange={e => setForm({ ...form, env_value: e.target.value })} className="input text-[12px] flex-1" placeholder="value" />
                  <button onClick={addEnvVar} className="btn btn-secondary text-[11px] px-3">Add</button>
                </div>
              </div>
            </div>

            <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
              <button onClick={handleDeploy} className="btn btn-primary">
                {deployTarget === 'local' ? 'Deploy Locally' : 'Deploy to Host'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Logs Modal */}
      {showLogs && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowLogs(null)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-3xl mx-4 max-h-[80vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                Logs — {services.find(s => s.id === showLogs)?.name}
              </h2>
              <button onClick={() => setShowLogs(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>
            <div className="flex-1 overflow-auto p-5">
              {logsLoading ? (
                <p className="text-[13px] text-[var(--text-tertiary)]">Loading logs...</p>
              ) : (
                <pre className="bg-black text-green-400 rounded-lg p-4 text-[11px] font-mono whitespace-pre-wrap leading-relaxed max-h-[60vh] overflow-auto">
                  {logs}
                </pre>
              )}
            </div>
          </div>
        </div>
      )}
      </div>
    </div>
  );
}
