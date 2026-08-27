'use client';

import { useState, useEffect, useCallback } from 'react';
import { getStoredUser } from '@/lib/api';

// ── Profile Types ────────────────────────────────────────────

export type DashboardProfile = 'overview' | 'operations' | 'development' | 'administration';

export interface DashboardProfileConfig {
  id: DashboardProfile;
  label: string;
  description: string;
  icon: string; // SVG path
  // Which stat cards to show (by key)
  statCards: string[];
  // Which widget sections to show
  widgets: string[];
  // Which quick actions to show
  quickActions: string[];
}

// All available widget keys
export const ALL_WIDGETS = [
  'deployment-health',
  'cluster-health',
  'services-catalog',
  'pipeline-activity',
  'environment-overview',
  'gitops-status',
  'security-compliance',
  'recent-activity',
  'system-status',
] as const;

export type WidgetKey = typeof ALL_WIDGETS[number];

// Profile definitions
export const PROFILE_CONFIGS: Record<DashboardProfile, DashboardProfileConfig> = {
  overview: {
    id: 'overview',
    label: 'Overview',
    description: 'Balanced view for everyone',
    icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
    statCards: ['services', 'clusters', 'deployments', 'pipelines', 'connections', 'environments'],
    widgets: ['deployment-health', 'cluster-health', 'services-catalog', 'pipeline-activity', 'recent-activity', 'system-status'],
    quickActions: ['new-pipeline', 'deploy-service', 'add-connection'],
  },
  operations: {
    id: 'operations',
    label: 'Operations',
    description: 'DevOps / SRE focused',
    icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
    statCards: ['clusters', 'deployments', 'pipelines', 'environments', 'docker-containers', 'gitops-repos'],
    widgets: ['deployment-health', 'cluster-health', 'pipeline-activity', 'environment-overview', 'gitops-status', 'recent-activity', 'system-status'],
    quickActions: ['add-cluster', 'check-drift', 'view-pipelines'],
  },
  development: {
    id: 'development',
    label: 'Development',
    description: 'Developer focused',
    icon: 'M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4',
    statCards: ['services', 'pipelines', 'deployments', 'environments'],
    widgets: ['services-catalog', 'pipeline-activity', 'deployment-health', 'environment-overview', 'recent-activity'],
    quickActions: ['new-pipeline', 'browse-services', 'my-scorecards'],
  },
  administration: {
    id: 'administration',
    label: 'Administration',
    description: 'Platform admin focused',
    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
    statCards: ['connections', 'clusters', 'deployments', 'services'],
    widgets: ['security-compliance', 'deployment-health', 'cluster-health', 'recent-activity', 'system-status'],
    quickActions: ['audit-log', 'vault-secrets', 'manage-users'],
  },
};

// ── Hook ─────────────────────────────────────────────────────

const LS_PREFIX = 'pepa-dashboard-profile';

function getStorageKey(): string {
  const user = getStoredUser();
  return user?.id ? `${LS_PREFIX}-${user.id}` : LS_PREFIX;
}

export function useDashboardProfile() {
  const [profile, setProfileState] = useState<DashboardProfile>('overview');

  // Load from localStorage on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem(getStorageKey());
      if (saved && saved in PROFILE_CONFIGS) {
        setProfileState(saved as DashboardProfile);
      }
    } catch { /* ignore */ }
  }, []);

  const setProfile = useCallback((p: DashboardProfile) => {
    setProfileState(p);
    try {
      localStorage.setItem(getStorageKey(), p);
    } catch { /* ignore */ }
  }, []);

  const config = PROFILE_CONFIGS[profile];

  const hasWidget = useCallback((widget: WidgetKey) => {
    return config.widgets.includes(widget);
  }, [config]);

  return {
    profile,
    setProfile,
    config,
    hasWidget,
  };
}
