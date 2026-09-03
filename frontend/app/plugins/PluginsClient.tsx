'use client';
import { useState, useEffect, useCallback } from 'react';
import { plugins, connections, type PluginInfo, type Connection } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';
import BrandIcon from '@/components/BrandIcon';

// Maps plugin names to the connection type that provides their credentials
// Note: fluxcd/argocd use clusters (not connections), so they're handled separately
const PLUGIN_CONN_MAP: Record<string, string> = {
  gitlab: 'gitlab',
  github: 'git',
  jira: 'jira',
  bitbucket: 'git',
  gitea: 'git',
  proxmox: 'proxmox',
  vmware: 'vmware',
  s3: 'storage',
  email: 'notification',
  webhook: 'notification',
  slack: 'notification',
  telegram: 'notification',
  teams: 'notification',
};

// Plugins that require a cluster instead of a connection
const PLUGIN_NEEDS_CLUSTER: string[] = ['fluxcd', 'argocd'];

// Plugin view from DB registration (marketplace install)
interface PluginView {
  name: string;
  version: string;
  type: string;
  status: string;
  enabled: boolean;
  actions: string[];
  config: Record<string, unknown>;
  dbId?: string;
}

const TYPE_LABELS: Record<string, string> = {
  git_provider: 'Git Provider',
  task_tracker: 'Task Tracker',
  cd_engine: 'CD Engine',
  notification: 'Notifications',
  ci_engine: 'CI Engine',
  monitoring: 'Monitoring',
  secret_manager: 'Secret Manager',
  cloud_provider: 'Cloud Provider',
  storage: 'Object Storage',
  virtualization: 'Virtualization',
  custom: 'Custom',
};

const TYPE_ICONS: Record<string, string> = {
  git_provider: 'git',
  task_tracker: 'jira',
  cd_engine: 'argocd',
  notification: 'slack',
  ci_engine: 'cicd',
  monitoring: 'discovery',
  secret_manager: 'vault',
  cloud_provider: 'storage',
  storage: 'storage',
  virtualization: 'proxmox',
  custom: 'plugin',
};

const TYPE_COLORS: Record<string, string> = {
  git_provider: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  task_tracker: 'bg-violet-500/10 text-violet-500 border-violet-500/20',
  cd_engine: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  notification: 'bg-pink-500/10 text-pink-500 border-pink-500/20',
  ci_engine: 'bg-amber-500/10 text-amber-600 border-amber-500/20',
  monitoring: 'bg-cyan-500/10 text-cyan-500 border-cyan-500/20',
  secret_manager: 'bg-red-500/10 text-red-500 border-red-500/20',
  cloud_provider: 'bg-indigo-500/10 text-indigo-500 border-indigo-500/20',
  storage: 'bg-amber-500/10 text-amber-500 border-amber-500/20',
  virtualization: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
};

const EXAMPLE_CONFIGS: Record<string, Record<string, unknown>> = {
  argocd: { server_url: 'https://argocd.example.com', auth_token: '<your ArgoCD API token>', insecure: false },
  fluxcd: { kubeconfig: '<paste your kubeconfig YAML here>', namespace: 'flux-system' },
  gitlab: { url: 'https://gitlab.com', token: '<Personal Access Token with api scope>', project_id: '12345' },
  github: { token: 'ghp_<your GitHub personal access token>', url: 'https://api.github.com' },
  gitea: { url: 'https://gitea.example.com', token: '<your Gitea API token>' },
  jira: { url: 'https://your-domain.atlassian.net', username: 'email@example.com', api_token: '<API token from id.atlassian.net>', project_key: 'PROJ' },
  slack: { webhook_url: 'https://hooks.slack.com/services/T.../B.../XXX', channel: '#deployments' },
  telegram: { bot_token: '<bot token from @BotFather>', chat_id: '-1001234567890' },
  teams: { webhook_url: 'https://outlook.office.com/webhook/...' },
  prometheus: { url: 'http://prometheus:9090', token: '' },
  bitbucket: { token: '<Bitbucket App Password or OAuth token>', url: 'https://api.bitbucket.org/2.0' },
};

