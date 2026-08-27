'use client';

import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import { workflows, type Workflow, type WorkflowExecution, type StepExecution } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import ConceptHelp from '@/components/ConceptHelp';
import EmptyState from '@/components/EmptyState';
import ConfirmModal from '@/components/ConfirmModal';
import { Toast } from '@/components/Interactive';

const statusBadge: Record<string, string> = {
  completed: 'badge-success',
  succeeded: 'badge-success',
  success: 'badge-success',
  failed: 'badge-danger',
  running: 'badge-warning',
  pending: 'badge-default',
  waiting: 'badge-warning',
};

// ── Runtime inputs ─────────────────────────────────────────
// Steps may reference {{ input.NAME }} placeholders; at run time the user
// fills the values in the Run modal and the engine substitutes them.

function extractInputs(wf: Workflow): string[] {
  const text = JSON.stringify(wf.spec || {});
  const names = new Set<string>();
  const re = /\{\{\s*input\.([a-zA-Z0-9_.-]+)\s*\}\}/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) names.add(m[1]);
  return Array.from(names);
}

const INPUT_DEFAULTS: Record<string, { value: string; hint: string }> = {
  project_name: { value: 'my-service', hint: 'Service / project to deploy' },
  image_tag: { value: 'latest', hint: 'Docker image tag' },
  stage: { value: 'dev', hint: 'Target stage (dev, staging, production)' },
  team_name: { value: 'platform-team', hint: 'Team that owns the deployment' },
};

// ── Built-in workflow templates ──────────────────────────────

interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  icon: string;
  steps: Array<{ label: string; type: string }>;
  spec: Record<string, unknown>;
}

const WORKFLOW_TEMPLATES: WorkflowTemplate[] = [
  {
    id: 'deploy-to-dev',
    name: 'Deploy to dev',
    description: 'Creates a real deployment in the dev stage — the result appears as a card on the GitOps Workflow board.',
    icon: '🚀',
    steps: [{ label: 'deploy', type: 'deploy' }],
    spec: {
      triggers: [],
      steps: [
        {
          name: 'deploy-to-dev',
          type: 'deploy',
          params: { project_name: '{{ input.project_name }}', image_tag: '{{ input.image_tag }}', stage: 'dev', team_name: '{{ input.team_name }}' },
        },
      ],
    },
  },
  {
    id: 'deploy-notify',
    name: 'Deploy + Notify',
    description: 'Deploys to dev and sends a Slack notification (simulated when no Slack plugin is connected).',
    icon: '🔔',
    steps: [{ label: 'deploy', type: 'deploy' }, { label: 'notify', type: 'plugin' }],
    spec: {
      triggers: [],
      steps: [
        {
          name: 'deploy-to-dev',
          type: 'deploy',
          params: { project_name: '{{ input.project_name }}', image_tag: '{{ input.image_tag }}', stage: 'dev', team_name: '{{ input.team_name }}' },
        },
        {
          name: 'notify-team',
          plugin: 'slack',
          action: 'send_message',
          depends_on: ['deploy-to-dev'],
          params: { channel: '#deployments', text: '{{ input.project_name }} {{ input.image_tag }} deployed to dev' },
        },
      ],
    },
  },
  {
    id: 'full-chain',
    name: 'Full chain simulation',
    description: 'Condition gate → simulated rollout → real deployment → notification. Shows a complete delivery chain.',
    icon: '⛓️',
    steps: [
      { label: 'condition', type: 'condition' },
      { label: 'deploy_sim', type: 'deploy_sim' },
      { label: 'deploy', type: 'deploy' },
      { label: 'notify', type: 'plugin' },
    ],
    spec: {
      triggers: [],
      steps: [
        { name: 'release-gate', type: 'condition', condition: '!input.project_name ==' },
        {
          name: 'simulate-rollout',
          type: 'deploy_sim',
          depends_on: ['release-gate'],
          params: { service_name: '{{ input.project_name }}', namespace: 'app-dev', image: '{{ input.project_name }}:{{ input.image_tag }}' },
        },
        {
          name: 'deploy-to-dev',
          type: 'deploy',
          depends_on: ['simulate-rollout'],
          params: { project_name: '{{ input.project_name }}', image_tag: '{{ input.image_tag }}', stage: 'dev', team_name: '{{ input.team_name }}' },
        },
        {
          name: 'notify-team',
          plugin: 'slack',
          action: 'send_message',
          depends_on: ['deploy-to-dev'],
          params: { channel: '#deployments', text: 'Full delivery chain completed for {{ input.project_name }}' },
        },
      ],
    },
  },
  {
    id: 'approval-gate',
    name: 'Approval gate demo',
    description: 'Pauses the execution and waits for approval before continuing — demonstrates gated deployments.',
    icon: '🔐',
    steps: [{ label: 'approval', type: 'approval' }, { label: 'deploy', type: 'deploy' }],
    spec: {
      triggers: [],
      steps: [
        {
          name: 'release-approval',
          type: 'approval',
          params: { approvers: ['platform-team'], message: 'Approve promotion of {{ input.project_name }}' },
        },
        {
          name: 'deploy-after-approval',
          type: 'deploy',
          depends_on: ['release-approval'],
          params: { project_name: '{{ input.project_name }}', image_tag: '{{ input.image_tag }}', stage: 'staging', team_name: '{{ input.team_name }}' },
        },
      ],
    },
  },
];

