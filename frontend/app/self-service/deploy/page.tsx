'use client';
import { useState, useCallback, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { selfService, type Environment, type ProjectDetection, type SelfServiceDeployment } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import { useEscapeKey } from '@/hooks/useEscapeKey';

export default function SelfServiceDeployPage() {
  return (
    <PermissionGuard resource="self_service" action="create">
      <SelfServiceDeployContent />
    </PermissionGuard>
  );
}

function SelfServiceDeployContent() {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [gitRepoUrl, setGitRepoUrl] = useState('');
  const [gitBranch, setGitBranch] = useState('main');
  const [selectedEnv, setSelectedEnv] = useState<string>('');
  const [blueprintType, setBlueprintType] = useState('auto');
  const [detection, setDetection] = useState<ProjectDetection | null>(null);
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [detecting, setDetecting] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [myDeployments, setMyDeployments] = useState<SelfServiceDeployment[]>([]);

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 4000);
  }, []);

  // Load environments
  useEffect(() => {
    selfService.environments()
      .then(res => setEnvironments(res.environments || []))
      .catch(() => {});
  }, []);

  // Load my deployments
  useEffect(() => {
    selfService.list()
      .then(res => setMyDeployments(res.deployments || []))
      .catch(() => {});
  }, []);

  const handleDetect = useCallback(async () => {
    if (!gitRepoUrl.trim()) return;
    setDetecting(true);
    setError(null);
    try {
      const result = await selfService.detect(gitRepoUrl, gitBranch);
      setDetection(result);
      setBlueprintType(result.suggested_blueprint || 'docker_compose');
      setStep(2);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Detection failed');
    } finally {
      setDetecting(false);
    }
  }, [gitRepoUrl, gitBranch]);

  const handleDeploy = useCallback(async () => {
    if (!selectedEnv) {
      setError('Please select an environment');
      return;
    }
    setDeploying(true);
    setError(null);
    try {
      const result = await selfService.deploy({
        git_repo_url: gitRepoUrl,
        git_branch: gitBranch,
        environment_id: selectedEnv,
        blueprint_type: blueprintType,
      });
      showToast(`Deployment started! ID: ${result.id.slice(0, 8)}`, 'success');
      setStep(4);
      // Refresh deployments list
      const res = await selfService.list();
      setMyDeployments(res.deployments || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deployment failed');
      showToast('Deployment failed', 'error');
    } finally {
      setDeploying(false);
    }
  }, [gitRepoUrl, gitBranch, selectedEnv, blueprintType, showToast]);

  const handleCancel = useCallback(async (id: string) => {
    try {
      await selfService.cancel(id);
      showToast('Deployment cancelled', 'success');
      const res = await selfService.list();
      setMyDeployments(res.deployments || []);
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Cancel failed', 'error');
    }
  }, [showToast]);

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
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Deploy Application</h1>
            <p className="page-subtitle-modern">
              Deploy any application from a Git repository to your environment
            </p>
          </div>
        </div>

        {/* Progress Steps */}
        <div className="flex items-center gap-2">
          {['Source', 'Configure', 'Environment', 'Deploy'].map((label, i) => (
            <div key={label} className="flex items-center gap-2">
              <div className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[12px] font-medium transition-colors ${
                step === i + 1 ? 'bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/20' :
                step > i + 1 ? 'bg-[var(--success)]/10 text-[var(--success)]' :
                'bg-[var(--bg)] text-[var(--text-tertiary)]'
              }`}>
                <span className={`w-5 h-5 rounded-full flex items-center justify-center text-[10px] ${
                  step > i + 1 ? 'bg-[var(--success)] text-white' :
                  step === i + 1 ? 'bg-[var(--accent)] text-white' :
                  'bg-[var(--border)] text-[var(--text-tertiary)]'
                }`}>
                  {step > i + 1 ? '✓' : i + 1}
                </span>
                {label}
              </div>
              {i < 3 && <div className="w-8 h-px bg-[var(--border)]" />}
            </div>
          ))}
        </div>

        {error && (
          <div className="card p-3 border-[var(--danger)]/20 bg-[var(--danger)]/5">
            <p className="text-[12px] text-[var(--danger)]">{error}</p>
          </div>
        )}

        {/* Step 1: Source */}
        {step === 1 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Git Repository</h3>
            </div>
            <div className="card-body space-y-4">
              <div>
                <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Repository URL</label>
                <input
                  value={gitRepoUrl}
                  onChange={e => setGitRepoUrl(e.target.value)}
                  placeholder="https://github.com/org/repo.git"
                  className="input text-[13px] w-full font-mono"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  Supports GitHub, GitLab, Bitbucket, and self-hosted Git repositories
                </p>
              </div>
              <div>
                <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Branch</label>
                <input
                  value={gitBranch}
                  onChange={e => setGitBranch(e.target.value)}
                  placeholder="main"
                  className="input text-[13px] w-full font-mono"
                />
              </div>
              <div className="flex justify-end pt-2">
                <button
                  onClick={handleDetect}
                  disabled={!gitRepoUrl.trim() || detecting}
                  className="btn btn-primary btn-sm"
                >
                  {detecting ? (
                    <>
                      <div className="loading-spinner w-3 h-3 mr-1.5" />
                      Analyzing...
                    </>
                  ) : (
                    <>
                      Analyze Repository
                      <svg className="w-4 h-4 ml-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                      </svg>
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Step 2: Configure */}
        {step === 2 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Project Configuration</h3>
            </div>
            <div className="card-body space-y-4">
              {detection && (
                <div className="p-3 rounded-lg bg-[var(--bg)] border border-[var(--border)]">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-[12px] font-medium text-[var(--text-primary)]">Detected:</span>
                    <span className="text-[11px] px-2 py-0.5 rounded-full bg-[var(--accent)]/10 text-[var(--accent)] capitalize">
                      {detection.detected_type}
                    </span>
                    <span className="text-[11px] text-[var(--text-tertiary)]">
                      ({Math.round(detection.confidence * 100)}% confidence)
                    </span>
                  </div>
                  {detection.indicators.length > 0 && (
                    <ul className="text-[11px] text-[var(--text-tertiary)] space-y-0.5">
                      {detection.indicators.map((ind, i) => (
                        <li key={i}>• {ind}</li>
                      ))}
                    </ul>
                  )}
                </div>
              )}

              <div>
                <label className="text-[12px] font-medium text-[var(--text-secondary)] mb-1 block">Blueprint Type</label>
                <select
                  value={blueprintType}
                  onChange={e => setBlueprintType(e.target.value)}
                  className="select text-[13px] w-full"
                >
                  <option value="auto">Auto-detect</option>
                  <option value="docker_compose">Docker Compose</option>
                  <option value="helm">Helm Chart</option>
                  <option value="raw_k8s">Raw Kubernetes Manifests</option>
                </select>
              </div>

              <div className="flex justify-between pt-2">
                <button onClick={() => setStep(1)} className="btn btn-secondary btn-sm">
                  <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                  </svg>
                  Back
                </button>
                <button onClick={() => setStep(3)} className="btn btn-primary btn-sm">
                  Next
                  <svg className="w-4 h-4 ml-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Step 3: Environment */}
        {step === 3 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Target Environment</h3>
            </div>
            <div className="card-body space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {environments.map(env => (
                  <button
                    key={env.id}
                    onClick={() => setSelectedEnv(env.id)}
                    className={`p-3 rounded-lg border text-left transition-all ${
                      selectedEnv === env.id
                        ? 'border-[var(--accent)] bg-[var(--accent)]/5'
                        : 'border-[var(--border)] hover:border-[var(--accent)]/50 hover:bg-[var(--bg)]'
                    }`}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      <span className="w-3 h-3 rounded-full" style={{ backgroundColor: env.color || '#6B7280' }} />
                      <span className="text-[13px] font-medium text-[var(--text-primary)]">{env.name}</span>
                    </div>
                    {env.description && (
                      <p className="text-[11px] text-[var(--text-tertiary)] line-clamp-2">{env.description}</p>
                    )}
                    <div className="flex items-center gap-2 mt-1.5">
                      {env.type && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)] capitalize">{env.type}</span>
                      )}
                      {env.cluster && (
                        <span className="text-[10px] font-mono text-[var(--text-tertiary)]">{env.cluster}</span>
                      )}
                    </div>
                  </button>
                ))}
              </div>

              {environments.length === 0 && (
                <div className="text-center py-6">
                  <p className="text-[12px] text-[var(--text-tertiary)]">No environments available</p>
                </div>
              )}

              <div className="flex justify-between pt-2">
                <button onClick={() => setStep(2)} className="btn btn-secondary btn-sm">
                  <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                  </svg>
                  Back
                </button>
                <button
                  onClick={handleDeploy}
                  disabled={!selectedEnv || deploying}
                  className="btn btn-primary btn-sm"
                >
                  {deploying ? (
                    <>
                      <div className="loading-spinner w-3 h-3 mr-1.5" />
                      Deploying...
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
                      </svg>
                      Deploy
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Step 4: Result */}
        {step === 4 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-body text-center py-8">
              <div className="w-16 h-16 rounded-full bg-[var(--success)]/10 flex items-center justify-center mx-auto mb-4">
                <svg className="w-8 h-8 text-[var(--success)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <h3 className="text-[16px] font-semibold text-[var(--text-primary)] mb-1">Deployment Started!</h3>
              <p className="text-[12px] text-[var(--text-tertiary)] mb-4">
                Your application is being deployed to the selected environment
              </p>
              <div className="flex items-center justify-center gap-3">
                <button onClick={() => router.push('/deployments')} className="btn btn-primary btn-sm">
                  View Deployments
                </button>
                <button onClick={() => { setStep(1); setGitRepoUrl(''); setDetection(null); setSelectedEnv(''); }} className="btn btn-secondary btn-sm">
                  Deploy Another
                </button>
              </div>
            </div>
          </div>
        )}

        {/* My Deployments */}
        {myDeployments.length > 0 && (
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">My Deployments</h3>
            </div>
            <div className="card-body pt-0">
              <div className="space-y-2">
                {myDeployments.slice(0, 5).map(dep => (
                  <div key={dep.id} className="flex items-center gap-3 p-2.5 rounded-lg bg-[var(--bg)] border border-[var(--border-light)]">
                    <div className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                      dep.status === 'deployed' ? 'bg-[var(--success)]' :
                      dep.status === 'failed' ? 'bg-[var(--danger)]' :
                      dep.status === 'cancelled' ? 'bg-[var(--text-tertiary)]' :
                      'bg-[var(--warning)] animate-pulse'
                    }`} />
                    <div className="flex-1 min-w-0">
                      <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">
                        {dep.git_repo_url.split('/').pop()?.replace('.git', '') || dep.git_repo_url}
                      </p>
                      <p className="text-[11px] text-[var(--text-tertiary)]">
                        {dep.git_branch} — {new Date(dep.created_at).toLocaleString()}
                      </p>
                    </div>
                    <span className={`text-[10px] px-2 py-0.5 rounded-full capitalize ${
                      dep.status === 'deployed' ? 'bg-[var(--success)]/10 text-[var(--success)]' :
                      dep.status === 'failed' ? 'bg-[var(--danger)]/10 text-[var(--danger)]' :
                      dep.status === 'cancelled' ? 'bg-[var(--text-tertiary)]/10 text-[var(--text-tertiary)]' :
                      'bg-[var(--warning)]/10 text-[var(--warning)]'
                    }`}>
                      {dep.status}
                    </span>
                    {(dep.status === 'pending' || dep.status === 'deploying') && (
                      <button
                        onClick={() => handleCancel(dep.id)}
                        className="text-[11px] text-[var(--danger)] hover:underline"
                      >
                        Cancel
                      </button>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