// Per-field descriptions shown above the config textarea
const CONFIG_FIELD_DESC: Record<string, Record<string, string>> = {
  prometheus: {
    url: 'Prometheus server URL (e.g. http://localhost:9090)',
    token: 'Optional: Bearer token for authenticated Prometheus instances',
  },
  slack: {
    webhook_url: 'Slack Incoming Webhook URL — create at api.slack.com/messaging/webhooks',
    channel: 'Default channel for notifications (e.g. #deployments)',
    bot_token: 'Alternative to webhook: Slack Bot Token (xoxb-...) for list_channels etc.',
  },
  telegram: {
    bot_token: 'Telegram Bot API token — get from @BotFather on Telegram',
    chat_id: 'Target chat ID (numeric like -1001234567890 or @channel username)',
  },
  teams: {
    webhook_url: 'Teams Incoming Webhook URL — create in channel: Connectors → Incoming Webhook',
  },
  argocd: {
    server_url: 'ArgoCD server URL (e.g. https://argocd.example.com)',
    auth_token: 'ArgoCD API token — generate in ArgoCD UI: User Info → Tokens',
    insecure: 'Skip TLS verification (true for self-signed certs, only in dev)',
  },
  fluxcd: {
    kubeconfig: 'Full kubeconfig YAML with cluster access credentials',
    namespace: 'Kubernetes namespace where Flux is installed (default: flux-system)',
  },
  gitlab: {
    url: 'GitLab instance URL (e.g. https://gitlab.com or https://git.yourcompany.com)',
    token: 'Personal Access Token with "api" scope — create in GitLab: Preferences → Access Tokens',
    project_id: 'Numeric project ID (found in project settings or URL)',
  },
  github: {
    token: 'GitHub Personal Access Token — create at github.com/settings/tokens',
    url: 'GitHub API URL (default: https://api.github.com, use enterprise URL for GHES)',
  },
  gitea: {
    url: 'Gitea instance URL (e.g. https://gitea.example.com)',
    token: 'Gitea API token — create in Settings → Applications',
  },
  jira: {
    url: 'Atlassian site URL (e.g. https://your-domain.atlassian.net)',
    username: 'Jira account email',
    api_token: 'API token — create at id.atlassian.com → Security → API tokens',
    project_key: 'Jira project key (e.g. PROJ, DEV, OPS)',
  },
  bitbucket: {
    token: 'Bitbucket App Password or OAuth token — create in Bitbucket: Personal Settings → App passwords',
    url: 'Bitbucket API URL (default: https://api.bitbucket.org/2.0)',
  },
};

// Hints shown on plugin cards to guide users on how to configure each plugin
const PLUGIN_CONFIG_HINTS: Record<string, { message: string; via: 'connection' | 'config'; link?: string }> = {
  prometheus: { message: 'Configure the Prometheus server URL via plugin configuration below.', via: 'config' },
  email: { message: 'Create a Notification connection (Email/SMTP) in Connections page to provide SMTP server credentials.', via: 'connection', link: '/connections' },
  webhook: { message: 'Create a Notification connection (Webhook) in Connections page to provide the target URL.', via: 'connection', link: '/connections' },
  slack: { message: 'Create a Notification connection (Slack) in Connections page to provide webhook URL or bot token.', via: 'connection', link: '/connections' },
  telegram: { message: 'Create a Notification connection (Telegram) in Connections page to provide bot token and chat ID.', via: 'connection', link: '/connections' },
  teams: { message: 'Create a Notification connection (Teams) in Connections page to provide the webhook URL.', via: 'connection', link: '/connections' },
  bitbucket: { message: 'Create a Git connection with Bitbucket provider in Connections page.', via: 'connection', link: '/connections' },
  gitea: { message: 'Create a Git connection with Gitea provider in Connections page.', via: 'connection', link: '/connections' },
  github: { message: 'Create a Git connection with GitHub provider in Connections page.', via: 'connection', link: '/connections' },
};

