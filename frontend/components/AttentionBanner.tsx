'use client';

import React from 'react';
import Link from 'next/link';

/* ─── Attention Item Types ──────────────────────────────── */

export interface AttentionItem {
  id: string;
  severity: 'critical' | 'warning' | 'info';
  title: string;
  description: string;
  href: string;
  actionLabel: string;
  icon: string; // SVG path
  time?: string; // relative time e.g. "12 min ago"
}

/* ─── Severity Styles ───────────────────────────────────── */

const severityStyles = {
  critical: {
    bg: 'bg-red-500/5',
    border: 'border-red-500/20',
    icon: 'text-red-500',
    badge: 'bg-red-500/10 text-red-600',
    dot: 'bg-red-500',
    actionBg: 'hover:bg-red-500/10',
  },
  warning: {
    bg: 'bg-amber-500/5',
    border: 'border-amber-500/20',
    icon: 'text-amber-600',
    badge: 'bg-amber-500/10 text-amber-600',
    dot: 'bg-amber-500',
    actionBg: 'hover:bg-amber-500/10',
  },
  info: {
    bg: 'bg-blue-500/5',
    border: 'border-blue-500/20',
    icon: 'text-blue-600',
    badge: 'bg-blue-500/10 text-blue-600',
    dot: 'bg-blue-500',
    actionBg: 'hover:bg-blue-500/10',
  },
};

/* ─── Attention Banner ──────────────────────────────────── */

interface AttentionBannerProps {
  items: AttentionItem[];
}

