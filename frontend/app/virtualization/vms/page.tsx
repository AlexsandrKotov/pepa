'use client';

import { useState, useEffect } from 'react';
import {
  virtualization,
  type ProxmoxVM,
  type ProxmoxNode,
  type ProxmoxStorage,
} from '@/lib/api';
import Link from 'next/link';
import ConfirmModal from '@/components/ConfirmModal';
import { useEscapeKey } from '@/hooks/useEscapeKey';

type VMSource = 'template' | 'iso' | 'empty';

export default function VirtualMachinesPage() {
  const [vms, setVMs] = useState<ProxmoxVM[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [proxmoxUrl, setProxmoxUrl] = useState('');

  // Create modal state
  const [showCreate, setShowCreate] = useState(false);
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [storages, setStorages] = useState<ProxmoxStorage[]>([]);
  const [templates, setTemplates] = useState<ProxmoxVM[]>([]);
  const [isos, setIsos] = useState<string[]>([]);
  const [form, setForm] = useState({
    name: '',
    node: '',
    vmid: '',
    cores: 2,
    memory_mb: 2048,
    disk_size: '32G',
    storage: '',
    network: 'vmbr0',
    source: 'template' as VMSource,
    template: 0,
    iso: '',
    start: true,
  });
  const [creating, setCreating] = useState(false);

  // Delete confirm state
  const [deleteTarget, setDeleteTarget] = useState<ProxmoxVM | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    loadVMs();
    virtualization.proxmox.getConnectionInfo()
      .then(res => setProxmoxUrl(res.data?.url || ''))
      .catch(() => {});
  }, []);

  const loadVMs = async () => {
    setLoading(true);
    try {
      const res = await virtualization.proxmox.listVMs();
      setVMs(Array.isArray(res.data) ? res.data : []);
    } catch {
      setError('Failed to load VMs');
    }
    setLoading(false);
  };

  const openCreateModal = async () => {
    setShowCreate(true);
    setError('');
    try {
      const [nodesRes, storageRes, vmsRes, nextIdRes] = await Promise.all([
        virtualization.proxmox.listNodes(),
        virtualization.proxmox.listStorage(),
        virtualization.proxmox.listVMs(),
        virtualization.proxmox.nextId().catch(() => null),
      ]);
      const nodeList = Array.isArray(nodesRes.data) ? nodesRes.data : [];
      setNodes(nodeList);
      const storageList = Array.isArray(storageRes.data) ? storageRes.data : [];
      setStorages(storageList);
      const allVMs = Array.isArray(vmsRes.data) ? vmsRes.data : [];
      setTemplates(allVMs.filter(v => v.template === 1));
      const firstNode = nodeList.find(n => n.status === 'online')?.node || '';
      const diskStorage = storageList.find(s => (s.content || '').includes('images'))?.storage || storageList[0]?.storage || '';
      setForm(f => ({
        ...f,
        node: f.node || firstNode,
        storage: f.storage || diskStorage,
        vmid: f.vmid || (nextIdRes?.data?.vmid ? String(nextIdRes.data.vmid) : ''),
        template: f.template || allVMs.find(v => v.template === 1)?.vmid || 0,
      }));
      // Collect ISOs from storages that serve ISO content
      const isoStorages = storageList.filter(s => (s.content || '').includes('iso'));
      const isoResults = await Promise.all(
        isoStorages.map(s =>
          virtualization.proxmox.listStorageContent(s.storage, 'iso')
            .then(res => (Array.isArray(res.data) ? res.data.map(c => c.volid) : []))
            .catch(() => [] as string[])
        )
      );
      setIsos(isoResults.flat());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load creation options');
    }
  };

  const handleCreate = async () => {
    if (!form.name.trim()) { setError('VM name is required'); return; }
    if (!form.node) { setError('Select a node'); return; }
    setCreating(true);
    setError('');
    try {
      const payload = {
        name: form.name.trim(),
        node: form.node,
        vmid: form.vmid ? parseInt(form.vmid, 10) : undefined,
        cores: form.cores,
        memory_mb: form.memory_mb,
        disk_size: form.source === 'template' ? undefined : form.disk_size,
        storage: form.source === 'template' ? undefined : form.storage,
        network: form.network,
        start: form.start,
        template: form.source === 'template' && form.template ? form.template : undefined,
        iso: form.source === 'iso' && form.iso ? form.iso : undefined,
      };
      await virtualization.proxmox.createVM(payload);
      setShowCreate(false);
      setForm(f => ({ ...f, name: '' }));
      setTimeout(loadVMs, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create VM');
    }
    setCreating(false);
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await virtualization.proxmox.deleteVM(deleteTarget.node, deleteTarget.vmid);
      setDeleteTarget(null);
      setTimeout(loadVMs, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete VM');
      setDeleteTarget(null);
    }
    setDeleting(false);
  };

  const handleAction = async (vm: ProxmoxVM, action: 'start' | 'stop' | 'shutdown' | 'reboot') => {
    setActionLoading(`${vm.node}-${vm.vmid}-${action}`);
    try {
      await virtualization.proxmox.vmAction(vm.node, vm.vmid, action);
      setTimeout(loadVMs, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action failed');
    }
    setActionLoading(null);
  };

  const statusColor = (s: string) => {
    switch (s) {
      case 'running': return 'bg-emerald-500/10 text-emerald-600';
      case 'stopped': return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
      case 'paused': return 'bg-yellow-500/10 text-yellow-600';
      default: return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const gb = bytes / 1024 / 1024 / 1024;
    if (gb >= 1) return `${gb.toFixed(1)} GB`;
    const mb = bytes / 1024 / 1024;
    return `${mb.toFixed(0)} MB`;
  };

  const visibleVMs = vms.filter(vm => !vm.template);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Virtual Machines</h1>
            <p className="page-subtitle-modern">Manage QEMU virtual machines across Proxmox nodes</p>
          </div>
          <div className="flex items-center gap-2">
            <Link href="/virtualization" className="btn btn-secondary">Back to Dashboard</Link>
            <button onClick={openCreateModal} className="btn btn-primary">+ Create VM</button>
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
        ) : visibleVMs.length === 0 ? (
          <div className="card card-body text-center py-16">
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">No virtual machines found</p>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              Create your first VM with the button above
            </p>
          </div>
        ) : (
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
                {visibleVMs.map(vm => (
                  <tr key={`${vm.node}-${vm.vmid}`} className="border-b border-[var(--border-light)] hover:bg-[var(--border-light)]/30 transition-colors">
                    <td className="px-4 py-3 font-mono text-[var(--text-secondary)]">{vm.vmid}</td>
                    <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                      {vm.name || `VM ${vm.vmid}`}
                      {proxmoxUrl && (
                        <a
                          href={`${proxmoxUrl}/#v1:0:1:=qemu=${vm.node}=${vm.vmid}`}
                          target="_blank"
                          rel="noreferrer"
                          className="block text-[10px] text-[var(--accent)] hover:underline font-normal"
                        >
                          Open in Proxmox
                        </a>
                      )}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.node}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(vm.status)}`}>
                        {vm.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{vm.status === 'running' ? `${Math.round(vm.cpu * 100)}%` : '-'}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {vm.status === 'running' ? formatBytes(vm.mem) : '-'} / {formatBytes(vm.maxmem)}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {formatBytes(vm.disk)} / {formatBytes(vm.maxdisk)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1">
                        {vm.status === 'stopped' && (
                          <>
                            <button
                              onClick={() => handleAction(vm, 'start')}
                              disabled={actionLoading === `${vm.node}-${vm.vmid}-start`}
                              className="px-2 py-1 text-[11px] text-emerald-600 hover:bg-emerald-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Start
                            </button>
                            <button
                              onClick={() => setDeleteTarget(vm)}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Delete
                            </button>
                          </>
                        )}
                        {vm.status === 'running' && (
                          <>
                            <button
                              onClick={() => handleAction(vm, 'shutdown')}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-yellow-600 hover:bg-yellow-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Shutdown
                            </button>
                            <button
                              onClick={() => handleAction(vm, 'stop')}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Stop
                            </button>
                            <button
                              onClick={() => handleAction(vm, 'reboot')}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded transition-colors disabled:opacity-50"
                            >
                              Reboot
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showCreate && (
        <CreateVMModal
          form={form}
          setForm={setForm}
          nodes={nodes}
          storages={storages}
          templates={templates}
          isos={isos}
          creating={creating}
          onCreate={handleCreate}
          onClose={() => setShowCreate(false)}
        />
      )}

      <ConfirmModal
        open={!!deleteTarget}
        title={`Delete VM ${deleteTarget?.vmid || ''}?`}
        description={`"${deleteTarget?.name || ''}" on node ${deleteTarget?.node || ''} will be permanently destroyed. This cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}

interface CreateVMModalProps {
  form: {
    name: string;
    node: string;
    vmid: string;
    cores: number;
    memory_mb: number;
    disk_size: string;
    storage: string;
    network: string;
    source: VMSource;
    template: number;
    iso: string;
    start: boolean;
  };
  setForm: React.Dispatch<React.SetStateAction<CreateVMModalProps['form']>>;
  nodes: ProxmoxNode[];
  storages: ProxmoxStorage[];
  templates: ProxmoxVM[];
  isos: string[];
  creating: boolean;
  onCreate: () => void;
  onClose: () => void;
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
          {/* Source */}
          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Source</label>
            <div className="flex gap-1 p-1 bg-[var(--border-light)]/50 rounded-lg">
              {([
                ['template', 'Clone template'],
                ['iso', 'Boot from ISO'],
                ['empty', 'Empty disk'],
              ] as Array<[VMSource, string]>).map(([value, label]) => (
                <button
                  key={value}
                  onClick={() => set('source', value)}
                  className={`flex-1 px-2 py-1.5 text-[11px] rounded-md transition-colors ${
                    form.source === value
                      ? 'bg-[var(--surface)] text-[var(--text-primary)] font-medium shadow-sm'
                      : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          {form.source === 'template' && (
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Template VM</label>
              {templates.length === 0 ? (
                <p className="text-[11px] text-[var(--text-tertiary)]">No VM templates found. Mark a stopped VM as template in Proxmox, or use another source.</p>
              ) : (
                <select value={form.template} onChange={e => set('template', parseInt(e.target.value, 10))} className={inputClass}>
                  {templates.map(t => (
                    <option key={`${t.node}-${t.vmid}`} value={t.vmid}>{t.vmid} — {t.name || 'unnamed'} ({t.node})</option>
                  ))}
                </select>
              )}
            </div>
          )}

          {form.source === 'iso' && (
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">ISO Image</label>
              {isos.length === 0 ? (
                <p className="text-[11px] text-[var(--text-tertiary)]">No ISO images found on storages with ISO content. Upload one in Proxmox first.</p>
              ) : (
                <select value={form.iso} onChange={e => set('iso', e.target.value)} className={inputClass}>
                  <option value="">Select ISO...</option>
                  {isos.map(iso => <option key={iso} value={iso}>{iso}</option>)}
                </select>
              )}
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Name</label>
              <input value={form.name} onChange={e => set('name', e.target.value)} placeholder="my-vm" className={inputClass} />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">VMID</label>
              <input value={form.vmid} onChange={e => set('vmid', e.target.value.replace(/\D/g, ''))} placeholder="auto" className={inputClass} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Node</label>
              <select value={form.node} onChange={e => set('node', e.target.value)} className={inputClass}>
                <option value="">Select node...</option>
                {nodes.filter(n => n.status === 'online').map(n => (
                  <option key={n.node} value={n.node}>{n.node}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Network bridge</label>
              <input value={form.network} onChange={e => set('network', e.target.value)} placeholder="vmbr0" className={inputClass} />
            </div>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Cores</label>
              <input type="number" min={1} value={form.cores} onChange={e => set('cores', parseInt(e.target.value, 10) || 1)} className={inputClass} />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Memory (MB)</label>
              <input type="number" min={256} step={256} value={form.memory_mb} onChange={e => set('memory_mb', parseInt(e.target.value, 10) || 512)} className={inputClass} />
            </div>
            {form.source !== 'template' && (
              <div>
                <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk size</label>
                <input value={form.disk_size} onChange={e => set('disk_size', e.target.value)} placeholder="32G" className={inputClass} />
              </div>
            )}
          </div>

          {form.source !== 'template' && (
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk storage</label>
              <select value={form.storage} onChange={e => set('storage', e.target.value)} className={inputClass}>
                <option value="">Select storage...</option>
                {storages.map(s => <option key={s.storage} value={s.storage}>{s.storage} ({s.type})</option>)}
              </select>
            </div>
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
