'use client';

import { useState, useEffect } from 'react';
import { virtualization, connections as connectionsAPI, type ProxmoxNode, type ProxmoxVM, type VMwareVM, type VMwareHost } from '@/lib/api';
import Link from 'next/link';

type Provider = 'proxmox' | 'vmware';

export default function VirtualizationDashboard() {
  const [availableProviders, setAvailableProviders] = useState<Provider[]>([]);
  const [provider, setProvider] = useState<Provider>('proxmox');
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [vms, setVMs] = useState<ProxmoxVM[]>([]);
  const [containers, setContainers] = useState<ProxmoxVM[]>([]);
  const [vmwareHosts, setVmwareHosts] = useState<VMwareHost[]>([]);
  const [vmwareVMs, setVmwareVMs] = useState<VMwareVM[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Detect which providers have connections configured
  useEffect(() => {
    connectionsAPI.list().then(res => {
      const conns = res.connections || [];
      const providers: Provider[] = [];
      if (conns.some(c => c.type === 'proxmox')) providers.push('proxmox');
      if (conns.some(c => c.type === 'vmware')) providers.push('vmware');
      setAvailableProviders(providers);
      // Default to first available provider
      if (providers.length > 0 && !providers.includes(provider)) {
        setProvider(providers[0]);
      }
    }).catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (availableProviders.length > 0) loadData();
  }, [provider, availableProviders]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      if (provider === 'proxmox') {
        const [nodesRes, vmsRes, containersRes] = await Promise.allSettled([
          virtualization.proxmox.listNodes(),
          virtualization.proxmox.listVMs(),
          virtualization.proxmox.listContainers(),
        ]);
        if (nodesRes.status === 'fulfilled') setNodes(Array.isArray(nodesRes.value.data) ? nodesRes.value.data : []);
        if (vmsRes.status === 'fulfilled') setVMs(Array.isArray(vmsRes.value.data) ? vmsRes.value.data : []);
        if (containersRes.status === 'fulfilled') setContainers(Array.isArray(containersRes.value.data) ? containersRes.value.data : []);
        if (nodesRes.status === 'rejected') {
          const msg = nodesRes.reason instanceof Error ? nodesRes.reason.message : 'Unknown error';
          setError(`Failed to connect to Proxmox: ${msg}`);
        }
      } else {
        const [hostsRes, vmsRes] = await Promise.allSettled([
          virtualization.vmware.listHosts(),
          virtualization.vmware.listVMs(),
        ]);
        if (hostsRes.status === 'fulfilled') setVmwareHosts(Array.isArray(hostsRes.value.data) ? hostsRes.value.data : []);
        if (vmsRes.status === 'fulfilled') setVmwareVMs(Array.isArray(vmsRes.value.data) ? vmsRes.value.data : []);
        if (hostsRes.status === 'rejected') {
          const msg = hostsRes.reason instanceof Error ? hostsRes.reason.message : 'Unknown error';
          setError(`Failed to connect to VMware vCenter: ${msg}`);
        }
      }
    } catch {
      setError('Failed to load virtualization data');
    }
    setLoading(false);
  };

  // Proxmox stats
  const pxRunningVMs = vms.filter(v => v.status === 'running').length;
  const pxRunningContainers = containers.filter(c => c.status === 'running').length;
  const pxOnlineNodes = nodes.filter(n => n.status === 'online').length;
  const pxTotalMemUsed = nodes.reduce((sum, n) => sum + n.mem, 0);
  const pxTotalMem = nodes.reduce((sum, n) => sum + n.maxmem, 0);

  // VMware stats
  const vmRunningVMs = vmwareVMs.filter(v => v.power_state === 'POWERED_ON').length;
  const vmConnectedHosts = vmwareHosts.filter(h => h.connection_state === 'CONNECTED').length;
  const vmTotalMemAllocated = vmwareVMs.reduce((sum, vm) => sum + (vm.memory_size_mib || 0), 0);

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <div className="page-animate">
            <h1 className="page-title-modern">Virtualization</h1>
            <p className="page-subtitle-modern">Manage virtual machines and hosts</p>
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

  if (availableProviders.length === 0) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <div className="page-animate">
            <h1 className="page-title-modern">Virtualization</h1>
            <p className="page-subtitle-modern">Manage virtual machines and hosts</p>
          </div>
          <div className="card card-body text-center py-16">
            <p className="text-[14px] text-[var(--text-secondary)] mb-2">No virtualization connections configured</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mb-5">Add a Proxmox VE or VMware vCenter connection to get started</p>
            <Link href="/connections" className="btn btn-primary">+ Add Connection</Link>
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
            <p className="page-subtitle-modern">
              {provider === 'proxmox' ? 'Proxmox VE — manage VMs, containers, and nodes' : 'VMware vCenter — manage ESXi hosts and virtual machines'}
            </p>
          </div>
          <div className="flex gap-2">
            {/* Provider tabs — only show when multiple providers are available */}
            {availableProviders.length > 1 && (
              <div className="flex rounded-lg border border-[var(--border)] overflow-hidden">
                {availableProviders.includes('proxmox') && (
                  <button
                    onClick={() => setProvider('proxmox')}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${provider === 'proxmox' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}
                  >
                    Proxmox
                  </button>
                )}
                {availableProviders.includes('vmware') && (
                  <button
                    onClick={() => setProvider('vmware')}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${provider === 'vmware' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}
                  >
                    VMware
                  </button>
                )}
              </div>
            )}
            <Link href="/virtualization/vms" className="btn btn-secondary">View VMs</Link>
            {provider === 'proxmox' && (
              <Link href="/virtualization/containers" className="btn btn-secondary">View Containers</Link>
            )}
          </div>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-xl p-4 text-[13px] text-red-500 flex items-center justify-between">
            <span>{error}</span>
            <Link href={`/connections?type=${provider}`} className="text-xs underline hover:no-underline">Configure Connection</Link>
          </div>
        )}

        {/* Proxmox view */}
        {provider === 'proxmox' && (
          <>
            {nodes.length > 0 && nodes.every(n => n.maxcpu === 0 && n.maxmem === 0) && (
              <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-xl p-4 text-[13px] text-yellow-600">
                <p className="font-medium mb-1">API token has no permissions</p>
                <p className="text-[12px] opacity-90">
                  Proxmox is reachable, but the token returns empty resources. In the Proxmox UI go to
                  Datacenter → Permissions, select the API token and assign a role.
                </p>
              </div>
            )}

            {/* Stats */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Nodes Online</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{pxOnlineNodes} / {nodes.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">VMs Running</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{pxRunningVMs} / {vms.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Containers Running</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{pxRunningContainers} / {containers.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Memory Used</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">
                  {pxTotalMem > 0 ? Math.round((pxTotalMemUsed / pxTotalMem) * 100) : 0}%
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
          </>
        )}

        {/* VMware view */}
        {provider === 'vmware' && (
          <>
            {/* Stats */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Hosts Connected</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{vmConnectedHosts} / {vmwareHosts.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">VMs Powered On</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{vmRunningVMs} / {vmwareVMs.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Total VMs</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">{vmwareVMs.length}</div>
              </div>
              <div className="card card-body">
                <div className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wide mb-1">Memory Allocated</div>
                <div className="text-2xl font-bold text-[var(--text-primary)]">
                  {vmTotalMemAllocated > 0 ? `${(vmTotalMemAllocated / 1024).toFixed(0)} GB` : '—'}
                </div>
              </div>
            </div>

            {/* ESXi Hosts */}
            <div>
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)] mb-3">ESXi Hosts</h2>
              {vmwareHosts.length === 0 ? (
                <div className="card card-body text-center py-8 text-[var(--text-tertiary)]">
                  No hosts found. Check your vCenter connection.
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                  {vmwareHosts.map(host => {
                    const hostVMs = vmwareVMs.filter(vm => vm.host === host.host);
                    const poweredOn = hostVMs.filter(v => v.power_state === 'POWERED_ON').length;
                    const hostMem = hostVMs.reduce((s, vm) => s + (vm.memory_size_mib || 0), 0);
                    return (
                      <Link key={host.host} href={`/virtualization/vms?host=${host.host}`} className="card card-body modern-card-hover group">
                        <div className="flex items-center justify-between mb-2">
                          <div className="flex items-center gap-2">
                            <span className={`w-2 h-2 rounded-full ${host.connection_state === 'CONNECTED' ? 'bg-emerald-500' : 'bg-red-500'}`} />
                            <h3 className="text-[13px] font-semibold text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">{host.name}</h3>
                          </div>
                          <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--border-light)] text-[var(--text-tertiary)]">
                            {host.connection_state?.toLowerCase()}
                          </span>
                        </div>
                        <div className="flex items-center gap-3 text-[11px]">
                          <span className="text-[var(--text-secondary)]">{hostVMs.length} VMs</span>
                          <span className="text-emerald-600">{poweredOn} on</span>
                          {hostMem > 0 && <span className="text-[var(--text-tertiary)]">{(hostMem / 1024).toFixed(1)} GB</span>}
                        </div>
                      </Link>
                    );
                  })}
                </div>
              )}
            </div>
          </>
        )}

        {/* Quick links */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Link href="/virtualization/vms" className="card card-body modern-card-hover group">
            <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              Virtual Machines
            </div>
            <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
              {provider === 'proxmox' ? `${vms.length} VMs across all nodes` : `${vmwareVMs.length} VMs in vCenter`}
            </div>
          </Link>
          {provider === 'proxmox' ? (
            <Link href="/virtualization/containers" className="card card-body modern-card-hover group">
              <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
                LXC Containers
              </div>
              <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
                {containers.length} containers across all nodes
              </div>
            </Link>
          ) : (
            <Link href="/virtualization/hosts" className="card card-body modern-card-hover group">
              <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
                ESXi Hosts
              </div>
              <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
                {vmwareHosts.length} hosts managed by vCenter
              </div>
            </Link>
          )}
          <Link href="/virtualization/hosts" className="card card-body modern-card-hover group">
            <div className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">
              Host Management
            </div>
            <div className="text-[11px] text-[var(--text-tertiary)] mt-1">
              Configure {provider === 'proxmox' ? 'Proxmox' : 'VMware'} connections
            </div>
          </Link>
        </div>
      </div>
    </div>
  );
}
