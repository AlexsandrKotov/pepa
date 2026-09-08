'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { deployments, type Deployment } from '@/lib/api';

interface BatchService {
  id: string;
  name: string;
  selected: boolean;
}

export default function BatchDeployPage() {
  const [deployList, setDeployList] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedServices, setSelectedServices] = useState<Set<string>>(new Set());
  const [targetNamespace, setTargetNamespace] = useState('default');
  const [executing, setExecuting] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    try {
      const res = await deployments.list();
      setDeployList(res.deployments || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  // Group deployments by project to get unique services
  const serviceMap = new Map<string, Deployment>();
  for (const d of deployList) {
    const key = d.gitlab_project_name || d.id;
    if (!serviceMap.has(key)) {
      serviceMap.set(key, d);
    }
  }
  const services: BatchService[] = Array.from(serviceMap.entries()).map(([key, d]) => ({
    id: key,
    name: d.gitlab_project_name || d.jira_issue_key || key.slice(0, 8),
    selected: selectedServices.has(key),
  }));

  const toggleService = (id: string) => {
    const next = new Set(selectedServices);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedServices(next);
  };

  const selectAll = () => {
    if (selectedServices.size === services.length) {
      setSelectedServices(new Set());
    } else {
      setSelectedServices(new Set(services.map(s => s.id)));
    }
  };

  const handleExecute = async () => {
    if (selectedServices.size === 0) return;
    setExecuting(true);
    setResult(null);
    try {
      // Create a batch operation for each selected service
      const promises = Array.from(selectedServices).map(svcId => {
        const dep = serviceMap.get(svcId);
        return deployments.create({
          gitlab_project_name: dep?.gitlab_project_name || svcId,
          target_namespace: targetNamespace,
          target_cluster_id: dep?.target_cluster_id || undefined,
          deploy_type: dep?.deploy_type || 'helm',
          replicas: dep?.replicas || 1,
          strategy: dep?.strategy || 'rolling',
          spec: dep?.spec || {},
          status: 'pending',
          created_by: 'batch-deploy',
          timeout_seconds: dep?.timeout_seconds || 300,
        });
      });
      const results = await Promise.allSettled(promises);
      const succeeded = results.filter(r => r.status === 'fulfilled').length;
      const failed = results.length - succeeded;
      setResult({
        ok: failed === 0,
        text: `Batch deploy completed: ${succeeded} succeeded, ${failed} failed out of ${results.length} services`,
      });
      setSelectedServices(new Set());
    } catch (e: unknown) {
      setResult({ ok: false, text: 'Batch deploy failed: ' + (e instanceof Error ? e.message : 'Error') });
    }
    setExecuting(false);
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6">
          <div className="flex items-center justify-center py-12">
            <div className="loading-spinner" />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Batch Deploy</h1>
            </div>
            <p className="page-subtitle-modern">Deploy multiple services at once</p>
          </div>
          <Link href="/deployments" className="text-[12px] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            {'\u2190'} Back to Deployments
          </Link>
        </div>

        {/* Feedback */}
        {result && (
          <div className={`rounded-xl border p-4 page-animate-up ${
            result.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
          }`}>
            <p className={`text-sm font-medium ${result.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {result.ok ? '\u2713 ' : '\u26A0 '}{result.text}
            </p>
          </div>
        )}

        {/* Configuration */}
        <div className="card page-animate-up page-delay-1">
          <div className="card-body space-y-4">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">Configuration</h3>
            <div className="flex items-center gap-4">
              <div>
                <label className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider block mb-1">Target Namespace</label>
                <input
                  type="text"
                  value={targetNamespace}
                  onChange={e => setTargetNamespace(e.target.value)}
                  className="input w-48"
                  placeholder="default"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Service Selection */}
        <div className="card page-animate-up page-delay-2">
          <div className="card-body">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">
                Select Services ({selectedServices.size}/{services.length})
              </h3>
              <button onClick={selectAll} className="text-[11px] text-[var(--accent)] hover:underline">
                {selectedServices.size === services.length ? 'Deselect All' : 'Select All'}
              </button>
            </div>
            {services.length === 0 ? (
              <p className="text-[13px] text-[var(--text-tertiary)] text-center py-6">No services found. Create a deployment first.</p>
            ) : (
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2 max-h-[400px] overflow-y-auto">
                {services.map(svc => (
                  <button
                    key={svc.id}
                    onClick={() => toggleService(svc.id)}
                    className={`text-left p-3 rounded-lg border transition-colors ${
                      svc.selected
                        ? 'border-blue-500/30 bg-blue-500/5'
                        : 'border-[var(--border)] hover:border-[var(--border-hover)]'
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <div className={`w-4 h-4 rounded border flex items-center justify-center text-[10px] ${
                        svc.selected ? 'bg-blue-500 border-blue-500 text-white' : 'border-[var(--border)]'
                      }`}>
                        {svc.selected && '\u2713'}
                      </div>
                      <span className="text-[12px] font-medium text-[var(--text-primary)] truncate">{svc.name}</span>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Execute */}
        <div className="flex justify-end gap-3 page-animate-up page-delay-3">
          <Link href="/deployments" className="btn btn-secondary text-[12px]">Cancel</Link>
          <button
            onClick={handleExecute}
            disabled={selectedServices.size === 0 || executing}
            className="btn btn-primary text-[12px]"
          >
            {executing ? 'Deploying...' : `Deploy ${selectedServices.size} Service${selectedServices.size !== 1 ? 's' : ''}`}
          </button>
        </div>
      </div>
    </div>
  );
}