export const AttentionBanner = React.memo(function AttentionBanner({ items }: AttentionBannerProps) {
  if (items.length === 0) return null;

  const criticalCount = items.filter(i => i.severity === 'critical').length;
  const maxSeverity: 'critical' | 'warning' | 'info' = criticalCount > 0 ? 'critical' : items.some(i => i.severity === 'warning') ? 'warning' : 'info';
  const style = severityStyles[maxSeverity];

  const headerLabel = items.length === 1
    ? '1 item needs your attention'
    : `${items.length} items need your attention`;

  return (
    <div className={`dash-animate-in rounded-xl border ${style.border} ${style.bg} overflow-hidden`}>
      {/* Header */}
      <div className="px-4 py-2.5 flex items-center gap-2.5 border-b border-[var(--border-light)]">
        <div className="relative flex-shrink-0">
          <div className={`w-2 h-2 rounded-full ${style.dot}`} />
          {maxSeverity === 'critical' && (
            <div className={`absolute inset-0 w-2 h-2 rounded-full ${style.dot} opacity-40 animate-ping`} />
          )}
        </div>
        <span className={`text-[12px] font-semibold ${style.icon}`}>{headerLabel}</span>
        {criticalCount > 0 && (
          <span className={`${style.badge} text-[10px] px-1.5 py-0.5 rounded-full font-medium`}>
            {criticalCount} critical
          </span>
        )}
      </div>

      {/* Items */}
      <div className="divide-y divide-[var(--border-light)]">
        {items.slice(0, 5).map((item, i) => {
          const itemStyle = severityStyles[item.severity];
          return (
            <div
              key={item.id}
              className="flex items-center gap-3 px-4 py-2.5 group"
              style={{ animationDelay: `${i * 0.05}s` }}
            >
              <div className={`w-7 h-7 rounded-lg ${itemStyle.bg} flex items-center justify-center flex-shrink-0`}>
                <svg className={`w-3.5 h-3.5 ${itemStyle.icon}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
                </svg>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-[12px] font-medium text-[var(--text-primary)] truncate">{item.title}</p>
                <p className="text-[10px] text-[var(--text-tertiary)] truncate">{item.description}</p>
              </div>
              {item.time && (
                <span className="text-[10px] text-[var(--text-tertiary)] whitespace-nowrap hidden sm:block">{item.time}</span>
              )}
              <Link
                href={item.href}
                className={`shrink-0 text-[11px] font-medium ${itemStyle.icon} ${itemStyle.actionBg} px-2 py-1 rounded-md transition-colors opacity-0 group-hover:opacity-100`}
              >
                {item.actionLabel} &rarr;
              </Link>
            </div>
          );
        })}
      </div>
    </div>
  );
});
AttentionBanner.displayName = 'AttentionBanner';

/* ─── Attention Builder ─────────────────────────────────── */
// Builds attention items from dashboard data

import type { Deployment, PipelineRun, PipelineSource } from '@/lib/api';

interface AttentionData {
  deployments: Deployment[];
  pipelineRuns: PipelineRun[];
  pipelineSources: PipelineSource[];
  vaultNeedsRotation: boolean;
  vaultSecrets: number;
  unhealthyConnections: number;
  totalConnections: number;
  inactiveClusters: number;
  totalClusters: number;
}

function formatRelativeTime(dateStr: string): string {
  const now = new Date();
  const d = new Date(dateStr);
  const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
  if (diffSec < 60) return 'just now';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  return `${Math.floor(diffSec / 86400)}d ago`;
}

const ICONS = {
  deploy: 'M12 4.5v15m7.5-7.5h-15',
  pipeline: 'M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z',
  vault: 'M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z',
  connection: 'M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.54a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374',
  cluster: 'M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3',
};

export function buildAttentionItems(data: AttentionData): AttentionItem[] {
  const items: AttentionItem[] = [];

  // Failed deployments
  const failedDeploys = data.deployments.filter(d => d.status === 'failed');
  for (const d of failedDeploys.slice(0, 2)) {
    const name = d.gitlab_project_name || d.image_repository || 'Deployment';
    items.push({
      id: `deploy-fail-${d.id}`,
      severity: 'critical',
      title: `${name} deployment failed`,
      description: d.error_message ? d.error_message.slice(0, 80) : `Failed in ${d.target_namespace || d.stage || 'unknown'}`,
      href: `/deployments`,
      actionLabel: 'View',
      icon: ICONS.deploy,
      time: formatRelativeTime(d.updated_at),
    });
  }

  // Failed pipeline runs
  const failedRuns = data.pipelineRuns.filter(r => r.status === 'failed');
  for (const r of failedRuns.slice(0, 2)) {
    const source = data.pipelineSources.find(s => s.id === r.source_id);
    const name = source?.name || 'Pipeline';
    items.push({
      id: `pipeline-fail-${r.id}`,
      severity: 'critical',
      title: `${name} pipeline failed`,
      description: r.error_message ? r.error_message.slice(0, 80) : `Run failed`,
      href: `/pipelines`,
      actionLabel: 'View',
      icon: ICONS.pipeline,
      time: formatRelativeTime(r.created_at),
    });
  }

  // Vault rotation needed
  if (data.vaultNeedsRotation && data.vaultSecrets > 0) {
    items.push({
      id: 'vault-rotation',
      severity: 'warning',
      title: 'Vault secret rotation overdue',
      description: `${data.vaultSecrets} secrets may need rotation`,
      href: '/vault',
      actionLabel: 'Review',
      icon: ICONS.vault,
    });
  }

  // Unhealthy connections
  if (data.unhealthyConnections > 0) {
    items.push({
      id: 'conn-unhealthy',
      severity: 'warning',
      title: `${data.unhealthyConnections} connection${data.unhealthyConnections > 1 ? 's' : ''} unhealthy`,
      description: `Out of ${data.totalConnections} total connections`,
      href: '/connections',
      actionLabel: 'Check',
      icon: ICONS.connection,
    });
  }

  // Inactive clusters
  if (data.inactiveClusters > 0) {
    items.push({
      id: 'cluster-inactive',
      severity: 'info',
      title: `${data.inactiveClusters} cluster${data.inactiveClusters > 1 ? 's' : ''} inactive`,
      description: `Out of ${data.totalClusters} total clusters`,
      href: '/clusters',
      actionLabel: 'View',
      icon: ICONS.cluster,
    });
  }

  return items;
}
