'use client';

import { useState, useEffect } from 'react';
import { marketplace, type MarketplacePlugin } from '@/lib/api';
import ConceptHelp from '@/components/ConceptHelp';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

export default function MarketplacePage() {
  const { isAdmin, hasPermission, loading: permLoading } = usePermission();
  const [availablePlugins, setAvailablePlugins] = useState<MarketplacePlugin[]>([]);
  const [loadingPlugins, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [installing, setInstalling] = useState<string | null>(null);
  const [expandedPlugin, setExpandedPlugin] = useState<string | null>(null);
  const [uninstallTarget, setUninstallTarget] = useState<string | null>(null);

  const canReadPlugins = isAdmin || hasPermission('plugins', 'read');

  useEffect(() => {
    if (canReadPlugins) loadPlugins();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canReadPlugins]);

  const loadPlugins = async () => {
    try {
      const data = await marketplace.list();
      setAvailablePlugins(data.plugins || []);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to load plugins', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleInstall = async (pluginId: string) => {
    setInstalling(pluginId);
    try {
      const res = await marketplace.install(pluginId);
      const hint = (res as { hint?: string })?.hint || 'Plugin installed successfully!';
      setToast({ message: hint, type: 'success' });
      await loadPlugins();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to install plugin', type: 'error' });
    } finally {
      setInstalling(null);
    }
  };

  const handleUninstall = async (pluginId: string) => {
    setInstalling(pluginId);
    try {
      await marketplace.uninstall(pluginId);
      setToast({ message: 'Plugin uninstalled successfully!', type: 'success' });
      await loadPlugins();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to uninstall plugin', type: 'error' });
    } finally {
      setInstalling(null);
      setUninstallTarget(null);
    }
  };

  const categories = Array.from(new Set(availablePlugins.map(p => p.category)));

  const filteredPlugins = availablePlugins.filter(p => {
    const matchesSearch = !searchTerm ||
      p.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      p.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
      p.id.toLowerCase().includes(searchTerm.toLowerCase());
    const matchesCategory = !selectedCategory || p.category === selectedCategory;
    return matchesSearch && matchesCategory;
  });

  const typeColors: Record<string, string> = {
    git_provider: 'bg-blue-500/10 text-blue-500',
    task_tracker: 'bg-violet-500/10 text-violet-500',
    cd_engine: 'bg-emerald-500/10 text-emerald-600',
    ci_engine: 'bg-amber-500/10 text-amber-600',
    notification: 'bg-pink-500/10 text-pink-500',
    monitoring: 'bg-cyan-500/10 text-cyan-500',
    secret_manager: 'bg-red-500/10 text-red-500',
    cloud_provider: 'bg-indigo-500/10 text-indigo-500',
    storage: 'bg-amber-500/10 text-amber-500',
    virtualization: 'bg-orange-500/10 text-orange-500',
  };

  if (permLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('plugins', 'read')) {
    return <ForbiddenPage resource="marketplace" />;
  }

  if (loadingPlugins) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <h1 className="page-title-modern">Marketplace</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-secondary)]">Loading marketplace...</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Header */}
      <div className="page-animate">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Plugin Marketplace</h1>
            <ConceptHelp term="marketplace" />
          </div>
          <p className="page-subtitle-modern">
            Install real plugins that extend PEPA with integrations for Git, CI/CD, monitoring, and more
          </p>
        </div>
      </div>

      {/* Search and Filter */}
      <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
        <div className="flex flex-col sm:flex-row gap-4">
          <div className="flex-1">
            <input
              type="text"
              placeholder="Search plugins..."
              className="input w-full"
              value={searchTerm}
              onChange={e => setSearchTerm(e.target.value)}
            />
          </div>
          <div className="flex gap-2 flex-wrap">
            <button
              className={`btn btn-sm ${!selectedCategory ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => setSelectedCategory(null)}
            >
              All
            </button>
            {categories.map(cat => (
              <button
                key={cat}
                className={`btn btn-sm ${selectedCategory === cat ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setSelectedCategory(selectedCategory === cat ? null : cat)}
              >
                {cat}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Plugins Grid */}
      {filteredPlugins.length === 0 ? (
        <div className="card card-body text-center py-12">
          <div className="text-4xl mb-3 opacity-30">🔍</div>
          <p className="text-[13px] text-[var(--text-secondary)]">No plugins found</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredPlugins.map(plugin => (
            <div key={plugin.id} className="card modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="card-header">
                <div>
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-[14px] font-medium text-[var(--text-primary)]">
                      {plugin.name || plugin.display_name || plugin.id}
                    </span>
                    {plugin.installed && plugin.running && (
                      <span className="text-[10px] px-1.5 py-0.5 bg-emerald-500/10 text-emerald-600 rounded-full">
                        Running
                      </span>
                    )}
                    {plugin.installed && !plugin.running && (
                      <span className="text-[10px] px-1.5 py-0.5 bg-yellow-500/10 text-yellow-600 rounded-full">
                        Installed
                      </span>
                    )}
                  </div>
                  <span className="text-[12px] text-[var(--text-tertiary)]">
                    v{plugin.version} by {plugin.author}
                  </span>
                </div>
                <span className={`badge ${typeColors[plugin.type] || 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
                  {plugin.type.replace(/_/g, ' ')}
                </span>
              </div>
              <div className="card-body">
                <p className="text-[12px] text-[var(--text-secondary)] mb-3 line-clamp-2">
                  {plugin.description}
                </p>

                {/* Actions preview */}
                {plugin.actions && plugin.actions.length > 0 && (
                  <div className="mb-3">
                    <button
                      className="text-[11px] text-[var(--accent)] hover:underline"
                      onClick={() => setExpandedPlugin(expandedPlugin === plugin.id ? null : plugin.id)}
                    >
                      {expandedPlugin === plugin.id ? 'Hide' : 'Show'} {plugin.actions.length} actions
                    </button>
                    {expandedPlugin === plugin.id && (
                      <div className="mt-2 space-y-1">
                        {plugin.actions.map(action => (
                          <div key={action.name} className="text-[11px] text-[var(--text-tertiary)]">
                            <span className="font-mono text-[var(--text-secondary)]">{action.name}</span>
                            {' — '}{action.description}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* Config requirements */}
                {plugin.requires_config && plugin.requires_config.length > 0 && (
                  <div className="text-[11px] text-[var(--text-tertiary)] mb-3">
                    Requires: {plugin.requires_config.join(', ')}
                  </div>
                )}

                <div className="flex items-center justify-between pt-3 border-t border-[var(--border-light)]">
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      {plugin.category}
                    </span>
                    {!plugin.binary_available && (
                      <span className="text-[10px] px-1.5 py-0.5 bg-orange-500/10 text-orange-500 rounded-full" title="Plugin binary not built yet">
                        no binary
                      </span>
                    )}
                  </div>
                  {plugin.installed ? (
                    <button
                      onClick={() => setUninstallTarget(plugin.id)}
                      disabled={installing === plugin.id}
                      className="text-[12px] text-red-500 hover:text-red-400 font-medium disabled:opacity-50"
                    >
                      {installing === plugin.id ? 'Uninstalling...' : 'Uninstall'}
                    </button>
                  ) : (
                    <button
                      onClick={() => handleInstall(plugin.id)}
                      disabled={installing === plugin.id || !plugin.binary_available}
                      className="btn btn-primary btn-sm disabled:opacity-50"
                      title={!plugin.binary_available ? 'Plugin binary is not built yet. Run `make plugins` to build it.' : ''}
                    >
                      {installing === plugin.id ? 'Installing...' : !plugin.binary_available ? 'No Binary' : 'Install'}
                    </button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}


      </div>

      <ConfirmModal
        open={!!uninstallTarget}
        title="Uninstall Plugin"
        description="Are you sure you want to uninstall this plugin? This action cannot be undone."
        confirmLabel="Uninstall"
        variant="danger"
        loading={!!uninstallTarget && installing === uninstallTarget}
        onConfirm={() => uninstallTarget && handleUninstall(uninstallTarget)}
        onCancel={() => setUninstallTarget(null)}
      />
    </div>
  );
}
