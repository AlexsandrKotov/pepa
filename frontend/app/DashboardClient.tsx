'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import Link from 'next/link';
import {
  audit, connections, clusters, deployments, services, environments,
  gitops, vault, pipelineSources, pipelineRuns, dockerServices,
  platformSettings,
  type Connection, type Cluster, type Deployment, type AuditEntry,
  type Service, type Environment, type GitopsRepo, type PipelineSource, type PipelineRun,
  type DockerService,
} from '@/lib/api';
import { getStoredUser } from '@/lib/api';
import CollapsibleSection from '@/components/CollapsibleSection';
import {
  StatCard, QuickActionButton, TimelineItem, ClusterRow,
  DeploymentHealthWidget, PipelineActivityWidget, EnvironmentOverviewWidget,
  GitOpsStatusWidget, SecurityComplianceWidget, ServicesCatalogWidget,
  EnhancedSystemStatus,
} from '@/components/DashboardWidgets';
import BrandIcon from '@/components/BrandIcon';
import { useDashboardProfile, PROFILE_CONFIGS, type DashboardProfile } from '@/hooks/useDashboardProfile';
import { usePermission } from '@/hooks/usePermission';

// ── Data types ───────────────────────────────────────────────

interface DashboardData {
  connList: Connection[];
  clusterList: Cluster[];
  deploymentList: Deployment[];
  recentAudit: AuditEntry[];
  serviceList: Service[];
  serviceTotal: number;
  envList: Environment[];
  gitopsRepos: GitopsRepo[];
  pipelineSourceList: PipelineSource[];
  pipelineRunList: PipelineRun[];
  dockerServiceList: DockerService[];
  vaultSecrets: number;
  vaultMode: string;
  vaultNeedsRotation: boolean;
  vaultSealed: boolean;
}

// ── Profile Selector ─────────────────────────────────────────

