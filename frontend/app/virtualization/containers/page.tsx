'use client';

import { useState, useEffect } from 'react';
import {
  virtualization,
  type ProxmoxVM,
  type ProxmoxNode,
  type ProxmoxStorage,
  type DeployDockerResult,
} from '@/lib/api';
import Link from 'next/link';
import ConfirmModal from '@/components/ConfirmModal';
import { useEscapeKey } from '@/hooks/useEscapeKey';

export default function ContainersPage() {
  const [containers, setContainers] = useState<ProxmoxVM[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [proxmoxUrl, setProxmoxUrl] = useState('');

  // Deploy modal state
  const [showDeploy, setShowDeploy] = useState(false);
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [storages, setStorages] = useState<ProxmoxStorage[]>([]);
  const [osTemplates, setOsTemplates] = useState<string[]>([]);
  const [catalogTemplates, setCatalogTemplates] = useState<Array<{ template: string; os?: string }>>([]);
  const [deployMode, setDeployMode] = useState<'os' | 'docker'>('docker');
  const [deployResult, setDeployResult] = useState<DeployDockerResult | null>(null);
  const [form, setForm] = useState({
    hostname: '',
    node: '',
    vmid: '',
    template: '',
    password: '',
    ssh_keys: '',
    cores: 1,
    memory_mb: 1024,
    disk_size: '8G',
    storage: '',
    network: 'vmbr0',
    start: true,
    // Docker workload fields
    source_type: 'registry' as 'registry' | 'folder' | 'docker_local',
    image: '',
    folder_path: '',
    container_name: '',
    ports: '',
    env: '',
  });
  const [deploying, setDeploying] = useState(false);

  // Delete confirm state
  const [deleteTarget, setDeleteTarget] = useState<ProxmoxVM | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    loadContainers();
    virtualization.proxmox.getConnectionInfo()
      .then(res => setProxmoxUrl(res.data?.url || ''))
      .catch(() => {});
  }, []);

  const loadContainers = async () => {
    setLoading(true);
    try {
      const res = await virtualization.proxmox.listContainers();
      setContainers(Array.isArray(res.data) ? res.data : []);
    } catch {
      setError('Failed to load containers');
    }
    setLoading(false);
  };

  const openDeployModal = async () => {
    setShowDeploy(true);
    setError('');
    try {
      const [nodesRes, storageRes, nextIdRes] = await Promise.all([
        virtualization.proxmox.listNodes(),
        virtualization.proxmox.listStorage(),
        virtualization.proxmox.nextId().catch(() => null),
      ]);
      const nodeList = Array.isArray(nodesRes.data) ? nodesRes.data : [];
      setNodes(nodeList);
      const storageList = Array.isArray(storageRes.data) ? storageRes.data : [];
      setStorages(storageList);
      const firstNode = nodeList.find(n => n.status === 'online')?.node || '';
      const diskStorage = storageList.find(s => (s.content || '').includes('rootdir'))?.storage || storageList[0]?.storage || '';
      setForm(f => ({
        ...f,
        node: f.node || firstNode,
        storage: f.storage || diskStorage,
        vmid: f.vmid || (nextIdRes?.data?.vmid ? String(nextIdRes.data.vmid) : ''),
      }));
      await loadTemplates(firstNode, storageList);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load deployment options');
    }
  };

  const loadTemplates = async (node: string, storageList: ProxmoxStorage[]) => {
    // Downloaded templates from storages with vztmpl content
    const tmplStorages = storageList.filter(s => (s.content || '').includes('vztmpl'));
    const tmplResults = await Promise.all(
      tmplStorages.map(s =>
        virtualization.proxmox.listStorageContent(s.storage, 'vztmpl')
          .then(res => (Array.isArray(res.data) ? res.data.map(c => c.volid) : []))
          .catch(() => [] as string[])
      )
    );
    const downloaded = tmplResults.flat();
    setOsTemplates(downloaded);
    setForm(f => f.template && downloaded.includes(f.template) ? f : { ...f, template: downloaded[0] || '' });

    // Downloadable catalog (aplinfo) — informational
    if (node) {
      virtualization.proxmox.listOSTemplates(node)
        .then(res => setCatalogTemplates(Array.isArray(res.data) ? res.data : []))
        .catch(() => setCatalogTemplates([]));
    }
  };

  const handleNodeChange = (node: string) => {
    setForm(f => ({ ...f, node }));
    virtualization.proxmox.listOSTemplates(node)
      .then(res => setCatalogTemplates(Array.isArray(res.data) ? res.data : []))
      .catch(() => setCatalogTemplates([]));
  };

  const handleDeploy = async () => {
    if (!form.hostname.trim()) { setError('Hostname is required'); return; }
    if (!form.node) { setError('Select a node'); return; }
    if (!form.template) { setError('Select an OS template'); return; }

    setDeploying(true);
    setError('');
    try {
      if (deployMode === 'docker') {
        if (form.source_type !== 'folder' && !form.image.trim()) {
          setError('Image name is required');
          setDeploying(false);
          return;
        }
        if (form.source_type === 'folder' && !form.folder_path.trim()) {
          setError('Folder path is required');
          setDeploying(false);
          return;
        }
        const ports = form.ports.split('\n').map(l => l.trim()).filter(Boolean);
        const env: Record<string, string> = {};
        for (const line of form.env.split('\n')) {
          const idx = line.indexOf('=');
          if (idx > 0) env[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
        }
        const res = await virtualization.proxmox.deployDocker({
          hostname: form.hostname.trim(),
          node: form.node,
          vmid: form.vmid ? parseInt(form.vmid, 10) : undefined,
          template: form.template,
          cores: form.cores,
          memory_mb: form.memory_mb,
          disk_size: form.disk_size,
          storage: form.storage,
          network: form.network,
          source_type: form.source_type,
          image: form.image.trim() || undefined,
          folder_path: form.folder_path.trim() || undefined,
          container_name: form.container_name.trim() || undefined,
          ports: ports.length > 0 ? ports : undefined,
          env: Object.keys(env).length > 0 ? env : undefined,
        });
        setDeployResult(res.data || null);
        setShowDeploy(false);
        setForm(f => ({ ...f, hostname: '', image: '', folder_path: '', container_name: '' }));
        setTimeout(loadContainers, 1500);
      } else {
        if (!form.password && !form.ssh_keys.trim()) {
          setError('Provide a root password or an SSH public key');
          setDeploying(false);
          return;
        }
        await virtualization.proxmox.createContainer({
          hostname: form.hostname.trim(),
          node: form.node,
          vmid: form.vmid ? parseInt(form.vmid, 10) : undefined,
          template: form.template,
          password: form.password || undefined,
          ssh_keys: form.ssh_keys.trim() || undefined,
          cores: form.cores,
          memory_mb: form.memory_mb,
          disk_size: form.disk_size,
          storage: form.storage,
          network: form.network,
          start: form.start,
        });
        setShowDeploy(false);
        setForm(f => ({ ...f, hostname: '', password: '', ssh_keys: '' }));
        setTimeout(loadContainers, 1500);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to deploy container');
    }
    setDeploying(false);
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await virtualization.proxmox.deleteContainer(deleteTarget.node, deleteTarget.vmid);
      setDeleteTarget(null);
      setTimeout(loadContainers, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete container');
      setDeleteTarget(null);
    }
    setDeleting(false);
  };

  const handleAction = async (ct: ProxmoxVM, action: 'start' | 'stop') => {
    setActionLoading(`${ct.node}-${ct.vmid}-${action}`);
    try {
      await virtualization.proxmox.containerAction(ct.node, ct.vmid, action);
      setTimeout(loadContainers, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action failed');
    }
    setActionLoading(null);
  };

  const statusColor = (s: string) => {
    switch (s) {
      case 'running': return 'bg-emerald-500/10 text-emerald-600';
      case 'stopped': return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
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

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">LXC Containers</h1>
            <p className="page-subtitle-modern">Deploy and manage Linux containers on Proxmox nodes</p>
          </div>
          <div className="flex items-center gap-2">
            <Link href="/virtualization" className="btn btn-secondary">Back to Dashboard</Link>
            <button onClick={openDeployModal} className="btn btn-primary">+ Deploy Container</button>
          </div>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-xl p-3 text-[12px] text-red-500 flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError('')} className="text-xs">Dismiss</button>
          </div>
        )}

        {deployResult && (
          <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-xl p-4 text-[12px] text-emerald-700">
            <div className="flex items-center justify-between mb-1">
              <p className="font-medium">
                Docker workload deployed — container {deployResult.vmid} ({deployResult.container_name}) at {deployResult.ip}
              </p>
              <button onClick={() => setDeployResult(null)} className="text-xs hover:underline">Dismiss</button>
            </div>
            {deployResult.log && (
              <pre className="text-[10px] font-mono text-emerald-800/80 whitespace-pre-wrap">{deployResult.log}</pre>
            )}
          </div>
        )}

        {loading ? (
          <div className="card card-body text-center py-16">
            <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
              <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <span className="text-[13px]">Loading containers...</span>
            </div>
          </div>
        ) : containers.length === 0 ? (
          <div className="card card-body text-center py-16">
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">No containers found</p>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              Deploy your first LXC container with the button above
            </p>
          </div>
        ) : (
          <div className="card overflow-hidden">
            <table className="w-full text-[12px]">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">VMID</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Node</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Status</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">CPU</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Memory</th>
                  <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Disk</th>
                  <th className="text-right px-4 py-3 font-medium text-[var(--text-tertiary)]">Actions</th>
                </tr>
              </thead>
              <tbody>
                {containers.map(ct => (
                  <tr key={`${ct.node}-${ct.vmid}`} className="border-b border-[var(--border-light)] hover:bg-[var(--border-light)]/30 transition-colors">
                    <td className="px-4 py-3 font-mono text-[var(--text-secondary)]">{ct.vmid}</td>
                    <td className="px-4 py-3 font-medium text-[var(--text-primary)]">
                      {ct.name || `CT ${ct.vmid}`}
                      {proxmoxUrl && (
                        <a
                          href={`${proxmoxUrl}/#v1:0:1:=lxc=${ct.node}=${ct.vmid}`}
                          target="_blank"
                          rel="noreferrer"
                          className="block text-[10px] text-[var(--accent)] hover:underline font-normal"
                        >
                          Open in Proxmox
                        </a>
                      )}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{ct.node}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(ct.status)}`}>
                        {ct.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">{ct.status === 'running' ? `${Math.round(ct.cpu * 100)}%` : '-'}</td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {ct.status === 'running' ? formatBytes(ct.mem) : '-'} / {formatBytes(ct.maxmem)}
                    </td>
                    <td className="px-4 py-3 text-[var(--text-secondary)]">
                      {formatBytes(ct.disk)} / {formatBytes(ct.maxdisk)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1">
                        {ct.status === 'stopped' && (
                          <>
                            <button
                              onClick={() => handleAction(ct, 'start')}
                              disabled={actionLoading === `${ct.node}-${ct.vmid}-start`}
                              className="px-2 py-1 text-[11px] text-emerald-600 hover:bg-emerald-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Start
                            </button>
                            <button
                              onClick={() => setDeleteTarget(ct)}
                              disabled={!!actionLoading}
                              className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50"
                            >
                              Delete
                            </button>
                          </>
                        )}
                        {ct.status === 'running' && (
                          <button
                            onClick={() => handleAction(ct, 'stop')}
                            disabled={!!actionLoading}
                            className="px-2 py-1 text-[11px] text-red-500 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50"
                          >
                            Stop
                          </button>
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

      {showDeploy && (
        <DeployContainerModal
          form={form}
          setForm={setForm}
          deployMode={deployMode}
          setDeployMode={setDeployMode}
          nodes={nodes}
          storages={storages}
          osTemplates={osTemplates}
          catalogTemplates={catalogTemplates}
          deploying={deploying}
          onNodeChange={handleNodeChange}
          onDeploy={handleDeploy}
          onClose={() => setShowDeploy(false)}
        />
      )}

      <ConfirmModal
        open={!!deleteTarget}
        title={`Delete container ${deleteTarget?.vmid || ''}?`}
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

interface DeployContainerModalProps {
  form: {
    hostname: string;
    node: string;
    vmid: string;
    template: string;
    password: string;
    ssh_keys: string;
    cores: number;
    memory_mb: number;
    disk_size: string;
    storage: string;
    network: string;
    start: boolean;
    source_type: 'registry' | 'folder' | 'docker_local';
    image: string;
    folder_path: string;
    container_name: string;
    ports: string;
    env: string;
  };
  setForm: React.Dispatch<React.SetStateAction<DeployContainerModalProps['form']>>;
  deployMode: 'os' | 'docker';
  setDeployMode: (m: 'os' | 'docker') => void;
  nodes: ProxmoxNode[];
  storages: ProxmoxStorage[];
  osTemplates: string[];
  catalogTemplates: Array<{ template: string; os?: string }>;
  deploying: boolean;
  onNodeChange: (node: string) => void;
  onDeploy: () => void;
  onClose: () => void;
}

function DeployContainerModal({ form, setForm, deployMode, setDeployMode, nodes, storages, osTemplates, catalogTemplates, deploying, onNodeChange, onDeploy, onClose }: DeployContainerModalProps) {
  useEscapeKey(() => { if (!deploying) onClose(); });

  const set = <K extends keyof DeployContainerModalProps['form']>(key: K, value: DeployContainerModalProps['form'][K]) =>
    setForm(f => ({ ...f, [key]: value }));

  const inputClass = 'w-full px-3 py-2 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/20';

  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => !deploying && onClose()} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-lg mx-4 overflow-hidden glass-modal max-h-[90vh] flex flex-col">
        <div className="px-5 py-4 border-b border-[#f0f0f0] flex items-center justify-between">
          <h3 className="text-[15px] font-semibold text-[#171717]">Deploy Container</h3>
          <button onClick={() => !deploying && onClose()} className="text-[#999] hover:text-[#333] text-lg leading-none">&times;</button>
        </div>

        <div className="p-5 space-y-4 overflow-y-auto">
          {/* Workload mode */}
          <div className="flex gap-1 p-1 bg-[var(--border-light)]/50 rounded-lg">
            {([
              ['docker', 'Docker workload'],
              ['os', 'Plain OS (LXC)'],
            ] as Array<['os' | 'docker', string]>).map(([value, label]) => (
              <button
                key={value}
                onClick={() => setDeployMode(value)}
                className={`flex-1 px-2 py-1.5 text-[11px] rounded-md transition-colors ${
                  deployMode === value
                    ? 'bg-[var(--surface)] text-[var(--text-primary)] font-medium shadow-sm'
                    : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                }`}
              >
                {label}
              </button>
            ))}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Hostname</label>
              <input value={form.hostname} onChange={e => set('hostname', e.target.value)} placeholder="my-app" className={inputClass} />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">VMID</label>
              <input value={form.vmid} onChange={e => set('vmid', e.target.value.replace(/\D/g, ''))} placeholder="auto" className={inputClass} />
            </div>
          </div>

          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Node</label>
            <select value={form.node} onChange={e => onNodeChange(e.target.value)} className={inputClass}>
              <option value="">Select node...</option>
              {nodes.filter(n => n.status === 'online').map(n => (
                <option key={n.node} value={n.node}>{n.node}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">OS Template</label>
            {osTemplates.length === 0 ? (
              <div className="text-[11px] text-[var(--text-tertiary)] space-y-1">
                <p>No OS templates downloaded. In Proxmox: node → Local (pve) → CT Templates → Templates → download e.g. debian-12 or ubuntu-24.04. {catalogTemplates.length > 0 && `Available in catalog: ${catalogTemplates.slice(0, 5).map(t => t.template).join(', ')}...`}</p>
              </div>
            ) : (
              <select value={form.template} onChange={e => set('template', e.target.value)} className={inputClass}>
                <option value="">Select template...</option>
                {osTemplates.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            )}
            {deployMode === 'docker' && (
              <p className="text-[10px] text-[var(--text-tertiary)] mt-1">The OS template becomes the Docker host inside LXC (nesting enabled automatically).</p>
            )}
          </div>

          {deployMode === 'docker' ? (
            <>
              {/* Docker source */}
              <div>
                <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Image source</label>
                <div className="flex gap-1 p-1 bg-[var(--border-light)]/50 rounded-lg">
                  {([
                    ['registry', 'Registry'],
                    ['folder', 'Local folder'],
                    ['docker_local', 'Local Docker'],
                  ] as Array<[DeployContainerModalProps['form']['source_type'], string]>).map(([value, label]) => (
                    <button
                      key={value}
                      onClick={() => set('source_type', value)}
                      className={`flex-1 px-2 py-1.5 text-[11px] rounded-md transition-colors ${
                        form.source_type === value
                          ? 'bg-[var(--surface)] text-[var(--text-primary)] font-medium shadow-sm'
                          : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>

              {form.source_type === 'registry' && (
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Image</label>
                  <input value={form.image} onChange={e => set('image', e.target.value)} placeholder="nginx:latest or registry.example.com/app:v1" className={inputClass} />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Pulled from the registry inside the container.</p>
                </div>
              )}

              {form.source_type === 'folder' && (
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Folder path on the PEPA server</label>
                  <input value={form.folder_path} onChange={e => set('folder_path', e.target.value)} placeholder="/data/compose-projects/my-app" className={`${inputClass} font-mono`} />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Folder with docker-compose.yml (runs `docker compose up -d`) or a Dockerfile (builds and runs). The folder must be accessible from the PEPA api-server container.</p>
                </div>
              )}

              {form.source_type === 'docker_local' && (
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Local Docker image</label>
                  <input value={form.image} onChange={e => set('image', e.target.value)} placeholder="my-app:latest" className={inputClass} />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Image from the Docker daemon on the PEPA server is transferred into the container (requires docker CLI + daemon access in the api-server).</p>
                </div>
              )}

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Container name</label>
                  <input value={form.container_name} onChange={e => set('container_name', e.target.value)} placeholder={form.hostname || 'defaults to hostname'} className={inputClass} />
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Ports (host:container)</label>
                  <input value={form.ports} onChange={e => set('ports', e.target.value)} placeholder="8080:80" className={inputClass} />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">One mapping per line for multiple.</p>
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Environment variables (KEY=VALUE)</label>
                <textarea value={form.env} onChange={e => set('env', e.target.value)} rows={2} placeholder={'DATABASE_URL=...\nLOG_LEVEL=info'} className={`${inputClass} font-mono`} />
              </div>
            </>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Root password</label>
                  <input type="password" value={form.password} onChange={e => set('password', e.target.value)} placeholder="optional" className={inputClass} autoComplete="new-password" />
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Network bridge</label>
                  <input value={form.network} onChange={e => set('network', e.target.value)} placeholder="vmbr0" className={inputClass} />
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">SSH public key (optional)</label>
                <textarea
                  value={form.ssh_keys}
                  onChange={e => set('ssh_keys', e.target.value)}
                  placeholder="ssh-ed25519 AAAA... user@host"
                  rows={2}
                  className={`${inputClass} font-mono`}
                />
              </div>
            </>
          )}

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Cores</label>
              <input type="number" min={1} value={form.cores} onChange={e => set('cores', parseInt(e.target.value, 10) || 1)} className={inputClass} />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Memory (MB)</label>
              <input type="number" min={128} step={128} value={form.memory_mb} onChange={e => set('memory_mb', parseInt(e.target.value, 10) || 512)} className={inputClass} />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk size</label>
              <input value={form.disk_size} onChange={e => set('disk_size', e.target.value)} placeholder="8G" className={inputClass} />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Disk storage</label>
              <select value={form.storage} onChange={e => set('storage', e.target.value)} className={inputClass}>
                <option value="">Select storage...</option>
                {storages.map(s => <option key={s.storage} value={s.storage}>{s.storage} ({s.type})</option>)}
              </select>
            </div>
            {deployMode === 'docker' && (
              <div>
                <label className="block text-[11px] font-medium text-[var(--text-secondary)] mb-1.5">Network bridge</label>
                <input value={form.network} onChange={e => set('network', e.target.value)} placeholder="vmbr0" className={inputClass} />
              </div>
            )}
          </div>

          {deployMode === 'os' && (
            <label className="flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
              <input type="checkbox" checked={form.start} onChange={e => set('start', e.target.checked)} className="rounded" />
              Start container after creation
            </label>
          )}

          {deployMode === 'docker' && (
            <p className="text-[10px] text-[var(--text-tertiary)]">
              PEPA creates the LXC (nesting enabled), injects the SSH key from the Proxmox connection settings, installs Docker inside, and deploys the workload. First deploy takes a few minutes.
            </p>
          )}
        </div>

        <div className="flex items-center gap-2 px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onClose} disabled={deploying} className="btn btn-secondary flex-1 justify-center">Cancel</button>
          <button
            onClick={onDeploy}
            disabled={deploying || !form.hostname.trim() || !form.node || !form.template}
            className="btn btn-primary flex-1 justify-center"
          >
            {deploying
              ? (deployMode === 'docker' ? 'Deploying... (may take a few minutes)' : 'Deploying...')
              : (deployMode === 'docker' ? 'Deploy Docker Workload' : 'Deploy Container')}
          </button>
        </div>
      </div>
    </div>
  );
}
