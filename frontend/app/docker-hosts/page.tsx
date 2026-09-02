'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { dockerHosts, type DockerHost, type DockerHostTestResult } from '@/lib/api';
import { VaultInput, VaultPickerModal, useVaultPicker } from '@/components/VaultInput';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';
import ConfirmModal from '@/components/ConfirmModal';

const defaultForm = {
  name: '', description: '', host_type: 'local' as DockerHost['host_type'],
  host_address: 'unix:///var/run/docker.sock',
  tls_ca_cert: '', tls_cert: '', tls_key: '', ssh_key: '',
};

export default function DockerHostsPage() {
  const { isAdmin, hasPermission, loading: permLoading } = usePermission();
  const [hosts, setHosts] = useState<DockerHost[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<DockerHost | null>(null);

  useEscapeKey(() => {
    if (showForm) { setShowForm(false); setEditing(null); }
  }, showForm);
  const [form, setForm] = useState({ ...defaultForm });
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<Record<string, DockerHostTestResult>>({});
  const [error, setError] = useState('');
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();

  const load = async () => {
    try {
      const res = await dockerHosts.list();
      setHosts(res.docker_hosts || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  useEffect(() => { if (isAdmin) load(); }, [isAdmin]);

  const openCreate = () => {
    setEditing(null);
    setForm({ ...defaultForm });
    setError('');
    setShowForm(true);
  };

  const openEdit = (h: DockerHost) => {
    setEditing(h);
    setForm({
      name: h.name, description: h.description, host_type: h.host_type,
      host_address: h.host_address,
      tls_ca_cert: '', tls_cert: '', tls_key: '', ssh_key: '',
    });
    setError('');
    setShowForm(true);
  };

  const handleSave = async () => {
    setError('');
    try {
      const merged = { ...form };
      for (const [field, ref] of Object.entries(vaultRefs)) {
        if (ref) (merged as Record<string, unknown>)[field] = ref;
      }
      if (editing) {
        await dockerHosts.update(editing.id, merged);
      } else {
        await dockerHosts.create(merged);
      }
      setShowForm(false);
      setVaultRefs({});
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const handleDelete = async (id: string) => {
    setDeleteConfirm(id);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    setDeleting(true);
    try {
      await dockerHosts.delete(deleteConfirm);
      load();
    } catch { /* ignore */ }
    setDeleting(false);
    setDeleteConfirm(null);
  };

  const handleTest = async (id: string) => {
    setTesting(id);
    try {
      const result = await dockerHosts.test(id);
      setTestResult(prev => ({ ...prev, [id]: result }));
      load();
    } catch (err) {
      setTestResult(prev => ({ ...prev, [id]: { status: 'error', error: err instanceof Error ? err.message : 'Test failed' } }));
    }
    setTesting(null);
  };

  const hostTypeLabel = (t: string) => {
    switch (t) {
      case 'local': return '🖥 Local';
      case 'tcp': return '🌐 TCP';
      case 'ssh': return '🔒 SSH';
      default: return t;
    }
  };

  const statusColor = (s: string) => {
    switch (s) {
      case 'connected': return 'bg-emerald-500/15 text-emerald-600';
      case 'error': return 'bg-red-500/15 text-red-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-secondary)]';
    }
  };

  if (permLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('docker', 'read')) {
    return <ForbiddenPage resource="docker_hosts" />;
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Docker Hosts</h1>
          <p className="page-subtitle-modern">Manage Docker engines for Compose deployments — local or remote</p>
        </div>
        <button onClick={openCreate} className="btn btn-primary">+ Add Docker Host</button>
      </div>

      {/* Grid */}
      {loading ? (
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p>
        </div>
      ) : hosts.length === 0 ? (
        <div className="card card-body text-center py-16">
          <div className="text-5xl mb-4 opacity-20">🐳</div>
          <p className="text-[14px] text-[var(--text-secondary)] mb-1">No Docker hosts registered</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
            Add a Docker host to deploy services via Docker Compose
          </p>
          <button onClick={openCreate} className="btn btn-primary">+ Add First Host</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {hosts.map(h => {
            const tr = testResult[h.id];
            return (
              <div key={h.id} className="card p-5 modern-card-hover group" style={{ borderRadius: '12px' }}>
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <span className="text-xl">🐳</span>
                    <div>
                      <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{h.name}</h3>
                      <span className="text-[10px] text-[var(--text-tertiary)]">{hostTypeLabel(h.host_type)}</span>
                    </div>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(h.status)}`}>
                    {h.status}
                  </span>
                </div>

                {h.description && (
                  <p className="text-[12px] text-[var(--text-secondary)] mb-3 line-clamp-2">{h.description}</p>
                )}

                <div className="space-y-1.5 mb-4">
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-14">Address:</span>
                    <span className="font-mono text-[var(--text-secondary)] truncate">{h.host_address}</span>
                  </div>
                  {h.docker_version && (
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-[var(--text-tertiary)] w-14">Version:</span>
                      <span className="text-[var(--text-secondary)]">{h.docker_version}</span>
                    </div>
                  )}
                  {h.os_arch && (
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-[var(--text-tertiary)] w-14">OS/Arch:</span>
                      <span className="text-[var(--text-secondary)]">{h.os_arch}</span>
                    </div>
                  )}
                  <div className="flex items-center gap-2 text-[11px]">
                    <span className="text-[var(--text-tertiary)] w-14">Containers:</span>
                    <span className="text-[var(--text-secondary)]">{h.containers_running} running</span>
                  </div>
                </div>

                {/* Test result */}
                {tr && (
                  <div className={`mb-3 p-2 rounded-lg text-[11px] ${tr.status === 'connected' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>
                    {tr.status === 'connected' ? (
                      <>Connected — Docker {tr.docker_version}, {tr.containers_running} containers running</>
                    ) : (
                      <>Failed: {tr.error}</>
                    )}
                  </div>
                )}

                <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                  <button
                    onClick={() => handleTest(h.id)}
                    disabled={testing === h.id}
                    className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50"
                  >
                    {testing === h.id ? 'Testing...' : 'Test Connection'}
                  </button>
                  <button onClick={() => openEdit(h)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded-lg transition-colors">Edit</button>
                  <button onClick={() => handleDelete(h.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors ml-auto">Delete</button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Create/Edit Modal */}
      {showForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
          <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
              <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                {editing ? 'Edit Docker Host' : 'Add Docker Host'}
              </h2>
              <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
            </div>

            <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
              )}

              <div>
                <label className="label">Name *</label>
                <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="my-docker-host" />
              </div>

              <div>
                <label className="label">Description</label>
                <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="Production Docker server" />
              </div>

              <div>
                <label className="label">Connection Type</label>
                <div className="grid grid-cols-3 gap-2">
                  {([
                    { value: 'local', label: '🖥 Local', desc: 'Unix socket' },
                    { value: 'tcp', label: '🌐 TCP', desc: 'Remote TCP+TLS' },
                    { value: 'ssh', label: '🔒 SSH', desc: 'SSH tunnel' },
                  ] as const).map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => {
                        const addr = opt.value === 'local' ? 'unix:///var/run/docker.sock' : opt.value === 'tcp' ? 'tcp://' : 'ssh://';
                        setForm({ ...form, host_type: opt.value, host_address: addr });
                      }}
                      className={`p-2.5 rounded-lg border text-left transition-all ${
                        form.host_type === opt.value
                          ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                          : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                      }`}
                    >
                      <div className="text-[12px] font-medium">{opt.label}</div>
                      <div className="text-[10px] text-[var(--text-tertiary)]">{opt.desc}</div>
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="label">Host Address</label>
                <input
                  value={form.host_address}
                  onChange={e => setForm({ ...form, host_address: e.target.value })}
                  className="input font-mono text-[12px]"
                  placeholder={
                    form.host_type === 'local' ? 'unix:///var/run/docker.sock' :
                    form.host_type === 'tcp' ? 'tcp://192.168.1.100:2376' :
                    'ssh://user@192.168.1.100'
                  }
                />
              </div>

              {/* TLS fields for TCP */}
              {form.host_type === 'tcp' && (
                <div className="space-y-3">
                  <p className="text-[11px] text-[var(--text-tertiary)]">TLS certificates (optional, for secured TCP connections)</p>
                  <VaultInput
                    label="CA Certificate"
                    field="tls_ca_cert"
                    value={form.tls_ca_cert}
                    onChange={v => setForm({ ...form, tls_ca_cert: v })}
                    vaultRef={vaultRefs.tls_ca_cert}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder="-----BEGIN CERTIFICATE-----"
                    isTextarea
                  />
                  <div className="grid grid-cols-2 gap-3">
                    <VaultInput
                      label="Client Certificate"
                      field="tls_cert"
                      value={form.tls_cert}
                      onChange={v => setForm({ ...form, tls_cert: v })}
                      vaultRef={vaultRefs.tls_cert}
                      onOpenVault={onOpenVaultPicker}
                      onRemoveVault={removeVaultRef}
                      placeholder="-----BEGIN CERTIFICATE-----"
                      isTextarea
                    />
                    <VaultInput
                      label="Client Key"
                      field="tls_key"
                      value={form.tls_key}
                      onChange={v => setForm({ ...form, tls_key: v })}
                      vaultRef={vaultRefs.tls_key}
                      onOpenVault={onOpenVaultPicker}
                      onRemoveVault={removeVaultRef}
                      placeholder="-----BEGIN RSA PRIVATE KEY-----"
                      isTextarea
                    />
                  </div>
                </div>
              )}

              {/* SSH key for SSH */}
              {form.host_type === 'ssh' && (
                <div>
                  <VaultInput
                    label="SSH private key (optional, for key-based authentication)"
                    field="ssh_key"
                    value={form.ssh_key}
                    onChange={v => setForm({ ...form, ssh_key: v })}
                    vaultRef={vaultRefs.ssh_key}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                    isTextarea
                  />
                </div>
              )}
            </div>

            <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
              <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
              <button
                onClick={handleSave}
                disabled={!form.name.trim() || !form.host_address.trim()}
                className="btn btn-primary"
              >
                {editing ? 'Save Changes' : 'Add Host'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteConfirm}
        title="Delete this Docker host?"
        description="This host will be permanently removed. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm(null)}
      />

      {VaultPicker}
      </div>
    </div>
  );
}