export default function AutomationPage() {
  const [list, setList] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Workflow | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [executions, setExecutions] = useState<Record<string, WorkflowExecution[]>>({});
  const [expandedExec, setExpandedExec] = useState<string | null>(null);
  const [steps, setSteps] = useState<Record<string, StepExecution[]>>({});
  const [showTemplates, setShowTemplates] = useState(false);
  const [runTarget, setRunTarget] = useState<Workflow | null>(null);

  const loadData = useCallback(async () => {
    try {
      const wfData = await workflows.list().catch(() => ({ workflows: [], total: 0 }));
      setList(wfData.workflows || []);

      // Latest execution per workflow (for the status column)
      const execs: Record<string, WorkflowExecution[]> = {};
      await Promise.all((wfData.workflows || []).slice(0, 10).map(async wf => {
        try {
          const data = await workflows.executions(wf.id);
          execs[wf.id] = (data.executions || []).slice(0, 3);
        } catch { /* ignore */ }
      }));
      setExecutions(execs);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  const handleRun = async (wf: Workflow, params?: Record<string, string>) => {
    setRunTarget(null);
    setRunningId(wf.id);
    setToast(null);
    try {
      const exec = await workflows.execute(wf.id, params);
      setToast({ message: `Workflow "${wf.name}" started`, type: 'success' });
      // Poll the execution until it finishes (max ~30s)
      for (let i = 0; i < 15; i++) {
        await new Promise(r => setTimeout(r, 2000));
        try {
          const data = await workflows.executions(wf.id);
          const fresh = (data.executions || []).find(e => e.id === exec.id);
          setExecutions(prev => ({ ...prev, [wf.id]: (data.executions || []).slice(0, 3) }));
          if (fresh && fresh.status !== 'running' && fresh.status !== 'pending') break;
        } catch { /* ignore */ }
      }
    } catch (err) {
      const fe = friendlyError(err);
      setToast({ message: fe.hint ? `${fe.hint} (${fe.message})` : fe.message, type: 'error' });
    } finally {
      setRunningId(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await workflows.delete(deleteTarget.id);
      setToast({ message: `Workflow "${deleteTarget.name}" deleted`, type: 'success' });
      setDeleteTarget(null);
      await loadData();
    } catch (err) {
      const fe = friendlyError(err);
      setToast({ message: fe.hint ? `${fe.hint} (${fe.message})` : fe.message, type: 'error' });
    } finally {
      setDeleting(false);
    }
  };

  const toggleExec = async (execId: string) => {
    if (expandedExec === execId) {
      setExpandedExec(null);
      return;
    }
    setExpandedExec(execId);
    if (!steps[execId]) {
      try {
        const data = await workflows.stepExecutions(execId);
        setSteps(prev => ({ ...prev, [execId]: data.step_executions || [] }));
      } catch { /* ignore */ }
    }
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <h1 className="page-title-modern">Automation</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-secondary)]">Loading workflows...</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Automation</h1>
              <ConceptHelp term="workflow" />
            </div>
            <p className="page-subtitle-modern">
              Workflows automate build, test, deploy and notify steps. Create one from a template and run it.
            </p>
          </div>
        <div className="flex gap-2">
          <button onClick={() => setShowTemplates(true)} className="btn btn-primary btn-sm">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" /></svg>
            New Workflow
          </button>
          <Link href="/pipelines/new" className="btn btn-sm" style={{ backgroundColor: '#10B981', color: 'white' }}>
            <svg className="w-3.5 h-3.5 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" /></svg>
            Build Pipeline
          </Link>
        </div>
      </div>
      </div>

      {/* Workflows list */}
      {list.length === 0 ? (
        <EmptyState
          icon="⚡"
          title="No workflows yet"
          description="A workflow is a set of automated steps that run in order: build, test, deploy, notify. Start from a built-in template or build one visually."
          actionOnClick={() => setShowTemplates(true)}
          actionLabel="New Workflow from Template"
          secondaryHref="/pipelines/new"
          secondaryLabel="Build Pipeline"
        />
      ) : (
        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-header">
            <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Workflows ({list.length})</h2>
          </div>
          <div className="divide-y divide-[var(--border-light)]">
            {list.map(wf => {
              const wfExecs = executions[wf.id] || [];
              const latest = wfExecs[0];
              return (
                <div key={wf.id} className="px-4 py-3">
                  <div className="flex items-center gap-3">
                    <div className="flex-1 min-w-0">
                      <p className="text-[13px] font-medium text-[var(--text-primary)]">{wf.name}</p>
                      {typeof (wf.spec as { description?: unknown }).description === 'string' && (
                        <p className="text-[11px] text-[var(--text-tertiary)] truncate">
                          {(wf.spec as { description?: string }).description}
                        </p>
                      )}
                    </div>
                    {latest && (
                      <span className={`badge ${statusBadge[latest.status] || 'badge-default'}`}>
                        {latest.status}
                      </span>
                    )}
                    <Link
                      href={`/pipelines/edit?id=${wf.id}`}
                      className="px-3 py-1.5 bg-[var(--border-light)] text-[var(--text-secondary)] text-[12px] rounded-lg hover:bg-[var(--border)] transition-colors"
                    >
                      Edit
                    </Link>
                    <button
                      onClick={() => setDeleteTarget(wf)}
                      className="px-2 py-1.5 text-red-500 hover:bg-red-500/10 text-[12px] rounded-lg transition-colors"
                      title="Delete workflow"
                    >
                      <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867.127A4.962 4.962 0 0116 2.5H8a4.962 4.962 0 00-2.133.627L5 7m4 4v6m4-6v6m2-10h2a2 2 0 012 2v8a2 2 0 01-2 2H7a2 2 0 01-2-2V9a2 2 0 012-2h2" />
                      </svg>
                    </button>
                    <button
                      onClick={() => {
                        // Pipelines with {{ input.X }} placeholders need values at run time.
                        if (extractInputs(wf).length > 0) setRunTarget(wf);
                        else handleRun(wf);
                      }}
                      disabled={runningId === wf.id}
                      className="px-3 py-1.5 bg-[var(--accent)] text-white text-[12px] rounded-lg hover:opacity-90 disabled:opacity-50 transition-colors"
                    >
                      {runningId === wf.id ? 'Running...' : 'Run'}
                    </button>
                  </div>

                  {/* Execution history */}
                  {wfExecs.length > 0 && (
                    <div className="mt-2 space-y-1">
                      {wfExecs.map(exec => (
                        <div key={exec.id} className="text-[11px]">
                          <button
                            onClick={() => toggleExec(exec.id)}
                            className="flex items-center gap-2 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors"
                          >
                            <span className={`badge ${statusBadge[exec.status] || 'badge-default'}`}>{exec.status}</span>
                            <span>{new Date(exec.created_at).toLocaleString()}</span>
                            {exec.duration_ms != null && <span>({Math.round(exec.duration_ms)} ms)</span>}
                            <span className="text-[var(--accent)]">{expandedExec === exec.id ? 'hide steps' : 'show steps'}</span>
                          </button>
                          {expandedExec === exec.id && (
                            <div className="mt-1 ml-2 space-y-0.5">
                              {!steps[exec.id] ? (
                                <p className="text-[var(--text-tertiary)]">Loading steps...</p>
                              ) : steps[exec.id].length === 0 ? (
                                <p className="text-[var(--text-tertiary)]">No step details recorded.</p>
                              ) : (
                                steps[exec.id].map((s, i) => (
                                  <div key={i} className="flex items-center gap-2">
                                    <span className={`badge ${statusBadge[s.status] || 'badge-default'}`}>{s.status}</span>
                                    <span className="text-[var(--text-secondary)]">{s.step_name}</span>
                                    {s.error && <span className="text-red-500 truncate max-w-md">{s.error}</span>}
                                  </div>
                                ))
                              )}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {deleteTarget && (
        <ConfirmModal
          open={true}
          title="Delete Workflow"
          description={`Are you sure you want to delete "${deleteTarget.name}"? This action cannot be undone.`}
          confirmLabel="Delete"
          variant="danger"
          loading={deleting}
          onConfirm={handleDelete}
          onCancel={() => setDeleteTarget(null)}
        />
      )}

      {showTemplates && (
        <TemplatesModal
          onClose={() => setShowTemplates(false)}
          onCreated={(name) => {
            setShowTemplates(false);
            setToast({ message: `Workflow "${name}" created — press Run to execute it`, type: 'success' });
            loadData();
          }}
          onError={(message) => setToast({ message, type: 'error' })}
        />
      )}

      {runTarget && (
        <RunModal
          workflow={runTarget}
          onClose={() => setRunTarget(null)}
          onRun={(params) => handleRun(runTarget, params)}
        />
      )}
      </div>
    </div>
  );
}

function RunModal({ workflow, onClose, onRun }: { workflow: Workflow; onClose: () => void; onRun: (params: Record<string, string>) => void }) {
  const names = extractInputs(workflow);
  const [values, setValues] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    names.forEach(n => { init[n] = INPUT_DEFAULTS[n]?.value || ''; });
    return init;
  });

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-md" onClick={e => e.stopPropagation()}>
        <div className="p-4 border-b border-[var(--border)]">
          <h3 className="text-[14px] font-medium text-[var(--text-primary)]">Run: {workflow.name}</h3>
          <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">
            Fill the inputs used by this pipeline — the same pipeline can deploy any service.
          </p>
        </div>
        <div className="p-4 space-y-3">
          {names.map(name => (
            <div key={name}>
              <label className="label">{name}</label>
              <input
                type="text"
                value={values[name] || ''}
                onChange={e => setValues({ ...values, [name]: e.target.value })}
                className="input"
                placeholder={INPUT_DEFAULTS[name]?.value || 'value'}
                autoFocus={name === names[0]}
              />
              {INPUT_DEFAULTS[name]?.hint && (
                <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">{INPUT_DEFAULTS[name].hint}</p>
              )}
            </div>
          ))}
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={onClose} className="btn btn-ghost">Cancel</button>
            <button onClick={() => onRun(values)} className="btn btn-primary">Run Workflow</button>
          </div>
        </div>
      </div>
    </div>
  );
}

function TemplatesModal({ onClose, onCreated, onError }: { onClose: () => void; onCreated: (name: string) => void; onError: (message: string) => void }) {
  const [creatingId, setCreatingId] = useState<string | null>(null);

  const handleCreate = async (tpl: WorkflowTemplate) => {
    setCreatingId(tpl.id);
    try {
      await workflows.create({ name: tpl.name, spec: tpl.spec, source: 'template' });
      onCreated(tpl.name);
    } catch (err) {
      onError(friendlyError(err).message);
    } finally {
      setCreatingId(null);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-2xl max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
          <div>
            <h3 className="text-[14px] font-medium text-[var(--text-primary)]">New Workflow</h3>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">Pick a template — it is created instantly and can be run right away</p>
          </div>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="p-4 grid grid-cols-1 sm:grid-cols-2 gap-3">
          {WORKFLOW_TEMPLATES.map(tpl => (
            <div key={tpl.id} className="card">
              <div className="card-body space-y-2">
                <div className="flex items-center gap-2">
                  <span className="text-xl">{tpl.icon}</span>
                  <h4 className="text-[13px] font-medium text-[var(--text-primary)]">{tpl.name}</h4>
                </div>
                <p className="text-[12px] text-[var(--text-secondary)]">{tpl.description}</p>
                <div className="flex flex-wrap gap-1">
                  {tpl.steps.map((s, i) => (
                    <span key={i} className="badge badge-default text-[10px]">{s.label}</span>
                  ))}
                </div>
                <button
                  onClick={() => handleCreate(tpl)}
                  disabled={creatingId === tpl.id}
                  className="btn btn-primary w-full !py-1.5 text-[12px] disabled:opacity-50"
                >
                  {creatingId === tpl.id ? 'Creating...' : 'Use Template'}
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
