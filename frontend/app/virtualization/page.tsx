'use client';

import { useState, useEffect } from 'react';
import { virtualization, type ProxmoxNode, type ProxmoxVM } from '@/lib/api';
import Link from 'next/link';

export default function VirtualizationDashboard() {
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [vms, setVMs] = useState<ProxmoxVM[]>([]);
  const [containers, setContainers] = useState<ProxmoxVM[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const [nodesRes, vmsRes, containersRes] = await Promise.allSettled([
        virtualization.proxmox.listNodes(),
        virtualization.proxmox.listVMs(),
        virtualization.proxmox.listContainers(),
      ]);
      if (nodesRes.status === 'fulfilled') setNodes(Array.isArray(nodesRes.value.data) ? nodesRes.value.data : []);
      if (vmsRes.status === 'fulfilled') setVMs(Array.isArray(vmsRes.value.data) ? vmsRes.value.data : []);
      if (containersRes.status === 'fulfilled') setContainers(Array.isArray(containersRes.value.data) ? containersRes.value.data : []);
      if (nodesRes.status === 'rejected') setError('Failed to connect to Proxmox. Check your connection settings.');
    } catch {
      setError('Failed to load virtualization data');
    }
    setLoading(false);
  };

  const runningVMs = vms.filter(v => v.status === 'running').length;
  const runningContainers = containers.filter(c => c.status === 'running').length;
  const onlineNodes = nodes.filter(n => n.status === 'online').length;
  const totalCPU = nodes.reduce((sum, n) => sum + (n.cpu * 100), 0) / (nodes.length || 1);
  const totalMemUsed = nodes.reduce((sum, n) => sum + n.mem, 0);
  const totalMem = nodes.reduce((sum, n) => sum + n.maxmem, 0);

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <div className="page-animate">
            <h1 className="page-title-modern">Virtualization</h1>
            <p className="page-subtitle-modern">Manage virtual machines and containers</p>
          </div>
          <div className="card card-body text-center py-16">
            <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
              <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <p className="text-[13px]">Loading...</p>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Virtualization</h1>
            <p className="page-subtitle-modern">Proxmox VE — manage VMs, containers, and nodes</p>
          </div>
          <div className="flex gap-2">
            <Link href="/virtualization/vms" className="btn btn-secondary">View VMs</Link>
            <Link href="/virtualization/containers" className="btn btn-secondary">View Containers</Link>
          </div>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-xl p-4 text-[13px] text-red-500 flex items-center justify-between">
            <span>{error}</span>
            <Link href="/connections?type=proxmox" className="text-xs underline hover:no-underline">Configure Connection</Link>
          </div>
        )}

        {nodes.length > 0 && nodes.every(n => n.maxcpu === 0 && n.maxmem === 0) && (
          <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-xl p-4 text-[13px] text-yellow-600">
            <p className="font-medium mb-1">API token has no permissions</p>
            <p className="text-[12px] opacity-90">
              Proxmox is reachable, but the token returns empty resources. In the Proxmox UI go to
              Datacenter → Permissions, select the API token and assign a role — e.g. <code>PVEAdministrator</code> (or
              <code> PVEAuditor</code> for read-only). If the token was created with “Privilege Separation” enabled,
              permissions must be granted to the token itself, not just the user.
            </p>
          </div>
        )}

        {/* Stats */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div className="card card-body">
            <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Nodes Online</div>
            <div className="text-2xl font-bold text-[var(--text-primary)]">{onlineNodes} / {nodes.length}</div>
          </div>
          <div className="card card-body">
            <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">VMs Running</div>
            <div className="text-2xl font-bold text-[var(--text-primary)]">{runningVMs} / {vms.length}</div>
          </div>
          <div className="card card-body">
            <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Containers Running</div>
            <div className="text-2xl font-bold text-[var(--text-primary)]">{runningContainers} / {containers.length}</div>
          </div>
          <div className="card card-body">
            <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Memory Used</div>
            <div className="text-2xl font-bold text-[var(--text-primary)]">
              {totalMem > 0 ? Math.round((totalMemUsed / totalMem) * 100) : 0}%
            </div>
          </div>
        </div>

        {/* Nodes */}
        <div>
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)] mb-3">Cluster Nodes</h2>
          {nodes.length === 0 ? (
            <div className="card card-body text-center py-8 text-[var(--text-tertiary)]">
              No nodes found. Check your Proxmox connection.
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {nodes.map(node => (
                <div key={node.node} className="card card-body modern-card-hover">
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <span className={`w-2 h-2 rounded-full ${node.status === 'online' ? 'bg-emerald-500' : 'bg-red-500'}`} />
                      <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{node.node}</h3>
                    </div>
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--border-light)] text-[var(--text-tertiary)]">
                      {node.status}
                    </span>
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-[11px]">
                      <span className="text-[var(--text-tertiary)]">CPU</span>
                      <span className="text-[var(--text-secondary)]">{Math.round(node.cpu * 100)}%</span>
                    </div>
                    <div className="w-full h-1.5 bg-[var(--border-light)] rounded-full overflow-hidden">
                      <div
                        className="h-full bg-[var(--accent)] rounded-full transition-all"
                        style={{ width: `${Math.min(node.cpu * 100, 100)}%` }}
                      />
                    </div>
                    <div className="flex justify-between text-[11px]">
                      <span className="text-[var(--text-tertiary)]">Memory</span>
                      <span className="text-[var(--text-secondary)]">
                        {(node.mem / 1024 / 1024 / 1024).toFixed(1)} / {(node.maxmem / 1024 / 1024 / 1024).toFixed(1)} GB
                      </span>
                    </div>
                    <div className="w-full h-1.5 bg-[var(--border-light)] rounded-full overflow-hidden">
                      <div
                        className="h-full bg-emerald-500 rounded-full transition-all"
                        style={{ width: `${node.maxmem > 0 ? Math.min((node.mem / node.maxmem) * 100, 100) : 0}%` }}
                      />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Quick links */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Link href="/virtualization/vms" className="card card-body modern-card-hover group">
            <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              Virtual Machines
            </div>
            <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
              {vms.length} VMs across all nodes
            </div>
          </Link>
          <Link href="/virtualization/containers" className="card card-body modern-card-hover group">
            <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              LXC Containers
            </div>
            <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
              {containers.length} containers across all nodes
            </div>
          </Link>
          <Link href="/virtualization/hosts" className="card card-body modern-card-hover group">
            <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              Host Management
            </div>
            <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
              Configure Proxmox connections
            </div>
          </Link>
        </div>
      </div>
    </div>
  );
}
