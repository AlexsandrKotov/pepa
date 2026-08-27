'use client';

import { useState, useEffect, useCallback } from 'react';
import { gitops, teamWorkflows, environments as envApi, workflows as workflowsApi, type WorkflowMR, type TeamWorkflowConfig, type TeamWorkflowStage, type TimelineEvent, type Environment, type Workflow } from '@/lib/api';

type GitOpsTeamConfig = TeamWorkflowConfig['gitops'];
type VerifyTeamConfig = TeamWorkflowConfig['verification'];
import { Toast } from '@/components/Interactive';
import ConceptHelp from '@/components/ConceptHelp';
import { useEscapeKey } from '@/hooks/useEscapeKey';

const FALLBACK_STAGES: TeamWorkflowStage[] = [
  { key: 'dev', label: 'Development', color: 'bg-green-500', auto_promote: true, requires_approval: false },
  { key: 'testing', label: 'Testing', color: 'bg-blue-500', auto_promote: false, requires_approval: false },
  { key: 'staging', label: 'Staging', color: 'bg-yellow-500', auto_promote: false, requires_approval: true },
  { key: 'production', label: 'Production', color: 'bg-red-500', auto_promote: false, requires_approval: true },
];

/** Map a hex colour to the closest Tailwind bg-* utility. */
function hexToTailwind(hex: string): string {
  const map: Record<string, string> = {
    '#10B981': 'bg-emerald-500', '#059669': 'bg-emerald-600',
    '#3B82F6': 'bg-blue-500', '#2563EB': 'bg-blue-600',
    '#F59E0B': 'bg-amber-500', '#D97706': 'bg-amber-600',
    '#EF4444': 'bg-red-500', '#DC2626': 'bg-red-600',
    '#8B5CF6': 'bg-violet-500', '#7C3AED': 'bg-violet-600',
    '#EC4899': 'bg-pink-500', '#F97316': 'bg-orange-500',
    '#6B7280': 'bg-gray-500', '#14B8A6': 'bg-teal-500',
  };
  return map[hex.toUpperCase()] || 'bg-gray-500';
}

/** Convert Environment records to workflow stages. */
function envsToStages(envs: Environment[]): TeamWorkflowStage[] {
  if (envs.length === 0) return FALLBACK_STAGES;
  return envs.map((e) => ({
    key: e.slug || e.name.toLowerCase().replace(/\s+/g, '-'),
    label: e.name,
    color: e.color ? hexToTailwind(e.color) : 'bg-gray-500',
    auto_promote: false,
    requires_approval: (e.slug || '').toLowerCase() === 'production' || (e.slug || '').toLowerCase() === 'staging',
  }));
}

const DEFAULT_TEAMS = ['platform-team', 'backend-team', 'frontend-team'];
const DEFAULT_GITOPS: GitOpsTeamConfig = { provider: 'argocd', repo_url: '', branch: 'main', path: 'manifests/' };
const DEFAULT_CI = { provider: 'gitlab', pipeline: '.gitlab-ci.yml' };
const DEFAULT_VERIFY: VerifyTeamConfig = { checks: ['health-check'] };
const VERIFY_CHECK_OPTIONS = ['health-check', 'smoke-test', 'metrics', 'logs'];

