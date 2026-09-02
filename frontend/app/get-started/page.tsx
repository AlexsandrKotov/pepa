'use client';

import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import {
  connections, clusters, entities, workflows, audit, platformSettings,
  type Workflow,
} from '@/lib/api';
import { friendlyError } from '@/lib/errors';

const SETTINGS_KEY = 'get_started';

interface StepStatus {
  done: boolean;
  detail: string;
}

export default function GetStartedPage() {
  const [loading, setLoading] = useState(true);
  const [statuses, setStatuses] = useState<Record<string, StepStatus>>({});
  const [completed, setCompleted] = useState(false);
  const [showCompleted, setShowCompleted] = useState(false);
  const [demoWf, setDemoWf] = useState<Workflow | null>(null);
  const [runState, setRunState] = useState<{ running: boolean; result?: string; error?: string }>({ running: false });

  const loadProgress = useCallback(async () => {
    try {
      const [connData, clusterData, entData, wfData, auditData] = await Promise.all([
        connections.list().catch(() => ({ connections: [], total: 0 })),
        clusters.list().catch(() => ({ clusters: [], total: 0 })),
        entities.list({ per_page: '1' }).catch(() => ({ items: [], total: 0, page: 1, per_page: 1, total_pages: 0 })),
        workflows.list().catch(() => ({ workflows: [], total: 0 })),
        audit.list({ per_page: '100' }).catch(() => ({ items: [], total: 0, page: 1, per_page: 100, total_pages: 0 })),
      ]);

      const connList = connData.connections || [];
      const clusterList = clusterData.clusters || [];
      const wfList = wfData.workflows || [];
      const ranWorkflow = (auditData.items || []).some(a => a.action === 'execute' && a.entity_type === 'workflow');

      setStatuses({
        cluster: {
          done: clusterList.length > 0,
          detail: clusterList.length > 0
            ? `${clusterList.length} cluster(s) connected`
            : 'No clusters yet',
        },
        gitlab: {
          done: connList.some(c => c.type === 'gitlab' || (c.type === 'git' && (c as any).config?.provider === 'gitlab')),
          detail: connList.some(c => c.type === 'gitlab' || (c.type === 'git' && (c as any).config?.provider === 'gitlab')) ? 'GitLab connected' : 'Not connected (optional)',
        },
        service: {
          done: entData.total > 0,
          detail: entData.total > 0 ? `${entData.total} entity(ies) in the catalog` : 'No services yet',
        },
        workflow: {
          done: wfList.length > 0,
          detail: wfList.length > 0 ? `${wfList.length} workflow(s) created` : 'No workflows yet',
        },
        run: {
          done: ranWorkflow,
          detail: ranWorkflow ? 'A workflow has been executed' : 'No runs yet',
        },
      });

      // Locate the demo workflow so step 5 can run it directly
      const hello = wfList.find(w => w.name === 'Hello PEPA');
      setDemoWf(hello || null);

      // Persisted "tour completed" acknowledgement
      try {
        const stored = await platformSettings.get(SETTINGS_KEY);
        const v = stored.value as { completed?: boolean } | undefined;
        if (v?.completed) setCompleted(true);
      } catch { /* key not set yet */ }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadProgress(); }, [loadProgress]);

  // Only required steps count toward completion (GitLab is optional)
  const requiredStepIds = ['cluster', 'service', 'workflow', 'run'];
  const allDone = requiredStepIds.every(id => statuses[id]?.done);

  // Auto-mark tour as completed when all steps are done — hides the sidebar tab
  useEffect(() => {
    if (allDone && !completed) {
      markCompleted();
    }
  }, [allDone, completed]);

  const markCompleted = async () => {
    setCompleted(true);
    setShowCompleted(false);
    try {
      await platformSettings.update(SETTINGS_KEY, { completed: true, completed_at: new Date().toISOString() });
      window.dispatchEvent(new Event('pepa-tour-completed'));
    } catch { /* non-critical */ }
  };

  const runDemo = async () => {
    if (!demoWf) return;
    setRunState({ running: true });
    try {
      await workflows.execute(demoWf.id);
      // Poll until finished (max ~20s)
      for (let i = 0; i < 10; i++) {
        await new Promise(r => setTimeout(r, 2000));
        try {
          const data = await workflows.executions(demoWf.id);
          const latest = (data.executions || [])[0];
          if (latest && latest.status !== 'running' && latest.status !== 'pending') {
            if (latest.status === 'completed' || latest.status === 'succeeded' || latest.status === 'success') {
              setRunState({ running: false, result: 'All 3 steps finished successfully. Check the Automation page for details.' });
            } else {
              setRunState({ running: false, error: `Execution finished with status "${latest.status}". Open the Automation page to inspect steps.` });
            }
            await loadProgress();
            return;
          }
        } catch { /* ignore */ }
      }
      setRunState({ running: false, result: 'Execution started. It is still running — check the Automation page.' });
    } catch (err) {
      const fe = friendlyError(err);
      setRunState({ running: false, error: fe.hint ? `${fe.hint} (${fe.message})` : fe.message });
    }
  };

  const STEPS = [
    {
      id: 'cluster',
      title: 'Connect a Kubernetes cluster',
      desc: 'PEPA needs a cluster to deploy to. Add your kubeconfig in the Clusters page — PEPA will check that it works.',
      href: '/clusters',
      action: 'Add cluster',
    },
    {
      id: 'gitlab',
      title: 'Connect GitLab (optional)',
      desc: 'With GitLab connected you can browse repositories and import services with one click.',
      href: '/connections?type=gitlab',
      action: 'Add connection',
    },
    {
      id: 'service',
      title: 'Register your first service',
      desc: 'A service is anything you run and maintain: an app, an API, a worker. Create one from a template.',
      href: '/services/new',
      action: 'Create service',
    },
    {
      id: 'workflow',
      title: 'Create a workflow',
      desc: 'Workflows automate steps like build, test, deploy, notify. Start from a ready-made template.',
      href: '/automation',
      action: 'Open Automation',
    },
    {
      id: 'run',
      title: 'Run a workflow',
      desc: 'Execute the demo workflow and watch every step complete — no external systems needed.',
      href: '/automation',
      action: 'Open Automation',
      runnable: true,
    },
  ];

  if (loading) {
    return (
      <div className="space-y-6">
        <h1 className="page-title-modern">Get Started</h1>
        <div className="card card-body text-center py-12">
          <p className="text-[13px] text-[var(--text-secondary)]">Checking your progress...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6 max-w-3xl">
      <div className="page-animate">
        <h1 className="page-title-modern">Get Started with PEPA</h1>
        <p className="page-subtitle-modern">
          Five steps from an empty platform to your first automated workflow run.
          Each step is checked automatically against your real data.
        </p>
      </div>

      {/* Completion state */}
      {completed && !showCompleted ? (
        <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="flex items-center justify-between">
            <p className="text-[13px] text-green-600 font-medium">
              ✓ You completed the Golden Path. The platform is yours — explore the features in the sidebar.
            </p>
            <button
              onClick={() => setShowCompleted(true)}
              className="text-[12px] text-[var(--accent)] hover:underline shrink-0 ml-4"
            >
              Show tour again
            </button>
          </div>
        </div>
      ) : null}

      {/* Steps */}
      {(!completed || showCompleted) && (
        <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <div className="divide-y divide-[var(--border-light)]">
            {STEPS.map((s, idx) => {
              const st = statuses[s.id];
              const done = !!st?.done;
              return (
                <div key={s.id} className="p-4">
                  <div className="flex items-start gap-3">
                    <span className={`w-7 h-7 rounded-full flex items-center justify-center text-[12px] font-medium shrink-0 mt-0.5 ${
                      done ? 'bg-emerald-500/15 text-emerald-600' : 'bg-[var(--bg)] text-[var(--text-tertiary)]'
                    }`}>
                      {done ? '✓' : idx + 1}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <p className={`text-[14px] font-medium ${done ? 'text-[var(--text-tertiary)] line-through' : 'text-[var(--text-primary)]'}`}>
                          {s.title}
                        </p>
                        <span className={`text-[11px] ${done ? 'text-green-600' : 'text-[var(--text-tertiary)]'}`}>
                          {st?.detail}
                        </span>
                      </div>
                      <p className="text-[12px] text-[var(--text-tertiary)] mt-1">{s.desc}</p>
                      <div className="flex items-center gap-3 mt-2">
                        {!done && (
                          <Link
                            href={s.href}
                            className="px-3 py-1.5 bg-[var(--accent)] text-white text-[12px] rounded-lg hover:opacity-90 transition-colors"
                          >
                            {s.action}
                          </Link>
                        )}
                        {s.runnable && !done && (
                          <button
                            onClick={runDemo}
                            disabled={runState.running || !demoWf}
                            title={!demoWf ? 'Load demo data first to get the Hello PEPA workflow' : undefined}
                            className="px-3 py-1.5 border border-[var(--border)] text-[var(--text-secondary)] text-[12px] rounded-lg hover:bg-[var(--bg)] disabled:opacity-50 transition-colors"
                          >
                            {runState.running ? 'Running...' : demoWf ? 'Run "Hello PEPA" now' : 'Run demo (load demo data first)'}
                          </button>
                        )}
                      </div>
                      {s.runnable && runState.result && (
                        <p className="text-[12px] text-green-600 mt-2">✓ {runState.result}</p>
                      )}
                      {s.runnable && runState.error && (
                        <p className="text-[12px] text-red-500 mt-2">{runState.error}</p>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
          {allDone && (
            <div className="p-4 border-t border-[var(--border-light)] bg-emerald-500/10 border-emerald-500/20">
              <div className="flex items-center justify-between">
                <p className="text-[13px] text-emerald-600 font-medium">
                  {statuses.gitlab?.done ? 'All five steps are done. Nice work!' : 'All required steps are done. Nice work!'}
                </p>
                <button
                  onClick={markCompleted}
                  className="px-4 py-2 bg-emerald-600 text-white text-[12px] rounded-lg hover:bg-emerald-500 transition-colors"
                >
                  Finish tour
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* What's next */}
      <div className="card page-animate-up page-delay-4" style={{ borderRadius: '12px' }}>
        <div className="card-header">
          <h2 className="text-[13px] font-medium text-[var(--text-primary)]">Where to go next</h2>
        </div>
        <div className="card-body space-y-2">
          {[
            { href: '/deployments', text: 'Deployments — deliver a service version to a cluster through the GitOps pipeline.' },
            { href: '/scorecards', text: 'Scorecards — measure production readiness of your services.' },
            { href: '/plugins', text: 'Plugins — see loaded integrations (GitLab, Jira, FluxCD, ArgoCD, Slack).' },
          ].map(l => (
            <Link key={l.href} href={l.href} className="block text-[12px] text-[var(--text-secondary)] hover:text-[var(--accent)] transition-colors">
              → {l.text}
            </Link>
          ))}
        </div>
      </div>
      </div>
    </div>
  );
}
