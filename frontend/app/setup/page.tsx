'use client';

import { useState, useEffect, useCallback } from 'react';
import { connections as connectionsAPI, organization, type ConnectionType } from '@/lib/api';
import { friendlyError, type FriendlyError } from '@/lib/errors';
import { useRouter } from 'next/navigation';
import { VaultInput, VaultPickerModal, useVaultPicker } from '@/components/VaultInput';

const STEPS = [
  {
    id: 'welcome',
    title: 'Welcome to PEPA',
    description: 'PEPA helps you deploy services from Jira to Kubernetes. Let\'s set up your environment.',
  },
  {
    id: 'org',
    title: 'Name Your Organization',
    description: 'This is your company or team name. It will appear throughout the platform.',
  },
  {
    id: 'kubernetes',
    title: 'Add Your First Cluster',
    description: 'Connect a Kubernetes cluster to start deploying services. You\'ll be redirected to the Clusters page.',
    redirect: '/clusters',
  },
  {
    id: 'gitlab',
    title: 'Connect GitLab',
    description: 'Integrate with GitLab for source code and CI/CD pipelines.',
    type: 'gitlab' as ConnectionType,
    optional: true,
  },
  {
    id: 'jira',
    title: 'Connect Jira',
    description: 'Link Jira for issue tracking and project management.',
    type: 'jira' as ConnectionType,
    optional: true,
  },
  {
    id: 'done',
    title: 'You\'re All Set!',
    description: 'Your PEPA environment is ready. Start exploring!',
  },
];

