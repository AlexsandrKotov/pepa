'use client';

import { useState, useEffect } from 'react';
import { connections as connectionsAPI, plugins as pluginsAPI, ai as aiAPI, type Connection, type ConnectionType, type PluginInfo } from '@/lib/api';
import Link from 'next/link';
import ConceptHelp from '@/components/ConceptHelp';
import { friendlyError } from '@/lib/errors';
import { VaultInput, VaultPickerModal, useVaultPicker } from '@/components/VaultInput';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

const CONNECTION_TYPES: { type: ConnectionType; label: string; icon: string; color: string; description: string; requiredPlugins?: string[] }[] = [
  { type: 'git', label: 'Git', icon: 'git', color: '#F05032', description: 'GitHub, GitLab, Gitea, Bitbucket, local' },
  { type: 'gitlab', label: 'GitLab', icon: 'gitlab', color: '#FC6D26', description: 'Source code and CI/CD', requiredPlugins: ['gitlab'] },
  { type: 'jira', label: 'Jira', icon: 'jira', color: '#0052CC', description: 'Issue tracking and project management', requiredPlugins: ['jira'] },
  { type: 'ai', label: 'AI Provider', icon: 'ai', color: '#8B5CF6', description: 'AI and LLM services' },
  { type: 'storage', label: 'Storage', icon: 'storage', color: '#F59E0B', description: 'Object storage (S3, MinIO)', requiredPlugins: ['s3'] },
  { type: 'proxmox', label: 'Proxmox VE', icon: 'proxmox', color: '#E57000', description: 'Virtual machines and LXC containers', requiredPlugins: ['proxmox'] },
  { type: 'vmware', label: 'VMware vCenter', icon: 'vmware', color: '#607D8B', description: 'ESXi virtual machines via vCenter', requiredPlugins: ['vmware'] },
  { type: 'notification', label: 'Notifications', icon: 'slack', color: '#E01E5A', description: 'Email, Webhook, Slack, Telegram, Microsoft Teams' },
];

const STATUS_COLORS: Record<string, string> = {
  connected: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  disconnected: 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]',
  error: 'bg-red-500/10 text-red-500 border-red-500/20',
};

// Plain-language "what you need" guidance per connection type
const TYPE_REQUIREMENTS: Record<ConnectionType, string> = {
  kubernetes: 'You need the cluster API server URL and authentication credentials (token or kubeconfig). For self-signed clusters, a CA certificate is required.',
  git: 'Select your git provider and provide the instance URL and an API token. Supports GitHub, GitLab, Gitea, Bitbucket, local repositories, and other git-compatible services.',
  gitlab: 'You need your GitLab instance URL and a personal/group access token with api scope. Create a token in GitLab under Preferences → Access Tokens.',
  jira: 'You need your Atlassian site URL (https://your-domain.atlassian.net) and an API token from id.atlassian.com → Security → API tokens.',
  ci: 'You need the CI system type, its URL and an API token with read access to pipelines.',
  ai: 'This is the single place to configure AI providers for the AI Assistant. Pick a provider (OpenAI, Anthropic, Groq, Qoder) and provide an API key and model, or use a Base URL for local models (Ollama, LM Studio). The most recently configured connection is used as the default provider.',
  storage: 'You need an S3-compatible endpoint plus access/secret keys (AWS S3, MinIO).',
  proxmox: 'You need the Proxmox VE API URL (e.g. https://proxmox.local:8006), an API Token ID (user@realm!tokenname), and the Token Secret. Create an API token in Proxmox under Datacenter → Permissions → API Tokens.',
  vmware: 'You need the vCenter Server URL (e.g. https://vcenter.example.com), a username (e.g. administrator@vsphere.local), and the password. Ensure the account has sufficient privileges to manage virtual machines.',
  notification: 'Configure a notification service to receive deployment alerts and workflow notifications. Choose Email (SMTP server), Webhook (any HTTP endpoint), Slack (webhook URL or bot token), Telegram (bot token + chat ID), or Microsoft Teams (incoming webhook URL).',
};

// Default Base URL / Model per AI provider
const AI_PROVIDER_DEFAULTS: Record<string, { url: string; model: string }> = {
  openai: { url: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },
  anthropic: { url: 'https://api.anthropic.com/v1', model: 'claude-3-5-sonnet-20241022' },
  groq: { url: 'https://api.groq.com/openai/v1', model: 'openai/gpt-oss-120b' },
  qoder: { url: 'https://api.qoder.com/v1', model: 'qoder-coder' },
  lmstudio: { url: 'http://host.docker.internal:1234/v1', model: 'local-model' },
  ollama: { url: 'http://host.docker.internal:11434', model: 'llama3' },
};

// Fields that support Vault references per connection type
const VAULT_FIELDS: Record<string, string[]> = {
  kubernetes: ['token', 'ca_certificate', 'kubeconfig'],
  git: ['token'],
  gitlab: ['token'],
  jira: ['password', 'token'],
  ci: ['token'],
  ai: ['api_key'],
  storage: ['access_key', 'secret_key'],
  proxmox: ['token_secret', 'ssh_private_key'],
  vmware: ['password'],
  notification: ['bot_token', 'webhook_url'],
};