export default function WorkflowsPage() {
  const [mrs, setMrs] = useState<WorkflowMR[]>([]);
  const [stages, setStages] = useState<TeamWorkflowStage[]>(FALLBACK_STAGES);
  const [loading, setLoading] = useState(true);
  const [showDeploy, setShowDeploy] = useState(false);
  const [showTeamConfig, setShowTeamConfig] = useState(false);
  const [showAddTeam, setShowAddTeam] = useState(false);
  const [selectedMR, setSelectedMR] = useState<WorkflowMR | null>(null);
  const [timeline, setTimeline] = useState<TimelineEvent[]>([]);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Team workflow state
  const [teams, setTeams] = useState<string[]>(DEFAULT_TEAMS);
  const [selectedTeam, setSelectedTeam] = useState<string>(DEFAULT_TEAMS[0]);
  const [teamConfig, setTeamConfig] = useState<TeamWorkflowConfig | null>(null);
  const [editStages, setEditStages] = useState<TeamWorkflowStage[]>(FALLBACK_STAGES);
  const [editGitOps, setEditGitOps] = useState<GitOpsTeamConfig>(DEFAULT_GITOPS);
  const [editChecks, setEditChecks] = useState<string[]>(DEFAULT_VERIFY.checks);
  const [busyId, setBusyId] = useState<string | null>(null);

  // Load teams once from the DB (merged with defaults).
  useEffect(() => {
    teamWorkflows.list()
      .then(({ workflows }) => {
        const names = (workflows || []).map(w => w.team_name).filter(Boolean);
        setTeams(Array.from(new Set([...DEFAULT_TEAMS, ...names])));
      })
      .catch(() => setTeams(DEFAULT_TEAMS));
  }, []);

  const loadData = useCallback(async () => {
    try {
      const [mrData, twData, envData] = await Promise.all([
        gitops.mrs(selectedTeam).catch(() => ({ merge_requests: [], total: 0 })),
        teamWorkflows.get(selectedTeam).catch(() => null),
        envApi.list().catch(() => ({ environments: [], total: 0 })),
      ]);
      setMrs(mrData.merge_requests || []);

      // Priority: team-specific config > environments from DB > fallback
      if (twData && twData.stages && twData.stages.length > 0) {
        setStages(twData.stages);
        setTeamConfig(twData);
      } else {
        const envStages = envsToStages(envData.environments || []);
        setStages(envStages);
        setTeamConfig(null);
      }
    } catch (err) {
      console.error('Failed to load workflow data:', err);
    } finally {
      setLoading(false);
    }
  }, [selectedTeam]);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 15000);
    return () => clearInterval(interval);
  }, [loadData]);

  const handleVerify = async (mr: WorkflowMR) => {
    try {
      const result = await gitops.verify(mr.id);
      setToast({
        message: `Verification ${result.verification_status}: ${result.checks.length} checks completed`,
        type: result.verification_status === 'verified' ? 'success' : 'error',
      });
    } catch {
      setToast({ message: 'Verification failed', type: 'error' });
    }
  };

  const handleViewTimeline = async (mr: WorkflowMR) => {
    setSelectedMR(mr);
    try {
      const data = await gitops.timeline(mr.id);
      setTimeline(data.events || []);
    } catch {
      setTimeline([]);
    }
  };

  const openTeamConfig = () => {
    setEditStages(stages);
    setEditGitOps(teamConfig?.gitops || DEFAULT_GITOPS);
    setEditChecks(teamConfig?.verification?.checks?.length ? teamConfig.verification.checks : DEFAULT_VERIFY.checks);
    setShowTeamConfig(true);
  };

  const handleSaveTeamConfig = async () => {
    try {
      const config: Partial<TeamWorkflowConfig> = {
        team_name: selectedTeam,
        stages: editStages,
        gitops: editGitOps,
        ci: teamConfig?.ci || DEFAULT_CI,
        verification: { checks: editChecks },
      };
      await teamWorkflows.save(selectedTeam, config);
      setStages(editStages);
      setShowTeamConfig(false);
      setToast({ message: `Workflow config saved for ${selectedTeam}`, type: 'success' });
      loadData();
    } catch {
      setToast({ message: 'Failed to save workflow config', type: 'error' });
    }
  };

  const handlePromote = async (mr: WorkflowMR) => {
    setBusyId(mr.id);
    try {
      const result = await gitops.promote(mr.id);
      if (result.awaiting_approval) {
        setToast({ message: result.message || `Promotion to ${result.target_stage} requires approval`, type: 'success' });
      } else {
        setToast({ message: result.promoted_to ? `Promoted to ${result.promoted_to}` : 'Promoted', type: 'success' });
      }
      loadData();
    } catch {
      setToast({ message: 'Promotion failed', type: 'error' });
    } finally {
      setBusyId(null);
    }
  };

  const handleApprove = async (mr: WorkflowMR) => {
    setBusyId(mr.id);
    try {
      const result = await gitops.approve(mr.id);
      setToast({ message: `Approved — deploying to ${result.promoted_to}`, type: 'success' });
      loadData();
    } catch {
      setToast({ message: 'Approval failed', type: 'error' });
    } finally {
      setBusyId(null);
    }
  };

  const handleRollback = async (mr: WorkflowMR) => {
    setBusyId(mr.id);
    try {
      await gitops.rollback(mr.id);
      setToast({ message: 'Rollback initiated', type: 'success' });
      loadData();
    } catch {
      setToast({ message: 'Rollback failed', type: 'error' });
    } finally {
      setBusyId(null);
    }
  };

  const addStage = () => {
    setEditStages([...editStages, { key: `stage-${editStages.length + 1}`, label: `Stage ${editStages.length + 1}`, color: 'bg-gray-500', auto_promote: false, requires_approval: false }]);
  };

  const removeStage = (idx: number) => {
    setEditStages(editStages.filter((_, i) => i !== idx));
  };

  const updateStage = (idx: number, field: keyof TeamWorkflowStage, value: string | boolean) => {
    setEditStages(editStages.map((s, i) => i === idx ? { ...s, [field]: value } : s));
  };

  const statusBadge = (s: string) => {
    switch (s) {
      case 'deployed': case 'promoted': return 'badge-success';
      case 'pending': case 'syncing': return 'badge-warning';
      case 'awaiting_approval': return 'bg-violet-500/10 text-violet-600';
      case 'failed': return 'badge-danger';
      case 'rolled_back': return 'bg-orange-500/10 text-orange-600';
      default: return 'badge-default';
    }
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <h1 className="page-title-modern">GitOps Workflow</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-tertiary)]">Loading pipeline...</p>
          </div>
        </div>
      </div>
    );
  }

  const mrsByStage = (stage: string) => mrs.filter(mr => mr.stage === stage);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Header */}
      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">GitOps Workflow</h1>
              <ConceptHelp term="gitops" />
            </div>
            <p className="page-subtitle-modern">
              {selectedTeam} &middot; {stages.length} stages &middot; {mrs.length} merge requests
            </p>
          </div>
        <div className="flex gap-2">
          <button onClick={loadData} className="btn btn-secondary btn-sm">Refresh</button>
          <button onClick={openTeamConfig} className="btn btn-secondary btn-sm">
            Configure Workflow
          </button>
          <button onClick={() => setShowDeploy(true)} className="btn btn-primary btn-sm">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Manual Deploy
          </button>
        </div>
      </div>
      </div>

      {/* Team Selector */}
      <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-[12px] text-[var(--text-tertiary)]">Team:</span>
          <div className="flex gap-1 bg-[var(--border-light)] rounded-lg p-0.5">
            {teams.map(team => (
              <button
                key={team}
                onClick={() => setSelectedTeam(team)}
                className={`px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${selectedTeam === team ? 'bg-white text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
              >
                {team}
              </button>
            ))}
          </div>
          <button
            onClick={() => setShowAddTeam(true)}
            className="text-[12px] text-[var(--accent)] hover:underline"
          >
            + Add Team
          </button>
        </div>
      </div>

      {/* Pipeline Stages */}
      <div className="flex items-center gap-2 mb-2 overflow-x-auto page-animate-up page-delay-2">
        {stages.map((stage, i) => (
          <div key={stage.key} className="flex items-center gap-2 flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-1">
              <div className={`w-2 h-2 rounded-full ${stage.color}`} />
              <span className="text-[12px] font-medium text-[var(--text-primary)] truncate">{stage.label}</span>
              <span className="text-[11px] text-[var(--text-tertiary)]">({mrsByStage(stage.key).length})</span>
              {stage.requires_approval && (
                <span className="text-[9px] px-1 py-0.5 bg-yellow-500/10 text-yellow-600 rounded">approval</span>
              )}
            </div>
            {i < stages.length - 1 && (
              <svg className="w-4 h-4 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
              </svg>
            )}
          </div>
        ))}
      </div>

      {/* Pipeline Board */}
      {mrs.length === 0 ? (
        <div className="card card-body text-center py-12 page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <p className="text-[13px] font-medium text-[var(--text-primary)]">No deployments for {selectedTeam}</p>
          <p className="text-[12px] text-[var(--text-tertiary)] mt-1">
            Use <button onClick={() => setShowDeploy(true)} className="text-[var(--accent)] hover:underline">Manual Deploy</button> or run a workflow from the{' '}
            <a href="/automation" className="text-[var(--accent)] hover:underline">Automation</a> page to create the first deployment.
          </p>
        </div>
      ) : (
      <div className={`grid gap-4`} style={{ gridTemplateColumns: `repeat(${stages.length}, minmax(0, 1fr))` }}>
        {stages.map(stage => {
          const stageMrsss = mrsByStage(stage.key);
          return (
            <div key={stage.key} className="space-y-3">
              {stageMrsss.length === 0 ? (
                <div className="card card-body text-center py-8">
                  <p className="text-[12px] text-[var(--text-tertiary)]">No deployments</p>
                </div>
              ) : (
                stageMrsss.map(mr => (
                  <div key={mr.id} className="card">
                    <div className="card-body space-y-2">
                      <div className="flex items-center justify-between">
                        <span className={`badge ${statusBadge(mr.status)}`}>{mr.status}</span>
                        <span className="text-[11px] text-[var(--text-tertiary)]">{formatTime(mr.created_at)}</span>
                      </div>
                      {mr.jira_issue_key && (
                        <p className="text-[13px] font-medium text-[var(--text-primary)]">{mr.jira_issue_key}</p>
                      )}
                      {mr.jira_summary && (
                        <p className="text-[12px] text-[var(--text-secondary)] line-clamp-2">{mr.jira_summary}</p>
                      )}
                      <div className="text-[11px] text-[var(--text-tertiary)]">
                        <span className="font-mono">{mr.image_tag || '-'}</span>
                      </div>
                      {mr.project_name && (
                        <p className="text-[11px] text-[var(--text-tertiary)]">{mr.project_name}</p>
                      )}
                      <div className="flex flex-wrap gap-x-2 gap-y-1 pt-1">
                        <button onClick={() => handleViewTimeline(mr)} className="text-[11px] text-[var(--accent)] hover:underline">
                          Timeline
                        </button>
                        {mr.status === 'deployed' && stage.key !== stages[stages.length - 1]?.key && (
                          <button onClick={() => handlePromote(mr)} disabled={busyId === mr.id} className="text-[11px] text-[var(--accent)] hover:underline disabled:opacity-50">
                            {busyId === mr.id ? '...' : 'Promote'}
                          </button>
                        )}
                        {mr.status === 'awaiting_approval' && (
                          <button onClick={() => handleApprove(mr)} disabled={busyId === mr.id} className="text-[11px] font-medium text-violet-600 hover:underline disabled:opacity-50">
                            {busyId === mr.id ? '...' : 'Approve'}
                          </button>
                        )}
                        {(mr.status === 'deployed' || mr.status === 'promoted') && (
                          <button onClick={() => handleVerify(mr)} className="text-[11px] text-green-600 hover:underline">
                            Verify
                          </button>
                        )}
                        {(mr.status === 'deployed' || mr.status === 'promoted' || mr.status === 'awaiting_approval') && (
                          <button onClick={() => handleRollback(mr)} disabled={busyId === mr.id} className="text-[11px] text-red-500 hover:underline disabled:opacity-50">
                            Rollback
                          </button>
                        )}
                        {mr.mr_url && (
                          <a href={mr.mr_url} target="_blank" rel="noopener noreferrer" className="text-[11px] text-[var(--accent)] hover:underline">
                            MR
                          </a>
                        )}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          );
        })}
      </div>
      )}

      {/* Team Workflow Config Modal */}
      {showTeamConfig && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => setShowTeamConfig(false)}>
          <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
              <div>
                <h3 className="text-[14px] font-medium text-[var(--text-primary)]">
                  Workflow Configuration: {selectedTeam}
                </h3>
                <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">Pipeline stages, GitOps source and verification for this team</p>
              </div>
              <button onClick={() => setShowTeamConfig(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="p-4 space-y-4">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <h4 className="text-[13px] font-medium text-[var(--text-primary)]">Pipeline Stages</h4>
                  <button onClick={addStage} className="text-[12px] text-[var(--accent)] hover:underline">+ Add Stage</button>
                </div>
                {editStages.map((stage, idx) => (
                  <div key={idx} className="flex items-center gap-3 p-3 bg-[var(--bg)] rounded-lg">
                    <div className={`w-3 h-3 rounded-full ${stage.color}`} />
                    <div className="flex-1 grid grid-cols-2 gap-2">
                      <input
                        value={stage.label}
                        onChange={e => updateStage(idx, 'label', e.target.value)}
                        className="input !py-1.5 !text-[12px]"
                        placeholder="Stage label"
                      />
                      <input
                        value={stage.key}
                        onChange={e => updateStage(idx, 'key', e.target.value)}
                        className="input !py-1.5 !text-[12px] font-mono"
                        placeholder="stage-key"
                      />
                    </div>
                    <div className="flex flex-col gap-1">
                      <label className="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
                        <input
                          type="checkbox"
                          checked={stage.requires_approval}
                          onChange={e => updateStage(idx, 'requires_approval', e.target.checked)}
                          className="rounded"
                        />
                        Approval
                      </label>
                      <label className="flex items-center gap-1.5 text-[11px] text-[var(--text-secondary)]">
                        <input
                          type="checkbox"
                          checked={stage.auto_promote}
                          onChange={e => updateStage(idx, 'auto_promote', e.target.checked)}
                          className="rounded"
                        />
                        Auto
                      </label>
                    </div>
                    <button
                      onClick={() => removeStage(idx)}
                      className="text-[11px] text-red-500 hover:text-red-400 p-1"
                      disabled={editStages.length <= 2}
                    >
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867.127A4.962 4.962 0 0116 2.5H8a4.962 4.962 0 00-2.133.627L5 7m4 4v6m4-6v6m2-10h2a2 2 0 012 2v8a2 2 0 01-2 2H7a2 2 0 01-2-2V9a2 2 0 012-2h2" />
                      </svg>
                    </button>
                  </div>
                ))}
              </div>

              {/* GitOps source */}
              <div className="space-y-2 pt-2 border-t border-[var(--border)]">
                <h4 className="text-[13px] font-medium text-[var(--text-primary)]">GitOps Source</h4>
                <div className="grid grid-cols-2 gap-2">
                  <div>
                    <label className="label">Provider</label>
                    <select value={editGitOps.provider} onChange={e => setEditGitOps({ ...editGitOps, provider: e.target.value })} className="input">
                      <option value="argocd">ArgoCD</option>
                      <option value="flux">Flux</option>
                      <option value="manual">Manual</option>
                    </select>
                  </div>
                  <div>
                    <label className="label">Branch</label>
                    <input value={editGitOps.branch} onChange={e => setEditGitOps({ ...editGitOps, branch: e.target.value })} className="input" placeholder="main" />
                  </div>
                </div>
                <div>
                  <label className="label">Manifest Repo URL</label>
                  <input value={editGitOps.repo_url} onChange={e => setEditGitOps({ ...editGitOps, repo_url: e.target.value })} className="input" placeholder="https://git.example.com/infra/manifests.git" />
                </div>
                <div>
                  <label className="label">Path</label>
                  <input value={editGitOps.path} onChange={e => setEditGitOps({ ...editGitOps, path: e.target.value })} className="input" placeholder="manifests/" />
                </div>
              </div>

              {/* Verification checks */}
              <div className="space-y-2 pt-2 border-t border-[var(--border)]">
                <h4 className="text-[13px] font-medium text-[var(--text-primary)]">Verification Checks</h4>
                <div className="flex flex-wrap gap-2">
                  {VERIFY_CHECK_OPTIONS.map(check => (
                    <label key={check} className="flex items-center gap-1.5 text-[12px] text-[var(--text-secondary)] px-2 py-1 bg-[var(--bg)] rounded-lg">
                      <input
                        type="checkbox"
                        checked={editChecks.includes(check)}
                        onChange={e => setEditChecks(e.target.checked ? [...editChecks, check] : editChecks.filter(c => c !== check))}
                        className="rounded"
                      />
                      {check}
                    </label>
                  ))}
                </div>
              </div>

              <div className="flex justify-end gap-2 pt-2 border-t border-[var(--border)]">
                <button onClick={() => setShowTeamConfig(false)} className="btn btn-ghost">Cancel</button>
                <button onClick={handleSaveTeamConfig} className="btn btn-primary">Save Workflow</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Timeline Modal */}
      {selectedMR && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => setSelectedMR(null)}>
          <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-lg max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">
                Deployment Timeline {selectedMR.jira_issue_key && `— ${selectedMR.jira_issue_key}`}
              </h3>
              <button onClick={() => setSelectedMR(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="p-4 space-y-3">
              {timeline.length === 0 ? (
                <p className="text-[13px] text-[var(--text-tertiary)] text-center py-6">No timeline events</p>
              ) : (
                timeline.map((event, i) => (
                  <div key={i} className="flex items-start gap-3">
                    <div className="w-6 h-6 rounded-full bg-[var(--border-light)] flex items-center justify-center text-[11px] text-[var(--text-secondary)] shrink-0 mt-0.5">
                      {event.status === 'completed' ? '\u2713' : event.status === 'in_progress' ? '\u25CB' : '\u2022'}
                    </div>
                    <div>
                      <p className="text-[13px] text-[var(--text-primary)]">{event.label}</p>
                      <p className="text-[11px] text-[var(--text-tertiary)]">{event.detail}</p>
                      <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">
                        {new Date(event.timestamp).toLocaleString()} &middot; {event.stage}
                      </p>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )}

      {/* Add Team Modal */}
      {showAddTeam && (
        <AddTeamModal
          existingTeams={teams}
          onClose={() => setShowAddTeam(false)}
          onCreated={(name) => {
            setTeams([...teams, name]);
            setSelectedTeam(name);
            setShowAddTeam(false);
            setToast({ message: `Team ${name} saved with default workflow`, type: 'success' });
          }}
          onError={(message) => setToast({ message, type: 'error' })}
        />
      )}

      {/* Manual Deploy Modal */}
      {showDeploy && <ManualDeployModal team={selectedTeam} stages={stages} onClose={() => setShowDeploy(false)} onDeployed={() => { setShowDeploy(false); loadData(); }} />}
      </div>
    </div>
  );
}

function AddTeamModal({ existingTeams, onClose, onCreated, onError }: { existingTeams: string[]; onClose: () => void; onCreated: (name: string) => void; onError: (message: string) => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  useEscapeKey(onClose);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) { setError('Team name is required'); return; }
    if (existingTeams.includes(trimmed)) { setError('Team already exists'); return; }
    setSaving(true);
    setError('');
    try {
      // Persist in DB with default stages so the config survives restarts.
      await teamWorkflows.save(trimmed, { team_name: trimmed, stages: FALLBACK_STAGES });
      onCreated(trimmed);
    } catch {
      onError('Failed to save team config');
      setError('Failed to save team config');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-md" onClick={e => e.stopPropagation()}>
        <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
          <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Add Team</h3>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="p-4 space-y-4">
            {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}
            <div>
              <label className="label">Team Name *</label>
              <input
                type="text"
                value={name}
                onChange={e => { setName(e.target.value); setError(''); }}
                className="input"
                placeholder="e.g., qa-team"
                autoFocus
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" onClick={onClose} className="btn btn-ghost">Cancel</button>
              <button type="submit" disabled={!name.trim() || saving} className="btn btn-primary">{saving ? 'Saving...' : 'Add Team'}</button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}

function ManualDeployModal({ team, stages, onClose, onDeployed }: { team: string; stages: TeamWorkflowStage[]; onClose: () => void; onDeployed: () => void }) {
  const [mode, setMode] = useState<'direct' | 'pipeline'>('direct');
  const [pipelines, setPipelines] = useState<Workflow[]>([]);
  const [pipelineId, setPipelineId] = useState('');
  const [imageTag, setImageTag] = useState('');
  const [imageRepo, setImageRepo] = useState('');
  const [projectName, setProjectName] = useState('');
  const [jiraKey, setJiraKey] = useState('');
  const [jiraSummary, setJiraSummary] = useState('');
  const [targetStage, setTargetStage] = useState(stages[0]?.key || 'dev');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  // Load saved pipelines (workflows) for the "via pipeline" mode.
  useEffect(() => {
    workflowsApi.list()
      .then(({ workflows }) => {
        setPipelines(workflows || []);
        if (workflows && workflows.length > 0) setPipelineId(workflows[0].id);
      })
      .catch(() => setPipelines([]));
  }, []);

  const targetLabel = stages.find(s => s.key === targetStage)?.label || targetStage;

  const handleDeploy = async () => {
    if (!imageTag) { setError('Image tag is required'); return; }
    setLoading(true);
    setError('');
    try {
      if (mode === 'pipeline') {
        if (!pipelineId) { setError('Select a pipeline'); return; }
        // Run the saved pipeline with the form values as inputs — the engine
        // substitutes them into {{ input.X }} placeholders of the steps.
        await workflowsApi.execute(pipelineId, {
          project_name: projectName,
          image_tag: imageTag,
          image_repository: imageRepo,
          stage: targetStage,
          team_name: team,
          team,
          jira_issue_key: jiraKey,
          jira_summary: jiraSummary,
        });
      } else {
        await gitops.deploy({
          image_tag: imageTag,
          image_repository: imageRepo,
          project_name: projectName,
          jira_issue_key: jiraKey,
          jira_summary: jiraSummary,
          namespace: `app-${targetStage}`,
          team,
          stage: targetStage,
        });
      }
      onDeployed();
    } catch (err) {
      setError(mode === 'pipeline' ? 'Pipeline execution failed' : 'Deployment failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-md" onClick={e => e.stopPropagation()}>
        <div className="p-4 border-b border-[var(--border)]">
          <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Manual Deploy to {targetLabel}</h3>
          <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">Team: {team}</p>
        </div>
        <div className="p-4 space-y-4">
          {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

          {/* Deploy mode: direct record vs. saved pipeline */}
          <div className="flex gap-1 bg-[var(--border-light)] rounded-lg p-0.5">
            <button
              onClick={() => setMode('direct')}
              className={`flex-1 px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${mode === 'direct' ? 'bg-white text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)]'}`}
            >
              Direct Deploy
            </button>
            <button
              onClick={() => setMode('pipeline')}
              className={`flex-1 px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors ${mode === 'pipeline' ? 'bg-white text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-tertiary)]'}`}
            >
              Via Pipeline
            </button>
          </div>

          {mode === 'pipeline' && (
            <div>
              <label className="label">Pipeline *</label>
              {pipelines.length === 0 ? (
                <p className="text-[12px] text-[var(--text-tertiary)] py-1">
                  No pipelines yet — build one in the <a href="/pipelines/new" className="text-[var(--accent)] hover:underline">Pipeline Builder</a>.
                </p>
              ) : (
                <select value={pipelineId} onChange={e => setPipelineId(e.target.value)} className="input">
                  {pipelines.map(p => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              )}
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                The pipeline runs with the values below as inputs (project name, tag, stage, team) and its deploy steps create cards on this board.
              </p>
            </div>
          )}

          <div>
            <label className="label">Target Environment</label>
            <select value={targetStage} onChange={e => setTargetStage(e.target.value)} className="input">
              {stages.map(s => (
                <option key={s.key} value={s.key}>{s.label}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="label">Image Tag *</label>
            <input type="text" value={imageTag} onChange={e => setImageTag(e.target.value)} className="input" placeholder="abc1234" />
          </div>
          <div>
            <label className="label">Image Repository</label>
            <input type="text" value={imageRepo} onChange={e => setImageRepo(e.target.value)} className="input" placeholder="registry.example.com/app" />
          </div>
          <div>
            <label className="label">Project Name</label>
            <input type="text" value={projectName} onChange={e => setProjectName(e.target.value)} className="input" placeholder="my-service" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label">Jira Issue</label>
              <input type="text" value={jiraKey} onChange={e => setJiraKey(e.target.value)} className="input" placeholder="PROJ-123" />
            </div>
            <div>
              <label className="label">Namespace</label>
              <input type="text" readOnly value={`app-${targetStage}`} className="input" placeholder={`app-${targetStage}`} />
            </div>
          </div>
          <div>
            <label className="label">Summary</label>
            <input type="text" value={jiraSummary} onChange={e => setJiraSummary(e.target.value)} className="input" placeholder="Brief description" />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={onClose} className="btn btn-ghost">Cancel</button>
            <button onClick={handleDeploy} disabled={loading} className="btn btn-primary">
              {loading ? (mode === 'pipeline' ? 'Running...' : 'Deploying...') : (mode === 'pipeline' ? 'Run Pipeline' : 'Deploy')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function formatTime(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const mins = Math.floor(diffMs / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