export default function SetupPage() {
  const router = useRouter();
  const [step, setStep] = useState(0);
  const [formData, setFormData] = useState<Record<string, Record<string, string>>>({});
  const [error, setError] = useState<FriendlyError | null>(null);
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();

  const handleCancel = useCallback(() => {
    router.push('/');
  }, [router]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleCancel();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [handleCancel]);

  const currentStep = STEPS[step];

  const handleNext = async () => {
    setError(null);

    // Handle org setup step
    if (currentStep.id === 'org') {
      const orgData = formData['org'];
      if (!orgData?.name?.trim() || !orgData?.slug?.trim()) {
        setError({ message: 'Name and slug are required', hint: 'Please fill in both fields', raw: '' } as FriendlyError);
        return;
      }
      setBusy(true);
      try {
        await organization.setup({ name: orgData.name, slug: orgData.slug });
      } catch (err) {
        setError(friendlyError(err));
        setBusy(false);
        return;
      }
      setBusy(false);
      if (step < STEPS.length - 1) setStep(step + 1);
      return;
    }

    // Handle redirect steps (e.g., kubernetes -> clusters page)
    if ('redirect' in currentStep && currentStep.redirect) {
      router.push(currentStep.redirect);
      return;
    }

    if (currentStep.type) {
      // Two-stage flow: first click creates + tests the connection and shows
      // the result; second click moves to the next step.
      if (!testResult) {
        const data = formData[currentStep.id] || {};
        // Merge vault references into config
        const mergedConfig = { ...data };
        for (const [field, ref] of Object.entries(vaultRefs)) {
          if (ref) mergedConfig[field] = ref;
        }
        setBusy(true);
        try {
          const conn = await connectionsAPI.create({
            type: currentStep.type,
            name: data.name || `${currentStep.type} Connection`,
            config: mergedConfig,
          });
          try {
            const t = await connectionsAPI.test(conn.id);
            const ok = t.status === 'connected' || t.status === 'ok' || t.status === 'healthy';
            setTestResult({ ok, message: ok ? 'Connected successfully.' : `Created, but test failed: ${t.message || t.status}` });
          } catch (testErr) {
            const fe = friendlyError(testErr);
            setTestResult({ ok: false, message: `Created, but test failed: ${fe.message}` });
          }
        } catch (err) {
          setError(friendlyError(err));
        } finally {
          setBusy(false);
        }
        return;
      }
      setTestResult(null);
    }

    if (step < STEPS.length - 1) {
      setStep(step + 1);
    } else {
      router.push('/get-started');
    }
  };

  const handleSkip = () => {
    setError(null);
    setTestResult(null);
    if (step < STEPS.length - 1) {
      setStep(step + 1);
    }
  };

  const handleBack = () => {
    setError(null);
    setTestResult(null);
    if (step > 0) {
      setStep(step - 1);
    }
  };

  const updateFormData = (field: string, value: string) => {
    setFormData(prev => ({
      ...prev,
      [currentStep.id]: {
        ...(prev[currentStep.id] || {}),
        [field]: value,
      },
    }));
  };

  return (
    <div className="min-h-screen page-mesh-bg flex items-center justify-center p-4">
      <div className="max-w-2xl w-full page-animate">
        {/* Progress indicator */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-2">
            {STEPS.map((s, i) => (
              <div key={s.id} className="flex items-center">
                <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${
                  i <= step ? 'bg-[var(--accent)] text-white' : 'bg-[var(--border)] text-[var(--text-tertiary)]'
                }`}>
                  {i + 1}
                </div>
                {i < STEPS.length - 1 && (
                  <div className={`w-12 h-1 ${i < step ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'}`} />
                )}
              </div>
            ))}
          </div>
          <p className="text-sm text-[var(--text-secondary)] text-center">Step {step + 1} of {STEPS.length}</p>
        </div>

        {/* Card */}
        <div className="bg-[var(--surface)] rounded-2xl shadow-xl p-8 modern-card-hover">
          <div className="mb-6">
            <h1 className="text-3xl font-bold text-[var(--text-primary)] mb-2">{currentStep.title}</h1>
            <p className="text-[var(--text-secondary)]">{currentStep.description}</p>
          </div>

          {/* Step content */}
          {currentStep.id === 'welcome' && (
            <div className="space-y-4">
              <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
                <h3 className="font-semibold text-blue-500 mb-2">What is PEPA?</h3>
                <p className="text-sm text-blue-500">
                  PEPA (Platform Engineering & Pipeline Automator) is your CNCF-level Internal Developer Portal.
                  It helps you manage Kubernetes clusters, automate deployments, track issues, and integrate with your favorite tools.
                </p>
              </div>
              <div className="grid grid-cols-3 gap-4 text-center">
                <div className="p-4 bg-[var(--bg)] rounded-lg">
                  <div className="text-2xl mb-2">☸</div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">Kubernetes</div>
                  <div className="text-xs text-[var(--text-tertiary)] mt-1">Deploy anywhere</div>
                </div>
                <div className="p-4 bg-[var(--bg)] rounded-lg">
                  <div className="text-2xl mb-2">🦊</div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">GitLab</div>
                  <div className="text-xs text-[var(--text-tertiary)] mt-1">Source & CI/CD</div>
                </div>
                <div className="p-4 bg-[var(--bg)] rounded-lg">
                  <div className="text-2xl mb-2">📋</div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">Jira</div>
                  <div className="text-xs text-[var(--text-tertiary)] mt-1">Issue tracking</div>
                </div>
              </div>
            </div>
          )}

          {currentStep.id === 'org' && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Organization Name *</label>
                <input
                  type="text"
                  value={formData['org']?.name || ''}
                  onChange={e => {
                    const slug = e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
                    setFormData(prev => ({
                      ...prev,
                      'org': { name: e.target.value, slug: prev['org']?.slug || slug },
                    }));
                  }}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="e.g., Acme Corp"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Slug *</label>
                <input
                  type="text"
                  value={formData['org']?.slug || ''}
                  onChange={e => {
                    const slug = e.target.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
                    setFormData(prev => ({ ...prev, 'org': { ...prev['org'], slug } }));
                  }}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder="e.g., acme-corp"
                />
                <p className="text-xs text-[var(--text-tertiary)] mt-1">URL-friendly identifier for your organization</p>
              </div>
              <div className="bg-[var(--bg)] rounded-lg p-4 mt-4">
                <p className="text-sm text-[var(--text-secondary)]">
                  <strong>Workspaces:</strong> After setup, you can create isolated workspaces
                  (staging, production, dev) under this organization from Settings → Workspaces.
                </p>
              </div>
            </div>
          )}

          {currentStep.id === 'kubernetes' && (
            <div className="space-y-4">
              <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
                <p className="text-sm text-blue-500">
                  Clusters are managed on the dedicated Clusters page. There you can paste your kubeconfig,
                  and PEPA will auto-detect all clusters, fill in the fields, and let you edit everything before saving.
                </p>
              </div>
              <div className="text-center py-4">
                <div className="text-5xl mb-3">☸️</div>
                <p className="text-sm text-[var(--text-secondary)]">Click below to go to the Clusters page and add your first cluster.</p>
              </div>
            </div>
          )}

          {(currentStep.id === 'gitlab' || currentStep.id === 'jira') && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Connection Name</label>
                <input
                  type="text"
                  value={formData[currentStep.id]?.name || ''}
                  onChange={e => updateFormData('name', e.target.value)}
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder={currentStep.id === 'gitlab' ? 'GitLab' : 'Jira'}
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">URL *</label>
                <input
                  type="url"
                  value={formData[currentStep.id]?.url || ''}
                  onChange={e => updateFormData('url', e.target.value)}
                  required
                  className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
                  placeholder={currentStep.id === 'gitlab' ? 'https://gitlab.com' : 'https://your-domain.atlassian.net'}
                />
              </div>
              <VaultInput
                label="API Token"
                field="token"
                value={formData[currentStep.id]?.token || ''}
                onChange={v => updateFormData('token', v)}
                vaultRef={vaultRefs.token}
                onOpenVault={onOpenVaultPicker}
                onRemoveVault={removeVaultRef}
                placeholder="Your API token"
                required
              />
            </div>
          )}

          {currentStep.id === 'done' && (
            <div className="text-center py-8">
              <div className="text-6xl mb-4">🎉</div>
              <h3 className="text-xl font-semibold text-[var(--text-primary)] mb-2">Setup Complete!</h3>
              <p className="text-[var(--text-secondary)] mb-6">
                You can always add more connections later from the Connections page.
              </p>
              <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-lg p-4 max-w-md mx-auto">
                <p className="text-sm text-emerald-600">
                  <strong>Next:</strong> follow the guided tour to run your first workflow.
                </p>
              </div>
            </div>
          )}

          {/* Inline error / test result feedback */}
          {error && (
            <div className="mt-6 bg-red-500/10 border border-red-500/20 rounded-lg p-4">
              <p className="text-sm font-medium text-red-500 mb-1">Could not save this connection</p>
              {error.hint && <p className="text-sm text-red-400 mb-1">{error.hint}</p>}
              <p className="text-xs text-red-500">{error.message}</p>
            </div>
          )}
          {testResult && (
            <div className={`mt-6 border rounded-lg p-4 ${testResult.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-amber-500/10 border-amber-500/20'}`}>
              <p className={`text-sm font-medium ${testResult.ok ? 'text-emerald-600' : 'text-amber-600'}`}>
                {testResult.ok ? '✓ ' : '⚠ '}{testResult.message}
              </p>
              <p className={`text-xs mt-1 ${testResult.ok ? 'text-emerald-500' : 'text-amber-500'}`}>
                {testResult.ok
                  ? 'Click Continue to move to the next step.'
                  : 'You can continue anyway and fix it later on the Connections page, or correct the details now.'}
              </p>
            </div>
          )}

          {/* Actions */}
          <div className="flex gap-3 mt-8">
            <button
              onClick={handleCancel}
              className="px-6 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--bg)] transition-colors"
            >
              Cancel
            </button>
            {step > 0 && currentStep.id !== 'welcome' && (
              <button
                onClick={handleBack}
                className="px-6 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--bg)] transition-colors"
              >
                Back
              </button>
            )}
            {currentStep.optional && currentStep.id !== 'done' && (
              <button
                onClick={handleSkip}
                className="px-6 py-2 text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors"
              >
                Skip for now
              </button>
            )}
            <button
              onClick={handleNext}
              disabled={busy}
              className="flex-1 py-2 bg-[var(--accent)] text-white rounded-lg hover:opacity-90 disabled:opacity-50 transition-colors"
            >
              {busy
                ? 'Saving...'
                : currentStep.id === 'done'
                  ? 'Go to Guided Tour'
                  : 'redirect' in currentStep && currentStep.redirect
                    ? `Go to ${currentStep.title.includes('Cluster') ? 'Clusters' : 'Page'}`
                    : testResult
                      ? 'Continue'
                      : currentStep.type
                        ? 'Save & Test Connection'
                        : 'Continue'}
            </button>
          </div>
        </div>
      </div>
      {VaultPicker}
    </div>
  );
}