export default function ConnectionsClient({ initialConnections, initialType }: { initialConnections?: Connection[]; initialType?: string }) {
  const [connections, setConnections] = useState(initialConnections ?? []);
  const [loading, setLoading] = useState(!initialConnections);
  const [showAddModal, setShowAddModal] = useState(false);
  const [selectedType, setSelectedType] = useState<ConnectionType | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Connection | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string; hint?: string } | null>(null);
  const [installedPluginNames, setInstalledPluginNames] = useState<Set<string>>(new Set());
  const [gitPluginStatus, setGitPluginStatus] = useState<Record<string, { installed: boolean; enabled: boolean }>>({});
  const [defaultAIProvider, setDefaultAIProvider] = useState('');
  const [settingDefault, setSettingDefault] = useState<string | null>(null);
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();

  // Fetch connections client-side (server-side has no auth token)
  useEffect(() => {
    if (initialConnections) return; // already provided
    connectionsAPI.list()
      .then(data => setConnections(data.connections || []))
      .catch(() => setConnections([]))
      .finally(() => setLoading(false));
  }, [initialConnections]);

  // Fetch installed plugins to determine which connection types are available
  useEffect(() => {
    pluginsAPI.list()
      .then(data => {
        const names = new Set((data.plugins || []).filter((p: PluginInfo) => p.enabled && p.status !== 'uninstalled').map((p: PluginInfo) => p.name));
        setInstalledPluginNames(names);
      })
      .catch(() => {});
    // Fetch git provider plugin status for availability indicators
    connectionsAPI.pluginStatus()
      .then(data => setGitPluginStatus(data || {}))
      .catch(() => {});
  }, []);

  // Fetch default AI provider
  useEffect(() => {
    aiAPI.status()
      .then(data => setDefaultAIProvider(data.default_provider || ''))
      .catch(() => {});
  }, []);

  const handleSetDefaultProvider = async (provider: string) => {
    setSettingDefault(provider);
    try {
      await aiAPI.setDefaultProvider(provider);
      setDefaultAIProvider(provider);
      setFeedback({ ok: true, text: `Default AI provider set to "${provider}"` });
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Failed to set default: ${fe.message}`, hint: fe.hint });
    } finally {
      setSettingDefault(null);
    }
  };

  // Auto-open modal with pre-selected type from URL parameter
  useEffect(() => {
    if (initialType && ['gitlab', 'git', 'jira', 'ai', 'storage', 'proxmox', 'notification'].includes(initialType)) {
      setSelectedType(initialType as ConnectionType);
      setShowAddModal(true);
    }
  }, [initialType]);

  const handleTest = async (id: string) => {
    setTesting(id);
    try {
      const result = await connectionsAPI.test(id);
      setConnections(prev => prev.map(c => c.id === id ? { ...c, status: result.status } : c));
      const ok = result.status === 'connected' || result.status === 'ok' || result.status === 'healthy';
      setFeedback({ ok, text: ok ? `Connection works: ${result.message}` : `Test failed: ${result.message}` });
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Test failed: ${fe.message}`, hint: fe.hint });
    } finally {
      setTesting(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await connectionsAPI.delete(deleteTarget.id);
      setConnections(prev => prev.filter(c => c.id !== deleteTarget.id));
      setDeleteTarget(null);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Delete failed: ${fe.message}`, hint: fe.hint });
      setDeleteTarget(null);
    } finally {
      setDeleting(false);
    }
  };

  const handleAdd = async (data: Record<string, unknown>) => {
    try {
      // Merge vault references into config
      const mergedConfig = { ...(data.config as Record<string, string> || {}) };
      for (const [field, ref] of Object.entries(vaultRefs)) {
        mergedConfig[field] = ref;
      }
      const conn = await connectionsAPI.create({ ...data, config: mergedConfig });
      setConnections(prev => [...prev, conn]);
      setShowAddModal(false);
      setSelectedType(null);
      setVaultRefs({});
      setFeedback({ ok: true, text: `Connection "${conn.name}" created. Use Test to verify it works.` });
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Create failed: ${fe.message}`, hint: fe.hint });
    }
  };

  // Filter connection types: show only if required plugin is installed (or no requirement)
  const availableConnectionTypes = CONNECTION_TYPES.filter(ct => {
    if (!ct.requiredPlugins) return true;
    return ct.requiredPlugins.some(name => installedPluginNames.has(name));
  });

  const grouped = availableConnectionTypes.map(ct => ({
    ...ct,
    items: connections.filter(c => c.type === ct.type),
  }));

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Connections</h1>
              <ConceptHelp term="connection" />
            </div>
            <p className="page-subtitle-modern">Manage external services and integrations</p>
          </div>
          <div className="flex gap-2">
            <Link href="/clusters" className="btn btn-secondary" data-tour="connections-manage-clusters">
              Manage Clusters
            </Link>
            <button
              onClick={() => setShowAddModal(true)}
              className="btn btn-primary"
              data-tour="connections-add"
            >
              + Add Connection
            </button>
          </div>
        </div>
      </div>

      {/* Inline feedback for test / create results */}
      {feedback && (
        <div className={`page-animate-up rounded-xl border p-4 flex items-start justify-between gap-3 ${
          feedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <div>
            <p className={`text-sm font-medium ${feedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {feedback.ok ? '✓ ' : '⚠ '}{feedback.text}
            </p>
            {feedback.hint && <p className="text-xs text-red-500 mt-1">{feedback.hint}</p>}
          </div>
          <button onClick={() => setFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      {loading ? (
        <div className="card card-body text-center py-12">
          <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
            <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            <p className="text-[13px]">Loading connections...</p>
          </div>
        </div>
      ) : connections.length === 0 ? (
        <div className="page-animate-up text-center py-20 card" style={{ borderRadius: '16px' }}>
          <div className="mb-4 opacity-20"><BrandIcon name="plugin" size={48} /></div>
          <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-2">No connections yet</h3>
          <p className="text-[var(--text-secondary)] mb-6">Connect your first service to get started</p>
          <button
            onClick={() => setShowAddModal(true)}
            className="btn btn-primary"
          >
            Add Connection
          </button>
        </div>
      ) : (
        <div className="space-y-8">
          {grouped.filter(g => g.items.length > 0).map((group, gi) => (
            <div key={group.type} className={`page-animate-up page-delay-${gi + 1}`}>
              <div className="flex items-center gap-3 mb-4">
                <div className="w-9 h-9 rounded-xl flex items-center justify-center" style={{ background: 'linear-gradient(135deg, rgba(0,102,255,0.08), rgba(139,92,246,0.08))' }}><BrandIcon name={group.icon} size={20} style={{ color: group.color }} /></div>
                <h2 className="text-base font-semibold text-[var(--text-primary)]">{group.label}</h2>
                <span className="text-sm text-[var(--text-tertiary)] bg-[var(--border-light)] px-2 py-0.5 rounded-full">{group.items.length}</span>
                {group.type === 'ai' && (
                  <span className="text-xs text-[var(--text-tertiary)]">Powers the AI Assistant — click a card to manage</span>
                )}
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {group.items.map(conn => {
                  const connProvider = (conn.config?.provider as string) || '';
                  const isDefaultAI = group.type === 'ai' && connProvider === defaultAIProvider && defaultAIProvider !== '';
                  return (
                  <Link
                    key={conn.id}
                    href={`/connections?id=${conn.id}`}
                    className="card card-body modern-card-hover hover:border-[var(--text-tertiary)]"
                    style={{ borderRadius: '12px' }}
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-[var(--text-primary)]">{conn.name}</h3>
                          {isDefaultAI && (
                            <span className="px-1.5 py-0.5 bg-[var(--accent)]/10 text-[var(--accent)] text-[10px] font-semibold rounded-full border border-[var(--accent)]/20">Default</span>
                          )}
                        </div>
                        {conn.description && (
                          <p className="text-sm text-[var(--text-secondary)] mt-1 line-clamp-2">{conn.description}</p>
                        )}
                      </div>
                      <span className={`text-xs px-2 py-1 rounded-full border ${STATUS_COLORS[conn.status] || STATUS_COLORS.disconnected}`}>
                        {conn.status}
                      </span>
                    </div>
                    {Object.keys(conn.labels || {}).length > 0 && (
                      <div className="flex flex-wrap gap-1 mb-3">
                        {Object.entries(conn.labels || {}).slice(0, 3).map(([k, v]) => (
                          <span key={k} className="text-xs bg-[var(--border-light)] text-[var(--text-tertiary)] px-2 py-0.5 rounded">
                            {k}={v}
                          </span>
                        ))}
                      </div>
                    )}
                    <div className="flex items-center justify-between pt-3 border-t border-[var(--border-light)]">
                      <span className="text-xs text-[var(--text-tertiary)]">
                        {conn.last_check_at ? `Last check: ${new Date(conn.last_check_at).toLocaleString()}` : 'Never tested'}
                      </span>
                      <div className="flex gap-2">
                        {group.type === 'ai' && !isDefaultAI && connProvider && (
                          <button
                            onClick={(e) => { e.preventDefault(); e.stopPropagation(); handleSetDefaultProvider(connProvider); }}
                            disabled={settingDefault === connProvider}
                            className="text-xs text-[var(--accent)] hover:opacity-80 disabled:opacity-50"
                            title="Set as default AI provider"
                          >
                            {settingDefault === connProvider ? 'Setting...' : 'Set default'}
                          </button>
                        )}
                        <button
                          onClick={(e) => { e.preventDefault(); handleTest(conn.id); }}
                          disabled={testing === conn.id}
                          className="text-xs text-[var(--accent)] hover:opacity-80 disabled:opacity-50"
                        >
                          {testing === conn.id ? 'Testing...' : 'Test'}
                        </button>
                        <button
                          onClick={(e) => { e.preventDefault(); setDeleteTarget(conn); }}
                          className="text-xs text-red-500 hover:text-red-400"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      )}

      {showAddModal && (
        <AddConnectionModal
          availableConnectionTypes={availableConnectionTypes}
          selectedType={selectedType}
          onSelectType={setSelectedType}
          onAdd={handleAdd}
          onClose={() => { setShowAddModal(false); setSelectedType(null); }}
          vaultRefs={vaultRefs}
          onVaultRefChange={setVaultRefs}
          onOpenVaultPicker={onOpenVaultPicker}
          onRemoveVault={removeVaultRef}
          gitPluginStatus={gitPluginStatus}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteTarget}
        title="Delete Connection"
        description={`Are you sure you want to delete "${deleteTarget?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      {VaultPicker}
      </div>
    </div>
  );
}

function AddConnectionModal({
  availableConnectionTypes,
  selectedType,
  onSelectType,
  onAdd,
  onClose,
  vaultRefs,
  onVaultRefChange,
  onOpenVaultPicker,
  onRemoveVault,
  gitPluginStatus,
}: {
  availableConnectionTypes: typeof CONNECTION_TYPES;
  selectedType: ConnectionType | null;
  onSelectType: (t: ConnectionType) => void;
  onAdd: (data: Record<string, unknown>) => void;
  onClose: () => void;
  vaultRefs: Record<string, string>;
  onVaultRefChange: (refs: Record<string, string>) => void;
  onOpenVaultPicker: (field: string) => void;
  onRemoveVault: (field: string) => void;
  gitPluginStatus: Record<string, { installed: boolean; enabled: boolean }>;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [notes, setNotes] = useState('');
  const [labels, setLabels] = useState('');
  const [config, setConfig] = useState<Record<string, string>>({});

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Block if required plugin is not available
    if (isPluginBlocked) return;
    const parsedLabels: Record<string, string> = {};
    labels.split(',').forEach(l => {
      const [k, v] = l.split('=').map(s => s.trim());
      if (k && v) parsedLabels[k] = v;
    });
    onAdd({
      type: selectedType,
      name,
      description,
      notes,
      labels: parsedLabels,
      config,
    });
  };

  // Check if the selected provider's plugin is missing or disabled
  const isPluginBlocked = (() => {
    if (!selectedType || !config.provider) return false;
    // Git providers that need a plugin
    if (selectedType === 'git' && ['github', 'gitlab', 'gitea', 'bitbucket'].includes(config.provider)) {
      const ps = gitPluginStatus[config.provider];
      return !ps || !ps.installed || !ps.enabled;
    }
    // Notification providers that need a plugin
    if (selectedType === 'notification' && ['slack', 'telegram', 'teams'].includes(config.provider)) {
      const ps = gitPluginStatus[config.provider];
      return !ps || !ps.installed || !ps.enabled;
    }
    // Standalone connection types with required plugins
    if (selectedType === 'gitlab') {
      const ps = gitPluginStatus.gitlab;
      return !ps || !ps.installed || !ps.enabled;
    }
    if (selectedType === 'jira') {
      const ps = gitPluginStatus.jira;
      return !ps || !ps.installed || !ps.enabled;
    }
    if (selectedType === 'proxmox') {
      const ps = gitPluginStatus.proxmox;
      return !ps || !ps.installed || !ps.enabled;
    }
    if (selectedType === 'storage') {
      const ps = gitPluginStatus.s3;
      return !ps || !ps.installed || !ps.enabled;
    }
    return false;
  })();

  if (!selectedType) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
        <div className="bg-[var(--surface)] rounded-2xl p-6 max-w-2xl w-full mx-4 max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
          <h2 className="text-xl font-bold mb-4">Select Connection Type</h2>
          <div className="grid grid-cols-2 gap-3">
            {availableConnectionTypes.map(ct => (
              <button
                key={ct.type}
                onClick={() => onSelectType(ct.type)}
                className="p-4 border border-[var(--border)] rounded-xl hover:border-blue-400 hover:bg-blue-500/10 transition-colors text-left"
              >
                <div className="text-2xl mb-2"><BrandIcon name={ct.icon} size={28} style={{ color: ct.color }} /></div>
                <div className="font-semibold">{ct.label}</div>
                <div className="text-xs text-[var(--text-tertiary)] mt-1">{ct.description}</div>
              </button>
            ))}
          </div>
          <button onClick={onClose} className="mt-4 w-full py-2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">Cancel</button>
        </div>
      </div>
    );
  }

  const typeInfo = CONNECTION_TYPES.find(ct => ct.type === selectedType)!;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-2xl p-6 max-w-lg w-full mx-4 max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <div className="flex items-center gap-3 mb-6">
          <BrandIcon name={typeInfo.icon} size={24} style={{ color: typeInfo.color }} />
          <div>
            <h2 className="text-xl font-bold">Add {typeInfo.label} Connection</h2>
            <p className="text-sm text-[var(--text-tertiary)]">{typeInfo.description}</p>
          </div>
        </div>

        <div className="mb-4 bg-blue-500/10 border border-blue-500/20 rounded-lg p-3">
          <p className="text-xs font-medium text-blue-500 mb-1">What you need</p>
          <p className="text-xs text-blue-500">{TYPE_REQUIREMENTS[selectedType]}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Name *</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              required
              className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
              placeholder="e.g., Production Cluster"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Description</label>
            <textarea
              value={description}
              onChange={e => setDescription(e.target.value)}
              rows={2}
              className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
              placeholder="Optional description"
            />
          </div>

          {selectedType === 'git' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Git Provider *</label>
                <select
                  value={config.provider || ''}
                  onChange={e => setConfig({ ...config, provider: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                >
                  <option value="">Select provider</option>
                  <option value="github">GitHub{gitPluginStatus.github?.installed ? (gitPluginStatus.github?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="gitlab">GitLab{gitPluginStatus.gitlab?.installed ? (gitPluginStatus.gitlab?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="gitea">Gitea{gitPluginStatus.gitea?.installed ? (gitPluginStatus.gitea?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="bitbucket">Bitbucket{gitPluginStatus.bitbucket?.installed ? (gitPluginStatus.bitbucket?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="local">Local (filesystem path)</option>
                  <option value="other">Other</option>
                </select>
                {config.provider && config.provider !== 'local' && config.provider !== 'other' && gitPluginStatus[config.provider] && (
                  <>
                    {!gitPluginStatus[config.provider]?.installed && (
                      <p className="text-xs text-amber-500 mt-1.5">The {config.provider.charAt(0).toUpperCase() + config.provider.slice(1)} plugin is not installed. Connection will work for basic operations, but repository browsing will be unavailable. Install it from the Marketplace.</p>
                    )}
                    {gitPluginStatus[config.provider]?.installed && !gitPluginStatus[config.provider]?.enabled && (
                      <p className="text-xs text-amber-500 mt-1.5">The {config.provider.charAt(0).toUpperCase() + config.provider.slice(1)} plugin is disabled. Enable it in the Plugins page to use repository browsing.</p>
                    )}
                  </>
                )}
                {config.provider === 'other' && (
                  <p className="text-xs text-[var(--text-tertiary)] mt-1.5">Use this for any git-compatible server. Basic operations (clone, push URL) will work. Repository browsing requires a matching plugin (GitHub, GitLab, Gitea, or Bitbucket) to be installed.</p>
                )}
              </div>
              {config.provider === 'local' ? (
                <div>
                  <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Repository Path *</label>
                  <input
                    type="text"
                    value={config.url || ''}
                    onChange={e => setConfig({ ...config, url: e.target.value })}
                    required
                    className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                    placeholder="/path/to/local/repo.git"
                  />
                  <p className="text-xs text-[var(--text-tertiary)] mt-1">Absolute path to a local git repository on the server filesystem.</p>
                </div>
              ) : (
                <div>
                  <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">URL *</label>
                  <input
                    type="url"
                    value={config.url || ''}
                    onChange={e => setConfig({ ...config, url: e.target.value })}
                    required
                    className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                    placeholder={
                      config.provider === 'github' ? 'https://github.com' :
                      config.provider === 'gitea' ? 'https://gitea.example.com' :
                      config.provider === 'bitbucket' ? 'https://bitbucket.org' :
                      config.provider === 'other' ? 'https://git.example.com' :
                      'https://git.example.com'
                    }
                  />
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Username</label>
                <input
                  type="text"
                  value={config.username || ''}
                  onChange={e => setConfig({ ...config, username: e.target.value })}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="admin"
                />
              </div>
              <VaultInput
                label="Token"
                field="token"
                value={config.token || ''}
                onChange={v => setConfig({ ...config, token: v })}
                vaultRef={vaultRefs.token}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
                placeholder={
                  config.provider === 'github' ? 'GitHub Personal Access Token' :
                  config.provider === 'gitea' ? 'Gitea API Token' :
                  config.provider === 'bitbucket' ? 'Bitbucket App Password' :
                  'API token for authentication'
                }
              />
            </>
          )}

          {(selectedType === 'gitlab' || selectedType === 'jira') && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">URL *</label>
                <input
                  type="url"
                  value={config.url || ''}
                  onChange={e => setConfig({ ...config, url: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder={selectedType === 'gitlab' ? 'https://gitlab.com' : 'https://your-domain.atlassian.net'}
                />
              </div>
              {selectedType === 'jira' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Username</label>
                    <input
                      type="text"
                      value={config.username || ''}
                      onChange={e => setConfig({ ...config, username: e.target.value })}
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="admin"
                    />
                  </div>
                  <VaultInput
                    label="Password"
                    field="password"
                    value={config.password || ''}
                    onChange={v => setConfig({ ...config, password: v })}
                    vaultRef={vaultRefs.password}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="Account password"
                  />
                </>
              )}
              <VaultInput
                label={selectedType === 'gitlab' ? 'Personal Access Token *' : 'Token'}
                field="token"
                value={config.token || ''}
                onChange={v => setConfig({ ...config, token: v })}
                vaultRef={vaultRefs.token}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
                placeholder={selectedType === 'gitlab' ? 'glpat-... (requires api scope)' : 'API token'}
                required={selectedType === 'gitlab'}
              />
              {selectedType === 'gitlab' && (
                <p className="text-xs text-[var(--text-tertiary)] -mt-2">Create a token in GitLab: Preferences → Access Tokens → enable <code className="bg-[var(--border-light)] px-1 rounded">api</code> scope.</p>
              )}
            </>
          )}

          {selectedType === 'ai' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Provider *</label>
                <select
                  value={config.provider || ''}
                  onChange={e => setConfig({ ...config, provider: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                >
                  <option value="">Select provider</option>
                  <option value="openai">OpenAI</option>
                  <option value="ollama">Ollama</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="groq">Groq</option>
                  <option value="qoder">Qoder</option>
                  <option value="lmstudio">LM Studio</option>
                </select>
              </div>
              {(() => {
                const isLocal = config.provider === 'ollama' || config.provider === 'lmstudio';
                return (
                  <VaultInput
                    label={isLocal ? 'API Key (optional)' : 'API Key'}
                    field="api_key"
                    value={config.api_key || ''}
                    onChange={v => setConfig({ ...config, api_key: v })}
                    vaultRef={vaultRefs.api_key}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder={config.provider === 'openai' ? 'sk-...' : isLocal ? 'Optional token' : 'API key'}
                  />
                );
              })()}
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Base URL</label>
                <input
                  type="url"
                  value={config.base_url || ''}
                  onChange={e => setConfig({ ...config, base_url: e.target.value })}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder={AI_PROVIDER_DEFAULTS[config.provider]?.url || 'https://api.openai.com/v1'}
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Model</label>
                <input
                  type="text"
                  value={config.model || ''}
                  onChange={e => setConfig({ ...config, model: e.target.value })}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder={AI_PROVIDER_DEFAULTS[config.provider]?.model || 'Model name'}
                />
              </div>
            </>
          )}

          {selectedType === 'storage' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Endpoint *</label>
                <input
                  type="url"
                  value={config.endpoint || ''}
                  onChange={e => setConfig({ ...config, endpoint: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="https://s3.amazonaws.com"
                />
              </div>
              <VaultInput
                label="Access Key"
                field="access_key"
                value={config.access_key || ''}
                onChange={v => setConfig({ ...config, access_key: v })}
                vaultRef={vaultRefs.access_key}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
              />
              <VaultInput
                label="Secret Key"
                field="secret_key"
                value={config.secret_key || ''}
                onChange={v => setConfig({ ...config, secret_key: v })}
                vaultRef={vaultRefs.secret_key}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
              />
            </>
          )}

          {selectedType === 'proxmox' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Proxmox API URL *</label>
                <input
                  type="url"
                  value={config.url || ''}
                  onChange={e => setConfig({ ...config, url: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="https://proxmox.local:8006"
                />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">The base URL of your Proxmox VE web interface (port 8006 by default).</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">API Token ID *</label>
                <input
                  type="text"
                  value={config.token_id || ''}
                  onChange={e => setConfig({ ...config, token_id: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="user@pam!mytoken"
                />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">Format: user@realm!tokenname (create in Datacenter → Permissions → API Tokens).</p>
              </div>
              <VaultInput
                label="API Token Secret"
                field="token_secret"
                value={config.token_secret || ''}
                onChange={v => setConfig({ ...config, token_secret: v })}
                vaultRef={vaultRefs.token_secret}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
                placeholder="API token secret value"
                required
              />
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="insecure_tls"
                  checked={config.insecure_tls === 'true'}
                  onChange={e => setConfig({ ...config, insecure_tls: e.target.checked ? 'true' : 'false' })}
                  className="w-4 h-4 rounded border-[var(--border)] text-[var(--accent)] focus:ring-[var(--accent)]"
                />
                <label htmlFor="insecure_tls" className="text-sm text-[var(--text-secondary)]">
                  Skip TLS verification (for self-signed certificates)
                </label>
              </div>
              <VaultInput
                label="SSH Private Key (for Docker-in-LXC deploys)"
                field="ssh_private_key"
                value={config.ssh_private_key || ''}
                onChange={v => setConfig({ ...config, ssh_private_key: v })}
                vaultRef={vaultRefs.ssh_private_key}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
                placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                isTextarea
              />
              <p className="text-xs text-[var(--text-tertiary)] -mt-2">
                Used to provision Docker workloads inside LXC containers. Generate with <code className="font-mono">ssh-keygen -t ed25519</code> — PEPA injects the public part into new containers automatically.
              </p>
            </>
          )}

          {selectedType === 'vmware' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">vCenter URL *</label>
                <input
                  type="url"
                  value={config.url || ''}
                  onChange={e => setConfig({ ...config, url: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="https://vcenter.example.com"
                />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">The base URL of your VMware vCenter Server.</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Username *</label>
                <input
                  type="text"
                  value={config.username || ''}
                  onChange={e => setConfig({ ...config, username: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="administrator@vsphere.local"
                />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">vCenter SSO username (e.g. administrator@vsphere.local).</p>
              </div>
              <VaultInput
                label="Password *"
                field="password"
                value={config.password || ''}
                onChange={v => setConfig({ ...config, password: v })}
                vaultRef={vaultRefs.password}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={onRemoveVault}
                placeholder="vCenter password"
                required
              />
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="vmware_insecure_tls"
                  checked={config.insecure_tls === 'true'}
                  onChange={e => setConfig({ ...config, insecure_tls: e.target.checked ? 'true' : 'false' })}
                  className="w-4 h-4 rounded border-[var(--border)] text-[var(--accent)] focus:ring-[var(--accent)]"
                />
                <label htmlFor="vmware_insecure_tls" className="text-sm text-[var(--text-secondary)]">
                  Skip TLS verification (for self-signed certificates)
                </label>
              </div>
            </>
          )}

                    {selectedType === 'notification' && (
            <>
              <div>
                <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Notification Provider *</label>
                <select
                  value={config.provider || ''}
                  onChange={e => setConfig({ ...config, provider: e.target.value })}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                >
                  <option value="">Select provider</option>
                  <option value="email">Email (SMTP)</option>
                  <option value="webhook">Webhook (HTTP)</option>
                  <option value="slack">Slack{gitPluginStatus.slack?.installed ? (gitPluginStatus.slack?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="telegram">Telegram{gitPluginStatus.telegram?.installed ? (gitPluginStatus.telegram?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                  <option value="teams">Microsoft Teams{gitPluginStatus.teams?.installed ? (gitPluginStatus.teams?.enabled ? ' \u2713' : ' (disabled)') : ''}</option>
                </select>
                {config.provider && (config.provider === 'slack' || config.provider === 'telegram' || config.provider === 'teams') && gitPluginStatus[config.provider] && (
                  <>
                    {!gitPluginStatus[config.provider]?.installed && (
                      <p className="text-xs text-amber-500 mt-1.5">The {config.provider.charAt(0).toUpperCase() + config.provider.slice(1)} plugin is not installed. Install it from the Marketplace to send notifications via {config.provider.charAt(0).toUpperCase() + config.provider.slice(1)}.</p>
                    )}
                    {gitPluginStatus[config.provider]?.installed && !gitPluginStatus[config.provider]?.enabled && (
                      <p className="text-xs text-amber-500 mt-1.5">The {config.provider.charAt(0).toUpperCase() + config.provider.slice(1)} plugin is disabled. Enable it in the Plugins page to send notifications.</p>
                    )}
                  </>
                )}
              </div>

              {config.provider === 'email' && (
                <>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">SMTP Host *</label>
                    <input
                      type="text"
                      value={config.smtp_host || ''}
                      onChange={e => setConfig({ ...config, smtp_host: e.target.value })}
                      required
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="smtp.gmail.com"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">SMTP Port *</label>
                    <input
                      type="text"
                      value={config.smtp_port || ''}
                      onChange={e => setConfig({ ...config, smtp_port: e.target.value })}
                      required
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="587"
                    />
                    <p className="text-xs text-[var(--text-tertiary)] mt-1">587 for STARTTLS, 465 for implicit TLS</p>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Username</label>
                    <input
                      type="text"
                      value={config.username || ''}
                      onChange={e => setConfig({ ...config, username: e.target.value })}
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="user@company.com"
                    />
                  </div>
                  <VaultInput
                    label="Password"
                    field="password"
                    value={config.password || ''}
                    onChange={v => setConfig({ ...config, password: v })}
                    vaultRef={vaultRefs.password}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="SMTP password or app-specific password"
                  />
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">From Address</label>
                    <input
                      type="text"
                      value={config.from || ''}
                      onChange={e => setConfig({ ...config, from: e.target.value })}
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="PEPA Notifications <noreply@company.com>"
                    />
                    <p className="text-xs text-[var(--text-tertiary)] mt-1">Defaults to username if empty</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="email_insecure_tls"
                      checked={config.insecure_tls === 'true'}
                      onChange={e => setConfig({ ...config, insecure_tls: e.target.checked ? 'true' : 'false' })}
                      className="w-4 h-4 rounded border-[var(--border)] text-[var(--accent)] focus:ring-[var(--accent)]"
                    />
                    <label htmlFor="email_insecure_tls" className="text-sm text-[var(--text-secondary)]">
                      Skip TLS verification (for self-signed certificates)
                    </label>
                  </div>
                </>
              )}

              {config.provider === 'webhook' && (
                <>
                  <VaultInput
                    label="Webhook URL *"
                    field="webhook_url"
                    value={config.webhook_url || ''}
                    onChange={v => setConfig({ ...config, webhook_url: v })}
                    vaultRef={vaultRefs.webhook_url}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="https://hooks.example.com/deployments"
                    required
                  />
                  <p className="text-xs text-[var(--text-tertiary)] -mt-2">Any HTTP(S) endpoint that accepts JSON POST requests</p>
                  <VaultInput
                    label="Shared Secret (optional)"
                    field="secret"
                    value={config.secret || ''}
                    onChange={v => setConfig({ ...config, secret: v })}
                    vaultRef={vaultRefs.secret}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="Sent as X-PEPA-Secret header for verification"
                  />
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Authorization Header (optional)</label>
                    <VaultInput
                      label=""
                      field="header.Authorization"
                      value={config['header.Authorization'] || ''}
                      onChange={v => setConfig({ ...config, 'header.Authorization': v })}
                      vaultRef={vaultRefs['header.Authorization']}
                      onOpenVault={onOpenVaultPicker}
                      onRemoveVault={onRemoveVault}
                      placeholder="Bearer eyJhbGciOi..."
                    />
                  </div>
                </>
              )}

              {config.provider === 'slack' && (
                <>
                  <p className="text-xs text-[var(--accent)] mb-2">At least one of Webhook URL or Bot Token is required.</p>
                  <VaultInput
                    label="Webhook URL"
                    field="webhook_url"
                    value={config.webhook_url || ''}
                    onChange={v => setConfig({ ...config, webhook_url: v })}
                    vaultRef={vaultRefs.webhook_url}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="https://hooks.slack.com/services/T.../B.../XXX"
                  />
                  <p className="text-xs text-[var(--text-tertiary)] -mt-2">Create at <code className="font-mono bg-[var(--border-light)] px-1 rounded">api.slack.com/messaging/webhooks</code></p>
                  <VaultInput
                    label="Bot Token (alternative)"
                    field="bot_token"
                    value={config.bot_token || ''}
                    onChange={v => setConfig({ ...config, bot_token: v })}
                    vaultRef={vaultRefs.bot_token}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="xoxb-... (for list_channels and richer API)"
                  />
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Default Channel</label>
                    <input
                      type="text"
                      value={config.channel || ''}
                      onChange={e => setConfig({ ...config, channel: e.target.value })}
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="#deployments"
                    />
                  </div>
                </>
              )}

              {config.provider === 'telegram' && (
                <>
                  <VaultInput
                    label="Bot Token *"
                    field="bot_token"
                    value={config.bot_token || ''}
                    onChange={v => setConfig({ ...config, bot_token: v })}
                    vaultRef={vaultRefs.bot_token}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"
                    required
                  />
                  <p className="text-xs text-[var(--text-tertiary)] -mt-2">Get from <code className="font-mono bg-[var(--border-light)] px-1 rounded">@BotFather</code> on Telegram</p>
                  <div>
                    <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Chat ID *</label>
                    <input
                      type="text"
                      value={config.chat_id || ''}
                      onChange={e => setConfig({ ...config, chat_id: e.target.value })}
                      required
                      className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                      placeholder="-1001234567890 or @channelname"
                    />
                    <p className="text-xs text-[var(--text-tertiary)] mt-1">Numeric chat ID or @channel username. Add the bot to the group/channel first.</p>
                  </div>
                </>
              )}

              {config.provider === 'teams' && (
                <>
                  <VaultInput
                    label="Webhook URL *"
                    field="webhook_url"
                    value={config.webhook_url || ''}
                    onChange={v => setConfig({ ...config, webhook_url: v })}
                    vaultRef={vaultRefs.webhook_url}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={onRemoveVault}
                    placeholder="https://outlook.office.com/webhook/..."
                    required
                  />
                  <p className="text-xs text-[var(--text-tertiary)] -mt-2">Create in Teams channel: Connectors → Incoming Webhook</p>
                </>
              )}
            </>
          )}

          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Labels</label>
            <input
              type="text"
              value={labels}
              onChange={e => setLabels(e.target.value)}
              className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
              placeholder="env=production, team=platform"
            />
            <p className="text-xs text-[var(--text-tertiary)] mt-1">Comma-separated key=value pairs</p>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Notes</label>
            <textarea
              value={notes}
              onChange={e => setNotes(e.target.value)}
              rows={2}
              className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
              placeholder="Internal notes"
            />
          </div>

          <div className="flex gap-3 pt-4">
            {isPluginBlocked && (
              <p className="text-xs text-red-500 w-full text-center mb-1">Install and enable the required plugin to create this connection.</p>
            )}
            <button
              type="submit"
              disabled={isPluginBlocked}
              className={`flex-1 py-2 rounded-lg transition-colors ${isPluginBlocked ? 'bg-[var(--border-light)] text-[var(--text-tertiary)] cursor-not-allowed' : 'bg-[var(--accent)] text-white hover:opacity-90'}`}
            >
              Create Connection
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-6 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--border-light)] transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}


