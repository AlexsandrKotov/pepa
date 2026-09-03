'use client';

import { useState, useEffect, useRef, useMemo } from 'react';
import { useSearchParams } from 'next/navigation';
import {
  virtualization,
  connections as connectionsAPI,
  type ProxmoxVM,
  type ProxmoxNode,
  type ProxmoxStorage,
  type VMwareVM,
  type VMwareVMDetail,
  type VMwareHost,
  type VMwareDatastore,
} from '@/lib/api';
import Link from 'next/link';
import ConfirmModal from '@/components/ConfirmModal';
import { useEscapeKey } from '@/hooks/useEscapeKey';

type Provider = 'proxmox' | 'vmware';
type VMSource = 'template' | 'iso' | 'empty';

export default function VirtualMachinesPage() {
  const searchParams = useSearchParams();
  const hostFromUrl = searchParams.get('host');

  const [availableProviders, setAvailableProviders] = useState<Provider[]>([]);
  const [provider, setProvider] = useState<Provider>('proxmox');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // VMware host filter from URL
  const [vmwareHostFilter, setVmwareHostFilter] = useState<string | null>(null);

  // Proxmox state
  const [pxVMs, setPxVMs] = useState<ProxmoxVM[]>([]);
  const [proxmoxUrl, setProxmoxUrl] = useState('');

  // VMware state
  const [vmwareVMs, setVmwareVMs] = useState<VMwareVM[]>([]);
  const [vcenterUrl, setVcenterUrl] = useState('');

  // VMware filters
  const [vmwarePowerFilter, setVmwarePowerFilter] = useState('');
  const [vmwareSearch, setVmwareSearch] = useState('');

  // Shared action state
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Proxmox create modal state
  const [showCreate, setShowCreate] = useState(false);
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [storages, setStorages] = useState<ProxmoxStorage[]>([]);
  const [templates, setTemplates] = useState<ProxmoxVM[]>([]);
  const [isos, setIsos] = useState<string[]>([]);
  const [form, setForm] = useState({
    name: '', node: '', vmid: '', cores: 2, memory_mb: 2048,
    disk_size: '32G', storage: '', network: 'vmbr0',
    source: 'template' as VMSource, template: 0, iso: '', start: true,
  });
  const [creating, setCreating] = useState(false);

  // Delete confirm state
  const [deleteTarget, setDeleteTarget] = useState<{ type: Provider; name: string; key: string } | null>(null);
  const [deleting, setDeleting] = useState(false);

  // VMware Details modal state
  const [vmwareDetail, setVmwareDetail] = useState<VMwareVMDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const detailRequestId = useRef(0);

  // VMware Clone modal state
  const [cloneTarget, setCloneTarget] = useState<VMwareVM | null>(null);
  const [cloneForm, setCloneForm] = useState({ name: '', host: '', datastore: '', power_on: false });
  const [cloning, setCloning] = useState(false);
  const [vmwareHosts, setVmwareHosts] = useState<VMwareHost[]>([]);
  const [vmwareDatastores, setVmwareDatastores] = useState<VMwareDatastore[]>([]);

  // VMware Edit Config modal state
  const [editTarget, setEditTarget] = useState<VMwareVM | null>(null);
  const [editForm, setEditForm] = useState({ cores: 0, memory_mib: 0 });
  const [editing, setEditing] = useState(false);
  const [editConfirmOpen, setEditConfirmOpen] = useState(false);

  // Detect available providers
  useEffect(() => {
    connectionsAPI.list().then(res => {
      const conns = res.connections || [];
      const providers: Provider[] = [];
      if (conns.some(c => c.type === 'proxmox')) providers.push('proxmox');
      if (conns.some(c => c.type === 'vmware')) providers.push('vmware');
      setAvailableProviders(providers);
      // Auto-switch to VMware if host filter is in URL
      if (hostFromUrl && providers.includes('vmware')) {
        setProvider('vmware');
        setVmwareHostFilter(hostFromUrl);
      } else if (providers.length > 0 && !providers.includes(provider)) {
        setProvider(providers[0]);
      }
    }).catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Load VMs when provider changes
  useEffect(() => {
    if (availableProviders.length === 0) { setLoading(false); return; }
    if (provider === 'proxmox') loadProxmoxVMs();
    else loadVmwareVMs();
  }, [provider, availableProviders]);

  const loadProxmoxVMs = async () => {
    setLoading(true); setError('');
    try {
      const res = await virtualization.proxmox.listVMs();
      setPxVMs(Array.isArray(res.data) ? res.data : []);
      virtualization.proxmox.getConnectionInfo()
        .then(r => setProxmoxUrl(r.data?.url || '')).catch(() => {});
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load Proxmox VMs'); }
    setLoading(false);
  };

  const loadVmwareVMs = async () => {
    setLoading(true); setError('');
    try {
      const [res, connRes, hostsRes] = await Promise.all([
        virtualization.vmware.listVMs(),
        virtualization.vmware.getConnectionInfo().catch(() => null),
        virtualization.vmware.listHosts().catch(() => null),
      ]);
      setVmwareVMs(Array.isArray(res.data) ? res.data : []);
      if (connRes?.data?.url) setVcenterUrl(connRes.data.url);
      if (hostsRes?.data) setVmwareHosts(Array.isArray(hostsRes.data) ? hostsRes.data : []);
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load VMware VMs'); }
    setLoading(false);
  };

  // Proxmox actions
  const handlePxAction = async (vm: ProxmoxVM, action: 'start' | 'stop' | 'shutdown' | 'reboot') => {
    setActionLoading(`px-${vm.node}-${vm.vmid}-${action}`);
    try {
      await virtualization.proxmox.vmAction(vm.node, vm.vmid, action);
      setTimeout(loadProxmoxVMs, 2000);
    } catch (err) { setError(err instanceof Error ? err.message : 'Action failed'); }
    setActionLoading(null);
  };

  // VMware actions
  const handleVmwareAction = async (vm: VMwareVM, action: 'start' | 'stop' | 'shutdown' | 'reboot' | 'suspend') => {
    setActionLoading(`vm-${vm.vm}-${action}`);
    try {
      await virtualization.vmware.vmAction(vm.vm, action);
      setTimeout(loadVmwareVMs, 2000);
    } catch (err) { setError(err instanceof Error ? err.message : 'Action failed'); }
    setActionLoading(null);
  };

  // VMware Details handler
  const openVmwareDetail = async (vm: VMwareVM) => {
    const reqId = ++detailRequestId.current;
    setDetailLoading(true);
    setVmwareDetail(null);
    try {
      const res = await virtualization.vmware.getVM(vm.vm);
      // Ignore stale responses if user clicked another VM meanwhile.
      if (reqId !== detailRequestId.current) return;
      setVmwareDetail(res.data);
    } catch (err) {
      if (reqId !== detailRequestId.current) return;
      setError(err instanceof Error ? err.message : 'Failed to load VM details');
    }
    setDetailLoading(false);
  };

  // VMware Clone handlers
  const openCloneModal = async (vm: VMwareVM) => {
    setCloneTarget(vm);
    setCloneForm({ name: `${vm.name}-clone`, host: vm.host || '', datastore: '', power_on: false });
    try {
      const [hostsRes, dsRes] = await Promise.all([
        virtualization.vmware.listHosts(),
        virtualization.vmware.listDatastores(),
      ]);
      setVmwareHosts(Array.isArray(hostsRes.data) ? hostsRes.data : []);
      setVmwareDatastores(Array.isArray(dsRes.data) ? dsRes.data : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load hosts and datastores for clone');
    }
  };

  const handleClone = async () => {
    if (!cloneTarget || !cloneForm.name.trim()) { setError('VM name is required'); return; }
    setCloning(true); setError('');
    try {
      await virtualization.vmware.cloneVM(cloneTarget.vm, {
        name: cloneForm.name.trim(),
        host: cloneForm.host || undefined,
        datastore: cloneForm.datastore || undefined,
        power_on: cloneForm.power_on,
      });
      setCloneTarget(null);
      setTimeout(loadVmwareVMs, 3000);
    } catch (err) { setError(err instanceof Error ? err.message : 'Clone failed'); }
    setCloning(false);
  };

  // VMware Edit Config handlers
  const openEditModal = (vm: VMwareVM) => {
    setEditTarget(vm);
    setEditForm({ cores: vm.cpu_count || 1, memory_mib: vm.memory_size_mib || 512 });
    setEditConfirmOpen(false);
  };

  const handleEditConfig = async () => {
    if (!editTarget) return;
    if (editForm.cores < 1 || editForm.memory_mib < 256) {
      setError('CPU cores must be >= 1 and memory >= 256 MB');
      return;
    }
    setEditing(true); setError('');
    try {
      await virtualization.vmware.reconfigureVM(editTarget.vm, {
        cores: editForm.cores,
        memory_mib: editForm.memory_mib,
      });
      setEditTarget(null);
      setEditConfirmOpen(false);
      setTimeout(loadVmwareVMs, 2000);
    } catch (err) { setError(err instanceof Error ? err.message : 'Reconfigure failed'); }
    setEditing(false);
  };

  // Delete handlers
  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      if (deleteTarget.type === 'proxmox') {
        const vm = pxVMs.find(v => `${v.node}-${v.vmid}` === deleteTarget.key);
        if (vm) await virtualization.proxmox.deleteVM(vm.node, vm.vmid);
        setTimeout(loadProxmoxVMs, 1500);
      } else {
        const vm = vmwareVMs.find(v => v.vm === deleteTarget.key);
        if (vm) await virtualization.vmware.deleteVM(vm.vm);
        setTimeout(loadVmwareVMs, 1500);
      }
      setDeleteTarget(null);
    } catch (err) { setError(err instanceof Error ? err.message : 'Delete failed'); }
    setDeleting(false);
  };

  // Proxmox create
  const openCreateModal = async () => {
    setShowCreate(true); setError('');
    try {
      const [nodesRes, storageRes, vmsRes, nextIdRes] = await Promise.all([
        virtualization.proxmox.listNodes(), virtualization.proxmox.listStorage(),
        virtualization.proxmox.listVMs(), virtualization.proxmox.nextId().catch(() => null),
      ]);
      const nodeList = Array.isArray(nodesRes.data) ? nodesRes.data : [];
      setNodes(nodeList);
      const storageList = Array.isArray(storageRes.data) ? storageRes.data : [];
      setStorages(storageList);
      const allVMs = Array.isArray(vmsRes.data) ? vmsRes.data : [];
      setTemplates(allVMs.filter(v => v.template === 1));
      const firstNode = nodeList.find(n => n.status === 'online')?.node || '';
      const diskStorage = storageList.find(s => (s.content || '').includes('images'))?.storage || storageList[0]?.storage || '';
      setForm(f => ({ ...f, node: f.node || firstNode, storage: f.storage || diskStorage,
        vmid: f.vmid || (nextIdRes?.data?.vmid ? String(nextIdRes.data.vmid) : ''),
        template: f.template || allVMs.find(v => v.template === 1)?.vmid || 0,
      }));
      const isoStorages = storageList.filter(s => (s.content || '').includes('iso'));
      const isoResults = await Promise.all(
        isoStorages.map(s => virtualization.proxmox.listStorageContent(s.storage, 'iso')
          .then(res => (Array.isArray(res.data) ? res.data.map(c => c.volid) : []))
          .catch(() => [] as string[]))
      );
      setIsos(isoResults.flat());
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load creation options'); }
  };

  const handleCreate = async () => {
    if (!form.name.trim()) { setError('VM name is required'); return; }
    if (!form.node) { setError('Select a node'); return; }
    setCreating(true); setError('');
    try {
      await virtualization.proxmox.createVM({
        name: form.name.trim(), node: form.node,
        vmid: form.vmid ? parseInt(form.vmid, 10) : undefined,
        cores: form.cores, memory_mb: form.memory_mb,
        disk_size: form.source === 'template' ? undefined : form.disk_size,
        storage: form.source === 'template' ? undefined : form.storage,
        network: form.network, start: form.start,
        template: form.source === 'template' && form.template ? form.template : undefined,
        iso: form.source === 'iso' && form.iso ? form.iso : undefined,
      });
      setShowCreate(false); setForm(f => ({ ...f, name: '' }));
      setTimeout(loadProxmoxVMs, 1500);
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to create VM'); }
    setCreating(false);
  };

  const pxStatusColor = (s: string) => {
    switch (s) {
      case 'running': return 'bg-emerald-500/10 text-emerald-600';
      case 'stopped': return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
      default: return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
    }
  };

  const vmwarePowerColor = (s: string) => {
    switch (s) {
      case 'POWERED_ON': return 'bg-emerald-500/10 text-emerald-600';
      case 'POWERED_OFF': return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
      case 'SUSPENDED': return 'bg-yellow-500/10 text-yellow-600';
      default: return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const gb = bytes / 1024 / 1024 / 1024;
    if (gb >= 1) return `${gb.toFixed(1)} GB`;
    return `${(bytes / 1024 / 1024).toFixed(0)} MB`;
  };

  const pxVisibleVMs = pxVMs.filter(vm => !vm.template);
  // Filtered VMware VMs (by host if filter is set)
  const vmwareFilteredVMs = vmwareHostFilter ? vmwareVMs.filter(vm => vm.host === vmwareHostFilter) : vmwareVMs;
  const totalVMs = provider === 'proxmox' ? pxVisibleVMs.length : vmwareFilteredVMs.length;

  // VMware memory summary (uses filtered VMs)
  const vmTotalMemAllocated = vmwareFilteredVMs.reduce((s, vm) => s + (vm.memory_size_mib || 0), 0);

  // ESXi host grouping
  const [collapsedHostGroups, setCollapsedHostGroups] = useState<Set<string>>(new Set());
  const toggleHostGroup = (hostId: string) => {
    setCollapsedHostGroups(prev => {
      const next = new Set(prev);
      if (next.has(hostId)) next.delete(hostId); else next.add(hostId);
      return next;
    });
  };

  const vmwareHostGroups = useMemo(() => {
    const hostMap = new Map<string, string>();
    vmwareHosts.forEach(h => hostMap.set(h.host, h.name));
    const groups = new Map<string, { hostId: string; hostName: string; vms: VMwareVM[] }>();
    vmwareVMs.forEach(vm => {
      const hostId = vm.host || '__unknown__';
      // Apply host filter if set
      if (vmwareHostFilter && hostId !== vmwareHostFilter) return;
      const hostName = hostMap.get(hostId) || 'Unknown Host';
      if (!groups.has(hostId)) groups.set(hostId, { hostId, hostName, vms: [] });
      groups.get(hostId)!.vms.push(vm);
    });
    return Array.from(groups.values())
      .sort((a, b) => a.hostName.localeCompare(b.hostName));
  }, [vmwareVMs, vmwareHosts, vmwareHostFilter]);

  // No connections state
  if (!loading && availableProviders.length === 0) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <div className="page-animate flex items-center justify-between">
            <div>
              <h1 className="page-title-modern">Virtual Machines</h1>
              <p className="page-subtitle-modern">Manage virtual machines across providers</p>
            </div>
            <Link href="/virtualization" className="btn btn-secondary">Back to Dashboard</Link>
          </div>
          <div className="card card-body text-center py-16">
            <p className="text-[14px] text-[var(--text-secondary)] mb-2">No virtualization connections configured</p>
            <Link href="/connections" className="btn btn-primary">+ Add Connection</Link>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Virtual Machines</h1>
            <p className="page-subtitle-modern">
              {provider === 'proxmox'
                ? `Manage QEMU virtual machines across Proxmox nodes`
                : `Manage VMware virtual machines in vCenter`}
            </p>
          </div>
          <div className="flex items-center gap-2">
            {availableProviders.length > 1 && (
              <div className="flex rounded-lg border border-[var(--border)] overflow-hidden">
                {availableProviders.includes('proxmox') && (
                  <button onClick={() => setProvider('proxmox')}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${provider === 'proxmox' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}>
                    Proxmox
                  </button>
                )}
                {availableProviders.includes('vmware') && (
                  <button onClick={() => setProvider('vmware')}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${provider === 'vmware' ? 'bg-[var(--accent)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'}`}>
                    VMware
                  </button>
                )}
              </div>
            )}
            <Link href="/virtualization" className="btn btn-secondary">Back to Dashboard</Link>
            {provider === 'proxmox' && (
              <button onClick={openCreateModal} className="btn btn-primary">+ Create VM</button>
            )}
          </div>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-xl p-3 text-[12px] text-red-500 flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError('')} className="text-xs">Dismiss</button>
          </div>
        )}

        {loading ? (
          <div className="card card-body text-center py-16">
            <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
              <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <span className="text-[13px]">Loading VMs...</span>
            </div>
          </div>
        ) : totalVMs === 0 ? (
          <div className="card card-body text-center py-16">
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">No virtual machines found</p>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              {provider === 'proxmox' ? 'Create your first VM with the button above' : 'Check your vCenter connection settings'}
            </p>
          </div>
        ) : provider === 'proxmox' ? (
          /* ── Proxmox VMs table ── */
          <div className="card overflow-hidden">
            <table className="w-full text-[12px]">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">VMID</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Name</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Node</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Status</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">CPU</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Memory</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Disk</th>
                  <th className="text-right px-4 py-3 font-medium text-[var(--text-tertiary)]">Actions</th>
                </tr>
              </thead>
              <tbody>
                {pxVisibleVMs.map(vm => (
                  <tr key={`${vm.node}-${vm.vmid}`} className="border-b border-[var(--border-light)] hover:bg-[var(--border-light)]/30 transition-colors">
                    <td className="px-4 py-3 font-mono text-[var(--text-secondary)]">{vm.vmid}</td>
                    <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                      {vm.name || `VM ${vm.vmid}`}
                      {proxmoxUrl && (
                        <a href={`${proxmoxUrl}/#v1:0:1:=qemu=${vm.node}=${vm.vmid}`} target="_blank" rel="noreferrer"
                          className="block text-[10px] text-[var(--accent)] hover:underline font-normal">Open in Proxmox</a>
                      )}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.node}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${pxStatusColor(vm.status)}`}>{vm.status}</span>
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.status === 'running' ? `${Math.round(vm.cpu * 100)}%` : '-'}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {vm.status === 'running' ? formatBytes(vm.mem) : '-'} / {formatBytes(vm.maxmem)}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{formatBytes(vm.disk)} / {formatBytes(vm.maxdisk)}</td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1">
                        {vm.status === 'stopped' && (
                          <>
                            <button onClick={() => handlePxAction(vm, 'start')}
                              disabled={actionLoading === `px-${vm.node}-${vm.vmid}-start`}
                              className="px-2 py-1 text-[11px] text-emerald-600 hover:bg-emerald-500/10 rounded transition-colors disabled:opacity-50">Start</button>
                            <button onClick={() => setDeleteTarget({ type: 'proxmox', name: vm.name || `VM ${vm.vmid}`, key: `${vm.node}-${vm.vmid}` })}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50">Delete</button>
                          </>
                        )}
                        {vm.status === 'running' && (
                          <>
                            <button onClick={() => handlePxAction(vm, 'shutdown')} disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-yellow-600 hover:bg-yellow-500/10 rounded transition-colors disabled:opacity-50">Shutdown</button>
                            <button onClick={() => handlePxAction(vm, 'stop')} disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50">Stop</button>
                            <button onClick={() => handlePxAction(vm, 'reboot')} disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded transition-colors disabled:opacity-50">Reboot</button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          /* ── VMware VMs table ── */
          <div className="space-y-4">
            {/* VMware Memory Stats */}
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
              <div className="card card-body !py-3 !px-4">
                <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wide">Powered On</div>
                <div className="text-lg font-bold text-[var(--text-primary)]">{vmwareFilteredVMs.filter(v => v.power_state === 'POWERED_ON').length}</div>
              </div>
              <div className="card card-body !py-3 !px-4">
                <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wide">Total VMs</div>
                <div className="text-lg font-bold text-[var(--text-primary)]">{vmwareFilteredVMs.length}</div>
              </div>
              <div className="card card-body !py-3 !px-4 md:col-span-1">
                <div className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wide">Memory Allocated</div>
                <div className="text-lg font-bold text-[var(--text-primary)]">
                  {vmTotalMemAllocated > 0 ? `${(vmTotalMemAllocated / 1024).toFixed(1)} GB` : '—'}
                </div>
              </div>
            </div>
            {/* VMware Filters */}
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex-1 min-w-[200px]">
                <input
                  type="text"
                  placeholder="Search VMs by name..."
                  value={vmwareSearch}
                  onChange={e => setVmwareSearch(e.target.value)}
                  className="w-full px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20"
                />
              </div>
              <select
                value={vmwarePowerFilter}
                onChange={e => setVmwarePowerFilter(e.target.value)}
                className="px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20"
              >
                <option value="">All States</option>
                <option value="POWERED_ON">Powered On</option>
                <option value="POWERED_OFF">Powered Off</option>
                <option value="SUSPENDED">Suspended</option>
              </select>
            </div>
            {/* Host filter indicator */}
            {vmwareHostFilter && (
              <div className="flex items-center gap-2 px-3 py-2 bg-[var(--accent-subtle)] border border-[var(--accent)]/20 rounded-lg">
                <span className="text-[11px] text-[var(--accent)] font-medium">
                  Filtered by host: {vmwareHosts.find(h => h.host === vmwareHostFilter)?.name || vmwareHostFilter}
                </span>
                <button
                  onClick={() => setVmwareHostFilter(null)}
                  className="ml-auto text-[11px] px-2 py-0.5 text-[var(--accent)] hover:bg-[var(--accent)]/10 rounded transition-colors"
                >
                  Clear filter
                </button>
              </div>
            )}
            {/* ESXi Host Groups */}
            <div className="space-y-3">
              {vmwareHostGroups.map(group => {
                const filteredVMs = group.vms.filter(vm => {
                  if (vmwareSearch && !vm.name.toLowerCase().includes(vmwareSearch.toLowerCase())) return false;
                  if (vmwarePowerFilter && vm.power_state !== vmwarePowerFilter) return false;
                  return true;
                });
                if (filteredVMs.length === 0 && (vmwareSearch || vmwarePowerFilter)) return null;
                const isCollapsed = collapsedHostGroups.has(group.hostId);
                const poweredOn = group.vms.filter(v => v.power_state === 'POWERED_ON').length;
                const groupMem = group.vms.reduce((s, vm) => s + (vm.memory_size_mib || 0), 0);
                return (
                  <div key={group.hostId} className="card overflow-hidden">
                    {/* Group Header */}
                    <button
                      onClick={() => toggleHostGroup(group.hostId)}
                      className="w-full flex items-center justify-between px-4 py-3 bg-[var(--surface)] hover:bg-[var(--border-light)]/40 transition-colors border-b border-[var(--border)]"
                    >
                      <div className="flex items-center gap-3">
                        <svg className={`w-3.5 h-3.5 text-[var(--text-tertiary)] transition-transform ${isCollapsed ? '' : 'rotate-90'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                        </svg>
                        <div className="flex items-center gap-2">
                          <svg className="w-4 h-4 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 14.25h13.5m-13.5 0a1.5 1.5 0 01-1.5-1.5m1.5 1.5a1.5 1.5 0 00-1.5 1.5v3a1.5 1.5 0 001.5 1.5h13.5a1.5 1.5 0 001.5-1.5v-3a1.5 1.5 0 00-1.5-1.5m-16.5 0a1.5 1.5 0 01-1.5-1.5v-3a1.5 1.5 0 011.5-1.5h13.5a1.5 1.5 0 011.5 1.5v3a1.5 1.5 0 01-1.5 1.5m0-9.5v-3a1.5 1.5 0 00-1.5-1.5h-13.5a1.5 1.5 0 00-1.5 1.5m16.5 9.5v-3a1.5 1.5 0 00-1.5-1.5h-13.5a1.5 1.5 0 00-1.5 1.5" />
                          </svg>
                          <span className="text-[13px] font-semibold text-[var(--text-primary)]">{group.hostName}</span>
                        </div>
                        <span className="text-[11px] text-[var(--text-tertiary)]">{group.vms.length} VMs</span>
                        <span className="text-[11px] text-emerald-600">{poweredOn} on</span>
                        {groupMem > 0 && (
                          <span className="text-[11px] text-[var(--text-tertiary)]">{(groupMem / 1024).toFixed(1)} GB</span>
                        )}
                      </div>
                    </button>
                    {/* VM Table (hidden when collapsed) */}
                    {!isCollapsed && (
                      <table className="w-full text-[12px]">
                        <thead>
                          <tr className="border-b border-[var(--border)]">
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">Name</th>
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">Power State</th>
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">CPUs</th>
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">Memory</th>
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">Guest OS</th>
                            <th className="text-left px-4 py-2.5 font-medium text-[var(--text-tertiary)]">IP Address</th>
                            <th className="text-right px-4 py-2.5 font-medium text-[var(--text-tertiary)]">Actions</th>
                          </tr>
                        </thead>
                        <tbody>
                          {filteredVMs.map(vm => (
                            <tr key={vm.vm} className="border-b border-[var(--border-light)] hover:bg-[var(--border-light)]/30 transition-colors">
                              <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                                <button onClick={() => openVmwareDetail(vm)} className="text-left hover:text-[var(--accent)] transition-colors">
                                  {vm.name}
                                </button>
                                {vcenterUrl && (
                                  <a href={`${vcenterUrl}/ui/app?vm=${vm.vm}`} target="_blank" rel="noreferrer"
                                    className="block text-[10px] text-[var(--accent)] hover:underline font-normal">Open in vCenter</a>
                                )}
                              </td>
                              <td className="px-4 py-3">
                                <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${vmwarePowerColor(vm.power_state)}`}>
                                  {vm.power_state?.replace('POWERED_', '').toLowerCase()}
                                </span>
                              </td>
                              <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.cpu_count}</td>
                              <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.memory_size_mib ? `${(vm.memory_size_mib / 1024).toFixed(1)} GB` : '-'}</td>
                              <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.guest_OS || '-'}</td>
                              <td className="px-4 py-3 font-mono text-[var(--text-secondary)]">{vm.ip_address || '-'}</td>
                              <td className="px-4 py-3">
                                <div className="flex justify-end gap-1">
                                  {vm.power_state === 'POWERED_OFF' && (
                                    <button onClick={() => handleVmwareAction(vm, 'start')}
                                      disabled={actionLoading === `vm-${vm.vm}-start`}
                                      className="px-2 py-1 text-[11px] text-emerald-600 hover:bg-emerald-500/10 rounded transition-colors disabled:opacity-50">Start</button>
                                  )}
                                  {vm.power_state === 'POWERED_ON' && (
                                    <>
                                      <button onClick={() => handleVmwareAction(vm, 'shutdown')} disabled={!!actionLoading}
                                        className="px-2 py-1 text-[11px] text-yellow-600 hover:bg-yellow-500/10 rounded transition-colors disabled:opacity-50">Shutdown</button>
                                      <button onClick={() => handleVmwareAction(vm, 'stop')} disabled={!!actionLoading}
                                        className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50">Stop</button>
                                      <button onClick={() => handleVmwareAction(vm, 'reboot')} disabled={!!actionLoading}
                                        className="px-2 py-1 text-[11px] text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded transition-colors disabled:opacity-50">Reboot</button>
                                    </>
                                  )}
                                  <button onClick={() => openCloneModal(vm)} disabled={!!actionLoading}
                                    className="px-2 py-1 text-[11px] text-blue-600 hover:bg-blue-500/10 rounded transition-colors disabled:opacity-50">Clone</button>
                                  <button onClick={() => openEditModal(vm)} disabled={!!actionLoading}
                                    className="px-2 py-1 text-[11px] text-purple-600 hover:bg-purple-500/10 rounded transition-colors disabled:opacity-50">Edit</button>
                                  <button onClick={() => setDeleteTarget({ type: 'vmware', name: vm.name, key: vm.vm })}
                                    disabled={!!actionLoading}
                                    className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50">Delete</button>
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {/* Proxmox Create VM Modal */}
      {showCreate && provider === 'proxmox' && (
        <CreateVMModal form={form} setForm={setForm} nodes={nodes} storages={storages}
          templates={templates} isos={isos} creating={creating}
          onCreate={handleCreate} onClose={() => setShowCreate(false)} />
      )}

      <ConfirmModal
        open={!!deleteTarget}
        title={`Delete ${deleteTarget?.name || ''}?`}
        description={`"${deleteTarget?.name || ''}" will be permanently destroyed. This cannot be undone.`}
        confirmLabel="Delete" variant="danger" loading={deleting}
        onConfirm={handleDelete} onCancel={() => setDeleteTarget(null)}
      />

      {/* VMware Details Modal */}
      {provider === 'vmware' && (vmwareDetail !== null || detailLoading) && (
        <VMDetailModal detail={vmwareDetail} loading={detailLoading} onClose={() => setVmwareDetail(null)} />
      )}

      {/* VMware Clone Modal */}
      {cloneTarget && (
        <CloneVMModal vm={cloneTarget} form={cloneForm} setForm={setCloneForm}
          hosts={vmwareHosts} datastores={vmwareDatastores} cloning={cloning}
          onClone={handleClone} onClose={() => setCloneTarget(null)} />
      )}

      {/* VMware Edit Config Modal */}
      {editTarget && (
        <EditVMConfigModal vm={editTarget} form={editForm} setForm={setEditForm}
          editing={editing} onSave={() => setEditConfirmOpen(true)} onClose={() => setEditTarget(null)} />
      )}

      {/* Reconfigure confirmation */}
      <ConfirmModal
        open={editConfirmOpen}
        title={`Reconfigure ${editTarget?.name || ''}?`}
        description={`CPU: ${editForm.cores} cores, Memory: ${editForm.memory_mib} MB. The VM may need to be powered off for changes to take effect.`}
        confirmLabel="Apply Changes" variant="danger" loading={editing}
        onConfirm={handleEditConfig} onCancel={() => setEditConfirmOpen(false)}
      />
    </div>
  );
}

/* ── Create VM Modal (Proxmox only) ── */

interface CreateVMModalProps {
  form: { name: string; node: string; vmid: string; cores: number; memory_mb: number;
    disk_size: string; storage: string; network: string; source: VMSource; template: number; iso: string; start: boolean; };
  setForm: React.Dispatch<React.SetStateAction<CreateVMModalProps['form']>>;
  nodes: ProxmoxNode[]; storages: ProxmoxStorage[]; templates: ProxmoxVM[]; isos: string[];
  creating: boolean; onCreate: () => void; onClose: () => void;
}

function CreateVMModal({ form, setForm, nodes, storages, templates, isos, creating, onCreate, onClose }: CreateVMModalProps) {
  useEscapeKey(() => { if (!creating) onClose(); });
  const set = <K extends keyof CreateVMModalProps['form']>(key: K, value: CreateVMModalProps['form'][K]) =>
    setForm(f => ({ ...f, [key]: value }));
  const inputClass = 'w-full px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20';

  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => !creating && onClose()} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-lg mx-4 overflow-hidden glass-modal max-h-[90vh] flex flex-col">
        <div className="px-5 py-4 border-b border-[#f0f0f0] flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-[#171717]">Create Virtual Machine</h3>
          <button onClick={() => !creating && onClose()} className="text-[#999] hover:text-[#333] text-lg leading-none">&times;</button>
        </div>
        <div className="p-5 space-y-4 overflow-y-auto">
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Source</label>
            <div className="flex gap-1 p-1 bg-[var(--border-light)]/50 rounded-lg">
              {([['template', 'Clone template'], ['iso', 'Boot from ISO'], ['empty', 'Empty disk']] as Array<[VMSource, string]>).map(([value, label]) => (
                <button key={value} onClick={() => set('source', value)}
                  className={`flex-1 px-2 py-1.5 text-[11px] rounded-md transition-colors ${form.source === value ? 'bg-[var(--surface)] text-[var(--text-primary)] font-medium shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}>
                  {label}
                </button>
              ))}
            </div>
          </div>
          {form.source === 'template' && (
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Template VM</label>
              {templates.length === 0 ? (
                <p className="text-[11px] text-[var(--text-tertiary)]">No templates found.</p>
              ) : (
                <select value={form.template} onChange={e => set('template', parseInt(e.target.value, 10))} className={inputClass}>
                  {templates.map(t => <option key={`${t.node}-${t.vmid}`} value={t.vmid}>{t.vmid} — {t.name || 'unnamed'} ({t.node})</option>)}
                </select>
              )}
            </div>
          )}
          {form.source === 'iso' && (
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">ISO Image</label>
              {isos.length === 0 ? (
                <p className="text-[11px] text-[var(--text-tertiary)]">No ISO images found.</p>
              ) : (
                <select value={form.iso} onChange={e => set('iso', e.target.value)} className={inputClass}>
                  <option value="">Select ISO...</option>
                  {isos.map(iso => <option key={iso} value={iso}>{iso}</option>)}
                </select>
              )}
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Name</label>
              <input value={form.name} onChange={e => set('name', e.target.value)} placeholder="my-vm" className={inputClass} /></div>
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">VMID</label>
              <input value={form.vmid} onChange={e => set('vmid', e.target.value.replace(/\D/g, ''))} placeholder="auto" className={inputClass} /></div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Node</label>
              <select value={form.node} onChange={e => set('node', e.target.value)} className={inputClass}>
                <option value="">Select node...</option>
                {nodes.filter(n => n.status === 'online').map(n => <option key={n.node} value={n.node}>{n.node}</option>)}
              </select></div>
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Network bridge</label>
              <input value={form.network} onChange={e => set('network', e.target.value)} placeholder="vmbr0" className={inputClass} /></div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Cores</label>
              <input type="number" min={1} value={form.cores} onChange={e => set('cores', parseInt(e.target.value, 10) || 1)} className={inputClass} /></div>
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Memory (MB)</label>
              <input type="number" min={256} step={256} value={form.memory_mb} onChange={e => set('memory_mb', parseInt(e.target.value, 10) || 512)} className={inputClass} /></div>
            {form.source !== 'template' && (
              <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk size</label>
                <input value={form.disk_size} onChange={e => set('disk_size', e.target.value)} placeholder="32G" className={inputClass} /></div>
            )}
          </div>
          {form.source !== 'template' && (
            <div><label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk storage</label>
              <select value={form.storage} onChange={e => set('storage', e.target.value)} className={inputClass}>
                <option value="">Select storage...</option>
                {storages.map(s => <option key={s.storage} value={s.storage}>{s.storage} ({s.type})</option>)}
              </select></div>
          )}
          <label className="flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
            <input type="checkbox" checked={form.start} onChange={e => set('start', e.target.checked)} className="rounded" />
            Start VM after creation
          </label>
        </div>
        <div className="flex items-center gap-2 px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onClose} disabled={creating} className="btn btn-secondary flex-1 justify-center">Cancel</button>
          <button onClick={onCreate} disabled={creating || !form.name.trim() || !form.node} className="btn btn-primary flex-1 justify-center">
            {creating ? 'Creating...' : 'Create VM'}
          </button>
        </div>
      </div>
    </div>
  );
}

/* ── VMware VM Details Modal ── */

function VMDetailModal({ detail, loading, onClose }: { detail: VMwareVMDetail | null; loading: boolean; onClose: () => void }) {
  useEscapeKey(onClose);
  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-md mx-4 overflow-hidden glass-modal">
        <div className="px-5 py-4 border-b border-[#f0f0f0] flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-[#171717]">VM Details</h3>
          <button onClick={onClose} className="text-[#999] hover:text-[#333] text-lg leading-none">&times;</button>
        </div>
        <div className="p-5">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <svg className="animate-spin w-5 h-5 text-[var(--accent)]" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            </div>
          ) : detail ? (
            <div className="space-y-3 text-[12px]">
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Name</span><span className="font-medium">{detail.name}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Power State</span><span>{detail.power_state}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">CPU</span><span>{detail.cpu.count} cores ({detail.cpu.cores_per_socket} per socket)</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Memory</span><span>{detail.memory.size_MiB} MB</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Guest OS</span><span>{detail.guest.name || detail.guest.os || '-'}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Hostname</span><span>{detail.guest.host_name || '-'}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">IP Address</span><span className="font-mono">{detail.guest.ip_address || '-'}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Host</span><span>{detail.host || '-'}</span></div>
              <div className="flex justify-between"><span className="text-[var(--text-tertiary)]">Cluster</span><span>{detail.cluster || '-'}</span></div>
            </div>
          ) : (
            <p className="text-[12px] text-[var(--text-tertiary)] text-center py-4">No details available</p>
          )}
        </div>
        <div className="flex items-center px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onClose} className="btn btn-secondary w-full justify-center">Close</button>
        </div>
      </div>
    </div>
  );
}

/* ── VMware Clone VM Modal ── */

function CloneVMModal({ vm, form, setForm, hosts, datastores, cloning, onClone, onClose }: {
  vm: VMwareVM;
  form: { name: string; host: string; datastore: string; power_on: boolean };
  setForm: React.Dispatch<React.SetStateAction<typeof form>>;
  hosts: VMwareHost[];
  datastores: VMwareDatastore[];
  cloning: boolean;
  onClone: () => void;
  onClose: () => void;
}) {
  useEscapeKey(() => { if (!cloning) onClose(); });
  const inputClass = 'w-full px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20';
  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => !cloning && onClose()} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-md mx-4 overflow-hidden glass-modal">
        <div className="px-5 py-4 border-b border-[#f0f0f0] flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-[#171717]">Clone VM: {vm.name}</h3>
          <button onClick={() => !cloning && onClose()} className="text-[#999] hover:text-[#333] text-lg leading-none">&times;</button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">New VM Name</label>
            <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} className={inputClass} placeholder="my-vm-clone" />
          </div>
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Target Host</label>
            <select value={form.host} onChange={e => setForm(f => ({ ...f, host: e.target.value }))} className={inputClass}>
              <option value="">Same as source ({vm.host || 'auto'})</option>
              {hosts.map(h => <option key={h.host} value={h.host}>{h.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Datastore</label>
            <select value={form.datastore} onChange={e => setForm(f => ({ ...f, datastore: e.target.value }))} className={inputClass}>
              <option value="">Same as source</option>
              {datastores.map(d => <option key={d.datastore} value={d.datastore}>{d.name}</option>)}
            </select>
          </div>
          <label className="flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
            <input type="checkbox" checked={form.power_on} onChange={e => setForm(f => ({ ...f, power_on: e.target.checked }))} className="rounded" />
            Power on after cloning
          </label>
        </div>
        <div className="flex items-center gap-2 px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onClose} disabled={cloning} className="btn btn-secondary flex-1 justify-center">Cancel</button>
          <button onClick={onClone} disabled={cloning || !form.name.trim()} className="btn btn-primary flex-1 justify-center">
            {cloning ? 'Cloning...' : 'Clone VM'}
          </button>
        </div>
      </div>
    </div>
  );
}

/* ── VMware Edit Config Modal ── */

function EditVMConfigModal({ vm, form, setForm, editing, onSave, onClose }: {
  vm: VMwareVM;
  form: { cores: number; memory_mib: number };
  setForm: React.Dispatch<React.SetStateAction<typeof form>>;
  editing: boolean;
  onSave: () => void;
  onClose: () => void;
}) {
  useEscapeKey(() => { if (!editing) onClose(); });
  const inputClass = 'w-full px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20';
  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => !editing && onClose()} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-sm mx-4 overflow-hidden glass-modal">
        <div className="px-5 py-4 border-b border-[#f0f0f0] flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-[#171717]">Edit: {vm.name}</h3>
          <button onClick={() => !editing && onClose()} className="text-[#999] hover:text-[#333] text-lg leading-none">&times;</button>
        </div>
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">CPU Cores</label>
            <input type="number" min={1} value={form.cores} onChange={e => setForm(f => ({ ...f, cores: parseInt(e.target.value, 10) || 1 }))} className={inputClass} />
          </div>
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Memory (MB)</label>
            <input type="number" min={256} step={256} value={form.memory_mib} onChange={e => setForm(f => ({ ...f, memory_mib: parseInt(e.target.value, 10) || 512 }))} className={inputClass} />
          </div>
          <p className="text-[10px] text-[var(--text-tertiary)]">Note: VM may need to be powered off for some changes to take effect.</p>
        </div>
        <div className="flex items-center gap-2 px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onClose} disabled={editing} className="btn btn-secondary flex-1 justify-center">Cancel</button>
          <button onClick={onSave} disabled={editing} className="btn btn-primary flex-1 justify-center">
            {editing ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  );
}
