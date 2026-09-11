'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  services, gitopsApplications, gitopsBindings, environments,
  type Service, type GitOpsAppSummary, type GitOpsBinding, type Environment, type DiscoveredApp,
} from '@/lib/api';

interface BindingWizardProps {
  open: boolean;
  onClose: () => void;
  onCreated?: (binding: GitOpsBinding) => void;
  preselectedServiceId?: string;
}

const STRATEGIES = [
  { value: 'kustomize_image', label: 'Kustomize Image', desc: 'Update image tag in kustomization.yaml' },
  { value: 'helm_values', label: 'Helm Values', desc: 'Update values in HelmRelease CR' },
  { value: 'raw_yaml', label: 'Raw YAML', desc: 'Direct YAML field edit' },
  { value: 'appset_param', label: 'AppSet Param', desc: 'ApplicationSet generator parameter' },
];

export default function BindingWizard({ open, onClose, onCreated, preselectedServiceId }: BindingWizardProps) {
  const [step, setStep] = useState(0);
  const [serviceList, setServiceList] = useState<Service[]>([]);
  const [selectedServiceId, setSelectedServiceId] = useState<string>(preselectedServiceId || '');
  const [apps, setApps] = useState<GitOpsAppSummary[]>([]);
  const [discovered, setDiscovered] = useState<DiscoveredApp[]>([]);
  const [envList, setEnvList] = useState<Environment[]>([]);
  const [selectedAppIdx, setSelectedAppIdx] = useState<number | null>(null);
  const [selectedEnvId, setSelectedEnvId] = useState<string>('');
  const [strategy, setStrategy] = useState('kustomize_image');
  const [bindingName, setBindingName] = useState('');
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [existingBindings, setExistingBindings] = useState<GitOpsBinding[]>([]);

  // Load initial data
  useEffect(() => {
    if (!open) return;
    setLoading(true);
    Promise.all([
      services.list({ per_page: '200' }).catch(() => ({ items: [], total: 0, page: 1, per_page: 200, total_pages: 0 })),
      gitopsApplications.list().catch(() => ({ applications: [], total: 0 })),
      environments.list().catch(() => ({ environments: [], total: 0 })),
    ]).then(([svcResult, appsResult, envResult]) => {
      setServiceList(svcResult.items || []);
      setApps(appsResult.applications || []);
      setEnvList(envResult.environments || []);
      if (preselectedServiceId) {
        setSelectedServiceId(preselectedServiceId);
        setStep(1);
      }
    }).catch(() => {
      setError('Failed to load data');
    }).finally(() => setLoading(false));
  }, [open, preselectedServiceId]);

  // Load existing bindings for selected service
  useEffect(() => {
    if (!selectedServiceId) return;
    gitopsBindings.byService(selectedServiceId)
      .then(r => setExistingBindings(r.bindings || []))
      .catch(() => setExistingBindings([]));
  }, [selectedServiceId]);

  // Discover apps when reaching step 1
  const handleDiscover = useCallback(async () => {
    setLoading(true);
    try {
      const result = await gitopsBindings.discover();
      setDiscovered(result.discovered || []);
    } catch {
      setError('Auto-discovery failed — showing all applications');
    } finally {
      setLoading(false);
      setStep(1);
    }
  }, []);

  const reset = () => {
    setStep(0);
    setSelectedServiceId(preselectedServiceId || '');
    setSelectedAppIdx(null);
    setSelectedEnvId('');
    setStrategy('kustomize_image');
    setBindingName('');
    setError(null);
    setDiscovered([]);
    setCreating(false);
  };

  const handleClose = () => {
    reset();
    onClose();
  };

  const handleCreate = async () => {
    if (selectedAppIdx === null || !selectedServiceId) return;
    const app = apps[selectedAppIdx];
    if (!app) return;

    setCreating(true);
    setError(null);
    try {
      const binding = await gitopsBindings.create({
        name: bindingName || `${app.name}-${app.namespace}`,
        service_id: selectedServiceId,
        engine_type: app.engine_type,
        app_name: app.name,
        app_namespace: app.namespace,
        argo_connection_id: app.connection_id || '',
        environment_id: selectedEnvId || undefined,
        update_strategy: strategy,
      });
      onCreated?.(binding);
      reset();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create binding');
    } finally {
      setCreating(false);
    }
  };

  if (!open) return null;

  const selectedApp = selectedAppIdx !== null ? apps[selectedAppIdx] : null;
  const boundAppKeys = new Set(existingBindings.map(b => `${b.app_name}:${b.app_namespace}`));

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={handleClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative w-full max-w-[560px] bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-6 py-4 border-b border-[var(--border)]">
          <div className="flex items-center justify-between">
            <h2 className="text-[16px] font-semibold text-[var(--text-primary)]">Create GitOps Binding</h2>
            <button onClick={handleClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
              <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          {/* Step indicator */}
          <div className="flex items-center gap-2 mt-3">
            {['Service', 'Application', 'Configure'].map((label, i) => (
              <div key={label} className="flex items-center gap-1.5">
                <div className={`w-6 h-6 rounded-full flex items-center justify-center text-[11px] font-medium ${
                  i <= step ? 'bg-[var(--accent)] text-white' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
                }`}>
                  {i + 1}
                </div>
                <span className={`text-[11px] ${i <= step ? 'text-[var(--text-primary)] font-medium' : 'text-[var(--text-tertiary)]'}`}>
                  {label}
                </span>
                {i < 2 && <div className={`w-6 h-px ${i < step ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'}`} />}
              </div>
            ))}
          </div>
        </div>

        {/* Error */}
        {error && (
          <div className="mx-6 mt-4 p-3 rounded-lg bg-[var(--danger)]/10 border border-[var(--danger)]/20 text-[12px] text-[var(--danger)]">
            {error}
            <button onClick={() => setError(null)} className="ml-2 underline">dismiss</button>
          </div>
        )}

        {/* Body */}
        <div className="px-6 py-4 max-h-[400px] overflow-y-auto">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <div className="loading-spinner" />
            </div>
          ) : step === 0 ? (
            /* Step 0: Select Service */
            <div className="space-y-3">
              <p className="text-[12px] text-[var(--text-secondary)]">
                Select the service to bind to a GitOps application.
              </p>
              {serviceList.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-[12px] text-[var(--text-tertiary)]">No services found. Create a service first.</p>
                </div>
              ) : (
                <div className="space-y-1">
                  {serviceList.map(svc => (
                    <button
                      key={svc.id}
                      onClick={() => { setSelectedServiceId(svc.id); setStep(1); }}
                      className={`w-full text-left p-3 rounded-lg border transition-all ${
                        selectedServiceId === svc.id
                          ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                          : 'border-[var(--border-light)] hover:border-[var(--border)] hover:bg-[var(--bg)]'
                      }`}
                    >
                      <p className="text-[13px] font-medium text-[var(--text-primary)]">{svc.name}</p>
                      <p className="text-[11px] text-[var(--text-tertiary)]">
                        {svc.namespace}{svc.language ? ` \u00b7 ${svc.language}` : ''}{svc.status ? ` \u00b7 ${svc.status}` : ''}
                      </p>
                    </button>
                  ))}
                </div>
              )}
            </div>
          ) : step === 1 ? (
            /* Step 1: Select Application */
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-[12px] text-[var(--text-secondary)]">
                  Select a GitOps application to bind.
                </p>
                {discovered.length === 0 && (
                  <button onClick={handleDiscover} className="text-[11px] text-[var(--accent)] hover:underline">
                    Auto-discover
                  </button>
                )}
              </div>
              {apps.length === 0 ? (
                <div className="text-center py-6">
                  <p className="text-[12px] text-[var(--text-tertiary)]">No GitOps applications found.</p>
                  <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                    Ensure FluxCD/ArgoCD connections are configured and applications exist in the cluster.
                  </p>
                </div>
              ) : (
                <div className="space-y-1">
                  {apps.map((app, i) => {
                    const isBound = boundAppKeys.has(`${app.name}:${app.namespace}`);
                    return (
                      <button
                        key={`${app.connection_id}-${app.namespace}-${app.name}-${i}`}
                        onClick={() => !isBound && (setSelectedAppIdx(i), setStep(2))}
                        disabled={isBound}
                        className={`w-full text-left p-3 rounded-lg border transition-all ${
                          isBound ? 'border-[var(--border-light)] opacity-50 cursor-not-allowed' :
                          selectedAppIdx === i
                            ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                            : 'border-[var(--border-light)] hover:border-[var(--border)] hover:bg-[var(--bg)]'
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className={`w-2 h-2 rounded-full shrink-0 ${
                            app.health === 'healthy' || app.health === 'Healthy' || app.health === 'Ready' ? 'bg-emerald-500' :
                            app.health === 'degraded' || app.health === 'Degraded' ? 'bg-red-500' :
                            'bg-blue-500'
                          }`} />
                          <p className="text-[13px] font-medium text-[var(--text-primary)] flex-1">{app.name}</p>
                          <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${
                            app.engine_type === 'fluxcd' ? 'bg-purple-500/15 text-purple-600' : 'bg-orange-500/15 text-orange-600'
                          }`}>
                            {app.engine_type === 'fluxcd' ? 'FluxCD' : 'ArgoCD'}
                          </span>
                        </div>
                        <p className="text-[11px] text-[var(--text-tertiary)] ml-4 mt-0.5">
                          {app.namespace}{app.revision ? ` \u00b7 ${app.revision.slice(0, 7)}` : ''}
                          {isBound && ' \u00b7 already bound'}
                        </p>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          ) : (
            /* Step 2: Configure Binding */
            <div className="space-y-4">
              {selectedApp && (
                <div className="p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg)]">
                  <p className="text-[12px] font-medium text-[var(--text-primary)]">{selectedApp.name}</p>
                  <p className="text-[11px] text-[var(--text-tertiary)]">
                    {selectedApp.namespace} \u00b7 {selectedApp.engine_type === 'fluxcd' ? 'FluxCD' : 'ArgoCD'}
                  </p>
                </div>
              )}

              {/* Binding name */}
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase block mb-1.5">
                  Binding Name (optional)
                </label>
                <input
                  type="text"
                  value={bindingName}
                  onChange={e => setBindingName(e.target.value)}
                  placeholder={selectedApp ? `${selectedApp.name}-${selectedApp.namespace}` : ''}
                  className="input w-full text-[13px]"
                />
              </div>

              {/* Environment */}
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase block mb-1.5">
                  Environment
                </label>
                <select
                  value={selectedEnvId}
                  onChange={e => setSelectedEnvId(e.target.value)}
                  className="select w-full text-[13px]"
                >
                  <option value="">None</option>
                  {envList.map(env => (
                    <option key={env.id} value={env.id}>{env.name}</option>
                  ))}
                </select>
              </div>

              {/* Update Strategy */}
              <div>
                <label className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase block mb-1.5">
                  Update Strategy
                </label>
                <div className="space-y-1.5">
                  {STRATEGIES.map(s => (
                    <button
                      key={s.value}
                      onClick={() => setStrategy(s.value)}
                      className={`w-full text-left p-2.5 rounded-lg border transition-all ${
                        strategy === s.value
                          ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                          : 'border-[var(--border-light)] hover:border-[var(--border)]'
                      }`}
                    >
                      <p className="text-[12px] font-medium text-[var(--text-primary)]">{s.label}</p>
                      <p className="text-[10px] text-[var(--text-tertiary)]">{s.desc}</p>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-[var(--border)] flex items-center justify-between">
          <button
            onClick={() => step > 0 ? setStep(step - 1) : handleClose()}
            className="btn btn-secondary btn-sm"
          >
            {step > 0 ? 'Back' : 'Cancel'}
          </button>
          <div className="flex items-center gap-2">
            {step === 0 && apps.length > 0 && (
              <button
                onClick={() => setStep(1)}
                className="btn btn-secondary btn-sm"
              >
                Skip to Apps
              </button>
            )}
            {step === 2 && (
              <button
                onClick={handleCreate}
                disabled={creating || selectedAppIdx === null}
                className="btn btn-primary btn-sm"
              >
                {creating ? 'Creating...' : 'Create Binding'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
