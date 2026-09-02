'use client';

import { useMemo } from 'react';
import type { Deployment, PipelineRun, PipelineSource, Connection } from '@/lib/api';

/* ─── Smart Action Definition ───────────────────────────── */

export interface SmartAction {
  id: string;
  href: string;
  label: string;
  icon: string; // SVG path
  priority: number; // lower = higher priority, shown first
  badge?: string; // optional badge text (e.g. "3")
  badgeColor?: string; // badge color class
  primary?: boolean;
}

/* ─── SVG Paths ─────────────────────────────────────────── */

const PATHS = {
  pipeline: 'M12 4.5v15m7.5-7.5h-15',
  deploy: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12',
  connection: 'M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.54a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374',
  vault: 'M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z',
  cluster: 'M12 4.5v15m7.5-7.5h-15',
  viewPipelines: 'M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z',
  audit: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08',
};

/* ─── Input Data ────────────────────────────────────────── */

interface SmartActionsInput {
  deployments: Deployment[];
  pipelineRuns: PipelineRun[];
  pipelineSources: PipelineSource[];
  connections: Connection[];
  vaultNeedsRotation: boolean;
  vaultSecrets: number;
  inactiveClusters: number;
  isAdmin: boolean;
}

/* ─── Hook ──────────────────────────────────────────────── */

export function useSmartActions(input: SmartActionsInput): SmartAction[] {
  return useMemo(() => {
    const actions: SmartAction[] = [];

    const failedDeploys = input.deployments.filter(d => d.status === 'failed');
    const failedRuns = input.pipelineRuns.filter(r => r.status === 'failed');
    const unhealthyConns = input.connections.filter(c => c.status !== 'connected' && c.status !== 'active');

    // 1. Critical: Failed deployments
    if (failedDeploys.length > 0) {
      actions.push({
        id: 'view-failed-deploys',
        href: '/deployments',
        label: 'View Failed Deployments',
        icon: PATHS.deploy,
        priority: 1,
        badge: String(failedDeploys.length),
        badgeColor: 'bg-red-500/15 text-red-500',
      });
    }

    // 2. Critical: Failed pipeline runs
    if (failedRuns.length > 0) {
      actions.push({
        id: 'view-failed-pipelines',
        href: '/pipelines',
        label: 'View Failed Pipelines',
        icon: PATHS.viewPipelines,
        priority: 2,
        badge: String(failedRuns.length),
        badgeColor: 'bg-red-500/15 text-red-500',
      });
    }

    // 3. Warning: Vault rotation
    if (input.vaultNeedsRotation && input.vaultSecrets > 0) {
      actions.push({
        id: 'rotate-vault',
        href: '/vault',
        label: 'Rotate Vault Secrets',
        icon: PATHS.vault,
        priority: 3,
        badgeColor: 'bg-amber-500/15 text-amber-600',
      });
    }

    // 4. Warning: Unhealthy connections
    if (unhealthyConns.length > 0) {
      actions.push({
        id: 'check-connections',
        href: '/connections',
        label: 'Check Connections',
        icon: PATHS.connection,
        priority: 4,
        badge: String(unhealthyConns.length),
        badgeColor: 'bg-amber-500/15 text-amber-600',
      });
    }

    // 5. Setup: No connections exist
    if (input.connections.length === 0) {
      actions.push({
        id: 'add-connection',
        href: '/connections',
        label: 'Add Connection',
        icon: PATHS.connection,
        priority: 5,
        primary: true,
      });
    }

    // 6. Setup: No services deployed
    if (input.deployments.length === 0) {
      actions.push({
        id: 'deploy-service',
        href: '/services/new',
        label: 'Deploy Your First Service',
        icon: PATHS.deploy,
        priority: 6,
        primary: true,
      });
    }

    // 7. Info: Inactive clusters
    if (input.inactiveClusters > 0) {
      actions.push({
        id: 'check-clusters',
        href: '/clusters',
        label: 'Check Clusters',
        icon: PATHS.cluster,
        priority: 7,
        badge: String(input.inactiveClusters),
        badgeColor: 'bg-blue-500/15 text-blue-600',
      });
    }

    // 8. Default actions (always available, lower priority)
    if (input.pipelineSources.length > 0) {
      actions.push({
        id: 'new-pipeline',
        href: '/pipeline-builder',
        label: 'New Pipeline',
        icon: PATHS.pipeline,
        priority: 50,
        primary: actions.length === 0, // primary only if no smart actions
      });
    }

    if (input.deployments.length > 0 || input.connections.length > 0) {
      actions.push({
        id: 'deploy-service-default',
        href: '/services/new',
        label: 'Deploy Service',
        icon: PATHS.deploy,
        priority: 51,
      });
    }

    // Admin defaults
    if (input.isAdmin) {
      actions.push({
        id: 'audit-log',
        href: '/audit',
        label: 'Audit Log',
        icon: PATHS.audit,
        priority: 60,
      });
    }

    // Sort by priority and take top 5
    actions.sort((a, b) => a.priority - b.priority);
    return actions.slice(0, 5);
  }, [input]);
}
