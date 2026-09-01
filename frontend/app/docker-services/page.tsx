'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { dockerHosts, dockerServices, type DockerHost, type DockerService, type DiscoveredDockerContainer } from '@/lib/api';

export default function DockerServicesPage() {
  const [services, setServices] = useState<DockerService[]>([]);
  const [hosts, setHosts] = useState<DockerHost[]>([]);
  const [loading, setLoading] = useState(true);
  const [showLogs, setShowLogs] = useState<string | null>(null);

  useEscapeKey(() => {
    if (showLogs) setShowLogs(null);
  }, showLogs !== null);
  const [logs, setLogs] = useState('');
  const [logsLoading, setLogsLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Discovered containers from Docker hosts
  const [discoveredContainers, setDiscoveredContainers] = useState<{ hostName: string; containers: DiscoveredDockerContainer[] }[]>([]);
  const [discovering, setDiscovering] = useState(false);
  const [showDiscovered, setShowDiscovered] = useState(true);

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
          <Link href="/services/new?template=docker-compose-import" className="btn btn-primary">+ Deploy First Service</Link>
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
