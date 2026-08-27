'use client';

import React, { useState, useEffect, useRef } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';

/* ─── Animated Counter ──────────────────────────────────── */

function AnimatedCounter({ value, duration = 800 }: { value: number; duration?: number }) {
  const [display, setDisplay] = useState(0);
  const ref = useRef<HTMLSpanElement>(null);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const start = performance.now();
    const step = (now: number) => {
      const progress = Math.min((now - start) / duration, 1);
      const eased = 1 - Math.pow(1 - progress, 3);
      setDisplay(Math.round(eased * value));
      if (progress < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }, [value, duration]);

  return <span ref={ref}>{display}</span>;
}

/* ─── Stat Card ─────────────────────────────────────────── */

interface StatCardProps {
  href: string;
  icon: React.ReactNode;
  label: string;
  value: number;
  subtitle?: string;
  subtitleColor?: string;
  iconBg: string;
  delay: number;
  accent?: boolean;
  children?: React.ReactNode;
}

export const StatCard = React.memo(function StatCard({ href, icon, label, value, subtitle, subtitleColor, iconBg, delay, accent, children }: StatCardProps) {
  const router = useRouter();

  const handleClick = (e: React.MouseEvent) => {
    // If click originated from an inner link, let it handle navigation
    if ((e.target as HTMLElement).closest('a')) return;
    router.push(href);
  };

  return (
    <div onClick={handleClick} className={`dash-animate-in dash-card-3d group card cursor-pointer glass-surface dash-delay-${delay}`} style={{ animationDelay: `${delay * 0.08}s` }}>
      <div className="card-body flex items-center gap-4">
        <div className={`w-11 h-11 rounded-xl ${iconBg} flex items-center justify-center flex-shrink-0 transition-transform duration-300 group-hover:scale-110`}>
          {icon}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-[11px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium">{label}</p>
          <p className="text-[26px] font-bold text-[var(--text-primary)] leading-tight tracking-tight">
            <AnimatedCounter value={value} />
          </p>
          {subtitle && (
            <p className={`text-[11px] ${subtitleColor || 'text-[var(--text-tertiary)]'}`}>{subtitle}</p>
          )}
          {children}
        </div>
        <svg className="w-4 h-4 text-[var(--text-tertiary)] opacity-0 group-hover:opacity-100 transition-opacity duration-200 ml-auto flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
        </svg>
      </div>
    </div>
  );
});
StatCard.displayName = 'StatCard';

/* ─── Quick Action Button ───────────────────────────────── */

interface QuickActionProps {
  href: string;
  icon: React.ReactNode;
  label: string;
  primary?: boolean;
  delay: number;
}

export const QuickActionButton = React.memo(function QuickActionButton({ href, icon, label, primary, delay }: QuickActionProps) {
  return (
    <Link
      href={href}
      className={`dash-animate-in inline-flex items-center gap-2 px-4 py-2 text-[12px] font-medium rounded-lg transition-all duration-200 hover:scale-[1.03] ${
        primary
          ? 'bg-[var(--accent)] text-white hover:opacity-85 shadow-sm'
          : 'bg-[var(--surface)] border border-[var(--border)] text-[var(--text-primary)] hover:border-[var(--text-tertiary)] hover:shadow-sm'
      }`}
      style={{ animationDelay: `${delay * 0.08}s` }}
    >
      {icon}
      {label}
    </Link>
  );
});
QuickActionButton.displayName = 'QuickActionButton';

/* ─── Activity Timeline Item ────────────────────────────── */

interface TimelineItemProps {
  action: string;
  entityType: string;
  time: string;
  delay: number;
}

export const TimelineItem = React.memo(function TimelineItem({ action, entityType, time, delay }: TimelineItemProps) {
  return (
    <div className="dash-animate-in flex items-center gap-3 px-4 py-3 hover:bg-[var(--bg)] transition-all duration-200" style={{ animationDelay: `${delay * 0.06}s` }}>
      <div className="w-1.5 h-1.5 rounded-full bg-[var(--accent)] flex-shrink-0" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="badge badge-accent text-[10px]">{action}</span>
          <span className="text-[12px] text-[var(--text-secondary)] truncate">{entityType}</span>
        </div>
      </div>
      <span className="text-[11px] text-[var(--text-tertiary)] whitespace-nowrap">{time}</span>
    </div>
  );
});
TimelineItem.displayName = 'TimelineItem';

/* ─── Deployment Row ────────────────────────────────────── */

interface DeploymentRowProps {
  name: string;
  namespace: string;
  status: string;
  date: string;
  delay: number;
}

export const DeploymentRow = React.memo(function DeploymentRow({ name, namespace, status, date, delay }: DeploymentRowProps) {
  const statusColors: Record<string, string> = {
    deployed: 'bg-emerald-500',
    failed: 'bg-red-500',
    pending: 'bg-amber-500',
  };
  const badgeColors: Record<string, string> = {
    deployed: 'badge-success',
    failed: 'badge-danger',
    pending: 'badge-warning',
  };

  return (
    <div className="dash-animate-in px-4 py-3 hover:bg-[var(--bg)] transition-all duration-200 group" style={{ animationDelay: `${delay * 0.06}s` }}>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3 min-w-0">
          <div className={`w-2 h-2 rounded-full flex-shrink-0 ${statusColors[status] || 'bg-gray-300'}`} />
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-[var(--text-primary)] truncate group-hover:text-[var(--accent)] transition-colors">{name}</p>
            <p className="text-[11px] text-[var(--text-tertiary)] truncate">{namespace}</p>
          </div>
        </div>
        <div className="flex items-center gap-2.5 flex-shrink-0">
          <span className={`badge ${badgeColors[status] || 'badge-default'}`}>{status}</span>
          <span className="text-[11px] text-[var(--text-tertiary)] whitespace-nowrap">{date}</span>
        </div>
      </div>
    </div>
  );
});
DeploymentRow.displayName = 'DeploymentRow';

/* ─── Cluster Row ───────────────────────────────────────── */

interface ClusterRowProps {
  name: string;
  environment: string;
  version: string;
  nodes: number;
  active: boolean;
  delay: number;
}

export const ClusterRow = React.memo(function ClusterRow({ name, environment, version, nodes, active, delay }: ClusterRowProps) {
  return (
    <Link
      href={`/clusters`}
      className="dash-animate-in flex items-center justify-between px-4 py-3 hover:bg-[var(--bg)] transition-all duration-200 group"
      style={{ animationDelay: `${delay * 0.06}s` }}
    >
      <div className="flex items-center gap-3">
        <div className="relative flex-shrink-0">
          <div className={`w-2.5 h-2.5 rounded-full ${active ? 'bg-emerald-500' : 'bg-gray-300'}`} />
          {active && (
            <div className="absolute inset-0 w-2.5 h-2.5 rounded-full bg-emerald-500 opacity-40 animate-ping" />
          )}
        </div>
        <div>
          <p className="text-[13px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">{name}</p>
          <p className="text-[11px] text-[var(--text-tertiary)]">{environment}</p>
        </div>
      </div>
      <div className="text-right">
        <p className="text-[12px] text-[var(--text-secondary)] font-mono">{version}</p>
        <p className="text-[10px] text-[var(--text-tertiary)]">{nodes} nodes</p>
      </div>
    </Link>
  );
});
ClusterRow.displayName = 'ClusterRow';

/* ─── System Status Widget ──────────────────────────────── */

interface SystemStatusProps {
  connections: number;
  activeClusters: number;
  deployed: number;
}

export const SystemStatus = React.memo(function SystemStatus({ connections, activeClusters, deployed }: SystemStatusProps) {
  return (
    <div className="dash-animate-in card overflow-hidden" style={{ animationDelay: '0.4s' }}>
      <div className="px-4 py-3 border-b border-[var(--border-light)]" style={{ background: 'linear-gradient(135deg, rgba(16,185,129,0.04), transparent)' }}>
        <div className="flex items-center gap-3">
          <div className="relative">
            <div className="w-2.5 h-2.5 rounded-full bg-emerald-500" />
            <div className="absolute inset-0 w-2.5 h-2.5 rounded-full bg-emerald-500 opacity-40 animate-ping" />
          </div>
          <div className="flex-1">
            <p className="text-[13px] font-semibold text-[var(--text-primary)]">All systems operational</p>
            <p className="text-[11px] text-[var(--text-tertiary)]">Platform healthy</p>
          </div>
          <span className="badge badge-success">Healthy</span>
        </div>
      </div>
      <div className="grid grid-cols-3 divide-x divide-[var(--border-light)]">
        {[
          { label: 'Connections', value: connections },
          { label: 'Active', value: activeClusters },
          { label: 'Deployed', value: deployed },
        ].map((item, i) => (
          <div key={i} className="py-4 text-center">
            <p className="text-[20px] font-bold text-[var(--text-primary)] tracking-tight">
              <AnimatedCounter value={item.value} />
            </p>
            <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">{item.label}</p>
          </div>
        ))}
      </div>
    </div>
  );
});
SystemStatus.displayName = 'SystemStatus';

/* ─── Deployment Health Widget ──────────────────────────── */

interface DeploymentHealthProps {
  total: number;
  deployed: number;
  failed: number;
  pending: number;
}

export const DeploymentHealthWidget = React.memo(function DeploymentHealthWidget({ total, deployed, failed, pending }: DeploymentHealthProps) {
  const successRate = total > 0 ? Math.round((deployed / total) * 100) : 0;
  const failRate = total > 0 ? Math.round((failed / total) * 100) : 0;

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.15s' }}>
      <div className="card-header flex items-center justify-between">
        <span className="text-[13px] font-medium text-[var(--text-primary)]">Deployment Health</span>
        <Link href="/deployments" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>
      </div>
      <div className="px-4 pb-4 space-y-3">
        {/* Success rate bar */}
        <div className="flex items-center gap-3">
          <div className="flex-1">
            <div className="flex items-center justify-between mb-1">
              <span className="text-[11px] text-[var(--text-tertiary)]">Success rate</span>
              <span className={`text-[13px] font-bold ${successRate >= 80 ? 'text-emerald-600' : successRate >= 50 ? 'text-amber-600' : 'text-red-600'}`}>{successRate}%</span>
            </div>
            <div className="h-2 bg-[var(--border-light)] rounded-full overflow-hidden flex">
              {total > 0 && (
                <>
                  <div className="bg-emerald-500 transition-all duration-500" style={{ width: `${(deployed / total) * 100}%` }} />
                  <div className="bg-amber-500 transition-all duration-500" style={{ width: `${(pending / total) * 100}%` }} />
                  <div className="bg-red-500 transition-all duration-500" style={{ width: `${(failed / total) * 100}%` }} />
                </>
              )}
            </div>
          </div>
        </div>
        {/* Counts */}
        <div className="grid grid-cols-3 gap-2">
          {[
            { label: 'Deployed', value: deployed, color: 'text-emerald-600', bg: 'bg-emerald-500/10' },
            { label: 'Pending', value: pending, color: 'text-amber-600', bg: 'bg-amber-500/10' },
            { label: 'Failed', value: failed, color: 'text-red-600', bg: 'bg-red-500/10' },
          ].map(item => (
            <div key={item.label} className={`${item.bg} rounded-lg px-3 py-2 text-center`}>
              <p className={`text-[18px] font-bold ${item.color}`}><AnimatedCounter value={item.value} /></p>
              <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">{item.label}</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
});
DeploymentHealthWidget.displayName = 'DeploymentHealthWidget';

/* ─── Pipeline Activity Widget ──────────────────────────── */

interface PipelineRunItem {
  id: string;
  name: string;
  status: string;
  trigger_type: string;
  duration_ms?: number;
  created_at: string;
}

interface PipelineActivityProps {
  runs: PipelineRunItem[];
  totalSources: number;
}

function formatDuration(ms?: number): string {
  if (!ms || ms <= 0) return '--';
  if (ms < 60000) return `${Math.round(ms / 1000)}s`;
  if (ms < 3600000) return `${Math.round(ms / 60000)}m`;
  return `${Math.round(ms / 3600000)}h ${Math.round((ms % 3600000) / 60000)}m`;
}

export const PipelineActivityWidget = React.memo(function PipelineActivityWidget({ runs, totalSources }: PipelineActivityProps) {
  const statusColors: Record<string, string> = {
    success: 'bg-emerald-500',
    failed: 'bg-red-500',
    running: 'bg-blue-500',
    pending: 'bg-amber-500',
    canceled: 'bg-gray-400',
  };
  const badgeColors: Record<string, string> = {
    success: 'badge-success',
    failed: 'badge-danger',
    running: 'badge-info',
    pending: 'badge-warning',
    canceled: 'badge-default',
  };

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.2s' }}>
      <div className="card-header flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Pipeline Activity</span>
          {totalSources > 0 && <span className="badge badge-default text-[10px]">{totalSources} sources</span>}
        </div>
        <Link href="/pipelines" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>
      </div>
      <div className="divide-y divide-[var(--border-light)]">
        {runs.length === 0 ? (
          <div className="px-4 py-10 text-center">
            <div className="w-10 h-10 rounded-2xl bg-[var(--border-light)] flex items-center justify-center mx-auto mb-2">
              <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
              </svg>
            </div>
            <p className="text-[12px] text-[var(--text-secondary)]">No pipeline runs yet</p>
            <Link href="/pipeline-builder" className="text-[11px] text-[var(--accent)] hover:underline mt-1 inline-block">Create a pipeline &rarr;</Link>
          </div>
        ) : (
          runs.slice(0, 5).map((run, i) => (
            <div key={run.id} className="dash-animate-in flex items-center gap-3 px-4 py-2.5 hover:bg-[var(--bg)] transition-all duration-200" style={{ animationDelay: `${i * 0.05}s` }}>
              <div className={`w-2 h-2 rounded-full flex-shrink-0 ${statusColors[run.status] || 'bg-gray-300'}`} />
              <div className="flex-1 min-w-0">
                <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">{run.name}</p>
                <p className="text-[10px] text-[var(--text-tertiary)]">{run.trigger_type} &middot; {formatDuration(run.duration_ms)}</p>
              </div>
              <span className={`badge ${badgeColors[run.status] || 'badge-default'} text-[10px]`}>{run.status}</span>
              <span className="text-[10px] text-[var(--text-tertiary)] whitespace-nowrap">{new Date(run.created_at).toLocaleDateString()}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
});
PipelineActivityWidget.displayName = 'PipelineActivityWidget';

/* ─── Environment Overview Widget ───────────────────────── */

interface EnvironmentItem {
  id: string;
  name: string;
  type?: string;
  status?: string;
  cluster?: string;
  namespace?: string;
  color?: string;
}

interface EnvironmentOverviewProps {
  environments: EnvironmentItem[];
}

export const EnvironmentOverviewWidget = React.memo(function EnvironmentOverviewWidget({ environments }: EnvironmentOverviewProps) {
  const statusColors: Record<string, string> = {
    active: 'bg-emerald-500',
    inactive: 'bg-gray-300',
    warning: 'bg-amber-500',
    error: 'bg-red-500',
  };

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.25s' }}>
      <div className="card-header flex items-center justify-between">
        <span className="text-[13px] font-medium text-[var(--text-primary)]">Environments</span>
        <Link href="/environments" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>
      </div>
      <div className="divide-y divide-[var(--border-light)]">
        {environments.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <p className="text-[12px] text-[var(--text-secondary)]">No environments configured</p>
            <Link href="/environments" className="text-[11px] text-[var(--accent)] hover:underline mt-1 inline-block">Add environment &rarr;</Link>
          </div>
        ) : (
          environments.slice(0, 6).map((env, i) => (
            <Link key={env.id} href="/environments" className="dash-animate-in flex items-center justify-between px-4 py-2.5 hover:bg-[var(--bg)] transition-all duration-200 group" style={{ animationDelay: `${i * 0.05}s` }}>
              <div className="flex items-center gap-3">
                <div className="w-3 h-3 rounded-full flex-shrink-0" style={{ backgroundColor: env.color || (env.status === 'active' ? '#10b981' : '#9ca3af') }} />
                <div>
                  <p className="text-[12px] font-medium text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">{env.name}</p>
                  {env.cluster && <p className="text-[10px] text-[var(--text-tertiary)]">{env.cluster}{env.namespace ? ` / ${env.namespace}` : ''}</p>}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {env.type && <span className="badge badge-default text-[10px]">{env.type}</span>}
                <div className={`w-2 h-2 rounded-full ${statusColors[env.status || 'active'] || 'bg-gray-300'}`} />
              </div>
            </Link>
          ))
        )}
      </div>
    </div>
  );
});
EnvironmentOverviewWidget.displayName = 'EnvironmentOverviewWidget';

/* ─── GitOps Status Widget ──────────────────────────────── */

interface GitOpsRepoItem {
  id: string;
  name: string;
  scan_status: string;
  engine_type: string;
  branch: string;
  last_scanned_at?: string;
  repo_url: string;
}

interface GitOpsStatusProps {
  repos: GitOpsRepoItem[];
}

export const GitOpsStatusWidget = React.memo(function GitOpsStatusWidget({ repos }: GitOpsStatusProps) {
  const scanStatusColors: Record<string, string> = {
    synced: 'text-emerald-600 bg-emerald-500/10',
    pending: 'text-amber-600 bg-amber-500/10',
    error: 'text-red-600 bg-red-500/10',
    scanning: 'text-blue-600 bg-blue-500/10',
  };

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.3s' }}>
      <div className="card-header flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">GitOps Repos</span>
          {repos.length > 0 && <span className="badge badge-default text-[10px]">{repos.length}</span>}
        </div>
        <Link href="/gitops" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>
      </div>
      <div className="divide-y divide-[var(--border-light)]">
        {repos.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <p className="text-[12px] text-[var(--text-secondary)]">No GitOps repositories</p>
            <Link href="/gitops" className="text-[11px] text-[var(--accent)] hover:underline mt-1 inline-block">Configure GitOps &rarr;</Link>
          </div>
        ) : (
          repos.slice(0, 5).map((repo, i) => (
            <Link key={repo.id} href="/gitops" className="dash-animate-in flex items-center justify-between px-4 py-2.5 hover:bg-[var(--bg)] transition-all duration-200 group" style={{ animationDelay: `${i * 0.05}s` }}>
              <div className="flex items-center gap-3 min-w-0">
                <div className="w-8 h-8 rounded-lg bg-[var(--border-light)] flex items-center justify-center flex-shrink-0">
                  <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.99 8.99 0 017.843 4.582M12 3a8.99 8.99 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418" />
                  </svg>
                </div>
                <div className="min-w-0">
                  <p className="text-[12px] font-medium text-[var(--text-primary)] truncate group-hover:text-[var(--accent)] transition-colors">{repo.name}</p>
                  <p className="text-[10px] text-[var(--text-tertiary)]">{repo.engine_type} &middot; {repo.branch}</p>
                </div>
              </div>
              <span className={`badge text-[10px] ${scanStatusColors[repo.scan_status] || 'badge-default'}`}>{repo.scan_status}</span>
            </Link>
          ))
        )}
      </div>
    </div>
  );
});
GitOpsStatusWidget.displayName = 'GitOpsStatusWidget';

/* ─── Security & Compliance Widget ──────────────────────── */

interface SecurityComplianceProps {
  vaultSecrets: number;
  vaultMode: string;
  needsRotation: boolean;
  totalConnections: number;
  recentAuditCount: number;
}

export const SecurityComplianceWidget = React.memo(function SecurityComplianceWidget({ vaultSecrets, vaultMode, needsRotation, totalConnections, recentAuditCount }: SecurityComplianceProps) {
  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.2s' }}>
      <div className="card-header flex items-center justify-between">
        <span className="text-[13px] font-medium text-[var(--text-primary)]">Security & Compliance</span>
        <Link href="/vault" className="text-[12px] text-[var(--accent)] hover:underline">View vault</Link>
      </div>
      <div className="px-4 pb-4 space-y-3">
        {/* Vault status */}
        <div className="flex items-center gap-3 p-3 rounded-lg bg-[var(--bg)]">
          <div className="w-9 h-9 rounded-lg bg-violet-500/10 flex items-center justify-center flex-shrink-0">
            <svg className="w-4 h-4 text-violet-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
            </svg>
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-[12px] font-medium text-[var(--text-primary)]">Vault ({vaultMode || 'N/A'})</p>
            <p className="text-[10px] text-[var(--text-tertiary)]">{vaultSecrets} secrets stored</p>
          </div>
          {needsRotation && (
            <span className="badge badge-warning text-[10px]">Rotation needed</span>
          )}
          {!needsRotation && vaultSecrets > 0 && (
            <span className="badge badge-success text-[10px]">Secure</span>
          )}
        </div>
        {/* Stats grid */}
        <div className="grid grid-cols-3 gap-2">
          <div className="bg-violet-500/5 rounded-lg px-3 py-2 text-center">
            <p className="text-[16px] font-bold text-violet-600"><AnimatedCounter value={vaultSecrets} /></p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Secrets</p>
          </div>
          <div className="bg-blue-500/5 rounded-lg px-3 py-2 text-center">
            <p className="text-[16px] font-bold text-blue-600"><AnimatedCounter value={totalConnections} /></p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Connections</p>
          </div>
          <div className="bg-amber-500/5 rounded-lg px-3 py-2 text-center">
            <p className="text-[16px] font-bold text-amber-600"><AnimatedCounter value={recentAuditCount} /></p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Audit (24h)</p>
          </div>
        </div>
      </div>
    </div>
  );
});
SecurityComplianceWidget.displayName = 'SecurityComplianceWidget';

/* ─── Services Catalog Widget ───────────────────────────── */

interface ServiceItem {
  id: string;
  name: string;
  status: string;
  namespace: string;
  language?: string;
  framework?: string;
}

interface ServicesCatalogProps {
  services: ServiceItem[];
  total: number;
}

export const ServicesCatalogWidget = React.memo(function ServicesCatalogWidget({ services, total }: ServicesCatalogProps) {
  const statusColors: Record<string, string> = {
    active: 'bg-emerald-500',
    running: 'bg-emerald-500',
    inactive: 'bg-gray-300',
    error: 'bg-red-500',
    degraded: 'bg-amber-500',
  };

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.25s' }}>
      <div className="card-header flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Services</span>
          {total > 0 && <span className="badge badge-default text-[10px]">{total}</span>}
        </div>
        <Link href="/services" className="text-[12px] text-[var(--accent)] hover:underline">View all</Link>
      </div>
      <div className="divide-y divide-[var(--border-light)]">
        {services.length === 0 ? (
          <div className="px-4 py-10 text-center">
            <p className="text-[12px] text-[var(--text-secondary)]">No services registered</p>
            <Link href="/services/new" className="text-[11px] text-[var(--accent)] hover:underline mt-1 inline-block">Deploy a service &rarr;</Link>
          </div>
        ) : (
          services.slice(0, 5).map((svc, i) => (
            <Link key={svc.id} href={`/services`} className="dash-animate-in flex items-center justify-between px-4 py-2.5 hover:bg-[var(--bg)] transition-all duration-200 group" style={{ animationDelay: `${i * 0.05}s` }}>
              <div className="flex items-center gap-3 min-w-0">
                <div className={`w-2 h-2 rounded-full flex-shrink-0 ${statusColors[svc.status] || 'bg-gray-300'}`} />
                <div className="min-w-0">
                  <p className="text-[12px] font-medium text-[var(--text-primary)] truncate group-hover:text-[var(--accent)] transition-colors">{svc.name}</p>
                  <p className="text-[10px] text-[var(--text-tertiary)]">{svc.namespace}{svc.language ? ` · ${svc.language}` : ''}</p>
                </div>
              </div>
              <span className={`badge ${svc.status === 'active' || svc.status === 'running' ? 'badge-success' : 'badge-default'} text-[10px]`}>{svc.status}</span>
            </Link>
          ))
        )}
      </div>
    </div>
  );
});
ServicesCatalogWidget.displayName = 'ServicesCatalogWidget';

/* ─── Enhanced System Status ────────────────────────────── */

interface EnhancedSystemStatusProps {
  connections: { total: number; healthy: number };
  clusters: { total: number; active: number };
  deployments: { total: number; successRate: number };
  vault: { sealed: boolean; secrets: number };
}

export const EnhancedSystemStatus = React.memo(function EnhancedSystemStatus({ connections, clusters, deployments, vault }: EnhancedSystemStatusProps) {
  // Compute overall health score (0-100)
  let score = 100;
  if (connections.total > 0) score -= Math.round(((connections.total - connections.healthy) / connections.total) * 25);
  if (clusters.total > 0) score -= Math.round(((clusters.total - clusters.active) / clusters.total) * 25);
  if (deployments.total > 0) score -= Math.round((1 - deployments.successRate / 100) * 25);
  if (vault.sealed) score -= 25;
  score = Math.max(0, Math.min(100, score));

  const healthLabel = score >= 90 ? 'Healthy' : score >= 60 ? 'Degraded' : 'Critical';
  const healthColor = score >= 90 ? 'text-emerald-600' : score >= 60 ? 'text-amber-600' : 'text-red-600';
  const healthBg = score >= 90 ? 'bg-emerald-500' : score >= 60 ? 'bg-amber-500' : 'bg-red-500';
  const badgeClass = score >= 90 ? 'badge-success' : score >= 60 ? 'badge-warning' : 'badge-danger';

  return (
    <div className="dash-animate-in card overflow-hidden" style={{ animationDelay: '0.4s' }}>
      <div className="px-4 py-3 border-b border-[var(--border-light)]" style={{ background: `linear-gradient(135deg, ${score >= 90 ? 'rgba(16,185,129,0.04)' : score >= 60 ? 'rgba(245,158,11,0.04)' : 'rgba(239,68,68,0.04)'}, transparent)` }}>
        <div className="flex items-center gap-3">
          <div className="relative">
            <div className={`w-2.5 h-2.5 rounded-full ${healthBg}`} />
            <div className={`absolute inset-0 w-2.5 h-2.5 rounded-full ${healthBg} opacity-40 animate-ping`} />
          </div>
          <div className="flex-1">
            <p className="text-[13px] font-semibold text-[var(--text-primary)]">Platform Health: <span className={healthColor}>{score}/100</span></p>
            <p className="text-[11px] text-[var(--text-tertiary)]">{healthLabel}</p>
          </div>
          <span className={`badge ${badgeClass}`}>{healthLabel}</span>
        </div>
      </div>
      <div className="grid grid-cols-4 divide-x divide-[var(--border-light)]">
        {[
          { label: 'Connections', value: `${connections.healthy}/${connections.total}`, sub: connections.healthy === connections.total ? 'All healthy' : `${connections.total - connections.healthy} issues` },
          { label: 'Clusters', value: `${clusters.active}/${clusters.total}`, sub: clusters.active === clusters.total ? 'All active' : `${clusters.total - clusters.active} inactive` },
          { label: 'Deploy Rate', value: `${deployments.successRate}%`, sub: `${deployments.total} total` },
          { label: 'Vault', value: vault.sealed ? 'Sealed' : 'Open', sub: `${vault.secrets} secrets` },
        ].map((item, i) => (
          <div key={i} className="py-3 text-center">
            <p className="text-[16px] font-bold text-[var(--text-primary)] tracking-tight">{item.value}</p>
            <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">{item.label}</p>
            <p className="text-[9px] text-[var(--text-tertiary)] mt-0.5">{item.sub}</p>
          </div>
        ))}
      </div>
    </div>
  );
});
EnhancedSystemStatus.displayName = 'EnhancedSystemStatus';