function ProfileSelector({ current, onChange }: { current: DashboardProfile; onChange: (p: DashboardProfile) => void }) {
  const [open, setOpen] = useState(false);
  const btnRef = useRef<HTMLButtonElement>(null);
  const dropRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ top: 0, right: 0 });

  // Calculate dropdown position relative to viewport
  const updatePosition = useCallback(() => {
    if (btnRef.current) {
      const rect = btnRef.current.getBoundingClientRect();
      setPos({ top: rect.bottom + 6, right: window.innerWidth - rect.right });
    }
  }, []);

  useEffect(() => {
    if (open) updatePosition();
  }, [open, updatePosition]);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (
        btnRef.current && !btnRef.current.contains(e.target as Node) &&
        dropRef.current && !dropRef.current.contains(e.target as Node)
      ) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  // Close on scroll of any ancestor
  useEffect(() => {
    if (!open) return;
    const handler = () => setOpen(false);
    window.addEventListener('scroll', handler, true);
    return () => window.removeEventListener('scroll', handler, true);
  }, [open]);

  const config = PROFILE_CONFIGS[current];

  return (
    <>
      <button
        ref={btnRef}
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 px-3 py-1.5 text-[12px] font-medium bg-[var(--surface)] border border-[var(--border)] rounded-lg hover:border-[var(--text-tertiary)] transition-all duration-200"
      >
        <svg className="w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d={config.icon} />
        </svg>
        <span>{config.label}</span>
        <svg className={`w-3 h-3 text-[var(--text-tertiary)] transition-transform ${open ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {open && createPortal(
        <div
          ref={dropRef}
          className="fixed w-56 bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-2xl py-1"
          style={{ top: pos.top, right: pos.right, zIndex: 9999 }}
        >
          {(Object.keys(PROFILE_CONFIGS) as DashboardProfile[]).map(p => {
            const pc = PROFILE_CONFIGS[p];
            const isActive = p === current;
            return (
              <button
                key={p}
                onClick={() => { onChange(p); setOpen(false); }}
                className={`w-full flex items-center gap-3 px-3 py-2.5 text-left transition-colors ${isActive ? 'bg-[var(--accent)]/5' : 'hover:bg-[var(--bg)]'}`}
              >
                <div className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 ${isActive ? 'bg-[var(--accent)]/10' : 'bg-[var(--border-light)]'}`}>
                  <svg className={`w-4 h-4 ${isActive ? 'text-[var(--accent)]' : 'text-[var(--text-tertiary)]'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d={pc.icon} />
                  </svg>
                </div>
                <div className="min-w-0">
                  <p className={`text-[12px] font-medium ${isActive ? 'text-[var(--accent)]' : 'text-[var(--text-primary)]'}`}>{pc.label}</p>
                  <p className="text-[10px] text-[var(--text-tertiary)]">{pc.description}</p>
                </div>
                {isActive && (
                  <svg className="w-4 h-4 text-[var(--accent)] ml-auto flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                  </svg>
                )}
              </button>
            );
          })}
        </div>,
        document.body
      )}
    </>
  );
}

// ── Welcome Banner ───────────────────────────────────────────

function WelcomeBanner({ userName, onDismiss }: { userName: string; onDismiss: () => void }) {
  return (
    <div className="dash-animate-in card overflow-hidden relative">
      <div className="absolute inset-0 opacity-[0.03]" style={{ background: 'radial-gradient(circle at 30% 50%, var(--accent), transparent 70%)' }} />
      <div className="relative px-5 py-4 flex items-center justify-between gap-4">
        <div>
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)] mb-0.5">Welcome, {userName}!</h2>
          <p className="text-[12px] text-[var(--text-secondary)]">Get started by setting up your first connection, or explore the platform.</p>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <Link href="/get-started" className="px-3.5 py-1.5 text-[12px] font-medium bg-[var(--accent)] text-white rounded-lg hover:opacity-85 transition-opacity">
            Guided Tour
          </Link>
          <button onClick={onDismiss} className="px-2 py-1.5 text-[11px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors">
            Dismiss
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Quick Actions Config ─────────────────────────────────────

interface QuickActionDef {
  id: string;
  href: string;
  label: string;
  icon: string; // SVG path
  primary?: boolean;
  adminOnly?: boolean;
  permission?: string; // RBAC resource for override (e.g. 'audit', 'settings')
}

const ALL_QUICK_ACTIONS: Record<string, QuickActionDef> = {
  'new-pipeline': { id: 'new-pipeline', href: '/pipeline-builder', label: 'New Pipeline', icon: 'M12 4.5v15m7.5-7.5h-15', primary: true },
  'deploy-service': { id: 'deploy-service', href: '/services/new', label: 'Deploy Service', icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12' },
  'add-connection': { id: 'add-connection', href: '/connections', label: 'Add Connection', icon: 'M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.54a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374' },
  'add-cluster': { id: 'add-cluster', href: '/clusters', label: 'Add Cluster', icon: 'M12 4.5v15m7.5-7.5h-15' },
  'check-drift': { id: 'check-drift', href: '/gitops/drift', label: 'Check Drift', icon: 'M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z' },
  'view-pipelines': { id: 'view-pipelines', href: '/pipelines', label: 'View Pipelines', icon: 'M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z' },
  'browse-services': { id: 'browse-services', href: '/services', label: 'Browse Services', icon: 'M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9' },
  'my-scorecards': { id: 'my-scorecards', href: '/scorecards', label: 'Scorecards', icon: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15c1.012 0 1.867.668 2.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25zM6.75 12h.008v.008H6.75V12zm0 3h.008v.008H6.75V15zm0 3h.008v.008H6.75V18z' },
  'audit-log': { id: 'audit-log', href: '/audit', label: 'Audit Log', icon: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08', adminOnly: true, permission: 'audit' },
  'vault-secrets': { id: 'vault-secrets', href: '/vault', label: 'Vault Secrets', icon: 'M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z' },
  'manage-users': { id: 'manage-users', href: '/settings/users', label: 'Manage Users', icon: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z', adminOnly: true, permission: 'settings' },
};

// ── Greeting helper ──────────────────────────────────────────

function getGreeting(): string {
  const h = new Date().getHours();
  if (h < 12) return 'Good morning';
  if (h < 18) return 'Good afternoon';
  return 'Good evening';
}

// ── Main Component ───────────────────────────────────────────

export default function DashboardClient() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [showWelcome, setShowWelcome] = useState(false);
  const [tourCompleted, setTourCompleted] = useState(false);
  const { profile, setProfile, config, hasWidget } = useDashboardProfile();
  const { isAdmin, hasPermission } = usePermission();

  const user = getStoredUser();
  const userName = user?.name?.split(' ')[0] || 'User';

  // Fetch all dashboard data
  useEffect(() => {
    Promise.all([
      connections.list().catch(() => ({ connections: [], total: 0 })),
      clusters.list().catch(() => ({ clusters: [], total: 0 })),
      deployments.list().catch(() => ({ deployments: [], total: 0 })),
      audit.list({ per_page: '8' }).catch(() => ({ items: [], total: 0, page: 1, per_page: 8, total_pages: 0 })),
      services.list({ per_page: '6' }).catch(() => ({ items: [], total: 0 })),
      environments.list().catch(() => ({ environments: [], total: 0 })),
      gitops.listRepos().catch(() => ({ repos: [], total: 0 })),
      pipelineSources.list({ per_page: '5' }).catch(() => ({ sources: [], total: 0 })),
      dockerServices.list().catch(() => ({ docker_services: [], total: 0 })),
      vault.getStatus().catch(() => ({ status: { total_secrets: 0, v1_secrets: 0, v2_secrets: 0, encryption_type: '', key_derivation: '', per_path_keys: false, needs_rotation: false, tenant_isolation: false, created_by_tracking: false, argon2_params: { time: 0, memory: 0, threads: 0, keyLen: 0 } }, mode: 'unknown' })),
    ]).then(([connData, clusterData, deploymentData, auditData, serviceData, envData, gitopsData, pipelineData, dockerData, vaultData]) => {
      // Fetch pipeline runs for all sources
      const sources = pipelineData.sources || [];
      const runPromises = sources.slice(0, 3).map(s =>
        pipelineRuns.list(s.id, { per_page: '3' }).catch(() => ({ runs: [], total: 0 }))
      );

      Promise.all(runPromises).then(runResults => {
        const allRuns = runResults.flatMap(r => r.runs || []).sort((a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
        ).slice(0, 5);

        setData({
          connList: connData.connections || [],
          clusterList: clusterData.clusters || [],
          deploymentList: deploymentData.deployments || [],
          recentAudit: auditData.items || [],
          serviceList: serviceData.items || [],
          serviceTotal: serviceData.total || 0,
          envList: envData.environments || [],
          gitopsRepos: gitopsData.repos || [],
          pipelineSourceList: sources,
          pipelineRunList: allRuns,
          dockerServiceList: dockerData.docker_services || [],
          vaultSecrets: vaultData.status?.total_secrets || 0,
          vaultMode: vaultData.mode || 'unknown',
          vaultNeedsRotation: vaultData.status?.needs_rotation || false,
          vaultSealed: false,
        });
        setLoading(false);
      });
    });

    // Check if welcome banner should show & if tour is completed
    try {
      const dismissed = localStorage.getItem('pepa-dashboard-welcome-dismissed');
      platformSettings.get('get_started').then(res => {
        const v = res.value as { completed?: boolean } | undefined;
        if (v?.completed) {
          setTourCompleted(true);
        } else if (!dismissed) {
          setShowWelcome(true);
        }
      }).catch(() => {});
    } catch { /* ignore */ }
  }, []);

  if (loading || !data) {
    return (
      <div className="space-y-8 dash-mesh-bg -mx-6 -my-6 px-6 py-6 min-h-[calc(100vh-48px)] flex items-center justify-center">
        <div className="flex items-center gap-3 text-[var(--text-tertiary)]">
          <svg className="animate-spin w-5 h-5" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <p className="text-[14px]">Loading dashboard...</p>
        </div>
      </div>
    );
  }

  const { connList, clusterList, deploymentList, recentAudit, serviceList, serviceTotal, envList, gitopsRepos, pipelineSourceList, pipelineRunList, dockerServiceList, vaultSecrets, vaultMode, vaultNeedsRotation, vaultSealed } = data;
  const activeClusters = clusterList.filter(c => c.is_active).length;
  const successfulDeploys = deploymentList.filter(d => d.status === 'deployed').length;
  const failedDeploys = deploymentList.filter(d => d.status === 'failed').length;
  const pendingDeploys = deploymentList.filter(d => d.status === 'pending' || d.status === 'progressing').length;
  const deploySuccessRate = deploymentList.length > 0 ? Math.round((successfulDeploys / deploymentList.length) * 100) : 0;
  const runningContainers = dockerServiceList.filter(s => s.status === 'running').length;

  // Build dynamic stat cards based on profile
  const statCardDefs: Record<string, { href: string; icon: React.ReactNode; label: string; value: number; subtitle?: string; subtitleColor?: string; iconBg: string; child?: React.ReactNode }> = {
    services: { href: '/services', icon: <BrandIcon name="services" size={20} />, label: 'Services', value: serviceTotal, subtitle: serviceTotal > 0 ? `${serviceList.filter(s => s.status === 'active').length} active` : undefined, subtitleColor: 'text-emerald-600', iconBg: 'bg-cyan-500/10' },
    clusters: { href: '/clusters', icon: <BrandIcon name="kubernetes" size={20} />, label: 'Clusters', value: clusterList.length, subtitle: clusterList.length > 0 ? `${activeClusters} active` : undefined, subtitleColor: 'text-emerald-600', iconBg: 'bg-emerald-500/10' },
    deployments: { href: '/deployments', icon: <BrandIcon name="argocd" size={20} />, label: 'Deployments', value: deploymentList.length, subtitle: deploymentList.length > 0 ? `${deploySuccessRate}% success` : undefined, subtitleColor: deploySuccessRate >= 80 ? 'text-emerald-600' : 'text-amber-600', iconBg: 'bg-violet-500/10' },
    pipelines: { href: '/pipelines', icon: <BrandIcon name="cicd" size={20} />, label: 'Pipelines', value: pipelineSourceList.length, iconBg: 'bg-amber-500/10', child: <Link href="/pipeline-builder" className="text-[11px] font-medium text-[var(--accent)] hover:underline">Build &rarr;</Link> },
    connections: { href: '/connections', icon: <BrandIcon name="plugin" size={20} />, label: 'Connections', value: connList.length, iconBg: 'bg-blue-500/10' },
    environments: { href: '/environments', icon: <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M2.25 15a4.5 4.5 0 004.5 4.5H18a3.75 3.75 0 001.332-7.257 3 3 0 00-3.758-3.848 5.25 5.25 0 00-10.233 2.33A4.502 4.502 0 002.25 15z" /></svg>, label: 'Environments', value: envList.length, iconBg: 'bg-teal-500/10' },
    'docker-containers': { href: '/docker-services', icon: <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg>, label: 'Containers', value: runningContainers, subtitle: `${dockerServiceList.length} services`, iconBg: 'bg-sky-500/10' },
    'gitops-repos': { href: '/gitops', icon: <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.99 8.99 0 017.843 4.582M12 3a8.99 8.99 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418" /></svg>, label: 'GitOps Repos', value: gitopsRepos.length, iconBg: 'bg-orange-500/10' },
  };

  const visibleStatCards = config.statCards.filter(k => k in statCardDefs);

  // Build quick actions based on profile (admin-only actions filtered; RBAC can override)
  const visibleQuickActions = config.quickActions.map(id => ALL_QUICK_ACTIONS[id]).filter(a => a && (!a.adminOnly || isAdmin || (a.permission ? hasPermission(a.permission, 'read') : false)));

  // Map pipeline runs for the widget
  const pipelineRunItems = pipelineRunList.map(r => {
    const source = pipelineSourceList.find(s => s.id === r.source_id);
    return {
      id: r.id,
      name: source?.name || `Pipeline ${r.source_id.slice(0, 8)}`,
      status: r.status,
      trigger_type: r.trigger_type,
      duration_ms: r.duration_ms,
      created_at: r.created_at,
    };
  });

  return (
    <div className="space-y-6 dash-mesh-bg -mx-6 -my-6 px-6 py-6 min-h-[calc(100vh-48px)]">
      {/* ─── Header ─────────────────────────────────────────── */}
      <div className="flex items-center justify-between dash-animate-in">
        <div>
          <h1 className="page-title">{getGreeting()}, {userName}</h1>
          <p className="page-subtitle">{config.description}</p>
        </div>
        <div className="flex items-center gap-3">
          <ProfileSelector current={profile} onChange={setProfile} />
          {!tourCompleted && (
            <Link href="/get-started" className="hidden md:inline text-[12px] text-[var(--accent)] hover:underline transition-colors">
              Guided tour
            </Link>
          )}
          <kbd className="hidden md:flex items-center gap-1 px-2.5 py-1 text-[10px] text-[var(--text-tertiary)] bg-[var(--surface)] border border-[var(--border)] rounded-lg">
            <span className="font-medium">⌘K</span>
            <span>Quick search</span>
          </kbd>
        </div>
      </div>

      {/* ─── Welcome Banner ─────────────────────────────────── */}
      {showWelcome && (
        <WelcomeBanner userName={userName} onDismiss={() => { setShowWelcome(false); try { localStorage.setItem('pepa-dashboard-welcome-dismissed', '1'); } catch {} }} />
      )}

      {/* ─── Stats Row ──────────────────────────────────────── */}
      <div className={`grid grid-cols-2 ${visibleStatCards.length <= 4 ? 'lg:grid-cols-4' : visibleStatCards.length <= 6 ? 'lg:grid-cols-3 xl:grid-cols-6' : 'lg:grid-cols-4 xl:grid-cols-4'} gap-4`}>
        {visibleStatCards.map((key, i) => {
          const card = statCardDefs[key];
          return (
            <StatCard
              key={key}
              href={card.href}
              icon={card.icon}
              label={card.label}
              value={card.value}
              subtitle={card.subtitle}
              subtitleColor={card.subtitleColor}
              iconBg={card.iconBg}
              delay={i + 1}
            >
              {card.child}
            </StatCard>
          );
        })}
      </div>

      {/* ─── Quick Actions ──────────────────────────────────── */}
      <div className="flex flex-wrap gap-2.5 dash-animate-in" style={{ animationDelay: '0.3s' }}>
        {visibleQuickActions.map((action, i) => (
          <QuickActionButton
            key={action.id}
            href={action.href}
            icon={<svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d={action.icon} /></svg>}
            label={action.label}
            primary={action.primary}
            delay={i + 1}
          />
        ))}
      </div>

      {/* ─── Main Content Grid ──────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* ─── Left Column ─────────────────────────────────── */}
        <div className="space-y-6">
          {/* Deployment Health */}
          {hasWidget('deployment-health') && deploymentList.length > 0 && (
            <DeploymentHealthWidget
              total={deploymentList.length}
              deployed={successfulDeploys}
              failed={failedDeploys}
              pending={pendingDeploys}
            />
          )}

          {/* Cluster Health */}
          {hasWidget('cluster-health') && clusterList.length > 0 && (
            <CollapsibleSection id="cluster-health" title="Cluster Health" defaultExpanded={true} action={<Link href="/clusters" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>}>
              <div className="divide-y divide-[var(--border-light)]">
                {clusterList.slice(0, 5).map((cluster, i) => (
                  <ClusterRow
                    key={cluster.id}
                    name={cluster.name}
                    environment={cluster.environment}
                    version={cluster.kubernetes_version}
                    nodes={cluster.node_count}
                    active={cluster.is_active}
                    delay={i + 1}
                  />
                ))}
              </div>
            </CollapsibleSection>
          )}

          {/* Services Catalog */}
          {hasWidget('services-catalog') && (
            <ServicesCatalogWidget services={serviceList} total={serviceTotal} />
          )}
        </div>

        {/* ─── Right Column ────────────────────────────────── */}
        <div className="space-y-6">
          {/* Pipeline Activity */}
          {hasWidget('pipeline-activity') && (
            <PipelineActivityWidget runs={pipelineRunItems} totalSources={pipelineSourceList.length} />
          )}

          {/* Environment Overview */}
          {hasWidget('environment-overview') && (
            <EnvironmentOverviewWidget environments={envList} />
          )}

          {/* GitOps Status */}
          {hasWidget('gitops-status') && (
            <GitOpsStatusWidget repos={gitopsRepos} />
          )}

          {/* Security & Compliance */}
          {hasWidget('security-compliance') && (
            <SecurityComplianceWidget
              vaultSecrets={vaultSecrets}
              vaultMode={vaultMode}
              needsRotation={vaultNeedsRotation}
              totalConnections={connList.length}
              recentAuditCount={recentAudit.length}
            />
          )}
        </div>
      </div>

      {/* ─── Full Width Sections ────────────────────────────── */}
      <div className="space-y-6">
        {/* Recent Activity */}
        {hasWidget('recent-activity') && (
          <CollapsibleSection id="recent-activity" title="Recent Activity" defaultExpanded={true} action={<Link href="/audit" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>}>
            <div className="divide-y divide-[var(--border-light)]">
              {recentAudit.length === 0 ? (
                <div className="px-4 py-12 text-center">
                  <div className="w-12 h-12 rounded-2xl bg-[var(--border-light)] flex items-center justify-center mx-auto mb-3">
                    <svg className="w-5 h-5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  </div>
                  <p className="text-[13px] text-[var(--text-secondary)] mb-1">No activity yet</p>
                  <p className="text-[11px] text-[var(--text-tertiary)]">Actions across the platform will appear here</p>
                </div>
              ) : (
                recentAudit.slice(0, 8).map((a, i) => (
                  <TimelineItem
                    key={a.id}
                    action={a.action}
                    entityType={a.entity_type}
                    time={new Date(a.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                    delay={i + 1}
                  />
                ))
              )}
            </div>
          </CollapsibleSection>
        )}

        {/* Enhanced System Status */}
        {hasWidget('system-status') && (
          <EnhancedSystemStatus
            connections={{ total: connList.length, healthy: connList.length }}
            clusters={{ total: clusterList.length, active: activeClusters }}
            deployments={{ total: deploymentList.length, successRate: deploySuccessRate }}
            vault={{ sealed: vaultSealed, secrets: vaultSecrets }}
          />
        )}
      </div>
    </div>
  );
}
