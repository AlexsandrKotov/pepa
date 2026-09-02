'use client';

import React from 'react';
import Link from 'next/link';
import type { Deployment, AuditEntry } from '@/lib/api';

/* ─── Types ─────────────────────────────────────────────── */

interface MyWorkspaceProps {
  userId: string;
  deployments: Deployment[];
  recentAudit: AuditEntry[];
  totalServices: number;
}

/* ─── Helpers ───────────────────────────────────────────── */

const statusColors: Record<string, string> = {
  deployed: 'bg-emerald-500',
  failed: 'bg-red-500',
  pending: 'bg-amber-500',
  progressing: 'bg-blue-500',
  cancelled: 'bg-gray-400',
  active: 'bg-emerald-500',
  running: 'bg-emerald-500',
  degraded: 'bg-amber-500',
  inactive: 'bg-gray-300',
};

const badgeColors: Record<string, string> = {
  deployed: 'badge-success',
  failed: 'badge-danger',
  pending: 'badge-warning',
  progressing: 'badge-info',
  cancelled: 'badge-default',
};

function formatRelativeTime(dateStr: string): string {
  const now = new Date();
  const d = new Date(dateStr);
  const diffSec = Math.floor((now.getTime() - d.getTime()) / 1000);
  if (diffSec < 60) return 'just now';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  return `${Math.floor(diffSec / 86400)}d ago`;
}

/* ─── My Workspace Widget ───────────────────────────────── */

export const MyWorkspaceWidget = React.memo(function MyWorkspaceWidget({ userId, deployments, recentAudit, totalServices }: MyWorkspaceProps) {
  // Get user's recent deployments
  const myDeployments = deployments
    .filter(d => d.created_by === userId || d.created_by === 'workflow-engine')
    .slice(0, 3);

  // Get user's recent audit actions (write operations only)
  const myActions = recentAudit
    .filter(a => a.user_id === userId && !['view', 'read', 'list', 'get'].includes(a.action?.toLowerCase() || ''))
    .slice(0, 3);

  // Count failed items that belong to user
  const myFailedDeploys = deployments.filter(d => d.status === 'failed' && (d.created_by === userId || d.created_by === 'workflow-engine')).length;

  const hasContent = myDeployments.length > 0 || myActions.length > 0 || myFailedDeploys > 0;

  if (!hasContent && totalServices === 0) {
    return null; // Don't render if nothing to show
  }

  return (
    <div className="dash-animate-in card" style={{ animationDelay: '0.1s' }}>
      <div className="card-header flex items-center justify-between">
        <span className="text-[13px] font-medium text-[var(--text-primary)]">My Workspace</span>
        {totalServices > 0 && (
          <Link href="/services" className="text-[12px] text-[var(--accent)] hover:underline">My services</Link>
        )}
      </div>
      <div className="px-4 pb-4 space-y-4">
        {/* My Recent Deployments */}
        {myDeployments.length > 0 && (
          <div>
            <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium mb-2">Recent Deployments</p>
            <div className="space-y-1.5">
              {myDeployments.map((d) => {
                const name = d.gitlab_project_name || d.image_repository || 'Deployment';
                return (
                  <Link key={d.id} href="/deployments" className="flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg hover:bg-[var(--bg)] transition-colors group">
                    <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${statusColors[d.status] || 'bg-gray-300'}`} />
                    <span className="text-[12px] text-[var(--text-primary)] truncate flex-1 group-hover:text-[var(--accent)] transition-colors">{name}</span>
                    <span className={`badge ${badgeColors[d.status] || 'badge-default'} text-[9px]`}>{d.status}</span>
                    <span className="text-[10px] text-[var(--text-tertiary)] whitespace-nowrap">{d.target_namespace || d.stage}</span>
                    <span className="text-[10px] text-[var(--text-tertiary)] whitespace-nowrap">{formatRelativeTime(d.created_at)}</span>
                  </Link>
                );
              })}
            </div>
          </div>
        )}

        {/* My Recent Actions */}
        {myActions.length > 0 && (
          <div>
            <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider font-medium mb-2">My Recent Actions</p>
            <div className="space-y-1">
              {myActions.map((a) => (
                <div key={a.id} className="flex items-center gap-2.5 px-2.5 py-1.5">
                  <div className="w-1.5 h-1.5 rounded-full bg-[var(--accent)] flex-shrink-0" />
                  <span className="badge badge-accent text-[9px]">{a.action}</span>
                  <span className="text-[11px] text-[var(--text-secondary)] truncate">{a.entity_type}</span>
                  <span className="text-[10px] text-[var(--text-tertiary)] ml-auto whitespace-nowrap">{formatRelativeTime(a.created_at)}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Quick Stats */}
        <div className="grid grid-cols-3 gap-2 pt-1">
          <Link href="/deployments" className="bg-[var(--bg)] rounded-lg px-3 py-2 text-center hover:bg-[var(--border-light)] transition-colors">
            <p className="text-[14px] font-bold text-[var(--text-primary)]">{deployments.filter(d => d.status === 'deployed').length}</p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Deployed</p>
          </Link>
          <Link href="/deployments" className="bg-[var(--bg)] rounded-lg px-3 py-2 text-center hover:bg-[var(--border-light)] transition-colors">
            <p className={`text-[14px] font-bold ${myFailedDeploys > 0 ? 'text-red-600' : 'text-[var(--text-primary)]'}`}>{myFailedDeploys}</p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Failed</p>
          </Link>
          <Link href="/services" className="bg-[var(--bg)] rounded-lg px-3 py-2 text-center hover:bg-[var(--border-light)] transition-colors">
            <p className="text-[14px] font-bold text-[var(--text-primary)]">{totalServices}</p>
            <p className="text-[9px] text-[var(--text-tertiary)] uppercase tracking-wider mt-0.5">Services</p>
          </Link>
        </div>
      </div>
    </div>
  );
});
MyWorkspaceWidget.displayName = 'MyWorkspaceWidget';
