'use client';
import { useState, useEffect, useCallback, useRef } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { vault, type VaultPath, type VaultEngine, type VaultConfig, type VaultStatus, type VaultACLEntry } from '@/lib/api';
import { listUsers, listTeams, getMe, type User, type Team } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import { usePermission } from '@/hooks/usePermission';
import GearIcon from '@/components/GearIcon';

interface Props {
  initialPaths?: VaultPath[];
  initialEngines?: VaultEngine[];
}

export default function VaultClient({ initialPaths, initialEngines }: Props) {
  return (
    <PermissionGuard resource="vault" action="read">
      <VaultClientContent initialPaths={initialPaths} initialEngines={initialEngines} />
    </PermissionGuard>
  );
}

function VaultClientContent({ initialPaths, initialEngines }: Props) {
  const { hasPermission } = usePermission();
  const canManageACL = hasPermission('vault', 'create');
  const [tab, setTab] = useState<'secrets' | 'access'>('secrets');
  const [paths, setPaths] = useState<VaultPath[]>(initialPaths ?? []);
  const [engines, setEngines] = useState<VaultEngine[]>(initialEngines ?? []);
  const [currentPrefix, setCurrentPrefix] = useState('');
  const [selectedSecret, setSelectedSecret] = useState<{ path: string; data: Record<string, string>; metadata: Record<string, unknown> } | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingPath, setEditingPath] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<{ status: string; message: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [vaultConfig, setVaultConfig] = useState<VaultConfig>({ mode: 'local' });
  const [showConfig, setShowConfig] = useState(false);
  const [mode, setMode] = useState<string>('local');
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set());
  const [vaultStatus, setVaultStatus] = useState<VaultStatus | null>(null);
  const [rotating, setRotating] = useState(false);
  const [aclEntries, setAclEntries] = useState<VaultACLEntry[]>([]);
  const [currentUserId, setCurrentUserId] = useState<string>('');
  const clipboardTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const revealTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // Cleanup timers on unmount
  useEffect(() => {
    return () => {
      if (clipboardTimerRef.current) clearTimeout(clipboardTimerRef.current);
      revealTimersRef.current.forEach(t => clearTimeout(t));
    };
  }, []);

  // Fetch initial data client-side when no SSR data is provided
  useEffect(() => {
    if (initialPaths === undefined) {
      vault.paths().then(res => setPaths(res.paths || [])).catch(() => {});
    }
    if (initialEngines === undefined) {
      vault.engines().then(res => setEngines(res.engines || [])).catch(() => {});
    }
  }, []);

  const toggleReveal = (key: string) => {
    setRevealedKeys(prev => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
        // Clear any existing auto-hide timer for this key
        const existing = revealTimersRef.current.get(key);
        if (existing) {
          clearTimeout(existing);
          revealTimersRef.current.delete(key);
        }
      } else {
        next.add(key);
        // Auto-hide after 60 seconds for security
        const timer = setTimeout(() => {
          setRevealedKeys(p => {
            const n = new Set(p);
            n.delete(key);
            return n;
          });
          revealTimersRef.current.delete(key);
        }, 60000);
        revealTimersRef.current.set(key, timer);
      }
      return next;
    });
  };

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  // Load vault config on mount
  useEffect(() => {
    vault.getConfig().then(res => {
      setVaultConfig(res.config);
      setMode(res.config.mode);
    }).catch(() => {});
    vault.getStatus().then(res => setVaultStatus(res.status)).catch(() => {});
    getMe().then(data => setCurrentUserId(data.user.id)).catch(() => {});
  }, []);

  const loadPaths = useCallback(async (prefix: string) => {
    setLoading(true);
    try {
      const res = await vault.paths(prefix || undefined);
      setPaths(res.paths || []);
    } catch {
      // ignore
    }
    setLoading(false);
  }, []);

  const loadSecret = useCallback(async (path: string) => {
    try {
      const res = await vault.getSecret(path);
      setSelectedSecret({ path, data: res.secret.data, metadata: res.secret.metadata });
    } catch (err) {
      showToast(`Failed to load secret: ${err}`, 'error');
    }
  }, [showToast]);

  const handleTestConnection = async () => {
    try {
      const res = await vault.testConnection(
        vaultConfig.address || '',
        vaultConfig.mode === 'remote' ? vaultConfig.token : undefined,
        vaultConfig.mount_path,
      );
      setConnectionStatus({ status: res.status, message: res.message });
      showToast(res.status === 'ok' ? 'Connection OK' : 'Connection failed', res.status === 'ok' ? 'success' : 'error');
    } catch (err) {
      setConnectionStatus({ status: 'error', message: String(err) });
      showToast('Connection failed', 'error');
    }
  };

  const handleSaveConfig = async (cfg: VaultConfig) => {
    try {
      await vault.saveConfig(cfg);
      setVaultConfig(cfg);
      setMode(cfg.mode);
      showToast('Vault configuration saved', 'success');
      setShowConfig(false);
      // Reload engines for new mode
      try {
        const res = await vault.engines();
        setEngines(res.engines || []);
      } catch { /* ignore */ }
      loadPaths('');
    } catch (err) {
      showToast(`Failed to save config: ${err}`, 'error');
    }
  };

  const handleDelete = async (path: string) => {
    if (!confirm(`Delete secret at "${path}"?`)) return;
    try {
      const res = await fetch(`/api/v1/vault/secrets/${path}`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
      });
      if (res.ok) {
        showToast('Secret deleted', 'success');
        setSelectedSecret(null);
        loadPaths(currentPrefix);
      } else {
        showToast('Failed to delete', 'error');
      }
    } catch (err) {
      showToast(`Delete failed: ${err}`, 'error');
    }
  };

  const handleSaveSecret = async (path: string, data: Record<string, string>): Promise<void> => {
    try {
      await vault.setSecret(path, data);
      showToast('Secret saved', 'success');
      setShowCreateModal(false);
      setEditingPath(null);
      loadPaths(currentPrefix);
      loadSecret(path);
    } catch (err) {
      showToast(`Save failed: ${err}`, 'error');
      throw err; // re-throw so modal can reset saving state
    }
  };

  const navigateTo = (prefix: string) => {
    setCurrentPrefix(prefix);
    setSelectedSecret(null);
    loadPaths(prefix);
  };

  const handleRotateKeys = async () => {
    if (!confirm('Re-encrypt all secrets with Argon2id per-path keys? This is safe and does not change secret values.')) return;
    setRotating(true);
    try {
      const res = await vault.rotateKeys();
      showToast(res.message, res.errors?.length ? 'error' : 'success');
      // Refresh status
      vault.getStatus().then(r => setVaultStatus(r.status)).catch(() => {});
    } catch (err) {
      showToast(`Rotation failed: ${err}`, 'error');
    }
    setRotating(false);
  };

  const loadACL = useCallback(async () => {
    try {
      const res = await vault.listACL();
      setAclEntries(res.entries || []);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { if (canManageACL) loadACL(); }, [loadACL, canManageACL]);

  const handleDeleteACL = async (id: string) => {
    try {
      await vault.deleteACL(id);
      showToast('ACL entry removed', 'success');
      loadACL();
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    }
  };

  // Breadcrumb parts
  const breadcrumbParts = currentPrefix ? currentPrefix.split('/').filter(Boolean) : [];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && (
        <div className={`fixed top-4 right-4 z-[60] px-4 py-2.5 rounded-lg text-[13px] shadow-lg ${
          toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
        }`}>
          {toast.message}
        </div>
      )}

      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Vault</h1>
            <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${
              mode === 'remote' ? 'bg-violet-500/10 text-violet-500 border border-violet-500/20'
              : 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20'
            }`}>
              {mode === 'remote' ? '🔗 HashiCorp Vault' : '🏠 Built-in KV'}
            </span>
          </div>
          <p className="page-subtitle-modern">
            {mode === 'remote'
              ? `Connected to ${vaultConfig.address} · mount: ${vaultConfig.mount_path || 'secret'} · ${engines.length} engine${engines.length !== 1 ? 's' : ''}`
              : `Built-in encrypted KV store · AES-256-GCM · ${engines.length} engine${engines.length !== 1 ? 's' : ''}`
            }
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => setShowConfig(!showConfig)} className="btn btn-secondary btn-sm">
            <GearIcon className="w-4 h-4" /> Configure
          </button>
          <button onClick={handleTestConnection} className="btn btn-secondary btn-sm">
            ◇ Test
          </button>
          {tab === 'secrets' && (
            <button onClick={() => { setEditingPath(null); setShowCreateModal(true); }} className="btn btn-primary btn-sm">
              + New Secret
            </button>
          )}
        </div>
      </div>

      {/* Tab navigation */}
      <div className="flex gap-1 border-b border-[var(--border)] page-animate-up page-delay-1">
        <button
          onClick={() => setTab('secrets')}
          className={`px-4 py-2.5 text-[13px] font-medium border-b-2 transition-colors ${
            tab === 'secrets' ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
          }`}
        >
          Secrets
          {paths.length > 0 && <span className="ml-1.5 px-1.5 py-0.5 text-[10px] bg-[var(--border-light)] rounded-full">{paths.length}</span>}
        </button>
        {canManageACL && (
          <button
            onClick={() => setTab('access')}
            className={`px-4 py-2.5 text-[13px] font-medium border-b-2 transition-colors ${
              tab === 'access' ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            }`}
          >
            Access Control
            {aclEntries.length > 0 && <span className="ml-1.5 px-1.5 py-0.5 text-[10px] bg-[var(--border-light)] rounded-full">{aclEntries.length}</span>}
          </button>
        )}
      </div>

      {/* Configuration panel */}
      {showConfig && (
        <div className="page-animate-up page-delay-1">
        <VaultConfigPanel
          config={vaultConfig}
          onSave={handleSaveConfig}
          onClose={() => setShowConfig(false)}
          onToast={showToast}
        />
        </div>
      )}

      {/* ── Secrets Tab ──────────────────────────────────────── */}
      {tab === 'secrets' && (
        <>
      {/* Connection status */}
      {connectionStatus && (
        <div className={`text-[12px] px-3 py-2 rounded-lg border ${
          connectionStatus.status === 'ok' ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600' : 'bg-red-500/10 border-red-500/20 text-red-500'
        }`}>
          <strong>{connectionStatus.status}</strong> — {connectionStatus.message}
        </div>
      )}

      {/* Engines info */}
      {engines.length > 0 && (
        <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="flex items-center gap-3 flex-wrap">
            {engines.map(e => (
              <div key={e.path} className="flex items-center gap-2 text-[12px]">
                <span className="px-2 py-0.5 rounded bg-[var(--bg)] border border-[var(--border)] text-[var(--text-secondary)]">
                  🔐 {e.path}/
                </span>
                <span className="text-[var(--text-tertiary)]">{e.description}</span>
                <span className="text-[var(--text-tertiary)]">v{e.version}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Security status (local mode) */}
      {mode === 'local' && vaultStatus && (
        <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="flex items-center justify-between flex-wrap gap-3">
            <div className="flex items-center gap-4 text-[12px] flex-wrap">
              <span className="flex items-center gap-1.5">
                <span className={`w-2 h-2 rounded-full ${vaultStatus.needs_rotation ? 'bg-amber-400' : 'bg-emerald-400'}`} />
                <span className="font-medium text-[var(--text-primary)]">{vaultStatus.encryption_type.toUpperCase()}</span>
              </span>
              <span className="text-[var(--text-tertiary)]">Key derivation: <strong className="text-[var(--text-secondary)]">{vaultStatus.key_derivation}</strong></span>
              <span className="text-[var(--text-tertiary)]">Per-path keys: <strong className={vaultStatus.per_path_keys ? 'text-emerald-600' : 'text-[var(--text-secondary)]'}>{vaultStatus.per_path_keys ? 'Yes' : 'No'}</strong></span>
              <span className="text-[var(--text-tertiary)]">Tenant isolation: <strong className={vaultStatus.tenant_isolation ? 'text-emerald-600' : 'text-amber-600'}>{vaultStatus.tenant_isolation ? 'Yes' : 'No'}</strong></span>
              <span className="text-[var(--text-tertiary)]">Audit trail: <strong className={vaultStatus.created_by_tracking ? 'text-emerald-600' : 'text-[var(--text-secondary)]'}>{vaultStatus.created_by_tracking ? 'Yes' : 'No'}</strong></span>
              <span className="text-[var(--text-tertiary)]">Secrets: <strong className="text-[var(--text-secondary)]">{vaultStatus.total_secrets}</strong></span>
              {vaultStatus.v1_secrets > 0 && (
                <span className="text-amber-600">{vaultStatus.v1_secrets} legacy (rotation recommended)</span>
              )}
              {vaultStatus.argon2_params && (
                <span className="text-[var(--text-tertiary)]" title={`Argon2id: ${vaultStatus.argon2_params.time} iterations, ${vaultStatus.argon2_params.memory}MB, ${vaultStatus.argon2_params.threads} threads`}>
                  Argon2: <strong className="text-[var(--text-secondary)]">t={vaultStatus.argon2_params.time}, m={vaultStatus.argon2_params.memory}MB</strong>
                </span>
              )}
            </div>
            <button
              onClick={handleRotateKeys}
              disabled={rotating}
              className="btn btn-secondary btn-sm disabled:opacity-50"
              title="Re-encrypt all secrets with current Argon2id per-path keys"
            >
              {rotating ? 'Rotating...' : '🔄 Rotate Keys'}
            </button>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 page-animate-up page-delay-2">
        {/* Left: Path browser */}
        <div className="lg:col-span-1 space-y-4">
          {/* Breadcrumb */}
          <div className="card card-body">
            <div className="flex items-center gap-1 text-[12px] flex-wrap">
              <button onClick={() => navigateTo('')} className="text-[var(--accent)] hover:underline">secret/</button>
              {breadcrumbParts.map((part, i) => (
                <span key={i} className="flex items-center gap-1">
                  <span className="text-[var(--text-tertiary)]">/</span>
                  <button
                    onClick={() => navigateTo(breadcrumbParts.slice(0, i + 1).join('/'))}
                    className="text-[var(--accent)] hover:underline"
                  >
                    {part}
                  </button>
                </span>
              ))}
            </div>
          </div>

          {/* Paths list */}
          <div className="card">
            <div className="card-header flex items-center justify-between">
              <span className="text-[13px] font-medium text-[var(--text-primary)]">Secrets</span>
              {loading && <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>}
            </div>
            <div className="card-body pt-0">
              {paths.length === 0 ? (
                <div className="text-center py-8">
                  <div className="text-3xl mb-2 opacity-20">🔐</div>
                  <p className="text-[13px] text-[var(--text-tertiary)]">No secrets yet</p>
                  <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Create your first secret to get started</p>
                </div>
              ) : (
                <div className="space-y-0.5">
                  {paths.map(p => (
                    <button
                      key={p.path}
                      onClick={() => {
                        if (p.has_children) {
                          navigateTo(p.path);
                        } else {
                          loadSecret(p.path);
                        }
                      }}
                      className={`w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-[12px] text-left transition-colors hover:bg-[var(--border-light)] ${
                        selectedSecret?.path === p.path ? 'bg-[var(--border-light)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'
                      }`}
                    >
                      <span>{p.has_children ? '📁' : '🔑'}</span>
                      <span className="truncate">{p.path.split('/').pop()}</span>
                      {p.has_children && <span className="ml-auto text-[10px] text-[var(--text-tertiary)]">→</span>}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Right: Secret detail */}
        <div className="lg:col-span-2">
          {selectedSecret ? (
            <div className="card">
              <div className="card-header flex items-center justify-between">
                <div>
                  <span className="text-[14px] font-medium text-[var(--text-primary)]">{selectedSecret.path}</span>
                  <div className="flex items-center gap-2 mt-0.5">
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      v{(selectedSecret.metadata as { version?: number })?.version || 1}
                    </span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      Updated: {new Date((selectedSecret.metadata as { updated_at?: string })?.updated_at || '').toLocaleString()}
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => { setEditingPath(selectedSecret.path); setShowCreateModal(true); }}
                    className="btn btn-secondary btn-sm"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => handleDelete(selectedSecret.path)}
                    className="btn btn-sm"
                    style={{ color: '#ef4444' }}
                  >
                    Delete
                  </button>
                </div>
              </div>
              <div className="card-body space-y-3">
                <div>
                  <span className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-2 block">Key-Value Pairs</span>
                  <div className="space-y-1.5">
                    {Object.entries(selectedSecret.data).map(([key, value]) => {
                      const isRevealed = revealedKeys.has(key);
                      return (
                        <div key={key} className="flex items-center gap-2 bg-[var(--bg)] rounded-lg p-2.5 border border-[var(--border)]">
                          <span className="text-[12px] font-mono font-medium text-[var(--text-primary)] min-w-[100px]">{key}</span>
                          <span className="text-[12px] font-mono text-[var(--text-secondary)] truncate flex-1">
                            {!isRevealed ? '••••••••' : value}
                          </span>
                          <button
                            onClick={() => toggleReveal(key)}
                            className="text-[11px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] shrink-0 p-0.5"
                            title={isRevealed ? 'Hide value' : 'Reveal value'}
                          >
                            {isRevealed ? (
                              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                              </svg>
                            ) : (
                              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                                <path strokeLinecap="round" strokeLinejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                              </svg>
                            )}
                          </button>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(value);
                              showToast('Copied to clipboard', 'success');
                              // Auto-clear clipboard after 30 seconds
                              if (clipboardTimerRef.current) clearTimeout(clipboardTimerRef.current);
                              clipboardTimerRef.current = setTimeout(() => {
                                navigator.clipboard.writeText('').catch(() => {});
                                showToast('Clipboard cleared for security', 'success');
                                clipboardTimerRef.current = null;
                              }, 30000);
                            }}
                            className="text-[11px] text-[var(--accent)] hover:underline shrink-0"
                          >
                            Copy
                          </button>
                          <button
                            onClick={() => {
                              showToast(`Use vault:${selectedSecret.path}/${key} in connections`, 'success');
                            }}
                            className="text-[11px] text-[var(--accent)] hover:underline shrink-0"
                          >
                            Reference
                          </button>
                        </div>
                      );
                    })}
                  </div>
                </div>

                {/* Usage hint */}
                <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-3 text-[12px] text-blue-500">
                  <strong>Usage:</strong> Reference these secrets in Connections or Pipeline configs using{' '}
                  <code className="bg-blue-500/15 px-1 py-0.5 rounded text-[11px]">vault:{selectedSecret.path}/&lt;key&gt;</code>
                </div>
              </div>
            </div>
          ) : (
            <div className="card card-body text-center py-16">
              <div className="text-4xl mb-3 opacity-20">🔐</div>
              <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">Select a secret</h3>
              <p className="text-[12px] text-[var(--text-tertiary)]">
                Choose a secret from the left panel to view its contents, or create a new one.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* Create/Edit Modal */}
      {showCreateModal && (
        <SecretModal
          initialPath={editingPath || ''}
          initialData={editingPath && selectedSecret?.path === editingPath ? selectedSecret.data : {}}
          onSave={handleSaveSecret}
          onClose={() => { setShowCreateModal(false); setEditingPath(null); }}
        />
      )}
        </>
      )}

      {/* ── Access Control Tab ───────────────────────────────── */}
      {tab === 'access' && (
        <div className="page-animate-up page-delay-2">
          <VaultACLPanel
            entries={aclEntries}
            currentUserId={currentUserId}
            onDelete={handleDeleteACL}
            onRefresh={loadACL}
            onToast={showToast}
          />
        </div>
      )}
      </div>
    </div>
  );
}

function SecretModal({ initialPath, initialData, onSave, onClose }: {
  initialPath: string;
  initialData: Record<string, string>;
  onSave: (path: string, data: Record<string, string>) => Promise<void>;
  onClose: () => void;
}) {
  useEscapeKey(onClose);
  const [path, setPath] = useState(initialPath);
  const [entries, setEntries] = useState<Array<{ key: string; value: string }>>(
    Object.keys(initialData).length > 0
      ? Object.entries(initialData).map(([key, value]) => ({ key, value }))
      : [{ key: '', value: '' }]
  );
  const [saving, setSaving] = useState(false);

  const addEntry = () => setEntries([...entries, { key: '', value: '' }]);
  const removeEntry = (idx: number) => setEntries(entries.filter((_, i) => i !== idx));
  const updateEntry = (idx: number, field: 'key' | 'value', val: string) => {
    const next = [...entries];
    next[idx] = { ...next[idx], [field]: val };
    setEntries(next);
  };

  const handleSave = async () => {
    if (!path.trim() || saving) return;
    const data: Record<string, string> = {};
    for (const e of entries) {
      if (e.key.trim()) data[e.key.trim()] = e.value;
    }
    setSaving(true);
    try {
      await onSave(path.trim(), data);
      onClose();
    } catch {
      // error toast already shown by parent
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div className="relative w-full max-w-[520px] bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] p-6" onClick={e => e.stopPropagation()}>
        <h2 className="text-[16px] font-semibold text-[var(--text-primary)] mb-4">
          {initialPath ? 'Edit Secret' : 'New Secret'}
        </h2>

        {/* Path */}
        <div className="mb-4">
          <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Secret Path</label>
          <input
            value={path}
            onChange={e => setPath(e.target.value)}
            placeholder="secret/my-app/db-password"
            className="input text-[13px]"
            disabled={!!initialPath}
          />
          <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Use slashes to organize secrets into folders</p>
        </div>

        {/* Key-Value entries */}
        <div className="mb-4">
          <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-2 block">Key-Value Pairs</label>
          <div className="space-y-2">
            {entries.map((entry, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <input
                  value={entry.key}
                  onChange={e => updateEntry(idx, 'key', e.target.value)}
                  placeholder="key"
                  className="input text-[12px] flex-1 font-mono"
                />
                <input
                  value={entry.value}
                  onChange={e => updateEntry(idx, 'value', e.target.value)}
                  placeholder="value"
                  className="input text-[12px] flex-[2] font-mono"
                  type="password"
                />
                {entries.length > 1 && (
                  <button onClick={() => removeEntry(idx)} className="text-[var(--text-tertiary)] hover:text-red-500 text-[14px]">
                    ×
                  </button>
                )}
              </div>
            ))}
          </div>
          <button onClick={addEntry} className="text-[12px] text-[var(--accent)] hover:underline mt-2">
            + Add another key
          </button>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-2 border-t border-[var(--border)]">
          <button onClick={onClose} className="btn btn-secondary btn-sm">Cancel</button>
          <button onClick={handleSave} className="btn btn-primary btn-sm" disabled={!path.trim() || saving}>
            {saving ? 'Saving...' : (initialPath ? 'Update' : 'Create')} Secret
          </button>
        </div>
      </div>
    </div>
  );
}

function VaultConfigPanel({ config, onSave, onClose, onToast }: {
  config: VaultConfig;
  onSave: (cfg: VaultConfig) => void;
  onClose: () => void;
  onToast: (msg: string, type: 'success' | 'error') => void;
}) {
  const [mode, setMode] = useState<'local' | 'remote'>(config.mode || 'local');
  const [address, setAddress] = useState(config.address || '');
  const [token, setToken] = useState('');
  const [mountPath, setMountPath] = useState(config.mount_path || 'secret');
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null);

  const handleTest = async () => {
    if (!address) return;
    setTesting(true);
    setTestResult(null);
    try {
      const res = await vault.testConnection(address, token || undefined, mountPath);
      setTestResult({ status: res.status, message: res.message });
    } catch (err) {
      setTestResult({ status: 'error', message: String(err) });
    }
    setTesting(false);
  };

  const handleSave = () => {
    if (mode === 'remote' && !address) {
      onToast('Address is required for remote mode', 'error');
      return;
    }
    onSave({ mode, address: mode === 'remote' ? address : undefined, token: mode === 'remote' ? token : undefined, mount_path: mountPath });
  };

  return (
    <div className="card">
      <div className="card-header flex items-center justify-between">
        <span className="text-[13px] font-medium text-[var(--text-primary)]">Vault Configuration</span>
        <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-[14px]">&times;</button>
      </div>
      <div className="card-body space-y-4">
        {/* Mode selector */}
        <div>
          <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-2 block">Backend Mode</label>
          <div className="flex gap-3">
            <button
              onClick={() => setMode('local')}
              className={`flex-1 p-3 rounded-lg border text-left transition-all ${
                mode === 'local' ? 'border-emerald-500/20 bg-emerald-500/10 ring-1 ring-emerald-500/20' : 'border-[var(--border)] bg-[var(--bg)] hover:border-[var(--border-light)]'
              }`}
            >
              <div className="text-[13px] font-medium text-[var(--text-primary)]">🏠 Built-in KV</div>
              <div className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Local encrypted store in PostgreSQL. AES-256-GCM. No external dependencies.</div>
            </button>
            <button
              onClick={() => setMode('remote')}
              className={`flex-1 p-3 rounded-lg border text-left transition-all ${
                mode === 'remote' ? 'border-violet-500/20 bg-violet-500/10 ring-1 ring-violet-500/20' : 'border-[var(--border)] bg-[var(--bg)] hover:border-[var(--border-light)]'
              }`}
            >
              <div className="text-[13px] font-medium text-[var(--text-primary)]">🔗 HashiCorp Vault</div>
              <div className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Connect to a remote Vault server via HTTP API. KV v2 engine.</div>
            </button>
          </div>
        </div>

        {/* Remote Vault config */}
        {mode === 'remote' && (
          <div className="space-y-3 p-3 rounded-lg bg-[var(--bg)] border border-[var(--border)]">
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Vault Address</label>
              <input
                value={address}
                onChange={e => setAddress(e.target.value)}
                placeholder="https://vault.example.com:8200"
                className="input text-[12px] font-mono"
              />
            </div>
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Token</label>
              <input
                value={token}
                onChange={e => setToken(e.target.value)}
                type="password"
                placeholder="hvs••••••••••••"
                className="input text-[12px] font-mono"
              />
              {config.token && !token && (
                <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Current token is saved. Enter a new one to update.</p>
              )}
            </div>
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">KV Engine Mount Path</label>
              <input
                value={mountPath}
                onChange={e => setMountPath(e.target.value)}
                placeholder="secret"
                className="input text-[12px] font-mono"
              />
            </div>
          </div>
        )}

        {/* Test result */}
        {mode === 'remote' && (
          <>
            {testResult && (
              <div className={`text-[12px] px-3 py-2 rounded-lg border ${
                testResult.status === 'ok' ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600' : 'bg-red-500/10 border-red-500/20 text-red-500'
              }`}>
                <strong>{testResult.status}</strong> — {testResult.message}
              </div>
            )}
            <button
              onClick={handleTest}
              disabled={!address || testing}
              className="btn btn-secondary btn-sm disabled:opacity-50"
            >
              {testing ? 'Testing...' : '◇ Test Connection'}
            </button>
          </>
        )}

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-2 border-t border-[var(--border)]">
          <button onClick={onClose} className="btn btn-secondary btn-sm">Cancel</button>
          <button onClick={handleSave} className="btn btn-primary btn-sm">
            Save Configuration
          </button>
        </div>
      </div>
    </div>
  );
}

function VaultACLPanel({ entries, currentUserId, onDelete, onRefresh, onToast }: {
  entries: VaultACLEntry[];
  currentUserId: string;
  onDelete: (id: string) => void;
  onRefresh: () => void;
  onToast: (msg: string, type: 'success' | 'error') => void;
}) {
  const [users, setUsers] = useState<User[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [showAdd, setShowAdd] = useState(false);
  const [pathPrefix, setPathPrefix] = useState('');
  const [targetType, setTargetType] = useState<'user' | 'team'>('user');
  const [selectedUserId, setSelectedUserId] = useState('');
  const [selectedTeamId, setSelectedTeamId] = useState('');
  const [canRead, setCanRead] = useState(true);
  const [canCreate, setCanCreate] = useState(false);
  const [canDelete, setCanDelete] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    listUsers().then(r => setUsers(r.users || [])).catch(() => {});
    listTeams().then(r => setTeams(r.teams || [])).catch(() => {});
  }, []);

  const handleAdd = async () => {
    if (!pathPrefix.trim()) return;
    if (targetType === 'user' && !selectedUserId) return;
    if (targetType === 'team' && !selectedTeamId) return;
    setSaving(true);
    try {
      await vault.createACL({
        path_prefix: pathPrefix.trim(),
        ...(targetType === 'user' ? { user_id: selectedUserId } : { team_id: selectedTeamId }),
        can_read: canRead,
        can_create: canCreate,
        can_delete: canDelete,
      });
      onToast('ACL entry added', 'success');
      onRefresh();
      setShowAdd(false);
      setPathPrefix('');
      setSelectedUserId('');
      setSelectedTeamId('');
    } catch (err) {
      onToast(`Failed: ${err}`, 'error');
    }
    setSaving(false);
  };

  return (
    <div className="space-y-6">
      {/* Description */}
      <div className="card card-body" style={{ borderRadius: '12px' }}>
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">Path-Based Access Control</h3>
            <p className="text-[12px] text-[var(--text-tertiary)]">
              Each user sees only their own secrets. Add rules to share specific path prefixes with other users or teams. Super Admins bypass ACL checks.
            </p>
          </div>
          <button onClick={() => setShowAdd(!showAdd)} className="btn btn-primary btn-sm shrink-0">
            {showAdd ? '✕ Cancel' : '+ Add Rule'}
          </button>
        </div>
      </div>

      {/* Add form */}
      {showAdd && (
        <div className="card page-animate-up">
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">New Access Rule</span>
          </div>
          <div className="card-body space-y-4">
            <div>
              <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1.5 block">Path Prefix</label>
              <input
                value={pathPrefix}
                onChange={e => setPathPrefix(e.target.value)}
                placeholder="e.g. team-a/secrets or db/prod"
                className="input text-[13px] font-mono w-full"
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">The rule applies to this path and all sub-paths</p>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1.5 block">Grant To</label>
                <div className="flex gap-2 mb-2">
                  <button onClick={() => setTargetType('user')} className={`flex-1 px-3 py-2 rounded-lg text-[12px] font-medium border transition-all ${targetType === 'user' ? 'border-blue-500/30 bg-blue-500/10 text-blue-500 ring-1 ring-blue-500/20' : 'border-[var(--border)] text-[var(--text-tertiary)] hover:border-[var(--border-light)]'}`}>
                    User
                  </button>
                  <button onClick={() => setTargetType('team')} className={`flex-1 px-3 py-2 rounded-lg text-[12px] font-medium border transition-all ${targetType === 'team' ? 'border-blue-500/30 bg-blue-500/10 text-blue-500 ring-1 ring-blue-500/20' : 'border-[var(--border)] text-[var(--text-tertiary)] hover:border-[var(--border-light)]'}`}>
                    Team
                  </button>
                </div>
                {targetType === 'user' ? (
                  <select value={selectedUserId} onChange={e => setSelectedUserId(e.target.value)} className="input text-[12px] w-full">
                    <option value="">Select user...</option>
                    {users.filter(u => u.is_active).map(u => (
                      <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
                    ))}
                  </select>
                ) : (
                  <select value={selectedTeamId} onChange={e => setSelectedTeamId(e.target.value)} className="input text-[12px] w-full">
                    <option value="">Select team...</option>
                    {teams.map(t => (
                      <option key={t.id} value={t.id}>{t.name}</option>
                    ))}
                  </select>
                )}
              </div>
              <div>
                <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1.5 block">Permissions</label>
                <div className="space-y-2 mt-1">
                  <label className="flex items-center gap-2.5 text-[12px] cursor-pointer">
                    <input type="checkbox" checked={canRead} onChange={e => setCanRead(e.target.checked)} className="rounded" />
                    <span className="text-[var(--text-primary)]">Read</span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">— view secrets at this path</span>
                  </label>
                  <label className="flex items-center gap-2.5 text-[12px] cursor-pointer">
                    <input type="checkbox" checked={canCreate} onChange={e => setCanCreate(e.target.checked)} className="rounded" />
                    <span className="text-[var(--text-primary)]">Create</span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">— write new secrets at this path</span>
                  </label>
                  <label className="flex items-center gap-2.5 text-[12px] cursor-pointer">
                    <input type="checkbox" checked={canDelete} onChange={e => setCanDelete(e.target.checked)} className="rounded" />
                    <span className="text-[var(--text-primary)]">Delete</span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">— remove secrets at this path</span>
                  </label>
                </div>
              </div>
            </div>
            <div className="flex justify-end pt-2 border-t border-[var(--border)]">
              <button onClick={handleAdd} disabled={saving || !pathPrefix.trim()} className="btn btn-primary btn-sm disabled:opacity-50">
                {saving ? 'Adding...' : 'Add Rule'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Entries list */}
      <div className="card">
        <div className="card-header">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Active Rules ({entries.length})</span>
        </div>
        <div className="card-body">
          {entries.length === 0 ? (
            <div className="text-center py-10">
              <div className="text-3xl mb-3 opacity-20">🔒</div>
              <p className="text-[13px] text-[var(--text-primary)] font-medium mb-1">No access rules yet</p>
              <p className="text-[12px] text-[var(--text-tertiary)] max-w-sm mx-auto">
                Users can only see their own secrets. Add rules to grant access to specific paths for other users or teams.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {entries.map(entry => {
                const isCreator = entry.created_by && entry.created_by === currentUserId;
                return (
                <div key={entry.id} className="flex items-center gap-3 p-3 rounded-xl bg-[var(--bg)] border border-[var(--border)] text-[12px]">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="px-2 py-1 rounded-lg bg-[var(--surface)] border border-[var(--border)] font-mono text-[11px] text-[var(--accent)] truncate">
                      {entry.path_prefix}
                    </span>
                    <span className="text-[var(--text-tertiary)] shrink-0">→</span>
                    <div className="flex items-center gap-1.5">
                      <span className="w-5 h-5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)] flex items-center justify-center text-[10px] font-medium">
                        {entry.user_name ? entry.user_name.charAt(0).toUpperCase() : entry.team_name ? entry.team_name.charAt(0).toUpperCase() : '?'}
                      </span>
                      <span className="text-[var(--text-primary)] font-medium">
                        {entry.user_name || entry.team_name || 'Unknown'}
                      </span>
                      <span className="text-[10px] text-[var(--text-tertiary)]">
                        {entry.user_name ? 'user' : 'team'}
                      </span>
                    </div>
                  </div>
                  <div className="flex gap-1 ml-auto shrink-0">
                    {entry.can_read && <span className="px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-600 text-[10px] font-medium">Read</span>}
                    {entry.can_create && <span className="px-2 py-0.5 rounded-md bg-blue-500/10 text-blue-500 text-[10px] font-medium">Create</span>}
                    {entry.can_delete && <span className="px-2 py-0.5 rounded-md bg-red-500/10 text-red-500 text-[10px] font-medium">Delete</span>}
                  </div>
                  {isCreator ? (
                    <button onClick={() => onDelete(entry.id)} className="text-[var(--text-tertiary)] hover:text-red-500 text-[12px] shrink-0 p-1" title="Remove rule">✕</button>
                  ) : (
                    <span className="text-[10px] text-[var(--text-tertiary)] shrink-0 px-1" title="Only the rule creator can modify or delete this entry">read-only</span>
                  )}
                </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
