'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { gitopsBindings, GitOpsBinding, DiscoveredApp } from '@/lib/api';

export default function GitOpsBindingsPage() {
  const [bindings, setBindings] = useState<GitOpsBinding[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState<DiscoveredApp[]>([]);
  const [showDiscover, setShowDiscover] = useState(false);
  const [error, setError] = useState('');

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

  useEffect(() => {
    loadBindings();
  }, []);

  const handleDiscover = async () => {
    setDiscovering(true);
    try {
      const res = await gitopsBindings.discover();
      setDiscovered(res.discovered || []);
      setShowDiscover(true);
      // Reload bindings after discovery
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

  const getEngineBadge = (engine: string) => {
    const colors = {
      argocd: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
      fluxcd: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
    };
    return colors[engine as keyof typeof colors] || 'bg-gray-100 text-gray-800';
  };

  const getHealthBadge = (health: string) => {
    const colors: Record<string, string> = {
      Healthy: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
      Degraded: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
      Progressing: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
      Synced: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
      OutOfSync: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
    };
    return colors[health] || 'bg-gray-100 text-gray-800';
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
      <div className="flex items-center justify-between mb-8">
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

      {/* Bindings Table */}
      {bindings.length === 0 ? (
        <div className="text-center py-12 bg-gray-50 dark:bg-gray-800 rounded-xl">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
          </svg>
          <h3 className="mt-2 text-lg font-medium text-gray-900 dark:text-white">No bindings</h3>
          <p className="mt-2 text-gray-500 dark:text-gray-400">
            Click &ldquo;Auto-Discover&rdquo; to find and bind GitOps applications
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
                  Update Strategy
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Auto-Bound
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
              {bindings.map((binding) => (
                <tr key={binding.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="flex items-center">
                      <Link
                        href={`/gitops/applications/${binding.argo_connection_id}/${binding.app_namespace}/${binding.app_name}`}
                        className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 font-medium"
                      >
                        {binding.name}
                      </Link>
                    </div>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className={`px-2 py-1 text-xs font-medium rounded ${getEngineBadge(binding.engine_type)}`}>
                      {binding.engine_type}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-gray-600 dark:text-gray-400">
                    {binding.app_namespace}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-gray-600 dark:text-gray-400">
                    {binding.environment || '-'}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-gray-600 dark:text-gray-400">
                    {binding.update_strategy}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    {binding.auto_bound ? (
                      <span className="px-2 py-1 text-xs bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200 rounded">
                        Yes
                      </span>
                    ) : (
                      <span className="px-2 py-1 text-xs bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-400 rounded">
                        Manual
                      </span>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right">
                    <button
                      onClick={() => handleDelete(binding.id)}
                      className="text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
