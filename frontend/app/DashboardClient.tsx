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
  StatCard, QuickActionButton, ClusterRow,
  DeploymentHealthWidget, PipelineActivityWidget, EnvironmentOverviewWidget,
  GitOpsStatusWidget, SecurityComplianceWidget, ServicesCatalogWidget,
  PlatformHealthBar,
} from '@/components/DashboardWidgets';
import { AttentionBanner, buildAttentionItems } from '@/components/AttentionBanner';
import { MyWorkspaceWidget } from '@/components/MyWorkspaceWidget';
import { useSmartActions } from '@/hooks/useSmartActions';
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
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);

  const user = getStoredUser();
  const userName = user?.name?.split(' ')[0] || 'User';

  // Smart actions hook (must be called before any early return)
  const smartActions = useSmartActions({
    deployments: data?.deploymentList ?? [],
    pipelineRuns: data?.pipelineRunList ?? [],
    pipelineSources: data?.pipelineSourceList ?? [],
    connections: data?.connList ?? [],
    vaultNeedsRotation: data?.vaultNeedsRotation ?? false,
    vaultSecrets: data?.vaultSecrets ?? 0,
    inactiveClusters: data ? data.clusterList.length - data.clusterList.filter(c => c.is_active).length : 0,
    isAdmin,
  });

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
        setLastUpdated(new Date());
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
  const unhealthyConnections = connList.filter(c => c.status !== 'connected' && c.status !== 'active').length;
  const inactiveClusters = clusterList.length - activeClusters;

  // Build attention items
  const attentionItems = buildAttentionItems({
    deployments: deploymentList,
    pipelineRuns: pipelineRunList,
    pipelineSources: pipelineSourceList,
    vaultNeedsRotation,
    vaultSecrets,
    unhealthyConnections,
    totalConnections: connList.length,
    inactiveClusters,
    totalClusters: clusterList.length,
  });

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
          <p className="page-subtitle">
            {config.description}
            {lastUpdated && (
              <span className="ml-2 text-[11px] text-[var(--text-tertiary)]">
                Updated {lastUpdated.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })}
              </span>
            )}
          </p>
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

      {/* ─── Attention Banner ────────────────────────────────── */}
      {attentionItems.length > 0 && (
        <AttentionBanner items={attentionItems} />
      )}

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
        {smartActions.map((action, i) => (
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
          {/* My Workspace */}
          {hasWidget('my-workspace') && user && (
            <MyWorkspaceWidget
              userId={user.id}
              deployments={deploymentList}
              recentAudit={recentAudit}
              totalServices={serviceTotal}
            />
          )}

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

      {/* ─── Platform Health Bar ────────────────────────────── */}
      {hasWidget('platform-health') && (
        <PlatformHealthBar
          connections={{ total: connList.length, healthy: connList.length - unhealthyConnections }}
          clusters={{ total: clusterList.length, active: activeClusters }}
          deployments={{ total: deploymentList.length, successRate: deploySuccessRate }}
          vault={{ sealed: vaultSealed, secrets: vaultSecrets }}
        />
      )}
    </div>
  );
}
