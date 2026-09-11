'use client';

import { useEffect, useState, useMemo } from 'react';
import Link from 'next/link';
import { gitopsBindings, environments, GitOpsBinding, DiscoveredApp, Environment, WriteBackResult } from '@/lib/api';
import BindingWizard from '@/components/BindingWizard';

export default function GitOpsBindingsPage() {
  const [bindings, setBindings] = useState<GitOpsBinding[]>([]);
  const [envs, setEnvs] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoveredApp[]>([]);
  const [showDiscover, setShowDiscover] = useState(false);
  const [error, setError] = useState('');

  // Filters
  const [filterEnv, setFilterEnv] = useState('');
  const [filterEngine, setFilterEngine] = useState('');
  const [searchQuery, setSearchQuery] = useState('');

  // Set environment modal
  const [settingEnv, setSettingEnv] = useState<GitOpsBinding | null>(null);
  const [showWizard, setShowWizard] = useState(false);

  // Write-back modal
  const [writeBackBinding, setWriteBackBinding] = useState<GitOpsBinding | null>(null);
  const [writeBackTag, setWriteBackTag] = useState('');
  const [writeBackName, setWriteBackName] = useState('');
  const [writeBackLoading, setWriteBackLoading] = useState(false);
  const [writeBackResult, setWriteBackResult] = useState<WriteBackResult | null>(null);
  const [writeBackError, setWriteBackError] = useState('');

  const loadBindings = async () => {
    try {
      const res = await gitopsBindings.list();
      setBindings(res.bindings || []);
    } catch (err) {
      setError('Failed to load bindings');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const loadEnvironments = async () => {
    try {
      const res = await environments.list();
      setEnvs(res.environments || []);
    } catch { /* ignore */ }
  };

  useEffect(() => {
    loadBindings();
    loadEnvironments();
  }, []);

  const handleDiscover = async () => {
    setDiscovering(true);
    try {
      const res = await gitopsBindings.discover();
      setDiscovered(res.discovered || []);
      setShowDiscover(true);
      await loadBindings();
    } catch (err) {
      setError('Discovery failed');
      console.error(err);
    } finally {
      setDiscovering(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this binding?')) return;
    try {
      await gitopsBindings.delete(id);
      setBindings(bindings.filter(b => b.id !== id));
    } catch (err) {
      setError('Failed to delete binding');
      console.error(err);
    }
  };

  const handleSetEnvironment = async (binding: GitOpsBinding, envId: string) => {
    try {
      if (envId) {
        await gitopsBindings.update(binding.id, {
          environment_id: envId,
          environment: envs.find(e => e.id === envId)?.slug,
        });
      } else {
        await gitopsBindings.update(binding.id, {
          clear_environment: true,
        });
      }
      await loadBindings();
      setSettingEnv(null);
    } catch (err) {
      setError('Failed to set environment');
      console.error(err);
    }
  };

  const handleWriteBack = async (dryRun: boolean) => {
    if (!writeBackBinding || !writeBackTag) return;
    setWriteBackLoading(true);
    setWriteBackError('');
    setWriteBackResult(null);
    try {
      const res = await gitopsBindings.writeBack(writeBackBinding.id, {
        image_tag: writeBackTag,
        image_name: writeBackName || undefined,
        dry_run: dryRun,
      });
      setWriteBackResult(res.result);
    } catch (err) {
      setWriteBackError(err instanceof Error ? err.message : 'Write-back failed');
    } finally {
      setWriteBackLoading(false);
    }
  };

  const openWriteBack = (binding: GitOpsBinding) => {
    setWriteBackBinding(binding);
    setWriteBackTag('');
    setWriteBackName('');
    setWriteBackResult(null);
    setWriteBackError('');
  };

  // Filtered bindings
  const filteredBindings = useMemo(() => {
    return bindings.filter(b => {
      if (filterEnv && b.environment_id !== filterEnv) return false;
      if (filterEngine && b.engine_type !== filterEngine) return false;
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        if (!b.app_name.toLowerCase().includes(q) &&
            !b.app_namespace.toLowerCase().includes(q) &&
            !b.name.toLowerCase().includes(q)) return false;
      }
      return true;
    });
  }, [bindings, filterEnv, filterEngine, searchQuery]);

  const getEngineBadge = (engine: string) => {
    const colors: Record<string, string> = {
      argocd: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
      fluxcd: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
    };
    return colors[engine] || 'bg-gray-100 text-gray-800';
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-3xl font-bold text-gray-900 dark:text-white">GitOps Bindings</h1>
          <p className="text-gray-600 dark:text-gray-400 mt-2">
            Map services to GitOps applications (ArgoCD / FluxCD)
          </p>
        </div>
        <div className="flex gap-3">
          <Link
            href="/gitops/applications"
            className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200 dark:hover:bg-gray-600"
          >
            Applications
          </Link>
          <button
            onClick={() => setShowWizard(true)}
            className="px-4 py-2 bg-green-600 text-white rounded-lg hover:bg-green-700"
          >
            Create Binding
          </button>
          <button
            onClick={handleDiscover}
            disabled={discovering}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            {discovering ? 'Discovering...' : 'Auto-Discover'}
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg text-red-700 dark:bg-red-900/20 dark:border-red-800 dark:text-red-400">
          {error}
        </div>
      )}

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-6">
        <input
          type="text"
          placeholder="Search apps..."
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-white w-64"
        />
        <select
          value={filterEnv}
          onChange={e => setFilterEnv(e.target.value)}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-white"
        >
          <option value="">All Environments</option>
          {envs.map(env => (
            <option key={env.id} value={env.id}>{env.name}</option>
          ))}
        </select>
        <select
          value={filterEngine}
          onChange={e => setFilterEngine(e.target.value)}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-white"
        >
          <option value="">All Engines</option>
          <option value="argocd">ArgoCD</option>
          <option value="fluxcd">FluxCD</option>
        </select>
        {(filterEnv || filterEngine || searchQuery) && (
          <button
            onClick={() => { setFilterEnv(''); setFilterEngine(''); setSearchQuery(''); }}
            className="px-3 py-2 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
          >
            Clear filters
          </button>
        )}
      </div>

      {/* Discovery Results Modal */}
      {showDiscover && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 max-w-2xl w-full mx-4 max-h-[80vh] overflow-auto">
            <h2 className="text-xl font-bold mb-4 text-gray-900 dark:text-white">
              Discovery Results
            </h2>
            {discovered.length === 0 ? (
              <p className="text-gray-600 dark:text-gray-400">No applications discovered</p>
            ) : (
              <div className="space-y-3">
                {discovered.map((app, idx) => (
                  <div
                    key={idx}
                    className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700 rounded-lg"
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className={`px-2 py-0.5 text-xs font-medium rounded ${getEngineBadge(app.app.engine_type)}`}>
                          {app.app.engine_type}
                        </span>
                        <span className="font-medium text-gray-900 dark:text-white">
                          {app.app.name}
                        </span>
                        <span className="text-gray-500 text-sm">
                          {app.app.namespace}
                        </span>
                      </div>
                      <div className="text-sm text-gray-500 mt-1">
                        Connection: {app.connection_name}
                      </div>
                    </div>
                    <span className={`px-2 py-1 text-xs rounded ${app.bound ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' : 'bg-gray-100 text-gray-800'}`}>
                      {app.bound ? 'Bound' : 'Unbound'}
                    </span>
                  </div>
                ))}
              </div>
            )}
            <button
              onClick={() => setShowDiscover(false)}
              className="mt-6 w-full px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200"
            >
              Close
            </button>
          </div>
        </div>
      )}

      {/* Set Environment Modal */}
      {settingEnv && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 max-w-md w-full mx-4">
            <h2 className="text-lg font-bold mb-4 text-gray-900 dark:text-white">
              Set Environment for &ldquo;{settingEnv.app_name}&rdquo;
            </h2>
            <div className="space-y-2">
              <button
                onClick={() => handleSetEnvironment(settingEnv, '')}
                className="w-full text-left px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-600 dark:text-gray-400"
              >
                None
              </button>
              {envs.map(env => (
                <button
                  key={env.id}
                  onClick={() => handleSetEnvironment(settingEnv, env.id)}
                  className={`w-full text-left px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 flex items-center gap-2 ${
                    settingEnv.environment_id === env.id ? 'bg-blue-50 dark:bg-blue-900/20' : ''
                  }`}
                >
                  <span className="w-3 h-3 rounded-full" style={{ backgroundColor: env.color || '#6B7280' }} />
                  <span className="text-gray-900 dark:text-white">{env.name}</span>
                  <span className="text-gray-500 text-sm ml-auto">{env.slug}</span>
                </button>
              ))}
            </div>
            <button
              onClick={() => setSettingEnv(null)}
              className="mt-4 w-full px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Bindings Table */}
      {filteredBindings.length === 0 ? (
        <div className="text-center py-12 bg-gray-50 dark:bg-gray-800 rounded-xl">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
          </svg>
          <h3 className="mt-2 text-lg font-medium text-gray-900 dark:text-white">
            {bindings.length === 0 ? 'No bindings' : 'No matching bindings'}
          </h3>
          <p className="mt-2 text-gray-500 dark:text-gray-400">
            {bindings.length === 0
              ? 'Click "Auto-Discover" to find and bind GitOps applications'
              : 'Try adjusting your filters'}
          </p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow overflow-hidden">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Application
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Engine
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Namespace
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Environment
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Strategy
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
              {filteredBindings.map((binding) => (
                <tr key={binding.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <Link
                      href={`/gitops/applications/${binding.argo_connection_id}/${binding.app_namespace}/${binding.app_name}`}
                      className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 font-medium"
                    >
                      {binding.name}
                    </Link>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className={`px-2 py-1 text-xs font-medium rounded ${getEngineBadge(binding.engine_type)}`}>
                      {binding.engine_type}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-gray-600 dark:text-gray-400">
                    {binding.app_namespace}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    {binding.env_name ? (
                      <button
                        onClick={() => setSettingEnv(binding)}
                        className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium hover:opacity-80 transition-opacity"
                        style={{
                          backgroundColor: (binding.env_color || '#6B7280') + '20',
                          color: binding.env_color || '#6B7280',
                        }}
                      >
                        <span className="w-2 h-2 rounded-full" style={{ backgroundColor: binding.env_color || '#6B7280' }} />
                        {binding.env_name}
                      </button>
                    ) : (
                      <button
                        onClick={() => setSettingEnv(binding)}
                        className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 text-sm"
                      >
                        Set...
                      </button>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-gray-600 dark:text-gray-400 text-sm">
                    {binding.update_strategy}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button
                        onClick={() => openWriteBack(binding)}
                        className="text-emerald-600 hover:text-emerald-800 dark:text-emerald-400 dark:hover:text-emerald-300 text-sm font-medium"
                        title="Write image tag back to Git"
                      >
                        Write Back
                      </button>
                      <button
                        onClick={() => handleDelete(binding.id)}
                        className="text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300 text-sm"
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="px-6 py-3 bg-gray-50 dark:bg-gray-700 text-sm text-gray-500 dark:text-gray-400">
            {filteredBindings.length} of {bindings.length} binding{bindings.length !== 1 ? 's' : ''}
          </div>
        </div>
      )}

      {/* Write-Back Modal */}
      {writeBackBinding && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-xl p-6 max-w-lg w-full mx-4">
            <h2 className="text-lg font-bold mb-1 text-gray-900 dark:text-white">
              Write Image Tag to Git
            </h2>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
              Update the image tag in the manifest repo for &ldquo;{writeBackBinding.app_name}&rdquo; ({writeBackBinding.update_strategy})
            </p>

            <div className="space-y-3">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Image Tag *</label>
                <input
                  type="text"
                  value={writeBackTag}
                  onChange={e => setWriteBackTag(e.target.value)}
                  placeholder="e.g. v1.2.3, sha-abc1234"
                  className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:ring-2 focus:ring-emerald-500 focus:border-transparent"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Image Name (optional)</label>
                <input
                  type="text"
                  value={writeBackName}
                  onChange={e => setWriteBackName(e.target.value)}
                  placeholder="e.g. registry.example.com/myapp"
                  className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm focus:ring-2 focus:ring-emerald-500 focus:border-transparent"
                />
              </div>
            </div>

            {writeBackError && (
              <div className="mt-3 p-3 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm">
                {writeBackError}
              </div>
            )}

            {writeBackResult && (
              <div className="mt-3 p-3 rounded-lg bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 text-sm space-y-1">
                <div className="font-medium">Write-back successful</div>
                <div>Commit: <code className="text-xs">{writeBackResult.commit_sha?.slice(0, 8) || 'preview'}</code></div>
                <div>Branch: {writeBackResult.branch}</div>
                <div>File: {writeBackResult.file_path}</div>
                {writeBackResult.mr_needed && (
                  <div className="text-amber-600 dark:text-amber-400">MR needed — push was rejected, branch created</div>
                )}
                {writeBackResult.diff && (
                  <details className="mt-2">
                    <summary className="cursor-pointer text-xs">View diff</summary>
                    <pre className="mt-1 text-xs overflow-x-auto bg-gray-100 dark:bg-gray-900 p-2 rounded">{writeBackResult.diff}</pre>
                  </details>
                )}
              </div>
            )}

            <div className="mt-4 flex items-center gap-2">
              <button
                onClick={() => handleWriteBack(false)}
                disabled={!writeBackTag || writeBackLoading}
                className="flex-1 px-4 py-2 bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
              >
                {writeBackLoading ? 'Writing...' : 'Commit to Git'}
              </button>
              <button
                onClick={() => handleWriteBack(true)}
                disabled={!writeBackTag || writeBackLoading}
                className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200 text-sm"
              >
                Preview
              </button>
              <button
                onClick={() => { setWriteBackBinding(null); setWriteBackResult(null); }}
                className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200 text-sm"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Binding Wizard */}
      <BindingWizard
        open={showWizard}
        onClose={() => setShowWizard(false)}
        onCreated={() => loadBindings()}
      />
    </div>
  );
}
