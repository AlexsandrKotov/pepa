'use client';
import { useState, useCallback } from 'react';
import Link from 'next/link';
import { environments, type Environment, type EnvironmentContents, type EnvVariable, type EnvCompareEntry } from '@/lib/api';

interface Props {
  environment: Environment;
  contents: EnvironmentContents | null;
  variables: EnvVariable[];
  allEnvironments: Array<{ id: string; name: string; slug: string; color: string }>;
}

type Tab = 'clusters' | 'deployments' | 'variables' | 'compare';

export default function EnvironmentDetailClient({ environment: initialEnv, contents, variables: initialVars, allEnvironments }: Props) {
  const [env] = useState<Environment>(initialEnv);
  const [contentsData] = useState<EnvironmentContents | null>(contents);
  const [vars, setVars] = useState<EnvVariable[]>(initialVars);
  const [tab, setTab] = useState<Tab>('clusters');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Compare state
  const [compareTarget, setCompareTarget] = useState('');
  const [compareResult, setCompareResult] = useState<{ comparison: EnvCompareEntry[]; differences: number; total: number } | null>(null);
  const [compareLoading, setCompareLoading] = useState(false);

  // Variable form
  const [newKey, setNewKey] = useState('');
  const [newValue, setNewValue] = useState('');
  const [newIsSecret, setNewIsSecret] = useState(false);

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const handleAddVariable = async () => {
    if (!newKey.trim()) return;
    try {
      await environments.setVariable(env.id, { key: newKey.trim(), value: newValue, is_secret: newIsSecret });
      showToast('Variable set', 'success');
      setNewKey('');
      setNewValue('');
      setNewIsSecret(false);
      // Refresh variables
      const res = await environments.variables(env.id);
      setVars(res.variables || []);
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    }
  };

  const handleDeleteVariable = async (key: string) => {
    try {
      await environments.deleteVariable(env.id, key);
      showToast('Variable deleted', 'success');
      setVars(vars.filter(v => v.key !== key));
    } catch (err) {
      showToast(`Failed: ${err}`, 'error');
    }
  };

  const handleCompare = async () => {
    if (!compareTarget) return;
    setCompareLoading(true);
    try {
      const result = await environments.compare(env.slug || '', compareTarget);
      setCompareResult({ ...result, comparison: result.comparison || [] });
    } catch (err) {
      showToast(`Compare failed: ${err}`, 'error');
    } finally {
      setCompareLoading(false);
    }
  };

  const clusters = contentsData?.clusters || [];
  const deployments = contentsData?.deployments || [];
  const summary = contentsData?.summary || { cluster_count: clusters.length, deployment_count: deployments.length, variable_count: vars.length };

  const tabs: { key: Tab; label: string; count?: number }[] = [
    { key: 'clusters', label: 'Clusters', count: summary.cluster_count },
    { key: 'deployments', label: 'Deployments', count: summary.deployment_count },
    { key: 'variables', label: 'Variables', count: vars.length },
    { key: 'compare', label: 'Compare' },
  ];

  const otherEnvs = allEnvironments.filter(e => e.id !== env.id);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && (
          <div className={`fixed top-4 right-4 z-50 px-4 py-2.5 rounded-lg text-[13px] shadow-lg ${
            toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
          }`}>
            {toast.message}
          </div>
        )}

        {/* Header */}
        <div className="page-animate">
          <Link href="/environments" className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] mb-2 inline-block">&larr; Back to Environments</Link>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <span className="w-4 h-4 rounded-full shrink-0" style={{ backgroundColor: env.color || '#6B7280' }} />
              <div>
                <div className="flex items-center gap-2">
                  <h1 className="page-title-modern">{env.name}</h1>
                  {env.is_default && (
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-blue-500/10 text-blue-500 border border-blue-500/20 font-medium">default</span>
                  )}
                  <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                    env.status === 'active' ? 'bg-emerald-500/15 text-emerald-600 border border-emerald-500/20' : 'bg-[var(--border-light)] text-[var(--text-secondary)] border border-[var(--border)]'
                  }`}>
                    {env.status || 'active'}
                  </span>
                </div>
                <p className="page-subtitle-modern">
                  {env.slug && <span className="font-mono bg-[var(--bg)] px-1.5 py-0.5 rounded text-[11px]">{env.slug}</span>}
                  {env.description && <span className="ml-2 text-[var(--text-tertiary)]">{env.description}</span>}
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Summary Cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Clusters</p>
            <p className="text-[20px] font-semibold text-[var(--text-primary)]">{summary.cluster_count}</p>
          </div>
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Deployments</p>
            <p className="text-[20px] font-semibold text-[var(--text-primary)]">{summary.deployment_count}</p>
          </div>
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Variables</p>
            <p className="text-[20px] font-semibold text-[var(--text-primary)]">{vars.length}</p>
          </div>
          <div className="card card-body">
            <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Type</p>
            <p className="text-[14px] font-medium text-[var(--text-primary)] capitalize">{env.type || env.slug || '-'}</p>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex items-center gap-1 border-b border-[var(--border)] pb-0">
          {tabs.map(t => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`px-4 py-2 text-[12px] font-medium rounded-t-lg transition-colors relative ${
                tab === t.key
                  ? 'text-[var(--accent)] border-b-2 border-[var(--accent)]'
                  : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
              }`}
            >
              {t.label}
              {t.count !== undefined && t.count > 0 && (
                <span className="ml-1.5 text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)]">{t.count}</span>
              )}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        {tab === 'clusters' && (
          <div className="space-y-3">
            {clusters.length === 0 ? (
              <div className="card card-body text-center py-8">
                <p className="text-[13px] text-[var(--text-tertiary)]">No clusters assigned to this environment</p>
              </div>
            ) : (
              clusters.map(cl => (
                <Link key={cl.id} href={`/clusters`} className="card modern-card-hover block">
                  <div className="card-body flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className={`w-2.5 h-2.5 rounded-full ${cl.status === 'connected' ? 'bg-green-400' : 'bg-gray-300'}`} />
                      <div>
                        <p className="text-[13px] font-medium text-[var(--text-primary)]">{cl.name}</p>
                        <p className="text-[11px] text-[var(--text-tertiary)]">{cl.kubernetes_version || 'unknown version'} &middot; {cl.node_count} node{cl.node_count !== 1 ? 's' : ''}</p>
                      </div>
                    </div>
                    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                      cl.status === 'connected' ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-secondary)]'
                    }`}>{cl.status}</span>
                  </div>
                </Link>
              ))
            )}
          </div>
        )}

        {tab === 'deployments' && (
          <div className="space-y-3">
            {deployments.length === 0 ? (
              <div className="card card-body text-center py-8">
                <p className="text-[13px] text-[var(--text-tertiary)]">No deployments in this environment</p>
              </div>
            ) : (
              deployments.map(dep => (
                <div key={dep.id} className="card">
                  <div className="card-body flex items-center justify-between">
                    <div>
                      <p className="text-[13px] font-medium text-[var(--text-primary)]">{dep.deploy_type}</p>
                      <p className="text-[11px] text-[var(--text-tertiary)]">
                        {dep.image_tag && <span className="font-mono">{dep.image_tag}</span>}
                        {dep.pods_total > 0 && <span className="ml-2">{dep.pods_ready}/{dep.pods_total} pods</span>}
                      </p>
                    </div>
                    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                      dep.status === 'deployed' || dep.status === 'running' ? 'bg-emerald-500/15 text-emerald-600' :
                      dep.status === 'deploying' ? 'bg-blue-500/15 text-blue-500' :
                      dep.status === 'failed' ? 'bg-red-500/15 text-red-500' :
                      'bg-[var(--border-light)] text-[var(--text-secondary)]'
                    }`}>{dep.status}</span>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {tab === 'variables' && (
          <div className="space-y-4">
            {/* Add variable form */}
            <div className="card">
              <div className="card-header">
                <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Add Variable</h3>
              </div>
              <div className="card-body">
                <div className="flex gap-2 items-end">
                  <div className="flex-1">
                    <label className="text-[11px] text-[var(--text-tertiary)] block mb-1">Key</label>
                    <input value={newKey} onChange={e => setNewKey(e.target.value)} placeholder="DATABASE_HOST" className="input text-[12px] font-mono w-full" />
                  </div>
                  <div className="flex-1">
                    <label className="text-[11px] text-[var(--text-tertiary)] block mb-1">Value</label>
                    <input value={newValue} onChange={e => setNewValue(e.target.value)} placeholder="value" className="input text-[12px] w-full" />
                  </div>
                  <label className="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)] cursor-pointer whitespace-nowrap">
                    <input type="checkbox" checked={newIsSecret} onChange={e => setNewIsSecret(e.target.checked)} className="rounded" />
                    Secret
                  </label>
                  <button onClick={handleAddVariable} disabled={!newKey.trim()} className="btn btn-primary btn-sm disabled:opacity-50">Add</button>
                </div>
              </div>
            </div>

            {/* Variables list */}
            {vars.length === 0 ? (
              <div className="card card-body text-center py-8">
                <p className="text-[13px] text-[var(--text-tertiary)]">No variables defined for this environment</p>
              </div>
            ) : (
              <div className="card overflow-hidden">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                      <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Key</th>
                      <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Value</th>
                      <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Source</th>
                      <th className="text-right px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {vars.map(v => (
                      <tr key={v.key} className="border-b border-[var(--border-light)] hover:bg-[var(--bg)]">
                        <td className="px-4 py-2 text-[12px] font-mono font-medium text-[var(--text-primary)]">{v.key}</td>
                        <td className="px-4 py-2 text-[12px] font-mono text-[var(--text-secondary)]">
                          {v.is_secret ? '••••••••' : v.value}
                        </td>
                        <td className="px-4 py-2">
                          <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)]">{v.source}</span>
                        </td>
                        <td className="px-4 py-2 text-right">
                          <button onClick={() => handleDeleteVariable(v.key)} className="text-[11px] text-red-400 hover:text-red-600">Delete</button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {tab === 'compare' && (
          <div className="space-y-4">
            <div className="card">
              <div className="card-header">
                <h3 className="text-[13px] font-medium text-[var(--text-primary)]">Compare Environments</h3>
              </div>
              <div className="card-body">
                <div className="flex gap-2 items-end">
                  <div className="flex-1">
                    <label className="text-[11px] text-[var(--text-tertiary)] block mb-1">Compare with</label>
                    <select value={compareTarget} onChange={e => setCompareTarget(e.target.value)} className="input text-[12px] w-full">
                      <option value="">Select environment...</option>
                      {otherEnvs.map(e => (
                        <option key={e.id} value={e.slug}>{e.name}</option>
                      ))}
                    </select>
                  </div>
                  <button onClick={handleCompare} disabled={!compareTarget || compareLoading} className="btn btn-primary btn-sm disabled:opacity-50">
                    {compareLoading ? 'Comparing...' : 'Compare'}
                  </button>
                </div>
              </div>
            </div>

            {compareResult && (
              <div className="card overflow-hidden">
                <div className="card-header">
                  <span className="text-[12px] text-[var(--text-primary)] font-medium">
                    {env.name} vs {compareTarget}
                  </span>
                  <span className="text-[11px] text-[var(--text-tertiary)]">
                    {compareResult.differences} difference{compareResult.differences !== 1 ? 's' : ''} in {compareResult.total} variable{compareResult.total !== 1 ? 's' : ''}
                  </span>
                </div>
                {compareResult.comparison.length === 0 ? (
                  <div className="card-body text-center py-6">
                    <p className="text-[13px] text-[var(--text-tertiary)]">No variables to compare</p>
                  </div>
                ) : (
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
                        <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Key</th>
                        <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">{env.name}</th>
                        <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">{compareTarget}</th>
                        <th className="text-left px-4 py-2 text-[11px] font-semibold text-[var(--text-tertiary)]">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {compareResult.comparison.map((entry, idx) => (
                        <tr key={idx} className={`border-b border-[var(--border-light)] ${
                          entry.status === 'different' ? 'bg-amber-50/50' :
                          entry.status === 'only_env1' ? 'bg-blue-50/30' :
                          entry.status === 'only_env2' ? 'bg-purple-50/30' : ''
                        }`}>
                          <td className="px-4 py-2 text-[12px] font-mono font-medium text-[var(--text-primary)]">{entry.key}</td>
                          <td className="px-4 py-2 text-[12px] font-mono text-[var(--text-secondary)]">
                            {entry.is_secret && entry.value1 ? '••••••••' : entry.value1 || <span className="text-[var(--text-tertiary)] italic">not set</span>}
                          </td>
                          <td className="px-4 py-2 text-[12px] font-mono text-[var(--text-secondary)]">
                            {entry.is_secret && entry.value2 ? '••••••••' : entry.value2 || <span className="text-[var(--text-tertiary)] italic">not set</span>}
                          </td>
                          <td className="px-4 py-2">
                            <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                              entry.status === 'same' ? 'bg-emerald-500/15 text-emerald-600' :
                              entry.status === 'different' ? 'bg-amber-500/15 text-amber-600' :
                              entry.status === 'only_env1' ? 'bg-blue-500/15 text-blue-500' :
                              'bg-purple-500/15 text-purple-500'
                            }`}>
                              {entry.status === 'same' ? 'Same' :
                               entry.status === 'different' ? 'Different' :
                               entry.status === 'only_env1' ? `Only ${env.name}` :
                               `Only ${compareTarget}`}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