export default function PluginsClient({ initialPlugins }: { initialPlugins?: PluginInfo[] }) {
  const [pluginList, setPluginList] = useState<PluginInfo[]>(initialPlugins ?? []);
  const [connectionList, setConnectionList] = useState<Connection[]>([]);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  
  const [filterType, setFilterType] = useState<string>('all');

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  useEffect(() => {
    connections.list().then(d => setConnectionList(d.connections || [])).catch(() => {});
    // Fetch plugin list client-side when no SSR data is provided
    if (initialPlugins === undefined) {
      plugins.list().then(d => setPluginList(d.plugins || [])).catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Show all plugins that are installed or running (built-in plugins have status 'running').
  const installedPlugins: PluginView[] = pluginList
    .filter(p => p.status === 'installed' || p.status === 'running')
    .map(p => ({
      name: p.name,
      version: p.version,
      type: p.type,
      status: p.status,
      enabled: p.enabled,
      actions: p.actions || [],
      config: (p.config || {}) as Record<string, unknown>,
      dbId: p.id,
    }))
    .sort((a, b) => a.name.localeCompare(b.name));

  const filteredPlugins = filterType === 'all'
    ? installedPlugins
    : installedPlugins.filter(p => p.type === filterType);

  const availableTypes = [...new Set(installedPlugins.map(p => p.type))].sort();
  const enabledCount = installedPlugins.filter(p => p.enabled).length;

  const [toggling, setToggling] = useState<string | null>(null);

  const handleToggle = async (name: string, currentlyEnabled: boolean) => {
    setToggling(name);
    try {
      if (currentlyEnabled) {
        await plugins.disable(name);
      } else {
        await plugins.enable(name);
      }
      setPluginList(prev => prev.map(p => p.name === name ? { ...p, enabled: !currentlyEnabled } : p));
      showToast(`Plugin ${currentlyEnabled ? 'disabled' : 'enabled'}`, 'success');
      // Notify sidebar to refresh enabled plugins list
      window.dispatchEvent(new CustomEvent('pepa:plugins-changed'));
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    } finally {
      setToggling(null);
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && (
        <div className={`fixed top-4 right-4 z-50 px-4 py-2.5 rounded-xl text-[13px] shadow-lg ${
          toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
        }`}>
          {toast.message}
        </div>
      )}

      {/* Header */}
      <div className="page-animate">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Plugins</h1>
            <ConceptHelp term="plugin" />
          </div>
          <p className="page-subtitle-modern">
            {installedPlugins.length} plugins &middot; {enabledCount} enabled
          </p>
        </div>
      </div>


      {/* Filter bar */}
      {availableTypes.length > 1 && (
        <div className="flex items-center gap-2 page-animate-up page-delay-2">
          <button
            onClick={() => setFilterType('all')}
            className={`px-3 py-1.5 rounded-lg text-[12px] font-medium transition-colors ${
              filterType === 'all' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)] border border-[var(--border)] hover:bg-[var(--border-light)]'
            }`}
          >
            All ({installedPlugins.length})
          </button>
          {availableTypes.map(t => (
            <button
              key={t}
              onClick={() => setFilterType(t)}
              className={`px-3 py-1.5 rounded-lg text-[12px] font-medium transition-colors ${
                filterType === t ? 'bg-[var(--accent)] text-white' : 'bg-[var(--bg)] text-[var(--text-secondary)] border border-[var(--border)] hover:bg-[var(--border-light)]'
              }`}
            >
              <BrandIcon name={TYPE_ICONS[t] || 'plugin'} size={14} className="inline-block mr-1" /> {TYPE_LABELS[t] || t} ({installedPlugins.filter(p => p.type === t).length})
            </button>
          ))}
        </div>
      )}

      {/* Plugin Grid */}
      {filteredPlugins.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30"><BrandIcon name="plugin" size={36} /></div>
          <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">No plugins installed yet</h3>
          <p className="text-[12px] text-[var(--text-tertiary)]">
            Go to <a href="/marketplace" className="text-[var(--accent)] hover:underline">Marketplace</a> to install plugins.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {filteredPlugins.map(plugin => {
            // Find the matching connection for this plugin
            const connType = PLUGIN_CONN_MAP[plugin.name];
            const linkedConn = connType
              ? connectionList.find(c => {
                  if (connType === 'git') {
                    // For git connections, match by provider in config
                    return c.type === 'git' && (c.config as Record<string, string>)?.provider === plugin.name;
                  }
                  return c.type === connType;
                })
              : undefined;
            return (
              <PluginCard
                key={plugin.name}
                plugin={plugin}
                linkedConnection={linkedConn}
                onToggle={handleToggle}
                onToast={showToast}
                toggling={toggling === plugin.name}
              />
            );
          })}
        </div>
      )}
      </div>
    </div>
  );
}

function PluginCard({
  plugin,
  linkedConnection,
  onToggle,
  onToast,
  toggling,
}: {
  plugin: PluginView;
  linkedConnection?: Connection;
  onToggle: (name: string, enabled: boolean) => void;
  onToast: (msg: string, type: 'success' | 'error') => void;
  toggling?: boolean;
}) {
  const [showConfig, setShowConfig] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [configText, setConfigText] = useState(JSON.stringify(plugin.config || {}, null, 2));
  const [editing, setEditing] = useState(false);
  const [selectedAction, setSelectedAction] = useState('');
  const [actionParams, setActionParams] = useState('{}');
  const [actionResult, setActionResult] = useState<string | null>(null);
  const [executing, setExecuting] = useState(false);
  const [actionConfigText, setActionConfigText] = useState('');
  const [healthStatus, setHealthStatus] = useState<{ status: string; message?: string } | null>(null);

  const handleSaveConfig = async () => {
    try {
      const parsed = JSON.parse(configText);
      await plugins.configure(plugin.name, parsed);
      setEditing(false);
      onToast('Configuration saved', 'success');
    } catch (err) {
      onToast(`Invalid JSON or save failed: ${err}`, 'error');
    }
  };

  const handleExecuteAction = async () => {
    if (!selectedAction) return;
    setExecuting(true);
    setActionResult(null);
    try {
      const params = JSON.parse(actionParams);
      // Parse the config override text and merge with stored config
      let configOverride: Record<string, string> = {};
      if (actionConfigText.trim()) {
        try {
          const parsed = JSON.parse(actionConfigText);
          for (const [k, v] of Object.entries(parsed)) {
            configOverride[k] = String(v);
          }
        } catch { /* ignore invalid JSON in config override */ }
      }
      const result = await plugins.execute(plugin.name, selectedAction, params, configOverride);
      setActionResult(JSON.stringify(result, null, 2));
    } catch (err) {
      setActionResult(`Error: ${err}`);
    } finally {
      setExecuting(false);
    }
  };

  const handleCheckHealth = async () => {
    try {
      const h = await plugins.health(plugin.name);
      setHealthStatus(h);
    } catch (err) {
      setHealthStatus({ status: 'error', message: String(err) });
    }
  };

  const exampleConfig = EXAMPLE_CONFIGS[plugin.name];
  // Check if config is effectively empty (no keys, or all values are empty strings/null/undefined)
  const isConfigEmpty = !plugin.config || Object.keys(plugin.config).length === 0 ||
    Object.values(plugin.config).every(v => v === '' || v === null || v === undefined);

  // Auto-expand config and pre-fill example when plugin has no real configuration
  useEffect(() => {
    if (isConfigEmpty && exampleConfig) {
      setShowConfig(true);
      setConfigText(JSON.stringify(exampleConfig, null, 2));
      setEditing(true);
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const typeColor = TYPE_COLORS[plugin.type] || 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]';
  const icon = TYPE_ICONS[plugin.type] || 'plugin';

  return (
    <div className="card modern-card-hover" style={{ borderRadius: '12px' }}>
      {/* Card Header */}
      <div className="card-header flex items-center justify-between">
        <div className="flex items-center gap-2">
          <BrandIcon name={icon} size={18} />
          <div>
            <div className="flex items-center gap-2">
              <span className="text-[14px] font-medium text-[var(--text-primary)]">{plugin.name}</span>
              {plugin.version && <span className="text-[11px] text-[var(--text-tertiary)]">v{plugin.version}</span>}
            </div>
            <div className="flex items-center gap-1.5 mt-0.5">
              <span className={`text-[11px] px-1.5 py-0.5 rounded border ${typeColor}`}>
                {TYPE_LABELS[plugin.type] || plugin.type}
              </span>
              <span className={`text-[11px] px-1.5 py-0.5 rounded-full ${
                plugin.status === 'running' ? 'bg-emerald-500/10 text-emerald-600' :
                plugin.status === 'uninstalled' ? 'bg-red-500/10 text-red-500' :
                'bg-[var(--bg)] text-[var(--text-tertiary)]'
              }`}>
                {plugin.status}
              </span>
              {linkedConnection ? (
                <span className="text-[11px] px-1.5 py-0.5 rounded-full bg-blue-500/10 text-blue-500 border border-blue-500/20" title={`Credentials from connection: ${linkedConnection.name}`}>
                  🔗 {linkedConnection.name}
                </span>
              ) : PLUGIN_NEEDS_CLUSTER.includes(plugin.name) ? (
                <span className="text-[11px] px-1.5 py-0.5 rounded-full bg-blue-500/10 text-blue-500 border border-blue-500/20" title="This plugin uses Kubernetes clusters for operations">
                  ☸ requires cluster
                </span>
              ) : PLUGIN_CONN_MAP[plugin.name] ? (
                <span className="text-[11px] px-1.5 py-0.5 rounded-full bg-orange-500/10 text-orange-500 border border-orange-500/20" title={`No connected ${PLUGIN_CONN_MAP[plugin.name]} connection found. Configure credentials in Connections page.`}>
                  ⚠️ no connection
                </span>
              ) : null}
            </div>
          </div>
        </div>
        <button
          onClick={() => onToggle(plugin.name, plugin.enabled)}
          disabled={toggling}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
            toggling ? 'opacity-50 cursor-not-allowed' : ''
          } ${
            plugin.enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'
          }`}
          title={plugin.enabled ? 'Disable plugin' : 'Enable plugin'}
        >
          <span className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
            plugin.enabled ? 'translate-x-[18px]' : 'translate-x-[3px]'
          }`} />
        </button>
      </div>

      {/* Card Body */}
      <div className="card-body space-y-3">
        {/* Actions list */}
        {plugin.actions.length > 0 && (
          <div>
            <span className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1 block">
              Actions ({plugin.actions.length})
            </span>
            <div className="flex flex-wrap gap-1">
              {plugin.actions.map(a => (
                <span key={a} className="text-[11px] px-1.5 py-0.5 rounded bg-[var(--bg)] border border-[var(--border)] text-[var(--text-tertiary)]">
                  {a}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Expandable sections */}
        <div className="flex gap-2 pt-1">
          <button
            onClick={() => setShowConfig(!showConfig)}
            className="text-[12px] text-[var(--accent)] hover:underline"
          >
            {showConfig ? '▾' : '▸'} Configuration
          </button>
          {plugin.actions.length > 0 && (
            <button
              onClick={() => setShowActions(!showActions)}
              className="text-[12px] text-[var(--accent)] hover:underline"
            >
              {showActions ? '▾' : '▸'} Test Actions
            </button>
          )}
          <button
            onClick={handleCheckHealth}
            className="text-[12px] text-[var(--accent)] hover:underline"
          >
            ◇ Health Check
          </button>
        </div>

        {/* Health status */}
        {healthStatus && (
          <div className={`text-[12px] px-3 py-2 rounded-lg border ${
            healthStatus.status === 'healthy' ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600' :
            healthStatus.status === 'degraded' ? 'bg-yellow-500/10 border-yellow-500/20 text-yellow-600' :
            'bg-red-500/10 border-red-500/20 text-red-500'
          }`}>
            <strong>{healthStatus.status}</strong>
            {healthStatus.message && ` — ${healthStatus.message}`}
          </div>
        )}

        {/* Configuration section */}
        {showConfig && (
          <div className="space-y-2">
            {/* Field descriptions — shown when editing or when config is auto-filled */}
            {editing && CONFIG_FIELD_DESC[plugin.name] && (
              <div className="bg-blue-500/5 border border-blue-500/20 rounded-lg p-3 space-y-1">
                <p className="text-[11px] font-medium text-blue-600 mb-1.5">Configuration fields</p>
                {Object.entries(CONFIG_FIELD_DESC[plugin.name]).map(([field, desc]) => (
                  <div key={field} className="flex gap-2 text-[11px]">
                    <code className="text-blue-600 font-mono shrink-0 min-w-[100px]">{field}</code>
                    <span className="text-[var(--text-tertiary)]">{desc}</span>
                  </div>
                ))}
              </div>
            )}
            {exampleConfig && !editing && (
              <button
                onClick={() => { setConfigText(JSON.stringify(exampleConfig, null, 2)); setEditing(true); }}
                className="text-[11px] text-[var(--text-secondary)] hover:text-[var(--accent)] hover:underline"
              >
                Load example configuration for {plugin.name}
              </button>
            )}
            {editing ? (
              <div className="space-y-2">
                <textarea
                  value={configText}
                  onChange={e => setConfigText(e.target.value)}
                  rows={6}
                  className="input font-mono text-[12px]"
                  style={{ fontFamily: 'monospace' }}
                />
                <div className="flex gap-2">
                  <button onClick={handleSaveConfig} className="btn btn-primary btn-sm">Save</button>
                  <button onClick={() => { setEditing(false); setConfigText(JSON.stringify(plugin.config || {}, null, 2)); }} className="btn btn-secondary btn-sm">Cancel</button>
                </div>
              </div>
            ) : (
              <pre className="bg-[var(--bg)] rounded-lg p-3 text-[12px] font-mono text-[var(--text-secondary)] overflow-x-auto whitespace-pre">
                {JSON.stringify(plugin.config || {}, null, 2)}
              </pre>
            )}
          </div>
        )}

        {/* Test Actions section */}
        {showActions && (
          <div className="space-y-2">
            <div className="flex gap-2">
              <select
                value={selectedAction}
                onChange={e => setSelectedAction(e.target.value)}
                className="input text-[12px] flex-1"
              >
                <option value="">Select action...</option>
                {plugin.actions.map(a => (
                  <option key={a} value={a}>{a}</option>
                ))}
              </select>
              <button
                onClick={handleExecuteAction}
                disabled={!selectedAction || executing}
                className="btn btn-primary btn-sm disabled:opacity-50"
              >
                {executing ? 'Running...' : 'Execute'}
              </button>
            </div>
            {/* Params */}
            <div>
              <span className="text-[11px] text-[var(--text-tertiary)] mb-1 block">Parameters</span>
              <textarea
                value={actionParams}
                onChange={e => setActionParams(e.target.value)}
                rows={3}
                className="input font-mono text-[12px]"
                style={{ fontFamily: 'monospace' }}
                placeholder='{"name": "my-app", "namespace": "default"}'
              />
            </div>
            {/* Connection config override */}
            <div>
              <div className="flex items-center justify-between mb-1">
                <span className="text-[11px] text-[var(--text-tertiary)]">Config override</span>
                <div className="flex items-center gap-2">
                  {linkedConnection && (
                    <button
                      onClick={() => {
                        const connConfig: Record<string, unknown> = {};
                        for (const [k, v] of Object.entries(linkedConnection.config || {})) {
                          connConfig[k] = v;
                        }
                        setActionConfigText(JSON.stringify(connConfig, null, 2));
                      }}
                      className="text-[11px] text-blue-600 hover:underline"
                    >
                      Load from {linkedConnection.name}
                    </button>
                  )}
                  <button
                    onClick={() => setActionConfigText(JSON.stringify(plugin.config || {}, null, 2))}
                    className="text-[11px] text-[var(--accent)] hover:underline"
                  >
                    Load stored config
                  </button>
                </div>
              </div>
              <textarea
                value={actionConfigText}
                onChange={e => setActionConfigText(e.target.value)}
                rows={3}
                className="input font-mono text-[12px]"
                style={{ fontFamily: 'monospace' }}
                placeholder='{"token": "..."} — connection config is auto-merged on the server'
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">
                {linkedConnection
                  ? `Connection "${linkedConnection.name}" credentials are automatically merged on the server. Override specific values here if needed.`
                  : 'Stored config is automatically merged on the server. Override specific values here if needed.'}
              </p>
            </div>
            {actionResult && (
              <pre className="bg-[var(--bg)] rounded-lg p-3 text-[12px] font-mono text-[var(--text-secondary)] overflow-x-auto whitespace-pre-wrap max-h-[200px] overflow-y-auto">
                {actionResult}
              </pre>
            )}
          </div>
        )}

        {/* Configuration hint */}
        {PLUGIN_CONFIG_HINTS[plugin.name] && (
          <div className="text-[12px] px-3 py-2 rounded-lg border bg-blue-500/5 border-blue-500/20 text-blue-600">
            {PLUGIN_CONFIG_HINTS[plugin.name].via === 'connection' && PLUGIN_CONFIG_HINTS[plugin.name].link ? (
              <>💡 {PLUGIN_CONFIG_HINTS[plugin.name].message} <a href={PLUGIN_CONFIG_HINTS[plugin.name].link} className="underline font-medium">Open Connections →</a></>
            ) : (
              <>💡 {PLUGIN_CONFIG_HINTS[plugin.name].message}</>
            )}
          </div>
        )}

        {/* Plugin state hint */}
        <div className="text-[12px] text-[var(--text-tertiary)] bg-[var(--bg)] rounded-lg p-2.5 border border-[var(--border)]">
          Installed from Marketplace. The plugin binary is loaded at startup from the plugin directory.
        </div>
      </div>
    </div>
  );
}
