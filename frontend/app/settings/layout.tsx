'use client';

import { usePathname } from 'next/navigation';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';
import Tabs from '@/components/Tabs';

const tabs = [
  { key: 'general', label: 'General', href: '/settings', icon: 'settings' },
  { key: 'auth', label: 'Authentication', href: '/settings/authentication', icon: 'shield' },
  { key: 'organization', label: 'Organization', href: '/settings/users', icon: 'users' },
  { key: 'observability', label: 'Observability', href: '/settings/observability', icon: 'chart' },
];

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { isAdmin, hasPermission, loading } = usePermission();

  // Determine active tab from pathname
  const activeKey = (() => {
    if (pathname === '/settings') return 'general';
    if (pathname.startsWith('/settings/authentication')) return 'auth';
    if (pathname.startsWith('/settings/users') || pathname.startsWith('/settings/teams')) return 'organization';
    if (pathname.startsWith('/settings/observability')) return 'observability';
    if (pathname.startsWith('/settings/workspaces')) return 'general';
    return 'general';
  })();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('settings', 'read')) {
    return <ForbiddenPage resource="settings" />;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="page-title-modern">Settings</h1>
        <p className="page-subtitle-modern">Configure platform, workspaces, and users</p>
      </div>

      <Tabs tabs={tabs} activeKey={activeKey} />

      {children}
    </div>
  );
}
